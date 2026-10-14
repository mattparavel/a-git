package store

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mattparavel/a-git/internal/object"
)

// Mem is an in-memory Store, used by tests and ephemeral servers.
type Mem struct {
	mu    sync.RWMutex
	repos map[string]*memRepo
}

type memRepo struct {
	objects    map[object.ID][]byte
	changesets map[object.ID][]byte
	wal        []WALEntry
	refs       map[string]object.ID
	reflog     []RefUpdate
	derived    map[string][]byte
	kv         map[string][]byte
	kvLocks    map[string]*sync.Mutex
}

// NewMem returns an empty in-memory store.
func NewMem() *Mem { return &Mem{repos: map[string]*memRepo{}} }

func (m *Mem) repo(name string) (*memRepo, error) {
	r := m.repos[name]
	if r == nil {
		return nil, fmt.Errorf("repository %q: %w", name, ErrNotFound)
	}
	return r, nil
}

func (m *Mem) CreateRepo(_ context.Context, name string) error {
	if !ValidRepoName(name) {
		return fmt.Errorf("invalid repository name %q", name)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.repos[name] != nil {
		return fmt.Errorf("repository %q: %w", name, ErrExists)
	}
	m.repos[name] = &memRepo{
		objects:    map[object.ID][]byte{},
		changesets: map[object.ID][]byte{},
		refs:       map[string]object.ID{},
		derived:    map[string][]byte{},
		kv:         map[string][]byte{},
		kvLocks:    map[string]*sync.Mutex{},
	}
	return nil
}

func (m *Mem) ListRepos(context.Context) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []string
	for n := range m.repos {
		out = append(out, n)
	}
	sort.Strings(out)
	return out, nil
}

func (m *Mem) RepoExists(_ context.Context, name string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.repos[name] != nil, nil
}

func (m *Mem) PutObject(_ context.Context, repo string, id object.ID, stored []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, err := m.repo(repo)
	if err != nil {
		return err
	}
	if _, ok := r.objects[id]; !ok {
		r.objects[id] = append([]byte(nil), stored...)
	}
	return nil
}

func (m *Mem) GetObject(_ context.Context, repo string, id object.ID) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	r, err := m.repo(repo)
	if err != nil {
		return nil, err
	}
	b, ok := r.objects[id]
	if !ok {
		return nil, fmt.Errorf("object %s: %w", id.Short(), ErrNotFound)
	}
	return b, nil
}

func (m *Mem) HasObject(_ context.Context, repo string, id object.ID) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	r, err := m.repo(repo)
	if err != nil {
		return false, err
	}
	_, ok := r.objects[id]
	return ok, nil
}

func (m *Mem) PutChangeset(_ context.Context, repo string, id object.ID, enc []byte) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, err := m.repo(repo)
	if err != nil {
		return false, err
	}
	if _, ok := r.changesets[id]; ok {
		return false, nil
	}
	r.changesets[id] = append([]byte(nil), enc...)
	r.wal = append(r.wal, WALEntry{Seq: int64(len(r.wal)) + 1, ID: id, Time: time.Now().UnixNano()})
	return true, nil
}

func (m *Mem) GetChangeset(_ context.Context, repo string, id object.ID) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	r, err := m.repo(repo)
	if err != nil {
		return nil, err
	}
	b, ok := r.changesets[id]
	if !ok {
		return nil, fmt.Errorf("changeset %s: %w", id.Short(), ErrNotFound)
	}
	return b, nil
}

