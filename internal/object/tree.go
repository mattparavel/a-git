package object

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Mode is a file mode in the git-compatible octal convention.
type Mode uint32

const (
	ModeNone Mode = 0
	ModeFile Mode = 0o100644
	ModeExec Mode = 0o100755
	ModeLink Mode = 0o120000
	ModeDir  Mode = 0o040000
)

// IsDir reports whether m is a directory mode.
func (m Mode) IsDir() bool { return m == ModeDir }

// IsFile reports whether m is a regular, executable or symlink file mode.
func (m Mode) IsFile() bool { return m == ModeFile || m == ModeExec || m == ModeLink }

// Valid reports whether m is one of the supported modes.
func (m Mode) Valid() bool { return m.IsFile() || m.IsDir() }

func (m Mode) String() string { return fmt.Sprintf("%06o", uint32(m)) }

// MarshalText encodes the mode as six octal digits.
func (m Mode) MarshalText() ([]byte, error) { return []byte(m.String()), nil }

// UnmarshalText decodes an octal mode.
func (m *Mode) UnmarshalText(b []byte) error {
	v, err := ParseMode(string(b))
	if err != nil {
		return err
	}
	*m = v
	return nil
}

// ParseMode parses an octal mode string.
func ParseMode(s string) (Mode, error) {
	v, err := strconv.ParseUint(s, 8, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid mode %q", s)
	}
	m := Mode(v)
	if m != ModeNone && !m.Valid() {
		return 0, fmt.Errorf("unsupported mode %q", s)
	}
	return m, nil
}

// TreeEntry is one entry of a directory.
type TreeEntry struct {
	Name string `json:"name"`
	Mode Mode   `json:"mode"`
	ID   ID     `json:"id"`
}

// Tree is a directory listing sorted by name (bytewise).
type Tree struct {
	Entries []TreeEntry
}

// EmptyTreeID is the ID of the tree with no entries.
var EmptyTreeID = Hash(KindTree, nil)

// Sort orders the entries canonically.
func (t *Tree) Sort() {
	sort.Slice(t.Entries, func(i, j int) bool { return t.Entries[i].Name < t.Entries[j].Name })
}

// Find looks up an entry by name.
func (t *Tree) Find(name string) (TreeEntry, bool) {
	i := sort.Search(len(t.Entries), func(i int) bool { return t.Entries[i].Name >= name })
	if i < len(t.Entries) && t.Entries[i].Name == name {
		return t.Entries[i], true
	}
	return TreeEntry{}, false
}

// Encode returns the canonical raw encoding of the tree:
// one "<mode> <hex id> <name>\n" line per entry, sorted by name.
func (t *Tree) Encode() []byte {
	var b bytes.Buffer
	b.Grow(len(t.Entries) * 90)
	for _, e := range t.Entries {
		b.WriteString(e.Mode.String())
		b.WriteByte(' ')
		b.WriteString(e.ID.String())
		b.WriteByte(' ')
		b.WriteString(e.Name)
		b.WriteByte('\n')
	}
	return b.Bytes()
}

// DecodeTree parses the raw encoding produced by Encode.
func DecodeTree(raw []byte) (*Tree, error) {
	t := &Tree{}
	if len(raw) == 0 {
		return t, nil
	}
	t.Entries = make([]TreeEntry, 0, bytes.Count(raw, []byte{'\n'}))
	prev := ""
	for len(raw) > 0 {
		nl := bytes.IndexByte(raw, '\n')
		if nl < 0 {
			return nil, fmt.Errorf("%w: unterminated tree entry", ErrCorrupt)
		}
		line := raw[:nl]
		raw = raw[nl+1:]
		// 6 mode + 1 + 64 id + 1 + name
		if len(line) < 6+1+64+1+1 || line[6] != ' ' || line[71] != ' ' {
			return nil, fmt.Errorf("%w: malformed tree entry %q", ErrCorrupt, line)
		}
		mode, err := ParseMode(string(line[:6]))
		if err != nil || mode == ModeNone {
			return nil, fmt.Errorf("%w: bad mode in tree entry", ErrCorrupt)
		}
		id, err := ParseID(string(line[7:71]))
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrCorrupt, err)
		}
		name := string(line[72:])
		if name <= prev && len(t.Entries) > 0 {
			return nil, fmt.Errorf("%w: tree entries out of order", ErrCorrupt)
		}
		prev = name
		t.Entries = append(t.Entries, TreeEntry{Name: name, Mode: mode, ID: id})
	}
	return t, nil
}

// EncodeTree encodes t and returns its ID and stored form.
func EncodeTree(t *Tree) (ID, []byte) { return Encode(KindTree, t.Encode()) }

// ValidatePath checks that p is a clean, relative, slash-separated path.
// The top-level ".agit" directory is reserved for working-copy metadata.
func ValidatePath(p string) error {
	if p == "" {
		return fmt.Errorf("empty path")
	}
	if strings.HasPrefix(p, "/") || strings.HasSuffix(p, "/") {
		return fmt.Errorf("path %q must be relative without trailing slash", p)
	}
	if strings.ContainsAny(p, "\x00\n\r\\") {
		return fmt.Errorf("path %q contains forbidden characters", p)
	}
	for i, c := range strings.Split(p, "/") {
		if c == "" || c == "." || c == ".." {
			return fmt.Errorf("path %q has an invalid component", p)
		}
		if i == 0 && c == ".agit" {
			return fmt.Errorf("path %q is inside the reserved .agit directory", p)
		}
	}
	return nil
}

// CleanPath normalises a user-supplied repository path ("./a//b/" -> "a/b").
// The empty string denotes the repository root.
func CleanPath(p string) string {
	parts := strings.Split(strings.ReplaceAll(p, "\\", "/"), "/")
	out := parts[:0]
	for _, c := range parts {
		switch c {
		case "", ".":
			continue
		case "..":
			if len(out) > 0 {
				out = out[:len(out)-1]
			}
			continue
		}
		out = append(out, c)
	}
	return strings.Join(out, "/")
}

// JoinPath joins a directory prefix and a name.
func JoinPath(dir, name string) string {
	if dir == "" {
		return name
	}
	if name == "" {
		return dir
	}
	return dir + "/" + name
}

// HasPathPrefix reports whether p equals prefix or lies beneath it.
// The empty prefix matches everything.
func HasPathPrefix(p, prefix string) bool {
	if prefix == "" || p == prefix {
		return true
	}
	return strings.HasPrefix(p, prefix) && p[len(prefix)] == '/'
}
