package server

import (
	"testing"

	mywant "mywant/engine/core"
)

func TestResolveMotionRequiresGoing(t *testing.T) {
	dx, dy, moved := resolveMotion(false, 0, true, 1, 2)
	if moved || dx != 0 || dy != 0 {
		t.Fatalf("expected a stopped character to produce no motion, got (%v, %v, %v)", dx, dy, moved)
	}
}

// The actual bug this guards: stepping onto a bare "going" want with no
// direction want anywhere in the mix used to move the character anyway,
// heading east — not because anything said "go east", but because an unset
// heading float64 defaults to 0 and 0 degrees happens to mean east. From the
// player's side that read as the plate itself shoving them off it the
// instant they stepped on, no input involved.
func TestResolveMotionGoingWithNoHeadingEverStandsStill(t *testing.T) {
	dx, dy, moved := resolveMotion(true, 0, false, 1, 2)
	if moved || dx != 0 || dy != 0 {
		t.Fatalf("expected a going character with no heading ever set to stand still, got (%v, %v, %v)", dx, dy, moved)
	}
}

func TestResolveMotionUsesCharacterSpeedAndGear(t *testing.T) {
	dx, dy, moved := resolveMotion(true, 0, true, 3, 5) // heading 0 = east
	if !moved {
		t.Fatalf("expected motion")
	}
	dist := dx*dx + dy*dy // heading 0 -> dy=0, so this is just dx^2
	want := 15.0 * 15.0   // gearMultiplier(3) * speed(5) = 15 cells east
	if dist < want-0.01 || dist > want+0.01 {
		t.Fatalf("expected distance from gear*speed (15), got squared distance %v", dist)
	}
}

func TestResolveMotionIsNotScaledByTickDuration(t *testing.T) {
	// Deliberately no tickSeconds parameter at all — distance is always
	// speed*gearMultiplier whole cells per call, regardless of how often the
	// caller happens to invoke this. See resolveMotion's own doc for why: a
	// cells-per-second version was tried and rejected for producing sub-cell
	// fractional movement at fast tick rates.
	dx, _, moved := resolveMotion(true, 0, true, 1, 4)
	if !moved || dx != 4 {
		t.Fatalf("expected exactly 4 whole cells east (speed*gear, no scaling), got dx=%v moved=%v", dx, moved)
	}
}

// The actual bug: a direction/gear want's dx/dy/value survives a restart by
// loading back from state.yaml (see want_restart_test.go), but YAML's own
// decoder hands an integer-looking scalar back as a Go int, not a float64 —
// unlike a value SetCurrent wrote fresh this tick, which really is a
// float64. A raw `.(float64)` type assertion on GetCurrent's result silently
// fails for exactly that reloaded-from-disk case, producing a zero vector
// even though hasDirection is (correctly) still true — which is what made a
// character already moving along one heading keep moving along it forever,
// since driveOneCharacterTick's `dirVectorX != 0 || dirVectorY != 0` guard
// falls straight through to the previously persisted heading on a silent
// zero.
func TestDirectionVectorOfCoercesIntPersistedFromYAML(t *testing.T) {
	w := &mywant.Want{Metadata: mywant.Metadata{ID: "dir-1", Type: "direction"}}
	w.StateLabels = map[string]mywant.StateLabel{"dx": mywant.LabelCurrent, "dy": mywant.LabelCurrent}
	w.SetCurrent("dx", int(-1))
	w.SetCurrent("dy", int(0))

	dx, dy := directionVectorOf(w)

	if dx != -1 || dy != 0 {
		t.Fatalf("expected (-1, 0) coerced from int, got (%v, %v)", dx, dy)
	}
}

func TestGearValueOfCoercesIntPersistedFromYAML(t *testing.T) {
	w := &mywant.Want{Metadata: mywant.Metadata{ID: "gear-1", Type: "gear"}}
	w.StateLabels = map[string]mywant.StateLabel{"value": mywant.LabelCurrent}
	w.SetCurrent("value", int(2))

	got := gearValueOf(w)

	if got != 2 {
		t.Fatalf("expected 2 coerced from int, got %v", got)
	}
}
