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
// A value change is delivered via POST /api/v1/webhooks/{id} with {"action":"set","value":...}.
type SliderWant struct{ Want }

func (s *SliderWant) GetLocals() *SliderLocals {
	return CheckLocalsInitialized[SliderLocals](&s.Want)
}

func (s *SliderWant) Initialize() {
	s.SetCurrent("value", s.GetFloatParam("default", 0))
	s.SetCurrent("min", s.GetFloatParam("min", 0))
	s.SetCurrent("max", s.GetFloatParam("max", 100))
	s.SetCurrent("step", s.GetFloatParam("step", 1))
	if tp := s.GetStringParam("target_param", ""); tp != "" {
		s.SetCurrent("target_param", tp)
	}
	s.StoreState("last_action_at", "")
}

// IsAchieved always returns false — slider is a persistent control.
func (s *SliderWant) IsAchieved() bool { return false }

// Progress processes a value change delivered via webhook.
func (s *SliderWant) Progress() {
	ConsumeWebhookAction(&s.Want, "last_action_at", func(action string, pm map[string]any) bool {
		if action != "set" {
			return false
		}
		v, ok := pm["value"].(float64)
		if !ok {
			return false
		}
		s.SetCurrent("value", v)
		return true
	})
}
