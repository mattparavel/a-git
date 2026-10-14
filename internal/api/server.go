package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mattparavel/a-git/internal/iam"
	"github.com/mattparavel/a-git/internal/object"
	"github.com/mattparavel/a-git/internal/repo"
	"github.com/mattparavel/a-git/internal/review"
	"github.com/mattparavel/a-git/internal/store"
)

// ServerConfig configures the authoritative server.
type ServerConfig struct {
	Store  store.Store
	Policy *iam.Policy // nil allows everything
	// ProtectedRefs are ref globs (e.g. "main", "release/*") that can only be
	// advanced by landing an approved review, unless the caller is an admin.
	ProtectedRefs []string
	Logger        *log.Logger
}

// Server is the authoritative tier. It owns storage, computes derived data
// and performs every write.
type Server struct {
	repos     *repo.Registry
	policy    *iam.Policy
	protected []string
	reviews   *review.Service
	logger    *log.Logger
	metrics   *Metrics
	mux       *http.ServeMux
}

// NewServer builds a server.
func NewServer(cfg ServerConfig) *Server {
	s := &Server{
		repos:     repo.NewRegistry(cfg.Store),
		policy:    cfg.Policy,
		protected: cfg.ProtectedRefs,
		logger:    cfg.Logger,
		metrics:   NewMetrics(),
		mux:       http.NewServeMux(),
	}
	s.reviews = review.NewService(s.repos, cfg.Policy)
	s.routes()
	return s
}

// Repos exposes the repository registry (used by tests and admin tools).
func (s *Server) Repos() *repo.Registry { return s.repos }

// Handler returns the HTTP handler.
func (s *Server) Handler() http.Handler { return s.metrics.Wrap(TierServer, s.mux) }

type handlerFunc func(w http.ResponseWriter, r *http.Request, c *reqCtx) error

// reqCtx carries per-request state.
type reqCtx struct {
	pr   iam.Principal
	repo *repo.Repo
	// filter is nil when the principal can read the whole repository;
	// otherwise it reports whether a path is readable.
	filter func(string) bool
}

func (c *reqCtx) canRead(p string) bool { return c.filter == nil || c.filter(p) }

// ident is the identity recorded on changesets the server creates for this
// request. Open servers accept the client's claimed author.
func (s *Server) ident(c *reqCtx, claimed string) string {
	if s.policy == nil && strings.Contains(claimed, "<") {
		return claimed
	}
	return s.policy.Ident(c.pr)
}

