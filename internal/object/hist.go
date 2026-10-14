package object

import (
	"bytes"
	"fmt"
	"sort"
)

// HistEntry links a directory history node to the history node of a child.
type HistEntry struct {
	Name string `json:"name"`
	ID   ID     `json:"id"`
}

// HistNode is one version of a file or directory in the history graph.
//
// Every file and directory reachable from a changeset's root tree has a
// history node. A node is shared by all changesets in which that path has the
// same version; a new node is only created when a changeset modifies the
// path (or, for directories, anything beneath it). Following Parents walks
// the earlier versions of the same path, so path history and blame never
// need to scan unrelated changesets.
type HistNode struct {
	Mode     Mode        // ModeDir for directories, a file mode otherwise
	Content  ID          // tree or blob ID of this version
	Link     ID          // changeset that introduced this version
	Parents  []ID        // history nodes of the previous versions
	Children []HistEntry // directories only, sorted by name
}

// Find looks up a child by name.
func (h *HistNode) Find(name string) (ID, bool) {
	i := sort.Search(len(h.Children), func(i int) bool { return h.Children[i].Name >= name })
	if i < len(h.Children) && h.Children[i].Name == name {
		return h.Children[i].ID, true
	}
	return Zero, false
}

// Encode returns the canonical raw encoding.
func (h *HistNode) Encode() []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "mode %s\ncontent %s\nlink %s\n", h.Mode, h.Content, h.Link)
	for _, p := range h.Parents {
		fmt.Fprintf(&b, "parent %s\n", p)
	}
	for _, c := range h.Children {
		fmt.Fprintf(&b, "child %s %s\n", c.ID, c.Name)
	}
	return b.Bytes()
}

// EncodeHist returns the ID and stored form of h.
func EncodeHist(h *HistNode) (ID, []byte) { return Encode(KindHist, h.Encode()) }

// DecodeHist parses the encoding produced by Encode.
func DecodeHist(raw []byte) (*HistNode, error) {
	h := &HistNode{}
	for len(raw) > 0 {
		nl := bytes.IndexByte(raw, '\n')
		if nl < 0 {
			return nil, fmt.Errorf("%w: unterminated hist line", ErrCorrupt)
		}
		line := string(raw[:nl])
		raw = raw[nl+1:]
		var err error
		switch {
		case len(line) > 5 && line[:5] == "mode ":
			h.Mode, err = ParseMode(line[5:])
		case len(line) > 8 && line[:8] == "content ":
			h.Content, err = ParseID(line[8:])
		case len(line) > 5 && line[:5] == "link ":
			h.Link, err = ParseID(line[5:])
		case len(line) > 7 && line[:7] == "parent ":
			var p ID
			p, err = ParseID(line[7:])
			h.Parents = append(h.Parents, p)
		case len(line) > 6+64+1 && line[:6] == "child ":
			var c ID
			c, err = ParseID(line[6 : 6+64])
			h.Children = append(h.Children, HistEntry{Name: line[6+64+1:], ID: c})
		default:
			err = fmt.Errorf("unknown line %q", line)
		}
		if err != nil {
			return nil, fmt.Errorf("%w: hist: %v", ErrCorrupt, err)
		}
	}
	return h, nil
}
