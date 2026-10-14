// Package api defines a-git's HTTP protocol and implements the
// authoritative server tier.
//
// Every tier (server, regional cache, per-node daemon) serves this same
// protocol, so a client never needs to know which tier it is talking to.
// Responses that are pure functions of immutable inputs (objects, changesets,
// anything addressed by full changeset IDs) are marked immutable and may be
// cached forever by any tier.
package api

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/mattparavel/a-git/internal/iam"
	"github.com/mattparavel/a-git/internal/object"
	"github.com/mattparavel/a-git/internal/repo"
	"github.com/mattparavel/a-git/internal/store"
)

// Version is the protocol version reported by /v1/info.
const Version = "a-git/1"

// Tier names. A tier only accepts requests from tiers below it, which keeps
// the dependency chain strictly ordered: client -> daemon -> cache -> server.
const (
	TierClient = "client"
	TierDaemon = "daemon"
	TierCache  = "cache"
	TierServer = "server"
)

// TierRank orders tiers; requests must flow from lower to higher rank.
func TierRank(t string) int {
	switch t {
	case TierClient:
		return 0
	case TierDaemon:
		return 1
	case TierCache:
		return 2
	case TierServer:
		return 3
	}
	return -1
}

// Header names.
const (
	HeaderVia        = "X-Agit-Via"   // comma-separated tiers a request passed through
	HeaderTier       = "X-Agit-Tier"  // tier that produced a response
	HeaderCache      = "X-Agit-Cache" // "hit" or "miss" on cached tiers
	HeaderObjectKind = "X-Agit-Kind"
	// HeaderUser carries a self-declared user name. It is only honoured by
	// servers running without IAM, where it identifies authors and
	// reviewers but grants nothing.
	HeaderUser         = "X-Agit-User"
	CacheImmutable     = "public, max-age=31536000, immutable"
	CachePrivate       = "private, no-store"
	ContentTypeObjects = "application/x-agit-objects"
)

// ErrorResponse is the JSON body of every non-2xx response.
type ErrorResponse struct {
	Error     string          `json:"error"`
	Code      string          `json:"code,omitempty"` // not_found, conflict, cas_failed, forbidden, ...
	Conflicts []repo.Conflict `json:"conflicts,omitempty"`
}

// InfoResponse describes a tier.
type InfoResponse struct {
	Tier     string `json:"tier"`
	Version  string `json:"version"`
	Upstream string `json:"upstream,omitempty"`
	IAM      bool   `json:"iam"`
}

// AccessResponse describes the caller's permissions on a repository.
type AccessResponse struct {
	Principal  string   `json:"principal"`
	Read       string   `json:"read"` // "full", "partial" or "none"
	Write      bool     `json:"write"`
	Admin      bool     `json:"admin"`
	Restricted []string `json:"restricted,omitempty"` // prefixes with their own rules
}

// ObjectRef names an object to fetch. CS and Path optionally say where the
// object was found; repositories with read-restricted directories need this
// context to authorize callers who cannot read everything.
type ObjectRef struct {
	ID   object.ID `json:"id"`
	CS   object.ID `json:"cs,omitzero"`
	Path string    `json:"path,omitempty"`
}

// GetObjectsRequest asks for several objects at once.
type GetObjectsRequest struct {
	Objects []ObjectRef `json:"objects"`
}

// IDsRequest carries a list of IDs.
type IDsRequest struct {
	IDs []object.ID `json:"ids"`
}

// MissingResponse lists the IDs the server lacks.
type MissingResponse struct {
	Missing []object.ID `json:"missing"`
}

// AuthzResponse answers an authorization query per object.
type AuthzResponse struct {
	Allowed []bool `json:"allowed"`
}

// CreateChangesetRequest creates a changeset on the server and optionally
// moves a ref to it in the same call.
type CreateChangesetRequest struct {
	Changeset object.Changeset `json:"changeset"`
	Ref       *RefMove         `json:"ref,omitempty"`
}

