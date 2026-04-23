package types

import (
	. "mywant/engine/core"
)

func init() {
	RegisterWantImplementation[TimerWant, TimerLocals]("timer")
}

// TimerLocals holds type-specific local state to detect changes.
type TimerLocals struct {
	LastEvery string `json:"last_every" yaml:"last_every"`
	LastAt    string `json:"last_at" yaml:"last_at"`
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

	locals := t.GetLocals()
	locals.LastEvery = every
	locals.LastAt = at
}

// IsAchieved always returns false — the timer is a persistent control want.
func (t *TimerWant) IsAchieved() bool { return false }

func (t *TimerWant) Progress() {
	locals := t.GetLocals()

	every, _ := t.GetStateString("every", "")
	at, _ := t.GetStateString("at", "")

	// Propagate only when values have changed.
	if every != locals.LastEvery || at != locals.LastAt {
		locals.LastEvery = every
		locals.LastAt = at
		t.propagateTimer(every, at)
	}
}

func (t *TimerWant) propagateTimer(every, at string) {
	targetParam, _ := t.GetStateString("target_param", "")
	if targetParam == "" {
		targetParam = t.GetStringParam("target_param", "")
	}
	if targetParam == "" || every == "" {
		return
	}

	spec := map[string]any{"every": every}
	if at != "" {
		spec["at"] = at
	}
	t.PropagateParameter(targetParam, spec)
}
