package repo

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/mattparavel/a-git/internal/object"
	"github.com/mattparavel/a-git/internal/store"
	"github.com/mattparavel/a-git/internal/textdiff"
)

// Conflict describes a path that could not be merged automatically.
type Conflict struct {
	Path string `json:"path"`
	Kind string `json:"kind"` // content, modify/delete, binary, mode, file/directory, symlink
}

// ConflictError is returned by merge-like operations that hit conflicts.
type ConflictError struct {
	Op        string
	Conflicts []Conflict
}

func (e *ConflictError) Error() string {
	s := fmt.Sprintf("%s: %d conflicting path(s)", e.Op, len(e.Conflicts))
	for i, c := range e.Conflicts {
		if i == 5 {
			s += ", ..."
			break
		}
		s += fmt.Sprintf("; %s (%s)", c.Path, c.Kind)
	}
	return s
}

// version is a file's state in one tree; a zero mode means absent.
type version struct {
	mode object.Mode
	id   object.ID
}

func (v version) absent() bool { return v.mode == object.ModeNone }

func (r *Repo) versionAt(ctx context.Context, root object.ID, path string) (version, error) {
	e, err := r.Lookup(ctx, root, path)
	if errors.Is(err, store.ErrNotFound) {
		return version{}, nil
	}
	if err != nil {
		return version{}, err
	}
	if e.Mode.IsDir() {
		return version{mode: object.ModeDir, id: e.ID}, nil
	}
	return version{mode: e.Mode, id: e.ID}, nil
}

type mergeLabels struct{ base, ours, theirs string }

// merge3 merges one path's three versions. It returns the merged version
// (absent = delete) and a conflict kind ("" when clean). For content
// conflicts the returned version holds the text with conflict markers.
func (r *Repo) merge3(ctx context.Context, base, ours, theirs version, labels mergeLabels) (version, string, error) {
	switch {
	case ours == theirs:
		return ours, "", nil
	case base == ours:
		return theirs, "", nil
	case base == theirs:
		return ours, "", nil
	}
	if ours.mode.IsDir() || theirs.mode.IsDir() || base.mode.IsDir() {
		return ours, "file/directory", nil
	}
	if ours.absent() || theirs.absent() {
		if ours.absent() {
			return theirs, "modify/delete", nil
		}
		return ours, "modify/delete", nil
	}
	if ours.mode == object.ModeLink || theirs.mode == object.ModeLink {
		return ours, "symlink", nil
	}
	mode := ours.mode
	switch {
	case ours.mode == theirs.mode:
	case base.mode == ours.mode:
		mode = theirs.mode
	case base.mode == theirs.mode:
		mode = ours.mode
	default:
		return ours, "mode", nil
	}
	if ours.id == theirs.id {
		return version{mode, ours.id}, "", nil
	}
	var baseData []byte
	if !base.absent() {
		var err error
		if baseData, err = r.ReadBlob(ctx, base.id); err != nil {
			return version{}, "", err
		}
	}
	oursData, err := r.ReadBlob(ctx, ours.id)
	if err != nil {
		return version{}, "", err
	}
	theirsData, err := r.ReadBlob(ctx, theirs.id)
	if err != nil {
		return version{}, "", err
	}
	if textdiff.IsBinary(baseData) || textdiff.IsBinary(oursData) || textdiff.IsBinary(theirsData) {
		return ours, "binary", nil
	}
	res := textdiff.Merge3(baseData, oursData, theirsData, labels.base, labels.ours, labels.theirs)
	id, err := r.PutObject(ctx, object.KindBlob, res.Merged)
	if err != nil {
		return version{}, "", err
	}
	kind := ""
	if len(res.Conflicts) > 0 {
		kind = "content"
	}
	return version{mode, id}, kind, nil
}

// MergeOptions controls Merge.
type MergeOptions struct {
	Author         string
	Message        string
	AllowConflicts bool // record conflict markers instead of failing
}

