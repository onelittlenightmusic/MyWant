package types

import . "mywant/engine/core"

func init() {
	RegisterWithInit(func() {
		RegisterWantImplementation[HeartEffectWant, HeartEffectLocals]("heart_effect")
	})
}

type HeartEffectLocals struct{}

type HeartEffectWant struct{ Want }

func (h *HeartEffectWant) GetLocals() *HeartEffectLocals {
	return CheckLocalsInitialized[HeartEffectLocals](&h.Want)
}

func (h *HeartEffectWant) Initialize() {
	if _, ok := h.GetCurrent("heart_triggers"); !ok {
		h.SetCurrent("heart_triggers", 0)
	}
	h.StoreState("last_trigger_at", "")
}

func (h *HeartEffectWant) IsAchieved() bool { return false }

func (h *HeartEffectWant) Progress() {
	ConsumeWebhookAction(&h.Want, "last_trigger_at", func(action string, _ map[string]any) bool {
		if action != "trigger" {
			return false
		}
		count := GetCurrent[float64](h, "heart_triggers", 0)
		h.SetCurrent("heart_triggers", count+1)
		return true
	})
}
