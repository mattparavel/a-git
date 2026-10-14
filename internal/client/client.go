// Package client is the Go client for the a-git protocol. It is used by the
// CLI, by the cache and daemon tiers to reach their upstream, and by tools
// such as the git bridge.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/mattparavel/a-git/internal/api"
	"github.com/mattparavel/a-git/internal/object"
	"github.com/mattparavel/a-git/internal/repo"
	"github.com/mattparavel/a-git/internal/review"
	"github.com/mattparavel/a-git/internal/store"
)

// Client talks to one a-git endpoint (any tier).
type Client struct {
	base  string
	hc    *http.Client
	Token string
	// Tier is the tier of the caller; it is appended to the X-Agit-Via
	// header so the receiving tier can enforce ordering.
	Tier string
	// User is a self-declared identity, honoured only by servers without
	// IAM (see api.HeaderUser).
	User string
}

// New returns a client for addr, which is an http(s) URL or
// "unix:///path/to/socket".
func New(addr, token string) (*Client, error) {
	c := &Client{Token: token, Tier: api.TierClient}
	switch {
	case strings.HasPrefix(addr, "unix://"):
		sock := strings.TrimPrefix(addr, "unix://")
		tr := &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", sock)
			},
			MaxIdleConnsPerHost: 64,
		}
		c.hc = &http.Client{Transport: tr}
		c.base = "http://agitd"
	case strings.HasPrefix(addr, "http://"), strings.HasPrefix(addr, "https://"):
		c.hc = &http.Client{Transport: &http.Transport{
			Proxy:               http.ProxyFromEnvironment,
			MaxIdleConnsPerHost: 64,
			IdleConnTimeout:     90 * time.Second,
		}}
		c.base = strings.TrimRight(addr, "/")
	default:
		return nil, fmt.Errorf("unsupported address %q (want http://, https:// or unix://)", addr)
	}
	return c, nil
}

// Address returns the base URL.
func (c *Client) Address() string { return c.base }

// HTTP returns the underlying HTTP client.
func (c *Client) HTTP() *http.Client { return c.hc }

// Error is a non-2xx response.
type Error struct {
	Status    int
	Code      string
	Message   string
	Conflicts []repo.Conflict
}

func (e *Error) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("HTTP %d", e.Status)
}

// IsNotFound reports whether err is a 404.
func IsNotFound(err error) bool {
	var e *Error
	return errors.As(err, &e) && e.Status == http.StatusNotFound
}

// IsConflict reports whether err is a merge/rebase conflict and returns the
// conflicting paths.
func IsConflict(err error) ([]repo.Conflict, bool) {
	var e *Error
	if errors.As(err, &e) && e.Code == "conflict" {
		return e.Conflicts, true
	}
	return nil, false
}

// IsCode reports whether err carries the given error code.
func IsCode(err error, code string) bool {
	var e *Error
	return errors.As(err, &e) && e.Code == code
}

// NewRequest builds a request with auth and tier headers.
func (c *Client) NewRequest(ctx context.Context, method, path string, q url.Values, body io.Reader) (*http.Request, error) {
	u := c.base + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return nil, err
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	if c.User != "" {
		req.Header.Set(api.HeaderUser, c.User)
	}
	if c.Tier != "" && c.Tier != api.TierClient {
		req.Header.Set(api.HeaderVia, c.Tier)
	}
	return req, nil
}

// Do sends req and decodes errors.
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		defer resp.Body.Close()
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		var er api.ErrorResponse
		if json.Unmarshal(b, &er) == nil && er.Error != "" {
			return nil, &Error{Status: resp.StatusCode, Code: er.Code, Message: er.Error, Conflicts: er.Conflicts}
		}
		return nil, &Error{Status: resp.StatusCode, Message: strings.TrimSpace(string(b))}
	}
	return resp, nil
}

