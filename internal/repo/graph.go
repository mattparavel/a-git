package repo

import (
	"container/heap"
	"context"
	"encoding/json"
	"fmt"

	"github.com/mattparavel/a-git/internal/object"
)

// GraphInfo is the commit-graph index derived for every changeset.
type GraphInfo struct {
	// Generation is 1 for root changesets and 1 + max(parent generations)
	// otherwise. An ancestor always has a smaller generation than its
	// descendants, which lets graph walks prune aggressively.
	Generation uint64 `json:"gen"`
	// Depth is the number of first-parent steps to a root.
	Depth uint64 `json:"depth"`
	// Skip[k] is the first-parent ancestor 2^k steps back. It makes "~N"
	// lookups and first-parent ancestry checks O(log N).
	Skip []object.ID `json:"skip,omitempty"`
}

func deriveGraph(ctx context.Context, r *Repo, id object.ID, cs *object.Changeset) ([]byte, error) {
	g := GraphInfo{Generation: 1}
	for i, p := range cs.Parents {
		pg, err := r.Graph(ctx, p)
		if err != nil {
			return nil, err
		}
		if pg.Generation+1 > g.Generation {
			g.Generation = pg.Generation + 1
		}
		if i == 0 {
			g.Depth = pg.Depth + 1
			g.Skip = append(g.Skip, p)
			for k := 0; k < len(pg.Skip); k++ {
				// skip[k+1] = skip[k] of skip[k]
				prev := g.Skip[k]
				pp, err := r.Graph(ctx, prev)
				if err != nil {
					return nil, err
				}
				if k >= len(pp.Skip) {
					break
				}
				g.Skip = append(g.Skip, pp.Skip[k])
			}
		}
	}
	return json.Marshal(g)
}

// Graph returns the graph index of a changeset.
func (r *Repo) Graph(ctx context.Context, id object.ID) (*GraphInfo, error) {
	v, err := r.Derived(ctx, DerivedGraph, id)
	if err != nil {
		return nil, err
	}
	var g GraphInfo
	if err := json.Unmarshal(v, &g); err != nil {
		return nil, err
	}
	return &g, nil
}

// FirstParentAncestor returns the changeset n first-parent steps before id.
func (r *Repo) FirstParentAncestor(ctx context.Context, id object.ID, n uint64) (object.ID, error) {
	cur := id
	for k := 0; n > 0; k++ {
		if n&1 == 1 {
			g, err := r.Graph(ctx, cur)
			if err != nil {
				return object.Zero, err
			}
			if k >= len(g.Skip) {
				return object.Zero, fmt.Errorf("%s has fewer than the requested ancestors: %w", id.Short(), ErrNotFound)
			}
			cur = g.Skip[k]
		}
		n >>= 1
	}
	return cur, nil
}

// IsAncestor reports whether a is an ancestor of (or equal to) b.
func (r *Repo) IsAncestor(ctx context.Context, a, b object.ID) (bool, error) {
	if a == b {
		return true, nil
	}
	ga, err := r.Graph(ctx, a)
	if err != nil {
		return false, err
	}
	gb, err := r.Graph(ctx, b)
	if err != nil {
		return false, err
	}
	if ga.Generation >= gb.Generation {
		return false, nil
	}
	// Fast path: a on b's first-parent chain.
	if gb.Depth >= ga.Depth {
		if anc, err := r.FirstParentAncestor(ctx, b, gb.Depth-ga.Depth); err == nil && anc == a {
			return true, nil
		}
	}
	// General case: walk b's ancestry, never descending below a's generation.
	seen := map[object.ID]bool{b: true}
	stack := []object.ID{b}
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		cs, err := r.GetChangeset(ctx, cur)
		if err != nil {
			return false, err
		}
		for _, p := range cs.Parents {
			if p == a {
				return true, nil
			}
			if seen[p] {
				continue
			}
			seen[p] = true
			pg, err := r.Graph(ctx, p)
			if err != nil {
				return false, err
			}
			if pg.Generation > ga.Generation {
				stack = append(stack, p)
			}
		}
	}
	return false, nil
}

type genItem struct {
	id    object.ID
	gen   uint64
	flags uint8
}

type genHeap []genItem

func (h genHeap) Len() int            { return len(h) }
func (h genHeap) Less(i, j int) bool  { return h[i].gen > h[j].gen }
func (h genHeap) Swap(i, j int)       { h[i], h[j] = h[j], h[i] }
func (h *genHeap) Push(x interface{}) { *h = append(*h, x.(genItem)) }
func (h *genHeap) Pop() interface{} {
	old := *h
	it := old[len(old)-1]
	*h = old[:len(old)-1]
	return it
}

// MergeBases returns the best common ancestors of a and b (those not
// reachable from another common ancestor), highest generation first.
func (r *Repo) MergeBases(ctx context.Context, a, b object.ID) ([]object.ID, error) {
	if a == b {
		return []object.ID{a}, nil
	}
	const (
		fromA = 1 << iota
		fromB
		stale
	)
	flags := map[object.ID]uint8{}
	h := &genHeap{}
	nonStale := 0 // queued entries pushed without the stale flag
	push := func(id object.ID, f uint8) error {
		if flags[id]&f == f {
			return nil
		}
		flags[id] |= f
		g, err := r.Graph(ctx, id)
		if err != nil {
			return err
		}
		if f&stale == 0 {
			nonStale++
		}
		heap.Push(h, genItem{id, g.Generation, f})
		return nil
	}
	if err := push(a, fromA); err != nil {
		return nil, err
	}
	if err := push(b, fromB); err != nil {
		return nil, err
	}
	var results []object.ID
	isResult := map[object.ID]bool{}
	// Walk both histories in generation order, "painting" each changeset
	// with the side(s) it is reachable from. A changeset reachable from both
	// is a common ancestor; everything below it is marked stale. The walk
	// ends when only stale changesets remain queued.
	for h.Len() > 0 && nonStale > 0 {
		it := heap.Pop(h).(genItem)
		if it.flags&stale == 0 {
			nonStale--
		}
		f := flags[it.id] & (fromA | fromB | stale)
		if f&(fromA|fromB) == fromA|fromB && f&stale == 0 {
			if !isResult[it.id] {
				isResult[it.id] = true
				results = append(results, it.id)
			}
			f |= stale
			flags[it.id] |= stale
		}
		cs, err := r.GetChangeset(ctx, it.id)
		if err != nil {
			return nil, err
		}
		for _, p := range cs.Parents {
			if err := push(p, f); err != nil {
				return nil, err
			}
		}
	}
	// Remove results that are ancestors of other results.
	var best []object.ID
	for i, x := range results {
		redundant := false
		for j, y := range results {
			if i == j {
				continue
			}
			anc, err := r.IsAncestor(ctx, x, y)
			if err != nil {
				return nil, err
			}
			if anc {
				redundant = true
				break
			}
		}
		if !redundant {
			best = append(best, x)
		}
	}
	return best, nil
}

// MergeBase returns a single best common ancestor of a and b, or Zero if
// they share no history.
func (r *Repo) MergeBase(ctx context.Context, a, b object.ID) (object.ID, error) {
	bases, err := r.MergeBases(ctx, a, b)
	if err != nil || len(bases) == 0 {
		return object.Zero, err
	}
	return bases[0], nil
}
