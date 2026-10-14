package iam

import "testing"

func TestPolicy(t *testing.T) {
	p, err := New(Config{
		Users: map[string]User{
			"alice": {Tokens: []string{"alice-token"}},
			"bob":   {Tokens: []string{"sha256:" + HashToken("bob-token")}},
			"root":  {Tokens: []string{"root-token"}},
		},
		Groups: map[string][]string{"eng": {"alice", "bob"}},
		Rules: []Rule{
			{Repo: "*", Path: "", Principals: []string{"anyone"}, Allow: Read},
			{Repo: "mono", Path: "", Principals: []string{"group:eng"}, Allow: Write},
			{Repo: "mono", Path: "", Principals: []string{"user:root"}, Allow: Admin},
			{Repo: "mono", Path: "secrets", Principals: []string{"user:alice"}, Allow: Write},
			{Repo: "mono", Path: "infra/prod", Principals: []string{"group:eng"}, Allow: Read},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	alice, err := p.Authenticate("alice-token")
	if err != nil || alice.Name != "alice" || len(alice.Groups) != 1 {
		t.Fatalf("alice: %+v %v", alice, err)
	}
	bob, _ := p.Authenticate("bob-token")
	root, _ := p.Authenticate("root-token")
	anon, _ := p.Authenticate("")
	if _, err := p.Authenticate("nope"); err == nil {
		t.Fatal("bad token accepted")
	}
	cases := []struct {
		who  Principal
		repo string
		path string
		want Level
	}{
		{anon, "mono", "src/a.go", Read},
		{anon, "other", "", Read},
		{bob, "mono", "src/a.go", Write},
		{bob, "mono", "secrets/key", None},
		{alice, "mono", "secrets/key", Write},
		{alice, "mono", "secretsfile", Write}, // prefix match is per component
		{bob, "mono", "infra/prod/x", Read},
		{anon, "mono", "infra/prod/x", None},
		{root, "mono", "secrets/key", Admin},
	}
	for _, c := range cases {
		if got := p.Level(c.who, c.repo, c.path); got != c.want {
			t.Errorf("Level(%s, %s, %s) = %s, want %s", c.who, c.repo, c.path, got, c.want)
		}
	}
	if p.FullRead(bob, "mono") || !p.FullRead(root, "mono") || !p.FullRead(anon, "other") {
		t.Fatal("FullRead")
	}
	if p.Filter(root, "mono") != nil || p.Filter(bob, "mono") == nil {
		t.Fatal("Filter")
	}
	if !p.CanSeeDir(bob, "mono", "infra") || p.CanSeeDir(bob, "mono", "secrets") {
		t.Fatal("CanSeeDir")
	}
	var nilPolicy *Policy
	if nilPolicy.Level(anon, "x", "y") != Admin || nilPolicy.Filter(anon, "x") != nil {
		t.Fatal("nil policy should allow everything")
	}
}
