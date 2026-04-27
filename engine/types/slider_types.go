package types

import . "mywant/engine/core"

func init() {
	RegisterWithInit(func() {
		RegisterWantImplementation[SliderWant, SliderLocals]("slider")
	})
}

// SliderLocals holds type-specific local state.
type SliderLocals struct{}

// SliderWant dynamically controls a parent want's parameter based on its current value.
// The target parent parameter is specified via the "target_param" param.
type SliderWant struct{ Want }

func (s *SliderWant) GetLocals() *SliderLocals {
	return CheckLocalsInitialized[SliderLocals](&s.Want)
}

func (s *SliderWant) Initialize() {
	s.StoreState("value", s.GetFloatParam("default", 0))
	s.StoreState("min", s.GetFloatParam("min", 0))
	s.StoreState("max", s.GetFloatParam("max", 100))
	s.StoreState("step", s.GetFloatParam("step", 1))
	s.StoreState("target_param", s.GetStringParam("target_param", ""))
}

// IsAchieved always returns false — slider is a persistent control.
func (s *SliderWant) IsAchieved() bool { return false }

// Progress reads the current value and propagates it to the parent's target parameter,
// or to global parameters if there is no parent.
func (s *SliderWant) Progress() {
	// Re-sync target_param from params if state is empty (handles late param assignment)
	targetParam, _ := s.GetStateString("target_param", "")
	if targetParam == "" {
		targetParam = s.GetStringParam("target_param", "")
		if targetParam != "" {
			s.StoreState("target_param", targetParam)
		} else {
			return
		}
	}

	value, _ := s.GetStateFloat64("value", 0.0)
	s.PropagateParameter(targetParam, value)
}