func (c *Client) doJSON(ctx context.Context, method, path string, q url.Values, in, out interface{}) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := c.NewRequest(ctx, method, path, q, body)
	if err != nil {
		return err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if out == nil {
		io.Copy(io.Discard, resp.Body)
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// Info describes the endpoint's tier.
func (c *Client) Info(ctx context.Context) (*api.InfoResponse, error) {
	var out api.InfoResponse
	return &out, c.doJSON(ctx, "GET", "/v1/info", nil, nil, &out)
}

// Metrics returns the endpoint's metrics.
func (c *Client) Metrics(ctx context.Context) (map[string]interface{}, error) {
	var out map[string]interface{}
	return out, c.doJSON(ctx, "GET", "/v1/metrics", nil, nil, &out)
}

// ListRepos lists repositories.
func (c *Client) ListRepos(ctx context.Context) ([]string, error) {
	var out struct {
		Repos []string `json:"repos"`
	}
	return out.Repos, c.doJSON(ctx, "GET", "/v1/repos", nil, nil, &out)
}

// CreateRepo creates a repository.
func (c *Client) CreateRepo(ctx context.Context, name string) error {
	return c.doJSON(ctx, "POST", "/v1/repos", nil, map[string]string{"name": name}, nil)
}

// Repo returns a client scoped to one repository.
func (c *Client) Repo(name string) *Repo {
	return &Repo{c: c, name: name, prefix: "/v1/repos/" + url.PathEscape(name)}
}

// Repo is a repository-scoped client.
type Repo struct {
	c      *Client
	name   string
	prefix string
}

// Name returns the repository name.
func (r *Repo) Name() string { return r.name }

// Client returns the underlying client.
func (r *Repo) Client() *Client { return r.c }

func (r *Repo) get(ctx context.Context, path string, q url.Values, out interface{}) error {
	return r.c.doJSON(ctx, "GET", r.prefix+path, q, nil, out)
}

func (r *Repo) post(ctx context.Context, path string, in, out interface{}) error {
	return r.c.doJSON(ctx, "POST", r.prefix+path, nil, in, out)
}

// Access returns the caller's permissions.
func (r *Repo) Access(ctx context.Context) (*api.AccessResponse, error) {
	var out api.AccessResponse
	return &out, r.get(ctx, "/access", nil, &out)
}

// ---- objects ----

// GetObject fetches one object in stored form and verifies it.
func (r *Repo) GetObject(ctx context.Context, ref api.ObjectRef) ([]byte, error) {
	q := url.Values{}
	if !ref.CS.IsZero() {
		q.Set("cs", ref.CS.String())
		q.Set("path", ref.Path)
	}
	req, err := r.c.NewRequest(ctx, "GET", r.prefix+"/objects/"+ref.ID.String(), q, nil)
	if err != nil {
		return nil, err
	}
	resp, err := r.c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if _, _, err := object.Verify(ref.ID, b); err != nil {
		return nil, err
	}
	return b, nil
}

// GetObjects streams several objects. fn is called for each object received
// (verified); IDs the endpoint did not return are reported as missing.
// Objects already delivered are not requested again if the stream breaks
// and the call is retried, which makes large fetches resumable.
func (r *Repo) GetObjects(ctx context.Context, refs []api.ObjectRef, fn func(id object.ID, stored []byte) error) (missing []object.ID, err error) {
	pending := refs
	for attempt := 0; len(pending) > 0; attempt++ {
		got := map[object.ID]bool{}
		var miss []object.ID
		err := r.getObjectsOnce(ctx, pending, func(id object.ID, stored []byte) error {
			if stored == nil {
				miss = append(miss, id)
				got[id] = true
				return nil
			}
			got[id] = true
			return fn(id, stored)
		})
		if err == nil {
			return miss, nil
		}
		if ctx.Err() != nil || attempt >= 3 || isPermanent(err) {
			return nil, err
		}
		// Resume with whatever did not arrive.
		var rest []api.ObjectRef
		for _, ref := range pending {
			if !got[ref.ID] {
				rest = append(rest, ref)
			}
		}
		missing = append(missing, miss...)
		pending = rest
		time.Sleep(time.Duration(100*(attempt+1)) * time.Millisecond)
	}
	return missing, nil
}

func isPermanent(err error) bool {
	var e *Error
	if errors.As(err, &e) {
		return e.Status < 500
	}
	return errors.Is(err, object.ErrCorrupt)
}

func (r *Repo) getObjectsOnce(ctx context.Context, refs []api.ObjectRef, fn func(object.ID, []byte) error) error {
	b, _ := json.Marshal(api.GetObjectsRequest{Objects: refs})
	req, err := r.c.NewRequest(ctx, "POST", r.prefix+"/objects/get", nil, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := r.c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	fr := api.NewFrameReader(resp.Body)
	for {
		id, stored, err := fr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if stored != nil {
			if _, _, err := object.Verify(id, stored); err != nil {
				return err
			}
		}
		if err := fn(id, stored); err != nil {
			return err
		}
	}
}

// MissingObjects returns the subset of ids the endpoint does not have.
func (r *Repo) MissingObjects(ctx context.Context, ids []object.ID) ([]object.ID, error) {
	var out api.MissingResponse
	return out.Missing, r.post(ctx, "/objects/missing", api.IDsRequest{IDs: ids}, &out)
}

// StoredObject is an object in stored form.
type StoredObject struct {
	ID     object.ID
	Stored []byte
}

// PutObjects uploads objects.
func (r *Repo) PutObjects(ctx context.Context, objs []StoredObject) error {
	pr, pw := io.Pipe()
	go func() {
		for _, o := range objs {
			if err := api.WriteFrame(pw, o.ID, o.Stored); err != nil {
				pw.CloseWithError(err)
				return
			}
		}
		pw.Close()
	}()
	req, err := r.c.NewRequest(ctx, "POST", r.prefix+"/objects", nil, pr)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", api.ContentTypeObjects)
	resp, err := r.c.Do(req)
	if err != nil {
		return err
	}
	io.Copy(io.Discard, resp.Body)
	return resp.Body.Close()
}

// UploadMissing uploads only the objects the server lacks, in batches.
func (r *Repo) UploadMissing(ctx context.Context, objs []StoredObject) (int, error) {
	uploaded := 0
	const batch = 2000
	for start := 0; start < len(objs); start += batch {
		end := min(start+batch, len(objs))
		chunk := objs[start:end]
		ids := make([]object.ID, len(chunk))
		byID := map[object.ID]StoredObject{}
		for i, o := range chunk {
			ids[i] = o.ID
			byID[o.ID] = o
		}
		missing, err := r.MissingObjects(ctx, ids)
		if err != nil {
			return uploaded, err
		}
		if len(missing) == 0 {
			continue
		}
		send := make([]StoredObject, 0, len(missing))
		for _, id := range missing {
			send = append(send, byID[id])
		}
		if err := r.PutObjects(ctx, send); err != nil {
			return uploaded, err
		}
		uploaded += len(send)
	}
	return uploaded, nil
}

// Snapshot streams every object reachable from rev (under prefix).
func (r *Repo) Snapshot(ctx context.Context, rev, prefix string, fn func(object.ID, []byte) error) (object.ID, error) {
	q := url.Values{"rev": {rev}}
	if prefix != "" {
		q.Set("prefix", prefix)
	}
	req, err := r.c.NewRequest(ctx, "GET", r.prefix+"/snapshot", q, nil)
	if err != nil {
		return object.Zero, err
	}
	resp, err := r.c.Do(req)
	if err != nil {
		return object.Zero, err
	}
	defer resp.Body.Close()
	cs, _ := object.ParseID(resp.Header.Get("X-Agit-Changeset"))
	fr := api.NewFrameReader(resp.Body)
	for {
		id, stored, err := fr.Next()
		if err == io.EOF {
			return cs, nil
		}
		if err != nil {
			return cs, err
		}
		if stored == nil {
			continue
		}
		if err := fn(id, stored); err != nil {
			return cs, err
		}
	}
}

// ---- changesets & refs ----

// GetChangeset fetches a changeset; unredacted changesets are verified
// against their ID.
func (r *Repo) GetChangeset(ctx context.Context, id object.ID) (*object.Changeset, error) {
	req, err := r.c.NewRequest(ctx, "GET", r.prefix+"/changesets/"+id.String(), nil, nil)
	if err != nil {
		return nil, err
	}
	resp, err := r.c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	cs, err := object.VerifyChangeset(id, bytes.TrimSpace(b))
	if err != nil {
		// Redacted views (partial read access) do not hash to their ID.
		return object.DecodeChangeset(b)
	}
	return cs, nil
}

// GetChangesets fetches several changesets.
func (r *Repo) GetChangesets(ctx context.Context, ids []object.ID) (map[object.ID]*object.Changeset, error) {
	var out api.ChangesetsResponse
	if err := r.post(ctx, "/changesets/get", api.IDsRequest{IDs: ids}, &out); err != nil {
		return nil, err
	}
	m := make(map[object.ID]*object.Changeset, len(out.Changesets))
	for k, v := range out.Changesets {
		id, err := object.ParseID(k)
		if err != nil {
			return nil, err
		}
		m[id] = v
	}
	return m, nil
}

// CreateChangeset creates a changeset on the server.
func (r *Repo) CreateChangeset(ctx context.Context, cs *object.Changeset, ref *api.RefMove) (*api.CreateChangesetResponse, error) {
	var out api.CreateChangesetResponse
	return &out, r.post(ctx, "/changesets", api.CreateChangesetRequest{Changeset: *cs, Ref: ref}, &out)
}

// Import bulk-creates changesets (parents first).
func (r *Repo) Import(ctx context.Context, css []object.Changeset) ([]object.ID, error) {
	var out api.ImportResponse
	return out.IDs, r.post(ctx, "/changesets/import", api.ImportRequest{Changesets: css}, &out)
}

// Resolve resolves a revision expression.
func (r *Repo) Resolve(ctx context.Context, rev string) (object.ID, error) {
	var out api.ResolveResponse
	return out.ID, r.get(ctx, "/resolve", url.Values{"rev": {rev}}, &out)
}

// Refs lists refs.
func (r *Repo) Refs(ctx context.Context, prefix string) ([]store.Ref, error) {
	var out api.RefsResponse
	return out.Refs, r.get(ctx, "/refs", url.Values{"prefix": {prefix}}, &out)
}

// UpdateRef performs a compare-and-swap ref update.
func (r *Repo) UpdateRef(ctx context.Context, name string, old, new object.ID) error {
	return r.c.doJSON(ctx, "PUT", r.prefix+"/refs/"+name, nil, api.UpdateRefRequest{Old: old, New: new}, nil)
}

// WAL reads the changeset log.
func (r *Repo) WAL(ctx context.Context, after int64, limit int) ([]store.WALEntry, error) {
	var out api.WALResponse
	q := url.Values{"after": {strconv.FormatInt(after, 10)}, "limit": {strconv.Itoa(limit)}}
	return out.Entries, r.get(ctx, "/wal", q, &out)
}

// RefLog reads ref transitions.
func (r *Repo) RefLog(ctx context.Context, after int64, limit int) ([]store.RefUpdate, error) {
	var out api.RefLogResponse
	q := url.Values{"after": {strconv.FormatInt(after, 10)}, "limit": {strconv.Itoa(limit)}}
	return out.Entries, r.get(ctx, "/reflog", q, &out)
}

// ---- queries ----

// RootTree returns the root tree ID of a changeset.
func (r *Repo) RootTree(ctx context.Context, cs object.ID) (object.ID, error) {
	var out struct {
		Tree object.ID `json:"tree"`
	}
	return out.Tree, r.get(ctx, "/derived/tree/"+cs.String(), nil, &out)
}

// Derived returns a derived value as raw JSON.
func (r *Repo) Derived(ctx context.Context, kind string, cs object.ID) (json.RawMessage, error) {
	var out json.RawMessage
	return out, r.get(ctx, "/derived/"+kind+"/"+cs.String(), nil, &out)
}

// Tree lists a directory (or describes a file) at rev.
func (r *Repo) Tree(ctx context.Context, rev, path string) (*api.TreeResponse, error) {
	var out api.TreeResponse
	return &out, r.get(ctx, "/tree/"+url.PathEscape(rev), url.Values{"path": {path}}, &out)
}

// File reads a file at rev without a checkout.
func (r *Repo) File(ctx context.Context, rev, path string) ([]byte, error) {
	req, err := r.c.NewRequest(ctx, "GET", r.prefix+"/file/"+url.PathEscape(rev), url.Values{"path": {path}}, nil)
	if err != nil {
		return nil, err
	}
	resp, err := r.c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

// LogOptions are query parameters for Log.
type LogOptions struct {
	Rev         string
	Exclude     string
	Path        string
	Limit       int
	FirstParent bool
	Follow      bool
}

// Log lists history.
func (r *Repo) Log(ctx context.Context, o LogOptions) ([]api.LogEntry, error) {
	q := url.Values{"rev": {o.Rev}}
	if o.Exclude != "" {
		q.Set("exclude", o.Exclude)
	}
	if o.Path != "" {
		q.Set("path", o.Path)
	}
	if o.Limit > 0 {
		q.Set("limit", strconv.Itoa(o.Limit))
	}
	if o.FirstParent {
		q.Set("first_parent", "1")
	}
	if o.Follow {
		q.Set("follow", "1")
	}
	var out api.LogResponse
	return out.Entries, r.get(ctx, "/log", q, &out)
}

// History lists the versions of a path.
func (r *Repo) History(ctx context.Context, rev, path string, limit int, follow bool) ([]repo.PathChange, error) {
	q := url.Values{"path": {path}, "limit": {strconv.Itoa(limit)}}
	if follow {
		q.Set("follow", "1")
	}
	var out api.HistoryResponse
	return out.Entries, r.get(ctx, "/history/"+url.PathEscape(rev), q, &out)
}

// Blame attributes the lines of a file.
func (r *Repo) Blame(ctx context.Context, rev, path string) ([]repo.BlameRange, error) {
	var out api.BlameResponse
	return out.Ranges, r.get(ctx, "/blame/"+url.PathEscape(rev), url.Values{"path": {path}}, &out)
}

// DiffOptions are query parameters for Diff.
type DiffOptions struct {
	From, To string
	Paths    []string
	Patch    bool
	Stat     bool
	Context  int
}

// Diff compares two revisions on the server.
func (r *Repo) Diff(ctx context.Context, o DiffOptions) (*api.DiffResponse, error) {
	q := url.Values{"to": {o.To}}
	if o.From != "" {
		q.Set("from", o.From)
	}
	for _, p := range o.Paths {
		q.Add("path", p)
	}
	if o.Patch {
		q.Set("patch", "1")
	}
	if o.Stat {
		q.Set("stat", "1")
	}
	if o.Context > 0 {
		q.Set("context", strconv.Itoa(o.Context))
	}
	var out api.DiffResponse
	return &out, r.get(ctx, "/diff", q, &out)
}

// MergeBases returns the merge bases of a and b.
func (r *Repo) MergeBases(ctx context.Context, a, b string) ([]object.ID, error) {
	var out api.MergeBaseResponse
	return out.Bases, r.get(ctx, "/merge-base", url.Values{"a": {a}, "b": {b}}, &out)
}

// IsAncestor reports whether a is an ancestor of b.
func (r *Repo) IsAncestor(ctx context.Context, a, b string) (bool, error) {
	var out api.AncestorResponse
	return out.Ancestor, r.get(ctx, "/is-ancestor", url.Values{"a": {a}, "b": {b}}, &out)
}

// Search finds paths.
func (r *Repo) Search(ctx context.Context, rev, prefix, pattern string, limit int) ([]repo.SearchResult, error) {
	q := url.Values{"rev": {rev}, "q": {pattern}, "prefix": {prefix}, "limit": {strconv.Itoa(limit)}}
	var out api.SearchResponse
	return out.Results, r.get(ctx, "/search", q, &out)
}

// ---- server-side writes ----

// Merge merges theirs into ours on the server.
func (r *Repo) Merge(ctx context.Context, req api.MergeRequest) (*repo.MergeResult, error) {
	var out repo.MergeResult
	return &out, r.post(ctx, "/merge", req, &out)
}

// Rebase rebases a stack on the server.
func (r *Repo) Rebase(ctx context.Context, req api.RebaseRequest) (*repo.RebaseResult, error) {
	var out repo.RebaseResult
	return &out, r.post(ctx, "/rebase", req, &out)
}

// Land lands head onto ref.
func (r *Repo) Land(ctx context.Context, req api.LandRequest) (*repo.LandResult, error) {
	var out repo.LandResult
	return &out, r.post(ctx, "/land", req, &out)
}

// Revert reverts a changeset.
func (r *Repo) Revert(ctx context.Context, req api.RevertRequest) (object.ID, error) {
	var out api.IDResponse
	return out.ID, r.post(ctx, "/revert", req, &out)
}

// ---- reviews ----

// CreateReview opens a review.
func (r *Repo) CreateReview(ctx context.Context, req review.CreateRequest) (*api.ReviewResponse, error) {
	var out api.ReviewResponse
	return &out, r.post(ctx, "/reviews", req, &out)
}

// Reviews lists reviews.
func (r *Repo) Reviews(ctx context.Context, state string) ([]*review.Review, error) {
	var out api.ReviewsResponse
	return out.Reviews, r.get(ctx, "/reviews", url.Values{"state": {state}}, &out)
}

// Review returns a review and its status.
func (r *Repo) Review(ctx context.Context, id string) (*api.ReviewResponse, error) {
	var out api.ReviewResponse
	return &out, r.get(ctx, "/reviews/"+url.PathEscape(id), nil, &out)
}

// UploadReview adds a revision.
func (r *Repo) UploadReview(ctx context.Context, id, head string) (*api.ReviewResponse, error) {
	var out api.ReviewResponse
	return &out, r.post(ctx, "/reviews/"+url.PathEscape(id)+"/revisions", api.ReviewUploadRequest{Head: head}, &out)
}

// CommentReview adds a comment.
func (r *Repo) CommentReview(ctx context.Context, id string, c api.ReviewCommentRequest) (*api.ReviewResponse, error) {
	var out api.ReviewResponse
	return &out, r.post(ctx, "/reviews/"+url.PathEscape(id)+"/comments", c, &out)
}

// ApproveReview approves the latest revision.
func (r *Repo) ApproveReview(ctx context.Context, id string) (*api.ReviewResponse, error) {
	var out api.ReviewResponse
	return &out, r.post(ctx, "/reviews/"+url.PathEscape(id)+"/approve", struct{}{}, &out)
}

// AbandonReview closes a review.
func (r *Repo) AbandonReview(ctx context.Context, id string) (*api.ReviewResponse, error) {
	var out api.ReviewResponse
	return &out, r.post(ctx, "/reviews/"+url.PathEscape(id)+"/abandon", struct{}{}, &out)
}

// LandReview lands an approved review.
func (r *Repo) LandReview(ctx context.Context, id string) (*review.LandResult, error) {
	var out review.LandResult
	return &out, r.post(ctx, "/reviews/"+url.PathEscape(id)+"/land", struct{}{}, &out)
}

// FindReview finds the review containing a changeset.
func (r *Repo) FindReview(ctx context.Context, cs string) (*api.ReviewResponse, error) {
	var out api.ReviewResponse
	return &out, r.get(ctx, "/landed", url.Values{"changeset": {cs}}, &out)
}
