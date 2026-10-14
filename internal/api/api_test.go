package api_test

import (
	"context"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/mattparavel/a-git/internal/api"
	"github.com/mattparavel/a-git/internal/client"
	"github.com/mattparavel/a-git/internal/iam"
	"github.com/mattparavel/a-git/internal/object"
	"github.com/mattparavel/a-git/internal/review"
	"github.com/mattparavel/a-git/internal/store"
)

var ctx = context.Background()

func startServer(t *testing.T, cfg api.ServerConfig) string {
	t.Helper()
	if cfg.Store == nil {
		cfg.Store = store.NewMem()
	}
	srv := httptest.NewServer(api.NewServer(cfg).Handler())
	t.Cleanup(srv.Close)
	return srv.URL
}

func newClient(t *testing.T, url, token string) *client.Client {
	t.Helper()
	c, err := client.New(url, token)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// commitFiles uploads blobs and creates a changeset through the API.
func commitFiles(t *testing.T, r *client.Repo, parent object.ID, msg string, files map[string]string, ref *api.RefMove) object.ID {
	t.Helper()
	cs := &object.Changeset{Author: "tester", AuthorTime: 1, Message: msg}
	if !parent.IsZero() {
		cs.Parents = []object.ID{parent}
	}
	var objs []client.StoredObject
	for p, content := range files {
		if content == "" {
			cs.Changes = append(cs.Changes, object.Change{Path: p})
			continue
		}
		id, stored := object.Encode(object.KindBlob, []byte(content))
		objs = append(objs, client.StoredObject{ID: id, Stored: stored})
		cs.Changes = append(cs.Changes, object.Change{Path: p, Mode: object.ModeFile, Blob: id})
	}
	if _, err := r.UploadMissing(ctx, objs); err != nil {
		t.Fatal(err)
	}
	res, err := r.CreateChangeset(ctx, cs, ref)
	if err != nil {
		t.Fatalf("create %q: %v", msg, err)
	}
	return res.ID
}

func TestEndToEnd(t *testing.T) {
	url := startServer(t, api.ServerConfig{})
	c := newClient(t, url, "")
	if err := c.CreateRepo(ctx, "mono"); err != nil {
		t.Fatal(err)
	}
	r := c.Repo("mono")
	c1 := commitFiles(t, r, object.Zero, "init", map[string]string{"README.md": "hello\n", "src/main.go": "package main\n"}, &api.RefMove{Name: "main"})
	c2 := commitFiles(t, r, c1, "edit", map[string]string{"src/main.go": "package main\n\nfunc main() {}\n"}, &api.RefMove{Name: "main", Old: c1})

	// Reads without a checkout.
	if b, err := r.File(ctx, "main", "src/main.go"); err != nil || !strings.Contains(string(b), "func main") {
		t.Fatalf("file: %q %v", b, err)
	}
	tree, err := r.Tree(ctx, "main", "")
	if err != nil || len(tree.Entries) != 2 {
		t.Fatalf("tree: %+v %v", tree, err)
	}
	log, err := r.Log(ctx, client.LogOptions{Rev: "main"})
	if err != nil || len(log) != 2 || log[0].ID != c2 {
		t.Fatalf("log: %+v %v", log, err)
	}
	if id, _ := r.Resolve(ctx, "main~1"); id != c1 {
		t.Fatal("resolve main~1")
	}
	d, err := r.Diff(ctx, client.DiffOptions{To: c2.String(), Patch: true})
	if err != nil || len(d.Files) != 1 || !strings.Contains(d.Files[0].Patch, "+func main() {}") {
		t.Fatalf("diff: %+v %v", d, err)
	}
	blame, err := r.Blame(ctx, "main", "src/main.go")
	if err != nil || len(blame) != 2 || blame[0].Changeset != c1 || blame[1].Changeset != c2 {
		t.Fatalf("blame: %+v %v", blame, err)
	}
	// Changesets verify against their IDs.
	cs, err := r.GetChangeset(ctx, c2)
	if err != nil || cs.ID() != c2 {
		t.Fatalf("changeset: %v", err)
	}
	// Objects fetched by walking from the root tree.
	root, _ := r.RootTree(ctx, c2)
	got := 0
	missing, err := r.GetObjects(ctx, []api.ObjectRef{{ID: root}, {ID: object.Hash(object.KindBlob, []byte("nope"))}}, func(id object.ID, stored []byte) error {
		got++
		return nil
	})
	if err != nil || got != 1 || len(missing) != 1 {
		t.Fatalf("objects: got %d missing %d err %v", got, len(missing), err)
	}
	// CAS failures surface as such.
	err = r.UpdateRef(ctx, "main", c1, c1)
	if !client.IsCode(err, "cas_failed") {
		t.Fatalf("expected cas_failed, got %v", err)
	}
	// Server-side land of a concurrent change.
	other := commitFiles(t, r, c1, "other", map[string]string{"README.md": "hello world\n"}, nil)
	res, err := r.Land(ctx, api.LandRequest{Head: other.String(), Ref: "main"})
	if err != nil || res.Rebase == nil {
		t.Fatalf("land: %+v %v", res, err)
	}
	if b, _ := r.File(ctx, "main", "README.md"); string(b) != "hello world\n" {
		t.Fatalf("README after land: %q", b)
	}
	// Conflicting land returns structured conflicts.
	bad := commitFiles(t, r, c1, "bad", map[string]string{"README.md": "goodbye\n"}, nil)
	_, err = r.Land(ctx, api.LandRequest{Head: bad.String(), Ref: "main"})
	if conflicts, ok := client.IsConflict(err); !ok || len(conflicts) != 1 || conflicts[0].Path != "README.md" {
		t.Fatalf("expected conflict, got %v", err)
	}
	// Event log.
	wal, err := r.WAL(ctx, 0, 100)
	if err != nil || len(wal) != 5 {
		t.Fatalf("wal: %d %v", len(wal), err)
	}
	// Tier ordering: a cache may not call a daemon.
	cc := newClient(t, url, "")
	cc.Tier = "bogus"
	if _, err := cc.Repo("mono").Refs(ctx, ""); err == nil {
		t.Fatal("expected tier-order error")
	}
}

func TestIAMAndReviews(t *testing.T) {
	policy, err := iam.New(iam.Config{
		Users: map[string]iam.User{
			"alice": {Tokens: []string{"alice"}},
			"bob":   {Tokens: []string{"bob"}},
			"carol": {Tokens: []string{"carol"}},
			"admin": {Tokens: []string{"admin"}},
		},
		Groups: map[string][]string{"eng": {"alice", "bob", "carol"}, "payments": {"carol"}},
		Rules: []iam.Rule{
			{Repo: "*", Principals: []string{"group:eng"}, Allow: iam.Write},
			{Repo: "*", Principals: []string{"user:admin"}, Allow: iam.Admin},
			{Repo: "*", Path: "secrets", Principals: []string{"user:carol"}, Allow: iam.Write},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	url := startServer(t, api.ServerConfig{Policy: policy, ProtectedRefs: []string{"main"}})
	admin := newClient(t, url, "admin")
	if err := newClient(t, url, "alice").CreateRepo(ctx, "mono"); err == nil {
		t.Fatal("non-admin created a repo")
	}
	if err := admin.CreateRepo(ctx, "mono"); err != nil {
		t.Fatal(err)
	}
	ar := admin.Repo("mono")
	c1 := commitFiles(t, ar, object.Zero, "init", map[string]string{
		"README.md":     "readme\n",
		"pay/REVIEWERS": "group:payments\n!trivial any\n",
		"pay/charge.go": "package pay\n\n// Charge charges.\nfunc Charge() int { return 1 }\n",
		"secrets/key":   "s3cret\n",
	}, &api.RefMove{Name: "main"})

	alice := newClient(t, url, "alice").Repo("mono")
	bob := newClient(t, url, "bob").Repo("mono")
	carol := newClient(t, url, "carol").Repo("mono")
	anon := newClient(t, url, "").Repo("mono")

	// Directory-level read and write protection.
	if _, err := anon.Refs(ctx, ""); err == nil {
		t.Fatal("anonymous read allowed")
	}
	if _, err := alice.File(ctx, "main", "secrets/key"); err == nil {
		t.Fatal("alice read a secret")
	}
	if b, err := carol.File(ctx, "main", "secrets/key"); err != nil || string(b) != "s3cret\n" {
		t.Fatalf("carol read: %v", err)
	}
	tree, _ := alice.Tree(ctx, "main", "")
	for _, e := range tree.Entries {
		if e.Name == "secrets" {
			t.Fatal("alice can see the secrets directory")
		}
	}
	acc, _ := alice.Access(ctx)
	if acc.Read != "partial" || !acc.Write {
		t.Fatalf("alice access: %+v", acc)
	}
	// Objects under secrets/ cannot be fetched by hash without a valid hint.
	e, _ := carol.Tree(ctx, "main", "secrets/key")
	if _, err := alice.GetObject(ctx, api.ObjectRef{ID: e.ID}); err == nil {
		t.Fatal("alice fetched a secret blob by hash")
	}
	if _, err := alice.GetObject(ctx, api.ObjectRef{ID: e.ID, CS: c1, Path: "secrets/key"}); err == nil {
		t.Fatal("alice fetched a secret blob with a hint")
	}
	readme, _ := alice.Tree(ctx, "main", "README.md")
	if _, err := alice.GetObject(ctx, api.ObjectRef{ID: readme.ID, CS: c1, Path: "README.md"}); err != nil {
		t.Fatalf("alice hinted fetch: %v", err)
	}
	// Writes to secrets/ are rejected for alice.
	blob, stored := object.Encode(object.KindBlob, []byte("x"))
	alice.PutObjects(ctx, []client.StoredObject{{ID: blob, Stored: stored}})
	_, err = alice.CreateChangeset(ctx, &object.Changeset{Parents: []object.ID{c1}, Author: "alice", Changes: []object.Change{{Path: "secrets/key", Mode: object.ModeFile, Blob: blob}}}, nil)
	if err == nil {
		t.Fatal("alice wrote a secret")
	}
	// Protected ref: direct updates and direct lands are refused.
	logic := commitFiles(t, alice, c1, "pay: charge two", map[string]string{"pay/charge.go": "package pay\n\n// Charge charges.\nfunc Charge() int { return 2 }\n"}, nil)
	if err := alice.UpdateRef(ctx, "main", c1, logic); err == nil {
		t.Fatal("protected ref updated directly")
	}
	if _, err := alice.Land(ctx, api.LandRequest{Head: logic.String(), Ref: "main"}); err == nil {
		t.Fatal("protected ref landed without review")
	}

	// Review flow: logic change under pay/ needs a payments owner.
	rr, err := alice.CreateReview(ctx, review.CreateRequest{Head: logic.String()})
	if err != nil {
		t.Fatal(err)
	}
	id := "R" + itoa(rr.Review.ID)
	if rr.Status.Approved || rr.Status.Class != "logic" {
		t.Fatalf("new review status: %+v", rr.Status)
	}
	if _, err := alice.ApproveReview(ctx, id); err != nil { // self-approval does not count
		t.Fatal(err)
	}
	if st, _ := bob.ApproveReview(ctx, id); st.Status.Approved {
		t.Fatal("bob is not a payments owner")
	}
	if _, err := alice.LandReview(ctx, id); err == nil {
		t.Fatal("landed without owner approval")
	}
	st, _ := carol.ApproveReview(ctx, id)
	if !st.Status.Approved {
		t.Fatalf("carol's approval should satisfy: %+v", st.Status)
	}
	// A comment-only revision keeps the approval.
	commentOnly := commitFiles(t, alice, c1, "pay: charge two", map[string]string{"pay/charge.go": "package pay\n\n// Charge charges the customer twice.\nfunc Charge() int { return 2 }\n"}, nil)
	up, err := alice.UploadReview(ctx, id, commentOnly.String())
	if err != nil || up.Review.Latest().Interdiff != "comments" || !up.Status.Approved {
		t.Fatalf("comment-only revision: %+v %+v %v", up.Review.Latest(), up.Status, err)
	}
	// A logic revision invalidates it.
	logic2 := commitFiles(t, alice, c1, "pay: charge three", map[string]string{"pay/charge.go": "package pay\n\n// Charge charges the customer twice.\nfunc Charge() int { return 3 }\n"}, nil)
	up, _ = alice.UploadReview(ctx, id, logic2.String())
	if up.Status.Approved || len(up.Status.Stale) == 0 {
		t.Fatalf("logic revision should need re-approval: %+v", up.Status)
	}
	carol.ApproveReview(ctx, id)
	landed, err := alice.LandReview(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if landed.Review.State != review.StateLanded {
		t.Fatalf("state: %s", landed.Review.State)
	}
	found, err := alice.FindReview(ctx, landed.Land.New.String())
	if err != nil || found.Review.ID != rr.Review.ID {
		t.Fatalf("find review by landed changeset: %v", err)
	}

	// Docs-only change in pay/ may be approved by any non-author.
	docs := commitFiles(t, bob, landed.Land.New, "docs", map[string]string{"pay/README.md": "Payments.\n"}, nil)
	rr2, err := bob.CreateReview(ctx, review.CreateRequest{Head: docs.String()})
	if err != nil {
		t.Fatal(err)
	}
	if rr2.Status.Class != "docs" {
		t.Fatalf("class: %s", rr2.Status.Class)
	}
	st, _ = alice.ApproveReview(ctx, itoa(rr2.Review.ID))
	if !st.Status.Approved {
		t.Fatalf("trivial change should accept any approver: %+v", st.Status)
	}
}

func itoa(n int) string { return strconv.Itoa(n) }
