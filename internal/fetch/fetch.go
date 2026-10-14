// Package fetch implements a-git's client-driven fetch protocol.
//
// There is no negotiation about what the client already has. The client
// starts from a root tree, looks in its local store for the trees it needs,
// asks for the missing ones by ID, and repeats one directory level at a
// time; then it does the same for blobs. Every request names immutable,
// content-addressed objects, so any tier can answer it from cache and an
// interrupted fetch resumes by asking only for what is still missing.
// Sparse checkouts need nothing special: the client simply never asks for
// trees outside the paths it wants.
package fetch

import (
	"context"
	"fmt"

	"github.com/mattparavel/a-git/internal/api"
	"github.com/mattparavel/a-git/internal/client"
	"github.com/mattparavel/a-git/internal/object"
)

// Local is an optional local object store consulted before the network.
type Local interface {
	Get(id object.ID) ([]byte, bool)
	Put(id object.ID, stored []byte)
}

// Fetcher retrieves trees and blobs for one changeset.
type Fetcher struct {
	Src   *client.Repo
	Local Local     // may be nil
	CS    object.ID // changeset the objects belong to (location hints)
	// FromCS is the changeset of the "from" side in Diff.
	FromCS object.ID
	Batch  int // objects per request (default 1000)

	// Counters for reporting.
	Requested, FromLocal int
}

// Want names an object and where it lives.
type Want struct {
	ID   object.ID
	Path string
	CS   object.ID // changeset containing Path (defaults to Fetcher.CS)
}

func (f *Fetcher) batch() int {
	if f.Batch > 0 {
		return f.Batch
	}
	return 1000
}

