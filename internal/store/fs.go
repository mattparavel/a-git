package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mattparavel/a-git/internal/object"
)

// FS is a Store backed by a directory tree. Several server processes may
// share one FS root (for example on a network filesystem): object and
// changeset writes are idempotent link/rename operations, log appends are
// single fixed-width O_APPEND writes, and only ref and KV updates take a
// short per-record file lock.
//
// Layout of each repository directory:
//
//	objects/ab/cdef...      stored objects
//	changesets/ab/cdef...   encoded changesets
//	wal                     fixed-width changeset log ("<id> <unix-nanos>\n")
//	refs/<escaped name>     "<id>\n"
//	reflog                  fixed-width JSON records of ref transitions
//	derived/<kind>/ab/...   derived data
//	kv/<ns>/<key>           mutable documents
//	locks/                  lock files for refs and kv
//	tmp/                    staging area for atomic writes
type FS struct {
	root string
}

const (
	walRecordSize    = 64 + 1 + 19 + 1
	reflogRecordSize = 512
)

// OpenFS opens (creating if needed) a filesystem store rooted at dir.
func OpenFS(dir string) (*FS, error) {
	if err := os.MkdirAll(filepath.Join(dir, "repos"), 0o755); err != nil {
		return nil, err
	}
	return &FS{root: dir}, nil
}

func (s *FS) repoDir(name string) string { return filepath.Join(s.root, "repos", name) }

func (s *FS) checkRepo(name string) (string, error) {
	if !ValidRepoName(name) {
		return "", fmt.Errorf("invalid repository name %q", name)
	}
	d := s.repoDir(name)
	if _, err := os.Stat(d); err != nil {
		return "", fmt.Errorf("repository %q: %w", name, ErrNotFound)
	}
	return d, nil
}

func (s *FS) CreateRepo(_ context.Context, name string) error {
	if !ValidRepoName(name) {
		return fmt.Errorf("invalid repository name %q", name)
	}
	d := s.repoDir(name)
	// Build the layout in a temporary directory and rename it into place so a
	// half-created repository is never visible.
	tmp, err := os.MkdirTemp(filepath.Join(s.root, "repos"), ".new-")
	if err != nil {
		return err
	}
	for _, sub := range []string{"objects", "changesets", "refs", "derived", "kv", "locks", "tmp"} {
		if err := os.Mkdir(filepath.Join(tmp, sub), 0o755); err != nil {
			os.RemoveAll(tmp)
			return err
		}
	}
	for _, f := range []string{"wal", "reflog"} {
		if err := os.WriteFile(filepath.Join(tmp, f), nil, 0o644); err != nil {
			os.RemoveAll(tmp)
			return err
		}
	}
	if err := os.Rename(tmp, d); err != nil {
		os.RemoveAll(tmp)
		if errors.Is(err, fs.ErrExist) || isNotEmpty(err) {
			return fmt.Errorf("repository %q: %w", name, ErrExists)
		}
		return err
	}
	return nil
}

func isNotEmpty(err error) bool {
	var le *os.LinkError
	if errors.As(err, &le) {
		return errors.Is(le.Err, syscall.ENOTEMPTY) || errors.Is(le.Err, syscall.EEXIST)
	}
	return false
}

func (s *FS) ListRepos(context.Context) ([]string, error) {
	ents, err := os.ReadDir(filepath.Join(s.root, "repos"))
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range ents {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			out = append(out, e.Name())
		}
	}
	return out, nil
}

func (s *FS) RepoExists(_ context.Context, name string) (bool, error) {
	_, err := s.checkRepo(name)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}

func shard(dir string, id object.ID) string {
	h := id.String()
	return filepath.Join(dir, h[:2], h[2:])
}

