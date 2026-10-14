package api

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/mattparavel/a-git/internal/iam"
	"github.com/mattparavel/a-git/internal/object"
	"github.com/mattparavel/a-git/internal/repo"
	"github.com/mattparavel/a-git/internal/store"
	"github.com/mattparavel/a-git/internal/textdiff"
)

func (s *Server) routes() {
	s.handle("GET /v1/info", false, s.info)
	s.handle("GET /v1/repos", false, s.listRepos)
	s.handle("POST /v1/repos", false, s.createRepo)
	s.handle("GET /v1/repos/{repo}/access", true, s.access)

	// Objects: immutable, content-addressed, cacheable anywhere.
	s.handle("GET /v1/repos/{repo}/objects/{id}", true, s.getObject)
	s.handle("POST /v1/repos/{repo}/objects/get", true, s.getObjects)
	s.handle("POST /v1/repos/{repo}/objects/missing", true, s.missingObjects)
	s.handle("POST /v1/repos/{repo}/objects", true, s.putObjects)
	s.handle("POST /v1/repos/{repo}/authz", true, s.authz)
	s.handle("GET /v1/repos/{repo}/snapshot", true, s.snapshot)

	// Changesets and refs.
	s.handle("GET /v1/repos/{repo}/changesets/{id}", true, s.getChangeset)
	s.handle("POST /v1/repos/{repo}/changesets/get", true, s.getChangesets)
	s.handle("POST /v1/repos/{repo}/changesets", true, s.createChangeset)
	s.handle("POST /v1/repos/{repo}/changesets/import", true, s.importChangesets)
	s.handle("GET /v1/repos/{repo}/resolve", true, s.resolveRev)
	s.handle("GET /v1/repos/{repo}/refs", true, s.listRefs)
	s.handle("GET /v1/repos/{repo}/refs/{name...}", true, s.getRef)
	s.handle("PUT /v1/repos/{repo}/refs/{name...}", true, s.updateRef)
	s.handle("GET /v1/repos/{repo}/wal", true, s.wal)
	s.handle("GET /v1/repos/{repo}/reflog", true, s.reflog)

	// Read-side queries answered without a checkout.
	s.handle("GET /v1/repos/{repo}/derived/{kind}/{id}", true, s.derived)
	s.handle("GET /v1/repos/{repo}/tree/{id}", true, s.tree)
	s.handle("GET /v1/repos/{repo}/file/{id}", true, s.file)
	s.handle("GET /v1/repos/{repo}/log", true, s.log)
	s.handle("GET /v1/repos/{repo}/history/{id}", true, s.history)
	s.handle("GET /v1/repos/{repo}/blame/{id}", true, s.blame)
	s.handle("GET /v1/repos/{repo}/diff", true, s.diff)
	s.handle("GET /v1/repos/{repo}/merge-base", true, s.mergeBase)
	s.handle("GET /v1/repos/{repo}/is-ancestor", true, s.isAncestor)
	s.handle("GET /v1/repos/{repo}/search", true, s.search)

	// Server-side history editing.
	s.handle("POST /v1/repos/{repo}/merge", true, s.merge)
	s.handle("POST /v1/repos/{repo}/rebase", true, s.rebase)
	s.handle("POST /v1/repos/{repo}/land", true, s.land)
	s.handle("POST /v1/repos/{repo}/revert", true, s.revert)

	s.reviewRoutes()
}

func (s *Server) info(w http.ResponseWriter, r *http.Request, c *reqCtx) error {
	writeJSON(w, http.StatusOK, InfoResponse{Tier: TierServer, Version: Version, IAM: s.policy != nil})
	return nil
}

func (s *Server) listRepos(w http.ResponseWriter, r *http.Request, c *reqCtx) error {
	names, err := s.repos.Store.ListRepos(r.Context())
	if err != nil {
		return err
	}
	var visible []string
	for _, n := range names {
		if s.policy.CanSeeDir(c.pr, n, "") {
			visible = append(visible, n)
		}
	}
	writeJSON(w, http.StatusOK, map[string][]string{"repos": visible})
	return nil
}

func (s *Server) createRepo(w http.ResponseWriter, r *http.Request, c *reqCtx) error {
	var req struct {
		Name string `json:"name"`
	}
	if err := readJSON(r, &req); err != nil {
		return err
	}
	if !store.ValidRepoName(req.Name) {
		return errBadRequest("invalid repository name %q", req.Name)
	}
	if s.policy.Level(c.pr, req.Name, "") < iam.Admin {
		return errForbidden("creating repository %q requires admin", req.Name)
	}
	if _, err := s.repos.Create(r.Context(), req.Name); err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, map[string]string{"name": req.Name})
	return nil
}