// RefMove is a compare-and-swap ref update.
type RefMove struct {
	Name string    `json:"name"`
	Old  object.ID `json:"old"`
}

// CreateChangesetResponse reports the new changeset.
type CreateChangesetResponse struct {
	ID   object.ID `json:"id"`
	Tree object.ID `json:"tree"`
}

// ImportRequest bulk-creates changesets, parents before children.
type ImportRequest struct {
	Changesets []object.Changeset `json:"changesets"`
}

// ImportResponse returns IDs in request order.
type ImportResponse struct {
	IDs []object.ID `json:"ids"`
}

// ChangesetsResponse maps IDs to changesets.
type ChangesetsResponse struct {
	Changesets map[string]*object.Changeset `json:"changesets"`
}

// ResolveResponse is the result of resolving a revision.
type ResolveResponse struct {
	ID object.ID `json:"id"`
}

// RefsResponse lists refs.
type RefsResponse struct {
	Refs []store.Ref `json:"refs"`
}

// UpdateRefRequest is a compare-and-swap ref update.
type UpdateRefRequest struct {
	Old object.ID `json:"old"`
	New object.ID `json:"new"`
}

// TreeEntry is a directory entry in API responses.
type TreeEntry struct {
	Name string      `json:"name"`
	Path string      `json:"path"`
	Mode object.Mode `json:"mode"`
	ID   object.ID   `json:"id"`
}

// TreeResponse lists one directory of a changeset.
type TreeResponse struct {
	Changeset object.ID   `json:"changeset"`
	Path      string      `json:"path"`
	Mode      object.Mode `json:"mode"`
	ID        object.ID   `json:"id"`
	Entries   []TreeEntry `json:"entries,omitempty"`
}

// LogEntry pairs a changeset with its ID.
type LogEntry struct {
	ID        object.ID         `json:"id"`
	Changeset *object.Changeset `json:"changeset"`
}

// LogResponse lists changesets.
type LogResponse struct {
	Entries []LogEntry `json:"entries"`
}

// HistoryResponse lists the versions of a path.
type HistoryResponse struct {
	Entries []repo.PathChange `json:"entries"`
}

// BlameResponse attributes lines.
type BlameResponse struct {
	Path   string            `json:"path"`
	Ranges []repo.BlameRange `json:"ranges"`
}

// DiffFile is one file in a diff.
type DiffFile struct {
	repo.FileDiff
	Status  string `json:"status"`
	Binary  bool   `json:"binary,omitempty"`
	Patch   string `json:"patch,omitempty"`
	Added   int    `json:"added,omitempty"`
	Deleted int    `json:"deleted,omitempty"`
}

// DiffResponse describes the differences between two changesets.
type DiffResponse struct {
	From  object.ID  `json:"from"`
	To    object.ID  `json:"to"`
	Files []DiffFile `json:"files"`
}

// MergeBaseResponse lists merge bases.
type MergeBaseResponse struct {
	Bases []object.ID `json:"bases"`
}

// AncestorResponse answers an ancestry query.
type AncestorResponse struct {
	Ancestor bool `json:"ancestor"`
}

// SearchResponse lists matching paths.
type SearchResponse struct {
	Results []repo.SearchResult `json:"results"`
}

// MergeRequest asks the server to merge two changesets.
type MergeRequest struct {
	Ours           string   `json:"ours"`
	Theirs         string   `json:"theirs"`
	Message        string   `json:"message,omitempty"`
	AllowConflicts bool     `json:"allow_conflicts,omitempty"`
	Ref            *RefMove `json:"ref,omitempty"`
	// Author is honoured only by servers without IAM; otherwise the
	// authenticated principal is the author.
	Author string `json:"author,omitempty"`
}

// RebaseRequest rebases the changesets in head that are not in base (by
// default, the merge base with onto) onto onto.
type RebaseRequest struct {
	Head string `json:"head"`
	Onto string `json:"onto"`
	Base string `json:"base,omitempty"`
}

