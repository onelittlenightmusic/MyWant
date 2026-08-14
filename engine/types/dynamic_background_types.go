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

	// Whose board this dresses.
	//
	// A background belongs to a person: it is something a character chose to
	// look at, and two people sharing a server need not share a picture. There
	// used to be a server-wide one underneath, which meant a picture nobody
	// had chosen could not be traced to anybody and could not be told from
	// another's — so a want that names no one now says so rather than dressing
	// everybody's board at once.
	characterID := want.GetStringParam("character_id", "")
	if characterID == "" {
		want.SetCurrent("status", "waiting: set character_id to whose canvas this dresses")
		return false, nil
	}
	if !SetCharacterCanvasBg(characterID, newURL) {
		msg := "no such character: " + characterID
		want.SetCurrent("last_error", msg)
		want.SetCurrent("status", "error: "+msg)
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
