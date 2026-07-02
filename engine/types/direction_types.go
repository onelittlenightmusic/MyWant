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

// maxDirectionMagnitude caps a direction want's vector length (grid cells),
// matching the radius of its on-canvas picker.
const maxDirectionMagnitude = 5

// DirectionWant is a persistent grid-vector heading control that targets one
// or more characters (drive category). Its value is a 2D vector (dx, dy) in
// grid-cell units (0=east/+X, increases clockwise toward +Y), with a
// magnitude of 1 to maxDirectionMagnitude. When multiple direction wants
// target the same character, the drive engine resolves the effective heading
// as the angle of the vector sum of each targeting want's (dx, dy) — so a
// longer vector pulls the combined heading harder than a shorter one.
// Movement speed is governed solely by gear, not by direction magnitude.
// A value change is delivered via POST /api/v1/webhooks/{id} with
// {"action":"set","dx":...,"dy":...}.
type DirectionWant struct{ Want }

func (d *DirectionWant) GetLocals() *DirectionLocals {
	return CheckLocalsInitialized[DirectionLocals](&d.Want)
}

func (d *DirectionWant) Initialize() {
	dx, dy := clampDirectionVector(d.GetFloatParam("dx", 1), d.GetFloatParam("dy", 0))
	d.SetCurrent("dx", dx)
	d.SetCurrent("dy", dy)
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
		dxRaw, ok1 := pm["dx"].(float64)
		dyRaw, ok2 := pm["dy"].(float64)
		if !ok1 || !ok2 {
			return false
		}
		dx, dy := clampDirectionVector(dxRaw, dyRaw)
		d.SetCurrent("dx", dx)
		d.SetCurrent("dy", dy)
		return true
	})
}

// clampDirectionVector scales (dx, dy) down proportionally if its magnitude
// exceeds maxDirectionMagnitude. A zero vector is pushed out to a unit
// vector along +X, since direction always has a magnitude of at least 1.
func clampDirectionVector(dx, dy float64) (float64, float64) {
	mag := math.Hypot(dx, dy)
	if mag == 0 {
		return 1, 0
	}
	if mag > maxDirectionMagnitude {
		scale := maxDirectionMagnitude / mag
		return dx * scale, dy * scale
	}
	return dx, dy
}