// writeAtomic writes data to path via a temp file. If exclusive is true the
// final step is a hard link, which fails if path already exists; created
// reports whether this call created the file.
func (s *FS) writeAtomic(repoDir, path string, data []byte, exclusive bool) (created bool, err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	f, err := os.CreateTemp(filepath.Join(repoDir, "tmp"), "w-")
	if err != nil {
		return false, err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err := f.Write(data); err != nil {
		f.Close()
		return false, err
	}
	if err := f.Close(); err != nil {
		return false, err
	}
	if exclusive {
		if err := os.Link(tmp, path); err != nil {
			if errors.Is(err, fs.ErrExist) {
				return false, nil
			}
			return false, err
		}
		return true, nil
	}
	if err := os.Rename(tmp, path); err != nil {
		return false, err
	}
	return true, nil
}

func (s *FS) PutObject(_ context.Context, repo string, id object.ID, stored []byte) error {
	d, err := s.checkRepo(repo)
	if err != nil {
		return err
	}
	p := shard(filepath.Join(d, "objects"), id)
	if _, err := os.Stat(p); err == nil {
		return nil
	}
	_, err = s.writeAtomic(d, p, stored, true)
	return err
}

func (s *FS) GetObject(_ context.Context, repo string, id object.ID) ([]byte, error) {
	d, err := s.checkRepo(repo)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(shard(filepath.Join(d, "objects"), id))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("object %s: %w", id.Short(), ErrNotFound)
	}
	return b, err
}

func (s *FS) HasObject(_ context.Context, repo string, id object.ID) (bool, error) {
	d, err := s.checkRepo(repo)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(shard(filepath.Join(d, "objects"), id))
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

func (s *FS) PutChangeset(_ context.Context, repo string, id object.ID, enc []byte) (bool, error) {
	d, err := s.checkRepo(repo)
	if err != nil {
		return false, err
	}
	p := shard(filepath.Join(d, "changesets"), id)
	if _, err := os.Stat(p); err == nil {
		return false, nil
	}
	created, err := s.writeAtomic(d, p, enc, true)
	if err != nil || !created {
		return false, err
	}
	// Exactly one writer wins the link above, so exactly one appends the
	// changeset to the log.
	rec := fmt.Sprintf("%s %019d\n", id, time.Now().UnixNano())
	if err := appendRecord(filepath.Join(d, "wal"), []byte(rec)); err != nil {
		return true, err
	}
	return true, nil
}

func appendRecord(path string, rec []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	_, err = f.Write(rec)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

func (s *FS) GetChangeset(_ context.Context, repo string, id object.ID) ([]byte, error) {
	d, err := s.checkRepo(repo)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(shard(filepath.Join(d, "changesets"), id))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("changeset %s: %w", id.Short(), ErrNotFound)
	}
	return b, err
}

func (s *FS) FindChangesets(_ context.Context, repo, prefix string, limit int) ([]object.ID, error) {
	d, err := s.checkRepo(repo)
	if err != nil {
		return nil, err
	}
	if !object.IsHexPrefix(prefix) {
		return nil, nil
	}
	var dirs []string
	if len(prefix) >= 2 {
		dirs = []string{prefix[:2]}
	} else {
		for i := 0; i < 256; i++ {
			h := fmt.Sprintf("%02x", i)
			if strings.HasPrefix(h, prefix) {
				dirs = append(dirs, h)
			}
		}
	}
	var out []object.ID
	for _, sub := range dirs {
		ents, err := os.ReadDir(filepath.Join(d, "changesets", sub))
		if err != nil {
			continue
		}
		for _, e := range ents {
			full := sub + e.Name()
			if len(full) == 64 && strings.HasPrefix(full, prefix) {
				if id, err := object.ParseID(full); err == nil {
					out = append(out, id)
					if limit > 0 && len(out) >= limit {
						return out, nil
					}
				}
			}
		}
	}
	return out, nil
}

func readRecords(path string, size int, after int64, limit int) ([][]byte, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()
	if after < 0 {
		after = 0
	}
	if limit <= 0 {
		limit = 1000
	}
	buf := make([]byte, size*limit)
	n, err := f.ReadAt(buf, after*int64(size))
	if err != nil && err != io.EOF {
		return nil, 0, err
	}
	var out [][]byte
	for off := 0; off+size <= n; off += size {
		rec := buf[off : off+size]
		if rec[size-1] != '\n' {
			break // torn record from an in-flight append
		}
		out = append(out, rec)
	}
	return out, after, nil
}

func (s *FS) ReadWAL(_ context.Context, repo string, after int64, limit int) ([]WALEntry, error) {
	d, err := s.checkRepo(repo)
	if err != nil {
		return nil, err
	}
	recs, start, err := readRecords(filepath.Join(d, "wal"), walRecordSize, after, limit)
	if err != nil {
		return nil, err
	}
	out := make([]WALEntry, 0, len(recs))
	for i, r := range recs {
		id, err := object.ParseID(string(r[:64]))
		if err != nil {
			return nil, fmt.Errorf("corrupt wal record %d: %v", start+int64(i)+1, err)
		}
		ts, _ := strconv.ParseInt(string(r[65:84]), 10, 64)
		out = append(out, WALEntry{Seq: start + int64(i) + 1, ID: id, Time: ts})
	}
	return out, nil
}

func refFile(d, name string) string {
	return filepath.Join(d, "refs", url.PathEscape(name))
}

// withLock runs fn while holding an exclusive flock on a lock file.
func withLock(path string, fn func() error) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return fn()
}

