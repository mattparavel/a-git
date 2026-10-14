// Package review implements a-git's code review service.
//
// A review tracks a stack of changesets headed for a target ref through a
// series of revisions. Its approval rules are semantic rather than purely
// mechanical:
//
//   - REVIEWERS files name the principals who own a directory; changes to a
//     path need approval from the owners of the nearest REVIEWERS file.
//   - When the author uploads a new revision, existing approvals carry over
//     if the difference between revisions is trivial (only comments or
//     whitespace changed), so reviewers re-review only consequential edits.
//   - Owners can declare that trivial changes (documentation, comments,
//     whitespace) need just one approval from anyone other than the author.
//
// Landing goes through the review, which records which changesets landed
// where. "Which review landed this changeset?" is answered from that index
// rather than by scanning history.
package review

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mattparavel/a-git/internal/iam"
	"github.com/mattparavel/a-git/internal/object"
	"github.com/mattparavel/a-git/internal/repo"
	"github.com/mattparavel/a-git/internal/semantic"
	"github.com/mattparavel/a-git/internal/store"
)

var (
	// ErrNotApproved is returned when landing a review that fails its rules.
	ErrNotApproved = errors.New("review is not approved")
	// ErrInvalid is returned for malformed or out-of-state requests.
	ErrInvalid = errors.New("invalid review request")
)

// State of a review.
const (
	StateOpen      = "open"
	StateLanded    = "landed"
	StateAbandoned = "abandoned"
)

// Revision is one uploaded version of a review's stack.
type Revision struct {
	Number int         `json:"number"`
	Head   object.ID   `json:"head"`
	Base   object.ID   `json:"base"`
	Stack  []object.ID `json:"stack"`
	Time   int64       `json:"time"`
	// Interdiff classifies the change from the previous revision; trivial
	// interdiffs let earlier approvals carry over.
	Interdiff string `json:"interdiff,omitempty"`
}

// Approval is a reviewer's sign-off on a revision.
type Approval struct {
	User     string `json:"user"`
	Revision int    `json:"revision"`
	Time     int64  `json:"time"`
}

// Comment is a review comment, optionally anchored to a line.
type Comment struct {
	ID       int    `json:"id"`
	Author   string `json:"author"`
	Body     string `json:"body"`
	Path     string `json:"path,omitempty"`
	Line     int    `json:"line,omitempty"`
	Revision int    `json:"revision"`
	Time     int64  `json:"time"`
}

// Review is the stored review document.
type Review struct {
	ID          int         `json:"id"`
	Title       string      `json:"title"`
	Description string      `json:"description,omitempty"`
	Author      string      `json:"author"`
	Target      string      `json:"target"`
	State       string      `json:"state"`
	Revisions   []Revision  `json:"revisions"`
	Approvals   []Approval  `json:"approvals,omitempty"`
	Comments    []Comment   `json:"comments,omitempty"`
	Created     int64       `json:"created"`
	Updated     int64       `json:"updated"`
	Landed      object.ID   `json:"landed,omitzero"` // new tip of the target after landing
	LandedStack []object.ID `json:"landed_stack,omitempty"`
}

// Latest returns the newest revision.
func (r *Review) Latest() *Revision { return &r.Revisions[len(r.Revisions)-1] }

// Service manages reviews stored in the repository metadata store.
type Service struct {
	repos  *repo.Registry
	policy *iam.Policy
	now    func() time.Time
}

// NewService returns a review service.
func NewService(repos *repo.Registry, policy *iam.Policy) *Service {
	return &Service{repos: repos, policy: policy, now: time.Now}
}

const (
	nsReviews = "reviews"
	nsLanded  = "landed"
	nsMeta    = "review-meta"
)

func key(id int) string { return strconv.Itoa(id) }