// MergeResult is the outcome of a server-side merge.
type MergeResult struct {
	ID          object.ID  `json:"id"`
	Base        object.ID  `json:"base"`
	Conflicts   []Conflict `json:"conflicts,omitempty"`
	FastForward bool       `json:"fast_forward,omitempty"`
}

// Merge creates a merge changeset with parents [ours, theirs]. If theirs is
// already contained in ours the result is ours; if ours is an ancestor of
// theirs no merge changeset is needed and theirs is returned as a
// fast-forward.
func (r *Repo) Merge(ctx context.Context, ours, theirs object.ID, opts MergeOptions) (*MergeResult, error) {
	if anc, err := r.IsAncestor(ctx, theirs, ours); err != nil {
		return nil, err
	} else if anc {
		return &MergeResult{ID: ours, Base: theirs}, nil
	}
	if anc, err := r.IsAncestor(ctx, ours, theirs); err != nil {
		return nil, err
	} else if anc {
		return &MergeResult{ID: theirs, Base: ours, FastForward: true}, nil
	}
	base, err := r.MergeBase(ctx, ours, theirs)
	if err != nil {
		return nil, err
	}
	baseTree, err := r.RootTree(ctx, base)
	if err != nil {
		return nil, err
	}
	oursTree, err := r.RootTree(ctx, ours)
	if err != nil {
		return nil, err
	}
	theirsTree, err := r.RootTree(ctx, theirs)
	if err != nil {
		return nil, err
	}
	// Only paths that changed on their side can require changes relative to
	// ours; everything else already has ours' content.
	theirDiffs, err := r.DiffTrees(ctx, baseTree, theirsTree)
	if err != nil {
		return nil, err
	}
	labels := mergeLabels{base: "base " + base.Short(), ours: "ours " + ours.Short(), theirs: "theirs " + theirs.Short()}
	var changes []object.Change
	var conflicts []Conflict
	for _, d := range theirDiffs {
		b := version{d.OldMode, d.OldBlob}
		t := version{d.NewMode, d.NewBlob}
		o, err := r.versionAt(ctx, oursTree, d.Path)
		if err != nil {
			return nil, err
		}
		res, kind, err := r.merge3(ctx, b, o, t, labels)
		if err != nil {
			return nil, err
		}
		if kind != "" {
			conflicts = append(conflicts, Conflict{Path: d.Path, Kind: kind})
		}
		if res != o && !res.mode.IsDir() {
			changes = append(changes, object.Change{Path: d.Path, Mode: res.mode, Blob: res.id})
		}
	}
	if len(conflicts) > 0 && !opts.AllowConflicts {
		return &MergeResult{Base: base, Conflicts: conflicts}, &ConflictError{Op: "merge", Conflicts: conflicts}
	}
	msg := opts.Message
	if msg == "" {
		msg = fmt.Sprintf("Merge %s into %s", theirs.Short(), ours.Short())
	}
	now := time.Now()
	cs := &object.Changeset{
		Parents:    []object.ID{ours, theirs},
		Author:     opts.Author,
		AuthorTime: now.Unix(),
		Message:    msg,
		Changes:    changes,
	}
	id, err := r.CreateChangeset(ctx, cs)
	if err != nil {
		var ce *ConflictError
		if errors.As(err, &ce) {
			return nil, err
		}
		// Structural problems (a file colliding with a directory) surface here.
		return &MergeResult{Base: base, Conflicts: []Conflict{{Path: "", Kind: "file/directory"}}},
			&ConflictError{Op: "merge", Conflicts: []Conflict{{Path: err.Error(), Kind: "file/directory"}}}
	}
	return &MergeResult{ID: id, Base: base, Conflicts: conflicts}, nil
}

