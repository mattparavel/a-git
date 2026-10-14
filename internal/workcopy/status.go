package workcopy

import (
	"bufio"
	"context"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/mattparavel/a-git/internal/object"
	"github.com/mattparavel/a-git/internal/repo"
)

// FileStatus describes a local change relative to the base changeset.
type FileStatus struct {
	Path   string      `json:"path"`
	Status string      `json:"status"` // "M" modified, "A" added, "D" deleted
	Mode   object.Mode `json:"mode,omitzero"`
	ID     object.ID   `json:"id,omitzero"` // new content (for M and A)
}

// ignoreRule is one line of .agitignore.
type ignoreRule struct {
	pattern  string
	negate   bool
	dirOnly  bool
	anchored bool // contains a slash: match against the full path
}

func (w *WorkCopy) loadIgnore() []ignoreRule {
	rules := []ignoreRule{{pattern: MetaDir, dirOnly: true}, {pattern: ".git", dirOnly: true}}
	f, err := os.Open(filepath.Join(w.Root, ".agitignore"))
	if err != nil {
		return rules
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		r := ignoreRule{}
		if strings.HasPrefix(line, "!") {
			r.negate, line = true, line[1:]
		}
		if strings.HasSuffix(line, "/") {
			r.dirOnly, line = true, strings.TrimSuffix(line, "/")
		}
		if strings.Contains(line, "/") {
			r.anchored, line = true, strings.TrimPrefix(line, "/")
		}
		r.pattern = line
		rules = append(rules, r)
	}
	return rules
}

func ignored(rules []ignoreRule, p string, isDir bool) bool {
	out := false
	base := path.Base(p)
	for _, r := range rules {
		if r.dirOnly && !isDir {
			continue
		}
		var m bool
		if r.anchored {
			m = repo.MatchGlob(r.pattern, p)
		} else {
			m, _ = path.Match(r.pattern, base)
		}
		if m {
			out = !r.negate
		}
	}
	return out
}

// diskFile reads a file's mode and content the way it would be committed.
func diskFile(abs string) (object.Mode, []byte, fs.FileInfo, error) {
	fi, err := os.Lstat(abs)
	if err != nil {
		return 0, nil, nil, err
	}
	switch {
	case fi.Mode()&fs.ModeSymlink != 0:
		target, err := os.Readlink(abs)
		return object.ModeLink, []byte(target), fi, err
	case fi.Mode().IsRegular():
		data, err := os.ReadFile(abs)
		mode := object.ModeFile
		if fi.Mode()&0o111 != 0 {
			mode = object.ModeExec
		}
		return mode, data, fi, err
	}
	return 0, nil, fi, fs.ErrInvalid
}

func statOf(fi fs.FileInfo) (size, mtime int64, ino uint64) {
	size, mtime = fi.Size(), fi.ModTime().UnixNano()
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		ino = st.Ino
	}
	return
}

func modeKindMatches(m object.Mode, fi fs.FileInfo) bool {
	switch m {
	case object.ModeLink:
		return fi.Mode()&fs.ModeSymlink != 0
	case object.ModeExec:
		return fi.Mode().IsRegular() && fi.Mode()&0o111 != 0
	case object.ModeFile:
		return fi.Mode().IsRegular() && fi.Mode()&0o111 == 0
	}
	return false
}

// Status compares the working files against the base changeset. Files whose
// size, mtime and inode match the recorded state are assumed unchanged
// without being read; others are hashed. Untracked, non-ignored files inside
// the sparse profile are reported as added. If paths is non-empty only those
// paths (and directories) are examined.
func (w *WorkCopy) Status(ctx context.Context, paths []string) ([]FileStatus, error) {
	rules := w.loadIgnore()
	inScope := func(p string, isDir bool) bool {
		if len(paths) == 0 {
			return true
		}
		for _, s := range paths {
			if object.HasPathPrefix(p, s) || isDir && object.HasPathPrefix(s, p) {
				return true
			}
		}
		return false
	}
	var out []FileStatus
	seen := map[string]bool{}
	refreshed := false
	err := filepath.WalkDir(w.Root, func(abs string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		rel, _ := filepath.Rel(w.Root, abs)
		if rel == "." {
			return nil
		}
		p := filepath.ToSlash(rel)
		if d.IsDir() {
			if ignored(rules, p, true) || !w.InSparse(p, true) || !inScope(p, true) {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() && d.Type()&fs.ModeSymlink == 0 {
			return nil
		}
		if !w.InSparse(p, false) || !inScope(p, false) {
			return nil
		}
		seen[p] = true
		rec, tracked := w.Files[p]
		if !tracked && ignored(rules, p, false) {
			return nil
		}
		fi, err := d.Info()
		if err != nil {
			return nil
		}
		if tracked && modeKindMatches(rec.Mode, fi) {
			size, mtime, ino := statOf(fi)
			if size == rec.Size && mtime == rec.MTime && ino == rec.Ino && mtime < w.stamp {
				return nil
			}
		}
		mode, data, fi, err := diskFile(abs)
		if err != nil {
			return nil
		}
		id := object.Hash(object.KindBlob, data)
		if tracked {
			if id == rec.ID && mode == rec.Mode {
				// Content unchanged; refresh the stat cache.
				size, mtime, ino := statOf(fi)
				w.Files[p] = FileState{Mode: mode, ID: id, Size: size, MTime: mtime, Ino: ino}
				refreshed = true
				return nil
			}
			out = append(out, FileStatus{Path: p, Status: "M", Mode: mode, ID: id})
		} else {
			out = append(out, FileStatus{Path: p, Status: "A", Mode: mode, ID: id})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	for p := range w.Files {
		if !seen[p] && w.InSparse(p, false) && inScope(p, false) {
			out = append(out, FileStatus{Path: p, Status: "D"})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	if refreshed {
		w.writeDirstate() // best effort; makes the next status cheaper
	}
	return out, nil
}
