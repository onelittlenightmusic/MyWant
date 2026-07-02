package types

import (
	"math"

	. "mywant/engine/core"
)

func init() {
	RegisterWithInit(func() {
		RegisterWantImplementation[DirectionWant, DirectionLocals]("direction")
	})
}

type DirectionLocals struct{}

// DirectionWant is a persistent 0-360° heading control that targets one or
// more characters (drive category). When multiple direction wants target the
// same character, the drive engine resolves the effective heading as the
// angle of the vector sum of each targeting want's unit heading vector. A
// value change is delivered via POST /api/v1/webhooks/{id} with
// {"action":"set","value":...} (degrees).
type DirectionWant struct{ Want }

func (d *DirectionWant) GetLocals() *DirectionLocals {
	return CheckLocalsInitialized[DirectionLocals](&d.Want)
}

func (d *DirectionWant) Initialize() {
	d.SetCurrent("degrees", wrapDegrees(d.GetFloatParam("default", 0)))
	if chars := d.GetStringSliceParam("characters"); len(chars) > 0 {
		d.SetCurrent("characters", chars)
	}
	d.StoreState("last_action_at", "")
}

func (d *DirectionWant) IsAchieved() bool { return false }

// Progress processes a value change delivered via webhook.
func (d *DirectionWant) Progress() {
	ConsumeWebhookAction(&d.Want, "last_action_at", func(action string, pm map[string]any) bool {
		if action != "set" {
			return false
		}
		v, ok := pm["value"].(float64)
		if !ok {
			return false
		}
		d.SetCurrent("degrees", wrapDegrees(v))
		return true
	})
}

// wrapDegrees normalizes a heading into [0, 360).
func wrapDegrees(v float64) float64 {
	v = math.Mod(v, 360)
	if v < 0 {
		v += 360
	}
	return v
}
