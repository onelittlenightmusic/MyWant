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

// ChoiceWant allows selecting a value from a list (provided via State)
// and propagating it to a target parameter.
type ChoiceWant struct {
	Want
}

func (c *ChoiceWant) GetLocals() *ChoiceLocals {
	return CheckLocalsInitialized[ChoiceLocals](&c.Want)
}

func (c *ChoiceWant) Initialize() {
	// Initial selection can be provided via 'default' param
	if def := c.GetStringParam("default", ""); def != "" {
		c.StoreState("selected", def)
	}
}

// IsAchieved always returns false — choice is a persistent control.
func (c *ChoiceWant) IsAchieved() bool { return false }

func (c *ChoiceWant) Progress() {
	// choices is populated via spec.imports (global state → local key).
	// getState("choices") transparently reads from global state — no explicit fetch needed.
	//
	// The selected value is propagated to the parent via expose entries, e.g.:
	//   exposes:
	//     - currentState: "selected"
	//       asGoal: "target_key"
	// Re-emit on each tick to ensure initial propagation fires after RegisterWant.
	if selected, ok := c.GetCurrent("selected"); ok && selected != nil {
		c.SetCurrent("selected", selected)
	}
}