func (s *Server) access(w http.ResponseWriter, r *http.Request, c *reqCtx) error {
	mutable(w)
	writeJSON(w, http.StatusOK, AccessFromLevel(s.policy, c.pr, c.repo.Name))
	return nil
}

// ---- objects ----

func parseID(s string) (object.ID, error) {
	id, err := object.ParseID(s)
	if err != nil {
		return id, errBadRequest("%v", err)
	}
	return id, nil
}

// authorizeObject checks that a principal who cannot read the whole
// repository may read an object, using the location hint supplied with the
// request: the object must really be at that path in that changeset, and the
// path must be readable.
func (s *Server) authorizeObject(r *http.Request, c *reqCtx, ref ObjectRef) bool {
	if c.filter == nil {
		return true
	}
	if ref.CS.IsZero() {
		return false
	}
	root, err := c.repo.RootTree(r.Context(), ref.CS)
	if err != nil {
		return false
	}
	p := object.CleanPath(ref.Path)
	e, err := c.repo.Lookup(r.Context(), root, p)
	if err != nil || e.ID != ref.ID {
		return false
	}
	if e.Mode.IsDir() {
		return s.policy.CanSeeDir(c.pr, c.repo.Name, p)
	}
	return c.canRead(p)
}

func (s *Server) getObject(w http.ResponseWriter, r *http.Request, c *reqCtx) error {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		return err
	}
	ref := ObjectRef{ID: id, Path: r.URL.Query().Get("path")}
	if cs := r.URL.Query().Get("cs"); cs != "" {
		if ref.CS, err = parseID(cs); err != nil {
			return err
		}
	}
	if !s.authorizeObject(r, c, ref) {
		return errForbidden("object %s is not readable without a valid location hint", id.Short())
	}
	stored, err := c.repo.GetStored(r.Context(), id)
	if err != nil {
		return err
	}
	immutable(w, c)
	w.Header().Set("ETag", `"`+id.String()+`"`)
	w.Header().Set(HeaderObjectKind, object.KindOf(stored).String())
	w.Header().Set("Content-Type", "application/octet-stream")
	// ServeContent handles Range requests, so large objects can be resumed.
	http.ServeContent(w, r, "", modTime, strings.NewReader(string(stored)))
	return nil
}

func (s *Server) getObjects(w http.ResponseWriter, r *http.Request, c *reqCtx) error {
	var req GetObjectsRequest
	if err := readJSON(r, &req); err != nil {
		return err
	}
	w.Header().Set("Content-Type", ContentTypeObjects)
	immutable(w, c)
	for _, ref := range req.Objects {
		var stored []byte
		if s.authorizeObject(r, c, ref) {
			b, err := c.repo.GetStored(r.Context(), ref.ID)
			if err != nil && !errors.Is(err, store.ErrNotFound) {
				return err
			}
			stored = b
		}
		if err := WriteFrame(w, ref.ID, stored); err != nil {
			return nil // client went away
		}
	}
	return nil
}

func (s *Server) missingObjects(w http.ResponseWriter, r *http.Request, c *reqCtx) error {
	var req IDsRequest
	if err := readJSON(r, &req); err != nil {
		return err
	}
	resp := MissingResponse{Missing: []object.ID{}}
	for _, id := range req.IDs {
		ok, err := c.repo.Store.HasObject(r.Context(), c.repo.Name, id)
		if err != nil {
			return err
		}
		if !ok {
			resp.Missing = append(resp.Missing, id)
		}
	}
	writeJSON(w, http.StatusOK, resp)
	return nil
}

func (s *Server) putObjects(w http.ResponseWriter, r *http.Request, c *reqCtx) error {
	if !s.policy.Can(c.pr, c.repo.Name, "", iam.Write) && !s.anyWrite(c) {
		return errForbidden("uploading objects requires write access")
	}
	fr := NewFrameReader(http.MaxBytesReader(nil, r.Body, 4<<30))
	n := 0
	for {
		id, stored, err := fr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return errBadRequest("%v", err)
		}
		if stored == nil {
			continue
		}
		if err := c.repo.PutStored(r.Context(), id, stored); err != nil {
			if errors.Is(err, object.ErrCorrupt) {
				return errBadRequest("%v", err)
			}
			return err
		}
		n++
	}
	writeJSON(w, http.StatusOK, map[string]int{"stored": n})
	return nil
}

