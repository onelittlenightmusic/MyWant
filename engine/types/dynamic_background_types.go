package types

import (
	"context"

	. "mywant/engine/core"
)

const dynamicBgAgentName = "dynamic_background_agent"
const dynamicBgCapability = "dynamic_background_agency"

func init() {
	RegisterWithInit(func() {
		RegisterWantImplementation[DynamicBackgroundWant, DynamicBackgroundLocals]("dynamic_background")
		RegisterMonitorAgentType(
			dynamicBgAgentName,
			[]Capability{Cap(dynamicBgCapability)},
			dynamicBackgroundMonitorFn,
		)
	})
}

// DynamicBackgroundLocals holds no extra per-instance state.
type DynamicBackgroundLocals struct{}

// DynamicBackgroundWant watches source_image_url (injected via spec.imports) and
// applies it as the canvas/dashboard background.
type DynamicBackgroundWant struct{ Want }

func (w *DynamicBackgroundWant) GetLocals() *DynamicBackgroundLocals {
	return CheckLocalsInitialized[DynamicBackgroundLocals](&w.Want)
}

func (w *DynamicBackgroundWant) Initialize() {
	w.SetCurrent("current_image_url", "")
	w.SetCurrent("status", "watching")
	w.SetCurrent("last_error", "")
	w.ExecuteAgents() //nolint:errcheck
}

func (w *DynamicBackgroundWant) IsAchieved() bool { return false }

func (w *DynamicBackgroundWant) Progress() {}

// dynamicBackgroundMonitorFn runs on each polling tick.
// source_image_url is injected by the engine via spec.imports.
func dynamicBackgroundMonitorFn(_ context.Context, want *Want) (bool, error) {
	currentURL := GetCurrent(want, "current_image_url", "")
	newURL := GetCurrent(want, "source_image_url", "")

	if newURL == "" {
		want.SetCurrent("status", "waiting: connect source want via spec.imports")
		return false, nil
	}

	if newURL == currentURL {
		want.SetCurrent("last_error", "")
		return false, nil
	}

	if err := SetCanvasBgURL(newURL); err != nil {
		want.SetCurrent("last_error", err.Error())
		want.SetCurrent("status", "error: "+err.Error())
		return false, nil
	}

	want.SetCurrent("current_image_url", newURL)
	short := newURL
	if len(short) > 60 {
		short = short[:57] + "…"
	}
	want.SetCurrent("status", "applied: "+short)
	want.SetCurrent("last_error", "")
	want.StoreLog("[DYNAMIC-BG] applied: %s", newURL)
	return false, nil
}
