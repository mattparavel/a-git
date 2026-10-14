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

	"github.com/mattparavel/a-git/internal/api"
	"github.com/mattparavel/a-git/internal/client"
	"github.com/mattparavel/a-git/internal/object"
)

// ExportOptions controls an export.
type ExportOptions struct {
	GitDir   string // target git repository (created if missing)
	Rev      string // a-git revision to export (default "main")
	GitRef   string // git ref to write (default "refs/heads/main")
	Progress func(format string, args ...interface{})
}

// ExportStats summarises an export.
type ExportStats struct {
	Commits int    `json:"commits"`
	Head    string `json:"head"`
}

// Export writes the history of opts.Rev into a git repository using
// `git fast-import`. It remembers what was exported (in the git directory)
// so repeated exports only send new changesets, which lets a git mirror
// follow an a-git ref.
func Export(ctx context.Context, repo *client.Repo, opts ExportOptions) (*ExportStats, error) {
	if opts.Rev == "" {
		opts.Rev = "main"
	}
	if opts.GitRef == "" {
		opts.GitRef = "refs/heads/main"
	}
	if _, err := os.Stat(filepath.Join(opts.GitDir, ".git")); errors.Is(err, os.ErrNotExist) {
		if _, err := os.Stat(filepath.Join(opts.GitDir, "HEAD")); errors.Is(err, os.ErrNotExist) {
			args := []string{"init", "-q"}
			if b, ok := strings.CutPrefix(opts.GitRef, "refs/heads/"); ok {
				args = append(args, "-b", b) // HEAD points at the exported branch
			}
			if out, err := exec.CommandContext(ctx, "git", append(args, opts.GitDir)...).CombinedOutput(); err != nil {
				return nil, fmt.Errorf("git init: %v: %s", err, out)
			}
		}
	}
	gitDir, err := exec.CommandContext(ctx, "git", "-C", opts.GitDir, "rev-parse", "--absolute-git-dir").Output()
	if err != nil {
		return nil, fmt.Errorf("%s is not a git repository", opts.GitDir)
	}
	mapFile := filepath.Join(strings.TrimSpace(string(gitDir)), "agit-export-map")
	exported, err := loadExportMap(mapFile)
	if err != nil {
		return nil, err
	}

	head, err := repo.Resolve(ctx, opts.Rev)
	if err != nil {
		return nil, err
	}
	order, css, err := collect(ctx, repo, head, exported)
	if err != nil {
		return nil, err
	}
	stats := &ExportStats{Head: head.String()}
	if len(order) == 0 {
		if sha, ok := exported[head]; ok {
			stats.Head = sha
			return stats, updateRef(ctx, opts.GitDir, opts.GitRef, sha)
		}
	}

	marksFile := mapFile + ".marks"
	cmd := exec.CommandContext(ctx, "git", "-C", opts.GitDir, "fast-import", "--quiet", "--export-marks="+marksFile)
	cmd.Stderr = os.Stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	w := bufio.NewWriterSize(stdin, 1<<20)
	markOf := map[object.ID]int{}
	nextMark := 1
	werr := func() error {
		for _, id := range order {
			cs := css[id]
			// Fetch and emit the blobs this changeset introduces.
			var refs []api.ObjectRef
			for _, ch := range cs.Changes {
				if !ch.IsDelete() {
					refs = append(refs, api.ObjectRef{ID: ch.Blob, CS: id, Path: ch.Path})
				}
			}
			blobMarks := map[object.ID]int{}
			if len(refs) > 0 {
				missing, err := repo.GetObjects(ctx, refs, func(bid object.ID, stored []byte) error {
					if _, done := blobMarks[bid]; done {
						return nil
					}
					_, raw, err := object.Decode(stored)
					if err != nil {
						return err
					}
					blobMarks[bid] = nextMark
					fmt.Fprintf(w, "blob\nmark :%d\ndata %d\n", nextMark, len(raw))
					w.Write(raw)
					w.WriteString("\n")
					nextMark++
					return nil
				})
				if err != nil {
					return err
				}
				if len(missing) > 0 {
					return fmt.Errorf("blob %s of %s is not available", missing[0].Short(), id.Short())
				}
			}
			mark := nextMark
			nextMark++
			markOf[id] = mark
			if len(cs.Parents) == 0 {
				fmt.Fprintf(w, "reset %s\n", opts.GitRef)
			}
			fmt.Fprintf(w, "commit %s\nmark :%d\n", opts.GitRef, mark)
			fmt.Fprintf(w, "author %s %d %s\n", gitIdent(cs.Author), cs.AuthorTime, formatTZ(cs.AuthorTZ))
			committer, ct, ctz := cs.Committer, cs.CommitTime, cs.CommitTZ
			if committer == "" {
				committer, ct, ctz = cs.Author, cs.AuthorTime, cs.AuthorTZ
			}
			fmt.Fprintf(w, "committer %s %d %s\n", gitIdent(committer), ct, formatTZ(ctz))
			fmt.Fprintf(w, "data %d\n%s", len(cs.Message), cs.Message)
			for i, p := range cs.Parents {
				ref := ""
				if m, ok := markOf[p]; ok {
					ref = ":" + strconv.Itoa(m)
				} else if sha, ok := exported[p]; ok {
					ref = sha
				} else {
					return fmt.Errorf("parent %s of %s was not exported", p.Short(), id.Short())
				}
				if i == 0 {
					fmt.Fprintf(w, "from %s\n", ref)
				} else {
					fmt.Fprintf(w, "merge %s\n", ref)
				}
			}
			for _, ch := range cs.Changes {
				if ch.IsDelete() {
					fmt.Fprintf(w, "D %s\n", quotePath(ch.Path))
				} else {
					fmt.Fprintf(w, "M %s :%d %s\n", ch.Mode, blobMarks[ch.Blob], quotePath(ch.Path))
				}
			}
			w.WriteString("\n")
			stats.Commits++
			if opts.Progress != nil && stats.Commits%500 == 0 {
				opts.Progress("exported %d changesets", stats.Commits)
			}
		}
		w.WriteString("done\n")
		return w.Flush()
	}()
	stdin.Close()
	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("git fast-import: %w", err)
	}
	if werr != nil {
		return nil, werr
	}
	// Record the git commit of every exported changeset.
	marks, err := readMarks(marksFile)
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(mapFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	for id, m := range markOf {
		fmt.Fprintf(f, "%s %s\n", id, marks[m])
		exported[id] = marks[m]
	}
	f.Close()
	os.Remove(marksFile)
	stats.Head = exported[head]
	return stats, updateRef(ctx, opts.GitDir, opts.GitRef, exported[head])
}

