package repo

import (
	"container/heap"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/mattparavel/a-git/internal/object"
)

// History nodes
//
// The "history" derived kind maps each changeset to the root of a Merkle
// structure of object.HistNode values that mirrors its file tree. A path's
// node is reused unchanged from a parent whenever the path has identical
// content there (preferring the first parent), and a new node is written
// only for paths the changeset actually changed. Each new node links back to
// the parent versions it replaced, so the history of any file or directory
// is a short walk over its own versions rather than a scan of the log. This
// mirrors git's default history simplification: a merge only appears in a
// path's history if the merged result differs from every parent.

func deriveHistory(ctx context.Context, r *Repo, id object.ID, cs *object.Changeset) ([]byte, error) {
	root, err := r.RootTree(ctx, id)
	if err != nil {
		return nil, err
	}
	parents := make([]object.ID, len(cs.Parents))
	for i, p := range cs.Parents {
		if parents[i], err = r.HistRoot(ctx, p); err != nil {
			return nil, err
		}
	}
	h, err := r.buildHist(ctx, id, object.ModeDir, root, parents)
	if err != nil {
		return nil, err
	}
	return h[:], nil
}

// HistRoot returns the root history node of a changeset.
func (r *Repo) HistRoot(ctx context.Context, cs object.ID) (object.ID, error) {
	v, err := r.Derived(ctx, DerivedHistory, cs)
	if err != nil {
		return object.Zero, err
	}
	var id object.ID
	copy(id[:], v)
	return id, nil
}

// buildHist returns the history node for a path whose new version is
// (mode, content), given the path's history nodes in each parent (Zero where
// the path is absent).
func (r *Repo) buildHist(ctx context.Context, link object.ID, mode object.Mode, content object.ID, parentNodes []object.ID) (object.ID, error) {
	var ids []object.ID
	var nodes []*object.HistNode
	for _, p := range parentNodes {
		if p.IsZero() || containsID(ids, p) {
			continue
		}
		n, err := r.ReadHist(ctx, p)
		if err != nil {
			return object.Zero, err
		}
		if n.Mode.IsDir() != mode.IsDir() {
			continue // a file replaced by a directory starts a new history
		}
		if n.Mode == mode && n.Content == content {
			return p, nil
		}
		ids = append(ids, p)
		nodes = append(nodes, n)
	}
	node := &object.HistNode{Mode: mode, Content: content, Link: link, Parents: ids}
	if mode.IsDir() {
		t, err := r.ReadTree(ctx, content)
		if err != nil {
			return object.Zero, err
		}
		oldTrees := make([]*object.Tree, len(nodes))
		for i, n := range nodes {
			if oldTrees[i], err = r.ReadTree(ctx, n.Content); err != nil {
				return object.Zero, err
			}
		}
		node.Children = make([]object.HistEntry, 0, len(t.Entries))
		for _, e := range t.Entries {
			childParents := make([]object.ID, 0, len(nodes))
			var reuse object.ID
			for i, n := range nodes {
				c, ok := n.Find(e.Name)
				if !ok {
					continue
				}
				// Cheap reuse check using the parent's tree entry, which
				// avoids reading the child history node.
				if old, ok := oldTrees[i].Find(e.Name); ok && old.Mode == e.Mode && old.ID == e.ID {
					reuse = c
					break
				}
				childParents = append(childParents, c)
			}
			child := reuse
			if child.IsZero() {
				if child, err = r.buildHist(ctx, link, e.Mode, e.ID, childParents); err != nil {
					return object.Zero, err
				}
			}
			node.Children = append(node.Children, object.HistEntry{Name: e.Name, ID: child})
		}
	}
	return r.WriteHist(ctx, node)
}

func containsID(ids []object.ID, id object.ID) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}

