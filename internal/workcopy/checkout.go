package workcopy

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mattparavel/a-git/internal/fetch"
	"github.com/mattparavel/a-git/internal/object"
	"github.com/mattparavel/a-git/internal/textdiff"
)

// writeFile materializes one file and returns its recorded state.
func (w *WorkCopy) writeFile(p string, mode object.Mode, id object.ID, raw []byte) (FileState, error) {
	abs := w.Abs(p)
	if err := w.ensureDir(filepath.Dir(abs)); err != nil {
		return FileState{}, err
	}
	if fi, err := os.Lstat(abs); err == nil && fi.IsDir() {
		// The tree replaced a directory with a file; tracked files below it
		// were removed by the same update, so only leftovers remain.
		if err := os.RemoveAll(abs); err != nil {
			return FileState{}, err
		}
	}
	if mode == object.ModeLink {
		os.Remove(abs)
		if err := os.Symlink(string(raw), abs); err != nil {
			return FileState{}, err
		}
	} else {
		perm := os.FileMode(0o644)
		if mode == object.ModeExec {
			perm = 0o755
		}
		tmp, err := os.CreateTemp(filepath.Dir(abs), ".agit-tmp-")
		if err != nil {
			return FileState{}, err
		}
		_, werr := tmp.Write(raw)
		cerr := tmp.Close()
		if werr == nil {
			werr = cerr
		}
		if werr == nil {
			werr = os.Chmod(tmp.Name(), perm)
		}
		if werr == nil {
			os.Remove(abs) // may be a symlink
			werr = os.Rename(tmp.Name(), abs)
		}
		if werr != nil {
			os.Remove(tmp.Name())
			return FileState{}, werr
		}
	}
	fi, err := os.Lstat(abs)
	if err != nil {
		return FileState{}, err
	}
	size, mtime, ino := statOf(fi)
	return FileState{Mode: mode, ID: id, Size: size, MTime: mtime, Ino: ino}, nil
}

// ensureDir creates dir, replacing any file that is in the way.
func (w *WorkCopy) ensureDir(dir string) error {
	err := os.MkdirAll(dir, 0o755)
	if err == nil {
		return nil
	}
	// A file (from an earlier version) may occupy a parent path.
	rel, rerr := filepath.Rel(w.Root, dir)
	if rerr != nil {
		return err
	}
	cur := w.Root
	for _, c := range strings.Split(rel, string(filepath.Separator)) {
		cur = filepath.Join(cur, c)
		if fi, serr := os.Lstat(cur); serr == nil && !fi.IsDir() {
			os.Remove(cur)
		}
	}
	return os.MkdirAll(dir, 0o755)
}

// removeFile deletes a file and any directories left empty.
func (w *WorkCopy) removeFile(p string) error {
	abs := w.Abs(p)
	if err := os.Remove(abs); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	for dir := filepath.Dir(abs); dir != w.Root && strings.HasPrefix(dir, w.Root); dir = filepath.Dir(dir) {
		if err := os.Remove(dir); err != nil {
			break // not empty
		}
	}
	return nil
}

// Progress receives progress messages; it may be nil.
type Progress func(format string, args ...interface{})

func (pr Progress) printf(format string, args ...interface{}) {
	if pr != nil {
		pr(format, args...)
	}
}

// Checkout materializes changeset cs into an empty working copy, fetching
// only the trees and blobs inside the sparse profile.
func (w *WorkCopy) Checkout(ctx context.Context, cs object.ID, progress Progress) error {
	root, err := w.Repo.RootTree(ctx, cs)
	if err != nil {
		return err
	}
	f := &fetch.Fetcher{Src: w.Repo, CS: cs}
	var entries []fetch.Entry
	if err := f.Walk(ctx, root, w.InSparse, func(e fetch.Entry) error {
		entries = append(entries, e)
		return nil
	}); err != nil {
		return err
	}
	progress.printf("fetching %d files", len(entries))
	if err := w.materialize(ctx, f, entries); err != nil {
		return err
	}
	w.State = State{Base: cs, Tree: root}
	progress.printf("fetched %d objects (%d from local store)", f.Requested, f.FromLocal)
	return w.Save()
}

func (w *WorkCopy) materialize(ctx context.Context, f *fetch.Fetcher, entries []fetch.Entry) error {
	byPath := map[string]fetch.Entry{}
	wants := make([]fetch.Want, 0, len(entries))
	for _, e := range entries {
		byPath[e.Path] = e
		wants = append(wants, fetch.Want{ID: e.ID, Path: e.Path})
	}
	return f.Get(ctx, wants, func(want fetch.Want, kind object.Kind, raw []byte) error {
		e := byPath[want.Path]
		st, err := w.writeFile(e.Path, e.Mode, e.ID, raw)
		if err != nil {
			return err
		}
		w.Files[e.Path] = st
		return nil
	})
}

// UpdateResult summarises an update.
type UpdateResult struct {
	Updated   int      `json:"updated"`
	Removed   int      `json:"removed"`
	Merged    []string `json:"merged,omitempty"`    // local edits merged cleanly with incoming changes
	Conflicts []string `json:"conflicts,omitempty"` // paths left with conflict markers or kept as-is
}

