package server

import (
	"strconv"
	"testing"

	mywant "mywant/engine/core"
)

// The three things the framework can do that the button-only version could
// not: see past the first want on a cell, react to a type that carries no
// form-type label, and treat a thing as a mover in its own right.

func wantAt(id, typeName string, x, y int) *mywant.Want {
	w := &mywant.Want{Metadata: mywant.Metadata{ID: id, Type: typeName}}
	// What the type definition's `state:` list supplies in a real deployment.
	// SetCurrent silently no-ops on an undeclared key, so without this a test
	// reads as "the rule never fired" when the rule fired and was ignored.
	w.StateLabels = map[string]mywant.StateLabel{
		targetsCharacters: mywant.LabelCurrent,
		targetsThings:     mywant.LabelCurrent,
	}
	w.SetLabel(canvasLabelX, strconv.Itoa(x))
	w.SetLabel(canvasLabelY, strconv.Itoa(y))
	return w
}

// A cell can hold more than one want, and the old lookup stopped at the first.
// Here the button is second, behind a note that no rule cares about.
func TestIntersectionSeesEveryWantOnTheCell(t *testing.T) {
	resetButtonOccupancy()
	note := wantAt("note-1", "note", 3, 3)
	btn := wantAt("btn-1", "direction", 3, 3)
	all := []*mywant.Want{note, btn}

	applyButtonOccupancy("chr-hero", 3, 3, all, isButtonType)

	if got := characterIDsOf(btn); len(got) != 1 || got[0] != "chr-hero" {
		t.Fatalf("button behind another want was never reached: %v", got)
	}
}

// Leaving a cell has to take the mover off every want it was standing on, not
// just whichever one happened to be found first.
func TestIntersectionLeavesEveryWantOnTheCell(t *testing.T) {
	resetButtonOccupancy()
	a := wantAt("btn-a", "direction", 4, 4)
	b := wantAt("btn-b", "direction", 4, 4)
	all := []*mywant.Want{a, b}

	applyButtonOccupancy("chr-hero", 4, 4, all, isButtonType)
	applyButtonOccupancy("chr-hero", 9, 9, all, isButtonType)

	if got := characterIDsOf(a); len(got) != 0 {
		t.Fatalf("still on the first want after walking away: %v", got)
	}
	if got := characterIDsOf(b); len(got) != 0 {
		t.Fatalf("still on the second want after walking away: %v", got)
	}
}

// A rule decides for itself which wants it is about, so a type with no
// form-type label is reachable. trash is the case that matters: it is not a
// button and never will be, and the old filter dropped it before any rule saw
// it. Nothing to assert on the want — the effect is on the mover, and it needs
// a server — so this asserts the rule was consulted and matched.
func TestTrashRuleAppliesToAThingWithoutTheButtonLabel(t *testing.T) {
	bin := wantAt("bin-1", "trash", 0, 0)
	ctx := intersectionContext{s: &Server{}, isButton: isButtonType}

	if !trashRule.applies(ctx, bin, mover{moverThing, "thg-1"}) {
		t.Fatal("a thing arriving on a bin did not match the trash rule")
	}
	if trashRule.applies(ctx, bin, mover{moverCharacter, "chr-hero"}) {
		t.Fatal("a character standing on a bin must not be archived")
	}
	if occupancyRule.applies(ctx, bin, mover{moverThing, "thg-1"}) {
		t.Fatal("trash carries no form-type label and must not be occupancy-tracked")
	}
}

// A thing is a mover on the same footing as a character: same transition, same
// rules, only the array it lands in differs.
func TestIntersectionTracksAThingInItsOwnArray(t *testing.T) {
	resetButtonOccupancy()
	btn := wantAt("btn-1", "direction", 6, 6)
	all := []*mywant.Want{btn}

	applyIntersections(nil, mover{moverThing, "thg-1"}, 6, 6, all, isButtonType)

	if got := targetIDsOf(btn, targetsThings); len(got) != 1 || got[0] != "thg-1" {
		t.Fatalf("expected thg-1 in things, got %v", got)
	}
	if got := characterIDsOf(btn); len(got) != 0 {
		t.Fatalf("a thing must not land in the characters array: %v", got)
	}
}

// A character and a thing on the same plate are two independent records, so one
// walking off must not take the other with it.
func TestIntersectionKeepsMoverKindsApart(t *testing.T) {
	resetButtonOccupancy()
	btn := wantAt("btn-1", "direction", 7, 7)
	all := []*mywant.Want{btn}

	applyIntersections(nil, mover{moverCharacter, "chr-hero"}, 7, 7, all, isButtonType)
	applyIntersections(nil, mover{moverThing, "thg-1"}, 7, 7, all, isButtonType)
	applyIntersections(nil, mover{moverThing, "thg-1"}, 9, 9, all, isButtonType)

	if got := characterIDsOf(btn); len(got) != 1 || got[0] != "chr-hero" {
		t.Fatalf("the thing leaving took the character off too: %v", got)
	}
	if got := targetIDsOf(btn, targetsThings); len(got) != 0 {
		t.Fatalf("the thing is still listed after leaving: %v", got)
	}
}
