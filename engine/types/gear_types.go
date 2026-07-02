package types

import . "mywant/engine/core"

func init() {
	RegisterWithInit(func() {
		RegisterWantImplementation[GearWant, GearLocals]("gear")
	})
}

type GearLocals struct{}

// GearWant is a persistent speed-multiplier control that targets one or more
// characters (drive category). When multiple gear wants target the same
// character, the drive engine multiplies their values together to compute
// the character's effective speed multiplier. A value change is delivered
// via POST /api/v1/webhooks/{id} with {"action":"set","value":...}.
type GearWant struct{ Want }

func (g *GearWant) GetLocals() *GearLocals {
	return CheckLocalsInitialized[GearLocals](&g.Want)
}

func (g *GearWant) Initialize() {
	g.SetCurrent("value", g.GetFloatParam("default", 1))
	g.SetCurrent("min", g.GetFloatParam("min", 0))
	g.SetCurrent("max", g.GetFloatParam("max", 5))
	g.SetCurrent("step", g.GetFloatParam("step", 0.1))
	if chars := g.GetStringSliceParam("characters"); len(chars) > 0 {
		g.SetCurrent("characters", chars)
	}
	g.StoreState("last_action_at", "")
}

func (g *GearWant) IsAchieved() bool { return false }

// Progress processes a value change delivered via webhook.
func (g *GearWant) Progress() {
	ConsumeWebhookAction(&g.Want, "last_action_at", func(action string, pm map[string]any) bool {
		if action != "set" {
			return false
		}
		v, ok := pm["value"].(float64)
		if !ok {
			return false
		}
		g.SetCurrent("value", v)
		return true
	})
}
