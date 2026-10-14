// Package iam implements a-git's access control: bearer-token principals,
// groups, and per-repository, per-directory permission rules.
//
// Rules grant a permission level to principals on a repository (glob) and a
// path prefix. For a given path, the governing rules are those with the
// longest prefix that has any rule at all: declaring rules on "secrets/"
// therefore replaces, rather than adds to, whatever the repository root
// grants. Repository admins keep access everywhere.
package iam

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"sort"
	"strings"

	"github.com/mattparavel/a-git/internal/object"
)

// Level is a permission level.
type Level int

const (
	None Level = iota
	Read
	Write
	Admin
)

func (l Level) String() string {
	switch l {
	case Read:
		return "read"
	case Write:
		return "write"
	case Admin:
		return "admin"
	}
	return "none"
}

// MarshalText implements encoding.TextMarshaler.
func (l Level) MarshalText() ([]byte, error) { return []byte(l.String()), nil }

// UnmarshalText implements encoding.TextUnmarshaler.
func (l *Level) UnmarshalText(b []byte) error {
	switch string(b) {
	case "none", "":
		*l = None
	case "read":
		*l = Read
	case "write":
		*l = Write
	case "admin":
		*l = Admin
	default:
		return fmt.Errorf("unknown permission level %q", b)
	}
	return nil
}

// User is an authenticated principal.
type User struct {
	// Tokens are accepted bearer tokens, either in plain text or as
	// "sha256:<hex>" digests so configuration files need not hold secrets.
	Tokens []string `json:"tokens"`
	Email  string   `json:"email,omitempty"`
}

// Rule grants Allow to Principals on paths under Path in repositories
// matching Repo.
type Rule struct {
	Repo       string   `json:"repo"`       // glob, "*" for all
	Path       string   `json:"path"`       // directory prefix, "" for the whole repository
	Principals []string `json:"principals"` // "user:NAME", "group:NAME", "authenticated", "anyone"
	Allow      Level    `json:"allow"`
}

// Config is the IAM section of the server configuration.
type Config struct {
	Users  map[string]User     `json:"users"`
	Groups map[string][]string `json:"groups"`
	Rules  []Rule              `json:"rules"`
}

// Principal is the identity behind a request. The zero value is anonymous.
type Principal struct {
	Name   string
	Groups []string
}

// Anonymous reports whether the principal is unauthenticated.
func (p Principal) Anonymous() bool { return p.Name == "" }

func (p Principal) String() string {
	if p.Anonymous() {
		return "anonymous"
	}
	return p.Name
}

// Policy evaluates permissions. A nil *Policy allows everything, which is
// the behaviour of a server started without an IAM configuration.
type Policy struct {
	cfg    Config
	tokens map[string]string // sha256 hex -> user
}

// Load reads a policy from a JSON file.
func Load(file string) (*Policy, error) {
	b, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(b, &cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", file, err)
	}
	return New(cfg)
}

// New builds a policy from a configuration.
func New(cfg Config) (*Policy, error) {
	p := &Policy{cfg: cfg, tokens: map[string]string{}}
	for name, u := range cfg.Users {
		for _, t := range u.Tokens {
			digest := ""
			if strings.HasPrefix(t, "sha256:") {
				digest = strings.ToLower(strings.TrimPrefix(t, "sha256:"))
			} else {
				digest = HashToken(t)
			}
			if other, dup := p.tokens[digest]; dup && other != name {
				return nil, fmt.Errorf("token shared by users %q and %q", other, name)
			}
			p.tokens[digest] = name
		}
	}
	for i, r := range cfg.Rules {
		if r.Repo == "" {
			cfg.Rules[i].Repo = "*"
		}
		cfg.Rules[i].Path = object.CleanPath(r.Path)
	}
	return p, nil
}

// HashToken returns the hex SHA-256 digest of a token.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// Authenticate maps a bearer token to a principal. An empty token yields the
// anonymous principal; an unknown token is an error.
func (p *Policy) Authenticate(token string) (Principal, error) {
	if p == nil || token == "" {
		return Principal{}, nil
	}
	digest := HashToken(token)
	for d, name := range p.tokens {
		if subtle.ConstantTimeCompare([]byte(d), []byte(digest)) == 1 {
			return Principal{Name: name, Groups: p.groupsOf(name)}, nil
		}
	}
	return Principal{}, fmt.Errorf("invalid credentials")
}

