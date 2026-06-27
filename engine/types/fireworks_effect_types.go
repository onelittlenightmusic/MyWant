package types

import . "mywant/engine/core"

func init() {
	RegisterWithInit(func() {
		RegisterWantImplementation[FireworksEffectWant, FireworksEffectLocals]("fireworks_effect")
	})
}

type FireworksEffectLocals struct{}

type FireworksEffectWant struct{ Want }

func (f *FireworksEffectWant) GetLocals() *FireworksEffectLocals {
	return CheckLocalsInitialized[FireworksEffectLocals](&f.Want)
}

func (f *FireworksEffectWant) Initialize() {
	if _, ok := f.GetCurrent("fireworks_triggers"); !ok {
		f.SetCurrent("fireworks_triggers", 0)
	}
	f.StoreState("last_trigger_at", "")
}

func (f *FireworksEffectWant) IsAchieved() bool { return false }

func (f *FireworksEffectWant) Progress() {
	ConsumeWebhookAction(&f.Want, "last_trigger_at", func(action string, _ map[string]any) bool {
		if action != "trigger" {
			return false
		}
		count := GetCurrent[float64](f, "fireworks_triggers", 0)
		f.SetCurrent("fireworks_triggers", count+1)
		return true
	})
}
