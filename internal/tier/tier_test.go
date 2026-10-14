package tier_test

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/mattparavel/a-git/internal/api"
	"github.com/mattparavel/a-git/internal/client"
	"github.com/mattparavel/a-git/internal/fetch"
	"github.com/mattparavel/a-git/internal/iam"
	"github.com/mattparavel/a-git/internal/object"
	"github.com/mattparavel/a-git/internal/store"
	"github.com/mattparavel/a-git/internal/tier"
)

var ctx = context.Background()

type stack struct {
	server, cache string
	daemonSock    string
	cacheProxy    *tier.Proxy
	daemonProxy   *tier.Proxy
	serverStore   store.Store
}

func newStack(t *testing.T, policy *iam.Policy) *stack {
	t.Helper()
	st := &stack{serverStore: store.NewMem()}
	srv := httptest.NewServer(api.NewServer(api.ServerConfig{Store: st.serverStore, Policy: policy}).Handler())
	t.Cleanup(srv.Close)
	st.server = srv.URL

	up, _ := client.New(srv.URL, "")
	up.Tier = api.TierCache
	st.cacheProxy = tier.NewProxy(tier.Config{Tier: api.TierCache, Upstream: up, Objects: tier.NewMemCache(64 << 20)})
	cache := httptest.NewServer(st.cacheProxy.Handler())
	t.Cleanup(cache.Close)
	st.cache = cache.URL

	dup, _ := client.New(cache.URL, "")
	dup.Tier = api.TierDaemon
	disk, err := tier.OpenDiskCache(filepath.Join(t.TempDir(), "objects"), 1<<30)
	if err != nil {
		t.Fatal(err)
	}
	st.daemonProxy = tier.NewProxy(tier.Config{Tier: api.TierDaemon, Upstream: dup, Objects: disk})
	st.daemonSock = filepath.Join(t.TempDir(), "agitd.sock")
	ln, err := net.Listen("unix", st.daemonSock)
	if err != nil {
		t.Fatal(err)
	}
	hs := &http.Server{Handler: st.daemonProxy.Handler()}
	go hs.Serve(ln)
	t.Cleanup(func() { hs.Close() })
	return st
}

func seed(t *testing.T, c *client.Client) (object.ID, map[string]string) {
	t.Helper()
	if err := c.CreateRepo(ctx, "mono"); err != nil {
		t.Fatal(err)
	}
	r := c.Repo("mono")
	files := map[string]string{
		"README.md":      "hello\n",
		"src/a/a.go":     "package a\n",
		"src/b/b.go":     "package b\n",
		"docs/guide.md":  "guide\n",
		"secrets/token":  "s3cret\n",
		"src/a/copy.txt": "hello\n", // same blob as README.md
	}
	cs := &object.Changeset{Author: "t", Message: "init"}
	var objs []client.StoredObject
	for p, content := range files {
		id, stored := object.Encode(object.KindBlob, []byte(content))
		objs = append(objs, client.StoredObject{ID: id, Stored: stored})
		cs.Changes = append(cs.Changes, object.Change{Path: p, Mode: object.ModeFile, Blob: id})
	}
	if _, err := r.UploadMissing(ctx, objs); err != nil {
		t.Fatal(err)
	}
	res, err := r.CreateChangeset(ctx, cs, &api.RefMove{Name: "main"})
	if err != nil {
		t.Fatal(err)
	}
	return res.ID, files
}

