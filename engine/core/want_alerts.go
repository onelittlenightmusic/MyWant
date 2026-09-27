package mywant

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	want_spec "github.com/onelittlenightmusic/want-spec"
)

// AlertRule is one promise the person who placed a want made about it
// (spec.alerts): while it holds, the want needs them.
//
// The promise is what lets silence mean something. If every way a want can
// need its person is written down and watched, then no alert holding is a
// real answer — "there is nothing you have to come back for" — and not just
// the absence of news.
type AlertRule = want_spec.AlertRule

// HeldAlert is an alert that holds right now, with what was seen.
type HeldAlert struct {
	Rule   AlertRule
	Detail string
}

// HeldAlerts evaluates the want's alerts against what is true now. Evaluated
// on demand, like a kata: there is no loop behind it, so the answer is never
// staler than the question.
//
// paused says the whole system has been stopped by its person: a silentFor
// alert does not hold then, since nothing is meant to be changing.
func (n *Want) HeldAlerts(now time.Time, paused bool) []HeldAlert {
	spec := n.GetSpec()
	if spec == nil || len(spec.Alerts) == 0 {
		return nil
	}
	var held []HeldAlert
	for _, rule := range spec.Alerts {
		if detail, ok := n.alertHolds(rule, now, paused); ok {
			held = append(held, HeldAlert{Rule: rule, Detail: detail})
		}
	}
	return held
}

func (n *Want) alertHolds(rule AlertRule, now time.Time, paused bool) (string, bool) {
	switch {
	case rule.When != nil:
		// Any label: the person wrote the promise about a field they can see,
		// not about how the want type happened to file it.
		actual, _ := n.getState(rule.When.Field)
		if actual == nil || !evaluateCondition(actual, rule.When.Operator, rule.When.Value) {
			return "", false
		}
		return fmt.Sprintf("%s = %v", rule.When.Field, actual), true

	case len(rule.Status) > 0:
		status := string(n.GetStatus())
		if !slices.Contains(rule.Status, status) {
			return "", false
		}
		return status, true

	case rule.SilentFor != "":
		if paused || n.GetStatus() == WantStatusSuspended {
			return "", false
		}
		d, err := parseAlertDuration(rule.SilentFor)
		if err != nil {
			return "", false
		}
		last, ok := n.lastChangedAt(rule.Field)
		// Never changed at all is not "stopped": there is no moment it went
		// quiet from, and a want that has only just been placed would
		// otherwise hold the alert from its first breath.
		if !ok || now.Sub(last) < d {
			return "", false
		}
		what := "state"
		if rule.Field != "" {
			what = rule.Field
		}
		return fmt.Sprintf("%s unchanged since %s", what, last.Format("01-02 15:04")), true
	}
	return "", false
}

// lastChangedAt is when field last changed, or when any state did if field is "".
func (n *Want) lastChangedAt(field string) (time.Time, bool) {
	if field != "" {
		return n.GetStateUpdatedAt(field)
	}
	var last time.Time
	for _, t := range n.GetStateTimestamps() {
		if t.After(last) {
			last = t
		}
	}
	return last, !last.IsZero()
}

// parseAlertDuration reads a Go duration, plus whole days as "2d" — the unit
// a person reaches for first when they mean "if nothing has happened for a
// couple of days".
func parseAlertDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if days, ok := strings.CutSuffix(s, "d"); ok {
		n, err := strconv.Atoi(days)
		if err != nil || n <= 0 {
			return 0, fmt.Errorf("invalid days %q", s)
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("invalid duration %q", s)
	}
	return d, nil
}
