package textdiff

import (
	"fmt"
	"math/rand/v2"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

// lines builds a line slice from its arguments, appending "\n" to each.
func lines(ss ...string) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = s + "\n"
	}
	return out
}

// numbered returns lines "1\n" .. "n\n".
func numbered(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("%d\n", i+1)
	}
	return out
}

// checkOps verifies that ops is a well-formed, coalesced edit script that
// transforms a into b, and returns the number of Equal lines.
func checkOps(t *testing.T, a, b []string, ops []Op) int {
	t.Helper()
	var out []string
	ai, bi, equal := 0, 0, 0
	for k, op := range ops {
		if op.AStart != ai || op.BStart != bi {
			t.Fatalf("op %d %+v not contiguous with position (%d,%d)", k, op, ai, bi)
		}
		if op.AEnd < op.AStart || op.BEnd < op.BStart || (op.AEnd == op.AStart && op.BEnd == op.BStart) {
			t.Fatalf("op %d %+v has an empty or negative range", k, op)
		}
		if k > 0 && ops[k-1].Kind == op.Kind {
			t.Fatalf("ops %d and %d not coalesced: %+v %+v", k-1, k, ops[k-1], op)
		}
		switch op.Kind {
		case Equal:
			if op.AEnd-op.AStart != op.BEnd-op.BStart {
				t.Fatalf("op %d %+v: Equal ranges differ in length", k, op)
			}
			for x := op.AStart; x < op.AEnd; x++ {
				if a[x] != b[op.BStart+x-op.AStart] {
					t.Fatalf("op %d %+v: a[%d]=%q != b[%d]=%q", k, op, x, a[x], op.BStart+x-op.AStart, b[op.BStart+x-op.AStart])
				}
			}
			out = append(out, a[op.AStart:op.AEnd]...)
			equal += op.AEnd - op.AStart
		case Delete:
			if op.BStart != op.BEnd {
				t.Fatalf("op %d %+v: Delete with non-empty B range", k, op)
			}
		case Insert:
			if op.AStart != op.AEnd {
				t.Fatalf("op %d %+v: Insert with non-empty A range", k, op)
			}
			out = append(out, b[op.BStart:op.BEnd]...)
		default:
			t.Fatalf("op %d has bad kind %v", k, op.Kind)
		}
		ai, bi = op.AEnd, op.BEnd
	}
	if ai != len(a) || bi != len(b) {
		t.Fatalf("ops end at (%d,%d), want (%d,%d)", ai, bi, len(a), len(b))
	}
	if !reflect.DeepEqual(out, b) && !(len(out) == 0 && len(b) == 0) {
		t.Fatalf("applying ops does not yield b")
	}
	return equal
}

// lcsLen computes the length of the longest common subsequence by dynamic
// programming, as a reference for minimality.
func lcsLen(a, b []string) int {
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			switch {
			case a[i-1] == b[j-1]:
				cur[j] = prev[j-1] + 1
			case prev[j] >= cur[j-1]:
				cur[j] = prev[j]
			default:
				cur[j] = cur[j-1]
			}
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}

func randLines(r *rand.Rand, n, alphabet int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = string(rune('a'+r.IntN(alphabet))) + "\n"
	}
	return out
}

// mutate returns a randomly edited copy of a, drawing new lines from the
// same alphabet, so that the two inputs share structure.
func mutate(r *rand.Rand, a []string, alphabet int) []string {
	var out []string
	for _, l := range a {
		switch r.IntN(6) {
		case 0: // delete
		case 1: // replace
			out = append(out, randLines(r, 1, alphabet)...)
		case 2: // insert before
			out = append(out, randLines(r, 1+r.IntN(3), alphabet)...)
			out = append(out, l)
		default:
			out = append(out, l)
		}
	}
	if r.IntN(3) == 0 {
		out = append(out, randLines(r, r.IntN(4), alphabet)...)
	}
	return out
}

