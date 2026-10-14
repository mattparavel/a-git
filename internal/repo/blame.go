package repo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/mattparavel/a-git/internal/object"
	"github.com/mattparavel/a-git/internal/store"
	"github.com/mattparavel/a-git/internal/textdiff"
)

// BlameRange attributes a run of consecutive lines of one file version to
// the changeset that introduced them.
type BlameRange struct {
	Start     int       `json:"start"` // first line (0-based) in this version
	Count     int       `json:"count"`
	Changeset object.ID `json:"changeset"`
	Path      string    `json:"path"`       // path of the file in Changeset
	OrigStart int       `json:"orig_start"` // first line (0-based) in Changeset's version
}

type blameLine struct {
	cs   object.ID
	path string
	line int
}

// Blame is derived per file history node rather than per changeset: a
// file version's blame is computed from the blame of its parent versions by
// diffing, so every version is only ever attributed once, and unrelated
// changesets are never visited.

// Blame returns line attribution for path in changeset cs.
func (r *Repo) Blame(ctx context.Context, cs object.ID, path string) ([]BlameRange, error) {
	node, err := r.HistNodeAt(ctx, cs, path)
	if err != nil {
		return nil, err
	}
	n, err := r.ReadHist(ctx, node)
	if err != nil {
		return nil, err
	}
	if n.Mode.IsDir() {
		return nil, fmt.Errorf("cannot blame directory %q", path)
	}
	return r.blameNode(ctx, node, path)
}

type blameParent struct {
	node object.ID
	path string
}

// blameParents returns the versions a file version was derived from: its
// history parents, or for a file created by a rename, the source file.
func (r *Repo) blameParents(ctx context.Context, n *object.HistNode, path string) ([]blameParent, error) {
	var out []blameParent
	for _, p := range n.Parents {
		out = append(out, blameParent{p, path})
	}
	if len(out) == 0 {
		from, prev, err := r.renameSource(ctx, n.Link, path)
		if err != nil {
			return nil, err
		}
		if !prev.IsZero() {
			if pn, err := r.ReadHist(ctx, prev); err == nil && !pn.Mode.IsDir() {
				out = append(out, blameParent{prev, from})
			}
		}
	}
	return out, nil
}

func (r *Repo) blameNode(ctx context.Context, node object.ID, path string) ([]BlameRange, error) {
	if v, err := r.Store.GetDerived(ctx, r.Name, DerivedBlame, node); err == nil {
		var out []BlameRange
		return out, json.Unmarshal(v, &out)
	} else if !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}

	// Iterative post-order over parent versions lacking a stored blame, so
	// long file histories do not recurse deeply.
	type frame struct {
		node object.ID
		path string
		post bool
	}
	results := map[object.ID][]BlameRange{}
	visited := map[object.ID]bool{}
	stack := []frame{{node: node, path: path}}
	for len(stack) > 0 {
		f := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		n, err := r.ReadHist(ctx, f.node)
		if err != nil {
			return nil, err
		}
		parents, err := r.blameParents(ctx, n, f.path)
		if err != nil {
			return nil, err
		}
		if f.post {
			b, err := r.computeBlame(ctx, n, f.path, parents, results)
			if err != nil {
				return nil, err
			}
			enc, _ := json.Marshal(b)
			if err := r.Store.PutDerived(ctx, r.Name, DerivedBlame, f.node, enc); err != nil {
				return nil, err
			}
			results[f.node] = b
			continue
		}
		if visited[f.node] {
			continue
		}
		visited[f.node] = true
		if v, err := r.Store.GetDerived(ctx, r.Name, DerivedBlame, f.node); err == nil {
			var b []BlameRange
			if err := json.Unmarshal(v, &b); err != nil {
				return nil, err
			}
			results[f.node] = b
			continue
		}
		stack = append(stack, frame{node: f.node, path: f.path, post: true})
		for _, p := range parents {
			if !visited[p.node] {
				stack = append(stack, frame{node: p.node, path: p.path})
			}
		}
	}
	return results[node], nil
}

func (r *Repo) computeBlame(ctx context.Context, n *object.HistNode, path string, parents []blameParent, results map[object.ID][]BlameRange) ([]BlameRange, error) {
	content, err := r.ReadBlob(ctx, n.Content)
	if err != nil {
		return nil, err
	}
	lines := textdiff.SplitLines(content)
	attr := make([]*blameLine, len(lines))
	for _, p := range parents {
		pn, err := r.ReadHist(ctx, p.node)
		if err != nil {
			return nil, err
		}
		pcontent, err := r.ReadBlob(ctx, pn.Content)
		if err != nil {
			return nil, err
		}
		pblame := expandBlame(results[p.node])
		m := textdiff.LineMap(textdiff.SplitLines(pcontent), lines)
		for j, i := range m {
			if i >= 0 && attr[j] == nil && i < len(pblame) {
				l := pblame[i]
				attr[j] = &l
			}
		}
	}
	for j := range attr {
		if attr[j] == nil {
			attr[j] = &blameLine{cs: n.Link, path: path, line: j}
		}
	}
	// Compress into ranges of lines that are consecutive in both versions.
	var out []BlameRange
	for j, a := range attr {
		if k := len(out) - 1; k >= 0 {
			last := &out[k]
			if last.Changeset == a.cs && last.Path == a.path && last.OrigStart+last.Count == a.line && last.Start+last.Count == j {
				last.Count++
				continue
			}
		}
		out = append(out, BlameRange{Start: j, Count: 1, Changeset: a.cs, Path: a.path, OrigStart: a.line})
	}
	if out == nil {
		out = []BlameRange{}
	}
	return out, nil
}

func expandBlame(ranges []BlameRange) []blameLine {
	var out []blameLine
	for _, rg := range ranges {
		for k := 0; k < rg.Count; k++ {
			out = append(out, blameLine{cs: rg.Changeset, path: rg.Path, line: rg.OrigStart + k})
		}
	}
	return out
}