func readRefFile(p string) (object.ID, error) {
	b, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return object.Zero, ErrNotFound
	}
	if err != nil {
		return object.Zero, err
	}
	return object.ParseID(string(bytes.TrimSpace(b)))
}

func (s *FS) GetRef(_ context.Context, repo, name string) (object.ID, error) {
	d, err := s.checkRepo(repo)
	if err != nil {
		return object.Zero, err
	}
	if !ValidRefName(name) {
		return object.Zero, fmt.Errorf("ref %q: %w", name, ErrNotFound)
	}
	id, err := readRefFile(refFile(d, name))
	if errors.Is(err, ErrNotFound) {
		return object.Zero, fmt.Errorf("ref %q: %w", name, ErrNotFound)
	}
	return id, err
}

func (s *FS) ListRefs(_ context.Context, repo, prefix string) ([]Ref, error) {
	d, err := s.checkRepo(repo)
	if err != nil {
		return nil, err
	}
	ents, err := os.ReadDir(filepath.Join(d, "refs"))
	if err != nil {
		return nil, err
	}
	var out []Ref
	for _, e := range ents {
		name, err := url.PathUnescape(e.Name())
		if err != nil || !strings.HasPrefix(name, prefix) {
			continue
		}
		id, err := readRefFile(filepath.Join(d, "refs", e.Name()))
		if err != nil {
			continue
		}
		out = append(out, Ref{Name: name, ID: id})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (s *FS) CASRef(_ context.Context, repo, name string, old, new object.ID, who string) error {
	d, err := s.checkRepo(repo)
	if err != nil {
		return err
	}
	if !ValidRefName(name) {
		return fmt.Errorf("invalid ref name %q", name)
	}
	p := refFile(d, name)
	lock := filepath.Join(d, "locks", "ref-"+url.PathEscape(name))
	return withLock(lock, func() error {
		cur, err := readRefFile(p)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
		if cur != old {
			return fmt.Errorf("ref %q is at %s, expected %s: %w", name, cur.Short(), old.Short(), ErrCASFailed)
		}
		if new.IsZero() {
			if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
		} else if _, err := s.writeAtomic(d, p, []byte(new.String()+"\n"), false); err != nil {
			return err
		}
		rec, _ := json.Marshal(RefUpdate{Name: name, Old: old, New: new, Time: time.Now().UnixNano(), Who: who})
		if len(rec) > reflogRecordSize-1 {
			rec, _ = json.Marshal(RefUpdate{Name: name, Old: old, New: new, Time: time.Now().UnixNano()})
		}
		padded := make([]byte, reflogRecordSize)
		copy(padded, rec)
		for i := len(rec); i < reflogRecordSize-1; i++ {
			padded[i] = ' '
		}
		padded[reflogRecordSize-1] = '\n'
		return appendRecord(filepath.Join(d, "reflog"), padded)
	})
}

func (s *FS) ReadRefLog(_ context.Context, repo string, after int64, limit int) ([]RefUpdate, error) {
	d, err := s.checkRepo(repo)
	if err != nil {
		return nil, err
	}
	recs, start, err := readRecords(filepath.Join(d, "reflog"), reflogRecordSize, after, limit)
	if err != nil {
		return nil, err
	}
	out := make([]RefUpdate, 0, len(recs))
	for i, r := range recs {
		var u RefUpdate
		if err := json.Unmarshal(bytes.TrimSpace(r), &u); err != nil {
			return nil, fmt.Errorf("corrupt reflog record %d: %v", start+int64(i)+1, err)
		}
		u.Seq = start + int64(i) + 1
		out = append(out, u)
	}
	return out, nil
}

func validDerivedKind(kind string) bool { return ValidKVKey(kind) }

func (s *FS) GetDerived(_ context.Context, repo, kind string, key object.ID) ([]byte, error) {
	d, err := s.checkRepo(repo)
	if err != nil {
		return nil, err
	}
	if !validDerivedKind(kind) {
		return nil, fmt.Errorf("invalid derived kind %q", kind)
	}
	b, err := os.ReadFile(shard(filepath.Join(d, "derived", kind), key))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrNotFound
	}
	return b, err
}

func (s *FS) PutDerived(_ context.Context, repo, kind string, key object.ID, value []byte) error {
	d, err := s.checkRepo(repo)
	if err != nil {
		return err
	}
	if !validDerivedKind(kind) {
		return fmt.Errorf("invalid derived kind %q", kind)
	}
	_, err = s.writeAtomic(d, shard(filepath.Join(d, "derived", kind), key), value, false)
	return err
}

func (s *FS) KVGet(_ context.Context, repo, ns, key string) ([]byte, error) {
	d, err := s.checkRepo(repo)
	if err != nil {
		return nil, err
	}
	if !ValidKVKey(ns) || !ValidKVKey(key) {
		return nil, fmt.Errorf("invalid key %q/%q", ns, key)
	}
	b, err := os.ReadFile(filepath.Join(d, "kv", ns, key))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrNotFound
	}
	return b, err
}

