package mywant

import (
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
