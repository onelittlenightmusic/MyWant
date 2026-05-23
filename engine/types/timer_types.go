package types

import (
	. "mywant/engine/core"
)

func init() {
	RegisterWithInit(func() {
		RegisterWantImplementation[TimerWant, TimerLocals]("timer")
	})
}

// TimerLocals holds type-specific local state to detect changes.
type TimerLocals struct {
	LastEvery        string `json:"last_every" yaml:"last_every"`
	LastAt           string `json:"last_at" yaml:"last_at"`
	LastTimerMode    string `json:"last_timer_mode" yaml:"last_timer_mode"`
	LastAtRecurrence string `json:"last_at_recurrence" yaml:"last_at_recurrence"`
}

// scheduleStateKey is the canonical current-state key for the computed WhenSpec output.
const scheduleStateKey = "schedule"

// TimerWant computes a WhenSpec (every/at schedule) and exposes it as "timer_spec" current state.
// The value is propagated to the parent via expose entries, e.g.:
//
//	exposes:
//	  - currentState: "timer_spec"
//	    asGoal: "schedule"
//
// For top-level wants writing to global state, use "as" instead of "asGoal".
type TimerWant struct{ Want }

func (t *TimerWant) GetLocals() *TimerLocals {
	return CheckLocalsInitialized[TimerLocals](&t.Want)
}

func (t *TimerWant) Initialize() {
	every := t.GetStringParam("default_every", "5m")
	at := ""
	timerMode := "every"
	atRecurrence := ""

	t.SetInternal("every", every)
	t.SetInternal("at", at)
	t.SetInternal("timer_mode", timerMode)
	t.SetInternal("at_recurrence", atRecurrence)

	locals := t.GetLocals()
	locals.LastEvery = every
	locals.LastAt = at
	locals.LastTimerMode = timerMode
	locals.LastAtRecurrence = atRecurrence

	// Compute and store initial schedule so expose handlers can propagate it on first tick.
	t.propagateTimer(every, at, timerMode, atRecurrence)
}

// IsAchieved always returns false — the timer is a persistent control want.
func (t *TimerWant) IsAchieved() bool { return false }

func (t *TimerWant) Progress() {
	locals := t.GetLocals()

	every := GetInternal[string](&t.Want, "every", "")
	at := GetInternal[string](&t.Want, "at", "")
	timerMode := GetInternal[string](&t.Want, "timer_mode", "every")
	atRecurrence := GetInternal[string](&t.Want, "at_recurrence", "")

	// Propagate only when relevant values have changed.
	if every != locals.LastEvery || at != locals.LastAt ||
		timerMode != locals.LastTimerMode || atRecurrence != locals.LastAtRecurrence {
		locals.LastEvery = every
		locals.LastAt = at
		locals.LastTimerMode = timerMode
		locals.LastAtRecurrence = atRecurrence
		t.propagateTimer(every, at, timerMode, atRecurrence)
	}
}

func (t *TimerWant) propagateTimer(every, at, timerMode, atRecurrence string) {
	var spec map[string]any

	if timerMode == "at" {
		// at モード: 時刻 + day/week の繰り返しのみ採用
		if at == "" {
			return
		}
		recurrenceEvery := "1d" // every day がデフォルト
		if atRecurrence == "week" {
			recurrenceEvery = "7d"
		}
		spec = map[string]any{
			"at":    at,
			"every": recurrenceEvery,
		}
	} else {
		// every モード: インターバルのみ採用、at は付与しない
		if every == "" {
			return
		}
		spec = map[string]any{"every": every}
	}

	t.SetCurrent(scheduleStateKey, spec)
}