// pick re-applies a set of changes, originally made on top of tree from, on
// top of tree onto. Paths untouched by the changes are never examined, so
// rebasing a small change onto a much newer base is cheap. It returns the
// changes to apply to onto; changes that are already present in onto are
// dropped.
func (r *Repo) pick(ctx context.Context, from, onto object.ID, changes []object.Change, labels mergeLabels) ([]object.Change, []Conflict, error) {
	var out []object.Change
	var conflicts []Conflict
	for _, ch := range changes {
		b, err := r.versionAt(ctx, from, ch.Path)
		if err != nil {
			return nil, nil, err
		}
		o, err := r.versionAt(ctx, onto, ch.Path)
		if err != nil {
			return nil, nil, err
		}
		t := version{ch.Mode, ch.Blob}
		res, kind, err := r.merge3(ctx, b, o, t, labels)
		if err != nil {
			return nil, nil, err
		}
		if kind != "" {
			conflicts = append(conflicts, Conflict{Path: ch.Path, Kind: kind})
		}
		if res != o && !res.mode.IsDir() {
			out = append(out, object.Change{Path: ch.Path, Mode: res.mode, Blob: res.id})
		}
	}
	return out, conflicts, nil
}

// RebaseResult maps original changesets to their rebased versions.
type RebaseResult struct {
	Head    object.ID            `json:"head"`
	Mapping map[string]object.ID `json:"mapping"` // old id -> new id (zero if dropped as empty)
	Dropped []object.ID          `json:"dropped,omitempty"`
}

// RebaseStack rebases the linear stack of changesets (oldest first) onto
// onto. Each changeset is re-applied with pick; changesets that become empty
// are dropped. Committer information records when the rebase happened.
func (r *Repo) RebaseStack(ctx context.Context, stack []object.ID, onto object.ID, committer string) (*RebaseResult, error) {
	res := &RebaseResult{Head: onto, Mapping: map[string]object.ID{}}
	cur := onto
	var prevOld object.ID
	for i, id := range stack {
		cs, err := r.GetChangeset(ctx, id)
		if err != nil {
			return nil, err
		}
		if len(cs.Parents) > 1 {
			return nil, fmt.Errorf("%w: cannot rebase merge changeset %s", ErrInvalid, id.Short())
		}
		if i > 0 && cs.FirstParent() != prevOld {
			return nil, fmt.Errorf("%w: changesets do not form a linear stack at %s", ErrInvalid, id.Short())
		}
		prevOld = id
		if cs.FirstParent() == cur {
			// Already in place: keep the original.
			res.Mapping[id.String()] = id
			cur = id
			continue
		}
		fromTree, err := r.RootTree(ctx, cs.FirstParent())
		if err != nil {
			return nil, err
		}
		ontoTree, err := r.RootTree(ctx, cur)
		if err != nil {
			return nil, err
		}
		labels := mergeLabels{base: "parent of " + id.Short(), ours: "onto " + cur.Short(), theirs: id.Short() + " " + cs.Summary()}
		changes, conflicts, err := r.pick(ctx, fromTree, ontoTree, cs.Changes, labels)
		if err != nil {
			return nil, err
		}
		if len(conflicts) > 0 {
			return nil, &ConflictError{Op: "rebase of " + id.Short(), Conflicts: conflicts}
		}
		if len(changes) == 0 {
			res.Mapping[id.String()] = object.Zero
			res.Dropped = append(res.Dropped, id)
			continue
		}
		ncs := *cs
		ncs.Parents = []object.ID{cur}
		ncs.Changes = changes
		ncs.Extra = copyExtra(cs.Extra)
		ncs.Extra[object.ExtraRebasedFrom] = id.String()
		ncs.Committer = committer
		ncs.CommitTime = time.Now().Unix()
		nid, err := r.CreateChangeset(ctx, &ncs)
		if err != nil {
			return nil, err
		}
		res.Mapping[id.String()] = nid
		cur = nid
	}
	res.Head = cur
	return res, nil
}

func copyExtra(m map[string]string) map[string]string {
	out := make(map[string]string, len(m)+1)
	for k, v := range m {
		out[k] = v
	}
	return out
}

