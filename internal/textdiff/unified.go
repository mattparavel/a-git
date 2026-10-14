package textdiff

import (
	"strconv"
	"strings"
)

// change is a maximal run of non-Equal ops: A[aStart:aEnd] is replaced by
// B[bStart:bEnd]. Either range may be empty, but not both.
type change struct {
	aStart, aEnd int
	bStart, bEnd int
}

// changes collapses the Delete/Insert ops of an edit script into changes.
func changes(ops []Op) []change {
	var cs []change
	prevEqual := true
	for _, op := range ops {
		if op.Kind == Equal {
			prevEqual = true
			continue
		}
		if !prevEqual {
			last := &cs[len(cs)-1]
			last.aEnd, last.bEnd = op.AEnd, op.BEnd
		} else {
			cs = append(cs, change{op.AStart, op.AEnd, op.BStart, op.BEnd})
		}
		prevEqual = false
	}
	return cs
}

// Unified renders a unified diff of a and b (like `diff -u` or git) with the
// given number of context lines (negative values are treated as 0).
//
// The output starts with "--- aName\n+++ bName\n", followed by hunks headed
// "@@ -l,s +l,s @@". As in GNU diff and git, l is 1-based, an empty range is
// reported as the line before it with count 0, and ",s" is omitted when the
// count is 1. Hunks whose changes are separated by at most 2*context
// unchanged lines are merged. Lines are prefixed with ' ', '-' or '+'; a
// line lacking a trailing newline is followed by "\n\\ No newline at end of
// file\n". If a and b are identical the result is "".
func Unified(aName, bName string, a, b []string, context int) string {
	context = max(context, 0)
	cs := changes(Lines(a, b))
	if len(cs) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("--- ")
	sb.WriteString(aName)
	sb.WriteString("\n+++ ")
	sb.WriteString(bName)
	sb.WriteString("\n")

	for len(cs) > 0 {
		// Gather the changes belonging to this hunk.
		k := 1
		for k < len(cs) && cs[k].aStart-cs[k-1].aEnd <= 2*context {
			k++
		}
		group := cs[:k]
		cs = cs[k:]

		// The regions around a group are unchanged, so leading and trailing
		// context have the same length in a and b.
		first, last := group[0], group[len(group)-1]
		before := min(context, first.aStart)
		after := min(context, len(a)-last.aEnd)
		aStart, aEnd := first.aStart-before, last.aEnd+after
		bStart, bEnd := first.bStart-before, last.bEnd+after

		sb.WriteString("@@ -")
		writeRange(&sb, aStart, aEnd-aStart)
		sb.WriteString(" +")
		writeRange(&sb, bStart, bEnd-bStart)
		sb.WriteString(" @@\n")

		pos := aStart
		for _, c := range group {
			writeDiffLines(&sb, ' ', a[pos:c.aStart])
			writeDiffLines(&sb, '-', a[c.aStart:c.aEnd])
			writeDiffLines(&sb, '+', b[c.bStart:c.bEnd])
			pos = c.aEnd
		}
		writeDiffLines(&sb, ' ', a[pos:aEnd])
	}
	return sb.String()
}

// writeRange writes a hunk-header range in GNU/git notation.
func writeRange(sb *strings.Builder, start, count int) {
	if count == 0 {
		// An empty range is identified by the line just before it.
		sb.WriteString(strconv.Itoa(start))
		sb.WriteString(",0")
		return
	}
	sb.WriteString(strconv.Itoa(start + 1))
	if count != 1 {
		sb.WriteByte(',')
		sb.WriteString(strconv.Itoa(count))
	}
}

func writeDiffLines(sb *strings.Builder, prefix byte, lines []string) {
	for _, l := range lines {
		sb.WriteByte(prefix)
		sb.WriteString(l)
		if !strings.HasSuffix(l, "\n") {
			sb.WriteString("\n\\ No newline at end of file\n")
		}
	}
}