// Update moves the working copy to changeset target. Only subtrees that
// differ between the current base and the target are fetched. Local edits
// to files that also changed upstream are merged three-way; conflicting
// regions are marked in the file.
func (w *WorkCopy) Update(ctx context.Context, target object.ID, force bool) (*UpdateResult, error) {
	targetTree, err := w.Repo.RootTree(ctx, target)
	if err != nil {
		return nil, err
	}
	res := &UpdateResult{}
	if targetTree == w.State.Tree {
		w.State.Base = target
		return res, w.Save()
	}
	status, err := w.Status(ctx, nil)
	if err != nil {
		return nil, err
	}
	dirty := map[string]FileStatus{}
	for _, s := range status {
		dirty[s.Path] = s
	}
	f := &fetch.Fetcher{Src: w.Repo, CS: target, FromCS: w.State.Base}
	type change struct {
		path     string
		old, new *fetch.Entry
	}
	var changes []change
	err = f.Diff(ctx, w.State.Tree, targetTree, w.InSparse, func(p string, old, new *fetch.Entry) error {
		changes = append(changes, change{p, old, new})
		return nil
	})
	if err != nil {
		return nil, err
	}
	// Deletions first so files can be replaced by directories.
	sort.SliceStable(changes, func(i, j int) bool { return changes[i].new == nil && changes[j].new != nil })
	var wants []fetch.Want
	newEntries := map[string]*fetch.Entry{}
	oldEntries := map[string]*fetch.Entry{}
	for _, c := range changes {
		d, isDirty := dirty[c.path]
		if c.new == nil {
			if isDirty && !force {
				res.Conflicts = append(res.Conflicts, c.path)
				delete(w.Files, c.path) // keep the local file, now untracked
				continue
			}
			if err := w.removeFile(c.path); err != nil {
				return nil, err
			}
			delete(w.Files, c.path)
			res.Removed++
			continue
		}
		if isDirty && !force && d.Status != "D" && d.ID == c.new.ID && d.Mode == c.new.Mode {
			// The local edit already matches the incoming version.
			st := w.Files[c.path]
			st.ID, st.Mode, st.Size = c.new.ID, c.new.Mode, -1
			w.Files[c.path] = st
			continue
		}
		newEntries[c.path] = c.new
		wants = append(wants, fetch.Want{ID: c.new.ID, Path: c.path})
		if isDirty && !force && c.old != nil && d.Status == "M" {
			oldEntries[c.path] = c.old
			wants = append(wants, fetch.Want{ID: c.old.ID, Path: c.path, CS: w.State.Base})
		}
	}
	contents := map[object.ID][]byte{}
	err = f.Get(ctx, wants, func(want fetch.Want, kind object.Kind, raw []byte) error {
		contents[want.ID] = raw
		return nil
	})
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(newEntries))
	for p := range newEntries {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		e := newEntries[p]
		raw := contents[e.ID]
		d, isDirty := dirty[p]
		if isDirty && !force && d.Status != "D" {
			local, err := os.ReadFile(w.Abs(p))
			if err != nil {
				return nil, err
			}
			var base []byte
			if old := oldEntries[p]; old != nil {
				base = contents[old.ID]
			}
			if textdiff.IsBinary(local) || textdiff.IsBinary(raw) || e.Mode == object.ModeLink {
				res.Conflicts = append(res.Conflicts, p)
				st := w.Files[p]
				st.ID, st.Mode, st.Size = e.ID, e.Mode, -1
				w.Files[p] = st
				continue
			}
			m := textdiff.Merge3(base, local, raw, "base", "local", "incoming "+target.Short())
			st, err := w.writeFile(p, e.Mode, e.ID, m.Merged)
			if err != nil {
				return nil, err
			}
			st.Size = -1 // content differs from the recorded blob: force a rehash
			w.Files[p] = st
			if len(m.Conflicts) > 0 {
				res.Conflicts = append(res.Conflicts, p)
			} else {
				res.Merged = append(res.Merged, p)
			}
			continue
		}
		st, err := w.writeFile(p, e.Mode, e.ID, raw)
		if err != nil {
			return nil, err
		}
		w.Files[p] = st
		res.Updated++
	}
	w.State = State{Base: target, Tree: targetTree}
	return res, w.Save()
}

// SetSparse changes the sparse profile, materializing newly included paths
// and removing files that fall outside it. Locally modified files that would
// be removed abort the operation unless force is set.
func (w *WorkCopy) SetSparse(ctx context.Context, profile []string, force bool) (added, removed int, err error) {
	// An empty profile, or one naming the root, means "everything".
	var clean []string
	for _, p := range profile {
		c := object.CleanPath(p)
		if c == "" {
			clean = nil
			break
		}
		clean = append(clean, c)
	}
	old := w.Config.Sparse
	w.Config.Sparse = clean
	newIn := func(p string, isDir bool) bool { return w.InSparse(p, isDir) }

	// Files to remove.
	var drop []string
	for p := range w.Files {
		if !newIn(p, false) {
			drop = append(drop, p)
		}
	}
	if len(drop) > 0 && !force {
		w.Config.Sparse = old
		status, err := w.Status(ctx, drop)
		if err != nil {
			return 0, 0, err
		}
		w.Config.Sparse = clean
		for _, s := range status {
			if s.Status != "A" {
				w.Config.Sparse = old
				return 0, 0, fmt.Errorf("%s has local changes; commit them or use --force", s.Path)
			}
		}
	}
	for _, p := range drop {
		if err := w.removeFile(p); err != nil {
			return 0, 0, err
		}
		delete(w.Files, p)
	}

	// Files to add.
	f := &fetch.Fetcher{Src: w.Repo, CS: w.State.Base}
	var entries []fetch.Entry
	err = f.Walk(ctx, w.State.Tree, newIn, func(e fetch.Entry) error {
		if _, ok := w.Files[e.Path]; !ok {
			entries = append(entries, e)
		}
		return nil
	})
	if err != nil {
		return 0, 0, err
	}
	if err := w.materialize(ctx, f, entries); err != nil {
		return 0, 0, err
	}
	return len(entries), len(drop), w.Save()
}
