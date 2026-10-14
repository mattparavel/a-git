package repo

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/mattparavel/a-git/internal/object"
	"github.com/mattparavel/a-git/internal/store"
)

var ctx = context.Background()

func newTestRepo(t *testing.T) *Repo {
	t.Helper()
	g := NewRegistry(store.NewMem())
	r, err := g.Create(ctx, "test")
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// del marks a deletion in commit's file map.
const del = "\x00delete"

var clock int64 = 1_700_000_000

func commit(t *testing.T, r *Repo, parents []object.ID, msg string, files map[string]string) object.ID {
	t.Helper()
	cs := &object.Changeset{Parents: parents, Author: "Test <t@example.com>", AuthorTime: clock, Message: msg}
	clock++
	for p, content := range files {
		if content == del {
			cs.Changes = append(cs.Changes, object.Change{Path: p})
			continue
		}
		id, err := r.PutObject(ctx, object.KindBlob, []byte(content))
		if err != nil {
			t.Fatal(err)
		}
		cs.Changes = append(cs.Changes, object.Change{Path: p, Mode: object.ModeFile, Blob: id})
	}
	id, err := r.CreateChangeset(ctx, cs)
	if err != nil {
		t.Fatalf("commit %q: %v", msg, err)
	}
	return id
}

func readFile(t *testing.T, r *Repo, cs object.ID, p string) string {
	t.Helper()
	b, _, err := r.ReadFileAt(ctx, cs, p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	return string(b)
}

func listFiles(t *testing.T, r *Repo, cs object.ID) string {
	t.Helper()
	root, err := r.RootTree(ctx, cs)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	r.WalkFiles(ctx, root, "", func(p string, e object.TreeEntry) error {
		b, _ := r.ReadBlob(ctx, e.ID)
		out = append(out, p+"="+string(b))
		return nil
	})
	return strings.Join(out, ",")
}

func TestTreeDerivation(t *testing.T) {
	r := newTestRepo(t)
	c1 := commit(t, r, nil, "init", map[string]string{"a.txt": "a", "dir/b.txt": "b", "dir/sub/c.txt": "c"})
	if got := listFiles(t, r, c1); got != "a.txt=a,dir/b.txt=b,dir/sub/c.txt=c" {
		t.Fatalf("c1 files: %s", got)
	}
	c2 := commit(t, r, []object.ID{c1}, "edit", map[string]string{"dir/sub/c.txt": del, "dir/b.txt": "b2", "new/x": "x"})
	if got := listFiles(t, r, c2); got != "a.txt=a,dir/b.txt=b2,new/x=x" {
		t.Fatalf("c2 files: %s", got)
	}
	// Unchanged subtrees are shared between changesets.
	t1, _ := r.RootTree(ctx, c1)
	t2, _ := r.RootTree(ctx, c2)
	e1, _ := r.Lookup(ctx, t1, "a.txt")
	e2, _ := r.Lookup(ctx, t2, "a.txt")
	if e1 != e2 {
		t.Fatal("unchanged file entry differs")
	}
	// File replaced by a directory in one changeset.
	c3 := commit(t, r, []object.ID{c2}, "file to dir", map[string]string{"a.txt": del, "a.txt/inner": "i"})
	if got := listFiles(t, r, c3); got != "a.txt/inner=i,dir/b.txt=b2,new/x=x" {
		t.Fatalf("c3 files: %s", got)
	}
	// Invalid changesets are rejected.
	bad := &object.Changeset{Parents: []object.ID{c3}, Author: "x", Changes: []object.Change{{Path: "nope"}}}
	if _, err := r.CreateChangeset(ctx, bad); err == nil {
		t.Fatal("expected error deleting missing file")
	}
	blob, _ := r.PutObject(ctx, object.KindBlob, []byte("z"))
	bad = &object.Changeset{Parents: []object.ID{c3}, Author: "x", Changes: []object.Change{{Path: "dir", Mode: object.ModeFile, Blob: blob}}}
	if _, err := r.CreateChangeset(ctx, bad); err == nil {
		t.Fatal("expected error replacing directory with file")
	}
	missing := object.Hash(object.KindBlob, []byte("never uploaded"))
	bad = &object.Changeset{Parents: []object.ID{c3}, Author: "x", Changes: []object.Change{{Path: "m", Mode: object.ModeFile, Blob: missing}}}
	if _, err := r.CreateChangeset(ctx, bad); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected missing blob error, got %v", err)
	}
	// Deleting everything yields the empty tree.
	c4 := commit(t, r, []object.ID{c3}, "wipe", map[string]string{"a.txt/inner": del, "dir/b.txt": del, "new/x": del})
	if root, _ := r.RootTree(ctx, c4); root != object.EmptyTreeID {
		t.Fatal("expected empty tree")
	}
}

func TestDiffTrees(t *testing.T) {
	r := newTestRepo(t)
	c1 := commit(t, r, nil, "init", map[string]string{"a": "1", "d/x": "1", "d/y": "1", "e/z": "1"})
	c2 := commit(t, r, []object.ID{c1}, "change", map[string]string{"a": "2", "d/x": del, "f/n": "1"})
	t1, _ := r.RootTree(ctx, c1)
	t2, _ := r.RootTree(ctx, c2)
	diffs, err := r.DiffTrees(ctx, t1, t2)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, d := range diffs {
		got = append(got, d.Status()+":"+d.Path)
	}
	if strings.Join(got, " ") != "modified:a deleted:d/x added:f/n" {
		t.Fatalf("diff: %v", got)
	}
	diffs, _ = r.DiffTrees(ctx, t1, t2, "d")
	if len(diffs) != 1 || diffs[0].Path != "d/x" {
		t.Fatalf("scoped diff: %+v", diffs)
	}
}

// buildDAG creates:
//
//	r - a1 - a2 - m - a3
//	  \           /
//	   b1 ------ b2
func buildDAG(t *testing.T, r *Repo) map[string]object.ID {
	ids := map[string]object.ID{}
	ids["r"] = commit(t, r, nil, "root", map[string]string{"f": "root\n"})
	ids["a1"] = commit(t, r, []object.ID{ids["r"]}, "a1", map[string]string{"a": "1"})
	ids["a2"] = commit(t, r, []object.ID{ids["a1"]}, "a2", map[string]string{"a": "2"})
	ids["b1"] = commit(t, r, []object.ID{ids["r"]}, "b1", map[string]string{"b": "1"})
	ids["b2"] = commit(t, r, []object.ID{ids["b1"]}, "b2", map[string]string{"b": "2"})
	ids["m"] = commit(t, r, []object.ID{ids["a2"], ids["b2"]}, "merge", map[string]string{"b": "2"})
	ids["a3"] = commit(t, r, []object.ID{ids["m"]}, "a3", map[string]string{"a": "3"})
	return ids
}

func TestGraph(t *testing.T) {
	r := newTestRepo(t)
	ids := buildDAG(t, r)
	g, _ := r.Graph(ctx, ids["a3"])
	if g.Generation != 5 || g.Depth != 4 {
		t.Fatalf("a3 graph: %+v", g)
	}
	for n := uint64(0); n <= 4; n++ {
		want := []string{"a3", "m", "a2", "a1", "r"}[n]
		got, err := r.FirstParentAncestor(ctx, ids["a3"], n)
		if err != nil || got != ids[want] {
			t.Fatalf("~%d = %s, want %s (%v)", n, got.Short(), want, err)
		}
	}
	if _, err := r.FirstParentAncestor(ctx, ids["a3"], 5); err == nil {
		t.Fatal("expected error past root")
	}
	cases := []struct {
		a, b string
		want bool
	}{
		{"r", "a3", true}, {"b1", "a3", true}, {"b2", "m", true}, {"a3", "r", false},
		{"b1", "a2", false}, {"a2", "b2", false}, {"m", "m", true},
	}
	for _, c := range cases {
		got, err := r.IsAncestor(ctx, ids[c.a], ids[c.b])
		if err != nil || got != c.want {
			t.Errorf("IsAncestor(%s,%s) = %v, want %v (%v)", c.a, c.b, got, c.want, err)
		}
	}
	mb, _ := r.MergeBase(ctx, ids["a2"], ids["b2"])
	if mb != ids["r"] {
		t.Fatalf("merge base a2,b2 = %s", mb.Short())
	}
	mb, _ = r.MergeBase(ctx, ids["a3"], ids["b2"])
	if mb != ids["b2"] {
		t.Fatalf("merge base a3,b2 = %s", mb.Short())
	}
	// Resolve expressions.
	r.MoveRef(ctx, "main", object.Zero, ids["a3"], "")
	for expr, want := range map[string]string{
		"main": "a3", "main~1": "m", "main^": "m", "main~1^2": "b2", "main~1^2~1": "b1", "main^^^": "a1",
		ids["b1"].String()[:10]: "b1", ids["m"].String(): "m",
	} {
		got, err := r.Resolve(ctx, expr)
		if err != nil || got != ids[want] {
			t.Errorf("Resolve(%q) = %s, %v; want %s", expr, got.Short(), err, want)
		}
	}
	if _, err := r.Resolve(ctx, "nosuchref"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown ref: %v", err)
	}
}

func TestLog(t *testing.T) {
	r := newTestRepo(t)
	ids := buildDAG(t, r)
	name := map[object.ID]string{}
	for k, v := range ids {
		name[v] = k
	}
	names := func(l []object.ID) string {
		var s []string
		for _, id := range l {
			s = append(s, name[id])
		}
		return strings.Join(s, " ")
	}
	all, _ := r.Log(ctx, ids["a3"], LogOptions{})
	if got := names(all); got != "a3 m b2 a2 b1 a1 r" {
		t.Fatalf("log: %s", got)
	}
	fp, _ := r.Log(ctx, ids["a3"], LogOptions{FirstParent: true})
	if got := names(fp); got != "a3 m a2 a1 r" {
		t.Fatalf("first-parent log: %s", got)
	}
	rng, _ := r.Log(ctx, ids["a3"], LogOptions{Exclude: ids["a2"]})
	if got := names(rng); got != "a3 m b2 b1" {
		t.Fatalf("a2..a3: %s", got)
	}
	lim, _ := r.Log(ctx, ids["a3"], LogOptions{Limit: 2})
	if got := names(lim); got != "a3 m" {
		t.Fatalf("limit: %s", got)
	}
	pl, _ := r.Log(ctx, ids["a3"], LogOptions{Path: "a"})
	if got := names(pl); got != "a3 a2 a1" {
		t.Fatalf("path log a: %s", got)
	}
	// The merge took b's content from its second parent, so history of "b"
	// follows that parent and the merge itself is not listed.
	pl, _ = r.Log(ctx, ids["a3"], LogOptions{Path: "b"})
	if got := names(pl); got != "b2 b1" {
		t.Fatalf("path log b: %s", got)
	}
	pl, _ = r.Log(ctx, ids["a3"], LogOptions{Path: ""})
	if len(pl) != 7 {
		t.Fatalf("root path log: %d", len(pl))
	}
	stack, _ := r.Stack(ctx, ids["b2"], ids["a2"])
	if got := names(stack); got != "b1 b2" {
		t.Fatalf("stack: %s", got)
	}
}

func TestHistoryDirectoriesAndRenames(t *testing.T) {
	r := newTestRepo(t)
	c1 := commit(t, r, nil, "c1", map[string]string{"src/a.go": "package a\n\nfunc A() {}\n", "README": "hi"})
	c2 := commit(t, r, []object.ID{c1}, "c2", map[string]string{"README": "hello"})
	c3 := commit(t, r, []object.ID{c2}, "c3", map[string]string{"src/a.go": "package a\n\nfunc A() {}\nfunc B() {}\n"})
	c4 := commit(t, r, []object.ID{c3}, "rename", map[string]string{"src/a.go": del, "pkg/a.go": "package a\n\nfunc A() {}\nfunc B() {}\n"})
	c5 := commit(t, r, []object.ID{c4}, "c5", map[string]string{"pkg/a.go": "package a\n\nfunc A() {}\nfunc B() {}\nfunc C() {}\n"})

	h, _ := r.PathHistory(ctx, c3, "src", 0, false)
	if len(h) != 2 || h[0].Changeset != c3 || h[1].Changeset != c1 {
		t.Fatalf("dir history: %+v", h)
	}
	copies, _ := r.Copies(ctx, c4)
	if len(copies) != 1 || copies[0].From != "src/a.go" || copies[0].To != "pkg/a.go" || copies[0].Similarity != 100 {
		t.Fatalf("copies: %+v", copies)
	}
	h, _ = r.PathHistory(ctx, c5, "pkg/a.go", 0, false)
	if len(h) != 2 {
		t.Fatalf("history without follow: %d entries", len(h))
	}
	h, _ = r.PathHistory(ctx, c5, "pkg/a.go", 0, true)
	var got []object.ID
	for _, e := range h {
		got = append(got, e.Changeset)
	}
	if fmt.Sprint(got) != fmt.Sprint([]object.ID{c5, c4, c3, c1}) {
		t.Fatalf("history with follow: %+v", h)
	}

	blame, err := r.Blame(ctx, c5, "pkg/a.go")
	if err != nil {
		t.Fatal(err)
	}
	perLine := map[int]object.ID{}
	for _, b := range blame {
		for k := 0; k < b.Count; k++ {
			perLine[b.Start+k] = b.Changeset
		}
	}
	want := []object.ID{c1, c1, c1, c3, c5}
	for i, w := range want {
		if perLine[i] != w {
			t.Fatalf("blame line %d = %s, want %s (%+v)", i, perLine[i].Short(), w.Short(), blame)
		}
	}
	// Line 3 was blamed through the rename to its original path.
	for _, b := range blame {
		if b.Changeset == c1 && b.Path != "src/a.go" {
			t.Fatalf("blame path: %+v", b)
		}
	}
}

func TestMerge(t *testing.T) {
	r := newTestRepo(t)
	base := commit(t, r, nil, "base", map[string]string{"f": "1\n2\n3\n4\n5\n", "g": "g\n"})
	ours := commit(t, r, []object.ID{base}, "ours", map[string]string{"f": "one\n2\n3\n4\n5\n", "o": "o"})
	theirs := commit(t, r, []object.ID{base}, "theirs", map[string]string{"f": "1\n2\n3\n4\nfive\n", "t": "t", "g": del})
	res, err := r.Merge(ctx, ours, theirs, MergeOptions{Author: "m"})
	if err != nil {
		t.Fatal(err)
	}
	if got := listFiles(t, r, res.ID); got != "f=one\n2\n3\n4\nfive\n,o=o,t=t" {
		t.Fatalf("merged: %q", got)
	}
	cs, _ := r.GetChangeset(ctx, res.ID)
	if len(cs.Parents) != 2 || cs.Parents[0] != ours {
		t.Fatal("merge parents")
	}
	// Conflict.
	theirs2 := commit(t, r, []object.ID{base}, "theirs2", map[string]string{"f": "uno\n2\n3\n4\n5\n"})
	_, err = r.Merge(ctx, ours, theirs2, MergeOptions{Author: "m"})
	var ce *ConflictError
	if !errors.As(err, &ce) || len(ce.Conflicts) != 1 || ce.Conflicts[0].Path != "f" {
		t.Fatalf("expected conflict on f, got %v", err)
	}
	res, err = r.Merge(ctx, ours, theirs2, MergeOptions{Author: "m", AllowConflicts: true})
	if err != nil || !strings.Contains(readFile(t, r, res.ID, "f"), "<<<<<<<") {
		t.Fatalf("conflict markers missing: %v", err)
	}
	// Fast-forward and already-merged cases.
	if res, _ := r.Merge(ctx, base, ours, MergeOptions{Author: "m"}); !res.FastForward || res.ID != ours {
		t.Fatalf("fast-forward: %+v", res)
	}
	if res, _ := r.Merge(ctx, ours, base, MergeOptions{Author: "m"}); res.ID != ours {
		t.Fatalf("already merged: %+v", res)
	}
}

func TestRebaseLandRevert(t *testing.T) {
	r := newTestRepo(t)
	c0 := commit(t, r, nil, "init", map[string]string{"f": "1\n2\n3\n4\n5\n6\n7\n8\n", "g": "g\n"})
	if err := r.MoveRef(ctx, "main", object.Zero, c0, "t"); err != nil {
		t.Fatal(err)
	}
	// Two developers stack changes on c0.
	a1 := commit(t, r, []object.ID{c0}, "a1", map[string]string{"f": "A\n2\n3\n4\n5\n6\n7\n8\n"})
	a2 := commit(t, r, []object.ID{a1}, "a2", map[string]string{"a": "new"})
	b1 := commit(t, r, []object.ID{c0}, "b1", map[string]string{"f": "1\n2\n3\n4\n5\n6\n7\nB\n"})

	res, err := r.Land(ctx, "main", a2, "alice", nil)
	if err != nil || res.New != a2 || res.Rebase != nil {
		t.Fatalf("fast-forward land: %+v %v", res, err)
	}
	res, err = r.Land(ctx, "main", b1, "bob", nil)
	if err != nil || res.Rebase == nil {
		t.Fatalf("rebasing land: %+v %v", res, err)
	}
	main, _ := r.Ref(ctx, "main")
	if got := readFile(t, r, main, "f"); got != "A\n2\n3\n4\n5\n6\n7\nB\n" {
		t.Fatalf("landed content: %q", got)
	}
	cs, _ := r.GetChangeset(ctx, main)
	if cs.Extra[object.ExtraRebasedFrom] != b1.String() || cs.Parents[0] != a2 {
		t.Fatalf("rebased changeset: %+v", cs)
	}
	if _, err := r.Land(ctx, "main", b1, "bob", nil); err == nil {
		// b1's change is already present, so the rebase is empty.
		t.Fatal("expected nothing to land")
	}

	// Conflicting land.
	c1 := commit(t, r, []object.ID{c0}, "conflict", map[string]string{"f": "X\n2\n3\n4\n5\n6\n7\n8\n"})
	_, err = r.Land(ctx, "main", c1, "carol", nil)
	var ce *ConflictError
	if !errors.As(err, &ce) {
		t.Fatalf("expected conflict, got %v", err)
	}

	// Revert a1 on top of main.
	rv, err := r.Revert(ctx, a1, main, "dave", "")
	if err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, r, rv, "f"); got != "1\n2\n3\n4\n5\n6\n7\nB\n" {
		t.Fatalf("reverted content: %q", got)
	}
}