func updateRef(ctx context.Context, gitDir, ref, sha string) error {
	if out, err := exec.CommandContext(ctx, "git", "-C", gitDir, "update-ref", ref, sha).CombinedOutput(); err != nil {
		return fmt.Errorf("git update-ref: %v: %s", err, out)
	}
	return nil
}

// collect returns the changesets reachable from head that have not been
// exported yet, parents first.
func collect(ctx context.Context, repo *client.Repo, head object.ID, exported map[object.ID]string) ([]object.ID, map[object.ID]*object.Changeset, error) {
	css := map[object.ID]*object.Changeset{}
	frontier := []object.ID{}
	if _, done := exported[head]; !done {
		frontier = append(frontier, head)
	}
	for len(frontier) > 0 {
		batch := frontier
		if len(batch) > 500 {
			batch = batch[:500]
		}
		frontier = frontier[len(batch):]
		got, err := repo.GetChangesets(ctx, batch)
		if err != nil {
			return nil, nil, err
		}
		for _, id := range batch {
			cs := got[id]
			if cs == nil {
				return nil, nil, fmt.Errorf("changeset %s not found", id.Short())
			}
			if _, seen := css[id]; seen {
				continue
			}
			css[id] = cs
			for _, p := range cs.Parents {
				if _, seen := css[p]; !seen {
					if _, done := exported[p]; !done {
						frontier = append(frontier, p)
					}
				}
			}
		}
	}
	// Topological order: parents before children (iterative DFS).
	var order []object.ID
	state := map[object.ID]int{} // 1 = visiting, 2 = done
	type frame struct {
		id   object.ID
		post bool
	}
	stack := []frame{{id: head}}
	for len(stack) > 0 {
		f := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		cs := css[f.id]
		if cs == nil {
			continue // already exported
		}
		if f.post {
			if state[f.id] != 2 {
				state[f.id] = 2
				order = append(order, f.id)
			}
			continue
		}
		if state[f.id] != 0 {
			continue
		}
		state[f.id] = 1
		stack = append(stack, frame{id: f.id, post: true})
		for i := len(cs.Parents) - 1; i >= 0; i-- {
			if state[cs.Parents[i]] == 0 {
				stack = append(stack, frame{id: cs.Parents[i]})
			}
		}
	}
	return order, css, nil
}

func loadExportMap(p string) (map[object.ID]string, error) {
	out := map[object.ID]string{}
	f, err := os.Open(p)
	if errors.Is(err, os.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		parts := strings.Fields(sc.Text())
		if len(parts) != 2 {
			continue
		}
		if id, err := object.ParseID(parts[0]); err == nil {
			out[id] = parts[1]
		}
	}
	return out, sc.Err()
}

func readMarks(p string) (map[int]string, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := map[int]string{}
	r := bufio.NewReader(f)
	for {
		line, err := r.ReadString('\n')
		if parts := strings.Fields(line); len(parts) == 2 {
			if m, perr := parseMark(parts[0]); perr == nil {
				out[m] = parts[1]
			}
		}
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
	}
}

// gitIdent makes an identity acceptable to git ("Name <email>").
func gitIdent(s string) string {
	s = strings.Map(func(r rune) rune {
		if r == '\n' || r == 0 {
			return ' '
		}
		return r
	}, strings.TrimSpace(s))
	if i, j := strings.IndexByte(s, '<'), strings.LastIndexByte(s, '>'); i >= 0 && j > i && strings.Count(s, "<") == 1 && strings.Count(s, ">") == 1 {
		return s
	}
	clean := strings.NewReplacer("<", "", ">", "").Replace(s)
	if clean == "" {
		clean = "unknown"
	}
	return clean + " <>"
}

// quotePath quotes a path for fast-import when needed.
func quotePath(p string) string {
	if strings.ContainsAny(p, "\"\\\n") || strings.HasPrefix(p, " ") || strings.ContainsRune(p, 0) {
		return strconv.Quote(p)
	}
	for i := 0; i < len(p); i++ {
		if p[i] < 0x20 || p[i] >= 0x7f {
			return strconv.Quote(p)
		}
	}
	return p
}
