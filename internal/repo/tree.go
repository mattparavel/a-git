package repo

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/mattparavel/a-git/internal/object"
)

// changeTrie groups a changeset's file changes by directory so a new root
// tree can be built by rewriting only the directories that contain changes.
type changeTrie struct {
	leaf     *object.Change
	children map[string]*changeTrie
}

func buildTrie(changes []object.Change) *changeTrie {
	root := &changeTrie{}
	for i := range changes {
		n := root
		for _, comp := range strings.Split(changes[i].Path, "/") {
			if n.children == nil {
				n.children = map[string]*changeTrie{}
			}
			c := n.children[comp]
			if c == nil {
				c = &changeTrie{}
				n.children[comp] = c
			}
			n = c
		}
		n.leaf = &changes[i]
	}
	return root
}

// ApplyChanges returns the root tree obtained by applying changes to base.
// Only directories that contain a change are rewritten, so the cost is
// proportional to the size of the change rather than the size of the tree.
// It fails if a deletion names a missing file or a file and a directory would
// collide.
func (r *Repo) ApplyChanges(ctx context.Context, base object.ID, changes []object.Change) (object.ID, error) {
	if base.IsZero() {
		base = object.EmptyTreeID
	}
	if len(changes) == 0 {
		return base, nil
	}
	id, nonEmpty, err := r.applyTrie(ctx, "", base, buildTrie(changes))
	if err != nil {
		return object.Zero, err
	}
	if !nonEmpty {
		return r.WriteTree(ctx, &object.Tree{})
	}
	return id, nil
}

func (r *Repo) applyTrie(ctx context.Context, dir string, base object.ID, n *changeTrie) (object.ID, bool, error) {
	t, err := r.ReadTree(ctx, base)
	if err != nil {
		return object.Zero, false, err
	}
	entries := make(map[string]object.TreeEntry, len(t.Entries)+len(n.children))
	for _, e := range t.Entries {
		entries[e.Name] = e
	}
	for name, child := range n.children {
		p := object.JoinPath(dir, name)
		existing, exists := entries[name]
		leaf := child.leaf
		deletesFile := leaf != nil && leaf.IsDelete()

		var sub object.ID
		subNonEmpty := false
		if len(child.children) > 0 {
			subBase := object.EmptyTreeID
			switch {
			case exists && existing.Mode.IsDir():
				subBase = existing.ID
			case exists && !deletesFile:
				return object.Zero, false, fmt.Errorf("cannot change %q/...: %q is a file", p, p)
			}
			sub, subNonEmpty, err = r.applyTrie(ctx, p, subBase, child)
			if err != nil {
				return object.Zero, false, err
			}
		}

		switch {
		case leaf == nil:
			if subNonEmpty {
				entries[name] = object.TreeEntry{Name: name, Mode: object.ModeDir, ID: sub}
			} else {
				delete(entries, name)
			}
		case leaf.IsDelete():
			if !exists || existing.Mode.IsDir() {
				return object.Zero, false, fmt.Errorf("cannot delete %q: no such file", p)
			}
			if subNonEmpty {
				entries[name] = object.TreeEntry{Name: name, Mode: object.ModeDir, ID: sub}
			} else {
				delete(entries, name)
			}
		default:
			if subNonEmpty {
				return object.Zero, false, fmt.Errorf("%q would be both a file and a directory", p)
			}
			if exists && existing.Mode.IsDir() && len(child.children) == 0 {
				return object.Zero, false, fmt.Errorf("cannot replace directory %q with a file without deleting its contents", p)
			}
			entries[name] = object.TreeEntry{Name: name, Mode: leaf.Mode, ID: leaf.Blob}
		}
	}
	if len(entries) == 0 {
		return object.Zero, false, nil
	}
	nt := &object.Tree{Entries: make([]object.TreeEntry, 0, len(entries))}
	for _, e := range entries {
		nt.Entries = append(nt.Entries, e)
	}
	nt.Sort()
	id, err := r.WriteTree(ctx, nt)
	return id, true, err
}

func deriveTree(ctx context.Context, r *Repo, id object.ID, cs *object.Changeset) ([]byte, error) {
	base, err := r.RootTree(ctx, cs.FirstParent())
	if err != nil {
		return nil, err
	}
	root, err := r.ApplyChanges(ctx, base, cs.Changes)
	if err != nil {
		return nil, err
	}
	return root[:], nil
}

