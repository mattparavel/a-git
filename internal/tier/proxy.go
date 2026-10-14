package tier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/mattparavel/a-git/internal/api"
	"github.com/mattparavel/a-git/internal/client"
	"github.com/mattparavel/a-git/internal/iam"
	"github.com/mattparavel/a-git/internal/lru"
	"github.com/mattparavel/a-git/internal/object"
)

// Config configures a caching tier.
type Config struct {
	Tier     string         // api.TierCache or api.TierDaemon
	Upstream *client.Client // the next tier up; its Tier must equal Tier
	Objects  ObjectCache
	// ResponseBytes bounds the cache of immutable API responses.
	ResponseBytes int64
	Logger        *log.Logger
}

// Proxy serves the a-git protocol from its caches and forwards everything
// else to its single upstream. Because a tier only ever talks to the tier
// directly above it, a failure in one tier cannot fan out sideways.
type Proxy struct {
	tier     string
	up       *client.Client
	objects  ObjectCache
	resp     *lru.Cache[string, *cachedResponse]
	logger   *log.Logger
	metrics  *api.Metrics
	mux      *http.ServeMux
	inflight sync.Map // object.ID -> *flight

	iamMu    sync.Mutex
	iamKnown bool
	iamOn    bool

	access *lru.Cache[string, accessEntry] // token digest + repo -> access
	authz  *lru.Cache[string, time.Time]   // token digest + id -> allowed until
}

type cachedResponse struct {
	status int
	header http.Header
	body   []byte
}

type accessEntry struct {
	resp    api.AccessResponse
	expires time.Time
}

type flight struct {
	done   chan struct{}
	stored []byte
	err    error
}

const (
	accessTTL = 30 * time.Second
	authzTTL  = 5 * time.Minute
)

// NewProxy builds a caching tier.
func NewProxy(cfg Config) *Proxy {
	if cfg.ResponseBytes == 0 {
		cfg.ResponseBytes = 64 << 20
	}
	p := &Proxy{
		tier:    cfg.Tier,
		up:      cfg.Upstream,
		objects: cfg.Objects,
		resp:    lru.New[string, *cachedResponse](cfg.ResponseBytes, func(c *cachedResponse) int64 { return int64(len(c.body)) + 256 }),
		logger:  cfg.Logger,
		metrics: api.NewMetrics(),
		mux:     http.NewServeMux(),
		access:  lru.New[string, accessEntry](100_000, nil),
		authz:   lru.New[string, time.Time](1_000_000, nil),
	}
	p.metrics.AddGauge("objects", func() interface{} { return p.objects.Stats() })
	p.metrics.AddGauge("responses", func() interface{} { return p.resp.Stats() })
	p.mux.HandleFunc("GET /v1/info", p.info)
	p.mux.HandleFunc("GET /v1/repos/{repo}/objects/{id}", p.getObject)
	p.mux.HandleFunc("POST /v1/repos/{repo}/objects/get", p.getObjects)
	p.mux.HandleFunc("/", p.forward)
	return p
}

// Handler returns the HTTP handler.
func (p *Proxy) Handler() http.Handler {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(api.HeaderTier, p.tier)
		if err := api.CheckVia(r, p.tier); err != nil {
			api.WriteError(w, err)
			return
		}
		p.mux.ServeHTTP(w, r)
	})
	return p.metrics.Wrap(p.tier, inner)
}

// Objects exposes the object cache (used for seeding and tests).
func (p *Proxy) Objects() ObjectCache { return p.objects }

func (p *Proxy) logf(format string, a ...interface{}) {
	if p.logger != nil {
		p.logger.Printf(format, a...)
	}
}

func (p *Proxy) info(w http.ResponseWriter, r *http.Request) {
	api.WriteJSON(w, http.StatusOK, api.InfoResponse{Tier: p.tier, Version: api.Version, Upstream: p.up.Address(), IAM: p.upstreamIAM(r.Context())})
}

