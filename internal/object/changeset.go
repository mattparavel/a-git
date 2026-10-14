package object

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Change records the new state of one file relative to a changeset's first
// parent. A change with Mode == ModeNone deletes the file.
type Change struct {
	Path string `json:"path"`
	Mode Mode   `json:"mode,omitzero"`
	Blob ID     `json:"blob,omitzero"`
}

// IsDelete reports whether the change removes the file.
func (c Change) IsDelete() bool { return c.Mode == ModeNone }

// Changeset is the unit of work in a-git. It carries roughly what a git
// commit does, except that instead of naming a complete tree it lists only the
// files that differ from the first parent. For merges the first parent is the
// baseline the changes apply to.
type Changeset struct {
	Parents    []ID              `json:"parents"`
	Author     string            `json:"author"`
	AuthorTime int64             `json:"author_time"`
	AuthorTZ   int               `json:"author_tz,omitzero"` // minutes east of UTC
	Committer  string            `json:"committer,omitzero"`
	CommitTime int64             `json:"commit_time,omitzero"`
	CommitTZ   int               `json:"commit_tz,omitzero"`
	Message    string            `json:"message"`
	Changes    []Change          `json:"changes"`
	Extra      map[string]string `json:"extra,omitzero"`
}

// Well-known Extra keys.
const (
	ExtraGitCommit   = "git-commit"   // original git commit hash for imported history
	ExtraRebasedFrom = "rebased-from" // changeset this one was rebased/cherry-picked from
	ExtraReverts     = "reverts"      // changeset reverted by this one
	ExtraReview      = "review"       // review that landed this changeset
)

// FirstParent returns the first parent, or Zero for a root changeset.
func (c *Changeset) FirstParent() ID {
	if len(c.Parents) == 0 {
		return Zero
	}
	return c.Parents[0]
}

// Normalize sorts the change list into canonical order.
func (c *Changeset) Normalize() {
	sort.Slice(c.Changes, func(i, j int) bool { return c.Changes[i].Path < c.Changes[j].Path })
	if c.Parents == nil {
		c.Parents = []ID{}
	}
	if c.Changes == nil {
		c.Changes = []Change{}
	}
}

// Validate checks structural invariants. It does not check that referenced
// parents or blobs exist; the server does that.
func (c *Changeset) Validate() error {
	seenParent := map[ID]bool{}
	for _, p := range c.Parents {
		if p.IsZero() {
			return fmt.Errorf("zero parent id")
		}
		if seenParent[p] {
			return fmt.Errorf("duplicate parent %s", p.Short())
		}
		seenParent[p] = true
	}
	if strings.TrimSpace(c.Author) == "" {
		return fmt.Errorf("missing author")
	}
	for i, ch := range c.Changes {
		if err := ValidatePath(ch.Path); err != nil {
			return err
		}
		if i > 0 && c.Changes[i-1].Path >= ch.Path {
			return fmt.Errorf("changes not sorted or duplicate path %q", ch.Path)
		}
		if ch.IsDelete() {
			if !ch.Blob.IsZero() {
				return fmt.Errorf("delete of %q must not carry a blob", ch.Path)
			}
			continue
		}
		if !ch.Mode.IsFile() {
			return fmt.Errorf("change to %q has non-file mode %s", ch.Path, ch.Mode)
		}
		if ch.Blob.IsZero() {
			return fmt.Errorf("change to %q is missing a blob", ch.Path)
		}
	}
	// A path cannot be both a file and the parent directory of another set path.
	for i := 0; i+1 < len(c.Changes); i++ {
		a := c.Changes[i]
		if a.IsDelete() {
			continue
		}
		for j := i + 1; j < len(c.Changes) && strings.HasPrefix(c.Changes[j].Path, a.Path); j++ {
			b := c.Changes[j]
			if HasPathPrefix(b.Path, a.Path) && !b.IsDelete() {
				return fmt.Errorf("path %q is both a file and a directory", a.Path)
			}
		}
	}
	return nil
}

// Encode returns the canonical JSON encoding of the changeset.
func (c *Changeset) Encode() []byte {
	cp := *c
	cp.Normalize()
	b, err := json.Marshal(&cp)
	if err != nil {
		panic(err) // all fields are plain data
	}
	return b
}

// ID returns the content address of the changeset.
func (c *Changeset) ID() ID { return hashChangeset(c.Encode()) }

// EncodeChangeset returns the ID and canonical encoding of c.
func EncodeChangeset(c *Changeset) (ID, []byte) {
	enc := c.Encode()
	return hashChangeset(enc), enc
}

// DecodeChangeset parses an encoded changeset.
func DecodeChangeset(b []byte) (*Changeset, error) {
	var c Changeset
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("%w: changeset: %v", ErrCorrupt, err)
	}
	c.Normalize()
	return &c, nil
}

// VerifyChangeset decodes b and checks it hashes to id.
func VerifyChangeset(id ID, b []byte) (*Changeset, error) {
	if got := hashChangeset(b); got != id {
		return nil, fmt.Errorf("%w: changeset %s hashes to %s", ErrCorrupt, id.Short(), got.Short())
	}
	return DecodeChangeset(b)
}

// Summary returns the first line of the message.
func (c *Changeset) Summary() string {
	s := c.Message
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}
