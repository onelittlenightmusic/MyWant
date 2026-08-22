package server

import (
	"strconv"
	"testing"

	mywant "mywant/engine/core"
)

// buttonWantAt builds a form-type:button want (as far as this package's own
// logic is concerned — the button-ness itself is supplied by the isButton
// func passed to applyButtonOccupancy, not by anything on the want).
func buttonWantAt(id string, x, y int) *mywant.Want {
	w := &mywant.Want{Metadata: mywant.Metadata{ID: id, Type: "direction"}}
	// Real deployment gets this from the type definition's `state:` list
	// (SetStateLabels), which is what makes SetCurrent/GetCurrent("characters")
	// work at all — SetCurrent silently no-ops on an undeclared key.
	w.StateLabels = map[string]mywant.StateLabel{"characters": mywant.LabelCurrent}
	w.SetLabel(canvasLabelX, strconv.Itoa(x))
	w.SetLabel(canvasLabelY, strconv.Itoa(y))
	return w
}

// goingWantAt builds an actual "going" want — unlike buttonWantAt, whose
// Type is deliberately something else so tests can supply their own
// isButton predicate — because toggleGoingOnStep checks Metadata.Type
// directly, not through that predicate. Carries no "going" state of its
// own any more — going lives on each targeted character's own
// "character_motion" want (see motionWantFor) — this is purely the trigger.
func goingWantAt(id string, x, y int) *mywant.Want {
	w := &mywant.Want{Metadata: mywant.Metadata{ID: id, Type: "going"}}
	w.StateLabels = map[string]mywant.StateLabel{"characters": mywant.LabelCurrent}
	w.SetLabel(canvasLabelX, strconv.Itoa(x))
	w.SetLabel(canvasLabelY, strconv.Itoa(y))
	return w
}

// motionWantFor builds a character's own "character_motion" want, named the
// way ensureCharacterMotionWant names it — the thing toggleGoingOnStep
// actually writes "going" to. startingGoing seeds its initial flag.
func motionWantFor(characterID string, startingGoing bool) *mywant.Want {
	w := &mywant.Want{Metadata: mywant.Metadata{ID: characterMotionWantName(characterID), Type: "character_motion"}}
	w.StateLabels = map[string]mywant.StateLabel{"going": mywant.LabelCurrent}
	w.SetCurrent("going", startingGoing)
	return w
}

func goingOf(w *mywant.Want) bool {
	return mywant.GetCurrent(w, "going", false)
}

func isGoingButtonType(typeName string) bool { return typeName == "going" }

func nonButtonWantAt(id string, x, y int) *mywant.Want {
	w := &mywant.Want{Metadata: mywant.Metadata{ID: id, Type: "weather"}}
	w.SetLabel(canvasLabelX, strconv.Itoa(x))
	w.SetLabel(canvasLabelY, strconv.Itoa(y))
	return w
}

func isButtonType(typeName string) bool { return typeName == "direction" }

func resetButtonOccupancy() {
	buttonOccupancyMu.Lock()
	characterOnButton = map[string]string{}
	buttonOccupancyMu.Unlock()
}

func TestApplyButtonOccupancyAddsOnStep(t *testing.T) {
	resetButtonOccupancy()
	btn := buttonWantAt("btn-1", 5, 5)
	all := []*mywant.Want{btn}

	applyButtonOccupancy("chr-hero", 5, 5, all, isButtonType)

	got := characterIDsOf(btn)
	if len(got) != 1 || got[0] != "chr-hero" {
		t.Fatalf("expected chr-hero in characters, got %v", got)
	}
}

func TestApplyButtonOccupancyRemovesOnStepOff(t *testing.T) {
	resetButtonOccupancy()
	btn := buttonWantAt("btn-1", 5, 5)
	all := []*mywant.Want{btn}

	applyButtonOccupancy("chr-hero", 5, 5, all, isButtonType)
	applyButtonOccupancy("chr-hero", 9, 9, all, isButtonType) // walked away, no button there

	got := characterIDsOf(btn)
	if len(got) != 0 {
		t.Fatalf("expected chr-hero removed after stepping off, got %v", got)
	}
}