// upstreamIAM reports whether the upstream enforces access control. When it
// does not, the tier can skip all authorization bookkeeping.
func (p *Proxy) upstreamIAM(ctx context.Context) bool {
	p.iamMu.Lock()
	defer p.iamMu.Unlock()
	if p.iamKnown {
		return p.iamOn
	}
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	info, err := p.up.Info(cctx)
	if err != nil {
		return true // fail closed; ask again next time
	}
	p.iamOn, p.iamKnown = info.IAM, true
	return p.iamOn
}

// upstreamAs returns an upstream client that carries the caller's token.
func (p *Proxy) upstreamAs(r *http.Request) *client.Client {
	cp := *p.up
	cp.Token = api.BearerToken(r)
	cp.User = r.Header.Get(api.HeaderUser)
	return &cp
}

func tokenKey(r *http.Request) string { return iam.HashToken(api.BearerToken(r)) }

// accessFor returns the caller's access to a repository, cached briefly.
func (p *Proxy) accessFor(r *http.Request, repoName string) (api.AccessResponse, error) {
	if !p.upstreamIAM(r.Context()) {
		return api.AccessResponse{Read: "full", Write: true, Admin: true}, nil
	}
	k := tokenKey(r) + "/" + repoName
	if e, ok := p.access.Get(k); ok && time.Now().Before(e.expires) {
		return e.resp, nil
	}
	acc, err := p.upstreamAs(r).Repo(repoName).Access(r.Context())
	if err != nil {
		return api.AccessResponse{}, err
	}
	p.access.Add(k, accessEntry{*acc, time.Now().Add(accessTTL)})
	return *acc, nil
}

// authorize filters object refs down to those the caller may read. Callers
// with full read access are allowed everything without further checks;
// others are checked per object (with the location hints they supplied) and
// the decisions cached.
func (p *Proxy) authorize(r *http.Request, repoName string, refs []api.ObjectRef) ([]bool, error) {
	allowed := make([]bool, len(refs))
	acc, err := p.accessFor(r, repoName)
	if err != nil {
		return nil, err
	}
	switch acc.Read {
	case "full":
		for i := range allowed {
			allowed[i] = true
		}
		return allowed, nil
	case "none":
		return allowed, nil
	}
	tk := tokenKey(r)
	var ask []api.ObjectRef
	var askIdx []int
	for i, ref := range refs {
		if until, ok := p.authz.Get(tk + "/" + ref.ID.String()); ok && time.Now().Before(until) {
			allowed[i] = true
			continue
		}
		ask = append(ask, ref)
		askIdx = append(askIdx, i)
	}
	if len(ask) == 0 {
		return allowed, nil
	}
	body, _ := json.Marshal(api.GetObjectsRequest{Objects: ask})
	req, err := p.upstreamAs(r).NewRequest(r.Context(), "POST", "/v1/repos/"+repoName+"/authz", nil, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set(api.HeaderVia, p.via(r))
	resp, err := p.up.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var az api.AuthzResponse
	if err := json.NewDecoder(resp.Body).Decode(&az); err != nil {
		return nil, err
	}
	for j, ok := range az.Allowed {
		if ok && j < len(askIdx) {
			allowed[askIdx[j]] = true
			p.authz.Add(tk+"/"+ask[j].ID.String(), time.Now().Add(authzTTL))
		}
	}
	return allowed, nil
}

func (p *Proxy) via(r *http.Request) string {
	if v := r.Header.Get(api.HeaderVia); v != "" {
		return v + "," + p.tier
	}
	return p.tier
}

// fetchOne returns an object from cache or upstream, coalescing concurrent
// misses for the same object into one upstream request.
func (p *Proxy) fetchOne(r *http.Request, repoName string, ref api.ObjectRef) ([]byte, bool, error) {
	if b, ok := p.objects.Get(ref.ID); ok {
		return b, true, nil
	}
	f := &flight{done: make(chan struct{})}
	if existing, loaded := p.inflight.LoadOrStore(ref.ID, f); loaded {
		f = existing.(*flight)
		select {
		case <-f.done:
		case <-r.Context().Done():
			return nil, false, r.Context().Err()
		}
		return f.stored, false, f.err
	}
	defer func() {
		close(f.done)
		p.inflight.Delete(ref.ID)
	}()
	up := p.upstreamAs(r)
	up.Tier = p.via(r)
	f.stored, f.err = up.Repo(repoName).GetObject(r.Context(), ref)
	if f.err == nil {
		p.objects.Put(ref.ID, f.stored)
	}
	return f.stored, false, f.err
}

func (p *Proxy) getObject(w http.ResponseWriter, r *http.Request) {
	repoName := r.PathValue("repo")
	id, err := object.ParseID(r.PathValue("id"))
	if err != nil {
		api.WriteError(w, api.NewError(http.StatusBadRequest, "invalid", err.Error()))
		return
	}
	ref := api.ObjectRef{ID: id, Path: r.URL.Query().Get("path")}
	if cs := r.URL.Query().Get("cs"); cs != "" {
		ref.CS, _ = object.ParseID(cs)
	}
	ok, err := p.authorize(r, repoName, []api.ObjectRef{ref})
	if err != nil {
		p.writeUpstreamError(w, err)
		return
	}
	if !ok[0] {
		api.WriteError(w, api.NewError(http.StatusForbidden, "forbidden", fmt.Sprintf("object %s is not readable", id.Short())))
		return
	}
	stored, hit, err := p.fetchOne(r, repoName, ref)
	if err != nil {
		p.writeUpstreamError(w, err)
		return
	}
	if hit {
		w.Header().Set(api.HeaderCache, "hit")
	} else {
		w.Header().Set(api.HeaderCache, "miss")
	}
	w.Header().Set("Cache-Control", api.CacheImmutable)
	w.Header().Set("ETag", `"`+id.String()+`"`)
	w.Header().Set(api.HeaderObjectKind, object.KindOf(stored).String())
	w.Header().Set("Content-Type", "application/octet-stream")
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(stored))
}