func (s *Server) handle(pattern string, needRepo bool, h handlerFunc) {
	s.mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(HeaderTier, TierServer)
		if err := CheckVia(r, TierServer); err != nil {
			s.writeError(w, err)
			return
		}
		pr, err := s.policy.Authenticate(bearerToken(r))
		if err != nil {
			s.writeError(w, &httpError{http.StatusUnauthorized, "unauthorized", err.Error()})
			return
		}
		if s.policy == nil {
			if u := strings.TrimSpace(r.Header.Get(HeaderUser)); u != "" && len(u) <= 200 {
				pr = iam.Principal{Name: u}
			}
		}
		c := &reqCtx{pr: pr}
		if needRepo {
			name := r.PathValue("repo")
			if s.policy.Level(pr, name, "") < iam.Read && !s.policy.CanSeeDir(pr, name, "") {
				s.writeError(w, errForbidden("no access to repository %q", name))
				return
			}
			rp, err := s.repos.Get(r.Context(), name)
			if err != nil {
				s.writeError(w, err)
				return
			}
			c.repo = rp
			c.filter = s.policy.Filter(pr, name)
		}
		if err := h(w, r, c); err != nil {
			s.writeError(w, err)
		}
	})
}

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if len(h) > 7 && strings.EqualFold(h[:7], "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

// CheckVia enforces strict tier ordering: a request may only arrive from a
// tier ranked below the receiving one, and never loop.
func CheckVia(r *http.Request, self string) error {
	via := r.Header.Get(HeaderVia)
	if via == "" {
		return nil
	}
	hops := strings.Split(via, ",")
	last := strings.TrimSpace(hops[len(hops)-1])
	for _, h := range hops {
		if strings.TrimSpace(h) == self {
			return &httpError{http.StatusLoopDetected, "loop", "request loop through tier " + self}
		}
	}
	if TierRank(last) < 0 || TierRank(last) >= TierRank(self) {
		return &httpError{http.StatusMisdirectedRequest, "tier_order", fmt.Sprintf("tier %s may not call tier %s", last, self)}
	}
	return nil
}

// ---- errors ----

type httpError struct {
	status int
	code   string
	msg    string
}

func (e *httpError) Error() string { return e.msg }

// NewError returns an error that WriteError renders with the given status
// and code.
func NewError(status int, code, msg string) error { return &httpError{status, code, msg} }

// WriteError renders err as a JSON error response.
func WriteError(w http.ResponseWriter, err error) { (&Server{}).writeError(w, err) }

// WriteJSON renders v as a JSON response.
func WriteJSON(w http.ResponseWriter, status int, v interface{}) { writeJSON(w, status, v) }

// BearerToken extracts the bearer token from a request.
func BearerToken(r *http.Request) string { return bearerToken(r) }

func errForbidden(format string, a ...interface{}) error {
	return &httpError{http.StatusForbidden, "forbidden", fmt.Sprintf(format, a...)}
}

func errBadRequest(format string, a ...interface{}) error {
	return &httpError{http.StatusBadRequest, "invalid", fmt.Sprintf(format, a...)}
}

func (s *Server) writeError(w http.ResponseWriter, err error) {
	resp := ErrorResponse{Error: err.Error()}
	status := http.StatusInternalServerError
	var he *httpError
	var ce *repo.ConflictError
	switch {
	case errors.As(err, &he):
		status, resp.Code = he.status, he.code
	case errors.As(err, &ce):
		status, resp.Code, resp.Conflicts = http.StatusConflict, "conflict", ce.Conflicts
	case errors.Is(err, store.ErrCASFailed):
		status, resp.Code = http.StatusConflict, "cas_failed"
	case errors.Is(err, repo.ErrNothingToLand):
		status, resp.Code = http.StatusConflict, "nothing_to_land"
	case errors.Is(err, review.ErrNotApproved):
		status, resp.Code = http.StatusForbidden, "not_approved"
	case errors.Is(err, store.ErrNotFound):
		status, resp.Code = http.StatusNotFound, "not_found"
	case errors.Is(err, store.ErrExists):
		status, resp.Code = http.StatusConflict, "exists"
	case errors.Is(err, repo.ErrAmbiguous), errors.Is(err, repo.ErrInvalid), errors.Is(err, review.ErrInvalid):
		status, resp.Code = http.StatusBadRequest, "invalid"
	case errors.Is(err, context.Canceled):
		status, resp.Code = 499, "canceled"
	default:
		if s.logger != nil {
			s.logger.Printf("internal error: %v", err)
		}
	}
	writeJSON(w, status, resp)
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.Encode(v)
}

func readJSON(r *http.Request, v interface{}) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 256<<20))
	if err := dec.Decode(v); err != nil {
		return errBadRequest("malformed request body: %v", err)
	}
	return nil
}

// immutable marks a response as cacheable forever by every tier. Responses
// filtered for a principal who cannot read everything are never shared.
func immutable(w http.ResponseWriter, c *reqCtx) {
	if c.filter == nil {
		w.Header().Set("Cache-Control", CacheImmutable)
	} else {
		w.Header().Set("Cache-Control", CachePrivate)
	}
}

func mutable(w http.ResponseWriter) { w.Header().Set("Cache-Control", "no-cache") }

// isFullID reports whether a revision is a full changeset ID, which makes
// any response computed from it immutable.
func isFullID(rev string) bool { return len(rev) == 64 && object.IsHexPrefix(rev) }

func (s *Server) resolve(ctx context.Context, c *reqCtx, rev string) (object.ID, error) {
	if rev == "" {
		return object.Zero, errBadRequest("missing revision")
	}
	return c.repo.Resolve(ctx, rev)
}

