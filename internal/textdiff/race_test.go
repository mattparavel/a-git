//go:build race

package textdiff

// raceEnabled reports whether the race detector is on, so timing tests can
// allow for its overhead.
const raceEnabled = true