func (p *Proxy) getObjects(w http.ResponseWriter, r *http.Request) {
	repoName := r.PathValue("repo")
	var req api.GetObjectsRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 256<<20)).Decode(&req); err != nil {
		api.WriteError(w, api.NewError(http.StatusBadRequest, "invalid", err.Error()))
		return
	}
	allowed, err := p.authorize(r, repoName, req.Objects)
	if err != nil {
		p.writeUpstreamError(w, err)
		return
	}
	w.Header().Set("Content-Type", api.ContentTypeObjects)
	var misses []api.ObjectRef
	hits := 0
	for i, ref := range req.Objects {
		if !allowed[i] {
			api.WriteFrame(w, ref.ID, nil)
			continue
		}
		if b, ok := p.objects.Get(ref.ID); ok {
			hits++
			if err := api.WriteFrame(w, ref.ID, b); err != nil {
				return
			}
			continue
		}
		misses = append(misses, ref)
	}
	if len(misses) == 0 {
		return
	}
	// Fetch all misses from upstream in one batch and stream them through
	// as they arrive, caching each one.
	up := p.upstreamAs(r)
	up.Tier = p.via(r)
	got := map[object.ID]bool{}
	missing, err := up.Repo(repoName).GetObjects(r.Context(), misses, func(id object.ID, stored []byte) error {
		got[id] = true
		p.objects.Put(id, stored)
		return api.WriteFrame(w, id, stored)
	})
	if err != nil {
		p.logf("batch fetch from upstream: %v", err)
		// Report what we could not deliver as missing; the client retries.
		for _, ref := range misses {
			if !got[ref.ID] {
				api.WriteFrame(w, ref.ID, nil)
			}
		}
		return
	}
	for _, id := range missing {
		api.WriteFrame(w, id, nil)
	}
}

