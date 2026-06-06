package types

import . "mywant/engine/core"

func init() {
	RegisterWithInit(func() {
		RegisterWantImplementation[SwitchWant, SwitchLocals]("switch")
	})
}

type SwitchLocals struct{}

// SwitchWant is a persistent toggle that stores its on/off state.
// A toggle event is delivered via POST /api/v1/webhooks/{id} with {"action":"on"} or {"action":"off"}.
// webhook_payload and webhook_received_at are declared in switch.yaml (label: current),
// so Progress() can read them via GetCurrent.
type SwitchWant struct{ Want }

func (sw *SwitchWant) GetLocals() *SwitchLocals {
	return CheckLocalsInitialized[SwitchLocals](&sw.Want)
}

func (sw *SwitchWant) Initialize() {
	if _, ok := sw.GetCurrent("on"); !ok {
		sw.SetCurrent("on", false)
	}
	label := sw.GetStringParam("label", "Switch")
	sw.SetCurrent("label", label)
	if tp := sw.GetStringParam("target_param", ""); tp != "" {
		sw.SetCurrent("target_param", tp)
	}
	sw.StoreState("last_action_at", "")
}

func (sw *SwitchWant) IsAchieved() bool { return false }

// Progress processes on/off action delivered via webhook.
func (sw *SwitchWant) Progress() {
	ConsumeWebhookAction(&sw.Want, "last_action_at", func(action string, _ map[string]any) bool {
		switch action {
		case "on":
			sw.SetCurrent("on", true)
		case "off":
			sw.SetCurrent("on", false)
		default:
			return false
		}
		return true
	})
}