func checkout(t *testing.T, r *client.Repo, cs object.ID, prefix string) map[string]string {
	t.Helper()
	root, err := r.RootTree(ctx, cs)
	if err != nil {
		t.Fatal(err)
	}
	f := &fetch.Fetcher{Src: r, CS: cs}
	var wants []fetch.Want
	include := func(p string, isDir bool) bool {
		return prefix == "" || object.HasPathPrefix(p, prefix) || isDir && object.HasPathPrefix(prefix, p)
	}
	if err := f.Walk(ctx, root, include, func(e fetch.Entry) error {
		wants = append(wants, fetch.Want{ID: e.ID, Path: e.Path})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	err = f.Get(ctx, wants, func(w fetch.Want, kind object.Kind, raw []byte) error {
		out[w.Path] = string(raw)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestTierChain(t *testing.T) {
	st := newStack(t, nil)
	direct, _ := client.New(st.server, "")
	cs, files := seed(t, direct)

	viaDaemon, err := client.New("unix://"+st.daemonSock, "")
	if err != nil {
		t.Fatal(err)
	}
	info, err := viaDaemon.Info(ctx)
	if err != nil || info.Tier != api.TierDaemon {
		t.Fatalf("info: %+v %v", info, err)
	}
	r := viaDaemon.Repo("mono")
	got := checkout(t, r, cs, "")
	if len(got) != len(files) {
		t.Fatalf("checkout: %d files", len(got))
	}
	for p, c := range files {
		if got[p] != c {
			t.Fatalf("%s = %q", p, got[p])
		}
	}
	daemonStats := st.daemonProxy.Objects().Stats()
	cacheStats := st.cacheProxy.Objects().Stats()
	if daemonStats.Objects == 0 || cacheStats.Objects == 0 {
		t.Fatalf("caches not populated: daemon %+v cache %+v", daemonStats, cacheStats)
	}

	// A second checkout is served entirely by the daemon.
	before := st.cacheProxy.Objects().Stats()
	got = checkout(t, r, cs, "src")
	if len(got) != 3 {
		t.Fatalf("sparse checkout: %v", got)
	}
	after := st.cacheProxy.Objects().Stats()
	if after.Hits != before.Hits || after.Misses != before.Misses {
		t.Fatalf("cache tier was consulted on a warm daemon: %+v -> %+v", before, after)
	}

	// Immutable API responses are cached by tiers; mutable ones are not.
	for i := 0; i < 2; i++ {
		req, _ := viaDaemon.NewRequest(ctx, "GET", "/v1/repos/mono/tree/"+cs.String(), nil, nil)
		resp, err := viaDaemon.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if want := []string{"miss", "hit"}[i]; resp.Header.Get(api.HeaderCache) != want {
			t.Fatalf("request %d: cache %q, want %q", i, resp.Header.Get(api.HeaderCache), want)
		}
	}
	req, _ := viaDaemon.NewRequest(ctx, "GET", "/v1/repos/mono/refs", nil, nil)
	for i := 0; i < 2; i++ {
		resp, err := viaDaemon.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.Header.Get(api.HeaderCache) == "hit" {
			t.Fatal("mutable response served from cache")
		}
	}

	// Writes pass through all tiers.
	id, stored := object.Encode(object.KindBlob, []byte("new\n"))
	if _, err := r.UploadMissing(ctx, []client.StoredObject{{ID: id, Stored: stored}}); err != nil {
		t.Fatal(err)
	}
	res, err := r.CreateChangeset(ctx, &object.Changeset{Parents: []object.ID{cs}, Author: "t", Changes: []object.Change{{Path: "new.txt", Mode: object.ModeFile, Blob: id}}}, &api.RefMove{Name: "main", Old: cs})
	if err != nil {
		t.Fatal(err)
	}
	if head, _ := r.Resolve(ctx, "main"); head != res.ID {
		t.Fatal("ref not advanced through tiers")
	}

	// Range requests (resumable downloads) work through the tiers.
	req, _ = viaDaemon.NewRequest(ctx, "GET", "/v1/repos/mono/objects/"+id.String(), nil, nil)
	req.Header.Set("Range", "bytes=1-")
	resp, err := viaDaemon.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	buf.ReadFrom(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent || !bytes.Equal(buf.Bytes(), stored[1:]) {
		t.Fatalf("range: %d %q", resp.StatusCode, buf.Bytes())
	}

	// Strict tier ordering: the server refuses requests that claim to come
	// from a tier above the cache, and the cache refuses requests from the
	// daemon pretending to be a cache.
	bad, _ := client.New(st.cache, "")
	bad.Tier = api.TierCache
	if _, err := bad.Repo("mono").Refs(ctx, ""); err == nil {
		t.Fatal("cache accepted a request from another cache")
	}

	// Prefetch warms the daemon for a path.
	ps, err := st.daemonProxy.Prefetch(ctx, "mono", "main", "", "")
	if err != nil || ps.Files != len(files)+1 {
		t.Fatalf("prefetch: %+v %v", ps, err)
	}
}

func TestTierIAM(t *testing.T) {
	policy, _ := iam.New(iam.Config{
		Users: map[string]iam.User{"admin": {Tokens: []string{"admin"}}, "dev": {Tokens: []string{"dev"}}},
		Rules: []iam.Rule{
			{Principals: []string{"user:admin"}, Allow: iam.Admin},
			{Principals: []string{"user:dev"}, Allow: iam.Write},
			{Path: "secrets", Principals: []string{"user:admin"}, Allow: iam.Admin},
		},
	})
	st := newStack(t, policy)
	admin, _ := client.New(st.server, "admin")
	cs, _ := seed(t, admin)

	// Admin warms the daemon with everything, including secrets.
	adminViaDaemon, _ := client.New("unix://"+st.daemonSock, "admin")
	if got := checkout(t, adminViaDaemon.Repo("mono"), cs, ""); got["secrets/token"] != "s3cret\n" {
		t.Fatal("admin checkout")
	}
	secret, _ := admin.Repo("mono").Tree(ctx, "main", "secrets/token")

	// The dev user shares the daemon but must not get the cached secret.
	dev, _ := client.New("unix://"+st.daemonSock, "dev")
	dr := dev.Repo("mono")
	if _, err := dr.GetObject(ctx, api.ObjectRef{ID: secret.ID}); err == nil {
		t.Fatal("dev read a cached secret by hash")
	}
	if _, err := dr.GetObject(ctx, api.ObjectRef{ID: secret.ID, CS: cs, Path: "secrets/token"}); err == nil {
		t.Fatal("dev read a cached secret with a hint")
	}
	got := checkout(t, dr, cs, "src")
	if len(got) != 3 {
		t.Fatalf("dev sparse checkout: %v", got)
	}
	// The README blob also lives under src/, so it is readable via that path.
	readme, _ := admin.Repo("mono").Tree(ctx, "main", "README.md")
	if _, err := dr.GetObject(ctx, api.ObjectRef{ID: readme.ID, CS: cs, Path: "src/a/copy.txt"}); err != nil {
		t.Fatalf("dev hinted read: %v", err)
	}
	anon, _ := client.New("unix://"+st.daemonSock, "")
	if _, err := anon.Repo("mono").GetObject(ctx, api.ObjectRef{ID: readme.ID}); err == nil {
		t.Fatal("anonymous read through daemon")
	}
}
