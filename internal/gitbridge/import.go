// Package gitbridge moves history between git and a-git.
//
// Import reads the output of `git fast-export`, whose commits already list
// file modifications relative to their first parent — the same shape as an
// a-git changeset — and ingests them in order, parents before children.
// Export does the reverse, producing a `git fast-import` stream, so a git
// mirror can be kept for tools that still need one.
package gitbridge

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mattparavel/a-git/internal/client"
	"github.com/mattparavel/a-git/internal/object"
)

// ImportOptions controls an import.
type ImportOptions struct {
	GitDir string   // path to the git repository
	Refs   []string // git refs/revisions to export (default: HEAD)
	// RefMap renames git refs to a-git refs; by default "refs/heads/X" -> "X".
	RefMap func(gitRef string) string
	// StateDir keeps marks between runs so later imports are incremental.
	StateDir string
	Batch    int // changesets per import request (default 500)
	Progress func(format string, args ...interface{})
	Warn     func(format string, args ...interface{})
}

// ImportStats summarises an import.
type ImportStats struct {
	Commits int               `json:"commits"`
	Blobs   int               `json:"blobs"`
	Skipped int               `json:"skipped_submodules"`
	Refs    map[string]string `json:"refs"`
}

type importer struct {
	ctx      context.Context
	repo     *client.Repo
	opts     ImportOptions
	stats    ImportStats
	blobs    map[int]object.ID // mark -> blob id
	commits  map[int]object.ID // mark -> changeset id
	gitSHA   map[int]string    // mark -> original commit sha
	pending  []client.StoredObject
	pendingC []object.Changeset
	refTips  map[string]object.ID
	mapOut   *bufio.Writer
}