// anyWrite reports whether the principal can write somewhere in the repo,
// which is enough to upload blobs (paths are checked at changeset time).
func (s *Server) anyWrite(c *reqCtx) bool {
	for _, pre := range s.policy.RestrictedPrefixes(c.repo.Name) {
		if s.policy.Can(c.pr, c.repo.Name, pre, iam.Write) {
			return true
		}
	}
	return false
}

func (s *Server) authz(w http.ResponseWriter, r *http.Request, c *reqCtx) error {
	var req GetObjectsRequest
	if err := readJSON(r, &req); err != nil {
		return err
	}
	resp := AuthzResponse{Allowed: make([]bool, len(req.Objects))}
	for i, ref := range req.Objects {
		resp.Allowed[i] = s.authorizeObject(r, c, ref)
	}
	mutable(w)
	writeJSON(w, http.StatusOK, resp)
	return nil
}

// snapshot streams every tree and blob reachable from a revision (optionally
// under a prefix). Daemons use it to seed their object store.
func (s *Server) snapshot(w http.ResponseWriter, r *http.Request, c *reqCtx) error {
	rev := r.URL.Query().Get("rev")
	if rev == "" {
		rev = "main"
	}
	cs, err := s.resolve(r.Context(), c, rev)
	if err != nil {
		return err
	}
	root, err := c.repo.RootTree(r.Context(), cs)
	if err != nil {
		return err
	}
	prefix := object.CleanPath(r.URL.Query().Get("prefix"))
	w.Header().Set("Content-Type", ContentTypeObjects)
	w.Header().Set("X-Agit-Changeset", cs.String())
	mutable(w)
	seen := map[object.ID]bool{}
	emit := func(id object.ID) error {
		if seen[id] {
			return nil
		}
		seen[id] = true
		stored, err := c.repo.GetStored(r.Context(), id)
		if err != nil {
			return err
		}
		return WriteFrame(w, id, stored)
	}
	var walk func(dir string, id object.ID) error
	walk = func(dir string, id object.ID) error {
		if !s.policy.CanSeeDir(c.pr, c.repo.Name, dir) {
			return nil
		}
		if err := emit(id); err != nil {
			return err
		}
		t, err := c.repo.ReadTree(r.Context(), id)
		if err != nil {
			return err
		}
		for _, e := range t.Entries {
			p := object.JoinPath(dir, e.Name)
			if prefix != "" && !object.HasPathPrefix(p, prefix) && !object.HasPathPrefix(prefix, p) {
				continue
			}
			if e.Mode.IsDir() {
				if err := walk(p, e.ID); err != nil {
					return err
				}
			} else if c.canRead(p) {
				if err := emit(e.ID); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return walk("", root)
}

// ---- changesets ----

// redact hides changes to unreadable paths from principals with partial
// read access. Redacted changesets no longer hash to their ID.
func redact(c *reqCtx, cs *object.Changeset) *object.Changeset {
	if c.filter == nil {
		return cs
	}
	cp := *cs
	cp.Changes = nil
	hidden := 0
	for _, ch := range cs.Changes {
		if c.canRead(ch.Path) {
			cp.Changes = append(cp.Changes, ch)
		} else {
			hidden++
		}
	}
	if hidden > 0 {
		cp.Extra = map[string]string{}
		for k, v := range cs.Extra {
			cp.Extra[k] = v
		}
		cp.Extra["redacted"] = fmt.Sprint(hidden)
	}
	return &cp
}

func (s *Server) getChangeset(w http.ResponseWriter, r *http.Request, c *reqCtx) error {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		return err
	}
	cs, err := c.repo.GetChangeset(r.Context(), id)
	if err != nil {
		return err
	}
	immutable(w, c)
	if c.filter == nil {
		// The canonical encoding, so clients can verify the ID.
		raw, err := c.repo.Store.GetChangeset(r.Context(), c.repo.Name, id)
		if err != nil {
			return err
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(raw)
		return nil
	}
	writeJSON(w, http.StatusOK, redact(c, cs))
	return nil
}

func (s *Server) getChangesets(w http.ResponseWriter, r *http.Request, c *reqCtx) error {
	var req IDsRequest
	if err := readJSON(r, &req); err != nil {
		return err
	}
	resp := ChangesetsResponse{Changesets: map[string]*object.Changeset{}}
	for _, id := range req.IDs {
		cs, err := c.repo.GetChangeset(r.Context(), id)
		if errors.Is(err, store.ErrNotFound) {
			continue
		}
		if err != nil {
			return err
		}
		resp.Changesets[id.String()] = redact(c, cs)
	}
	immutable(w, c)
	writeJSON(w, http.StatusOK, resp)
	return nil
}

func (s *Server) checkWritePaths(c *reqCtx, changes []object.Change) error {
	for _, ch := range changes {
		if !s.policy.Can(c.pr, c.repo.Name, ch.Path, iam.Write) {
			return errForbidden("%s may not modify %q", c.pr, ch.Path)
		}
	}
	return nil
}

func (s *Server) createChangeset(w http.ResponseWriter, r *http.Request, c *reqCtx) error {
	var req CreateChangesetRequest
	if err := readJSON(r, &req); err != nil {
		return err
	}
	if err := s.checkWritePaths(c, req.Changeset.Changes); err != nil {
		return err
	}
	if len(req.Changeset.Changes) == 0 && !s.policy.Can(c.pr, c.repo.Name, "", iam.Write) && !s.anyWrite(c) {
		return errForbidden("%s may not create changesets", c.pr)
	}
	if req.Ref != nil {
		if err := s.checkRefWrite(c, req.Ref.Name); err != nil {
			return err
		}
	}
	id, err := c.repo.CreateChangeset(r.Context(), &req.Changeset)
	if err != nil {
		return err
	}
	if req.Ref != nil {
		if err := c.repo.MoveRef(r.Context(), req.Ref.Name, req.Ref.Old, id, c.pr.String()); err != nil {
			return err
		}
	}
	tree, err := c.repo.RootTree(r.Context(), id)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, CreateChangesetResponse{ID: id, Tree: tree})
	return nil
}

func (s *Server) importChangesets(w http.ResponseWriter, r *http.Request, c *reqCtx) error {
	var req ImportRequest
	if err := readJSON(r, &req); err != nil {
		return err
	}
	resp := ImportResponse{IDs: make([]object.ID, 0, len(req.Changesets))}
	for i := range req.Changesets {
		cs := &req.Changesets[i]
		if err := s.checkWritePaths(c, cs.Changes); err != nil {
			return err
		}
		id, err := c.repo.CreateChangeset(r.Context(), cs)
		if err != nil {
			return fmt.Errorf("changeset %d of %d: %w", i+1, len(req.Changesets), err)
		}
		resp.IDs = append(resp.IDs, id)
	}
	writeJSON(w, http.StatusOK, resp)
	return nil
}

func (s *Server) resolveRev(w http.ResponseWriter, r *http.Request, c *reqCtx) error {
	rev := r.URL.Query().Get("rev")
	id, err := s.resolve(r.Context(), c, rev)
	if err != nil {
		return err
	}
	if isFullID(rev) {
		immutable(w, c)
	} else {
		mutable(w)
	}
	writeJSON(w, http.StatusOK, ResolveResponse{ID: id})
	return nil
}

func (s *Server) listRefs(w http.ResponseWriter, r *http.Request, c *reqCtx) error {
	refs, err := c.repo.Store.ListRefs(r.Context(), c.repo.Name, r.URL.Query().Get("prefix"))
	if err != nil {
		return err
	}
	if refs == nil {
		refs = []store.Ref{}
	}
	mutable(w)
	writeJSON(w, http.StatusOK, RefsResponse{Refs: refs})
	return nil
}

func (s *Server) getRef(w http.ResponseWriter, r *http.Request, c *reqCtx) error {
	name := r.PathValue("name")
	id, err := c.repo.Ref(r.Context(), name)
	if err != nil {
		return err
	}
	mutable(w)
	writeJSON(w, http.StatusOK, store.Ref{Name: name, ID: id})
	return nil
}

func (s *Server) checkRefWrite(c *reqCtx, name string) error {
	if !s.policy.Can(c.pr, c.repo.Name, "", iam.Write) {
		return errForbidden("%s may not update refs in %q", c.pr, c.repo.Name)
	}
	if refProtected(s.protected, name) && !s.policy.Can(c.pr, c.repo.Name, "", iam.Admin) {
		return errForbidden("ref %q is protected: land an approved review instead", name)
	}
	return nil
}

func (s *Server) updateRef(w http.ResponseWriter, r *http.Request, c *reqCtx) error {
	name := r.PathValue("name")
	var req UpdateRefRequest
	if err := readJSON(r, &req); err != nil {
		return err
	}
	if err := s.checkRefWrite(c, name); err != nil {
		return err
	}
	if err := c.repo.MoveRef(r.Context(), name, req.Old, req.New, c.pr.String()); err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, store.Ref{Name: name, ID: req.New})
	return nil
}

func (s *Server) wal(w http.ResponseWriter, r *http.Request, c *reqCtx) error {
	entries, err := c.repo.Store.ReadWAL(r.Context(), c.repo.Name, int64(queryInt(r, "after", 0)), queryInt(r, "limit", 1000))
	if err != nil {
		return err
	}
	if entries == nil {
		entries = []store.WALEntry{}
	}
	mutable(w)
	writeJSON(w, http.StatusOK, WALResponse{Entries: entries})
	return nil
}

func (s *Server) reflog(w http.ResponseWriter, r *http.Request, c *reqCtx) error {
	entries, err := c.repo.Store.ReadRefLog(r.Context(), c.repo.Name, int64(queryInt(r, "after", 0)), queryInt(r, "limit", 1000))
	if err != nil {
		return err
	}
	if entries == nil {
		entries = []store.RefUpdate{}
	}
	mutable(w)
	writeJSON(w, http.StatusOK, RefLogResponse{Entries: entries})
	return nil
}

// ---- read-side queries ----

// revParam resolves a path-segment revision and sets cache headers: full
// IDs make the response immutable.
func (s *Server) revParam(w http.ResponseWriter, r *http.Request, c *reqCtx, rev string) (object.ID, error) {
	id, err := s.resolve(r.Context(), c, rev)
	if err != nil {
		return id, err
	}
	if isFullID(rev) {
		immutable(w, c)
	} else {
		mutable(w)
	}
	return id, nil
}

func (s *Server) derived(w http.ResponseWriter, r *http.Request, c *reqCtx) error {
	kind := r.PathValue("kind")
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		return err
	}
	// The root tree ID and graph index reveal no file content (objects are
	// still authorized individually), and partial readers need the root tree
	// to start a sparse checkout.
	if c.filter != nil && kind != repo.DerivedGraph && kind != repo.DerivedTree {
		return errForbidden("derived data %q requires full read access", kind)
	}
	var v interface{}
	switch kind {
	case repo.DerivedTree:
		t, err := c.repo.RootTree(r.Context(), id)
		if err != nil {
			return err
		}
		v = map[string]object.ID{"tree": t}
	case repo.DerivedGraph:
		if v, err = c.repo.Graph(r.Context(), id); err != nil {
			return err
		}
	case repo.DerivedHistory:
		h, err := c.repo.HistRoot(r.Context(), id)
		if err != nil {
			return err
		}
		v = map[string]object.ID{"root": h}
	case repo.DerivedCopies:
		if v, err = c.repo.Copies(r.Context(), id); err != nil {
			return err
		}
	default:
		return &httpError{http.StatusNotFound, "not_found", fmt.Sprintf("unknown derived data kind %q", kind)}
	}
	immutable(w, c)
	writeJSON(w, http.StatusOK, v)
	return nil
}

