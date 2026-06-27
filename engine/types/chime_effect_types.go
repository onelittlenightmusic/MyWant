package types

import . "mywant/engine/core"

func init() {
	RegisterWithInit(func() {
		RegisterWantImplementation[ChimeEffectWant, ChimeEffectLocals]("chime_effect")
	})
}

type ChimeEffectLocals struct{}

type ChimeEffectWant struct{ Want }

func (c *ChimeEffectWant) GetLocals() *ChimeEffectLocals {
	return CheckLocalsInitialized[ChimeEffectLocals](&c.Want)
}

func (c *ChimeEffectWant) Initialize() {
	if _, ok := c.GetCurrent("chime_triggers"); !ok {
		c.SetCurrent("chime_triggers", 0)
	}
	c.StoreState("last_trigger_at", "")
}

func (c *ChimeEffectWant) IsAchieved() bool { return false }

func (c *ChimeEffectWant) Progress() {
	ConsumeWebhookAction(&c.Want, "last_trigger_at", func(action string, _ map[string]any) bool {
		if action != "trigger" {
			return false
		}
		count := GetCurrent[float64](c, "chime_triggers", 0)
		c.SetCurrent("chime_triggers", count+1)
		return true
	})
}
