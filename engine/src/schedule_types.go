package mywant

import "time"

// WhenSpec defines a scheduled execution time for a Want
type WhenSpec struct {
	At    string `json:"at" yaml:"at"`       // Time expression like "7am", "17:30", "midnight"
	Every string `json:"every" yaml:"every"` // Frequency like "day", "5 minutes", "2 hours"
}

// ParsedSchedule represents a parsed and processed schedule
type ParsedSchedule struct {
	Time          time.Time     // Next execution time
	Interval      time.Duration // Time between executions
	IsAbsolute    bool          // Whether this is a fixed time (at) vs relative interval (every)
	OriginalAt    string        // Original 'at' expression
	OriginalEvery string        // Original 'every' expression
}