// Import runs `git fast-export` in opts.GitDir and ingests its stream.
func Import(ctx context.Context, repo *client.Repo, opts ImportOptions) (*ImportStats, error) {
	if len(opts.Refs) == 0 {
		opts.Refs = []string{"HEAD"}
	}
	if opts.RefMap == nil {
		opts.RefMap = DefaultRefMap
	}
	if opts.Batch <= 0 {
		opts.Batch = 500
	}
	args := []string{"-C", opts.GitDir, "fast-export", "--show-original-ids", "--reencode=yes",
		"--signed-tags=strip", "--tag-of-filtered-object=drop", "--use-done-feature"}
	im := &importer{ctx: ctx, repo: repo, opts: opts, blobs: map[int]object.ID{}, commits: map[int]object.ID{},
		gitSHA: map[int]string{}, refTips: map[string]object.ID{}, stats: ImportStats{Refs: map[string]string{}}}
	if opts.StateDir != "" {
		if err := os.MkdirAll(opts.StateDir, 0o755); err != nil {
			return nil, err
		}
		gitMarks := filepath.Join(opts.StateDir, "git-marks")
		if _, err := os.Stat(gitMarks); err == nil {
			args = append(args, "--import-marks="+gitMarks)
			if err := im.loadMarks(filepath.Join(opts.StateDir, "agit-marks")); err != nil {
				return nil, err
			}
		}
		args = append(args, "--export-marks="+gitMarks+".new")
		f, err := os.OpenFile(filepath.Join(opts.StateDir, "commit-map"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		im.mapOut = bufio.NewWriter(f)
		defer im.mapOut.Flush()
	}
	args = append(args, opts.Refs...)
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Stderr = os.Stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	perr := im.parse(bufio.NewReaderSize(out, 1<<20))
	if perr != nil {
		cmd.Process.Kill()
		cmd.Wait()
		return nil, perr
	}
	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("git fast-export: %w", err)
	}
	if err := im.flush(); err != nil {
		return nil, err
	}
	if err := im.updateRefs(); err != nil {
		return nil, err
	}
	if opts.StateDir != "" {
		gitMarks := filepath.Join(opts.StateDir, "git-marks")
		if err := os.Rename(gitMarks+".new", gitMarks); err != nil {
			return nil, err
		}
		if err := im.saveMarks(filepath.Join(opts.StateDir, "agit-marks")); err != nil {
			return nil, err
		}
	}
	return &im.stats, nil
}

// DefaultRefMap maps refs/heads/X to X and refs/tags/X to tags/X; other
// refs are skipped.
func DefaultRefMap(ref string) string {
	switch {
	case strings.HasPrefix(ref, "refs/heads/"):
		return strings.TrimPrefix(ref, "refs/heads/")
	case strings.HasPrefix(ref, "refs/tags/"):
		return "tags/" + strings.TrimPrefix(ref, "refs/tags/")
	}
	return ""
}

func (im *importer) progress(format string, args ...interface{}) {
	if im.opts.Progress != nil {
		im.opts.Progress(format, args...)
	}
}

func (im *importer) warn(format string, args ...interface{}) {
	if im.opts.Warn != nil {
		im.opts.Warn(format, args...)
	}
}

func (im *importer) loadMarks(p string) error {
	f, err := os.Open(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		parts := strings.Fields(sc.Text())
		if len(parts) != 3 {
			continue
		}
		mark, _ := strconv.Atoi(parts[1])
		id, err := object.ParseID(parts[2])
		if err != nil {
			return err
		}
		if parts[0] == "b" {
			im.blobs[mark] = id
		} else {
			im.commits[mark] = id
		}
	}
	return sc.Err()
}

func (im *importer) saveMarks(p string) error {
	var b strings.Builder
	for m, id := range im.blobs {
		fmt.Fprintf(&b, "b %d %s\n", m, id)
	}
	for m, id := range im.commits {
		fmt.Fprintf(&b, "c %d %s\n", m, id)
	}
	return os.WriteFile(p, []byte(b.String()), 0o644)
}

func parseMark(s string) (int, error) {
	if !strings.HasPrefix(s, ":") {
		return 0, fmt.Errorf("expected a mark, got %q", s)
	}
	return strconv.Atoi(s[1:])
}

// readData reads a "data <n>" payload.
func readData(r *bufio.Reader, line string) ([]byte, error) {
	if !strings.HasPrefix(line, "data ") {
		return nil, fmt.Errorf("expected data, got %q", line)
	}
	n, err := strconv.Atoi(strings.TrimSpace(line[5:]))
	if err != nil {
		return nil, fmt.Errorf("bad data length %q", line)
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

func readLine(r *bufio.Reader) (string, error) {
	s, err := r.ReadString('\n')
	if err != nil {
		if err == io.EOF && s != "" {
			return s, nil
		}
		return "", err
	}
	return strings.TrimSuffix(s, "\n"), nil
}

// unquotePath decodes git's C-style quoted paths.
func unquotePath(p string) (string, error) {
	if strings.HasPrefix(p, `"`) {
		return strconv.Unquote(p)
	}
	return p, nil
}

// parseIdent splits "Name <email> 1700000000 +0100".
func parseIdent(s string) (who string, when int64, tz int, err error) {
	end := strings.LastIndexByte(s, '>')
	if end < 0 {
		return "", 0, 0, fmt.Errorf("bad identity %q", s)
	}
	who = s[:end+1]
	f := strings.Fields(s[end+1:])
	if len(f) != 2 {
		return "", 0, 0, fmt.Errorf("bad identity time %q", s)
	}
	when, err = strconv.ParseInt(f[0], 10, 64)
	if err != nil {
		return "", 0, 0, err
	}
	tz, err = parseTZ(f[1])
	return who, when, tz, err
}

func parseTZ(s string) (int, error) {
	if len(s) != 5 || (s[0] != '+' && s[0] != '-') {
		return 0, fmt.Errorf("bad timezone %q", s)
	}
	h, err1 := strconv.Atoi(s[1:3])
	m, err2 := strconv.Atoi(s[3:5])
	if err1 != nil || err2 != nil {
		return 0, fmt.Errorf("bad timezone %q", s)
	}
	v := h*60 + m
	if s[0] == '-' {
		v = -v
	}
	return v, nil
}

func formatTZ(minutes int) string {
	sign := '+'
	if minutes < 0 {
		sign, minutes = '-', -minutes
	}
	return fmt.Sprintf("%c%02d%02d", sign, minutes/60, minutes%60)
}

func (im *importer) parse(r *bufio.Reader) error {
	var pushback string
	next := func() (string, error) {
		if pushback != "" {
			l := pushback
			pushback = ""
			return l, nil
		}
		return readLine(r)
	}
	for {
		line, err := next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		switch {
		case line == "" || strings.HasPrefix(line, "feature ") || strings.HasPrefix(line, "option ") || strings.HasPrefix(line, "progress "):
		case line == "done":
			return nil
		case line == "blob":
			if err := im.parseBlob(r, next); err != nil {
				return err
			}
		case strings.HasPrefix(line, "commit "):
			pb, err := im.parseCommit(r, next, strings.TrimPrefix(line, "commit "))
			if err != nil {
				return err
			}
			pushback = pb
		case strings.HasPrefix(line, "reset "):
			ref := strings.TrimPrefix(line, "reset ")
			l, err := next()
			if err != nil {
				return err
			}
			if strings.HasPrefix(l, "from ") {
				m, err := parseMark(strings.TrimPrefix(l, "from "))
				if err != nil {
					return err
				}
				im.refTips[ref] = im.commits[m]
			} else {
				delete(im.refTips, ref)
				pushback = l
			}
		case strings.HasPrefix(line, "tag "):
			if err := im.parseTag(r, next, strings.TrimPrefix(line, "tag ")); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unexpected fast-export line %q", line)
		}
	}
}

func (im *importer) parseBlob(r *bufio.Reader, next func() (string, error)) error {
	mark := 0
	for {
		line, err := next()
		if err != nil {
			return err
		}
		switch {
		case strings.HasPrefix(line, "mark "):
			if mark, err = parseMark(line[5:]); err != nil {
				return err
			}
		case strings.HasPrefix(line, "original-oid "):
		case strings.HasPrefix(line, "data "):
			data, err := readData(r, line)
			if err != nil {
				return err
			}
			id, stored := object.Encode(object.KindBlob, data)
			im.blobs[mark] = id
			im.pending = append(im.pending, client.StoredObject{ID: id, Stored: stored})
			im.stats.Blobs++
			if len(im.pending) >= 2000 {
				return im.flushBlobs()
			}
			return nil
		default:
			return fmt.Errorf("unexpected line in blob: %q", line)
		}
	}
}

func (im *importer) parseTag(r *bufio.Reader, next func() (string, error), name string) error {
	var target object.ID
	for {
		line, err := next()
		if err != nil {
			return err
		}
		switch {
		case strings.HasPrefix(line, "from "):
			m, err := parseMark(line[5:])
			if err != nil {
				return err
			}
			target = im.commits[m]
		case strings.HasPrefix(line, "data "):
			if _, err := readData(r, line); err != nil {
				return err
			}
			if !target.IsZero() {
				im.refTips["refs/tags/"+name] = target
			}
			return nil
		}
	}
}

func (im *importer) parseCommit(r *bufio.Reader, next func() (string, error), ref string) (string, error) {
	cs := object.Changeset{}
	mark := 0
	sha := ""
	changes := map[string]object.Change{}
	var order []string
	setChange := func(c object.Change) {
		if _, ok := changes[c.Path]; !ok {
			order = append(order, c.Path)
		}
		changes[c.Path] = c
	}
	for {
		line, err := next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		switch {
		case strings.HasPrefix(line, "mark "):
			if mark, err = parseMark(line[5:]); err != nil {
				return "", err
			}
		case strings.HasPrefix(line, "original-oid "):
			sha = strings.TrimPrefix(line, "original-oid ")
		case strings.HasPrefix(line, "author "):
			if cs.Author, cs.AuthorTime, cs.AuthorTZ, err = parseIdent(line[7:]); err != nil {
				return "", err
			}
		case strings.HasPrefix(line, "committer "):
			if cs.Committer, cs.CommitTime, cs.CommitTZ, err = parseIdent(line[10:]); err != nil {
				return "", err
			}
		case strings.HasPrefix(line, "encoding "):
		case strings.HasPrefix(line, "data "):
			msg, err := readData(r, line)
			if err != nil {
				return "", err
			}
			cs.Message = string(msg)
		case strings.HasPrefix(line, "from "), strings.HasPrefix(line, "merge "):
			sp := strings.IndexByte(line, ' ')
			m, err := parseMark(line[sp+1:])
			if err != nil {
				return "", err
			}
			p, ok := im.commits[m]
			if !ok {
				return "", fmt.Errorf("commit %s refers to unknown parent mark %d", sha, m)
			}
			cs.Parents = append(cs.Parents, p)
		case strings.HasPrefix(line, "M "):
			f := strings.SplitN(line, " ", 4)
			if len(f) != 4 {
				return "", fmt.Errorf("bad modify line %q", line)
			}
			p, err := unquotePath(f[3])
			if err != nil {
				return "", err
			}
			mode, err := object.ParseMode(f[1])
			if err != nil || f[1] == "160000" {
				im.stats.Skipped++
				im.warn("skipping %s (mode %s, e.g. a submodule) in %s", p, f[1], sha)
				continue
			}
			m, err := parseMark(f[2])
			if err != nil {
				return "", fmt.Errorf("inline or sha blob references are not supported: %q", line)
			}
			setChange(object.Change{Path: p, Mode: mode, Blob: im.blobs[m]})
		case strings.HasPrefix(line, "D "):
			p, err := unquotePath(line[2:])
			if err != nil {
				return "", err
			}
			setChange(object.Change{Path: p})
		case line == "deleteall":
			return "", fmt.Errorf("fast-export 'deleteall' is not supported (do not use --full-tree)")
		case line == "":
			// A blank line ends the commit.
			goto done
		default:
			// Start of the next command: hand it back to the caller.
			if err := im.finishCommit(&cs, mark, sha, ref, changes, order); err != nil {
				return "", err
			}
			return line, nil
		}
	}
done:
	return "", im.finishCommit(&cs, mark, sha, ref, changes, order)
}

func (im *importer) finishCommit(cs *object.Changeset, mark int, sha, ref string, changes map[string]object.Change, order []string) error {
	for _, p := range order {
		cs.Changes = append(cs.Changes, changes[p])
	}
	if sha != "" {
		cs.Extra = map[string]string{object.ExtraGitCommit: sha}
	}
	cs.Normalize()
	id := cs.ID()
	im.commits[mark] = id
	im.gitSHA[mark] = sha
	im.refTips[ref] = id
	im.pendingC = append(im.pendingC, *cs)
	im.stats.Commits++
	if im.mapOut != nil && sha != "" {
		fmt.Fprintf(im.mapOut, "%s %s\n", sha, id)
	}
	if len(im.pendingC) >= im.opts.Batch {
		return im.flush()
	}
	return nil
}

func (im *importer) flushBlobs() error {
	if len(im.pending) == 0 {
		return nil
	}
	if _, err := im.repo.UploadMissing(im.ctx, im.pending); err != nil {
		return fmt.Errorf("uploading blobs: %w", err)
	}
	im.pending = im.pending[:0]
	return nil
}

func (im *importer) flush() error {
	if err := im.flushBlobs(); err != nil {
		return err
	}
	if len(im.pendingC) == 0 {
		return nil
	}
	ids, err := im.repo.Import(im.ctx, im.pendingC)
	if err != nil {
		return fmt.Errorf("importing changesets: %w", err)
	}
	for i, id := range ids {
		if want := im.pendingC[i].ID(); id != want {
			return fmt.Errorf("server assigned %s to changeset %d, expected %s", id.Short(), i, want.Short())
		}
	}
	im.progress("imported %d commits", im.stats.Commits)
	im.pendingC = im.pendingC[:0]
	return nil
}

func (im *importer) updateRefs() error {
	for gitRef, id := range im.refTips {
		name := im.opts.RefMap(gitRef)
		if name == "" || id.IsZero() {
			continue
		}
		cur, err := im.repo.Resolve(im.ctx, name)
		if err != nil && !client.IsNotFound(err) {
			return err
		}
		if cur == id {
			im.stats.Refs[name] = id.String()
			continue
		}
		if err := im.repo.UpdateRef(im.ctx, name, cur, id); err != nil {
			return fmt.Errorf("updating %s: %w", name, err)
		}
		im.stats.Refs[name] = id.String()
	}
	return nil
}