func (s *Server) tree(w http.ResponseWriter, r *http.Request, c *reqCtx) error {
	rev := r.PathValue("id")
	cs, err := s.revParam(w, r, c, rev)
	if err != nil {
		return err
	}
	p := object.CleanPath(r.URL.Query().Get("path"))
	if !s.policy.CanSeeDir(c.pr, c.repo.Name, p) && !c.canRead(p) {
		return errForbidden("no read access to %q", p)
	}
	e, err := c.repo.LookupAt(r.Context(), cs, p)
	if err != nil {
		return err
	}
	resp := TreeResponse{Changeset: cs, Path: p, Mode: e.Mode, ID: e.ID}
	if e.Mode.IsDir() {
		t, err := c.repo.ReadTree(r.Context(), e.ID)
		if err != nil {
			return err
		}
		resp.Entries = []TreeEntry{}
		for _, te := range t.Entries {
			cp := object.JoinPath(p, te.Name)
			if te.Mode.IsDir() && !s.policy.CanSeeDir(c.pr, c.repo.Name, cp) || !te.Mode.IsDir() && !c.canRead(cp) {
				continue
			}
			resp.Entries = append(resp.Entries, TreeEntry{Name: te.Name, Path: cp, Mode: te.Mode, ID: te.ID})
		}
	}
	writeJSON(w, http.StatusOK, resp)
	return nil
}

