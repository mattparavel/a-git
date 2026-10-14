# a-git

A server-centric version control system, built from the design ideas
Anthropic describes in
[*Redesigning source control*](https://www.anthropic.com/engineering/redesigning-source-control)
(their in-house system is called Scion). This is an independent
implementation in Go with no third-party dependencies.

Git assumes every clone is a full, offline replica. a-git assumes clients are
**always online**: agents, CI jobs and developers on a fast network. Under that
assumption most of git's machinery can move to a server:

- Commits, merges, rebases and lands run **on the server**. There is no push.
- History, diffs, blame, merge-bases and file reads are **API calls**. Many
  jobs never need a checkout.
- A working copy is **optional and sparse**. It is a directory of files plus a
  few hundred bytes of metadata, not a replica of history.
- Objects are immutable and content-addressed, so **every tier can cache
  everything**: per-node daemon → regional cache → authoritative server.
- Commits are cheap and lock-free. **Refs are the only mutable state** and move
  by compare-and-swap.

```
 agit CLI / CI job / agent                                   (any machine)
        │  unix socket
        ▼
     agitd ─ per-node daemon: shared, bounded on-disk object store
        │  http
        ▼
   agit-cache ─ regional tier: in-memory object + immutable-response cache
        │  http
        ▼
   agit-server ─ authoritative: storage, derived data, all writes, IAM, reviews
```

Every tier speaks the same protocol, so clients can point at any of them.

## Quick start

```sh
go build -o bin/ ./cmd/...          # agit, agit-server, agit-cache, agitd
./scripts/demo.sh                    # full end-to-end tour with real processes
```

By hand:

```sh
bin/agit-server -data /tmp/agit-data -create demo &
bin/agit-cache  -upstream http://127.0.0.1:7070 &
bin/agitd       -upstream http://127.0.0.1:7071 &     # listens on a unix socket

bin/agit import-git ~/src/some-git-repo --repo demo   # bring history over
bin/agit clone demo work --sparse src/app             # fetches only src/app
cd work
$EDITOR src/app/main.go
bin/agit status
bin/agit commit -m "app: fix the thing"     # changeset is created on the server
bin/agit land                               # server rebases onto main and CAS-advances it
```

The CLI finds the daemon's socket automatically. Without a daemon, set
`AGIT_SERVER=http://host:port` (or pass `--server`) to talk to a cache or the
server directly.

## Commands

| Area | Commands |
|---|---|
| Working copy | `clone [--sparse]`, `status`, `diff`, `commit`, `update`, `sync`, `sparse list/add/remove/set` |
| Server-side history editing | `land`, `rebase`, `merge`, `revert`, `ref set/delete` |
| Reads, with or without a checkout | `log [--path --follow]`, `show`, `cat REV:PATH`, `ls`, `blame`, `search`, `merge-base`, `refs`, `stack` |
| Review | `review create/update/show/list/approve/comment/land/abandon/find` |
| Operations | `import-git`, `export-git`, `snapshot`, `bisect`, `events`, `info --metrics`, `repos`, `init-repo` |

Revisions accept ref names, abbreviated IDs, `~N`, `^`, `^N`, and `@` (the
working copy's changeset): for example `main~3`, `@^`, or `9f2c1e^2`.

## What's implemented

- **Changesets.** A changeset lists only the files that changed relative to its
  first parent. Root trees are *derived*, never stored in the changeset, and
  rebuilt incrementally (`internal/object`, `internal/repo/tree.go`).
- **Derived data framework.** Each kind is defined recursively from parents and
  computed eagerly or lazily per kind (`internal/repo/derive.go`):
  - root trees (eager)
  - commit-graph generation numbers and skip lists, for O(log n) `~N` and
    ancestry checks (eager)
  - per-path history nodes, for O(versions) `log --path`
  - blame, derived per file version
  - rename/copy detection
- **Lock-free writes.**
  - Changesets are content-addressed and idempotent.
  - A fixed-width write-ahead log gives sequence numbers without coordination.
  - Refs move by compare-and-swap. `land` retries on contention instead of
    holding a global merge-queue lock (`internal/store`, `internal/repo/merge.go`).
- **Client-driven fetch protocol.** The client walks from a root tree one level
  at a time and requests only the missing objects by ID. Objects travel in their
  stored form (no deltas), so requests are cacheable at every tier and
  resumable (batch retry, HTTP `Range`). Sparse checkouts need no protocol
  support (`internal/fetch`).
- **Tiers.**
  - Strict ordering is enforced with a `Via` header.
  - Concurrent misses for the same object are coalesced.
  - Immutable API responses are cached.
  - The daemon can be seeded from a snapshot pack and keep revisions warm with
    `-prefetch` (`internal/tier`).
- **IAM.**
  - Bearer tokens, groups, and per-repository and per-directory rules, where
    the most specific rule wins.
  - Partial readers get filtered listings and redacted changesets.
  - Hash-addressed object reads are authorized through location hints, so a
    shared daemon cache never leaks a restricted blob.
  - Protected refs only advance through approved reviews (`internal/iam`).
- **Code review with semantic rules** (`internal/review`, `internal/semantic`):
  - `REVIEWERS` files define owners, with `!require N` and `!trivial any`
    directives.
  - Approvals survive revisions that change only comments or whitespace.
  - Docs-only changes can be approved by anyone except the author.
  - Landed changesets are indexed, so you can ask "which review landed X?"
- **Git interop.**
  - `import-git` streams `git fast-export`, incrementally with `--state`.
  - `export-git` drives `git fast-import` to keep a git mirror.
  - The round trip reproduces byte-identical git commit hashes
    (`internal/gitbridge`).
- **Working copy** (`internal/workcopy`):
  - No staging area.
  - Stat-cached status that correctly handles edits made in the same second.
  - `update` merges local edits three-way.
  - Exec bits, symlinks and file↔directory transitions are handled.

See [docs/DESIGN.md](docs/DESIGN.md) for the architecture, data model and
protocol, and how each piece maps to the ideas in the article.

For a guided tour, open the animated explainer in
[docs/explainer/](docs/explainer/README.md). It runs in the browser (open
`index.html`) and is rendered to `a-git-explainer.mp4` with
`node scripts/explainer/render.mjs`.

## Configuration

`agit-server` flags:

- `-data DIR` persists to a directory. Several server processes may share it
  (writes are atomic link/rename and CAS).
- `-iam policy.json` enables access control.
- `-protect main,release/*` makes those refs advance only by landing approved
  reviews.

Example policy:

```json
{
  "users":  {"alice": {"tokens": ["sha256:<hex digest>"], "email": "alice@example.com"},
             "ci":    {"tokens": ["ci-secret"]}},
  "groups": {"eng": ["alice", "bob"]},
  "rules": [
    {"repo": "*",    "path": "",        "principals": ["group:eng"],   "allow": "write"},
    {"repo": "*",    "path": "",        "principals": ["user:ci"],     "allow": "read"},
    {"repo": "mono", "path": "secrets", "principals": ["user:alice"],  "allow": "write"}
  ]
}
```

Clients pass tokens with `AGIT_TOKEN`. Without `-iam` the server is open and
trusts the self-declared `AGIT_USER` only to label authors and reviewers.

## Development

```sh
go test -race ./...     # unit, API, tier, git round-trip and working-copy tests
go vet ./...
./scripts/demo.sh       # end-to-end smoke test (KEEP=1 to keep the temp dir)
```

## Limitations

This is a faithful small-scale model of the design, not a production
deployment:

- The filesystem store stands in for a distributed database plus a blob store.
- There is no FUSE-based virtual filesystem. Sparse profiles and the
  checkout-free API cover the same need.
- Commit signatures are not preserved by `import-git`.