func TestSplitLines(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"a", []string{"a"}},
		{"\n", []string{"\n"}},
		{"a\n", []string{"a\n"}},
		{"a\nb", []string{"a\n", "b"}},
		{"a\nb\n", []string{"a\n", "b\n"}},
		{"a\n\nb\n\n", []string{"a\n", "\n", "b\n", "\n"}},
		{"a\r\nb", []string{"a\r\n", "b"}},
	}
	for _, tt := range tests {
		got := SplitLines([]byte(tt.in))
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("SplitLines(%q) = %q, want %q", tt.in, got, tt.want)
		}
		if strings.Join(got, "") != tt.in {
			t.Errorf("SplitLines(%q) does not round-trip", tt.in)
		}
	}
}

func TestLinesExact(t *testing.T) {
	tests := []struct {
		name string
		a, b []string
		want []Op
	}{
		{"both empty", nil, nil, nil},
		{"identical", lines("a", "b"), lines("a", "b"), []Op{{Equal, 0, 2, 0, 2}}},
		{"insert into empty", nil, lines("a", "b"), []Op{{Insert, 0, 0, 0, 2}}},
		{"delete all", lines("a", "b"), nil, []Op{{Delete, 0, 2, 0, 0}}},
		{"replace middle", lines("a", "b", "c"), lines("a", "x", "c"),
			[]Op{{Equal, 0, 1, 0, 1}, {Delete, 1, 2, 1, 1}, {Insert, 2, 2, 1, 2}, {Equal, 2, 3, 2, 3}}},
		{"insert at start", lines("b", "c"), lines("a", "b", "c"),
			[]Op{{Insert, 0, 0, 0, 1}, {Equal, 0, 2, 1, 3}}},
		{"delete at end", lines("a", "b", "c"), lines("a", "b"),
			[]Op{{Equal, 0, 2, 0, 2}, {Delete, 2, 3, 2, 2}}},
		{"total rewrite", lines("a", "b"), lines("c", "d", "e"),
			[]Op{{Delete, 0, 2, 0, 0}, {Insert, 2, 2, 0, 3}}},
		{"newline change", []string{"a\n", "b"}, lines("a", "b"),
			[]Op{{Equal, 0, 1, 0, 1}, {Delete, 1, 2, 1, 1}, {Insert, 2, 2, 1, 2}}},
		{"insertion slides down", lines("a", "b", "c"), lines("a", "b", "x", "b", "c"),
			[]Op{{Equal, 0, 2, 0, 2}, {Insert, 2, 2, 2, 4}, {Equal, 2, 3, 4, 5}}},
		{"blank-line block", lines("f() {", "}", "", "g() {", "}"),
			lines("f() {", "}", "", "h() {", "}", "", "g() {", "}"),
			[]Op{{Equal, 0, 3, 0, 3}, {Insert, 3, 3, 3, 6}, {Equal, 3, 5, 6, 8}}},
		{"interleaved", lines("a", "b", "c", "d", "e"), lines("b", "x", "d", "e", "y"),
			[]Op{{Delete, 0, 1, 0, 0}, {Equal, 1, 2, 0, 1}, {Delete, 2, 3, 1, 1}, {Insert, 3, 3, 1, 2},
				{Equal, 3, 5, 2, 4}, {Insert, 5, 5, 4, 5}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Lines(tt.a, tt.b)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Lines = %+v\nwant    %+v", got, tt.want)
			}
			checkOps(t, tt.a, tt.b, got)
		})
	}
}

func TestLinesRandomMinimal(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	for iter := 0; iter < 3000; iter++ {
		alphabet := 1 + r.IntN(12)
		a := randLines(r, r.IntN(201), alphabet)
		var b []string
		if r.IntN(2) == 0 {
			b = randLines(r, r.IntN(201), alphabet)
		} else {
			b = mutate(r, a, alphabet+r.IntN(3)) // sometimes adds lines unique to b
		}
		ops := Lines(a, b)
		equal := checkOps(t, a, b, ops)
		if want := lcsLen(a, b); equal != want {
			t.Fatalf("iter %d: %d equal lines, LCS is %d\na=%q\nb=%q", iter, equal, want, a, b)
		}
		added, deleted := Stats(ops)
		if added != len(b)-equal || deleted != len(a)-equal {
			t.Fatalf("iter %d: Stats = (%d,%d), want (%d,%d)", iter, added, deleted, len(b)-equal, len(a)-equal)
		}
	}
}