func (s *Server) file(w http.ResponseWriter, r *http.Request, c *reqCtx) error {
	cs, err := s.revParam(w, r, c, r.PathValue("id"))
	if err != nil {
		return err
	}
	p := object.CleanPath(r.URL.Query().Get("path"))
	if !c.canRead(p) {
		return errForbidden("no read access to %q", p)
	}
	data, mode, err := c.repo.ReadFileAt(r.Context(), cs, p)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("X-Agit-Mode", mode.String())
	w.Write(data)
	return nil
}

func (s *Server) log(w http.ResponseWriter, r *http.Request, c *reqCtx) error {
	q := r.URL.Query()
	rev := q.Get("rev")
	if rev == "" {
		rev = "main"
	}
	head, err := s.resolve(r.Context(), c, rev)
	if err != nil {
		return err
	}
	opts := repo.LogOptions{
		Limit:       queryInt(r, "limit", 50),
		FirstParent: queryBool(r, "first_parent"),
		Path:        object.CleanPath(q.Get("path")),
		Follow:      queryBool(r, "follow"),
	}
	if opts.Limit > 10000 {
		opts.Limit = 10000
	}
	if opts.Path != "" && !c.canRead(opts.Path) && !s.policy.CanSeeDir(c.pr, c.repo.Name, opts.Path) {
		return errForbidden("no read access to %q", opts.Path)
	}
	cacheable := isFullID(rev)
	if ex := q.Get("exclude"); ex != "" {
		if opts.Exclude, err = s.resolve(r.Context(), c, ex); err != nil {
			return err
		}
		cacheable = cacheable && isFullID(ex)
	}
	ids, err := c.repo.Log(r.Context(), head, opts)
	if err != nil {
		return err
	}
	resp := LogResponse{Entries: make([]LogEntry, 0, len(ids))}
	for _, id := range ids {
		cs, err := c.repo.GetChangeset(r.Context(), id)
		if err != nil {
			return err
		}
		resp.Entries = append(resp.Entries, LogEntry{ID: id, Changeset: redact(c, cs)})
	}
	if cacheable {
		immutable(w, c)
	} else {
		mutable(w)
	}
	writeJSON(w, http.StatusOK, resp)
	return nil
}

