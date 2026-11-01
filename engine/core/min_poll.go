package mywant

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// atLeastMinPoll is a monitor's interval, no shorter than the server allows:
// MYWANT_MIN_POLL_MS, set for a small server in its config's environments
// (as MYWANT_PARALLEL is for how many at once). Unset, a monitor polls as
// its type asks. On fly's shared CPU a plugin monitor every second or two —
// each tick a fresh python3 in the runner — used the whole CPU the Machine
// is given and every request queued behind it (2026-11-02: spotify at 1s,
// door at 2s).
func atLeastMinPoll(d time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv("MYWANT_MIN_POLL_MS"))
	if raw == "" {
		return d
	}
	ms, err := strconv.Atoi(raw)
	if err != nil || ms <= 0 {
		return d
	}
	if min := time.Duration(ms) * time.Millisecond; d < min {
		return min
	}
	return d
}
