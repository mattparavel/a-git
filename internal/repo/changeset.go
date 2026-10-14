package repo

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/mattparavel/a-git/internal/object"
	"github.com/mattparavel/a-git/internal/store"
)

// CreateChangeset validates and stores a changeset and derives its eager
// data. It is idempotent: creating an existing changeset returns its ID.
// Creating a changeset takes no locks and touches no refs; its cost depends
// on the size of the change, not the size of the repository.
func (r *Repo) CreateChangeset(ctx context.Context, cs *object.Changeset) (object.ID, error) {
	cs.Normalize()
	if err := cs.Validate(); err != nil {
		return object.Zero, fmt.Errorf("%w: invalid changeset: %v", ErrInvalid, err)
	}
	id, enc := object.EncodeChangeset(cs)
	if ok, err := r.HasChangeset(ctx, id); err != nil {
		return object.Zero, err
	} else if ok {
		return id, r.deriveEager(ctx, id)
	}
	for _, p := range cs.Parents {
		if ok, err := r.HasChangeset(ctx, p); err != nil {
			return object.Zero, err
		} else if !ok {
			return object.Zero, fmt.Errorf("parent %s: %w", p.Short(), ErrNotFound)
		}
	}
	for _, ch := range cs.Changes {
		if ch.IsDelete() {
			continue
		}
		stored, err := r.Store.GetObject(ctx, r.Name, ch.Blob)
		if errors.Is(err, store.ErrNotFound) {
			return object.Zero, fmt.Errorf("blob %s for %q has not been uploaded: %w", ch.Blob.Short(), ch.Path, ErrNotFound)
		} else if err != nil {
			return object.Zero, err
		}
		if object.KindOf(stored) != object.KindBlob {
			return object.Zero, fmt.Errorf("%w: object %s for %q is not a blob", ErrInvalid, ch.Blob.Short(), ch.Path)
		}
	}
	// Build the root tree before accepting the changeset so that changes
	// which do not apply cleanly to the first parent are rejected.
	base, err := r.RootTree(ctx, cs.FirstParent())
	if err != nil {
		return object.Zero, err
	}
	root, err := r.ApplyChanges(ctx, base, cs.Changes)
	if err != nil {
		return object.Zero, fmt.Errorf("%w: changes do not apply to the first parent: %v", ErrInvalid, err)
	}
	if _, err := r.Store.PutChangeset(ctx, r.Name, id, enc); err != nil {
		return object.Zero, err
	}
	r.changesets.Add(id, cs)
	if err := r.putDerived(ctx, DerivedTree, id, root[:]); err != nil {
		return object.Zero, err
	}
	return id, r.deriveEager(ctx, id)
}

// MoveRef advances a ref with compare-and-swap semantics. The target must be
// an existing changeset.
func (r *Repo) MoveRef(ctx context.Context, name string, old, new object.ID, who string) error {
	if !store.ValidRefName(name) {
		return fmt.Errorf("%w: invalid ref name %q", ErrInvalid, name)
	}
	if !new.IsZero() {
		if ok, err := r.HasChangeset(ctx, new); err != nil {
			return err
		} else if !ok {
			return fmt.Errorf("changeset %s: %w", new.Short(), ErrNotFound)
		}
	}
	return r.Store.CASRef(ctx, r.Name, name, old, new, who)
}

// Ref returns the changeset a ref points to.
func (r *Repo) Ref(ctx context.Context, name string) (object.ID, error) {
	return r.Store.GetRef(ctx, r.Name, name)
}

// ErrAmbiguous is returned when a revision prefix matches several changesets.
var ErrAmbiguous = errors.New("ambiguous revision")

// ErrInvalid wraps errors caused by malformed requests.
var ErrInvalid = errors.New("invalid request")

// Resolve turns a revision expression into a changeset ID. Supported forms:
// a ref name ("main", "release/1.2"), a full or abbreviated (>= 4 hex digits)
// changeset ID, each optionally followed by any sequence of "~N" (N-th
// first-parent ancestor), "^" (first parent) and "^N" (N-th parent).
func (r *Repo) Resolve(ctx context.Context, rev string) (object.ID, error) {
	rev = strings.TrimSpace(rev)
	if rev == "" {
		return object.Zero, fmt.Errorf("%w: empty revision", ErrInvalid)
	}
	cut := strings.IndexAny(rev, "~^")
	base, suffix := rev, ""
	if cut >= 0 {
		base, suffix = rev[:cut], rev[cut:]
	}
	id, err := r.resolveBase(ctx, base)
	if err != nil {
		return object.Zero, err
	}
	for len(suffix) > 0 {
		op := suffix[0]
		suffix = suffix[1:]
		n := 0
		digits := 0
		for digits < len(suffix) && suffix[digits] >= '0' && suffix[digits] <= '9' {
			digits++
		}
		if digits > 0 {
			n, _ = strconv.Atoi(suffix[:digits])
			suffix = suffix[digits:]
		} else {
			n = 1
		}
		switch op {
		case '~':
			if id, err = r.FirstParentAncestor(ctx, id, uint64(n)); err != nil {
				return object.Zero, fmt.Errorf("%s: %w", rev, err)
			}
		case '^':
			if n == 0 {
				continue
			}
			cs, err := r.GetChangeset(ctx, id)
			if err != nil {
				return object.Zero, err
			}
			if n > len(cs.Parents) {
				return object.Zero, fmt.Errorf("%s: changeset %s has no parent %d: %w", rev, id.Short(), n, ErrNotFound)
			}
			id = cs.Parents[n-1]
		}
	}
	return id, nil
}

func (r *Repo) resolveBase(ctx context.Context, s string) (object.ID, error) {
	if len(s) == 64 && object.IsHexPrefix(s) {
		id, err := object.ParseID(s)
		if err != nil {
			return object.Zero, err
		}
		if ok, err := r.HasChangeset(ctx, id); err != nil {
			return object.Zero, err
		} else if !ok {
			return object.Zero, fmt.Errorf("changeset %s: %w", id.Short(), ErrNotFound)
		}
		return id, nil
	}
	if store.ValidRefName(s) {
		id, err := r.Ref(ctx, s)
		if err == nil {
			return id, nil
		}
		if !errors.Is(err, store.ErrNotFound) {
			return object.Zero, err
		}
	}
	if len(s) >= 4 && object.IsHexPrefix(s) {
		ids, err := r.Store.FindChangesets(ctx, r.Name, s, 2)
		if err != nil {
			return object.Zero, err
		}
		switch len(ids) {
		case 1:
			return ids[0], nil
		case 2:
			return object.Zero, fmt.Errorf("%q: %w", s, ErrAmbiguous)
		}
	}
	return object.Zero, fmt.Errorf("unknown revision %q: %w", s, ErrNotFound)
}
