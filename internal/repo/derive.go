package repo

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/mattparavel/a-git/internal/object"
	"github.com/mattparavel/a-git/internal/store"
)

// Derived data
//
// Anything that can be computed from changesets — the root tree, commit-graph
// indexes, per-path history, rename detection — is "derived data". Each kind
// is defined recursively: its value for a changeset is a function of the
// changeset itself and the values already derived for its ancestors. The
// engine guarantees that before a value is derived, every ancestor has been
// derived, so derivation only ever does work proportional to the new
// changesets.
//
// Kinds marked Eager are derived when a changeset is created; the rest are
// derived lazily the first time someone asks. Values are deterministic, so
// any server can compute them and concurrent writers never disagree. Clients
// never see the internal format, so new kinds can be added freely.

// Derived data kinds.
const (
	DerivedTree    = "tree"    // root tree ID
	DerivedGraph   = "graph"   // generation number, first-parent depth, skip list
	DerivedHistory = "history" // root history node ID
	DerivedCopies  = "copies"  // detected renames/copies
	DerivedBlame   = "blame"   // keyed by file history node, not changeset
)

// Deriver computes one kind of derived data.
type Deriver struct {
	Kind  string
	Eager bool
	// Derive computes the value for changeset id. When it is called, the
	// value of this kind already exists for every ancestor of id.
	Derive func(ctx context.Context, r *Repo, id object.ID, cs *object.Changeset) ([]byte, error)
}

var derivers = map[string]*Deriver{}

// derivationOrder is the order eager kinds are computed at creation time.
var derivationOrder []string

func registerDeriver(d *Deriver) {
	derivers[d.Kind] = d
	derivationOrder = append(derivationOrder, d.Kind)
}

func init() {
	registerDeriver(&Deriver{Kind: DerivedTree, Eager: true, Derive: deriveTree})
	registerDeriver(&Deriver{Kind: DerivedGraph, Eager: true, Derive: deriveGraph})
	registerDeriver(&Deriver{Kind: DerivedHistory, Eager: false, Derive: deriveHistory})
	registerDeriver(&Deriver{Kind: DerivedCopies, Eager: false, Derive: deriveCopies})
}

// DerivedKinds lists the registered changeset-keyed derived data kinds.
func DerivedKinds() []string { return append([]string(nil), derivationOrder...) }

// IsEager reports whether a kind is derived at changeset creation time.
func IsEager(kind string) bool {
	d := derivers[kind]
	return d != nil && d.Eager
}

func (r *Repo) cachedDerived(ctx context.Context, kind string, id object.ID) ([]byte, error) {
	key := kind + "/" + id.String()
	if v, ok := r.derived.Get(key); ok {
		return v, nil
	}
	v, err := r.Store.GetDerived(ctx, r.Name, kind, id)
	if err != nil {
		return nil, err
	}
	r.derived.Add(key, v)
	return v, nil
}

func (r *Repo) putDerived(ctx context.Context, kind string, id object.ID, v []byte) error {
	if err := r.Store.PutDerived(ctx, r.Name, kind, id, v); err != nil {
		return err
	}
	r.derived.Add(kind+"/"+id.String(), v)
	return nil
}

func (r *Repo) kindLock(kind string) *sync.Mutex {
	m, _ := r.deriveMu.LoadOrStore(kind, &sync.Mutex{})
	return m.(*sync.Mutex)
}

// Derived returns the derived value of the given kind for changeset id,
// computing it (and any missing ancestor values) if necessary.
func (r *Repo) Derived(ctx context.Context, kind string, id object.ID) ([]byte, error) {
	d := derivers[kind]
	if d == nil {
		return nil, fmt.Errorf("unknown derived data kind %q", kind)
	}
	if v, err := r.cachedDerived(ctx, kind, id); err == nil {
		return v, nil
	} else if !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}

	// Serialise slow-path derivation per kind within this process so that two
	// requests do not redo the same ancestor walk. Other processes may race
	// us; that is harmless because derivation is deterministic.
	mu := r.kindLock(kind)
	mu.Lock()
	defer mu.Unlock()

	order, err := r.underivedAncestors(ctx, kind, id)
	if err != nil {
		return nil, err
	}
	var last []byte
	for _, cid := range order {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		cs, err := r.GetChangeset(ctx, cid)
		if err != nil {
			return nil, err
		}
		v, err := d.Derive(ctx, r, cid, cs)
		if err != nil {
			return nil, fmt.Errorf("deriving %s for %s: %w", kind, cid.Short(), err)
		}
		if err := r.putDerived(ctx, kind, cid, v); err != nil {
			return nil, err
		}
		last = v
	}
	if last == nil {
		// Someone else derived it while we waited for the lock.
		return r.cachedDerived(ctx, kind, id)
	}
	return last, nil
}

// underivedAncestors returns id and every ancestor lacking a value of kind,
// ordered so that parents come before children. Because values are always
// derived parents-first, the walk can stop at any changeset that already has
// a value: all of its ancestors are guaranteed to have one too.
func (r *Repo) underivedAncestors(ctx context.Context, kind string, id object.ID) ([]object.ID, error) {
	type frame struct {
		id   object.ID
		post bool
	}
	var order []object.ID
	visited := map[object.ID]bool{}
	stack := []frame{{id: id}}
	for len(stack) > 0 {
		f := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if f.post {
			order = append(order, f.id)
			continue
		}
		if visited[f.id] {
			continue
		}
		visited[f.id] = true
		if _, err := r.cachedDerived(ctx, kind, f.id); err == nil {
			continue
		} else if !errors.Is(err, store.ErrNotFound) {
			return nil, err
		}
		cs, err := r.GetChangeset(ctx, f.id)
		if err != nil {
			return nil, err
		}
		stack = append(stack, frame{id: f.id, post: true})
		for i := len(cs.Parents) - 1; i >= 0; i-- {
			if !visited[cs.Parents[i]] {
				stack = append(stack, frame{id: cs.Parents[i]})
			}
		}
	}
	return order, nil
}

// deriveEager computes all eager kinds for a freshly stored changeset.
func (r *Repo) deriveEager(ctx context.Context, id object.ID) error {
	for _, kind := range derivationOrder {
		if derivers[kind].Eager {
			if _, err := r.Derived(ctx, kind, id); err != nil {
				return err
			}
		}
	}
	return nil
}
