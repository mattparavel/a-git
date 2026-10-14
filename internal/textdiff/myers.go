package textdiff

import "math"

// The middle-snake search of a sub-problem gives up on minimality after a
// number of rounds; each round extends both the forward and the backward
// search by one edit, so the edit distance explored is about twice the round
// count. Past the limit the search splits at the furthest point it reached
// (see bestSplit), which costs O(limit) work per line of input, so the limit
// is chosen as costBudget/size clamped to [minCostLimit, maxCostLimit]:
// inputs of up to 16k lines (both sides together) get an exact diff whenever
// the edit distance is below about 4096, and larger inputs trade minimality
// for bounded time, never going below git's floor of 256 rounds.
//
// These are variables only so that tests can exercise the fallback path.
var (
	minCostLimit = 256
	maxCostLimit = 2048
	costBudget   = 1 << 25
)

// costLimit returns the round limit for a problem with size total lines.
func costLimit(size int) int {
	return max(min(costBudget/max(size, 1), maxCostLimit), minCostLimit)
}

// diffMiddle diffs a and b (whose first and last lines are known to differ,
// or which are empty) and returns per-line change flags: ra[i] reports that
// a[i] is deleted, rb[j] that b[j] is inserted.
func diffMiddle(a, b []string) (ra, rb []bool) {
	ra = make([]bool, len(a))
	rb = make([]bool, len(b))
	if len(a) == 0 || len(b) == 0 {
		markAll(ra)
		markAll(rb)
		return ra, rb
	}

	// Intern lines so the inner loops compare ints rather than strings.
	ids := make(map[string]int, len(a))
	ia := make([]int, len(a))
	for i, s := range a {
		id, ok := ids[s]
		if !ok {
			id = len(ids)
			ids[s] = id
		}
		ia[i] = id
	}
	na := len(ids) // ids below na occur in a
	ib := make([]int, len(b))
	inB := make([]bool, na)
	for j, s := range b {
		id, ok := ids[s]
		if !ok {
			id = len(ids)
			ids[s] = id
		}
		ib[j] = id
		if id < na {
			inB[id] = true
		}
	}
	ids = nil // release the map before the (possibly long) search

	// Lines that occur in only one input can never be part of a common
	// subsequence. Marking them changed up front preserves minimality while
	// shrinking the search, dramatically so for wholesale rewrites.
	fa, aIdx := make([]int, 0, len(a)), make([]int, 0, len(a))
	for i, id := range ia {
		if inB[id] {
			fa = append(fa, id)
			aIdx = append(aIdx, i)
		} else {
			ra[i] = true
		}
	}
	fb, bIdx := make([]int, 0, len(b)), make([]int, 0, len(b))
	for j, id := range ib {
		if id < na {
			fb = append(fb, id)
			bIdx = append(bIdx, j)
		} else {
			rb[j] = true
		}
	}

	if len(fa) > 0 && len(fb) > 0 {
		d := newMyers(fa, fb)
		d.compare(0, len(fa), 0, len(fb))
		for k, ch := range d.ra {
			if ch {
				ra[aIdx[k]] = true
			}
		}
		for k, ch := range d.rb {
			if ch {
				rb[bIdx[k]] = true
			}
		}
	} else {
		markAll(ra)
		markAll(rb)
	}

	shiftBoundaries(ra, ia, rb)
	shiftBoundaries(rb, ib, ra)
	return ra, rb
}

func markAll(flags []bool) {
	for i := range flags {
		flags[i] = true
	}
}

// myers holds the state of a linear-space Myers diff of two interned
// sequences.
type myers struct {
	a, b   []int
	ra, rb []bool // output change flags, as for diffMiddle

	// fd and bd hold, per diagonal k = x-y (stored at index k+off), the
	// furthest-reaching x of the forward and backward searches. They are
	// shared by all split calls; each call initialises what it reads.
	fd, bd []int
	off    int

	maxCost int // round limit before the heuristic split kicks in
}

func newMyers(a, b []int) *myers {
	n, m := len(a), len(b)
	// Diagonals range over [-m, n]; one sentinel slot is needed on each side.
	size := n + m + 3
	v := make([]int, 2*size)
	return &myers{
		a: a, b: b,
		ra: make([]bool, n), rb: make([]bool, m),
		fd: v[:size], bd: v[size:],
		off:     m + 1,
		maxCost: costLimit(n + m),
	}
}