func (s *Server) history(w http.ResponseWriter, r *http.Request, c *reqCtx) error {
	cs, err := s.revParam(w, r, c, r.PathValue("id"))
	if err != nil {
		return err
	}
	p := object.CleanPath(r.URL.Query().Get("path"))
	if !c.canRead(p) {
		return errForbidden("no read access to %q", p)
	}
	h, err := c.repo.PathHistory(r.Context(), cs, p, queryInt(r, "limit", 100), queryBool(r, "follow"))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, HistoryResponse{Entries: h})
	return nil
}

func (s *Server) blame(w http.ResponseWriter, r *http.Request, c *reqCtx) error {
	cs, err := s.revParam(w, r, c, r.PathValue("id"))
	if err != nil {
		return err
	}
	p := object.CleanPath(r.URL.Query().Get("path"))
	if !c.canRead(p) {
		return errForbidden("no read access to %q", p)
	}
	ranges, err := c.repo.Blame(r.Context(), cs, p)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, BlameResponse{Path: p, Ranges: ranges})
	return nil
}

func (s *Server) diff(w http.ResponseWriter, r *http.Request, c *reqCtx) error {
	q := r.URL.Query()
	toRev := q.Get("to")
	to, err := s.resolve(r.Context(), c, toRev)
	if err != nil {
		return err
	}
	fromRev := q.Get("from")
	var from object.ID
	if fromRev == "" {
		// Default: the first parent of "to" (what the changeset changed).
		cs, err := c.repo.GetChangeset(r.Context(), to)
		if err != nil {
			return err
		}
		from = cs.FirstParent()
	} else if from, err = s.resolve(r.Context(), c, fromRev); err != nil {
		return err
	}
	fromTree, err := c.repo.RootTree(r.Context(), from)
	if err != nil {
		return err
	}
	toTree, err := c.repo.RootTree(r.Context(), to)
	if err != nil {
		return err
	}
	var prefixes []string
	for _, p := range q["path"] {
		if p = object.CleanPath(p); p != "" {
			prefixes = append(prefixes, p)
		}
	}
	diffs, err := c.repo.DiffTrees(r.Context(), fromTree, toTree, prefixes...)
	if err != nil {
		return err
	}
	patch := queryBool(r, "patch")
	context := queryInt(r, "context", 3)
	resp := DiffResponse{From: from, To: to, Files: []DiffFile{}}
	for _, d := range diffs {
		if !c.canRead(d.Path) {
			continue
		}
		f := DiffFile{FileDiff: d, Status: d.Status()}
		if patch || queryBool(r, "stat") {
			var a, b []byte
			if !d.OldBlob.IsZero() {
				if a, err = c.repo.ReadBlob(r.Context(), d.OldBlob); err != nil {
					return err
				}
			}
			if !d.NewBlob.IsZero() {
				if b, err = c.repo.ReadBlob(r.Context(), d.NewBlob); err != nil {
					return err
				}
			}
			if textdiff.IsBinary(a) || textdiff.IsBinary(b) {
				f.Binary = true
			} else {
				al, bl := textdiff.SplitLines(a), textdiff.SplitLines(b)
				f.Added, f.Deleted = textdiff.Stats(textdiff.Lines(al, bl))
				if patch {
					aName, bName := "a/"+d.Path, "b/"+d.Path
					if d.OldBlob.IsZero() {
						aName = "/dev/null"
					}
					if d.NewBlob.IsZero() {
						bName = "/dev/null"
					}
					f.Patch = textdiff.Unified(aName, bName, al, bl, context)
				}
			}
		}
		resp.Files = append(resp.Files, f)
	}
	if isFullID(toRev) && (fromRev == "" || isFullID(fromRev)) {
		immutable(w, c)
	} else {
		mutable(w)
	}
	writeJSON(w, http.StatusOK, resp)
	return nil
}

