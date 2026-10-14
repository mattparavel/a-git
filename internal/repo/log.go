package repo

import (
	"container/heap"
	"context"
	"path"
	"strings"

	"github.com/mattparavel/a-git/internal/object"
)

// LogOptions controls Log.
type LogOptions struct {
	Limit       int       // maximum entries (0 = 100)
	FirstParent bool      // follow only first parents
	Path        string    // only changesets that modified this path
	Follow      bool      // with Path: continue across renames
	Exclude     object.ID // omit ancestors of this changeset (the "a" in "a..b")
}

type logItem struct {
	id    object.ID
	gen   uint64
	time  int64
	flags uint8
}

type logHeap []logItem

func (h logHeap) Len() int { return len(h) }
func (h logHeap) Less(i, j int) bool {
	if h[i].gen != h[j].gen {
		return h[i].gen > h[j].gen
	}
	return h[i].time > h[j].time
}
func (h logHeap) Swap(i, j int)       { h[i], h[j] = h[j], h[i] }
func (h *logHeap) Push(x interface{}) { *h = append(*h, x.(logItem)) }
func (h *logHeap) Pop() interface{} {
	old := *h
	it := old[len(old)-1]
	*h = old[:len(old)-1]
	return it
}

// Log lists changesets reachable from head, newest (highest generation)
// first.
func (r *Repo) Log(ctx context.Context, head object.ID, opts LogOptions) ([]object.ID, error) {
	if opts.Limit <= 0 {
		opts.Limit = 100
	}
	if opts.Path != "" {
		hist, err := r.PathHistory(ctx, head, opts.Path, 0, opts.Follow)
		if err != nil {
			return nil, err
		}
		var out []object.ID
		for _, h := range hist {
			if !opts.Exclude.IsZero() {
				if anc, err := r.IsAncestor(ctx, h.Changeset, opts.Exclude); err != nil {
					return nil, err
				} else if anc {
					continue
				}
			}
			out = append(out, h.Changeset)
			if len(out) >= opts.Limit {
				break
			}
		}
		return out, nil
	}

	const (
		interesting = 1 << iota
		uninteresting
	)
	flags := map[object.ID]uint8{}
	h := &logHeap{}
	live := 0 // queued interesting entries
	push := func(id object.ID, f uint8) error {
		if flags[id]&f == f {
			return nil
		}
		flags[id] |= f
		g, err := r.Graph(ctx, id)
		if err != nil {
			return err
		}
		cs, err := r.GetChangeset(ctx, id)
		if err != nil {
			return err
		}
		if f == interesting {
			live++
		}
		heap.Push(h, logItem{id: id, gen: g.Generation, time: cs.AuthorTime, flags: f})
		return nil
	}
	if err := push(head, interesting); err != nil {
		return nil, err
	}
	if !opts.Exclude.IsZero() {
		if err := push(opts.Exclude, uninteresting); err != nil {
			return nil, err
		}
	}
	var out []object.ID
	emitted := map[object.ID]bool{}
	// Generation order guarantees that by the time a changeset is popped,
	// every descendant that could mark it uninteresting has been processed.
	for h.Len() > 0 && live > 0 && len(out) < opts.Limit {
		it := heap.Pop(h).(logItem)
		if it.flags == interesting {
			live--
		}
		f := flags[it.id]
		if f&uninteresting == 0 && !emitted[it.id] {
			emitted[it.id] = true
			out = append(out, it.id)
		}
		cs, err := r.GetChangeset(ctx, it.id)
		if err != nil {
			return nil, err
		}
		parents := cs.Parents
		if opts.FirstParent && len(parents) > 1 {
			parents = parents[:1]
		}
		pf := uint8(interesting)
		if f&uninteresting != 0 {
			pf = uninteresting
		}
		for _, p := range parents {
			if err := push(p, pf); err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}

// Stack returns the changesets reachable from head but not from base, oldest
// first. It is used to find the local changes to rebase or land.
func (r *Repo) Stack(ctx context.Context, head, base object.ID) ([]object.ID, error) {
	ids, err := r.Log(ctx, head, LogOptions{Exclude: base, Limit: 10000})
	if err != nil {
		return nil, err
	}
	for i, j := 0, len(ids)-1; i < j; i, j = i+1, j-1 {
		ids[i], ids[j] = ids[j], ids[i]
	}
	return ids, nil
}

// SearchResult is one match of a path search.
type SearchResult struct {
	Path string      `json:"path"`
	Mode object.Mode `json:"mode"`
	ID   object.ID   `json:"id"`
}

// SearchPaths finds files under prefix whose path matches pattern. A
// pattern containing glob metacharacters is matched against the whole path
// (with "**" matching any number of directories) and against the base name;
// otherwise it is a case-insensitive substring match.
func (r *Repo) SearchPaths(ctx context.Context, cs object.ID, prefix, pattern string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 1000
	}
	root, err := r.RootTree(ctx, cs)
	if err != nil {
		return nil, err
	}
	glob := strings.ContainsAny(pattern, "*?[")
	lower := strings.ToLower(pattern)
	var out []SearchResult
	errStop := context.Canceled
	err = r.WalkFiles(ctx, root, prefix, func(p string, e object.TreeEntry) error {
		ok := false
		if glob {
			ok = MatchGlob(pattern, p) || MatchGlob(pattern, path.Base(p))
		} else {
			ok = strings.Contains(strings.ToLower(p), lower)
		}
		if ok {
			out = append(out, SearchResult{Path: p, Mode: e.Mode, ID: e.ID})
			if len(out) >= limit {
				return errStop
			}
		}
		return nil
	})
	if err != nil && err != errStop {
		return nil, err
	}
	return out, nil
}

// MatchGlob matches a slash-separated path against a glob pattern in which
// "**" matches zero or more whole path components.
func MatchGlob(pattern, name string) bool {
	return matchParts(strings.Split(pattern, "/"), strings.Split(name, "/"))
}

func matchParts(pat, parts []string) bool {
	for len(pat) > 0 {
		if pat[0] == "**" {
			for i := 0; i <= len(parts); i++ {
				if matchParts(pat[1:], parts[i:]) {
					return true
				}
			}
			return false
		}
		if len(parts) == 0 {
			return false
		}
		if ok, _ := path.Match(pat[0], parts[0]); !ok {
			return false
		}
		pat, parts = pat[1:], parts[1:]
	}
	return len(parts) == 0
}