func (s *Service) load(ctx context.Context, r *repo.Repo, id int) (*Review, error) {
	b, err := r.Store.KVGet(ctx, r.Name, nsReviews, key(id))
	if errors.Is(err, store.ErrNotFound) {
		return nil, fmt.Errorf("review %d: %w", id, store.ErrNotFound)
	}
	if err != nil {
		return nil, err
	}
	var rv Review
	return &rv, json.Unmarshal(b, &rv)
}

// update applies fn to a review atomically.
func (s *Service) update(ctx context.Context, r *repo.Repo, id int, fn func(*Review) error) (*Review, error) {
	var out *Review
	err := r.Store.KVUpdate(ctx, r.Name, nsReviews, key(id), func(old []byte) ([]byte, error) {
		if old == nil {
			return nil, fmt.Errorf("review %d: %w", id, store.ErrNotFound)
		}
		var rv Review
		if err := json.Unmarshal(old, &rv); err != nil {
			return nil, err
		}
		if err := fn(&rv); err != nil {
			return nil, err
		}
		rv.Updated = s.now().Unix()
		out = &rv
		return json.Marshal(&rv)
	})
	return out, err
}

func (s *Service) nextID(ctx context.Context, r *repo.Repo) (int, error) {
	var id int
	err := r.Store.KVUpdate(ctx, r.Name, nsMeta, "seq", func(old []byte) ([]byte, error) {
		n, _ := strconv.Atoi(string(old))
		id = n + 1
		return []byte(strconv.Itoa(id)), nil
	})
	return id, err
}

// ParseID accepts "12" or "R12".
func ParseID(s string) (int, error) {
	n, err := strconv.Atoi(strings.TrimPrefix(strings.TrimPrefix(s, "R"), "r"))
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("%w: bad review id %q", ErrInvalid, s)
	}
	return n, nil
}

func (s *Service) makeRevision(ctx context.Context, r *repo.Repo, number int, head object.ID, target string) (Revision, error) {
	tip, err := r.Ref(ctx, target)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return Revision{}, err
	}
	base := object.Zero
	if !tip.IsZero() {
		if base, err = r.MergeBase(ctx, head, tip); err != nil {
			return Revision{}, err
		}
	}
	stack, err := r.Stack(ctx, head, base)
	if err != nil {
		return Revision{}, err
	}
	if len(stack) == 0 {
		return Revision{}, fmt.Errorf("%w: %s is already on %s; nothing to review", ErrInvalid, head.Short(), target)
	}
	return Revision{Number: number, Head: head, Base: base, Stack: stack, Time: s.now().Unix()}, nil
}

// CreateRequest opens a review.
type CreateRequest struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Head        string `json:"head"`
	Target      string `json:"target,omitempty"`
}

// Create opens a review for the stack ending at req.Head.
func (s *Service) Create(ctx context.Context, repoName string, pr iam.Principal, req CreateRequest) (*Review, error) {
	r, err := s.repos.Get(ctx, repoName)
	if err != nil {
		return nil, err
	}
	if !s.policy.Can(pr, repoName, "", iam.Read) {
		return nil, fmt.Errorf("%w: no access", ErrInvalid)
	}
	if req.Target == "" {
		req.Target = "main"
	}
	head, err := r.Resolve(ctx, req.Head)
	if err != nil {
		return nil, err
	}
	rev, err := s.makeRevision(ctx, r, 1, head, req.Target)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.Title) == "" {
		cs, err := r.GetChangeset(ctx, head)
		if err != nil {
			return nil, err
		}
		req.Title = cs.Summary()
	}
	id, err := s.nextID(ctx, r)
	if err != nil {
		return nil, err
	}
	now := s.now().Unix()
	rv := &Review{
		ID: id, Title: req.Title, Description: req.Description, Author: pr.String(),
		Target: req.Target, State: StateOpen, Revisions: []Revision{rev}, Created: now, Updated: now,
	}
	b, _ := json.Marshal(rv)
	err = r.Store.KVUpdate(ctx, r.Name, nsReviews, key(id), func(old []byte) ([]byte, error) {
		if old != nil {
			return nil, fmt.Errorf("review %d: %w", id, store.ErrExists)
		}
		return b, nil
	})
	if err != nil {
		return nil, err
	}
	s.indexStack(ctx, r, id, rev.Stack)
	return rv, nil
}