func (s *Server) twoRevs(w http.ResponseWriter, r *http.Request, c *reqCtx) (object.ID, object.ID, error) {
	q := r.URL.Query()
	a, err := s.resolve(r.Context(), c, q.Get("a"))
	if err != nil {
		return a, a, err
	}
	b, err := s.resolve(r.Context(), c, q.Get("b"))
	if err != nil {
		return a, b, err
	}
	if isFullID(q.Get("a")) && isFullID(q.Get("b")) {
		immutable(w, c)
	} else {
		mutable(w)
	}
	return a, b, nil
}

func (s *Server) mergeBase(w http.ResponseWriter, r *http.Request, c *reqCtx) error {
	a, b, err := s.twoRevs(w, r, c)
	if err != nil {
		return err
	}
	bases, err := c.repo.MergeBases(r.Context(), a, b)
	if err != nil {
		return err
	}
	if bases == nil {
		bases = []object.ID{}
	}
	writeJSON(w, http.StatusOK, MergeBaseResponse{Bases: bases})
	return nil
}

func (s *Server) isAncestor(w http.ResponseWriter, r *http.Request, c *reqCtx) error {
	a, b, err := s.twoRevs(w, r, c)
	if err != nil {
		return err
	}
	ok, err := c.repo.IsAncestor(r.Context(), a, b)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, AncestorResponse{Ancestor: ok})
	return nil
}

func (s *Server) search(w http.ResponseWriter, r *http.Request, c *reqCtx) error {
	q := r.URL.Query()
	rev := q.Get("rev")
	if rev == "" {
		rev = "main"
	}
	cs, err := s.revParam(w, r, c, rev)
	if err != nil {
		return err
	}
	res, err := c.repo.SearchPaths(r.Context(), cs, object.CleanPath(q.Get("prefix")), q.Get("q"), queryInt(r, "limit", 1000))
	if err != nil {
		return err
	}
	out := []repo.SearchResult{}
	for _, m := range res {
		if c.canRead(m.Path) {
			out = append(out, m)
		}
	}
	writeJSON(w, http.StatusOK, SearchResponse{Results: out})
	return nil
}

// ---- server-side history editing ----

func (s *Server) merge(w http.ResponseWriter, r *http.Request, c *reqCtx) error {
	var req MergeRequest
	if err := readJSON(r, &req); err != nil {
		return err
	}
	ours, err := s.resolve(r.Context(), c, req.Ours)
	if err != nil {
		return err
	}
	theirs, err := s.resolve(r.Context(), c, req.Theirs)
	if err != nil {
		return err
	}
	if req.Ref != nil {
		if err := s.checkRefWrite(c, req.Ref.Name); err != nil {
			return err
		}
	} else if !s.policy.Can(c.pr, c.repo.Name, "", iam.Write) && !s.anyWrite(c) {
		return errForbidden("%s may not create changesets", c.pr)
	}
	res, err := c.repo.Merge(r.Context(), ours, theirs, repo.MergeOptions{
		Author: s.ident(c, req.Author), Message: req.Message, AllowConflicts: req.AllowConflicts,
	})
	if err != nil {
		return err
	}
	if !res.ID.IsZero() && res.ID != ours {
		cs, err := c.repo.GetChangeset(r.Context(), res.ID)
		if err != nil {
			return err
		}
		if !res.FastForward {
			if err := s.checkWritePaths(c, cs.Changes); err != nil {
				return err
			}
		}
	}
	if req.Ref != nil {
		if err := c.repo.MoveRef(r.Context(), req.Ref.Name, req.Ref.Old, res.ID, c.pr.String()); err != nil {
			return err
		}
	}
	writeJSON(w, http.StatusOK, res)
	return nil
}

