package types

import (
	. "mywant/engine/core"
)

func init() {
	RegisterWithInit(func() {
		RegisterWantImplementation[TimerWant, TimerLocals]("timer")
	})
}

// TimerLocals holds type-specific local state.
type TimerLocals struct{}

// scheduleStateKey is the canonical current-state key for the computed WhenSpec output.
const scheduleStateKey = "schedule"

// TimerWant computes a WhenSpec (every/at schedule) and stores it as the "schedule" current state.
// A schedule change is delivered via POST /api/v1/webhooks/{id} with:
//
//	{"action":"set","timer_mode":"every|at","every":"5m","at":"09:00","at_recurrence":"day|week","at_weekday":"mon"}
//
// Use asGlobalParam in the want's exposes spec to write the schedule to a named global parameter:
//
//	exposes:
//	  - currentState: schedule
//	    asGlobalParam: global_timer_1
type TimerWant struct{ Want }

func (t *TimerWant) GetLocals() *TimerLocals {
	return CheckLocalsInitialized[TimerLocals](&t.Want)
}

func (t *TimerWant) Initialize() {
	every := t.GetStringParam("default_every", "5m")
	t.SetInternal("every", every)
	t.SetInternal("at", "")
	t.SetInternal("timer_mode", "every")
	t.SetInternal("at_recurrence", "")
	t.SetInternal("at_weekday", "")
	t.StoreState("last_action_at", "")
	t.propagateTimer(every, "", "every", "")
}

// IsAchieved always returns false — the timer is a persistent control want.
func (t *TimerWant) IsAchieved() bool { return false }

// Progress processes a schedule update delivered via webhook.
func (t *TimerWant) Progress() {
	ConsumeWebhookAction(&t.Want, "last_action_at", func(action string, pm map[string]any) bool {
		if action != "set" {
			return false
		}
		every, _ := pm["every"].(string)
		at, _ := pm["at"].(string)
		timerMode, _ := pm["timer_mode"].(string)
		if timerMode == "" {
			timerMode = "every"
		}
		atRecurrence, _ := pm["at_recurrence"].(string)
		atWeekday, _ := pm["at_weekday"].(string)
		t.SetInternal("every", every)
		t.SetInternal("at", at)
		t.SetInternal("timer_mode", timerMode)
		t.SetInternal("at_recurrence", atRecurrence)
		t.SetInternal("at_weekday", atWeekday)
		t.propagateTimer(every, at, timerMode, atRecurrence)
		return true
	})
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
