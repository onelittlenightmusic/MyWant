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

// TimerWant controls a global parameter of WhenSpec type (every/at).
// The user sets the scheduling interval via the dashboard and the value
// is propagated to the named global parameter so other wants can use it
// via fromGlobalParam references.
type TimerWant struct{ Want }

func (t *TimerWant) GetLocals() *TimerLocals {
	return CheckLocalsInitialized[TimerLocals](&t.Want)
}

func (t *TimerWant) Initialize() {
	targetParam := t.GetStringParam("target_param", "")
	t.StoreState("target_param", targetParam)

	// Seed from global param if already set, otherwise fall back to defaults.
	every := t.GetStringParam("default_every", "5m")
	at := ""
	timerMode := "every"
	atRecurrence := ""

	if targetParam != "" {
		if raw, ok := GetGlobalParameter(targetParam); ok {
			if m, ok := raw.(map[string]any); ok {
				if v, ok := m["every"].(string); ok && v != "" {
					every = v
				}
				if v, ok := m["at"].(string); ok {
					at = v
				}
			}
		}
	}

	t.StoreState("every", every)
	t.StoreState("at", at)
	t.StoreState("timer_mode", timerMode)
	t.StoreState("at_recurrence", atRecurrence)

	locals := t.GetLocals()
	locals.LastEvery = every
	locals.LastAt = at
	locals.LastTimerMode = timerMode
	locals.LastAtRecurrence = atRecurrence

	// Force propagation on startup so global param always reflects current mode.
	t.propagateTimer(every, at, timerMode, atRecurrence)
}

// IsAchieved always returns false — the timer is a persistent control want.
func (t *TimerWant) IsAchieved() bool { return false }

func (t *TimerWant) Progress() {
	locals := t.GetLocals()

	every, _ := t.GetStateString("every", "")
	at, _ := t.GetStateString("at", "")
	timerMode, _ := t.GetStateString("timer_mode", "every")
	atRecurrence, _ := t.GetStateString("at_recurrence", "")

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
	targetParam, _ := t.GetStateString("target_param", "")
	if targetParam == "" {
		targetParam = t.GetStringParam("target_param", "")
	}
	if targetParam == "" {
		return
	}

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

	t.PropagateParameter(targetParam, spec)
}
