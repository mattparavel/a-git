package workcopy

import (
	"context"
	"fmt"
	"os"
	"os/user"
	"strings"
	"time"

	"github.com/mattparavel/a-git/internal/api"
	"github.com/mattparavel/a-git/internal/client"
	"github.com/mattparavel/a-git/internal/object"
)

// DefaultAuthor returns the author identity for new changesets: $AGIT_AUTHOR
// or "user <user@host>".
func DefaultAuthor() string {
	if a := os.Getenv("AGIT_AUTHOR"); a != "" {
		return a
	}
	name := "unknown"
	if u, err := user.Current(); err == nil && u.Username != "" {
		name = u.Username
	}
	host, _ := os.Hostname()
	if host == "" {
		host = "localhost"
	}
	return fmt.Sprintf("%s <%s@%s>", name, name, host)
}

// CommitOptions controls Commit.
type CommitOptions struct {
	Message string
	Author  string
	Paths   []string // restrict to these paths (default: all changes)
	// Ref, if set, is advanced to the new changeset with compare-and-swap
	// from Old (the server refuses this for protected refs).
	Ref *api.RefMove
}

// ErrNothingToCommit is returned when there are no local changes.
var ErrNothingToCommit = fmt.Errorf("nothing to commit")

// Commit records local changes as a new changeset on top of the base. The
// changeset is created on the server (there is nothing to push); only blobs
// the server does not already have are uploaded. Commits do not move any
// ref: the working copy simply rebases onto its own new changeset.
func (w *WorkCopy) Commit(ctx context.Context, opts CommitOptions) (object.ID, []FileStatus, error) {
	status, err := w.Status(ctx, opts.Paths)
	if err != nil {
		return object.Zero, nil, err
	}
	if len(status) == 0 {
		return object.Zero, nil, ErrNothingToCommit
	}
	if strings.TrimSpace(opts.Message) == "" {
		return object.Zero, nil, fmt.Errorf("a commit message is required")
	}
	if opts.Author == "" {
		opts.Author = DefaultAuthor()
	}
	now := time.Now()
	_, offset := now.Zone()
	cs := &object.Changeset{
		Author: opts.Author, AuthorTime: now.Unix(), AuthorTZ: offset / 60,
		Message: opts.Message,
	}
	if !w.State.Base.IsZero() {
		cs.Parents = []object.ID{w.State.Base}
	}
	var objs []client.StoredObject
	newStates := map[string]FileState{}
	for _, s := range status {
		if s.Status == "D" {
			cs.Changes = append(cs.Changes, object.Change{Path: s.Path})
			continue
		}
		if err := object.ValidatePath(s.Path); err != nil {
			return object.Zero, nil, err
		}
		mode, data, fi, err := diskFile(w.Abs(s.Path))
		if err != nil {
			return object.Zero, nil, err
		}
		id, stored := object.Encode(object.KindBlob, data)
		objs = append(objs, client.StoredObject{ID: id, Stored: stored})
		cs.Changes = append(cs.Changes, object.Change{Path: s.Path, Mode: mode, Blob: id})
		size, mtime, ino := statOf(fi)
		newStates[s.Path] = FileState{Mode: mode, ID: id, Size: size, MTime: mtime, Ino: ino}
	}
	if _, err := w.Repo.UploadMissing(ctx, objs); err != nil {
		return object.Zero, nil, fmt.Errorf("uploading: %w", err)
	}
	res, err := w.Repo.CreateChangeset(ctx, cs, opts.Ref)
	if err != nil {
		return object.Zero, nil, err
	}
	for _, s := range status {
		if s.Status == "D" {
			delete(w.Files, s.Path)
		} else {
			w.Files[s.Path] = newStates[s.Path]
		}
	}
	w.State = State{Base: res.ID, Tree: res.Tree}
	return res.ID, status, w.Save()
}