func (m *Mem) FindChangesets(_ context.Context, repo, prefix string, limit int) ([]object.ID, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	r, err := m.repo(repo)
	if err != nil {
		return nil, err
	}
	var out []object.ID
	for id := range r.changesets {
		if strings.HasPrefix(id.String(), prefix) {
			out = append(out, id)
			if limit > 0 && len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

func (m *Mem) ReadWAL(_ context.Context, repo string, after int64, limit int) ([]WALEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	r, err := m.repo(repo)
	if err != nil {
		return nil, err
	}
	if after < 0 {
		after = 0
	}
	if after >= int64(len(r.wal)) {
		return nil, nil
	}
	out := r.wal[after:]
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return append([]WALEntry(nil), out...), nil
}

func (m *Mem) GetRef(_ context.Context, repo, name string) (object.ID, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	r, err := m.repo(repo)
	if err != nil {
		return object.Zero, err
	}
	id, ok := r.refs[name]
	if !ok {
		return object.Zero, fmt.Errorf("ref %q: %w", name, ErrNotFound)
	}
	return id, nil
}

func (m *Mem) ListRefs(_ context.Context, repo, prefix string) ([]Ref, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	r, err := m.repo(repo)
	if err != nil {
		return nil, err
	}
	var out []Ref
	for n, id := range r.refs {
		if strings.HasPrefix(n, prefix) {
			out = append(out, Ref{Name: n, ID: id})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (m *Mem) CASRef(_ context.Context, repo, name string, old, new object.ID, who string) error {
	if !ValidRefName(name) {
		return fmt.Errorf("invalid ref name %q", name)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	r, err := m.repo(repo)
	if err != nil {
		return err
	}
	cur := r.refs[name]
	if cur != old {
		return fmt.Errorf("ref %q is at %s, expected %s: %w", name, cur.Short(), old.Short(), ErrCASFailed)
	}
	if new.IsZero() {
		delete(r.refs, name)
	} else {
		r.refs[name] = new
	}
	r.reflog = append(r.reflog, RefUpdate{Seq: int64(len(r.reflog)) + 1, Name: name, Old: old, New: new, Time: time.Now().UnixNano(), Who: who})
	return nil
}

func (m *Mem) ReadRefLog(_ context.Context, repo string, after int64, limit int) ([]RefUpdate, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	r, err := m.repo(repo)
	if err != nil {
		return nil, err
	}
	if after < 0 {
		after = 0
	}
	if after >= int64(len(r.reflog)) {
		return nil, nil
	}
	out := r.reflog[after:]
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return append([]RefUpdate(nil), out...), nil
}

func (m *Mem) GetDerived(_ context.Context, repo, kind string, key object.ID) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	r, err := m.repo(repo)
	if err != nil {
		return nil, err
	}
	b, ok := r.derived[kind+"/"+key.String()]
	if !ok {
		return nil, ErrNotFound
	}
	return b, nil
}

func (m *Mem) PutDerived(_ context.Context, repo, kind string, key object.ID, value []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, err := m.repo(repo)
	if err != nil {
		return err
	}
	r.derived[kind+"/"+key.String()] = append([]byte(nil), value...)
	return nil
}

func (m *Mem) KVGet(_ context.Context, repo, ns, key string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	r, err := m.repo(repo)
	if err != nil {
		return nil, err
	}
	b, ok := r.kv[ns+"/"+key]
	if !ok {
		return nil, ErrNotFound
	}
	return b, nil
}

func (m *Mem) KVUpdate(_ context.Context, repo, ns, key string, fn func([]byte) ([]byte, error)) error {
	if !ValidKVKey(ns) || !ValidKVKey(key) {
		return fmt.Errorf("invalid key %q/%q", ns, key)
	}
	m.mu.Lock()
	r, err := m.repo(repo)
	if err != nil {
		m.mu.Unlock()
		return err
	}
	k := ns + "/" + key
	l := r.kvLocks[k]
	if l == nil {
		l = &sync.Mutex{}
		r.kvLocks[k] = l
	}
	m.mu.Unlock()

	l.Lock()
	defer l.Unlock()
	m.mu.RLock()
	old := r.kv[k]
	m.mu.RUnlock()
	nv, err := fn(old)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if nv == nil {
		delete(r.kv, k)
	} else {
		r.kv[k] = append([]byte(nil), nv...)
	}
	return nil
}

func (m *Mem) KVList(_ context.Context, repo, ns, prefix string) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	r, err := m.repo(repo)
	if err != nil {
		return nil, err
	}
	var out []string
	p := ns + "/" + prefix
	for k := range r.kv {
		if strings.HasPrefix(k, p) {
			out = append(out, k[len(ns)+1:])
		}
	}
	sort.Strings(out)
	return out, nil
}
