package mywant

import (
	"fmt"
	"strings"
	"time"
)

// paramConversion is the pair of kinds a parameter sits between: what the wire
// feeding it carries, and what the want reading it expects.
type paramConversion struct {
	From string
	To   string
}

// ConvertSubTypeValue rewrites a value from one subType into another when the
// two say the same thing in different words.
//
// This exists because of a wire that was correct and useless. A route's
// `departure` is a `time` — "22:47", which is what a departure board says — and
// a reminder's `event_time` is a `datetime`, an instant. Both are the moment to
// leave; only the writing differs. Refusing to connect them, or making somebody
// place a converter want between the two, would be charging a person for
// arithmetic the board can do.
//
// Deliberately narrow. Only conversions where the second value is ENTAILED by
// the first belong here — no parsing of free text, no guessing at units, no
// lossy rounding. When a conversion is not one of those, the answer is `false`
// and the value is passed through untouched, which leaves the mismatch visible
// instead of inventing a number.
//
// Returns false for anything it does not know, including a value already in the
// target shape.
func ConvertSubTypeValue(v any, from, to string) (any, bool) {
	s, ok := v.(string)
	if !ok || s == "" || from == to {
		return v, false
	}
	switch {
	case from == "time" && to == "datetime":
		t, ok := nextOccurrenceOf(s, time.Now())
		if !ok {
			return v, false
		}
		return t.Format(time.RFC3339), true
	case from == "date" && to == "datetime":
		d, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(s), time.Local)
		if err != nil {
			return v, false
		}
		return d.Format(time.RFC3339), true
	case from == "datetime" && to == "time":
		t, err := time.Parse(time.RFC3339, strings.TrimSpace(s))
		if err != nil {
			return v, false
		}
		return t.Local().Format("15:04"), true
	case from == "datetime" && to == "date":
		t, err := time.Parse(time.RFC3339, strings.TrimSpace(s))
		if err != nil {
			return v, false
		}
		return t.Local().Format("2006-01-02"), true
	}
	return v, false
}

// nextOccurrenceOf reads "22:47" as the moment that clock time comes round
// next.
//
// The next one, not today's: a departure at 00:20 read at 23:50 is twenty
// minutes away, and reading it as this morning's would put the alarm sixteen
// hours in the past — which for a reminder means it fires at once and for a
// timetable means the wrong day. A clock time is only ever the next one.
func nextOccurrenceOf(hhmm string, now time.Time) (time.Time, bool) {
	t, err := time.Parse("15:04", strings.TrimSpace(hhmm))
	if err != nil {
		return time.Time{}, false
	}
	at := time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), 0, 0, now.Location())
	if at.Before(now) {
		at = at.AddDate(0, 0, 1)
	}
	return at, true
}

// describeConversion is for logs: "time → datetime".
func describeConversion(c paramConversion) string {
	return fmt.Sprintf("%s → %s", c.From, c.To)
}
