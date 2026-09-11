// Package timewin holds small helpers shared by the packages that keep a
// sorted, in-memory sliding window of timestamps for local rate limiting
// (internal/forum, internal/chatlog, internal/profile, internal/site,
// internal/pluginapi) — before this package existed each of them carried
// its own byte-identical copy of KeepAfter.
package timewin

import "time"

// KeepAfter drops the leading run of timestamps in ts that are at or
// before cutoff, returning the remaining (still time-ascending) slice.
// ts must already be sorted ascending — as it is for every caller, since
// each only ever appends time.Now() to the end of its window.
func KeepAfter(ts []time.Time, cutoff time.Time) []time.Time {
	i := 0
	for i < len(ts) && !ts[i].After(cutoff) {
		i++
	}
	return ts[i:]
}