func (s *FS) KVUpdate(_ context.Context, repo, ns, key string, fn func([]byte) ([]byte, error)) error {
	d, err := s.checkRepo(repo)
	if err != nil {
		return err
	}
	if !ValidKVKey(ns) || !ValidKVKey(key) {
		return fmt.Errorf("invalid key %q/%q", ns, key)
	}
	p := filepath.Join(d, "kv", ns, key)
	return withLock(filepath.Join(d, "locks", "kv-"+ns+"-"+key), func() error {
		old, err := os.ReadFile(p)
		if errors.Is(err, fs.ErrNotExist) {
			old, err = nil, nil
		}
		if err != nil {
			return err
		}
		nv, err := fn(old)
		if err != nil {
			return err
		}
		if nv == nil {
			if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
			return nil
		}
		_, err = s.writeAtomic(d, p, nv, false)
		return err
	})
}

func (s *FS) KVList(_ context.Context, repo, ns, prefix string) ([]string, error) {
	d, err := s.checkRepo(repo)
	if err != nil {
		return nil, err
	}
	if !ValidKVKey(ns) {
		return nil, fmt.Errorf("invalid namespace %q", ns)
	}
	ents, err := os.ReadDir(filepath.Join(d, "kv", ns))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range ents {
		if !e.IsDir() && strings.HasPrefix(e.Name(), prefix) {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out, nil
}