func queryInt(r *http.Request, name string, def int) int {
	if v := r.URL.Query().Get(name); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func queryBool(r *http.Request, name string) bool {
	v := r.URL.Query().Get(name)
	return v == "1" || v == "true" || v == "yes"
}

func refProtected(globs []string, ref string) bool {
	for _, g := range globs {
		if ok, _ := path.Match(g, ref); ok {
			return true
		}
	}
	return false
}

// ---- metrics ----

// Metrics counts requests per route and tier.
type Metrics struct {
	mu     sync.Mutex
	start  time.Time
	routes map[string]*RouteStats
	extra  map[string]func() interface{}
}

// RouteStats are counters for one route.
type RouteStats struct {
	Requests  int64   `json:"requests"`
	Errors    int64   `json:"errors"`
	BytesOut  int64   `json:"bytes_out"`
	TotalMS   float64 `json:"total_ms"`
	CacheHits int64   `json:"cache_hits,omitempty"`
}

// NewMetrics returns an empty metrics registry.
func NewMetrics() *Metrics {
	return &Metrics{start: time.Now(), routes: map[string]*RouteStats{}, extra: map[string]func() interface{}{}}
}

// AddGauge registers a named value reported by Snapshot.
func (m *Metrics) AddGauge(name string, fn func() interface{}) {
	m.mu.Lock()
	m.extra[name] = fn
	m.mu.Unlock()
}

type countingWriter struct {
	http.ResponseWriter
	status int
	n      int64
}

func (cw *countingWriter) WriteHeader(code int) {
	cw.status = code
	cw.ResponseWriter.WriteHeader(code)
}

func (cw *countingWriter) Write(b []byte) (int, error) {
	if cw.status == 0 {
		cw.status = 200
	}
	n, err := cw.ResponseWriter.Write(b)
	cw.n += int64(n)
	return n, err
}

func (cw *countingWriter) Flush() {
	if f, ok := cw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Wrap instruments h and serves /v1/info and /v1/metrics.
func (m *Metrics) Wrap(tier string, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/metrics" && r.Method == http.MethodGet {
			writeJSON(w, http.StatusOK, m.Snapshot(tier))
			return
		}
		start := time.Now()
		cw := &countingWriter{ResponseWriter: w}
		h.ServeHTTP(cw, r)
		route := routeName(r.URL.Path)
		m.mu.Lock()
		st := m.routes[route]
		if st == nil {
			st = &RouteStats{}
			m.routes[route] = st
		}
		st.Requests++
		if cw.status >= 400 {
			st.Errors++
		}
		st.BytesOut += cw.n
		st.TotalMS += float64(time.Since(start).Microseconds()) / 1000
		if w.Header().Get(HeaderCache) == "hit" {
			st.CacheHits++
		}
		m.mu.Unlock()
	})
}

// routeName collapses a URL path into a route label ("/v1/repos/*/objects/*").
func routeName(p string) string {
	parts := strings.Split(strings.Trim(p, "/"), "/")
	if len(parts) >= 3 && parts[0] == "v1" && parts[1] == "repos" {
		parts[2] = "*"
		if len(parts) >= 5 {
			switch parts[3] {
			case "objects", "changesets", "tree", "file", "blame", "history", "refs", "reviews", "derived":
				if parts[4] != "get" && parts[4] != "missing" && parts[4] != "import" {
					parts = append(parts[:4], "*")
				}
			}
		}
	}
	return "/" + strings.Join(parts, "/")
}

// Snapshot returns the current metrics.
func (m *Metrics) Snapshot(tier string) map[string]interface{} {
	m.mu.Lock()
	defer m.mu.Unlock()
	routes := map[string]RouteStats{}
	for k, v := range m.routes {
		routes[k] = *v
	}
	out := map[string]interface{}{
		"tier":           tier,
		"uptime_seconds": int64(time.Since(m.start).Seconds()),
		"routes":         routes,
	}
	for k, fn := range m.extra {
		out[k] = fn()
	}
	return out
}
