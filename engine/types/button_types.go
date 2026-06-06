package types

import . "mywant/engine/core"

func init() {
	RegisterWithInit(func() {
		RegisterWantImplementation[ButtonWant, ButtonLocals]("button")
	})
}

type ButtonLocals struct{}

// ButtonWant is a persistent user-control that counts button presses.
// A press event is delivered via POST /api/v1/webhooks/{id} with {"action":"press"}.
// webhook_payload and webhook_received_at are declared in button.yaml (label: current),
// so Progress() can read them via GetCurrent.
type ButtonWant struct{ Want }

func (b *ButtonWant) GetLocals() *ButtonLocals {
	return CheckLocalsInitialized[ButtonLocals](&b.Want)
}

func (b *ButtonWant) Initialize() {
	b.StoreState("label", b.GetStringParam("label", "Push"))
	if _, ok := b.GetCurrent("pressed_count"); !ok {
		b.SetCurrent("pressed_count", 0)
	}
	b.StoreState("last_press_at", "")
}

func (b *ButtonWant) IsAchieved() bool { return false }

// Progress increments pressed_count on each press event (delivered via webhook).
// The count is propagated to the parent via expose entries, e.g.:
//
//	exposes:
//	  - currentState: "pressed_count"
//	    asGoal: "trigger_count"
func (b *ButtonWant) Progress() {
	ConsumeWebhookAction(&b.Want, "last_press_at", func(action string, _ map[string]any) bool {
		if action != "press" {
			return false
		}
		count := GetCurrent[float64](b, "pressed_count", 0)
		b.SetCurrent("pressed_count", count+1)
		return true
	})
}
