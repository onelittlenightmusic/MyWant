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
// A toggle event is delivered via POST /api/v1/webhooks/{id} with
// {"action":"going"} or {"action":"stopped"}.
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
	gw.StoreState("last_action_at", "")
}

func (gw *GoingWant) IsAchieved() bool { return false }

// Progress processes a going/stopped action delivered via webhook — the
// sidebar's own toggle, not a footstep (see button_occupancy.go's
// toggleGoingOnStep for that path). Applies to every character this want
// currently targets (its live `characters` list — static config or
// footstep occupancy), same as a footstep would for whoever's on it right
// now.
func (gw *GoingWant) Progress() {
	ConsumeWebhookAction(&gw.Want, "last_action_at", func(action string, _ map[string]any) bool {
		var goingVal bool
		switch action {
		case "going":
			goingVal = true
		case "stopped":
			goingVal = false
		default:
			return false
		}
		for _, id := range GetCurrent(&gw.Want, "characters", []string(nil)) {
			applyGoingTo(id, goingVal, false)
		}
		return true
	})
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