// Get returns the raw content of each wanted object, fetching what the local
// store lacks.
func (f *Fetcher) Get(ctx context.Context, wants []Want, fn func(w Want, kind object.Kind, raw []byte) error) error {
	byID := map[object.ID][]Want{}
	var order []object.ID
	for _, w := range wants {
		if _, seen := byID[w.ID]; !seen {
			order = append(order, w.ID)
		}
		byID[w.ID] = append(byID[w.ID], w)
	}
	var remote []api.ObjectRef
	for _, id := range order {
		if f.Local != nil {
			if stored, ok := f.Local.Get(id); ok {
				if kind, raw, err := object.Decode(stored); err == nil {
					f.FromLocal++
					for _, x := range byID[id] {
						if err := fn(x, kind, raw); err != nil {
							return err
						}
					}
					continue
				}
			}
		}
		first := byID[id][0]
		cs := first.CS
		if cs.IsZero() {
			cs = f.CS
		}
		remote = append(remote, api.ObjectRef{ID: id, CS: cs, Path: first.Path})
	}
	for start := 0; start < len(remote); start += f.batch() {
		chunk := remote[start:min(start+f.batch(), len(remote))]
		f.Requested += len(chunk)
		missing, err := f.Src.GetObjects(ctx, chunk, func(id object.ID, stored []byte) error {
			if f.Local != nil {
				f.Local.Put(id, stored)
			}
			kind, raw, err := object.Decode(stored)
			if err != nil {
				return err
			}
			for _, x := range byID[id] {
				if err := fn(x, kind, raw); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
		if len(missing) > 0 {
			return fmt.Errorf("server could not provide %d object(s), e.g. %s: not found or not readable", len(missing), missing[0].Short())
		}
	}
	return nil
}

// Entry is a file found by Walk.
type Entry struct {
	Path string
	Mode object.Mode
	ID   object.ID
}

// Walk lists the files under root for which include returns true, fetching
// trees one level at a time in batches. include is called for directories
// (to decide whether to descend) and files; a nil include accepts all.
func (f *Fetcher) Walk(ctx context.Context, root object.ID, include func(path string, isDir bool) bool, fn func(Entry) error) error {
	if include == nil {
		include = func(string, bool) bool { return true }
	}
	level := []Want{{ID: root, Path: ""}}
	for len(level) > 0 {
		var next []Want
		var files []Entry
		err := f.Get(ctx, level, func(w Want, kind object.Kind, raw []byte) error {
			if kind != object.KindTree {
				return fmt.Errorf("object %s at %q is a %s, expected a tree", w.ID.Short(), w.Path, kind)
			}
			t, err := object.DecodeTree(raw)
			if err != nil {
				return err
			}
			for _, e := range t.Entries {
				p := object.JoinPath(w.Path, e.Name)
				if !include(p, e.Mode.IsDir()) {
					continue
				}
				if e.Mode.IsDir() {
					next = append(next, Want{ID: e.ID, Path: p})
				} else {
					files = append(files, Entry{Path: p, Mode: e.Mode, ID: e.ID})
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
		for _, e := range files {
			if err := fn(e); err != nil {
				return err
			}
		}
		level = next
	}
	return nil
}

// Tree fetches and decodes a single tree.
func (f *Fetcher) Tree(ctx context.Context, id object.ID, path string) (*object.Tree, error) {
	if id == object.EmptyTreeID {
		return &object.Tree{}, nil
	}
	var out *object.Tree
	err := f.Get(ctx, []Want{{ID: id, Path: path}}, func(_ Want, kind object.Kind, raw []byte) error {
		if kind != object.KindTree {
			return fmt.Errorf("object %s is a %s, expected a tree", id.Short(), kind)
		}
		t, err := object.DecodeTree(raw)
		out = t
		return err
	})
	return out, err
}

// Diff lists the file-level differences between two root trees, restricted
// by include, fetching only the subtrees that differ.
func (f *Fetcher) Diff(ctx context.Context, from, to object.ID, include func(path string, isDir bool) bool, fn func(path string, old, new *Entry) error) error {
	if include == nil {
		include = func(string, bool) bool { return true }
	}
	type pair struct {
		path     string
		from, to object.ID
	}
	level := []pair{{"", from, to}}
	for len(level) > 0 {
		var wants []Want
		for _, p := range level {
			if !p.from.IsZero() {
				wants = append(wants, Want{ID: p.from, Path: p.path, CS: f.FromCS})
			}
			if !p.to.IsZero() {
				wants = append(wants, Want{ID: p.to, Path: p.path})
			}
		}
		trees := map[object.ID]*object.Tree{object.EmptyTreeID: {}}
		err := f.Get(ctx, wants, func(w Want, kind object.Kind, raw []byte) error {
			if kind != object.KindTree {
				return fmt.Errorf("object %s at %q is not a tree", w.ID.Short(), w.Path)
			}
			t, err := object.DecodeTree(raw)
			trees[w.ID] = t
			return err
		})
		if err != nil {
			return err
		}
		var next []pair
		for _, p := range level {
			a, b := trees[p.from], trees[p.to]
			if a == nil {
				a = &object.Tree{}
			}
			if b == nil {
				b = &object.Tree{}
			}
			i, j := 0, 0
			for i < len(a.Entries) || j < len(b.Entries) {
				var ea, eb *object.TreeEntry
				switch {
				case j >= len(b.Entries) || i < len(a.Entries) && a.Entries[i].Name < b.Entries[j].Name:
					ea = &a.Entries[i]
					i++
				case i >= len(a.Entries) || b.Entries[j].Name < a.Entries[i].Name:
					eb = &b.Entries[j]
					j++
				default:
					ea, eb = &a.Entries[i], &b.Entries[j]
					i++
					j++
				}
				if ea != nil && eb != nil && *ea == *eb {
					continue
				}
				name := ""
				if ea != nil {
					name = ea.Name
				} else {
					name = eb.Name
				}
				path := object.JoinPath(p.path, name)
				var oldFile, newFile *Entry
				var oldDir, newDir object.ID
				if ea != nil {
					if ea.Mode.IsDir() {
						oldDir = ea.ID
					} else {
						oldFile = &Entry{Path: path, Mode: ea.Mode, ID: ea.ID}
					}
				}
				if eb != nil {
					if eb.Mode.IsDir() {
						newDir = eb.ID
					} else {
						newFile = &Entry{Path: path, Mode: eb.Mode, ID: eb.ID}
					}
				}
				if (oldFile != nil || newFile != nil) && include(path, false) {
					if err := fn(path, oldFile, newFile); err != nil {
						return err
					}
				}
				if (!oldDir.IsZero() || !newDir.IsZero()) && include(path, true) {
					next = append(next, pair{path, oldDir, newDir})
				}
			}
		}
		level = next
	}
	return nil
}
