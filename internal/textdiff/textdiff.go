// Package textdiff is the line-oriented diff and merge engine.
//
// It provides:
//
//   - Lines / Bytes: a line diff based on Myers' O(ND) algorithm in its
//     linear-space ("middle snake") form, with common prefix/suffix
//     stripping, line interning, discarding of lines unique to one side,
//     a cost cap that keeps pathological inputs tractable, and GNU-diff
//     style boundary shifting for more readable output.
//   - Unified: git/GNU compatible unified diff rendering.
//   - LineMap: carry-over mapping between versions, used to propagate blame.
//   - Merge3: a diff3-style three-way merge with git-style conflict markers.
//
// All functions are safe for concurrent use.
package textdiff

import (
	"bytes"
	"strings"
)

// SplitLines splits content into lines, each line INCLUDING its trailing
// "\n" if present. The final line may lack a trailing newline. Empty input
// yields nil (zero lines).
//
// The content is copied once; the returned lines are substrings of that copy.
func SplitLines(b []byte) []string {
	if len(b) == 0 {
		return nil
	}
	s := string(b)
	n := strings.Count(s, "\n")
	if s[len(s)-1] != '\n' {
		n++
	}
	lines := make([]string, 0, n)
	for len(s) > 0 {
		i := strings.IndexByte(s, '\n')
		if i < 0 {
			lines = append(lines, s)
			break
		}
		lines = append(lines, s[:i+1])
		s = s[i+1:]
	}
	return lines
}

// OpKind is the kind of an edit-script hunk.
type OpKind int

// Edit-script hunk kinds.
const (
	Equal OpKind = iota
	Delete
	Insert
)

// String returns a short name for the kind.
func (k OpKind) String() string {
	switch k {
	case Equal:
		return "Equal"
	case Delete:
		return "Delete"
	case Insert:
		return "Insert"
	}
	return "OpKind(?)"
}

// Op is a contiguous run in an edit script transforming A into B.
//
// For Equal: A[AStart:AEnd] == B[BStart:BEnd].
// For Delete: A[AStart:AEnd] is removed; BStart == BEnd is the position in B.
// For Insert: B[BStart:BEnd] is inserted; AStart == AEnd is the position in A.
type Op struct {
	Kind         OpKind
	AStart, AEnd int
	BStart, BEnd int
}

// Lines computes a line diff between a and b. Lines are compared as whole
// strings.
//
// The result is minimal (the Equal ops form a longest common subsequence)
// except for sub-problems whose edit distance exceeds an internal cost bound,
// where a cheaper, valid but possibly non-minimal path is chosen so that the
// worst case stays bounded. Memory use is linear in len(a)+len(b).
//
// The returned ops are in order, cover a and b completely, and adjacent ops
// never share a kind. Within a changed region the Delete op precedes the
// Insert op. If both inputs are empty the result is empty.
func Lines(a, b []string) []Op {
	n, m := len(a), len(b)

	// Common prefix and suffix are handled without interning.
	pre := 0
	for pre < n && pre < m && a[pre] == b[pre] {
		pre++
	}
	suf := 0
	for suf < n-pre && suf < m-pre && a[n-1-suf] == b[m-1-suf] {
		suf++
	}

	var ops []Op
	ops = appendOp(ops, Op{Kind: Equal, AStart: 0, AEnd: pre, BStart: 0, BEnd: pre})
	ra, rb := diffMiddle(a[pre:n-suf], b[pre:m-suf])
	ops = appendChangeOps(ops, ra, rb, pre, pre)
	ops = appendOp(ops, Op{Kind: Equal, AStart: n - suf, AEnd: n, BStart: m - suf, BEnd: m})
	return ops
}

// Bytes is a convenience for Lines(SplitLines(a), SplitLines(b)).
func Bytes(a, b []byte) []Op {
	return Lines(SplitLines(a), SplitLines(b))
}

// Stats returns the number of inserted and deleted lines in ops.
func Stats(ops []Op) (added, deleted int) {
	for _, op := range ops {
		switch op.Kind {
		case Insert:
			added += op.BEnd - op.BStart
		case Delete:
			deleted += op.AEnd - op.AStart
		}
	}
	return added, deleted
}

// LineMap returns, for every line index j of b, the index i of the line in a
// that it was carried over from (according to the Equal ops of Lines(a, b)),
// or -1 if line j of b is new. It is used to propagate blame.
func LineMap(a, b []string) []int {
	m := make([]int, len(b))
	for j := range m {
		m[j] = -1
	}
	for _, op := range Lines(a, b) {
		if op.Kind != Equal {
			continue
		}
		for k := 0; k < op.AEnd-op.AStart; k++ {
			m[op.BStart+k] = op.AStart + k
		}
	}
	return m
}

// binarySniffLen is how much of a file IsBinary inspects (the same as git).
const binarySniffLen = 8000

// IsBinary reports whether content looks binary, i.e. contains a NUL byte in
// its first 8000 bytes.
func IsBinary(b []byte) bool {
	if len(b) > binarySniffLen {
		b = b[:binarySniffLen]
	}
	return bytes.IndexByte(b, 0) >= 0
}

// appendOp appends op to ops, dropping it if empty and coalescing it with the
// previous op when they have the same kind and are contiguous.
func appendOp(ops []Op, op Op) []Op {
	if op.AStart == op.AEnd && op.BStart == op.BEnd {
		return ops
	}
	if n := len(ops); n > 0 {
		last := &ops[n-1]
		if last.Kind == op.Kind && last.AEnd == op.AStart && last.BEnd == op.BStart {
			last.AEnd, last.BEnd = op.AEnd, op.BEnd
			return ops
		}
	}
	return append(ops, op)
}

// appendChangeOps converts per-line change flags (ra[i]: line i of A was
// deleted; rb[j]: line j of B was inserted) into ops, offsetting all
// positions by aoff/boff. Unchanged lines of A and B are paired in order;
// the caller guarantees that both sides have the same number of them.
func appendChangeOps(ops []Op, ra, rb []bool, aoff, boff int) []Op {
	n, m := len(ra), len(rb)
	i, j := 0, 0
	for i < n || j < m {
		switch {
		case i < n && ra[i]:
			s := i
			for i < n && ra[i] {
				i++
			}
			ops = appendOp(ops, Op{Kind: Delete, AStart: aoff + s, AEnd: aoff + i, BStart: boff + j, BEnd: boff + j})
		case j < m && rb[j]:
			s := j
			for j < m && rb[j] {
				j++
			}
			ops = appendOp(ops, Op{Kind: Insert, AStart: aoff + i, AEnd: aoff + i, BStart: boff + s, BEnd: boff + j})
		default:
			si, sj := i, j
			for i < n && j < m && !ra[i] && !rb[j] {
				i++
				j++
			}
			if i == si {
				panic("textdiff: internal error: unbalanced change flags")
			}
			ops = appendOp(ops, Op{Kind: Equal, AStart: aoff + si, AEnd: aoff + i, BStart: boff + sj, BEnd: boff + j})
		}
	}
	return ops
}
