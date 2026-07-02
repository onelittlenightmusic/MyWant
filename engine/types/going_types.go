package types

import . "mywant/engine/core"

func init() {
	RegisterWithInit(func() {
		RegisterWantImplementation[GoingWant, GoingLocals]("going")
	})
}

type GoingLocals struct{}

// GoingWant is a persistent going/stopped toggle that targets one or more
// characters (drive category). The drive engine reads its "going" state each
// tick to decide whether the targeted characters should move; when multiple
// going wants target the same character, stopped wins over going.
// A toggle event is delivered via POST /api/v1/webhooks/{id} with
// {"action":"going"} or {"action":"stopped"}.
type GoingWant struct{ Want }

func (gw *GoingWant) GetLocals() *GoingLocals {
	return CheckLocalsInitialized[GoingLocals](&gw.Want)
}

func (gw *GoingWant) Initialize() {
	gw.SetCurrent("going", gw.GetBoolParam("default", false))
	if chars := gw.GetStringSliceParam("characters"); len(chars) > 0 {
		gw.SetCurrent("characters", chars)
	}
	gw.StoreState("last_action_at", "")
}

func (gw *GoingWant) IsAchieved() bool { return false }

// Progress processes a going/stopped action delivered via webhook.
func (gw *GoingWant) Progress() {
	ConsumeWebhookAction(&gw.Want, "last_action_at", func(action string, _ map[string]any) bool {
		switch action {
		case "going":
			gw.SetCurrent("going", true)
		case "stopped":
			gw.SetCurrent("going", false)
		default:
			return false
		}
		return true
	})
}
