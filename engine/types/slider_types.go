package types

import . "mywant/engine/core"

func init() {
	RegisterWithInit(func() {
		RegisterWantImplementation[SliderWant, SliderLocals]("slider")
	})
}

// SliderLocals holds type-specific local state.
type SliderLocals struct{}

// SliderWant dynamically controls a parent want's goal or state based on its current value.
// The output is propagated via expose entries, e.g.:
//
//	exposes:
//	  - currentState: "value"
//	    asGoal: "budget_limit"
type SliderWant struct{ Want }

func (s *SliderWant) GetLocals() *SliderLocals {
	return CheckLocalsInitialized[SliderLocals](&s.Want)
}

func (s *SliderWant) Initialize() {
	s.StoreState("value", s.GetFloatParam("default", 0))
	s.StoreState("min", s.GetFloatParam("min", 0))
	s.StoreState("max", s.GetFloatParam("max", 100))
	s.StoreState("step", s.GetFloatParam("step", 1))
}

// IsAchieved always returns false — slider is a persistent control.
func (s *SliderWant) IsAchieved() bool { return false }

// Progress re-emits the current value so expose handlers fire on the first tick
// (initial propagation) as well as on every user-driven value change.
func (s *SliderWant) Progress() {
	value, _ := s.GetStateFloat64("value", 0.0)
	s.SetCurrent("value", value)
}