func TestApplyButtonOccupancySwitchesBetweenTwoButtons(t *testing.T) {
	resetButtonOccupancy()
	btnA := buttonWantAt("btn-a", 1, 1)
	btnB := buttonWantAt("btn-b", 2, 2)
	all := []*mywant.Want{btnA, btnB}

	applyButtonOccupancy("chr-hero", 1, 1, all, isButtonType)
	applyButtonOccupancy("chr-hero", 2, 2, all, isButtonType)

	if got := characterIDsOf(btnA); len(got) != 0 {
		t.Errorf("expected chr-hero removed from btn-a, got %v", got)
	}
	if got := characterIDsOf(btnB); len(got) != 1 || got[0] != "chr-hero" {
		t.Errorf("expected chr-hero in btn-b, got %v", got)
	}
}

func TestApplyButtonOccupancyMultipleCharactersOnOneButton(t *testing.T) {
	resetButtonOccupancy()
	btn := buttonWantAt("btn-1", 3, 3)
	all := []*mywant.Want{btn}

	applyButtonOccupancy("chr-a", 3, 3, all, isButtonType)
	applyButtonOccupancy("chr-b", 3, 3, all, isButtonType)

	got := characterIDsOf(btn)
	if len(got) != 2 {
		t.Fatalf("expected both characters on btn-1, got %v", got)
	}
}

func TestApplyButtonOccupancyIgnoresNonButtonWant(t *testing.T) {
	resetButtonOccupancy()
	notBtn := nonButtonWantAt("weather-1", 4, 4)
	all := []*mywant.Want{notBtn}

	applyButtonOccupancy("chr-hero", 4, 4, all, isButtonType)

	got := characterIDsOf(notBtn)
	if len(got) != 0 {
		t.Fatalf("non-button want should never gain a characters entry, got %v", got)
	}
}

func TestApplyButtonOccupancyStandingStillDoesNothing(t *testing.T) {
	resetButtonOccupancy()
	btn := buttonWantAt("btn-1", 5, 5)
	all := []*mywant.Want{btn}

	applyButtonOccupancy("chr-hero", 5, 5, all, isButtonType)
	applyButtonOccupancy("chr-hero", 5.1, 4.9, all, isButtonType) // rounds to the same cell

	got := characterIDsOf(btn)
	if len(got) != 1 {
		t.Fatalf("expected exactly one entry (no duplicate) for chr-hero, got %v", got)
	}
}

// This is the reported behaviour: stepping onto a going want must actually
// start it, not just record that someone is standing there. Before this, a
// going want's own going/stopped toggle only ever moved via its card's
// webhook, so the card kept reading STOPPED — and the drive engine kept
// reading no vote at all for it — until someone opened the sidebar and
// flipped it by hand. Now it's the *stepping character's own*
// "character_motion" want that gets set — see motionWantFor.
func TestApplyButtonOccupancyStepOntoGoingWantStartsIt(t *testing.T) {
	resetButtonOccupancy()
	going := goingWantAt("going-1", 5, 5)
	motion := motionWantFor("chr-hero", false)
	all := []*mywant.Want{going, motion}

	applyButtonOccupancy("chr-hero", 5, 5, all, isGoingButtonType)

	if !goingOf(motion) {
		t.Fatalf("expected stepping onto the want to set chr-hero's own going=true, got %v", goingOf(motion))
	}
}