// Revert creates a changeset on top of onto that undoes changeset target.
func (r *Repo) Revert(ctx context.Context, target, onto object.ID, author, message string) (object.ID, error) {
	cs, err := r.GetChangeset(ctx, target)
	if err != nil {
		return object.Zero, err
	}
	parentTree, err := r.RootTree(ctx, cs.FirstParent())
	if err != nil {
		return object.Zero, err
	}
	targetTree, err := r.RootTree(ctx, target)
	if err != nil {
		return object.Zero, err
	}
	// The inverse of target relative to its first parent.
	var inverse []object.Change
	for _, ch := range cs.Changes {
		v, err := r.versionAt(ctx, parentTree, ch.Path)
		if err != nil {
			return object.Zero, err
		}
		if v.mode.IsDir() {
			v = version{}
		}
		inverse = append(inverse, object.Change{Path: ch.Path, Mode: v.mode, Blob: v.id})
	}
	ontoTree, err := r.RootTree(ctx, onto)
	if err != nil {
		return object.Zero, err
	}
	labels := mergeLabels{base: target.Short(), ours: "onto " + onto.Short(), theirs: "revert of " + target.Short()}
	changes, conflicts, err := r.pick(ctx, targetTree, ontoTree, inverse, labels)
	if err != nil {
		return object.Zero, err
	}
	if len(conflicts) > 0 {
		return object.Zero, &ConflictError{Op: "revert of " + target.Short(), Conflicts: conflicts}
	}
	if message == "" {
		message = fmt.Sprintf("Revert %q\n\nThis reverts changeset %s.", cs.Summary(), target)
	}
	rev := &object.Changeset{
		Parents:    []object.ID{onto},
		Author:     author,
		AuthorTime: time.Now().Unix(),
		Message:    message,
		Changes:    changes,
		Extra:      map[string]string{object.ExtraReverts: target.String()},
	}
	return r.CreateChangeset(ctx, rev)
}

// LandResult describes a successful land.
type LandResult struct {
	Ref      string        `json:"ref"`
	Old      object.ID     `json:"old"`
	New      object.ID     `json:"new"`
	Rebase   *RebaseResult `json:"rebase,omitempty"`
	Attempts int           `json:"attempts"`
}

// ErrNothingToLand is returned when head is already contained in the ref.
var ErrNothingToLand = errors.New("nothing to land: changes are already on the target")

// Land puts the changes in head that are not yet on ref on top of ref and
// advances ref, without any global lock. If someone else lands first the
// compare-and-swap fails and Land simply rebases onto the new tip and tries
// again. Only true content conflicts stop it.
func (r *Repo) Land(ctx context.Context, ref string, head object.ID, who string, check func(newHead object.ID) error) (*LandResult, error) {
	const maxAttempts = 50
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		tip, err := r.Ref(ctx, ref)
		if errors.Is(err, store.ErrNotFound) {
			tip = object.Zero
		} else if err != nil {
			return nil, err
		}
		res := &LandResult{Ref: ref, Old: tip, Attempts: attempt}
		newHead := head
		if !tip.IsZero() {
			if anc, err := r.IsAncestor(ctx, head, tip); err != nil {
				return nil, err
			} else if anc {
				return nil, ErrNothingToLand
			}
			ff, err := r.IsAncestor(ctx, tip, head)
			if err != nil {
				return nil, err
			}
			if !ff {
				stack, err := r.Stack(ctx, head, tip)
				if err != nil {
					return nil, err
				}
				rb, err := r.RebaseStack(ctx, stack, tip, who)
				if err != nil {
					return nil, err
				}
				if rb.Head == tip {
					return nil, ErrNothingToLand
				}
				res.Rebase = rb
				newHead = rb.Head
			}
		}
		if check != nil {
			if err := check(newHead); err != nil {
				return nil, err
			}
		}
		err = r.MoveRef(ctx, ref, tip, newHead, who)
		if errors.Is(err, store.ErrCASFailed) {
			continue
		}
		if err != nil {
			return nil, err
		}
		res.New = newHead
		return res, nil
	}
	return nil, fmt.Errorf("land on %s: gave up after %d attempts due to contention", ref, maxAttempts)
}

// SortConflicts orders conflicts by path.
func SortConflicts(c []Conflict) {
	sort.Slice(c, func(i, j int) bool { return c[i].Path < c[j].Path })
}
