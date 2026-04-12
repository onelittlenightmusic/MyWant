package types

import (
	. "mywant/engine/core"
)

func init() {
	RegisterWantImplementation[ChoiceWant, ChoiceLocals]("choice")
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
	c.StoreState("target_param", c.GetStringParam("target_param", ""))
	// Initial selection can be provided via 'default' param
	if def := c.GetStringParam("default", ""); def != "" {
		c.StoreState("selected", def)
	}
}

// IsAchieved always returns false — choice is a persistent control.
func (c *ChoiceWant) IsAchieved() bool { return false }
func (c *ChoiceWant) Progress() {
	// 1. パラメータから import パスを取得
	importPath := c.GetStringParam("choice_import_field", "")
	if importPath != "" {
		if val, ok := c.GetParentState(importPath); ok {
			c.StoreState("choices", val)
		}
	}

	targetParam := c.GetStringParam("target_param", "")
	selected, _ := c.GetCurrent("selected")
	if selected != nil && targetParam != "" {
		c.PropagateParameter(targetParam, selected)
	}
}

