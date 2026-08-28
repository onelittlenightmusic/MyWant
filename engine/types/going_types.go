package types

import . "mywant/engine/core"

func init() {
	RegisterWithInit(func() {
		RegisterWantImplementation[GoingWant, GoingLocals]("going")
	})
}

type GoingLocals struct{}

// GoingWant is an instruction, not a state: stepping on it (or toggling its
// card from the sidebar) tells every character it currently targets to flip
// their *own* going/stopped flag — kept on that character's own
// "character_motion" want (see character_motion_types.go's "going" field),
// not here. Two characters who happen to share this same want's tile each
// flip only their own flag; neither can start or stop the other, which is
// the whole reason going lives with the character and not on a want they
// might be sharing.
//
// Every way of asking arrives the same way: as a webhook payload on this
// want, drained by Progress. A footstep is one too (see button_occupancy.go),
// so there is exactly one place that decides what a going instruction means
// and exactly one that writes the flag — the two used to be written twice, and
// only one of the copies knew who had asked.
//
//	{"action": "going"}                          every character this want targets
//	{"action": "stopped", "character_id": "..."}  that character alone
//	{"action": "toggle",  "character_id": "..."}  flip that character's own flag
//
// Queue-based (AppendState/DrainState) rather than the single-slot
// webhook_payload other user-control wants use: footsteps are driven by
// movement and two characters can land in the same ~100ms reconcile tick, which
// a single overwritable slot would silently drop.
type GoingWant struct{ Want }

func (gw *GoingWant) GetLocals() *GoingLocals {
	return CheckLocalsInitialized[GoingLocals](&gw.Want)
}

func (gw *GoingWant) Initialize() {
	if chars := gw.GetStringSliceParam("characters"); len(chars) > 0 {
		gw.SetCurrent("characters", chars)
		// A statically-targeted character (via the "characters" param, as
		// opposed to one who arrives later by footstep) starts at this
		// want's configured default the moment it's deployed — the same
		// "cart-going: default: true" case going.yaml's own example
		// documents. Only seeds a character who has no going flag of their
		// own yet; never overwrites one a player already toggled, the same
		// restart-survival guard direction/gear's own dx/dy/value use.
		defaultGoing := gw.GetBoolParam("default", false)
		for _, id := range chars {
			applyGoingTo(id, defaultGoing, true)
		}
	}
}

func (gw *GoingWant) IsAchieved() bool { return false }

// Progress drains every going instruction accumulated since the last tick and
// carries each one out, in the order it arrived.
func (gw *GoingWant) Progress() {
	for _, entry := range gw.DrainState("webhook_queue") {
		m, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		pm, ok := m["payload"].(map[string]any)
		if !ok {
			continue
		}
		action, _ := pm["action"].(string)
		characterID, _ := pm["character_id"].(string)
		gw.applyGoingAction(action, characterID)
	}
}

// applyGoingAction carries out one going instruction.
//
// Who it applies to is the whole of what used to differ between the two paths:
// a footstep and a card press are both made by somebody, and name them; a
// webhook from a script or an agent names nobody, and then the want's own
// targets are who it meant.
//
// "toggle" is the pressure-plate meaning — step on it again to turn it off —
// and is per character, read from the flag that character actually has now.
func (gw *GoingWant) applyGoingAction(action, characterID string) {
	targets := []string{characterID}
	if characterID == "" {
		targets = GetCurrent(&gw.Want, "characters", []string(nil))
	}
	for _, id := range targets {
		if id == "" {
			continue
		}
		switch action {
		case "going":
			applyGoingTo(id, true, false)
		case "stopped":
			applyGoingTo(id, false, false)
		case "toggle":
			applyGoingTo(id, !goingOf(id), false)
		}
	}
}

// goingOf reads a character's own going flag, for the toggle case. Same want
// applyGoingTo writes to, looked up the same way — see its doc for why the flag
// lives on the character rather than on whichever want asked about it.
func goingOf(characterID string) bool {
	builder := GetGlobalChainBuilder()
	if builder == nil || characterID == "" {
		return false
	}
	name := "motion-" + characterID
	for _, w := range builder.GetWants() {
		if w.Metadata.ID == name {
			v, _ := w.GetCurrent("going")
			b, _ := v.(bool)
			return b
		}
	}
	return false
}

// applyGoingTo sets characterID's own going/stopped flag by writing straight
// to their "character_motion" want's "going" field — see that type's own
// doc for why the flag lives there and not on whichever going want asked
// for the change.
//
// onlyIfUnset restricts the write to a character who has no going flag of
// their own yet, for Initialize()'s static-default seeding — never touches
// one a player already toggled just because the want that names them got
// redeployed or the server restarted.
//
// The name "motion-"+characterID must match characterMotionWantName in
// engine/server/character_motion_wants.go; duplicated rather than imported
// because engine/types cannot import engine/server (see
// engine/core/character_motion_hook.go for the same constraint elsewhere).
func applyGoingTo(characterID string, goingVal bool, onlyIfUnset bool) {
	builder := GetGlobalChainBuilder()
	if builder == nil || characterID == "" {
		return
	}
	name := "motion-" + characterID
	for _, w := range builder.GetWants() {
		if w.Metadata.ID != name {
			continue
		}
		if onlyIfUnset {
			if _, ok := w.GetCurrent("going"); ok {
				return
			}
		}
		w.SetCurrent("going", goingVal)
		return
	}
}