// HistNodeAt returns the history node of path in changeset cs.
func (r *Repo) HistNodeAt(ctx context.Context, cs object.ID, path string) (object.ID, error) {
	cur, err := r.HistRoot(ctx, cs)
	if err != nil {
		return object.Zero, err
	}
	if path == "" {
		return cur, nil
	}
	for _, comp := range strings.Split(path, "/") {
		n, err := r.ReadHist(ctx, cur)
		if err != nil {
			return object.Zero, err
		}
		c, ok := n.Find(comp)
		if !ok {
			return object.Zero, fmt.Errorf("path %q: %w", path, ErrNotFound)
		}
		cur = c
	}
	return cur, nil
}

// PathChange is one entry of a path's history.
type PathChange struct {
	Changeset object.ID   `json:"changeset"`
	Path      string      `json:"path"`
	Mode      object.Mode `json:"mode"`
	Content   object.ID   `json:"content"`
	Node      object.ID   `json:"node"`
}

type histItem struct {
	node object.ID
	path string
	gen  uint64
	seq  int
}

type histHeap []histItem

func (h histHeap) Len() int { return len(h) }
func (h histHeap) Less(i, j int) bool {
	if h[i].gen != h[j].gen {
		return h[i].gen > h[j].gen
	}
	return h[i].seq < h[j].seq
}
func (h histHeap) Swap(i, j int)       { h[i], h[j] = h[j], h[i] }
func (h *histHeap) Push(x interface{}) { *h = append(*h, x.(histItem)) }
func (h *histHeap) Pop() interface{} {
	old := *h
	it := old[len(old)-1]
	*h = old[:len(old)-1]
	return it
}

// PathHistory lists the changesets that modified path (a file or a
// directory), newest first, as seen from changeset cs. With follow set, the
// history of a file continues across detected renames.
func (r *Repo) PathHistory(ctx context.Context, cs object.ID, path string, limit int, follow bool) ([]PathChange, error) {
	start, err := r.HistNodeAt(ctx, cs, path)
	if err != nil {
		return nil, err
	}
	h := &histHeap{}
	seen := map[object.ID]bool{}
	seq := 0
	push := func(node object.ID, p string) error {
		if seen[node] {
			return nil
		}
		seen[node] = true
		n, err := r.ReadHist(ctx, node)
		if err != nil {
			return err
		}
		g, err := r.Graph(ctx, n.Link)
		if err != nil {
			return err
		}
		seq++
		heap.Push(h, histItem{node: node, path: p, gen: g.Generation, seq: seq})
		return nil
	}
	if err := push(start, path); err != nil {
		return nil, err
	}
	var out []PathChange
	for h.Len() > 0 && (limit <= 0 || len(out) < limit) {
		it := heap.Pop(h).(histItem)
		n, err := r.ReadHist(ctx, it.node)
		if err != nil {
			return nil, err
		}
		out = append(out, PathChange{Changeset: n.Link, Path: it.path, Mode: n.Mode, Content: n.Content, Node: it.node})
		for _, p := range n.Parents {
			if err := push(p, it.path); err != nil {
				return nil, err
			}
		}
		if follow && len(n.Parents) == 0 && !n.Mode.IsDir() {
			from, prev, err := r.renameSource(ctx, n.Link, it.path)
			if err != nil {
				return nil, err
			}
			if !prev.IsZero() {
				if err := push(prev, from); err != nil {
					return nil, err
				}
			}
		}
	}
	return out, nil
}

// renameSource returns the source path and its history node in the first
// parent if path was created by a rename or copy in changeset link.
func (r *Repo) renameSource(ctx context.Context, link object.ID, path string) (string, object.ID, error) {
	copies, err := r.Copies(ctx, link)
	if err != nil {
		return "", object.Zero, err
	}
	for _, c := range copies {
		if c.To != path {
			continue
		}
		cs, err := r.GetChangeset(ctx, link)
		if err != nil {
			return "", object.Zero, err
		}
		if len(cs.Parents) == 0 {
			return "", object.Zero, nil
		}
		node, err := r.HistNodeAt(ctx, cs.Parents[0], c.From)
		if err != nil {
			return "", object.Zero, nil
		}
		return c.From, node, nil
	}
	return "", object.Zero, nil
}

