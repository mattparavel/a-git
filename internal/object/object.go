// Package object defines a-git's immutable, content-addressed data model.
//
// There are four kinds of record:
//
//   - Blobs hold file contents.
//   - Trees list the entries of one directory (name, mode, object ID).
//   - History nodes ("hist") form a Merkle structure parallel to the file tree
//     that records which changeset last touched each file or directory and
//     the previous versions it came from. They power path history and blame.
//   - Changesets are the unit of work. Unlike a git commit, a changeset does
//     not point at a full tree: it records only the files that changed
//     relative to its first parent. The root tree of a changeset is derived
//     data computed by the server.
//
// Blobs, trees and history nodes live in the object store and are addressed
// by the SHA-256 of their kind, length and raw content. Every object is
// immutable, so any cache tier can serve any object without consulting the
// authoritative server.
package object

import (
	"bytes"
	"compress/zlib"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strconv"
)

// ID is a SHA-256 content address.
type ID [32]byte

// Zero is the zero ID. It is used to mean "no object" (for example the empty
// tree of a changeset without a parent is represented by EmptyTreeID, not Zero).
var Zero ID

// IsZero reports whether id is the zero ID.
func (id ID) IsZero() bool { return id == Zero }

// String returns the full lowercase hex form of id.
func (id ID) String() string { return hex.EncodeToString(id[:]) }

// Short returns an abbreviated hex form suitable for display.
func (id ID) Short() string { return hex.EncodeToString(id[:6]) }

// MarshalText encodes id as hex; the zero ID encodes as the empty string.
func (id ID) MarshalText() ([]byte, error) {
	if id.IsZero() {
		return []byte{}, nil
	}
	return []byte(id.String()), nil
}

// UnmarshalText decodes a hex ID; the empty string decodes to Zero.
func (id *ID) UnmarshalText(b []byte) error {
	if len(b) == 0 {
		*id = Zero
		return nil
	}
	p, err := ParseID(string(b))
	if err != nil {
		return err
	}
	*id = p
	return nil
}

// ParseID parses a full 64-character hex ID.
func ParseID(s string) (ID, error) {
	var id ID
	if len(s) != 64 {
		return id, fmt.Errorf("invalid object id %q: want 64 hex characters", s)
	}
	if _, err := hex.Decode(id[:], []byte(s)); err != nil {
		return id, fmt.Errorf("invalid object id %q: %v", s, err)
	}
	return id, nil
}

// MustParseID is ParseID for constants and tests; it panics on error.
func MustParseID(s string) ID {
	id, err := ParseID(s)
	if err != nil {
		panic(err)
	}
	return id
}

// IsHexPrefix reports whether s could be an abbreviated hex ID.
func IsHexPrefix(s string) bool {
	if len(s) == 0 || len(s) > 64 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

// Kind identifies the type of a stored object.
type Kind byte

const (
	KindBlob Kind = 'b'
	KindTree Kind = 't'
	KindHist Kind = 'h'
)

func (k Kind) String() string {
	switch k {
	case KindBlob:
		return "blob"
	case KindTree:
		return "tree"
	case KindHist:
		return "hist"
	}
	return fmt.Sprintf("kind(%d)", byte(k))
}

// Valid reports whether k is a known object kind.
func (k Kind) Valid() bool { return k == KindBlob || k == KindTree || k == KindHist }

// Hash computes the ID of an object of the given kind and raw content.
func Hash(kind Kind, raw []byte) ID {
	h := sha256.New()
	h.Write([]byte(kind.String()))
	h.Write([]byte{' '})
	h.Write([]byte(strconv.Itoa(len(raw))))
	h.Write([]byte{0})
	h.Write(raw)
	var id ID
	h.Sum(id[:0])
	return id
}

// hashChangeset computes the ID of an encoded changeset record. Changesets
// use their own hash domain so they can never collide with stored objects.
func hashChangeset(enc []byte) ID {
	h := sha256.New()
	h.Write([]byte("changeset "))
	h.Write([]byte(strconv.Itoa(len(enc))))
	h.Write([]byte{0})
	h.Write(enc)
	var id ID
	h.Sum(id[:0])
	return id
}

// Stored form
//
// Objects are stored, cached and sent over the wire in exactly the same
// "stored form" so no tier ever needs to re-encode them:
//
//	byte 0   kind ('b', 't', 'h')
//	byte 1   encoding (0 = raw, 1 = zlib)
//	byte 2.. payload
//
// There is no delta compression: each object stands alone, which keeps
// fetches independent, cacheable and resumable.

const (
	encRaw  byte = 0
	encZlib byte = 1

	// compressThreshold is the size below which compression is skipped.
	compressThreshold = 64
)

// ErrCorrupt is returned when a stored object fails to decode or verify.
var ErrCorrupt = errors.New("corrupt object")

// Encode returns the ID and stored form of an object.
func Encode(kind Kind, raw []byte) (ID, []byte) {
	id := Hash(kind, raw)
	if len(raw) < compressThreshold {
		out := make([]byte, 2+len(raw))
		out[0], out[1] = byte(kind), encRaw
		copy(out[2:], raw)
		return id, out
	}
	var buf bytes.Buffer
	buf.Grow(len(raw)/2 + 16)
	buf.WriteByte(byte(kind))
	buf.WriteByte(encZlib)
	zw, _ := zlib.NewWriterLevel(&buf, zlib.BestSpeed)
	zw.Write(raw)
	zw.Close()
	// Incompressible content is stored raw.
	if buf.Len() >= len(raw)+2 {
		out := make([]byte, 2+len(raw))
		out[0], out[1] = byte(kind), encRaw
		copy(out[2:], raw)
		return id, out
	}
	return id, buf.Bytes()
}

// Decode decodes a stored object into its kind and raw content.
func Decode(stored []byte) (Kind, []byte, error) {
	if len(stored) < 2 {
		return 0, nil, fmt.Errorf("%w: short object", ErrCorrupt)
	}
	kind := Kind(stored[0])
	if !kind.Valid() {
		return 0, nil, fmt.Errorf("%w: unknown kind %d", ErrCorrupt, stored[0])
	}
	switch stored[1] {
	case encRaw:
		return kind, stored[2:], nil
	case encZlib:
		zr, err := zlib.NewReader(bytes.NewReader(stored[2:]))
		if err != nil {
			return 0, nil, fmt.Errorf("%w: %v", ErrCorrupt, err)
		}
		raw, err := io.ReadAll(zr)
		if err != nil {
			return 0, nil, fmt.Errorf("%w: %v", ErrCorrupt, err)
		}
		return kind, raw, nil
	}
	return 0, nil, fmt.Errorf("%w: unknown encoding %d", ErrCorrupt, stored[1])
}

// KindOf returns the kind of a stored object without decoding it.
func KindOf(stored []byte) Kind {
	if len(stored) == 0 {
		return 0
	}
	return Kind(stored[0])
}

// Verify decodes stored and checks that it hashes to id.
func Verify(id ID, stored []byte) (Kind, []byte, error) {
	kind, raw, err := Decode(stored)
	if err != nil {
		return 0, nil, err
	}
	if got := Hash(kind, raw); got != id {
		return 0, nil, fmt.Errorf("%w: object %s hashes to %s", ErrCorrupt, id.Short(), got.Short())
	}
	return kind, raw, nil
}
