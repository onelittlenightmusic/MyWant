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

// A footstep on a going want queues the instruction that flips the stepping
// character's own flag.
//
// The write itself is not here any more, and deliberately: a footstep and the
// card's toggle are one instruction with two doors, and the interpreting —
// which action, applied to whom — belongs to the going want (see
// engine/types/going_types.go). What this package is responsible for is
// noticing the footstep and saying whose it was, which is what these assert.
func queuedGoingActions(t *testing.T, w *mywant.Want) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, entry := range w.DrainState("webhook_queue") {
		m, ok := entry.(map[string]any)
		if !ok {
			t.Fatalf("queue entry is not a map: %T", entry)
		}
		pm, ok := m["payload"].(map[string]any)
		if !ok {
			t.Fatalf("queue entry has no payload map: %v", m)
		}
		out = append(out, pm)
	}
	return out
}

func assertOneToggleFor(t *testing.T, w *mywant.Want, characterID string) {
	t.Helper()
	got := queuedGoingActions(t, w)
	if len(got) != 1 {
		t.Fatalf("expected exactly one queued instruction, got %d: %v", len(got), got)
	}
	if got[0]["action"] != "toggle" || got[0]["character_id"] != characterID {
		t.Fatalf("expected {toggle, %s}, got %v", characterID, got[0])
	}
}

func TestApplyButtonOccupancyStepOntoGoingWantQueuesAToggle(t *testing.T) {
	resetButtonOccupancy()
	going := goingWantAt("going-1", 5, 5)
	all := []*mywant.Want{going}

	applyButtonOccupancy("chr-hero", 5, 5, all, isGoingButtonType)

	assertOneToggleFor(t, going, "chr-hero")
}

// Leaving asks for nothing. Going, once set, stays set regardless of where the
// character wanders next — it lives on their own want, not on the tile — so
// stepping off must not queue a second instruction that would undo it.
func TestApplyButtonOccupancyStepOffGoingWantQueuesNothing(t *testing.T) {
	resetButtonOccupancy()
	going := goingWantAt("going-1", 5, 5)
	all := []*mywant.Want{going}

	applyButtonOccupancy("chr-hero", 5, 5, all, isGoingButtonType) // step on
	applyButtonOccupancy("chr-hero", 9, 9, all, isGoingButtonType) // step off: nothing there

	assertOneToggleFor(t, going, "chr-hero")
}

// A going want is a toggle, not a one-way switch — which is why a footstep
// queues "toggle" rather than "going". Two distinct footsteps are two
// instructions; what they resolve to is the going want's business.
func TestApplyButtonOccupancyTwoFootstepsQueueTwoToggles(t *testing.T) {
	resetButtonOccupancy()
	going := goingWantAt("going-1", 5, 5)
	all := []*mywant.Want{going}

	applyButtonOccupancy("chr-hero", 5, 5, all, isGoingButtonType) // on
	applyButtonOccupancy("chr-hero", 9, 9, all, isGoingButtonType) // off elsewhere
	applyButtonOccupancy("chr-hero", 5, 5, all, isGoingButtonType) // on again

	got := queuedGoingActions(t, going)
	if len(got) != 2 {
		t.Fatalf("expected two queued instructions, got %d: %v", len(got), got)
	}
	for i, pm := range got {
		if pm["action"] != "toggle" || pm["character_id"] != "chr-hero" {
			t.Fatalf("instruction %d: expected {toggle, chr-hero}, got %v", i, pm)
		}
	}
}

// One character's footstep can only ever ask about their own flag. A second
// character standing on the very same tile must not appear in the instruction.
func TestApplyButtonOccupancyStepOntoGoingWantNamesOnlyTheStepper(t *testing.T) {
	resetButtonOccupancy()
	going := goingWantAt("going-1", 5, 5)
	all := []*mywant.Want{going}

	applyButtonOccupancy("chr-new", 5, 5, all, isGoingButtonType)

	assertOneToggleFor(t, going, "chr-new")
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

	if got := queuedGoingActions(t, dir); len(got) != 0 {
		t.Fatalf("a direction want should never ask about anyone's going, got %v", got)
	}
	_ = motion
}
