// Package tier implements a-git's caching tiers: the regional cache
// (agit-cache) and the per-node daemon (agitd). Both serve the full a-git
// protocol and differ only in where they keep objects (memory or disk) and
// how clients reach them (TCP or a Unix socket).
package tier

import (
	"container/list"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/mattparavel/a-git/internal/lru"
	"github.com/mattparavel/a-git/internal/object"
)

// ObjectCache stores objects in stored form, keyed by ID. Objects are
// immutable, so a cached copy never needs invalidation.
type ObjectCache interface {
	Get(id object.ID) ([]byte, bool)
	Put(id object.ID, stored []byte)
	Has(id object.ID) bool
	Stats() CacheStats
}

// CacheStats describes an object cache.
type CacheStats struct {
	Kind      string  `json:"kind"`
	Objects   int     `json:"objects"`
	Bytes     int64   `json:"bytes"`
	MaxBytes  int64   `json:"max_bytes"`
	Hits      int64   `json:"hits"`
	Misses    int64   `json:"misses"`
	Evictions int64   `json:"evictions"`
	HitRate   float64 `json:"hit_rate"`
}

// MemCache is an in-memory LRU object cache.
type MemCache struct {
	c *lru.Cache[object.ID, []byte]
}

// NewMemCache returns a memory cache bounded to maxBytes.
func NewMemCache(maxBytes int64) *MemCache {
	return &MemCache{c: lru.New[object.ID, []byte](maxBytes, func(b []byte) int64 { return int64(len(b)) + 64 })}
}

func (m *MemCache) Get(id object.ID) ([]byte, bool) { return m.c.Get(id) }
func (m *MemCache) Put(id object.ID, b []byte)      { m.c.Add(id, b) }
func (m *MemCache) Has(id object.ID) bool {
	_, ok := m.c.Peek(id)
	return ok
}

func (m *MemCache) Stats() CacheStats {
	s := m.c.Stats()
	return CacheStats{Kind: "memory", Objects: s.Entries, Bytes: s.Cost, MaxBytes: s.MaxCost, Hits: s.Hits, Misses: s.Misses, Evictions: s.Evictions, HitRate: s.HitRate}
}

// DiskCache keeps objects as files under a directory, shared by every
// working copy on a machine. Its size is bounded: the least recently used
// objects are deleted when the total exceeds the budget. Objects are written
// in the same stored form they arrive in, so nothing is re-encoded.
type DiskCache struct {
	dir      string
	maxBytes int64

	mu    sync.Mutex
	ll    *list.List // front = most recently used
	index map[object.ID]*list.Element
	bytes int64

	hits, misses, evictions atomic.Int64
}

type diskEntry struct {
	id   object.ID
	size int64
}

// OpenDiskCache opens (creating) a disk cache and indexes existing objects.
func OpenDiskCache(dir string, maxBytes int64) (*DiskCache, error) {
	if err := os.MkdirAll(filepath.Join(dir, "tmp"), 0o755); err != nil {
		return nil, err
	}
	d := &DiskCache{dir: dir, maxBytes: maxBytes, ll: list.New(), index: map[object.ID]*list.Element{}}
	err := filepath.WalkDir(dir, func(p string, e fs.DirEntry, err error) error {
		if err != nil || e.IsDir() {
			if e != nil && e.IsDir() && e.Name() == "tmp" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(dir, p)
		hex := filepath.ToSlash(rel)
		if len(hex) != 65 || hex[2] != '/' {
			return nil
		}
		id, perr := object.ParseID(hex[:2] + hex[3:])
		if perr != nil {
			return nil
		}
		info, ierr := e.Info()
		if ierr != nil {
			return nil
		}
		d.index[id] = d.ll.PushBack(&diskEntry{id, info.Size()})
		d.bytes += info.Size()
		return nil
	})
	if err != nil {
		return nil, err
	}
	d.evict()
	return d, nil
}

func (d *DiskCache) path(id object.ID) string {
	h := id.String()
	return filepath.Join(d.dir, h[:2], h[2:])
}

func (d *DiskCache) Get(id object.ID) ([]byte, bool) {
	d.mu.Lock()
	el, ok := d.index[id]
	if ok {
		d.ll.MoveToFront(el)
	}
	d.mu.Unlock()
	if !ok {
		d.misses.Add(1)
		return nil, false
	}
	b, err := os.ReadFile(d.path(id))
	if err != nil {
		d.mu.Lock()
		if el, ok := d.index[id]; ok {
			d.ll.Remove(el)
			delete(d.index, id)
			d.bytes -= el.Value.(*diskEntry).size
		}
		d.mu.Unlock()
		d.misses.Add(1)
		return nil, false
	}
	d.hits.Add(1)
	return b, true
}

func (d *DiskCache) Has(id object.ID) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, ok := d.index[id]
	return ok
}

func (d *DiskCache) Put(id object.ID, stored []byte) {
	if d.Has(id) {
		return
	}
	p := d.path(id)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return
	}
	f, err := os.CreateTemp(filepath.Join(d.dir, "tmp"), "obj-")
	if err != nil {
		return
	}
	_, werr := f.Write(stored)
	cerr := f.Close()
	if werr != nil || cerr != nil {
		os.Remove(f.Name())
		return
	}
	if err := os.Rename(f.Name(), p); err != nil {
		os.Remove(f.Name())
		return
	}
	d.mu.Lock()
	if _, ok := d.index[id]; !ok {
		d.index[id] = d.ll.PushFront(&diskEntry{id, int64(len(stored))})
		d.bytes += int64(len(stored))
	}
	d.mu.Unlock()
	d.evict()
}

func (d *DiskCache) evict() {
	if d.maxBytes <= 0 {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for d.bytes > d.maxBytes && d.ll.Len() > 0 {
		el := d.ll.Back()
		e := el.Value.(*diskEntry)
		if err := os.Remove(d.path(e.id)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return
		}
		d.ll.Remove(el)
		delete(d.index, e.id)
		d.bytes -= e.size
		d.evictions.Add(1)
	}
}

func (d *DiskCache) Stats() CacheStats {
	d.mu.Lock()
	n, b := d.ll.Len(), d.bytes
	d.mu.Unlock()
	s := CacheStats{Kind: "disk", Objects: n, Bytes: b, MaxBytes: d.maxBytes, Hits: d.hits.Load(), Misses: d.misses.Load(), Evictions: d.evictions.Load()}
	if t := s.Hits + s.Misses; t > 0 {
		s.HitRate = float64(s.Hits) / float64(t)
	}
	return s
}