// indexStack records that changesets belong to a review. Failures are not
// fatal: the index is an accelerator.
func (s *Service) indexStack(ctx context.Context, r *repo.Repo, id int, ids []object.ID) {
	for _, cs := range ids {
		r.Store.KVUpdate(ctx, r.Name, nsLanded, cs.String(), func([]byte) ([]byte, error) {
			return []byte(key(id)), nil
		})
	}
}

// Get returns a review.
func (s *Service) Get(ctx context.Context, repoName string, id int) (*Review, error) {
	r, err := s.repos.Get(ctx, repoName)
	if err != nil {
		return nil, err
	}
	return s.load(ctx, r, id)
}

// List returns reviews, newest first, optionally filtered by state.
func (s *Service) List(ctx context.Context, repoName, state string, limit int) ([]*Review, error) {
	r, err := s.repos.Get(ctx, repoName)
	if err != nil {
		return nil, err
	}
	keys, err := r.Store.KVList(ctx, r.Name, nsReviews, "")
	if err != nil {
		return nil, err
	}
	ids := make([]int, 0, len(keys))
	for _, k := range keys {
		if n, err := strconv.Atoi(k); err == nil {
			ids = append(ids, n)
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(ids)))
	var out []*Review
	for _, id := range ids {
		rv, err := s.load(ctx, r, id)
		if err != nil {
			return nil, err
		}
		if state != "" && rv.State != state {
			continue
		}
		out = append(out, rv)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

// Upload adds a new revision whose stack ends at head.
func (s *Service) Upload(ctx context.Context, repoName string, id int, pr iam.Principal, headRev string) (*Review, error) {
	r, err := s.repos.Get(ctx, repoName)
	if err != nil {
		return nil, err
	}
	head, err := r.Resolve(ctx, headRev)
	if err != nil {
		return nil, err
	}
	cur, err := s.load(ctx, r, id)
	if err != nil {
		return nil, err
	}
	if cur.State != StateOpen {
		return nil, fmt.Errorf("%w: review %d is %s", ErrInvalid, id, cur.State)
	}
	if cur.Latest().Head == head {
		return cur, nil
	}
	rev, err := s.makeRevision(ctx, r, len(cur.Revisions)+1, head, cur.Target)
	if err != nil {
		return nil, err
	}
	class, _, err := s.interdiff(ctx, r, cur.Latest(), &rev)
	if err != nil {
		return nil, err
	}
	rev.Interdiff = class.String()
	rv, err := s.update(ctx, r, id, func(rv *Review) error {
		if rv.State != StateOpen {
			return fmt.Errorf("%w: review %d is %s", ErrInvalid, id, rv.State)
		}
		rev.Number = len(rv.Revisions) + 1
		rv.Revisions = append(rv.Revisions, rev)
		return nil
	})
	if err == nil {
		s.indexStack(ctx, r, id, rev.Stack)
	}
	return rv, err
}

// Comment adds a comment to the latest revision.
func (s *Service) Comment(ctx context.Context, repoName string, id int, pr iam.Principal, body, path string, line int) (*Review, error) {
	if strings.TrimSpace(body) == "" {
		return nil, fmt.Errorf("%w: empty comment", ErrInvalid)
	}
	r, err := s.repos.Get(ctx, repoName)
	if err != nil {
		return nil, err
	}
	return s.update(ctx, r, id, func(rv *Review) error {
		rv.Comments = append(rv.Comments, Comment{
			ID: len(rv.Comments) + 1, Author: pr.String(), Body: body, Path: object.CleanPath(path),
			Line: line, Revision: rv.Latest().Number, Time: s.now().Unix(),
		})
		return nil
	})
}

// Approve records the principal's approval of the latest revision.
func (s *Service) Approve(ctx context.Context, repoName string, id int, pr iam.Principal) (*Review, error) {
	if pr.Anonymous() {
		return nil, fmt.Errorf("%w: anonymous users cannot approve", ErrInvalid)
	}
	r, err := s.repos.Get(ctx, repoName)
	if err != nil {
		return nil, err
	}
	return s.update(ctx, r, id, func(rv *Review) error {
		if rv.State != StateOpen {
			return fmt.Errorf("%w: review %d is %s", ErrInvalid, id, rv.State)
		}
		n := rv.Latest().Number
		for _, a := range rv.Approvals {
			if a.User == pr.Name && a.Revision == n {
				return nil
			}
		}
		rv.Approvals = append(rv.Approvals, Approval{User: pr.Name, Revision: n, Time: s.now().Unix()})
		return nil
	})
}

// Abandon closes a review without landing it.
func (s *Service) Abandon(ctx context.Context, repoName string, id int, pr iam.Principal) (*Review, error) {
	r, err := s.repos.Get(ctx, repoName)
	if err != nil {
		return nil, err
	}
	return s.update(ctx, r, id, func(rv *Review) error {
		if rv.State != StateOpen {
			return fmt.Errorf("%w: review %d is %s", ErrInvalid, id, rv.State)
		}
		if rv.Author != pr.String() && !s.policy.Can(pr, repoName, "", iam.Admin) {
			return fmt.Errorf("%w: only the author or an admin can abandon a review", ErrInvalid)
		}
		rv.State = StateAbandoned
		return nil
	})
}

// FindByChangeset returns the review that contains (or landed) a changeset.
func (s *Service) FindByChangeset(ctx context.Context, repoName string, cs object.ID) (*Review, error) {
	r, err := s.repos.Get(ctx, repoName)
	if err != nil {
		return nil, err
	}
	b, err := r.Store.KVGet(ctx, r.Name, nsLanded, cs.String())
	if err != nil {
		return nil, fmt.Errorf("no review contains %s: %w", cs.Short(), err)
	}
	id, err := strconv.Atoi(string(b))
	if err != nil {
		return nil, err
	}
	return s.load(ctx, r, id)
}

// ---- semantic classification ----

// FileClass is the classification of one file's change.
type FileClass struct {
	Path  string `json:"path"`
	Class string `json:"class"`
}

func (s *Service) contentAt(ctx context.Context, r *repo.Repo, cs object.ID, p string) ([]byte, bool, error) {
	if cs.IsZero() {
		return nil, false, nil
	}
	data, _, err := r.ReadFileAt(ctx, cs, p)
	if errors.Is(err, store.ErrNotFound) {
		return nil, false, nil
	}
	if err != nil {
		// A directory where a file used to be counts as absent.
		if e, lerr := r.LookupAt(ctx, cs, p); lerr == nil && e.Mode.IsDir() {
			return nil, false, nil
		}
		return nil, false, err
	}
	return data, true, nil
}

func (s *Service) changedPaths(ctx context.Context, r *repo.Repo, base, head object.ID) ([]string, error) {
	bt, err := r.RootTree(ctx, base)
	if err != nil {
		return nil, err
	}
	ht, err := r.RootTree(ctx, head)
	if err != nil {
		return nil, err
	}
	diffs, err := r.DiffTrees(ctx, bt, ht)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(diffs))
	for i, d := range diffs {
		out[i] = d.Path
	}
	return out, nil
}

func (s *Service) classify(ctx context.Context, r *repo.Repo, from, to object.ID, paths []string) (semantic.Class, []FileClass, error) {
	overall := semantic.Identical
	var files []FileClass
	for _, p := range paths {
		a, aok, err := s.contentAt(ctx, r, from, p)
		if err != nil {
			return 0, nil, err
		}
		b, bok, err := s.contentAt(ctx, r, to, p)
		if err != nil {
			return 0, nil, err
		}
		c := semantic.Classify(p, a, b, aok, bok)
		if c == semantic.Identical {
			continue
		}
		files = append(files, FileClass{Path: p, Class: c.String()})
		overall = semantic.Combine(overall, c)
	}
	return overall, files, nil
}

// interdiff classifies what changed between two revisions of a review by
// comparing, for every file either revision touches, its content at the two
// heads.
func (s *Service) interdiff(ctx context.Context, r *repo.Repo, a, b *Revision) (semantic.Class, []FileClass, error) {
	pa, err := s.changedPaths(ctx, r, a.Base, a.Head)
	if err != nil {
		return 0, nil, err
	}
	pb, err := s.changedPaths(ctx, r, b.Base, b.Head)
	if err != nil {
		return 0, nil, err
	}
	set := map[string]bool{}
	for _, p := range append(pa, pb...) {
		set[p] = true
	}
	paths := make([]string, 0, len(set))
	for p := range set {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return s.classify(ctx, r, a.Head, b.Head, paths)
}

// ---- REVIEWERS files ----

// Owners is a parsed REVIEWERS file.
type Owners struct {
	Dir        string   `json:"dir"`
	Principals []string `json:"principals"`
	Require    int      `json:"require"`
	// TrivialAny lets anyone but the author approve trivial changes here.
	TrivialAny bool `json:"trivial_any,omitempty"`
}

// ParseOwners parses a REVIEWERS file. Each non-comment line is a user name,
// "group:NAME", or a directive: "!require N" or "!trivial any".
func ParseOwners(dir string, data []byte) (*Owners, error) {
	o := &Owners{Dir: dir, Require: 1}
	sc := bufio.NewScanner(bytes.NewReader(data))
	for n := 1; sc.Scan(); n++ {
		line := sc.Text()
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		switch f[0] {
		case "!require":
			if len(f) != 2 {
				return nil, fmt.Errorf("REVIEWERS:%d: usage: !require N", n)
			}
			v, err := strconv.Atoi(f[1])
			if err != nil || v < 1 {
				return nil, fmt.Errorf("REVIEWERS:%d: bad count %q", n, f[1])
			}
			o.Require = v
		case "!trivial":
			if len(f) != 2 || (f[1] != "any" && f[1] != "owners") {
				return nil, fmt.Errorf("REVIEWERS:%d: usage: !trivial any|owners", n)
			}
			o.TrivialAny = f[1] == "any"
		default:
			for _, p := range f {
				o.Principals = append(o.Principals, p)
			}
		}
	}
	return o, sc.Err()
}

// ownersFor returns the nearest REVIEWERS file governing path in changeset
// cs, or nil if none applies.
func (s *Service) ownersFor(ctx context.Context, r *repo.Repo, cs object.ID, p string, cache map[string]*Owners) (*Owners, error) {
	dir := p
	for {
		if i := strings.LastIndexByte(dir, '/'); i >= 0 {
			dir = dir[:i]
		} else {
			dir = ""
		}
		if o, ok := cache[dir]; ok {
			if o != nil {
				return o, nil
			}
		} else {
			data, ok, err := s.contentAt(ctx, r, cs, object.JoinPath(dir, "REVIEWERS"))
			if err != nil {
				return nil, err
			}
			var o *Owners
			if ok {
				if o, err = ParseOwners(dir, data); err != nil {
					return nil, err
				}
			}
			cache[dir] = o
			if o != nil {
				return o, nil
			}
		}
		if dir == "" {
			return nil, nil
		}
	}
}

func (s *Service) isOwner(o *Owners, user string) bool {
	for _, p := range o.Principals {
		switch {
		case p == user, p == "user:"+user, p == "*":
			return true
		case strings.HasPrefix(p, "group:"):
			for _, m := range s.policy.Members(p[6:]) {
				if m == user {
					return true
				}
			}
		}
	}
	return false
}

// Requirement is one approval rule a review must satisfy.
type Requirement struct {
	Scope     string   `json:"scope"` // directory of the REVIEWERS file ("" for root / default)
	Owners    []string `json:"owners,omitempty"`
	Need      int      `json:"need"`
	Have      []string `json:"have"`
	Paths     []string `json:"paths"`
	Trivial   bool     `json:"trivial,omitempty"`
	Satisfied bool     `json:"satisfied"`
}

// Status is the evaluated approval state of a review.
type Status struct {
	Review       int           `json:"review"`
	State        string        `json:"state"`
	Revision     int           `json:"revision"`
	Class        string        `json:"class"` // semantic class of the whole change
	Files        []FileClass   `json:"files"`
	Valid        []Approval    `json:"valid_approvals"`
	Stale        []Approval    `json:"stale_approvals,omitempty"`
	Requirements []Requirement `json:"requirements"`
	Approved     bool          `json:"approved"`
	Reasons      []string      `json:"reasons,omitempty"`
}

// carriesOver reports whether an approval of revision n is still valid for
// the latest revision: every later interdiff must be trivial.
func carriesOver(rv *Review, n int) bool {
	for _, rev := range rv.Revisions[n:] {
		switch rev.Interdiff {
		case semantic.Identical.String(), semantic.WhitespaceOnly.String(), semantic.CommentOnly.String():
		default:
			return false
		}
	}
	return true
}

// Evaluate computes a review's approval status.
func (s *Service) Evaluate(ctx context.Context, repoName string, id int) (*Status, error) {
	r, err := s.repos.Get(ctx, repoName)
	if err != nil {
		return nil, err
	}
	rv, err := s.load(ctx, r, id)
	if err != nil {
		return nil, err
	}
	return s.evaluate(ctx, r, rv)
}

func (s *Service) evaluate(ctx context.Context, r *repo.Repo, rv *Review) (*Status, error) {
	latest := rv.Latest()
	st := &Status{Review: rv.ID, State: rv.State, Revision: latest.Number}
	paths, err := s.changedPaths(ctx, r, latest.Base, latest.Head)
	if err != nil {
		return nil, err
	}
	class, files, err := s.classify(ctx, r, latest.Base, latest.Head, paths)
	if err != nil {
		return nil, err
	}
	st.Class, st.Files = class.String(), files
	if st.Files == nil {
		st.Files = []FileClass{}
	}
	trivial := class <= semantic.DocsOnly

	// Approvals by people other than the author, valid for this revision.
	approvers := map[string]bool{}
	for _, a := range rv.Approvals {
		if a.User == rv.Author {
			continue
		}
		if carriesOver(rv, a.Revision) {
			st.Valid = append(st.Valid, a)
			approvers[a.User] = true
		} else {
			st.Stale = append(st.Stale, a)
		}
	}
	if st.Valid == nil {
		st.Valid = []Approval{}
	}

	// Group changed paths by governing REVIEWERS file (evaluated at the new
	// head, so a change cannot weaken its own review requirements without
	// the existing owners noticing: owners files are read at the base too).
	cache := map[string]*Owners{}
	baseCache := map[string]*Owners{}
	groups := map[string]*Requirement{}
	var order []string
	for _, p := range paths {
		o, err := s.ownersFor(ctx, r, latest.Head, p, cache)
		if err != nil {
			return nil, err
		}
		if bo, err := s.ownersFor(ctx, r, latest.Base, p, baseCache); err != nil {
			return nil, err
		} else if bo != nil && (o == nil || bo.Dir != o.Dir || strings.HasSuffix(p, "REVIEWERS")) {
			o = bo // the base's owners govern changes to ownership itself
		}
		scope := ""
		if o != nil {
			scope = o.Dir
		}
		req := groups[scope]
		if req == nil {
			req = &Requirement{Scope: scope, Need: 1}
			if o != nil {
				req.Owners = o.Principals
				req.Need = o.Require
				if trivial && o.TrivialAny {
					req.Trivial = true
					req.Need = 1
				}
			}
			groups[scope] = req
			order = append(order, scope)
			// Count matching approvals.
			for u := range approvers {
				if o == nil || req.Trivial || s.isOwner(o, u) {
					req.Have = append(req.Have, u)
				}
			}
			sort.Strings(req.Have)
		}
		req.Paths = append(req.Paths, p)
	}
	sort.Strings(order)
	st.Approved = rv.State == StateOpen && len(paths) > 0
	if len(paths) == 0 {
		st.Reasons = append(st.Reasons, "the review no longer changes anything")
	}
	for _, scope := range order {
		req := groups[scope]
		if req.Have == nil {
			req.Have = []string{}
		}
		req.Satisfied = len(req.Have) >= req.Need
		if !req.Satisfied {
			st.Approved = false
			where := "/" + scope
			if len(req.Owners) > 0 {
				st.Reasons = append(st.Reasons, fmt.Sprintf("%s needs %d approval(s) from %s, has %d", where, req.Need, strings.Join(req.Owners, ", "), len(req.Have)))
			} else {
				st.Reasons = append(st.Reasons, fmt.Sprintf("%s needs %d approval(s) from someone other than the author", where, req.Need))
			}
		}
		st.Requirements = append(st.Requirements, *req)
	}
	if st.Requirements == nil {
		st.Requirements = []Requirement{}
	}
	if rv.State != StateOpen {
		st.Reasons = append(st.Reasons, "review is "+rv.State)
	}
	return st, nil
}

// LandResult reports a landed review.
type LandResult struct {
	Review *Review          `json:"review"`
	Land   *repo.LandResult `json:"land"`
}

// Land lands an approved review onto its target, rebasing as needed. The
// approval rules are evaluated against the stack as reviewed; landing
// itself only rebases, so it cannot introduce unreviewed content without
// failing on conflicts.
func (s *Service) Land(ctx context.Context, repoName, idStr string, pr iam.Principal) (*LandResult, error) {
	id, err := ParseID(idStr)
	if err != nil {
		return nil, err
	}
	r, err := s.repos.Get(ctx, repoName)
	if err != nil {
		return nil, err
	}
	rv, err := s.load(ctx, r, id)
	if err != nil {
		return nil, err
	}
	if rv.State != StateOpen {
		return nil, fmt.Errorf("%w: review %d is %s", ErrInvalid, id, rv.State)
	}
	if !s.policy.Can(pr, repoName, "", iam.Write) {
		return nil, fmt.Errorf("%w: %s may not land changes", ErrNotApproved, pr)
	}
	st, err := s.evaluate(ctx, r, rv)
	if err != nil {
		return nil, err
	}
	if !st.Approved {
		return nil, fmt.Errorf("%w: %s", ErrNotApproved, strings.Join(st.Reasons, "; "))
	}
	for _, f := range st.Files {
		if !s.policy.Can(pr, repoName, f.Path, iam.Write) {
			return nil, fmt.Errorf("%w: %s may not modify %q", ErrNotApproved, pr, f.Path)
		}
	}
	head := rv.Latest().Head
	res, err := r.Land(ctx, rv.Target, head, s.policy.Ident(pr), nil)
	if err != nil {
		return nil, err
	}
	var landed []object.ID
	if res.Rebase != nil {
		for _, old := range rv.Latest().Stack {
			if nid := res.Rebase.Mapping[old.String()]; !nid.IsZero() {
				landed = append(landed, nid)
			}
		}
	} else {
		landed = rv.Latest().Stack
	}
	out, err := s.update(ctx, r, id, func(x *Review) error {
		x.State = StateLanded
		x.Landed = res.New
		x.LandedStack = landed
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.indexStack(ctx, r, id, landed)
	return &LandResult{Review: out, Land: res}, nil
}
