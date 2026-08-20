package server

import (
	"testing"

	mywant "mywant/engine/core"
)

func goingWant(id string, going bool) *mywant.Want {
	w := &mywant.Want{Metadata: mywant.Metadata{ID: id, Type: "going"}}
	w.StateLabels = map[string]mywant.StateLabel{
		"characters": mywant.LabelCurrent,
		"going":      mywant.LabelCurrent,
	}
	w.SetCurrent("going", going)
	return w
}

func TestCarryForwardGoingTargetsReadsOwnerLiveState(t *testing.T) {
	owner := map[string]string{"chr-1": "want-a"}
	goingWants := map[string]*mywant.Want{"want-a": goingWant("want-a", true)}
	targets := map[string]*driveTarget{}

	carryForwardGoingTargets(targets, owner, goingWants)

	got, ok := targets["chr-1"]
	if !ok {
		t.Fatalf("expected a carried-forward target for chr-1")
	}
	if !resolveGoing(got.goingVotes) {
		t.Fatalf("expected the carried-forward vote to reflect the owner's current going=true")
	}
}

// This is the second half of the reported behaviour: toggling the SAME
// going want's own card — not by anyone standing on it — must keep
// controlling a character it once claimed, even after that character has
// walked away.
func TestCarryForwardGoingTargetsReflectsOwnerToggledToStopped(t *testing.T) {
	owner := map[string]string{"chr-1": "want-a"}
	goingWants := map[string]*mywant.Want{"want-a": goingWant("want-a", false)}
	targets := map[string]*driveTarget{}

	carryForwardGoingTargets(targets, owner, goingWants)

	if resolveGoing(targets["chr-1"].goingVotes) {
		t.Fatalf("expected the carried-forward vote to reflect the owner's current going=false")
	}
}

func TestCarryForwardGoingTargetsSkipsCharacterAlreadyVotingThisTick(t *testing.T) {
	owner := map[string]string{"chr-1": "want-a"}
	// want-a says stopped, but chr-1 is standing on a *different* going want
	// this tick (want-b, already in targets) which says going. The stale
	// owner must not override the live vote.
	goingWants := map[string]*mywant.Want{"want-a": goingWant("want-a", false)}
	targets := map[string]*driveTarget{
		"chr-1": {gearMultiplier: 1, goingVotes: []bool{true}},
	}

	carryForwardGoingTargets(targets, owner, goingWants)

	if !resolveGoing(targets["chr-1"].goingVotes) {
		t.Fatalf("expected the live vote from the want currently stood on to win")
	}
	if len(targets["chr-1"].goingVotes) != 1 {
		t.Fatalf("expected no extra vote appended, got %v", targets["chr-1"].goingVotes)
	}
}

func TestCarryForwardGoingTargetsForgetsOwnerWhoseWantIsGone(t *testing.T) {
	owner := map[string]string{"chr-1": "want-deleted"}
	targets := map[string]*driveTarget{}

	carryForwardGoingTargets(targets, owner, map[string]*mywant.Want{})

	if _, ok := targets["chr-1"]; ok {
		t.Fatalf("expected no target for a character whose owning want no longer exists")
	}
	if _, ok := owner["chr-1"]; ok {
		t.Fatalf("expected the dangling owner entry to be forgotten")
	}
}