// compare marks the changed lines of a[xoff:xlim] versus b[yoff:ylim].
func (d *myers) compare(xoff, xlim, yoff, ylim int) {
	a, b := d.a, d.b
	for {
		for xoff < xlim && yoff < ylim && a[xoff] == b[yoff] {
			xoff++
			yoff++
		}
		for xoff < xlim && yoff < ylim && a[xlim-1] == b[ylim-1] {
			xlim--
			ylim--
		}
		if xoff == xlim || yoff == ylim {
			markAll(d.ra[xoff:xlim])
			markAll(d.rb[yoff:ylim])
			return
		}

		x, y := d.split(xoff, xlim, yoff, ylim)
		if x < xoff || x > xlim || y < yoff || y > ylim ||
			(x == xoff && y == yoff) || (x == xlim && y == ylim) {
			// A degenerate split would not make progress. This cannot happen
			// for a genuine middle snake; it is a guard for the heuristic.
			markAll(d.ra[xoff:xlim])
			markAll(d.rb[yoff:ylim])
			return
		}

		// Recurse into the smaller half and iterate on the larger one, which
		// keeps the stack depth logarithmic even for lopsided splits.
		if (x-xoff)+(y-yoff) <= (xlim-x)+(ylim-y) {
			d.compare(xoff, x, yoff, y)
			xoff, yoff = x, y
		} else {
			d.compare(x, xlim, y, ylim)
			xlim, ylim = x, y
		}
	}
}

// split returns a point (x, y) on a minimal edit path from (xoff, yoff) to
// (xlim, ylim), found by running Myers' search forward from the top-left
// corner and backward from the bottom-right corner until the two meet (the
// "middle snake"). Both sub-problems on either side of the point have a
// strictly smaller edit distance.
//
// If the search exceeds d.maxCost rounds, it gives up on minimality and
// returns the furthest point reached by either search, which still splits
// the problem into two strictly smaller ones.
//
// The caller guarantees a[xoff] != b[yoff] and a[xlim-1] != b[ylim-1].
func (d *myers) split(xoff, xlim, yoff, ylim int) (x, y int) {
	const none = math.MaxInt // invalid entry in bd (invalid in fd is -1)
	a, b, fd, bd, off := d.a, d.b, d.fd, d.bd, d.off

	dmin, dmax := xoff-ylim, xlim-yoff // valid diagonals
	fmid, bmid := xoff-yoff, xlim-ylim // start diagonals
	fmin, fmax := fmid, fmid           // active forward diagonals
	bmin, bmax := bmid, bmid           // active backward diagonals
	odd := (fmid-bmid)&1 != 0          // parity of the total edit distance

	fd[off+fmid] = xoff
	bd[off+bmid] = xlim

	for c := 1; ; c++ {
		// Extend the forward search by one edit on each active diagonal.
		// Diagonals outside [dmin, dmax] do not intersect the rectangle;
		// the slot just past each end of the range is reset to a sentinel.
		if fmin > dmin {
			fmin--
			fd[off+fmin-1] = -1
		} else {
			fmin++
		}
		if fmax < dmax {
			fmax++
			fd[off+fmax+1] = -1
		} else {
			fmax--
		}
		for k := fmax; k >= fmin; k -= 2 {
			// Step down (insertion) from diagonal k+1 or right (deletion)
			// from diagonal k-1, whichever reaches further while staying
			// inside the rectangle.
			x := -1
			if t := fd[off+k+1]; t >= 0 && t-k <= ylim {
				x = t
			}
			if t := fd[off+k-1]; t >= 0 && t < xlim && t+1 > x {
				x = t + 1
			}
			if x < 0 {
				fd[off+k] = -1
				continue
			}
			y := x - k
			for x < xlim && y < ylim && a[x] == b[y] {
				x++
				y++
			}
			fd[off+k] = x
			if odd && bmin <= k && k <= bmax && bd[off+k] <= x {
				return x, y
			}
		}

		// Extend the backward search likewise.
		if bmin > dmin {
			bmin--
			bd[off+bmin-1] = none
		} else {
			bmin++
		}
		if bmax < dmax {
			bmax++
			bd[off+bmax+1] = none
		} else {
			bmax--
		}
		for k := bmax; k >= bmin; k -= 2 {
			// Step up from diagonal k-1 or left from diagonal k+1.
			x := none
			if t := bd[off+k-1]; t != none && t-k >= yoff {
				x = t
			}
			if t := bd[off+k+1]; t != none && t > xoff && t-1 < x {
				x = t - 1
			}
			if x == none {
				bd[off+k] = none
				continue
			}
			y := x - k
			for x > xoff && y > yoff && a[x-1] == b[y-1] {
				x--
				y--
			}
			bd[off+k] = x
			if !odd && fmin <= k && k <= fmax && x <= fd[off+k] {
				return x, y
			}
		}

		if c >= d.maxCost {
			return d.bestSplit(xoff, xlim, yoff, ylim, fmin, fmax, bmin, bmax)
		}
	}
}