// TestLinesHeuristicValid forces the cost-limit fallback on small inputs and
// checks that the resulting (possibly non-minimal) scripts are still valid.
func TestLinesHeuristicValid(t *testing.T) {
	defer func(lo, hi int) { minCostLimit, maxCostLimit = lo, hi }(minCostLimit, maxCostLimit)

	r := rand.New(rand.NewPCG(3, 4))
	nonMinimal := 0
	for iter := 0; iter < 2000; iter++ {
		minCostLimit = 1 + r.IntN(32)
		maxCostLimit = minCostLimit
		alphabet := 2 + r.IntN(6)
		a := randLines(r, r.IntN(400), alphabet)
		b := randLines(r, r.IntN(400), alphabet)
		if r.IntN(2) == 0 {
			b = mutate(r, a, alphabet)
		}
		equal := checkOps(t, a, b, Lines(a, b))
		if equal < lcsLen(a, b) {
			nonMinimal++
		}
	}
	if nonMinimal == 0 {
		t.Errorf("cost-limit heuristic was never exercised")
	}
	t.Logf("%d non-minimal results", nonMinimal)
}

// timeLimit scales a wall-clock budget for slow test configurations.
func timeLimit(d time.Duration) time.Duration {
	if raceEnabled {
		return 10 * d
	}
	return d
}

func TestLinesLargeFewEdits(t *testing.T) {
	const n = 100_000
	a := make([]string, n)
	for i := range a {
		a[i] = fmt.Sprintf("line %d of a fairly large file\n", i)
	}
	b := make([]string, 0, n+10)
	for i := 0; i < n; i++ {
		switch {
		case i == 10 || i == 50_000 || i == n-10:
			b = append(b, fmt.Sprintf("modified %d\n", i))
		case i >= 30_000 && i < 30_003:
			// deleted
		case i == 70_000:
			b = append(b, "new 1\n", "new 2\n", "new 3\n", "new 4\n", "new 5\n", a[i])
		default:
			b = append(b, a[i])
		}
	}

	start := time.Now()
	ops := Lines(a, b)
	elapsed := time.Since(start)
	checkOps(t, a, b, ops)
	added, deleted := Stats(ops)
	if added != 8 || deleted != 6 {
		t.Errorf("Stats = (%d,%d), want (8,6)", added, deleted)
	}
	if limit := timeLimit(time.Second); elapsed > limit {
		t.Errorf("diff of %d lines took %v, want < %v", n, elapsed, limit)
	}
}

func TestLinesLargeRewrite(t *testing.T) {
	const n = 20_000
	a := make([]string, n)
	b := make([]string, n)
	for i := range a {
		a[i] = fmt.Sprintf("old line %d\n", i)
		b[i] = fmt.Sprintf("new line %d\n", i)
	}

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	start := time.Now()
	ops := Lines(a, b)
	elapsed := time.Since(start)
	runtime.ReadMemStats(&after)

	want := []Op{{Delete, 0, n, 0, 0}, {Insert, n, n, 0, n}}
	if !reflect.DeepEqual(ops, want) {
		t.Fatalf("Lines = %+v, want %+v", ops, want)
	}
	if limit := timeLimit(time.Second); elapsed > limit {
		t.Errorf("rewrite of %d lines took %v, want < %v", n, elapsed, limit)
	}
	// Linear memory is a few MB here; anything quadratic would be gigabytes.
	if alloc := after.TotalAlloc - before.TotalAlloc; alloc > 32<<20 {
		t.Errorf("rewrite of %d lines allocated %d bytes", n, alloc)
	}
}

// TestLinesLargeChurn diffs two large unrelated files drawn from a small
// alphabet, where the edit distance is huge and the cost limit must keep
// the running time bounded.
func TestLinesLargeChurn(t *testing.T) {
	r := rand.New(rand.NewPCG(5, 6))
	const n = 20_000
	a := randLines(r, n, 16)
	b := randLines(r, n, 16)

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	start := time.Now()
	ops := Lines(a, b)
	elapsed := time.Since(start)
	runtime.ReadMemStats(&after)

	checkOps(t, a, b, ops)
	if limit := timeLimit(2 * time.Second); elapsed > limit {
		t.Errorf("churn diff of %d lines took %v, want < %v", n, elapsed, limit)
	}
	if alloc := after.TotalAlloc - before.TotalAlloc; alloc > 32<<20 {
		t.Errorf("churn diff of %d lines allocated %d bytes", n, alloc)
	}
	t.Logf("churn: %v, %d ops", elapsed, len(ops))
}