func (p *Policy) groupsOf(user string) []string {
	var out []string
	for g, members := range p.cfg.Groups {
		for _, m := range members {
			if m == user {
				out = append(out, g)
				break
			}
		}
	}
	sort.Strings(out)
	return out
}

// Ident returns a "Name <email>" identity for a principal, used as the
// author or committer of changesets the server creates on its behalf.
func (p *Policy) Ident(pr Principal) string {
	name := pr.String()
	if strings.ContainsAny(name, "<>") {
		return name
	}
	email := ""
	if p != nil {
		email = p.cfg.Users[pr.Name].Email
	}
	if email == "" {
		email = name + "@a-git"
	}
	return fmt.Sprintf("%s <%s>", name, email)
}

// Members returns the users in a group.
func (p *Policy) Members(group string) []string {
	if p == nil {
		return nil
	}
	return append([]string(nil), p.cfg.Groups[group]...)
}

func (p *Policy) matches(pr Principal, who string) bool {
	switch {
	case who == "anyone":
		return true
	case who == "authenticated":
		return !pr.Anonymous()
	case strings.HasPrefix(who, "user:"):
		return !pr.Anonymous() && pr.Name == who[5:]
	case strings.HasPrefix(who, "group:"):
		for _, g := range pr.Groups {
			if g == who[6:] {
				return true
			}
		}
	}
	return false
}

func repoMatch(glob, repo string) bool {
	ok, _ := path.Match(glob, repo)
	return ok
}

// Level returns the principal's permission for a path in a repository
// ("" is the repository root).
func (p *Policy) Level(pr Principal, repo, filePath string) Level {
	if p == nil {
		return Admin
	}
	filePath = object.CleanPath(filePath)
	var admin Level
	boundary := -1
	var best Level
	for _, r := range p.cfg.Rules {
		if !repoMatch(r.Repo, repo) || !object.HasPathPrefix(filePath, r.Path) {
			continue
		}
		matched := p.anyMatch(pr, r.Principals)
		if r.Path == "" && r.Allow == Admin && matched {
			admin = Admin
		}
		l := len(r.Path)
		if l > boundary {
			boundary, best = l, None
		}
		if l == boundary && matched && r.Allow > best {
			best = r.Allow
		}
	}
	if admin == Admin {
		return Admin
	}
	return best
}

func (p *Policy) anyMatch(pr Principal, whos []string) bool {
	for _, w := range whos {
		if p.matches(pr, w) {
			return true
		}
	}
	return false
}

// Can reports whether the principal has at least level on a path.
func (p *Policy) Can(pr Principal, repo, filePath string, level Level) bool {
	return p.Level(pr, repo, filePath) >= level
}

// RestrictedPrefixes returns the path prefixes in repo below the root that
// carry their own rules. Paths beneath them may be readable by fewer
// principals than the repository root.
func (p *Policy) RestrictedPrefixes(repo string) []string {
	if p == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, r := range p.cfg.Rules {
		if r.Path != "" && repoMatch(r.Repo, repo) && !seen[r.Path] {
			seen[r.Path] = true
			out = append(out, r.Path)
		}
	}
	sort.Strings(out)
	return out
}

// FullRead reports whether the principal can read every path in repo.
func (p *Policy) FullRead(pr Principal, repo string) bool {
	if !p.Can(pr, repo, "", Read) {
		return false
	}
	for _, pre := range p.RestrictedPrefixes(repo) {
		if !p.Can(pr, repo, pre, Read) {
			return false
		}
	}
	return true
}

// Filter returns a predicate reporting whether the principal may read a
// path, or nil if the principal can read everything (the common case, which
// lets callers skip per-path checks).
func (p *Policy) Filter(pr Principal, repo string) func(string) bool {
	if p.FullRead(pr, repo) {
		return nil
	}
	return func(fp string) bool { return p.Can(pr, repo, fp, Read) }
}

// CanSeeDir reports whether a directory should be visible in listings: it is
// readable itself or contains a readable restricted prefix.
func (p *Policy) CanSeeDir(pr Principal, repo, dir string) bool {
	if p.Can(pr, repo, dir, Read) {
		return true
	}
	for _, pre := range p.RestrictedPrefixes(repo) {
		if object.HasPathPrefix(pre, dir) && p.Can(pr, repo, pre, Read) {
			return true
		}
	}
	return false
}
