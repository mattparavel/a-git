package workcopy

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mattparavel/a-git/internal/api"
	"github.com/mattparavel/a-git/internal/client"
	"github.com/mattparavel/a-git/internal/object"
	"github.com/mattparavel/a-git/internal/store"
)

var ctx = context.Background()

func setup(t *testing.T, files map[string]string) (string, object.ID) {
	t.Helper()
	srv := httptest.NewServer(api.NewServer(api.ServerConfig{Store: store.NewMem()}).Handler())
	t.Cleanup(srv.Close)
	c, _ := client.New(srv.URL, "")
	c.CreateRepo(ctx, "r")
	r := c.Repo("r")
	cs := &object.Changeset{Author: "t <t@t>", Message: "init"}
	var objs []client.StoredObject
	for p, content := range files {
		id, stored := object.Encode(object.KindBlob, []byte(content))
		objs = append(objs, client.StoredObject{ID: id, Stored: stored})
		cs.Changes = append(cs.Changes, object.Change{Path: p, Mode: object.ModeFile, Blob: id})
	}
	r.UploadMissing(ctx, objs)
	res, err := r.CreateChangeset(ctx, cs, &api.RefMove{Name: "main"})
	if err != nil {
		t.Fatal(err)
	}
	return srv.URL, res.ID
}

func clone(t *testing.T, url string, cs object.ID, sparse ...string) *WorkCopy {
	t.Helper()
	dir := t.TempDir()
	wc, err := Init(dir, Config{Repo: "r", Remote: url, Sparse: sparse})
	if err != nil {
		t.Fatal(err)
	}
	if err := wc.Checkout(ctx, cs, nil); err != nil {
		t.Fatal(err)
	}
	return wc
}

