package textdiff

import (
	"bytes"
	"strings"
)

// Conflict describes one conflicting region of a 3-way merge as line ranges
// (0-based, half-open) in each input.
type Conflict struct {
	BaseStart, BaseEnd     int
	OursStart, OursEnd     int
	TheirsStart, TheirsEnd int
}

// MergeResult is the outcome of Merge3.
type MergeResult struct {
	Merged    []byte     // merged content; conflicting regions rendered with markers
	Conflicts []Conflict // empty means clean merge
}

// Merge3 performs a diff3-style three-way line merge of ours and theirs
// relative to base.
//
// Both sides are diffed against base. Changes from either side whose base
// ranges overlap or touch (including insertions at the same point) are
// grouped into one chunk, as git does; chunks are separated by at least one
// base line that neither side changed. A chunk changed by one side only
// takes that side's version, a chunk changed identically by both sides
// takes it once, and anything else is a conflict, rendered in git's diff3
// style:
//
//	<<<<<<< oursLabel
//	ours lines
//	||||||| baseLabel
//	base lines
//	=======
//	theirs lines
//	>>>>>>> theirsLabel
//
// A marker label that is empty is omitted together with its separating
// space. If a section's last line lacks a newline, one is added before the
// next marker. If ours equals theirs, or one side equals base, the other
// side is returned without diffing. Merged never aliases the inputs.
func Merge3(base, ours, theirs []byte, baseLabel, oursLabel, theirsLabel string) MergeResult {
	switch {
	case bytes.Equal(ours, theirs), bytes.Equal(base, theirs):
		return MergeResult{Merged: bytes.Clone(ours)}
	case bytes.Equal(base, ours):
		return MergeResult{Merged: bytes.Clone(theirs)}
	}

	bl, ol, tl := SplitLines(base), SplitLines(ours), SplitLines(theirs)
	oc := changes(Lines(bl, ol))
	tc := changes(Lines(bl, tl))

	var out bytes.Buffer
	out.Grow(max(len(ours), len(theirs)) + 64)
	var conflicts []Conflict

	basePos := 0           // next base line not yet emitted
	oDelta, tDelta := 0, 0 // side index minus base index in unchanged regions
	i, j := 0, 0           // next unprocessed change of each side
	for i < len(oc) || j < len(tc) {
		// Start a chunk at the earliest pending change and absorb every
		// change of either side that overlaps or touches it.
		lo := len(bl)
		if i < len(oc) {
			lo = oc[i].aStart
		}
		if j < len(tc) {
			lo = min(lo, tc[j].aStart)
		}
		hi := lo
		i0, j0 := i, j
		for {
			if i < len(oc) && oc[i].aStart <= hi {
				hi = max(hi, oc[i].aEnd)
				i++
			} else if j < len(tc) && tc[j].aStart <= hi {
				hi = max(hi, tc[j].aEnd)
				j++
			} else {
				break
			}
		}

		writeLines(&out, bl[basePos:lo])
		basePos = hi

		oStart, oEnd := sideRange(oc[i0:i], lo, hi, oDelta)
		tStart, tEnd := sideRange(tc[j0:j], lo, hi, tDelta)
		oDelta, tDelta = oEnd-hi, tEnd-hi

		baseSec, oursSec, theirsSec := bl[lo:hi], ol[oStart:oEnd], tl[tStart:tEnd]
		switch {
		case j == j0 || equalLines(theirsSec, baseSec):
			writeLines(&out, oursSec)
		case i == i0 || equalLines(oursSec, baseSec):
			writeLines(&out, theirsSec)
		case equalLines(oursSec, theirsSec):
			writeLines(&out, oursSec)
		default:
			conflicts = append(conflicts, Conflict{
				BaseStart: lo, BaseEnd: hi,
				OursStart: oStart, OursEnd: oEnd,
				TheirsStart: tStart, TheirsEnd: tEnd,
			})
			writeMarker(&out, "<<<<<<<", oursLabel)
			writeSection(&out, oursSec)
			writeMarker(&out, "|||||||", baseLabel)
			writeSection(&out, baseSec)
			writeMarker(&out, "=======", "")
			writeSection(&out, theirsSec)
			writeMarker(&out, ">>>>>>>", theirsLabel)
		}
	}
	writeLines(&out, bl[basePos:])

	return MergeResult{Merged: out.Bytes(), Conflicts: conflicts}
}

// sideRange maps the base range [lo, hi) of a merge chunk to the
// corresponding range of one side, given that side's changes within the
// chunk (cs, possibly none) and its index offset delta before the chunk.
// Outside its changes a side is identical to base, so the mapping is a
// shift there.
func sideRange(cs []change, lo, hi, delta int) (start, end int) {
	if len(cs) == 0 {
		return lo + delta, hi + delta
	}
	first, last := cs[0], cs[len(cs)-1]
	return first.bStart - (first.aStart - lo), last.bEnd + (hi - last.aEnd)
}

func equalLines(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func writeLines(buf *bytes.Buffer, lines []string) {
	for _, l := range lines {
		buf.WriteString(l)
	}
}

// writeSection writes a conflict section, terminating its last line if it
// lacks a newline so that the following marker starts on its own line.
func writeSection(buf *bytes.Buffer, lines []string) {
	writeLines(buf, lines)
	if n := len(lines); n > 0 && !strings.HasSuffix(lines[n-1], "\n") {
		buf.WriteByte('\n')
	}
}

func writeMarker(buf *bytes.Buffer, marker, label string) {
	buf.WriteString(marker)
	if label != "" {
		buf.WriteByte(' ')
		buf.WriteString(label)
	}
	buf.WriteByte('\n')
}
