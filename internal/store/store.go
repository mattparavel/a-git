// Package store defines a-git's authoritative storage interfaces and two
// implementations: an in-memory store for tests and a filesystem store for
// real deployments.
//
// The storage split mirrors a cloud deployment: immutable objects (blobs,
// trees, history nodes) go to a blob store, while changesets, refs, derived
// data and review records go to a transactional metadata store. Nothing in the
// write path needs a global lock:
//
//   - Objects and changesets are content-addressed, so concurrent writers of the
//     same record write identical bytes and inserts are idempotent.
//   - Each new changeset is appended to a per-repository write-ahead log whose
//     entries have a fixed width, so the log position doubles as a sequence
//     number without any coordination between writers.
//   - Refs are the only mutable state and are updated by compare-and-swap on a
//     single record.
package store

import (
	"context"
	"errors"

	"github.com/mattparavel/a-git/internal/object"
)

var (
	// ErrNotFound is returned when a record does not exist.
	ErrNotFound = errors.New("not found")
	// ErrCASFailed is returned when a compare-and-swap precondition fails.
	ErrCASFailed = errors.New("compare-and-swap failed: ref has moved")
	// ErrExists is returned when creating something that already exists.
	ErrExists = errors.New("already exists")
)

// WALEntry is one record of a repository's changeset log.
type WALEntry struct {
	Seq  int64     `json:"seq"`
	ID   object.ID `json:"id"`
	Time int64     `json:"time"` // unix nanoseconds when the changeset was accepted
}

// RefUpdate records one successful ref transition.
type RefUpdate struct {
	Seq  int64     `json:"seq"`
	Name string    `json:"name"`
	Old  object.ID `json:"old"`
	New  object.ID `json:"new"`
	Time int64     `json:"time"`
	Who  string    `json:"who,omitempty"`
}

// Ref is a named pointer to a changeset.
type Ref struct {
	Name string    `json:"name"`
	ID   object.ID `json:"id"`
}

// Store is the complete storage backend used by the authoritative server.
type Store interface {
	// Repositories.
	CreateRepo(ctx context.Context, name string) error
	ListRepos(ctx context.Context) ([]string, error)
	RepoExists(ctx context.Context, name string) (bool, error)

	// Objects, in stored form (see object.Encode).
	PutObject(ctx context.Context, repo string, id object.ID, stored []byte) error
	GetObject(ctx context.Context, repo string, id object.ID) ([]byte, error)
	HasObject(ctx context.Context, repo string, id object.ID) (bool, error)

	// Changesets. PutChangeset is idempotent; created reports whether this call
	// inserted the record (and therefore appended it to the log).
	PutChangeset(ctx context.Context, repo string, id object.ID, enc []byte) (created bool, err error)
	GetChangeset(ctx context.Context, repo string, id object.ID) ([]byte, error)
	// FindChangesets returns the IDs of changesets whose hex form starts with prefix.
	FindChangesets(ctx context.Context, repo, prefix string, limit int) ([]object.ID, error)
	ReadWAL(ctx context.Context, repo string, afterSeq int64, limit int) ([]WALEntry, error)

	// Refs.
	GetRef(ctx context.Context, repo, name string) (object.ID, error)
	ListRefs(ctx context.Context, repo, prefix string) ([]Ref, error)
	// CASRef sets ref name to new if it currently equals old. A zero old means
	// the ref must not exist; a zero new deletes it.
	CASRef(ctx context.Context, repo, name string, old, new object.ID, who string) error
	ReadRefLog(ctx context.Context, repo string, afterSeq int64, limit int) ([]RefUpdate, error)

	// Derived data, keyed by (kind, id). Values are deterministic functions of
	// their key, so concurrent writers never disagree.
	GetDerived(ctx context.Context, repo, kind string, key object.ID) ([]byte, error)
	PutDerived(ctx context.Context, repo, kind string, key object.ID, value []byte) error

	// Small mutable documents (reviews, settings). Update runs fn atomically
	// with respect to other Updates of the same key; fn receives nil when the
	// key does not exist and returns the new value (nil deletes).
	KVGet(ctx context.Context, repo, ns, key string) ([]byte, error)
	KVUpdate(ctx context.Context, repo, ns, key string, fn func(old []byte) ([]byte, error)) error
	KVList(ctx context.Context, repo, ns, prefix string) ([]string, error)
}

// ValidRepoName reports whether name is an acceptable repository name.
func ValidRepoName(name string) bool {
	if name == "" || len(name) > 100 || name[0] == '.' || name[0] == '-' {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.') {
			return false
		}
	}
	return true
}

// ValidRefName reports whether name is an acceptable ref name
// (slash-separated components of [A-Za-z0-9._-]).
func ValidRefName(name string) bool {
	if name == "" || len(name) > 200 {
		return false
	}
	comp := 0
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c == '/':
			if comp == 0 {
				return false
			}
			comp = 0
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '_':
			comp++
		case c == '.':
			if comp == 0 {
				return false // no hidden components or ".."
			}
			comp++
		default:
			return false
		}
	}
	if comp == 0 {
		return false
	}
	// Refs must not look like changeset IDs or revision expressions.
	if object.IsHexPrefix(name) && len(name) >= 8 {
		return false
	}
	return true
}

// ValidKVKey reports whether key is safe to use as a KV key or namespace.
func ValidKVKey(key string) bool {
	if key == "" || len(key) > 200 || key[0] == '.' {
		return false
	}
	for i := 0; i < len(key); i++ {
		c := key[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.') {
			return false
		}
	}
	return true
}