// FileDiff describes how one file differs between two trees.
type FileDiff struct {
	Path    string      `json:"path"`
	OldMode object.Mode `json:"old_mode,omitzero"`
	OldBlob object.ID   `json:"old_blob,omitzero"`
	NewMode object.Mode `json:"new_mode,omitzero"`
	NewBlob object.ID   `json:"new_blob,omitzero"`
}

// Status returns "added", "deleted" or "modified".
func (d FileDiff) Status() string {
	switch {
	case d.OldMode == object.ModeNone:
		return "added"
	case d.NewMode == object.ModeNone:
		return "deleted"
	}
	return "modified"
}

// AsChange converts the diff into the change that turns old into new.
func (d FileDiff) AsChange() object.Change {
	return object.Change{Path: d.Path, Mode: d.NewMode, Blob: d.NewBlob}
}

// DiffTrees returns the file-level differences between trees a and b, in
// path order. Identical subtrees are skipped by comparing IDs, so the cost
// depends on how much differs, not on the size of the trees. If prefixes is
// non-empty only paths under one of them are reported (and only those
// subtrees are read).
func (r *Repo) DiffTrees(ctx context.Context, a, b object.ID, prefixes ...string) ([]FileDiff, error) {
	var out []FileDiff
	err := r.diffTrees(ctx, "", a, b, prefixes, func(d FileDiff) error {
		out = append(out, d)
		return nil
	})
	return out, err
}

func inScope(p string, isDir bool, prefixes []string) bool {
	if len(prefixes) == 0 {
		return true
	}
	for _, pre := range prefixes {
		if object.HasPathPrefix(p, pre) {
			return true
		}
		// A directory that is an ancestor of a prefix must still be entered.
		if isDir && object.HasPathPrefix(pre, p) {
			return true
		}
	}
	return false
}

func (r *Repo) diffTrees(ctx context.Context, dir string, a, b object.ID, prefixes []string, emit func(FileDiff) error) error {
	if a == b {
		return nil
	}
	ta, err := r.ReadTree(ctx, a)
	if err != nil {
		return err
	}
	tb, err := r.ReadTree(ctx, b)
	if err != nil {
		return err
	}
	i, j := 0, 0
	for i < len(ta.Entries) || j < len(tb.Entries) {
		var ea, eb *object.TreeEntry
		switch {
		case j >= len(tb.Entries) || (i < len(ta.Entries) && ta.Entries[i].Name < tb.Entries[j].Name):
			ea = &ta.Entries[i]
			i++
		case i >= len(ta.Entries) || tb.Entries[j].Name < ta.Entries[i].Name:
			eb = &tb.Entries[j]
			j++
		default:
			ea, eb = &ta.Entries[i], &tb.Entries[j]
			i++
			j++
		}
		name := ""
		if ea != nil {
			name = ea.Name
		} else {
			name = eb.Name
		}
		p := object.JoinPath(dir, name)
		if ea != nil && eb != nil && ea.Mode == eb.Mode && ea.ID == eb.ID {
			continue
		}
		var aDir, bDir object.ID
		var aFile, bFile *object.TreeEntry
		if ea != nil {
			if ea.Mode.IsDir() {
				aDir = ea.ID
			} else {
				aFile = ea
			}
		}
		if eb != nil {
			if eb.Mode.IsDir() {
				bDir = eb.ID
			} else {
				bFile = eb
			}
		}
		// Deletions before additions keeps output valid as a change list when
		// a file is replaced by a directory of the same name or vice versa.
		if aFile != nil || bFile != nil {
			if inScope(p, false, prefixes) {
				d := FileDiff{Path: p}
				if aFile != nil {
					d.OldMode, d.OldBlob = aFile.Mode, aFile.ID
				}
				if bFile != nil {
					d.NewMode, d.NewBlob = bFile.Mode, bFile.ID
				}
				if err := emit(d); err != nil {
					return err
				}
			}
		}
		if !aDir.IsZero() || !bDir.IsZero() {
			if inScope(p, true, prefixes) {
				if aDir.IsZero() {
					aDir = object.EmptyTreeID
				}
				if bDir.IsZero() {
					bDir = object.EmptyTreeID
				}
				if err := r.diffTrees(ctx, p, aDir, bDir, prefixes, emit); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// ChangesBetween returns the change list that turns tree a into tree b,
// sorted canonically.
func (r *Repo) ChangesBetween(ctx context.Context, a, b object.ID) ([]object.Change, error) {
	diffs, err := r.DiffTrees(ctx, a, b)
	if err != nil {
		return nil, err
	}
	out := make([]object.Change, len(diffs))
	for i, d := range diffs {
		out[i] = d.AsChange()
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}
