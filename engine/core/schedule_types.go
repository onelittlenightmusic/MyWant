package mywant

import (
	"strings"
	"time"

	want_spec "github.com/onelittlenightmusic/want-spec"
)

// WhenSpec defines a scheduled execution time for a Want
type WhenSpec = want_spec.WhenSpec

// ParsedSchedule represents a parsed and processed schedule
type ParsedSchedule struct {
	Time          time.Time     // Next execution time
	Interval      time.Duration // Time between executions
	IsAbsolute    bool          // Whether this is a fixed time (at) vs relative interval (every)
	OriginalAt    string        // Original 'at' expression
	OriginalEvery string        // Original 'every' expression
}

// A want's own wake: Progress run again at a time the want names — once, on
// top of whatever schedule it has (spec.when, a type's default-every). For a
// moment only the want knows: fifteen minutes before the reservation it just
// found, to see that it still holds and say so (its type's timing "soon").
//
// The want says it from its Progress with the standard function WakeAt(t) (a
// plugin script: a line {"_wake_at": "<RFC3339>"}, mywant.current().
// wake_at(...)); any write of its state field wake_at does the same, the
// API's included. The time is kept in that field, so it outlives a restart of
// the want and of the server. Naming another time replaces it; a zero time
// (or "") takes it back.
//
// It is one more entry in the want's schedule: writing it tells the scheduler
// (scheduleChanged), which restarts the want when it comes, as for any
// schedule, and drops it — a wake is done once. The restart
// takes back a wake whose time has passed (prepareForRestart), so it is not
// done again after the server starts.

// WakeAtState is the state field a want's wake is kept in (RFC3339).
const WakeAtState = "wake_at"

// wakeGrace is how late a wake is still done: one that came while the server
// was down is done when it is back, unless it is older than this — a moment
// that long gone is not worth waking for.
const wakeGrace = 10 * time.Minute

// WakeAt asks for Progress to run again at t, once.
func (n *Want) WakeAt(t time.Time) {
	if t.IsZero() {
		n.StoreState(WakeAtState, "")
		return
	}
	n.StoreState(WakeAtState, t.Format(time.RFC3339))
}

// wakeOf is the want's wake, if it has one.
func wakeOf(n *Want) (time.Time, bool) {
	v, _ := n.State.Load(WakeAtState)
	return parseWake(v)
}

// parseWake reads a wake_at value: ok is false for none.
func parseWake(v any) (time.Time, bool) {
	s, _ := v.(string)
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, s)
	return t, err == nil
}
