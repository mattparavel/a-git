# a-git design

This document describes how a-git works and how each part corresponds to the
ideas in Anthropic's
[*Redesigning source control*](https://www.anthropic.com/engineering/redesigning-source-control).
The article gives high-level principles rather than schemas or wire formats.
Everything concrete below is this project's own design.

## Principles, and where they live

| Principle | a-git |
|---|---|
| Clients are always online | No local history. Every read and write is an API call (`internal/api`, `internal/client`). |
| Smart server, thin client | Merge, rebase, land, revert, blame, log and diff run in `internal/repo`. The CLI mostly formats results. |
| Source control as APIs | Every operation is an HTTP endpoint. `agit cat/log/blame/diff --repo X` need no checkout. |
| Horizontal scaling, no maintenance | Immutable content-addressed objects. No packfiles, repacking or GC of deltas. The FS store tolerates several writer processes. |
| Cheap, incremental, lock-free commits | Changesets list only their changes, inserts are idempotent, the WAL needs no coordination, refs use CAS (`internal/store`). |
| Few refs | Commits never move refs. Only `land` (or an explicit `ref set`) does, so most work never touches global mutable state. |
| Optional, sparse working copies | `internal/workcopy` with sparse profiles. The fetch protocol only asks for what the profile needs. |

## Data model (`internal/object`)

All IDs are SHA-256.

**Blob.** The bytes of a file.

**Tree.** One directory. It is a sorted list of `<mode> <id> <name>` lines.
Modes follow git: `100644`, `100755`, `120000` and `040000`.

**Changeset.** The unit of work. It is canonical JSON:

```json
{
  "parents": ["<id>", "..."],
  "author": "Ada <ada@example.com>", "author_time": 1704144845, "author_tz": 330,
  "committer": "...", "commit_time": 0, "commit_tz": 0,
  "message": "...",
  "changes": [
    {"path": "src/a.go", "mode": "100644", "blob": "<id>"},
    {"path": "old.txt"}
  ],
  "extra": {"git-commit": "<sha>", "rebased-from": "<id>", "reverts": "<id>"}
}
```

A changeset does not contain a tree. Its `changes` are the files that differ
from the **first parent**. A change without a mode is a deletion. Merges use
the first parent as the baseline, so a merge records only what differs from
it.

**History node.** A Merkle structure that runs parallel to the trees. For a
path, a node holds:

- that path's content (a tree or blob ID)
- the changeset that introduced this version (`link`)
- the earlier versions it replaced (`parents`)
- for directories, child history nodes

**Stored form.** Objects are stored, cached and sent in one format:
`kind byte, encoding byte (raw|zlib), payload`. There are no deltas, so each
object stands alone. That keeps transfers simple to cache and to resume.

## Storage (`internal/store`)

The `Store` interface separates immutable objects (blob storage) from
metadata (a transactional database). There are two implementations:
`Mem` for tests and `FS` for real use.

| Record | Write discipline |
|---|---|
| Objects | Content-addressed. Written to a temp file, then hard-linked into place, so concurrent writers are harmless. |
| Changesets | Same as objects. Exactly one writer wins the `link()`, and only that writer appends to the log. |
| WAL | Each repo has a log of fixed-width records (`<id> <unix-nanos>\n`) written with `O_APPEND`. A record's offset ÷ width is its sequence number. Writers need no coordination. |
| Refs | Compare-and-swap under a per-ref file lock (the "single-row transaction"). Every transition is appended to a fixed-width ref log. |
| Derived data | Deterministic values written by rename. Racing writers always agree. |
| KV (reviews) | Read-modify-write under a per-key lock. |

## Derived data (`internal/repo/derive.go`)

A *deriver* computes a value for a changeset from the changeset itself and
the values already derived for its ancestors. The engine:

1. Finds underived ancestors with an iterative post-order walk. It stops at any
   changeset that already has a value, because values are always derived
   parents first, so everything below one is also derived.
2. Derives them oldest first and persists each value.
3. Serialises slow-path derivation per kind within a process. Other processes
   may race; that is harmless.

| Kind | Mode | Value | Notes |
|---|---|---|---|
| `tree` | eager | root tree ID | Applies the change trie to the parent's tree, rewriting only the directories on changed paths. Cost grows with the size of the change, not the repo. Changesets that don't apply are rejected at creation. |
| `graph` | eager | generation, first-parent depth, skip list | Skip list is `skip[k]` = the ancestor 2^k first-parent steps back. Gives O(log n) for `~N` and fast pruning for `IsAncestor` and `MergeBases`. |
| `history` | lazy | root history node | A path keeps its node from the first parent that has identical content. A new node is written only when content changed. `log --path`, including directories, walks only that path's versions. |
| `copies` | lazy | detected renames | Matches identical blobs between deletes and adds, then line-set similarity of at least 50%. Used by `log --follow` and blame. |
| `blame` | lazy | line ranges per file version | Keyed by history node, not changeset. Computed from parent versions' blame plus a line diff, crossing renames through `copies`. |

New kinds are invisible to clients, so they can be added freely.

## Server-side operations (`internal/repo`)

**Create changeset.** Steps:

1. Validate the structure.
2. Check that the parents exist and that every blob was uploaded.
3. Build the root tree. This also rejects changes that don't apply.
4. Store the changeset and append it to the WAL.
5. Derive the eager kinds.

No ref moves.

**Merge.** Three-way merge with the merge base. It only examines paths that
changed on the `theirs` side. File contents are merged with a diff3-style line
merge (`internal/textdiff`). Conflicts come back as structured data, or as
conflict markers when `allow_conflicts` is set.

**Rebase / pick.** Re-applies each changeset's own change list on a new base.
A path where the new base still matches the old base is copied as-is. Only
paths both sides touched are content-merged. Changes that are already present
are dropped.

**Land.** The replacement for a merge queue:

1. Read the ref.
2. Fast-forward if possible; otherwise rebase the stack onto the tip.
3. Compare-and-swap the ref.
4. If someone else landed first, the CAS fails. Re-read and try again.

No global lock is held at any point, and only real content conflicts stop a
land. Validation is expected to happen after merge and before deploy. (The
article makes the same trade-off: it relies on post-merge testing and lets
agents resolve the occasional conflict.)

**Revert.** Inverts the changeset relative to its first parent, then picks
that inverse onto the target.

## Protocol (`internal/api`)

HTTP + JSON, plus a binary frame stream for objects: `32-byte id | u32 length
| stored bytes`, where length 0 means "unavailable".

The key endpoints:

```
GET  /v1/repos/{r}/objects/{id}           one object (supports Range)
POST /v1/repos/{r}/objects/get            batch: [{id, cs, path}] -> frames
POST /v1/repos/{r}/objects/missing        which of these do you lack?
POST /v1/repos/{r}/objects                upload frames (verified)
POST /v1/repos/{r}/changesets             create (optionally CAS a ref too)
POST /v1/repos/{r}/changesets/import      bulk ingestion
GET  /v1/repos/{r}/tree|file|blame|history/{rev}?path=
GET  /v1/repos/{r}/log?rev=&exclude=&path=&follow=&first_parent=
GET  /v1/repos/{r}/diff?from=&to=&path=&patch=1
GET  /v1/repos/{r}/merge-base?a=&b=       /is-ancestor   /search   /resolve
POST /v1/repos/{r}/merge | rebase | land | revert
PUT  /v1/repos/{r}/refs/{name}            {old, new} compare-and-swap
GET  /v1/repos/{r}/wal | reflog           event streams for post-merge automation
GET  /v1/repos/{r}/snapshot?rev=          all objects reachable from rev (daemon seeding)
/v1/repos/{r}/reviews/...                 review service
```

A response is marked `Cache-Control: immutable` when it is a pure function of
immutable inputs: object reads, or queries whose revisions are all full
changeset IDs. Caching tiers store exactly those responses.

### Fetch (`internal/fetch`)

There is no negotiation step where the client says what it has. Instead:

1. The client resolves a revision to its root tree.
2. It checks its local store for each tree on the current level and batches
   requests for the missing ones.
3. It decodes the trees and repeats for the next level, skipping paths outside
   the sparse profile.
4. It fetches blobs the same way.
5. For `update`, it walks the old and new root trees together and descends only
   into subtrees whose IDs differ.

Every request names immutable objects, so any tier can answer it, and a broken
stream is retried for only the IDs that didn't arrive.

## Tiers (`internal/tier`)

`Proxy` serves the full protocol in front of exactly one upstream:

- **Objects** come from the tier's `ObjectCache`: memory LRU for `agit-cache`,
  bounded disk LRU for `agitd`. Concurrent misses for one object are coalesced.
  Batch misses go upstream as one request and stream back through.
- **Immutable responses**, as marked by the server, are cached in memory.
  Everything else, including all writes, is forwarded.
- **Ordering**: each hop appends itself to `X-Agit-Via`. A tier refuses
  requests from a peer or a higher tier, and refuses loops. Failures therefore
  can't spread sideways between tiers.
- **Authorization**: if the upstream runs IAM, the tier caches each principal's
  `/access` answer briefly. Full readers are served cached objects directly.
  Partial readers get a per-object decision from the server, based on location
  hints, and that decision is cached. A secret cached on a shared daemon by one
  user is never served to another.
- **Warm starts**: `agitd -seed-pack FILE` imports a pack written by
  `agit snapshot`. `-prefetch repo@rev:prefix` keeps a revision warm by
  fetching only what is missing.

## Working copy (`internal/workcopy`)

`.agit/` holds three files:

- `config.json`: repo, remote, target ref, sparse profile
- `state.json`: base changeset and its root tree
- `dirstate`: per-file mode, blob ID, size, mtime and inode

There is no staging area. Every non-ignored file inside the profile is
tracked.

- **Status.** Trusts stat data only for files last modified before the
  dirstate was written. Anything else is rehashed. This avoids git's "racily
  clean" problem.
- **Commit.** Hashes the changed files, uploads only the blobs the server
  lacks, creates the changeset on the server, and moves the working copy's base
  to it. No ref moves.
- **Update.** Fetches only the subtrees that differ. Local edits are merged
  three-way against incoming changes; conflicts get markers. Files can turn
  into directories and back.
- **Sparse.** Changing the profile materializes or removes files in place, and
  refuses to drop local edits unless forced.

## Review (`internal/review`, `internal/semantic`)

A review is a stack of changesets with numbered revisions. Its approval
status is computed fresh on every read.

- **Owners.** The nearest `REVIEWERS` file above each changed path names who
  may approve. It can contain `user`, `group:NAME`, `!require N` and
  `!trivial any`. Ownership files are read at both the base and the head, so a
  change can't weaken its own requirements.
- **Semantic carry-over.** Each new revision is classified against the
  previous one as identical, whitespace, comments, docs or logic. The
  classifier is a language-aware tokenizer covering about 50 languages that
  knows comment and string syntax. An approval stays valid while every later
  interdiff is whitespace- or comment-only.
- **Trivial changes.** In scopes marked `!trivial any`, a review whose overall
  change is docs, comments or whitespace needs only one approval from anyone
  other than the author.
- **Landing** goes through `repo.Land`. The review records where its
  changesets landed, so "which review landed X?" is a key lookup rather than a
  history scan.

## Git interop (`internal/gitbridge`)

- **Import.** `git fast-export --show-original-ids` already emits commits as
  "file changes relative to the first parent", which is exactly the changeset
  shape. The importer computes changeset IDs locally, uploads missing blobs,
  and creates changesets in bulk, parents first. Git and a-git marks are kept
  so later runs are incremental.
- **Export.** Collects the changesets that haven't been exported yet, sorts
  them topologically, and writes a `fast-import` stream.

Commit metadata is preserved exactly, so a round trip reproduces the original
git commit hashes. The test suite checks this.