func (s *Server) rebase(w http.ResponseWriter, r *http.Request, c *reqCtx) error {
	var req RebaseRequest
	if err := readJSON(r, &req); err != nil {
		return err
	}
	head, err := s.resolve(r.Context(), c, req.Head)
	if err != nil {
		return err
	}
	onto, err := s.resolve(r.Context(), c, req.Onto)
	if err != nil {
		return err
	}
	base := onto
	if req.Base != "" {
		if base, err = s.resolve(r.Context(), c, req.Base); err != nil {
			return err
		}
	}
	stack, err := c.repo.Stack(r.Context(), head, base)
	if err != nil {
		return err
	}
	for _, id := range stack {
		cs, err := c.repo.GetChangeset(r.Context(), id)
		if err != nil {
			return err
		}
		if err := s.checkWritePaths(c, cs.Changes); err != nil {
			return err
		}
	}
	res, err := c.repo.RebaseStack(r.Context(), stack, onto, s.ident(c, ""))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, res)
	return nil
}

func (s *Server) land(w http.ResponseWriter, r *http.Request, c *reqCtx) error {
	var req LandRequest
	if err := readJSON(r, &req); err != nil {
		return err
	}
	if req.Ref == "" {
		req.Ref = "main"
	}
	if req.Review != "" {
		res, err := s.reviews.Land(r.Context(), c.repo.Name, req.Review, c.pr)
		if err != nil {
			return err
		}
		writeJSON(w, http.StatusOK, res)
		return nil
	}
	if !s.policy.Can(c.pr, c.repo.Name, "", iam.Write) {
		return errForbidden("%s may not land on %q", c.pr, req.Ref)
	}
	if refProtected(s.protected, req.Ref) && !s.policy.Can(c.pr, c.repo.Name, "", iam.Admin) {
		return errForbidden("ref %q is protected: create a review and land it once approved", req.Ref)
	}
	head, err := s.resolve(r.Context(), c, req.Head)
	if err != nil {
		return err
	}
	res, err := c.repo.Land(r.Context(), req.Ref, head, s.ident(c, ""), func(newHead object.ID) error {
		return s.checkStackWrite(r, c, req.Ref, newHead)
	})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, res)
	return nil
}

// checkStackWrite verifies write permission for every path changed between
// the current tip of ref and newHead.
func (s *Server) checkStackWrite(r *http.Request, c *reqCtx, ref string, newHead object.ID) error {
	if s.policy == nil {
		return nil
	}
	tip, err := c.repo.Ref(r.Context(), ref)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return err
	}
	stack, err := c.repo.Stack(r.Context(), newHead, tip)
	if err != nil {
		return err
	}
	for _, id := range stack {
		cs, err := c.repo.GetChangeset(r.Context(), id)
		if err != nil {
			return err
		}
		if err := s.checkWritePaths(c, cs.Changes); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) revert(w http.ResponseWriter, r *http.Request, c *reqCtx) error {
	var req RevertRequest
	if err := readJSON(r, &req); err != nil {
		return err
	}
	target, err := s.resolve(r.Context(), c, req.Changeset)
	if err != nil {
		return err
	}
	if req.Onto == "" {
		req.Onto = "main"
	}
	onto, err := s.resolve(r.Context(), c, req.Onto)
	if err != nil {
		return err
	}
	tcs, err := c.repo.GetChangeset(r.Context(), target)
	if err != nil {
		return err
	}
	if err := s.checkWritePaths(c, tcs.Changes); err != nil {
		return err
	}
	if req.Ref != nil {
		if err := s.checkRefWrite(c, req.Ref.Name); err != nil {
			return err
		}
	}
	id, err := c.repo.Revert(r.Context(), target, onto, s.ident(c, req.Author), req.Message)
	if err != nil {
		return err
	}
	if req.Ref != nil {
		if err := c.repo.MoveRef(r.Context(), req.Ref.Name, req.Ref.Old, id, c.pr.String()); err != nil {
			return err
		}
	}
	writeJSON(w, http.StatusOK, IDResponse{ID: id})
	return nil
}

// sortedKeys is a small helper for deterministic output.
func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
