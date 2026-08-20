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