// bestSplit implements the cost-limit heuristic of split: it returns the
// forward point maximising x+y or the backward point minimising x+y,
// whichever has made more progress from its corner.
func (d *myers) bestSplit(xoff, xlim, yoff, ylim, fmin, fmax, bmin, bmax int) (x, y int) {
	const none = math.MaxInt
	fbest, fx := -1, 0
	for k := fmax; k >= fmin; k -= 2 {
		if x := d.fd[d.off+k]; x >= 0 && 2*x-k > fbest {
			fbest, fx = 2*x-k, x
		}
	}
	bbest, bx := none, 0
	for k := bmax; k >= bmin; k -= 2 {
		if x := d.bd[d.off+k]; x != none && 2*x-k < bbest {
			bbest, bx = 2*x-k, x
		}
	}
	switch {
	case fbest < 0 && bbest == none:
		return xoff, yoff // degenerate; rejected by compare
	case bbest == none, fbest >= 0 && fbest-(xoff+yoff) > (xlim+ylim)-bbest:
		return fx, fbest - fx
	default:
		return bx, bbest - bx
	}
}

// shiftBoundaries slides runs of changed lines to canonical positions, as
// GNU diff does. A run can be moved whenever the line just before it equals
// its last line (or the line just after it equals its first line) without
// changing the edit distance. Each run is first moved up, merging with
// earlier runs where possible, then down as far as possible, merging with
// later runs; finally it is moved back up to line up with a run of changes
// in the other file if it passed one. Without such alignment, runs end up
// as low as possible, which tends to keep blocks such as "}\n\nfunc f() {"
// together in the expected way.
//
// changed holds the flags of one file and eq its interned lines; other holds
// the (unmodified) flags of the other file. Unchanged lines of the two files
// correspond in order; j tracks the position in other that corresponds to i.
func shiftBoundaries(changed []bool, eq []int, other []bool) {
	n, m := len(changed), len(other)
	i, j := 0, 0
	for {
		// Find the start of the next run, tracking the corresponding point
		// in the other file.
		for i < n && !changed[i] {
			for j < m && other[j] {
				j++
			}
			j++
			i++
		}
		if i == n {
			return
		}
		start := i
		for i < n && changed[i] {
			i++
		}
		for j < m && other[j] {
			j++
		}

		var corresponding int
		for {
			runLength := i - start

			// Move the run up while the previous unchanged line matches the
			// last changed one, merging with earlier runs.
			for start > 0 && eq[start-1] == eq[i-1] {
				start--
				changed[start] = true
				i--
				changed[i] = false
				for start > 0 && changed[start-1] {
					start--
				}
				j--
				for other[j] {
					j--
				}
			}

			// corresponding is the end of the run at the last position where
			// it is adjacent to a run of changes in the other file, or n.
			corresponding = n
			if j > 0 && other[j-1] {
				corresponding = i
			}

			// Move the run down while its first line matches the following
			// unchanged one, merging with later runs.
			for i < n && eq[start] == eq[i] {
				changed[start] = false
				start++
				changed[i] = true
				i++
				for i < n && changed[i] {
					i++
				}
				j++
				for j < m && other[j] {
					corresponding = i
					j++
				}
			}

			if runLength == i-start {
				break
			}
		}

		// Move the fully merged run back to line up with the other file's
		// changes, if possible.
		for corresponding < i {
			start--
			changed[start] = true
			i--
			changed[i] = false
			j--
			for other[j] {
				j--
			}
		}
	}
}