// ---- copy and rename detection ----

// Copy records that a file added by a changeset came from another path in
// its first parent.
type Copy struct {
	From       string `json:"from"`
	To         string `json:"to"`
	Similarity int    `json:"similarity"` // percent of lines in common
	Rename     bool   `json:"rename"`     // the source was deleted by the same changeset
}

const (
	maxRenameCandidates = 100
	minSimilarity       = 50
)

func deriveCopies(ctx context.Context, r *Repo, id object.ID, cs *object.Changeset) ([]byte, error) {
	base, err := r.RootTree(ctx, cs.FirstParent())
	if err != nil {
		return nil, err
	}
	type file struct {
		path string
		blob object.ID
	}
	var added, deleted []file
	for _, ch := range cs.Changes {
		old, err := r.Lookup(ctx, base, ch.Path)
		exists := err == nil && !old.Mode.IsDir()
		switch {
		case ch.IsDelete() && exists:
			deleted = append(deleted, file{ch.Path, old.ID})
		case !ch.IsDelete() && !exists:
			added = append(added, file{ch.Path, ch.Blob})
		}
	}
	var out []Copy
	if len(added) > 0 && len(deleted) > 0 {
		used := map[int]bool{}
		var pending []file
		// Exact renames first.
		byBlob := map[object.ID][]int{}
		for i, d := range deleted {
			byBlob[d.blob] = append(byBlob[d.blob], i)
		}
		for _, a := range added {
			matched := false
			for _, i := range byBlob[a.blob] {
				if !used[i] {
					used[i] = true
					out = append(out, Copy{From: deleted[i].path, To: a.path, Similarity: 100, Rename: true})
					matched = true
					break
				}
			}
			if !matched {
				pending = append(pending, a)
			}
		}
		// Then similar-content renames, if the candidate set is small.
		if len(pending) > 0 && len(pending)*len(deleted) <= maxRenameCandidates*maxRenameCandidates {
			lineSets := map[object.ID]map[string]int{}
			lines := func(b object.ID) (map[string]int, error) {
				if s, ok := lineSets[b]; ok {
					return s, nil
				}
				data, err := r.ReadBlob(ctx, b)
				if err != nil {
					return nil, err
				}
				s := map[string]int{}
				for _, l := range strings.SplitAfter(string(data), "\n") {
					if l != "" {
						s[l]++
					}
				}
				lineSets[b] = s
				return s, nil
			}
			for _, a := range pending {
				la, err := lines(a.blob)
				if err != nil {
					return nil, err
				}
				best, bestScore := -1, 0
				for i, d := range deleted {
					if used[i] {
						continue
					}
					ld, err := lines(d.blob)
					if err != nil {
						return nil, err
					}
					if s := similarity(la, ld); s > bestScore {
						best, bestScore = i, s
					}
				}
				if best >= 0 && bestScore >= minSimilarity {
					used[best] = true
					out = append(out, Copy{From: deleted[best].path, To: a.path, Similarity: bestScore, Rename: true})
				}
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].To < out[j].To })
	if out == nil {
		out = []Copy{}
	}
	return json.Marshal(out)
}

func similarity(a, b map[string]int) int {
	na, nb, common := 0, 0, 0
	for l, c := range a {
		na += c
		if d := b[l]; d > 0 {
			common += min(c, d)
		}
	}
	for _, c := range b {
		nb += c
	}
	if na+nb == 0 {
		return 100
	}
	return 200 * common / (na + nb)
}

// Copies returns the renames detected in a changeset.
func (r *Repo) Copies(ctx context.Context, cs object.ID) ([]Copy, error) {
	v, err := r.Derived(ctx, DerivedCopies, cs)
	if err != nil {
		return nil, err
	}
	var out []Copy
	err = json.Unmarshal(v, &out)
	return out, err
}