func TestConcurrentLands(t *testing.T) {
	r := newTestRepo(t)
	c0 := commit(t, r, nil, "init", map[string]string{"seed": "s"})
	r.MoveRef(ctx, "main", object.Zero, c0, "")
	const n = 16
	heads := make([]object.ID, n)
	for i := range heads {
		heads[i] = commit(t, r, []object.ID{c0}, fmt.Sprint("change ", i), map[string]string{fmt.Sprintf("file%02d", i): fmt.Sprint(i)})
	}
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := range heads {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = r.Land(ctx, "main", heads[i], "agent", nil)
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("land %d: %v", i, err)
		}
	}
	main, _ := r.Ref(ctx, "main")
	files := listFiles(t, r, main)
	for i := 0; i < n; i++ {
		if !strings.Contains(files, fmt.Sprintf("file%02d=%d", i, i)) {
			t.Fatalf("missing file%02d in %s", i, files)
		}
	}
	l, _ := r.Log(ctx, main, LogOptions{})
	if len(l) != n+1 {
		t.Fatalf("expected linear history of %d, got %d", n+1, len(l))
	}
}

func TestSearchPaths(t *testing.T) {
	r := newTestRepo(t)
	c := commit(t, r, nil, "c", map[string]string{"src/main.go": "", "src/util/str.go": "", "docs/README.md": "", "Makefile": ""})
	res, _ := r.SearchPaths(ctx, c, "", "**/*.go", 0)
	if len(res) != 2 {
		t.Fatalf("glob: %+v", res)
	}
	res, _ = r.SearchPaths(ctx, c, "", "readme", 0)
	if len(res) != 1 || res[0].Path != "docs/README.md" {
		t.Fatalf("substring: %+v", res)
	}
	res, _ = r.SearchPaths(ctx, c, "src/util", "*.go", 0)
	if len(res) != 1 {
		t.Fatalf("prefix: %+v", res)
	}
}

func TestLazyDerivationOverLongHistory(t *testing.T) {
	r := newTestRepo(t)
	prev := commit(t, r, nil, "c0", map[string]string{"f": "0"})
	for i := 1; i < 300; i++ {
		prev = commit(t, r, []object.ID{prev}, fmt.Sprint("c", i), map[string]string{"f": fmt.Sprint(i), fmt.Sprintf("d%d/x", i%7): fmt.Sprint(i)})
	}
	// History has never been derived; asking for it at the tip derives the
	// whole chain iteratively.
	h, err := r.PathHistory(ctx, prev, "f", 0, false)
	if err != nil || len(h) != 300 {
		t.Fatalf("history: %d %v", len(h), err)
	}
	anc, err := r.FirstParentAncestor(ctx, prev, 299)
	if err != nil {
		t.Fatal(err)
	}
	if cs, _ := r.GetChangeset(ctx, anc); cs.Message != "c0" {
		t.Fatalf("~299 = %q", cs.Message)
	}
}