// This is the reported regression: standing on a direction want (or gear —
// anything that isn't itself a going want) still creates a targets entry,
// for its own dx/dy vote, with no goingVotes of its own. Treating "has a
// targets entry at all" as "already voting on going" skipped the carried-
// forward vote and left that entry's goingVotes empty for the tick — which
// resolveGoing then reads as stopped. A character that was going must keep
// going while it's merely steering, not stop the moment it touches the
// wheel.
func TestCarryForwardGoingTargetsDoesNotStopACharacterOnlyVotingDirection(t *testing.T) {
	owner := map[string]string{"chr-1": "want-a"}
	goingWants := map[string]*mywant.Want{"want-a": goingWant("want-a", true)}
	// What the direction branch of the want-collection loop in
	// driveEngineTickOnce leaves behind: a target with a direction vote and
	// no going vote at all.
	targets := map[string]*driveTarget{
		"chr-1": {gearMultiplier: 1, dirVectorX: 1, dirVectorY: 0, hasDirection: true},
	}

	carryForwardGoingTargets(targets, owner, goingWants)

	if !resolveGoing(targets["chr-1"].goingVotes) {
		t.Fatalf("expected the owner's going=true to be carried forward despite the existing direction vote")
	}
	if !targets["chr-1"].hasDirection || targets["chr-1"].dirVectorX != 1 {
		t.Fatalf("expected the existing direction vote to survive, got %+v", targets["chr-1"])
	}
}

// This is the first half of the reported behaviour: a character that steps
// onto a going want's tile and starts moving keeps moving at the same speed
// after it walks off the tile, as long as nothing has since told it to stop.
func TestResolveDriveTickKeepsMovingAfterLeavingTheGoingTile(t *testing.T) {
	headings := map[string]float64{}
	owner := map[string]string{}
	goingWants := map[string]*mywant.Want{"want-a": goingWant("want-a", true)}

	// Tick 1: standing on going want "want-a" (going=true), facing east.
	targets := map[string]*driveTarget{
		"chr-1": {
			gearMultiplier: 1,
			goingVotes:     []bool{true},
			dirVectorX:     1,
			dirVectorY:     0,
			hasDirection:   true,
		},
	}
	owner["chr-1"] = "want-a" // set by the want loop in driveEngineTickOnce
	moves := resolveDriveTick(targets, headings, 1, func(string) float64 { return 2 }, 1)
	first, ok := moves["chr-1"]
	if !ok || first.dx <= 0 {
		t.Fatalf("expected eastward motion on the tick it steps onto the going tile, got %v ok=%v", first, ok)
	}

	// Tick 2: walked off every drive-category button — no target at all this
	// tick, exactly as the want-collection loop would leave it. Go through
	// carryForwardGoingTargets, as the real tick does, since that's what's
	// supposed to keep chr-1 in play.
	targets2 := map[string]*driveTarget{}
	carryForwardGoingTargets(targets2, owner, goingWants)
	moves2 := resolveDriveTick(targets2, headings, 1, func(string) float64 { return 2 }, 1)

	second, ok := moves2["chr-1"]
	if !ok {
		t.Fatalf("expected chr-1 to keep moving after walking off the going tile")
	}
	if second.dx != first.dx || second.dy != first.dy {
		t.Fatalf("expected the same motion as before leaving the tile, got %v want %v", second, first)
	}
}

func TestResolveDriveTickUsesCharacterSpeedOverBase(t *testing.T) {
	headings := map[string]float64{}
	targets := map[string]*driveTarget{
		"chr-1": {gearMultiplier: 1, goingVotes: []bool{true}},
	}

	moves := resolveDriveTick(targets, headings, 1, func(string) float64 { return 5 }, 1)

	m := moves["chr-1"]
	dist := m.dx*m.dx + m.dy*m.dy // heading 0 → dy=0, so this is just dx^2
	if dist < 24 || dist > 26 {   // expect distance ≈ 5 (speed) * 1 (gear) * 1s → dx=5, dist=25
		t.Fatalf("expected the character's own speed (5) to be used, got squared distance %v", dist)
	}
}

func TestResolveDriveTickStoppedProducesNoMotion(t *testing.T) {
	headings := map[string]float64{}
	targets := map[string]*driveTarget{
		"chr-1": {gearMultiplier: 1, goingVotes: []bool{false}},
	}

	moves := resolveDriveTick(targets, headings, 1, func(string) float64 { return 2 }, 1)

	if _, moved := moves["chr-1"]; moved {
		t.Fatalf("expected no motion for a stopped character")
	}
}