func TestBytesAndStats(t *testing.T) {
	ops := Bytes([]byte("a\nb\nc\n"), []byte("a\nB\nc\nd"))
	want := []Op{{Equal, 0, 1, 0, 1}, {Delete, 1, 2, 1, 1}, {Insert, 2, 2, 1, 2}, {Equal, 2, 3, 2, 3}, {Insert, 3, 3, 3, 4}}
	if !reflect.DeepEqual(ops, want) {
		t.Fatalf("Bytes = %+v, want %+v", ops, want)
	}
	if added, deleted := Stats(ops); added != 2 || deleted != 1 {
		t.Errorf("Stats = (%d,%d), want (2,1)", added, deleted)
	}
	if added, deleted := Stats(nil); added != 0 || deleted != 0 {
		t.Errorf("Stats(nil) = (%d,%d)", added, deleted)
	}
}

func TestUnified(t *testing.T) {
	replace := func(base []string, repl map[int]string) []string {
		out := append([]string(nil), base...)
		for i, s := range repl {
			out[i-1] = s + "\n"
		}
		return out
	}
	tests := []struct {
		name    string
		a, b    []string
		context int
		want    string
	}{
		{"identical", lines("a", "b"), lines("a", "b"), 3, ""},
		{"both empty", nil, nil, 3, ""},
		{"modify middle", numbered(9), replace(numbered(9), map[int]string{5: "five"}), 3,
			"--- a\n+++ b\n@@ -2,7 +2,7 @@\n 2\n 3\n 4\n-5\n+five\n 6\n 7\n 8\n"},
		{"single line ranges", numbered(5), replace(numbered(5), map[int]string{3: "three"}), 0,
			"--- a\n+++ b\n@@ -3 +3 @@\n-3\n+three\n"},
		{"no newline at eof", []string{"a\n", "b\n", "c"}, []string{"a\n", "b\n", "d"}, 3,
			"--- a\n+++ b\n@@ -1,3 +1,3 @@\n a\n b\n-c\n\\ No newline at end of file\n+d\n\\ No newline at end of file\n"},
		{"newline added at eof", []string{"a\n", "b"}, lines("a", "b"), 3,
			"--- a\n+++ b\n@@ -1,2 +1,2 @@\n a\n-b\n\\ No newline at end of file\n+b\n"},
		{"context without newline", []string{"a\n", "b\n", "c"}, []string{"x\n", "b\n", "c"}, 3,
			"--- a\n+++ b\n@@ -1,3 +1,3 @@\n-a\n+x\n b\n c\n\\ No newline at end of file\n"},
		{"insert at start", lines("b", "c", "d", "e", "f"), lines("a", "b", "c", "d", "e", "f"), 3,
			"--- a\n+++ b\n@@ -1,3 +1,4 @@\n+a\n b\n c\n d\n"},
		{"insert at start no context", lines("b", "c"), lines("a", "b", "c"), 0,
			"--- a\n+++ b\n@@ -0,0 +1 @@\n+a\n"},
		{"insert into empty", nil, lines("x", "y"), 3,
			"--- a\n+++ b\n@@ -0,0 +1,2 @@\n+x\n+y\n"},
		{"insert after line", lines("a", "b", "c"), lines("a", "b", "x", "c"), 0,
			"--- a\n+++ b\n@@ -2,0 +3 @@\n+x\n"},
		{"delete at end", lines("a", "b", "c", "d"), lines("a", "b", "c"), 2,
			"--- a\n+++ b\n@@ -2,3 +2,2 @@\n b\n c\n-d\n"},
		{"delete everything", lines("x"), nil, 3,
			"--- a\n+++ b\n@@ -1 +0,0 @@\n-x\n"},
		{"negative context", lines("a", "b", "c"), lines("a", "x", "c"), -5,
			"--- a\n+++ b\n@@ -2 +2 @@\n-b\n+x\n"},
		{"two hunks", numbered(20), replace(numbered(20), map[int]string{2: "two", 15: "fifteen"}), 3,
			"--- a\n+++ b\n" +
				"@@ -1,5 +1,5 @@\n 1\n-2\n+two\n 3\n 4\n 5\n" +
				"@@ -12,7 +12,7 @@\n 12\n 13\n 14\n-15\n+fifteen\n 16\n 17\n 18\n"},
		{"merged hunks", numbered(20), replace(numbered(20), map[int]string{5: "five", 11: "eleven"}), 3,
			"--- a\n+++ b\n@@ -2,13 +2,13 @@\n 2\n 3\n 4\n-5\n+five\n 6\n 7\n 8\n 9\n 10\n-11\n+eleven\n 12\n 13\n 14\n"},
		{"gap of exactly 2*context merges", numbered(20), replace(numbered(20), map[int]string{5: "five", 12: "twelve"}), 3,
			"--- a\n+++ b\n@@ -2,14 +2,14 @@\n 2\n 3\n 4\n-5\n+five\n 6\n 7\n 8\n 9\n 10\n 11\n-12\n+twelve\n 13\n 14\n 15\n"},
		{"gap of 2*context+1 splits", numbered(20), replace(numbered(20), map[int]string{5: "five", 13: "thirteen"}), 3,
			"--- a\n+++ b\n" +
				"@@ -2,7 +2,7 @@\n 2\n 3\n 4\n-5\n+five\n 6\n 7\n 8\n" +
				"@@ -10,7 +10,7 @@\n 10\n 11\n 12\n-13\n+thirteen\n 14\n 15\n 16\n"},
		{"offset second hunk", lines("a", "b", "c", "d", "e", "f", "g", "h"), lines("a", "x", "y", "c", "d", "e", "f", "g"), 1,
			"--- a\n+++ b\n@@ -1,3 +1,4 @@\n a\n-b\n+x\n+y\n c\n@@ -7,2 +8 @@\n g\n-h\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Unified("a", "b", tt.a, tt.b, tt.context)
			if got != tt.want {
				t.Errorf("Unified mismatch\ngot:\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}

func TestLineMap(t *testing.T) {
	tests := []struct {
		name string
		a, b []string
		want []int
	}{
		{"empty", nil, nil, []int{}},
		{"all new", nil, lines("a", "b"), []int{-1, -1}},
		{"all deleted", lines("a", "b"), nil, []int{}},
		{"identical", lines("a", "b", "c"), lines("a", "b", "c"), []int{0, 1, 2}},
		{"insert and delete", lines("a", "b", "c"), lines("x", "a", "c", "y"), []int{-1, 0, 2, -1}},
		{"modified line", lines("a", "b", "c"), lines("a", "B", "c"), []int{0, -1, 2}},
		{"moved block", lines("a", "b", "c", "d", "e"), lines("d", "e", "a", "b", "c"), []int{-1, -1, 0, 1, 2}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := LineMap(tt.a, tt.b); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("LineMap = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsBinary(t *testing.T) {
	big := make([]byte, 9000)
	for i := range big {
		big[i] = 'x'
	}
	late := append([]byte(nil), big...)
	late[8500] = 0
	early := append([]byte(nil), big...)
	early[7999] = 0
	tests := []struct {
		name string
		in   []byte
		want bool
	}{
		{"empty", nil, false},
		{"text", []byte("hello\nworld\n"), false},
		{"utf8", []byte("héllo 世界\n"), false},
		{"nul", []byte("a\x00b"), true},
		{"nul at 7999", early, true},
		{"nul past 8000", late, false},
	}
	for _, tt := range tests {
		if got := IsBinary(tt.in); got != tt.want {
			t.Errorf("IsBinary(%s) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestMerge3(t *testing.T) {
	tests := []struct {
		name               string
		base, ours, theirs string
		want               string
		conflicts          []Conflict
	}{
		{
			name:   "clean non-overlapping",
			base:   "1\n2\n3\n4\n5\n6\n7\n",
			ours:   "1\ntwo\n3\n4\n5\n6\n7\n",
			theirs: "1\n2\n3\n4\n5\nsix\n7\n",
			want:   "1\ntwo\n3\n4\n5\nsix\n7\n",
		},
		{
			name:   "clean separated by one line",
			base:   "a\nb\nc\nd\n",
			ours:   "a\nB\nc\nd\n",
			theirs: "a\nb\nc\nD\n",
			want:   "a\nB\nc\nD\n",
		},
		{
			name:   "clean insert and delete",
			base:   "a\nb\nc\nd\ne\n",
			ours:   "x\na\nb\nc\nd\ne\n",
			theirs: "a\nb\nc\ne\n",
			want:   "x\na\nb\nc\ne\n",
		},
		{
			name:   "identical change",
			base:   "a\nb\nc\n",
			ours:   "a\nB\nc\nd\n",
			theirs: "a\nB\nc\n",
			want:   "a\nB\nc\nd\n",
		},
		{
			name:   "identical change plus separate edits",
			base:   "1\n2\n3\n4\n5\n6\n7\n",
			ours:   "one\n2\n3\nfour\n5\n6\n7\n",
			theirs: "1\n2\n3\nfour\n5\n6\nseven\n",
			want:   "one\n2\n3\nfour\n5\n6\nseven\n",
		},
		{
			name:      "conflict",
			base:      "a\nb\nc\n",
			ours:      "a\nB1\nc\n",
			theirs:    "a\nB2\nc\n",
			want:      "a\n<<<<<<< ours\nB1\n||||||| base\nb\n=======\nB2\n>>>>>>> theirs\nc\n",
			conflicts: []Conflict{{BaseStart: 1, BaseEnd: 2, OursStart: 1, OursEnd: 2, TheirsStart: 1, TheirsEnd: 2}},
		},
		{
			name:      "adjacent edits conflict as in git",
			base:      "a\nb\nc\nd\n",
			ours:      "a\nB\nc\nd\n",
			theirs:    "a\nb\nC\nd\n",
			want:      "a\n<<<<<<< ours\nB\nc\n||||||| base\nb\nc\n=======\nb\nC\n>>>>>>> theirs\nd\n",
			conflicts: []Conflict{{BaseStart: 1, BaseEnd: 3, OursStart: 1, OursEnd: 3, TheirsStart: 1, TheirsEnd: 3}},
		},
		{
			name:   "ours unchanged",
			base:   "a\nb\n",
			ours:   "a\nb\n",
			theirs: "a\nc\n",
			want:   "a\nc\n",
		},
		{
			name:   "theirs unchanged",
			base:   "a\nb\n",
			ours:   "x\n",
			theirs: "a\nb\n",
			want:   "x\n",
		},
		{
			name:   "both identical",
			base:   "a\n",
			ours:   "b\n",
			theirs: "b\n",
			want:   "b\n",
		},
		{
			name:      "insertion at same point conflicts",
			base:      "a\nb\n",
			ours:      "a\nX\nb\n",
			theirs:    "a\nY\nb\n",
			want:      "a\n<<<<<<< ours\nX\n||||||| base\n=======\nY\n>>>>>>> theirs\nb\n",
			conflicts: []Conflict{{BaseStart: 1, BaseEnd: 1, OursStart: 1, OursEnd: 2, TheirsStart: 1, TheirsEnd: 2}},
		},
		{
			name:   "identical insertion at same point",
			base:   "a\nb\nc\n",
			ours:   "a\nX\nb\nc\n",
			theirs: "a\nX\nb\nC\n",
			want:   "a\nX\nb\nC\n",
		},
		{
			name:   "insertions at different points",
			base:   "a\nb\n",
			ours:   "X\na\nb\n",
			theirs: "a\nb\nY\n",
			want:   "X\na\nb\nY\n",
		},
		{
			name:      "delete vs modify",
			base:      "a\nb\nc\n",
			ours:      "a\nc\n",
			theirs:    "a\nB\nc\n",
			want:      "a\n<<<<<<< ours\n||||||| base\nb\n=======\nB\n>>>>>>> theirs\nc\n",
			conflicts: []Conflict{{BaseStart: 1, BaseEnd: 2, OursStart: 1, OursEnd: 1, TheirsStart: 1, TheirsEnd: 2}},
		},
		{
			name:   "both delete same line",
			base:   "a\nb\nc\n",
			ours:   "a\nc\nd\n",
			theirs: "a\nc\n",
			want:   "a\nc\nd\n",
		},
		{
			name:      "empty base both add different",
			base:      "",
			ours:      "x\ny\n",
			theirs:    "x\nz\n",
			want:      "<<<<<<< ours\nx\ny\n||||||| base\n=======\nx\nz\n>>>>>>> theirs\n",
			conflicts: []Conflict{{BaseStart: 0, BaseEnd: 0, OursStart: 0, OursEnd: 2, TheirsStart: 0, TheirsEnd: 2}},
		},
		{
			name:   "empty base both add same",
			base:   "",
			ours:   "x\n",
			theirs: "x\n",
			want:   "x\n",
		},
		{
			name:      "no trailing newline conflict",
			base:      "a\nb",
			ours:      "a\nc",
			theirs:    "a\nd",
			want:      "a\n<<<<<<< ours\nc\n||||||| base\nb\n=======\nd\n>>>>>>> theirs\n",
			conflicts: []Conflict{{BaseStart: 1, BaseEnd: 2, OursStart: 1, OursEnd: 2, TheirsStart: 1, TheirsEnd: 2}},
		},
		{
			name:   "no trailing newline clean",
			base:   "a\nb\nc",
			ours:   "A\nb\nc",
			theirs: "a\nb\nC",
			want:   "A\nb\nC",
		},
		{
			name:   "newline added on one side",
			base:   "a\nb\nc",
			ours:   "a\nb\nc\n",
			theirs: "A\nb\nc",
			want:   "A\nb\nc\n",
		},
		{
			name:   "multiple conflicts and clean regions",
			base:   "1\n2\n3\n4\n5\n6\n7\n8\n9\n",
			ours:   "1\nO2\n3\n4\n5\nO6\n7\n8\n9\n",
			theirs: "1\nT2\n3\n4\nT5\n5\n6\n7\n8\nT9\n",
			want: "1\n<<<<<<< ours\nO2\n||||||| base\n2\n=======\nT2\n>>>>>>> theirs\n3\n4\nT5\n5\n" +
				"O6\n7\n8\nT9\n",
			conflicts: []Conflict{{BaseStart: 1, BaseEnd: 2, OursStart: 1, OursEnd: 2, TheirsStart: 1, TheirsEnd: 2}},
		},
		{
			name:      "delete all vs modify",
			base:      "a\nb\n",
			ours:      "",
			theirs:    "a\nB\n",
			want:      "<<<<<<< ours\n||||||| base\na\nb\n=======\na\nB\n>>>>>>> theirs\n",
			conflicts: []Conflict{{BaseStart: 0, BaseEnd: 2, OursStart: 0, OursEnd: 0, TheirsStart: 0, TheirsEnd: 2}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := Merge3([]byte(tt.base), []byte(tt.ours), []byte(tt.theirs), "base", "ours", "theirs")
			if string(res.Merged) != tt.want {
				t.Errorf("merged mismatch\ngot:\n%s\nwant:\n%s", res.Merged, tt.want)
			}
			if !reflect.DeepEqual(res.Conflicts, tt.conflicts) {
				t.Errorf("conflicts = %+v, want %+v", res.Conflicts, tt.conflicts)
			}
		})
	}
}

func TestMerge3EmptyLabels(t *testing.T) {
	res := Merge3([]byte("a\n"), []byte("b\n"), []byte("c\n"), "", "", "")
	want := "<<<<<<<\nb\n|||||||\na\n=======\nc\n>>>>>>>\n"
	if string(res.Merged) != want {
		t.Errorf("got %q, want %q", res.Merged, want)
	}
}

func TestMerge3DoesNotAlias(t *testing.T) {
	base, ours := []byte("a\n"), []byte("b\n")
	res := Merge3(base, ours, base, "base", "ours", "theirs")
	res.Merged[0] = 'z'
	if string(ours) != "b\n" {
		t.Errorf("Merge3 result aliases its input")
	}
}

// TestMerge3RandomDisjoint merges random edits to the two halves of a file
// of unique lines; the edits are separated by an untouched line, so the
// merge must be clean and contain both sides' changes.
func TestMerge3RandomDisjoint(t *testing.T) {
	r := rand.New(rand.NewPCG(7, 8))
	fresh := 0
	edit := func(ls []string, tag string) []string {
		var out []string
		newLine := func() string { fresh++; return fmt.Sprintf("%s%d\n", tag, fresh) }
		for _, l := range ls {
			switch r.IntN(5) {
			case 0: // delete
			case 1:
				out = append(out, newLine())
			case 2:
				out = append(out, newLine(), l)
			default:
				out = append(out, l)
			}
		}
		if r.IntN(2) == 0 {
			out = append(out, newLine())
		}
		return out
	}
	for iter := 0; iter < 500; iter++ {
		n := 1 + r.IntN(60)
		base := make([]string, n)
		for i := range base {
			base[i] = fmt.Sprintf("base%d\n", i)
		}
		s := r.IntN(n)
		oursHead := edit(base[:s], "o")
		theirsTail := edit(base[s+1:], "t")
		ours := concat(oursHead, base[s:])
		theirs := concat(base[:s+1], theirsTail)
		want := strings.Join(concat(oursHead, []string{base[s]}, theirsTail), "")

		res := Merge3([]byte(strings.Join(base, "")), []byte(strings.Join(ours, "")), []byte(strings.Join(theirs, "")), "b", "o", "t")
		if len(res.Conflicts) != 0 || string(res.Merged) != want {
			t.Fatalf("iter %d: conflicts %+v\ngot:\n%s\nwant:\n%s", iter, res.Conflicts, res.Merged, want)
		}
		// The merge is symmetric when clean.
		res2 := Merge3([]byte(strings.Join(base, "")), []byte(strings.Join(theirs, "")), []byte(strings.Join(ours, "")), "b", "t", "o")
		if string(res2.Merged) != want {
			t.Fatalf("iter %d: swapped merge differs:\n%s", iter, res2.Merged)
		}
	}
}

// TestMerge3RandomConflictRanges checks on random inputs that conflict
// ranges are consistent and that taking either side of every conflict
// reproduces a sensible document.
func TestMerge3RandomConflictRanges(t *testing.T) {
	r := rand.New(rand.NewPCG(9, 10))
	for iter := 0; iter < 1000; iter++ {
		alphabet := 2 + r.IntN(8)
		base := randLines(r, r.IntN(40), alphabet)
		ours := mutate(r, base, alphabet+2)
		theirs := mutate(r, base, alphabet+2)
		join := func(ls []string) []byte { return []byte(strings.Join(ls, "")) }
		res := Merge3(join(base), join(ours), join(theirs), "base", "ours", "theirs")

		prevBase, prevOurs, prevTheirs := 0, 0, 0
		for _, c := range res.Conflicts {
			if c.BaseStart < prevBase || c.BaseEnd < c.BaseStart || c.BaseEnd > len(base) ||
				c.OursStart < prevOurs || c.OursEnd < c.OursStart || c.OursEnd > len(ours) ||
				c.TheirsStart < prevTheirs || c.TheirsEnd < c.TheirsStart || c.TheirsEnd > len(theirs) {
				t.Fatalf("iter %d: bad conflict %+v (base %d, ours %d, theirs %d lines)", iter, c, len(base), len(ours), len(theirs))
			}
			prevBase, prevOurs, prevTheirs = c.BaseEnd, c.OursEnd, c.TheirsEnd
		}
		markers := strings.Count(string(res.Merged), "<<<<<<< ours\n")
		if markers != len(res.Conflicts) {
			t.Fatalf("iter %d: %d conflict markers, %d conflicts", iter, markers, len(res.Conflicts))
		}
		if len(res.Conflicts) == 0 {
			// A clean merge of a side with an unchanged side is that side.
			if again := Merge3(join(base), join(ours), join(base), "", "", ""); string(again.Merged) != string(join(ours)) {
				t.Fatalf("iter %d: merge with unchanged theirs is not ours", iter)
			}
		}
	}
}

func concat(parts ...[]string) []string {
	var out []string
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func BenchmarkLinesLargeFewEdits(b *testing.B) {
	const n = 100_000
	x := make([]string, n)
	for i := range x {
		x[i] = fmt.Sprintf("line %d\n", i)
	}
	y := append([]string(nil), x...)
	for i := 0; i < n; i += 997 {
		y[i] = "changed\n"
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Lines(x, y)
	}
}

func BenchmarkLinesChurn(b *testing.B) {
	r := rand.New(rand.NewPCG(11, 12))
	x := randLines(r, 20_000, 16)
	y := randLines(r, 20_000, 16)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Lines(x, y)
	}
}