// Leaving must not undo it — going, once set, stays set regardless of where
// the character wanders next; it lives on their own want, not on the tile.
func TestApplyButtonOccupancyStepOffGoingWantDoesNotStopIt(t *testing.T) {
	resetButtonOccupancy()
	going := goingWantAt("going-1", 5, 5)
	motion := motionWantFor("chr-hero", false)
	all := []*mywant.Want{going, motion}

	applyButtonOccupancy("chr-hero", 5, 5, all, isGoingButtonType) // step on: starts it
	applyButtonOccupancy("chr-hero", 9, 9, all, isGoingButtonType) // step off: nothing there

	if !goingOf(motion) {
		t.Fatalf("expected going to remain true after stepping off, got %v", goingOf(motion))
	}
}

// This is the second half of the reported behaviour: a going want is a
// toggle, not a one-way switch — stepping onto one that's already going
// stops it, the same "press it again to turn it off" a real pressure plate
// reads as.
func TestApplyButtonOccupancyStepOntoAlreadyGoingWantStopsIt(t *testing.T) {
	resetButtonOccupancy()
	going := goingWantAt("going-1", 5, 5)
	motion := motionWantFor("chr-hero", true)
	all := []*mywant.Want{going, motion}

	applyButtonOccupancy("chr-hero", 5, 5, all, isGoingButtonType)

	if goingOf(motion) {
		t.Fatalf("expected a second footstep to toggle going to false, got %v", goingOf(motion))
	}
}

// This is the reported bug, now fixed at its root rather than patched
// around: going lives on each character's own "character_motion" want, so
// one character's footstep on a shared going want can only ever flip their
// own flag. A second, unrelated character's flag — even one who is
// currently, physically also standing on the very same tile — must be
// completely untouched.
func TestApplyButtonOccupancyStepOntoGoingWantOnlyAffectsTheStepper(t *testing.T) {
	resetButtonOccupancy()
	going := goingWantAt("going-1", 5, 5)
	stepper := motionWantFor("chr-new", false)
	bystander := motionWantFor("chr-bystander", true) // already going, from something else entirely
	all := []*mywant.Want{going, stepper, bystander}

	applyButtonOccupancy("chr-new", 5, 5, all, isGoingButtonType)

	if !goingOf(stepper) {
		t.Fatalf("expected chr-new's own going to be set true, got %v", goingOf(stepper))
	}
	if !goingOf(bystander) {
		t.Fatalf("expected chr-bystander's going to be completely untouched, got %v", goingOf(bystander))
	}
}

// Two separate footsteps toggle twice: on, then off. A single continuous
// step (same cell reported again) is filtered out earlier, by the
// newWantID == prevWantID guard, and never reaches the toggle at all — this
// exercises the pair as two genuinely distinct step-on events, e.g. stepping
// off and back on, or off onto a neighbouring button and back.
func TestApplyButtonOccupancyTwoFootstepsToggleOnThenOff(t *testing.T) {
	resetButtonOccupancy()
	going := goingWantAt("going-1", 5, 5)
	motion := motionWantFor("chr-hero", false)
	all := []*mywant.Want{going, motion}

	applyButtonOccupancy("chr-hero", 5, 5, all, isGoingButtonType) // step on: false -> true
	applyButtonOccupancy("chr-hero", 9, 9, all, isGoingButtonType) // step off elsewhere
	applyButtonOccupancy("chr-hero", 5, 5, all, isGoingButtonType) // step on again: true -> false

	if goingOf(motion) {
		t.Fatalf("expected the second distinct footstep to toggle going back to false, got %v", goingOf(motion))
	}
}

// Stepping onto a *different* form-type:button want (direction, gear — or
// anything else that isn't itself a going want) must not touch anyone's
// going flag at all.
func TestApplyButtonOccupancyStepOntoNonGoingButtonDoesNotStartGoing(t *testing.T) {
	resetButtonOccupancy()
	dir := buttonWantAt("dir-1", 5, 5) // Type: "direction", not "going"
	motion := motionWantFor("chr-hero", false)
	all := []*mywant.Want{dir, motion}

	applyButtonOccupancy("chr-hero", 5, 5, all, isButtonType)

	if goingOf(motion) {
		t.Fatalf("a direction want should never start anyone's going")
	}
}
