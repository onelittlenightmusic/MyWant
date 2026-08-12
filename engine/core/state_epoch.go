package mywant

// state_epoch.go — a single counter answering "has anything worth persisting
// changed?" in constant time.
//
// writeStatsToMemory runs on a 1s ticker and rebuilt every want, marshalled the
// lot to YAML, and only then compared an md5 to decide the write was
// unnecessary. On a world whose wants are mostly static scenery that verdict is
// "unnecessary" almost every second, and the cost of reaching it was the single
// largest item in the server's CPU profile.
//
// StoreState already refuses to record a write whose value is unchanged, so it
// is the natural place to observe real change. Bumping a counter there lets the
// stats writer skip the whole rebuild with one atomic load.
//
// The counter is deliberately global rather than per-ChainBuilder: wants do not
// know which builder owns them, and an over-eager write is harmless where a
// missed one is not.

import "sync/atomic"

var stateEpoch atomic.Uint64

// bumpStateEpoch records that some want's persisted state changed.
//
// Call this from every mutation that writeStatsToMemory would serialise:
// state values, status, labels, and order keys. Missing a call means a change
// can sit unwritten until the next forced write, so err towards calling it.
func bumpStateEpoch() {
	stateEpoch.Add(1)
}

// currentStateEpoch reads the counter.
func currentStateEpoch() uint64 {
	return stateEpoch.Load()
}
