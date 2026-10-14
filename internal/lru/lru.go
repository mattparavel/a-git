// Package lru provides a small, size-bounded, concurrency-safe LRU cache.
package lru

import (
	"container/list"
	"sync"
)

// Cache is an LRU cache bounded by the total "cost" of its entries.
type Cache[K comparable, V any] struct {
	mu      sync.Mutex
	maxCost int64
	cost    int64
	ll      *list.List
	items   map[K]*list.Element
	costFn  func(V) int64

	hits, misses, evictions int64
}

type entry[K comparable, V any] struct {
	key  K
	val  V
	cost int64
}

// New returns a cache that holds entries until their summed cost exceeds
// maxCost. costFn may be nil, in which case every entry costs 1.
func New[K comparable, V any](maxCost int64, costFn func(V) int64) *Cache[K, V] {
	if costFn == nil {
		costFn = func(V) int64 { return 1 }
	}
	return &Cache[K, V]{maxCost: maxCost, ll: list.New(), items: map[K]*list.Element{}, costFn: costFn}
}

// Get returns the value for key and marks it recently used.
func (c *Cache[K, V]) Get(key K) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		c.ll.MoveToFront(el)
		c.hits++
		return el.Value.(*entry[K, V]).val, true
	}
	c.misses++
	var zero V
	return zero, false
}

// Peek returns the value without affecting recency or statistics.
func (c *Cache[K, V]) Peek(key K) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		return el.Value.(*entry[K, V]).val, true
	}
	var zero V
	return zero, false
}

// Add inserts or replaces key.
func (c *Cache[K, V]) Add(key K, val V) {
	cost := c.costFn(val)
	if cost > c.maxCost {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		e := el.Value.(*entry[K, V])
		c.cost += cost - e.cost
		e.val, e.cost = val, cost
		c.ll.MoveToFront(el)
	} else {
		c.items[key] = c.ll.PushFront(&entry[K, V]{key, val, cost})
		c.cost += cost
	}
	for c.cost > c.maxCost && c.ll.Len() > 0 {
		el := c.ll.Back()
		e := el.Value.(*entry[K, V])
		c.ll.Remove(el)
		delete(c.items, e.key)
		c.cost -= e.cost
		c.evictions++
	}
}

// Remove deletes key if present.
func (c *Cache[K, V]) Remove(key K) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		c.ll.Remove(el)
		delete(c.items, key)
		c.cost -= el.Value.(*entry[K, V]).cost
	}
}

// Stats describes cache usage.
type Stats struct {
	Entries   int     `json:"entries"`
	Cost      int64   `json:"cost"`
	MaxCost   int64   `json:"max_cost"`
	Hits      int64   `json:"hits"`
	Misses    int64   `json:"misses"`
	Evictions int64   `json:"evictions"`
	HitRate   float64 `json:"hit_rate"`
}

// Stats returns a snapshot of usage counters.
func (c *Cache[K, V]) Stats() Stats {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := Stats{Entries: c.ll.Len(), Cost: c.cost, MaxCost: c.maxCost, Hits: c.hits, Misses: c.misses, Evictions: c.evictions}
	if t := c.hits + c.misses; t > 0 {
		s.HitRate = float64(c.hits) / float64(t)
	}
	return s
}
