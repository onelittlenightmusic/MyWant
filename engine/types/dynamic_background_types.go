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
//
// Each tick asks what the named character is wearing and, if it is not this
// picture, dresses them in it. It used to ask itself instead — comparing the
// incoming URL against the one it remembered applying, and doing nothing when
// they matched. Everything about that memory could be true while the screen
// disagreed: point the want at somebody else and the URL had not changed, so
// nobody was dressed and the previous character kept a picture that was no
// longer theirs; clear a character's background by hand and it never came
// back, because the want was still sure it had applied it.
//
// Asking the character is the same work and cannot drift: what is applied is
// read from where it is applied.
func dynamicBackgroundMonitorFn(_ context.Context, want *Want) (bool, error) {
	applied, status, failure := dynamicBackgroundDress(want)

	// The one place this want writes anything, and only where a value differs
	// from what is already there. Every field is worked out afresh each tick,
	// which is what keeps them honest; a tick that finds nothing to say is
	// silent rather than restating itself.
	//
	// Status is written here rather than beside the step that produced it,
	// which is the shape the stale status came from: the early return that
	// reported "waiting: connect source want" during a reconcile was the last
	// one that ever spoke, and the line sat there over a background that had
	// long since been applied. Every path now ends in the same place.
	if status != GetCurrent(want, "status", "") {
		want.SetCurrent("status", status)
	}
	if failure != GetCurrent(want, "last_error", "") {
		want.SetCurrent("last_error", failure)
	}
	// A tick that applied nothing leaves the last picture standing: the
	// character is still wearing it, and current_image_url is the final result
	// other wants read.
	if applied != "" && applied != GetCurrent(want, "current_image_url", "") {
		want.SetCurrent("current_image_url", applied)
	}
	return false, nil
}

// dynamicBackgroundDress puts the picture on the character named by
// character_id, and reports what came of it: what is now on their canvas (empty
// if this tick applied nothing), a status line, and an error to record.
//
// It writes to the character and to the log, and to no want state — saying what
// happened is the caller's job, so there is exactly one place where the want's
// own account of itself is kept up to date.
func dynamicBackgroundDress(want *Want) (applied, status, failure string) {
	newURL := GetCurrent(want, "source_image_url", "")
	if newURL == "" {
		return "", "waiting: connect source want via spec.imports", ""
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
		return "", "waiting: set character_id to whose canvas this dresses", ""
	}

	// Nothing is taken off anyone here. Re-pointing this want at somebody else
	// leaves the previous character wearing the last picture it gave them,
	// which they can clear from their own settings — undressing them
	// automatically would mean knowing who they were, and want state does not
	// survive a param edit (the want is rebuilt from the edited config),
	// which is exactly the moment that memory would be needed.
	who, ok := GetCharacter(characterID)
	if !ok {
		msg := "no such character: " + characterID
		return "", "error: " + msg, msg
	}

	if who.Display.CanvasBgURL != newURL {
		if !SetCharacterCanvasBg(characterID, newURL) {
			msg := "no such character: " + characterID
			return "", "error: " + msg, msg
		}
		want.StoreLog("[DYNAMIC-BG] applied to %s: %s", who.Name, newURL)
	}

	short := newURL
	if len(short) > 60 {
		short = short[:57] + "…"
	}
	return newURL, "applied to " + who.Name + ": " + short, ""
}
