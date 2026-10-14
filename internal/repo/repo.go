// Package repo implements a-git's server-side repository logic: changeset
// creation, derived data, and the history, diff, merge, rebase and land
// operations that clients invoke over the API instead of running locally.
package repo

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/mattparavel/a-git/internal/lru"
	"github.com/mattparavel/a-git/internal/object"
	"github.com/mattparavel/a-git/internal/store"
)

// Repo is a handle on one repository in a store.
type Repo struct {
	Name  string
	Store store.Store

	trees      *lru.Cache[object.ID, *object.Tree]
	hists      *lru.Cache[object.ID, *object.HistNode]
	changesets *lru.Cache[object.ID, *object.Changeset]
	derived    *lru.Cache[string, []byte]

	deriveMu sync.Map // derived kind -> *sync.Mutex
}

// Open returns a handle for an existing repository.
func Open(ctx context.Context, s store.Store, name string) (*Repo, error) {
	ok, err := s.RepoExists(ctx, name)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("repository %q: %w", name, store.ErrNotFound)
	}
	return newRepo(s, name), nil
}

func newRepo(s store.Store, name string) *Repo {
	return &Repo{
		Name:       name,
		Store:      s,
		trees:      lru.New[object.ID, *object.Tree](200_000, func(t *object.Tree) int64 { return int64(len(t.Entries)) + 1 }),
		hists:      lru.New[object.ID, *object.HistNode](200_000, func(h *object.HistNode) int64 { return int64(len(h.Children)) + 1 }),
		changesets: lru.New[object.ID, *object.Changeset](50_000, func(c *object.Changeset) int64 { return int64(len(c.Changes)) + 1 }),
		derived:    lru.New[string, []byte](64<<20, func(b []byte) int64 { return int64(len(b)) + 64 }),
	}
}

// Registry caches Repo handles so their in-memory caches are shared.
type Registry struct {
	Store store.Store
	mu    sync.Mutex
	repos map[string]*Repo
}

// NewRegistry returns a registry over s.
func NewRegistry(s store.Store) *Registry { return &Registry{Store: s, repos: map[string]*Repo{}} }