// LandRequest lands head on a ref.
type LandRequest struct {
	Head   string `json:"head"`
	Ref    string `json:"ref"`
	Review string `json:"review,omitempty"`
}

// RevertRequest reverts a changeset on top of onto.
type RevertRequest struct {
	Changeset string   `json:"changeset"`
	Onto      string   `json:"onto"`
	Message   string   `json:"message,omitempty"`
	Ref       *RefMove `json:"ref,omitempty"`
	Author    string   `json:"author,omitempty"` // see MergeRequest.Author
}

// IDResponse carries a single changeset ID.
type IDResponse struct {
	ID object.ID `json:"id"`
}

// WALResponse lists log entries.
type WALResponse struct {
	Entries []store.WALEntry `json:"entries"`
}

// RefLogResponse lists ref transitions.
type RefLogResponse struct {
	Entries []store.RefUpdate `json:"entries"`
}

// AccessFromLevel summarises a principal's access for AccessResponse.
func AccessFromLevel(p *iam.Policy, pr iam.Principal, repoName string) AccessResponse {
	a := AccessResponse{Principal: pr.String(), Read: "none"}
	switch {
	case p.FullRead(pr, repoName):
		a.Read = "full"
	case p.Can(pr, repoName, "", iam.Read) || len(p.RestrictedPrefixes(repoName)) > 0 && anyReadable(p, pr, repoName):
		a.Read = "partial"
	}
	a.Write = p.Can(pr, repoName, "", iam.Write)
	a.Admin = p.Can(pr, repoName, "", iam.Admin)
	a.Restricted = p.RestrictedPrefixes(repoName)
	return a
}

func anyReadable(p *iam.Policy, pr iam.Principal, repoName string) bool {
	for _, pre := range p.RestrictedPrefixes(repoName) {
		if p.Can(pr, repoName, pre, iam.Read) {
			return true
		}
	}
	return false
}

// Object frames
//
// Batches of objects travel as a sequence of frames:
//
//	32 bytes  object ID
//	 4 bytes  big-endian length N (0 means "not available")
//	 N bytes  object in stored form
//
// Each frame is independent, so an interrupted transfer can be resumed by
// asking again for just the objects that did not arrive.

// MaxObjectSize bounds a single object on the wire.
const MaxObjectSize = 1 << 30

// WriteFrame writes one object frame.
func WriteFrame(w io.Writer, id object.ID, stored []byte) error {
	var hdr [36]byte
	copy(hdr[:32], id[:])
	binary.BigEndian.PutUint32(hdr[32:], uint32(len(stored)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	_, err := w.Write(stored)
	return err
}

// FrameReader reads object frames.
type FrameReader struct {
	r *bufio.Reader
}

// NewFrameReader wraps r.
func NewFrameReader(r io.Reader) *FrameReader { return &FrameReader{r: bufio.NewReaderSize(r, 64<<10)} }

// Next returns the next frame, or io.EOF at the end of the stream. A nil
// stored slice means the sender did not have the object.
func (f *FrameReader) Next() (object.ID, []byte, error) {
	var hdr [36]byte
	var id object.ID
	if _, err := io.ReadFull(f.r, hdr[:]); err != nil {
		if errors.Is(err, io.ErrUnexpectedEOF) {
			return id, nil, fmt.Errorf("truncated object stream")
		}
		return id, nil, err
	}
	copy(id[:], hdr[:32])
	n := binary.BigEndian.Uint32(hdr[32:])
	if n == 0 {
		return id, nil, nil
	}
	if n > MaxObjectSize {
		return id, nil, fmt.Errorf("object %s too large (%d bytes)", id.Short(), n)
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(f.r, buf); err != nil {
		return id, nil, fmt.Errorf("truncated object %s: %w", id.Short(), err)
	}
	return id, buf, nil
}