func read(t *testing.T, wc *WorkCopy, p string) string {
	t.Helper()
	b, err := os.ReadFile(wc.Abs(p))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func write(t *testing.T, wc *WorkCopy, p, content string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(wc.Abs(p)), 0o755)
	if err := os.WriteFile(wc.Abs(p), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func statusString(t *testing.T, wc *WorkCopy) string {
	t.Helper()
	st, err := wc.Status(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, s := range st {
		out = append(out, s.Status+" "+s.Path)
	}
	return strings.Join(out, ", ")
}

func TestCloneStatusCommit(t *testing.T) {
	url, cs := setup(t, map[string]string{"a.txt": "a\n", "dir/b.txt": "b\n", "other/c.txt": "c\n"})
	wc := clone(t, url, cs)
	if read(t, wc, "dir/b.txt") != "b\n" {
		t.Fatal("content")
	}
	if s := statusString(t, wc); s != "" {
		t.Fatalf("fresh clone status: %s", s)
	}
	write(t, wc, "a.txt", "A\n")
	write(t, wc, "new/n.txt", "n\n")
	os.Remove(wc.Abs("other/c.txt"))
	write(t, wc, ".agitignore", "*.log\nbuild/\n")
	write(t, wc, "debug.log", "noise")
	write(t, wc, "build/out.bin", "noise")
	if s := statusString(t, wc); s != "A .agitignore, M a.txt, A new/n.txt, D other/c.txt" {
		t.Fatalf("status: %s", s)
	}
	id, _, err := wc.Commit(ctx, CommitOptions{Message: "change", Author: "t <t@t>"})
	if err != nil {
		t.Fatal(err)
	}
	if s := statusString(t, wc); s != "" {
		t.Fatalf("status after commit: %s", s)
	}
	if b, _ := wc.Repo.File(ctx, id.String(), "new/n.txt"); string(b) != "n\n" {
		t.Fatal("committed content not on server")
	}
	// Reopen from disk: state persists.
	wc2, err := Open(wc.Root, "")
	if err != nil {
		t.Fatal(err)
	}
	if wc2.State.Base != id || statusString(t, wc2) != "" {
		t.Fatal("reopened state")
	}
	// Executable bit and symlinks round-trip.
	write(t, wc2, "run.sh", "#!/bin/sh\n")
	os.Chmod(wc2.Abs("run.sh"), 0o755)
	os.Symlink("a.txt", wc2.Abs("link"))
	id2, _, err := wc2.Commit(ctx, CommitOptions{Message: "modes", Author: "t <t@t>"})
	if err != nil {
		t.Fatal(err)
	}
	tr, _ := wc2.Repo.Tree(ctx, id2.String(), "")
	modes := map[string]string{}
	for _, e := range tr.Entries {
		modes[e.Name] = e.Mode.String()
	}
	if modes["run.sh"] != "100755" || modes["link"] != "120000" {
		t.Fatalf("modes: %v", modes)
	}
	wc3 := clone(t, url, id2)
	if target, err := os.Readlink(wc3.Abs("link")); err != nil || target != "a.txt" {
		t.Fatalf("symlink: %q %v", target, err)
	}
	if fi, _ := os.Stat(wc3.Abs("run.sh")); fi.Mode()&0o111 == 0 {
		t.Fatal("exec bit lost")
	}
}

func TestStatusDetectsSameSizeEdit(t *testing.T) {
	url, cs := setup(t, map[string]string{"f": "aaaa\n"})
	wc := clone(t, url, cs)
	// Same size, written immediately: stat alone could miss this.
	write(t, wc, "f", "bbbb\n")
	if s := statusString(t, wc); s != "M f" {
		t.Fatalf("status: %q", s)
	}
}

func TestUpdateMergesLocalEdits(t *testing.T) {
	url, cs := setup(t, map[string]string{"f.txt": "1\n2\n3\n4\n5\n6\n", "g.txt": "g\n", "gone.txt": "x\n"})
	alice := clone(t, url, cs)
	bob := clone(t, url, cs)

	write(t, bob, "f.txt", "ONE\n2\n3\n4\n5\n6\n")
	os.Remove(bob.Abs("gone.txt"))
	write(t, bob, "added/new.txt", "new\n")
	bobCS, _, err := bob.Commit(ctx, CommitOptions{Message: "bob", Author: "b <b@b>"})
	if err != nil {
		t.Fatal(err)
	}

	// Alice has an uncommitted edit to a different region of f.txt.
	write(t, alice, "f.txt", "1\n2\n3\n4\n5\nSIX\n")
	res, err := alice.Update(ctx, bobCS, false)
	if err != nil {
		t.Fatal(err)
	}
	if read(t, alice, "f.txt") != "ONE\n2\n3\n4\n5\nSIX\n" || len(res.Merged) != 1 || len(res.Conflicts) != 0 {
		t.Fatalf("merge: %q %+v", read(t, alice, "f.txt"), res)
	}
	if _, err := os.Stat(alice.Abs("gone.txt")); !os.IsNotExist(err) {
		t.Fatal("deleted file still present")
	}
	if read(t, alice, "added/new.txt") != "new\n" {
		t.Fatal("added file missing")
	}
	// The merged local edit is still a local change.
	if s := statusString(t, alice); s != "M f.txt" {
		t.Fatalf("status after merge: %s", s)
	}

	// Conflicting edit gets markers.
	write(t, bob, "g.txt", "bob\n")
	bobCS2, _, _ := bob.Commit(ctx, CommitOptions{Message: "bob2", Author: "b <b@b>"})
	write(t, alice, "g.txt", "alice\n")
	res, err = alice.Update(ctx, bobCS2, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Conflicts) != 1 || !strings.Contains(read(t, alice, "g.txt"), "<<<<<<<") {
		t.Fatalf("conflict: %+v %q", res, read(t, alice, "g.txt"))
	}
}

func TestSparse(t *testing.T) {
	url, cs := setup(t, map[string]string{"a/1": "1", "a/2": "2", "b/1": "1", "c/d/1": "1", "top": "t"})
	wc := clone(t, url, cs, "a")
	if len(wc.Files) != 2 {
		t.Fatalf("sparse clone has %d files", len(wc.Files))
	}
	if _, err := os.Stat(wc.Abs("b")); !os.IsNotExist(err) {
		t.Fatal("b materialized")
	}
	// Files outside the profile are invisible to status even if created.
	write(t, wc, "b/stray", "x")
	if s := statusString(t, wc); s != "" {
		t.Fatalf("status: %s", s)
	}
	os.RemoveAll(wc.Abs("b"))
	added, removed, err := wc.SetSparse(ctx, []string{"a", "c/d"}, false)
	if err != nil || added != 1 || removed != 0 {
		t.Fatalf("add: %d %d %v", added, removed, err)
	}
	write(t, wc, "a/1", "modified")
	if _, _, err := wc.SetSparse(ctx, []string{"c"}, false); err == nil {
		t.Fatal("dropping a modified file should fail")
	}
	write(t, wc, "a/1", "1")
	added, removed, err = wc.SetSparse(ctx, []string{"c"}, false)
	if err != nil || added != 0 || removed != 2 {
		t.Fatalf("remove: %d %d %v", added, removed, err)
	}
	if _, err := os.Stat(wc.Abs("a")); !os.IsNotExist(err) {
		t.Fatal("a still present")
	}
	added, _, err = wc.SetSparse(ctx, nil, false)
	if err != nil || added != 4 || len(wc.Files) != 5 {
		t.Fatalf("full: %d %v (%d files)", added, err, len(wc.Files))
	}
	// Commits from a sparse working copy only touch materialized paths and
	// update leaves unmaterialized paths alone.
	sp := clone(t, url, cs, "b")
	write(t, sp, "b/1", "changed")
	id, _, err := sp.Commit(ctx, CommitOptions{Message: "b", Author: "t <t@t>"})
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := sp.Repo.File(ctx, id.String(), "a/1"); string(b) != "1" {
		t.Fatal("unmaterialized file changed by sparse commit")
	}
	if _, err := wc.Update(ctx, id, false); err != nil || read(t, wc, "b/1") != "changed" {
		t.Fatalf("update: %v", err)
	}
}

func TestFileDirectoryTransitions(t *testing.T) {
	url, cs := setup(t, map[string]string{"x": "file\n", "d/inner": "i\n"})
	wc := clone(t, url, cs)
	other := clone(t, url, cs)
	os.Remove(other.Abs("x"))
	write(t, other, "x/now-a-dir", "y\n")
	os.RemoveAll(other.Abs("d"))
	write(t, other, "d", "now a file\n")
	id, _, err := other.Commit(ctx, CommitOptions{Message: "swap", Author: "t <t@t>"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wc.Update(ctx, id, false); err != nil {
		t.Fatal(err)
	}
	if read(t, wc, "x/now-a-dir") != "y\n" || read(t, wc, "d") != "now a file\n" {
		t.Fatal("transitions not applied")
	}
	if s := statusString(t, wc); s != "" {
		t.Fatalf("status: %s", s)
	}
}