// forward relays any other request upstream. Successful GET responses that
// the upstream marked immutable are cached and served locally afterwards,
// but only to callers who can read the whole repository (the upstream never
// marks per-principal filtered responses as shareable).
func (p *Proxy) forward(w http.ResponseWriter, r *http.Request) {
	cacheable := r.Method == http.MethodGet
	repoName := ""
	if parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/v1/repos/"), "/"); strings.HasPrefix(r.URL.Path, "/v1/repos/") && len(parts) > 1 {
		repoName = parts[0]
	}
	key := r.URL.RequestURI()
	if cacheable && repoName != "" {
		acc, err := p.accessFor(r, repoName)
		if err != nil {
			p.writeUpstreamError(w, err)
			return
		}
		cacheable = acc.Read == "full"
		if cacheable {
			if c, ok := p.resp.Get(key); ok {
				for k, v := range c.header {
					w.Header()[k] = v
				}
				w.Header().Set(api.HeaderCache, "hit")
				w.Header().Set(api.HeaderTier, p.tier)
				w.WriteHeader(c.status)
				w.Write(c.body)
				return
			}
		}
	} else {
		cacheable = false
	}

	up := p.upstreamAs(r)
	req, err := up.NewRequest(r.Context(), r.Method, r.URL.Path, r.URL.Query(), r.Body)
	if err != nil {
		api.WriteError(w, err)
		return
	}
	req.Header.Set(api.HeaderVia, p.via(r))
	if ct := r.Header.Get("Content-Type"); ct != "" {
		req.Header.Set("Content-Type", ct)
	}
	if rg := r.Header.Get("Range"); rg != "" {
		req.Header.Set("Range", rg)
	}
	resp, err := up.HTTP().Do(req)
	if err != nil {
		api.WriteError(w, api.NewError(http.StatusBadGateway, "upstream", err.Error()))
		return
	}
	defer resp.Body.Close()
	for _, h := range []string{"Content-Type", "Cache-Control", "ETag", "X-Agit-Mode", "X-Agit-Changeset", api.HeaderObjectKind, "Content-Range", "Accept-Ranges"} {
		if v := resp.Header.Get(h); v != "" {
			w.Header().Set(h, v)
		}
	}
	store := cacheable && resp.StatusCode == http.StatusOK && strings.Contains(resp.Header.Get("Cache-Control"), "immutable")
	if !store {
		w.Header().Set(api.HeaderCache, "miss")
		w.WriteHeader(resp.StatusCode)
		copyFlush(w, resp.Body)
		return
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20+1))
	if err != nil {
		api.WriteError(w, api.NewError(http.StatusBadGateway, "upstream", err.Error()))
		return
	}
	if len(body) <= 32<<20 {
		hdr := http.Header{}
		for k, v := range w.Header() {
			if k != api.HeaderTier && k != api.HeaderCache {
				hdr[k] = v
			}
		}
		p.resp.Add(key, &cachedResponse{status: resp.StatusCode, header: hdr, body: body})
	}
	w.Header().Set(api.HeaderCache, "miss")
	w.WriteHeader(resp.StatusCode)
	w.Write(body)
}

func copyFlush(w http.ResponseWriter, r io.Reader) {
	buf := make([]byte, 64<<10)
	fl, _ := w.(http.Flusher)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return
			}
			if fl != nil {
				fl.Flush()
			}
		}
		if err != nil {
			return
		}
	}
}

func (p *Proxy) writeUpstreamError(w http.ResponseWriter, err error) {
	if ce, ok := err.(*client.Error); ok {
		api.WriteJSON(w, ce.Status, api.ErrorResponse{Error: ce.Message, Code: ce.Code, Conflicts: ce.Conflicts})
		return
	}
	api.WriteError(w, api.NewError(http.StatusBadGateway, "upstream", err.Error()))
}
