# a-git

Server-centric VCS in Go (stdlib only). See README.md and docs/DESIGN.md.

- Build: `go build -o bin/ ./cmd/...`
- Test: `go test -race ./...` and `go vet ./...`; end-to-end: `./scripts/demo.sh`
- Layout: `internal/object` (data model) → `store` (storage) → `repo` (derived data + server-side ops) →
  `api` (HTTP server) / `client` → `tier` (cache + daemon) / `fetch` / `workcopy` / `gitbridge` / `review`.
- Changesets list changes relative to the FIRST parent; root trees are derived data, never stored in changesets.
- Anything cached by a tier must be immutable; mark responses with `immutable(w, c)` only when every
  revision input is a full changeset ID.
- Keep repository and IAM checks in `internal/api/handlers.go`; `internal/repo` is policy-free.