// Get returns the handle for name.
func (g *Registry) Get(ctx context.Context, name string) (*Repo, error) {
	g.mu.Lock()
	r := g.repos[name]
	g.mu.Unlock()
	if r != nil {
		return r, nil
	}
	r, err := Open(ctx, g.Store, name)
	if err != nil {
		return nil, err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if existing := g.repos[name]; existing != nil {
		return existing, nil
	}
	g.repos[name] = r
	return r, nil
}

// Create creates a new, empty repository and returns its handle.
func (g *Registry) Create(ctx context.Context, name string) (*Repo, error) {
	if err := g.Store.CreateRepo(ctx, name); err != nil {
		return nil, err
	}
	return g.Get(ctx, name)
}

// ErrNotFound reports a missing object, changeset, path or ref.
var ErrNotFound = store.ErrNotFound

// ---- objects ----

// PutObject stores an object given its raw content and returns its ID.
func (r *Repo) PutObject(ctx context.Context, kind object.Kind, raw []byte) (object.ID, error) {
	id, stored := object.Encode(kind, raw)
	return id, r.Store.PutObject(ctx, r.Name, id, stored)
}

// PutStored stores an object already in stored form after verifying it.
func (r *Repo) PutStored(ctx context.Context, id object.ID, stored []byte) error {
	if _, _, err := object.Verify(id, stored); err != nil {
		return err
	}
	return r.Store.PutObject(ctx, r.Name, id, stored)
}

// GetStored returns an object in stored form.
func (r *Repo) GetStored(ctx context.Context, id object.ID) ([]byte, error) {
	return r.Store.GetObject(ctx, r.Name, id)
}

// ReadObject returns an object's kind and raw content.
func (r *Repo) ReadObject(ctx context.Context, id object.ID) (object.Kind, []byte, error) {
	stored, err := r.Store.GetObject(ctx, r.Name, id)
	if err != nil {
		return 0, nil, err
	}
	return object.Decode(stored)
}

// ReadBlob returns the content of a blob.
func (r *Repo) ReadBlob(ctx context.Context, id object.ID) ([]byte, error) {
	kind, raw, err := r.ReadObject(ctx, id)
	if err != nil {
		return nil, err
	}
	if kind != object.KindBlob {
		return nil, fmt.Errorf("object %s is a %s, not a blob", id.Short(), kind)
	}
	return raw, nil
}

// ReadTree returns a decoded tree. Trees are immutable and cached.
func (r *Repo) ReadTree(ctx context.Context, id object.ID) (*object.Tree, error) {
	if id.IsZero() || id == object.EmptyTreeID {
		return &object.Tree{}, nil
	}
	if t, ok := r.trees.Get(id); ok {
		return t, nil
	}
	kind, raw, err := r.ReadObject(ctx, id)
	if err != nil {
		return nil, err
	}
	if kind != object.KindTree {
		return nil, fmt.Errorf("object %s is a %s, not a tree", id.Short(), kind)
	}
	t, err := object.DecodeTree(raw)
	if err != nil {
		return nil, err
	}
	r.trees.Add(id, t)
	return t, nil
}

// WriteTree stores a tree and returns its ID.
func (r *Repo) WriteTree(ctx context.Context, t *object.Tree) (object.ID, error) {
	id, stored := object.EncodeTree(t)
	if err := r.Store.PutObject(ctx, r.Name, id, stored); err != nil {
		return object.Zero, err
	}
	r.trees.Add(id, t)
	return id, nil
}

// ReadHist returns a decoded history node.
func (r *Repo) ReadHist(ctx context.Context, id object.ID) (*object.HistNode, error) {
	if h, ok := r.hists.Get(id); ok {
		return h, nil
	}
	kind, raw, err := r.ReadObject(ctx, id)
	if err != nil {
		return nil, err
	}
	if kind != object.KindHist {
		return nil, fmt.Errorf("object %s is a %s, not a history node", id.Short(), kind)
	}
	h, err := object.DecodeHist(raw)
	if err != nil {
		return nil, err
	}
	r.hists.Add(id, h)
	return h, nil
}

// WriteHist stores a history node and returns its ID.
func (r *Repo) WriteHist(ctx context.Context, h *object.HistNode) (object.ID, error) {
	id, stored := object.EncodeHist(h)
	if err := r.Store.PutObject(ctx, r.Name, id, stored); err != nil {
		return object.Zero, err
	}
	r.hists.Add(id, h)
	return id, nil
}

// ---- changesets ----

// GetChangeset loads a changeset by ID.
func (r *Repo) GetChangeset(ctx context.Context, id object.ID) (*object.Changeset, error) {
	if c, ok := r.changesets.Get(id); ok {
		return c, nil
	}
	b, err := r.Store.GetChangeset(ctx, r.Name, id)
	if err != nil {
		return nil, err
	}
	c, err := object.DecodeChangeset(b)
	if err != nil {
		return nil, err
	}
	r.changesets.Add(id, c)
	return c, nil
}

// HasChangeset reports whether a changeset exists.
func (r *Repo) HasChangeset(ctx context.Context, id object.ID) (bool, error) {
	_, err := r.GetChangeset(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}

// RootTree returns the root tree of a changeset (derived data).
func (r *Repo) RootTree(ctx context.Context, cs object.ID) (object.ID, error) {
	if cs.IsZero() {
		return object.EmptyTreeID, nil
	}
	v, err := r.Derived(ctx, DerivedTree, cs)
	if err != nil {
		return object.Zero, err
	}
	var id object.ID
	copy(id[:], v)
	return id, nil
}

// Lookup returns the tree entry at path in tree root. The empty path returns
// a synthetic directory entry for the root itself.
func (r *Repo) Lookup(ctx context.Context, root object.ID, path string) (object.TreeEntry, error) {
	if path == "" {
		return object.TreeEntry{Mode: object.ModeDir, ID: root}, nil
	}
	cur := object.TreeEntry{Mode: object.ModeDir, ID: root}
	start := 0
	for i := 0; i <= len(path); i++ {
		if i < len(path) && path[i] != '/' {
			continue
		}
		name := path[start:i]
		start = i + 1
		if !cur.Mode.IsDir() {
			return object.TreeEntry{}, fmt.Errorf("path %q: %w", path, ErrNotFound)
		}
		t, err := r.ReadTree(ctx, cur.ID)
		if err != nil {
			return object.TreeEntry{}, err
		}
		e, ok := t.Find(name)
		if !ok {
			return object.TreeEntry{}, fmt.Errorf("path %q: %w", path, ErrNotFound)
		}
		cur = e
	}
	return cur, nil
}

// LookupAt returns the tree entry for path in changeset cs.
func (r *Repo) LookupAt(ctx context.Context, cs object.ID, path string) (object.TreeEntry, error) {
	root, err := r.RootTree(ctx, cs)
	if err != nil {
		return object.TreeEntry{}, err
	}
	return r.Lookup(ctx, root, path)
}

// ReadFileAt returns the content and mode of a file in changeset cs.
func (r *Repo) ReadFileAt(ctx context.Context, cs object.ID, path string) ([]byte, object.Mode, error) {
	e, err := r.LookupAt(ctx, cs, path)
	if err != nil {
		return nil, 0, err
	}
	if e.Mode.IsDir() {
		return nil, 0, fmt.Errorf("path %q is a directory", path)
	}
	b, err := r.ReadBlob(ctx, e.ID)
	return b, e.Mode, err
}

// WalkFiles calls fn for every file under tree root (restricted to prefix,
// "" for everything), in path order.
func (r *Repo) WalkFiles(ctx context.Context, root object.ID, prefix string, fn func(path string, e object.TreeEntry) error) error {
	start, err := r.Lookup(ctx, root, prefix)
	if err != nil {
		return err
	}
	if !start.Mode.IsDir() {
		return fn(prefix, start)
	}
	var walk func(dir string, id object.ID) error
	walk = func(dir string, id object.ID) error {
		t, err := r.ReadTree(ctx, id)
		if err != nil {
			return err
		}
		for _, e := range t.Entries {
			p := object.JoinPath(dir, e.Name)
			if e.Mode.IsDir() {
				if err := walk(p, e.ID); err != nil {
					return err
				}
			} else if err := fn(p, e); err != nil {
				return err
			}
		}
		return nil
	}
	return walk(prefix, start.ID)
}
