package types

import (
	. "mywant/engine/core"
)

func init() {
	RegisterWithInit(func() {
		RegisterWantImplementation[ChoiceWant, ChoiceLocals]("choice")
	})
}

// ChoiceLocals holds type-specific local state.
type ChoiceLocals struct{}

// ChoiceWant allows selecting a value from a list and propagating it to a target parameter.
// A selection event is delivered via POST /api/v1/webhooks/{id} with {"action":"select","value":...}.
type ChoiceWant struct {
	Want
}

func (c *ChoiceWant) GetLocals() *ChoiceLocals {
	return CheckLocalsInitialized[ChoiceLocals](&c.Want)
}

func (c *ChoiceWant) Initialize() {
	if def := c.GetStringParam("default", ""); def != "" {
		c.SetCurrent("selected", def)
	}
	c.StoreState("last_action_at", "")
}

// IsAchieved always returns false — choice is a persistent control.
func (c *ChoiceWant) IsAchieved() bool { return false }

func (c *ChoiceWant) Progress() {
	ConsumeWebhookAction(&c.Want, "last_action_at", func(action string, pm map[string]any) bool {
		if action != "select" {
			return false
		}
		c.SetCurrent("selected", pm["value"])
		return true
	})
}
