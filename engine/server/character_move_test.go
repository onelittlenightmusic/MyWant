package server

import (
	"strconv"
	"testing"

	mywant "mywant/engine/core"
)

// wallAt builds a wall want occupying `length+1` cells from (x, y).
func wallAt(id string, x, y, rotation, length int) *mywant.Want {
	w := &mywant.Want{Metadata: mywant.Metadata{ID: id, Type: "wall"}}
	w.SetLabel(canvasLabelX, strconv.Itoa(x))
	w.SetLabel(canvasLabelY, strconv.Itoa(y))
	w.SetLabel(canvasLabelRotation, strconv.Itoa(rotation))
	w.SetLabel(canvasLabelLength, strconv.Itoa(length))
	return w
}

// doorAt builds a door want, locked or not. A door declares `locked` in its
// type definition's state list; SetCurrent no-ops on an undeclared key, so
// the label has to be seeded here the same way real deployment does it.
func doorAt(id string, x, y int, locked bool) *mywant.Want {
	w := &mywant.Want{Metadata: mywant.Metadata{ID: id, Type: "door"}}
	w.StateLabels = map[string]mywant.StateLabel{"locked": mywant.LabelCurrent}
	w.SetLabel(canvasLabelX, strconv.Itoa(x))
	w.SetLabel(canvasLabelY, strconv.Itoa(y))
	w.SetCurrent("locked", locked)
	return w
}

// An empty board constrains nothing — the check must be invisible when there
// is nothing to bump into, or every ordinary step pays for a feature it isn't
// using.
func TestResolveMoveWithNoWallsAppliesUnchanged(t *testing.T) {
	blocked := blockedCells(nil)
	x, y, stopped := resolveMove(blocked, 0, 0, 3, 4, true)
	if x != 3 || y != 4 || stopped {
		t.Fatalf("expected (3,4) unblocked, got (%v,%v) stopped=%v", x, y, stopped)
	}
}

// This is the actual bug this file was written for: a "going" want moves a
// character by its own drive tick, which never goes near the browser, so the
// browser's wall check — the only one that existed — had no say in it and the
// character walked straight through.
func TestResolveMoveWalkingIsStoppedByAWall(t *testing.T) {
	blocked := blockedCells([]*mywant.Want{wallAt("w", 2, 0, 0, 0)})
	x, _, stopped := resolveMove(blocked, 0, 0, 4, 0, true)
	if !stopped {
		t.Fatal("expected walking into a wall to be stopped")
	}
	if x >= 2 {
		t.Fatalf("expected to stop short of the wall at x=2, ended at x=%v", x)
	}
}

// Walking into a wall should leave you against it, not back where you
// started — otherwise a character held against a wall visibly snaps backwards
// every tick instead of standing there.
func TestResolveMoveWalkingStopsAgainstTheWallNotAtTheStart(t *testing.T) {
	blocked := blockedCells([]*mywant.Want{wallAt("w", 5, 0, 0, 0)})
	x, _, stopped := resolveMove(blocked, 0, 0, 6, 0, true)
	if !stopped {
		t.Fatal("expected to be stopped")
	}
	if x <= 0 {
		t.Fatalf("expected to advance up to the wall, ended at x=%v (never moved)", x)
	}
}

// A gear-multiplied push covers several cells in one tick. Checking only the
// destination would let it jump clean over a one-cell wall without ever
// landing on it — through the wall, in one frame, with both endpoints legal.
func TestResolveMoveWalkingCannotTunnelThroughAWall(t *testing.T) {
	blocked := blockedCells([]*mywant.Want{wallAt("w", 2, 0, 0, 0)})
	x, _, stopped := resolveMove(blocked, 0, 0, 4, 0, true)
	if !stopped || x >= 2 {
		t.Fatalf("expected the wall at x=2 to stop a 4-cell stride, got x=%v stopped=%v", x, stopped)
	}
}

// A warp does not travel through the cells it crosses, so a wall it never
// touches has no business blocking it.
func TestResolveMoveWarpMayCrossAWall(t *testing.T) {
	blocked := blockedCells([]*mywant.Want{wallAt("w", 2, 0, 0, 0)})
	x, y, stopped := resolveMove(blocked, 0, 0, 4, 0, false)
	if stopped || x != 4 || y != 0 {
		t.Fatalf("expected a warp over the wall to land at (4,0), got (%v,%v) stopped=%v", x, y, stopped)
	}
}

// ...but nothing may come to rest inside a wall, by any route at all.
func TestResolveMoveWarpIntoAWallIsRefused(t *testing.T) {
	blocked := blockedCells([]*mywant.Want{wallAt("w", 2, 0, 0, 0)})
	x, y, stopped := resolveMove(blocked, 0, 0, 2, 0, false)
	if !stopped || x != 0 || y != 0 {
		t.Fatalf("expected warping into a wall to be refused, got (%v,%v) stopped=%v", x, y, stopped)
	}
}

// A wall stretched across several cells blocks all of them. Reading only its
// anchor left the rest of its own span walk-through.
func TestResolveMoveWallBlocksItsWholeFootprint(t *testing.T) {
	// Anchored at (2,0), rotation 90 (down), length 2 → covers (2,0),(2,1),(2,2).
	blocked := blockedCells([]*mywant.Want{wallAt("w", 2, 0, 90, 2)})
	for _, cell := range [][2]int{{2, 0}, {2, 1}, {2, 2}} {
		if !blocked[cell] {
			t.Fatalf("expected cell %v to be blocked by the wall's span", cell)
		}
	}
	if blocked[[2]int{2, 3}] {
		t.Fatal("cell past the end of the span must not be blocked")
	}
}

// A door is only a wall while it is locked — an unlocked one is a doorway,
// which is the entire point of it being a door and not a wall.
func TestResolveMoveLockedDoorBlocksUnlockedDoesNot(t *testing.T) {
	locked := blockedCells([]*mywant.Want{doorAt("d", 1, 0, true)})
	if _, _, stopped := resolveMove(locked, 0, 0, 1, 0, true); !stopped {
		t.Fatal("expected a locked door to block")
	}

	unlocked := blockedCells([]*mywant.Want{doorAt("d", 1, 0, false)})
	if x, _, stopped := resolveMove(unlocked, 0, 0, 1, 0, true); stopped || x != 1 {
		t.Fatalf("expected an unlocked door to be walkable, got x=%v stopped=%v", x, stopped)
	}
}

// `stopped` is what the collision sound hangs on, so it has to be true
// exactly when something solid was hit and false otherwise — a move that
// simply had nowhere to go must not knock.
func TestResolveMoveReportsStoppedOnlyOnAnActualHit(t *testing.T) {
	blocked := blockedCells([]*mywant.Want{wallAt("w", 2, 0, 0, 0)})

	if _, _, stopped := resolveMove(blocked, 0, 0, 1, 0, true); stopped {
		t.Fatal("a clear step must not report a collision")
	}
	if _, _, stopped := resolveMove(blocked, 0, 0, 0, 0, true); stopped {
		t.Fatal("standing still is not a collision")
	}
	if _, _, stopped := resolveMove(blocked, 0, 0, 3, 0, true); !stopped {
		t.Fatal("walking into the wall must report a collision")
	}
	// Held against it: still a collision every tick, which is what lets the
	// client turn a stay into footsteps rather than one knock and silence.
	if _, _, stopped := resolveMove(blocked, 1, 0, 2, 0, true); !stopped {
		t.Fatal("pressing on against the wall must keep reporting a collision")
	}
}

// An ordinary want is scenery, not an obstacle: you walk onto a button to
// press it, onto a note to read it. Only walls and locked doors block.
func TestResolveMoveOrdinaryWantsDoNotBlock(t *testing.T) {
	blocked := blockedCells([]*mywant.Want{buttonWantAt("b", 1, 0)})
	if x, _, stopped := resolveMove(blocked, 0, 0, 1, 0, true); stopped || x != 1 {
		t.Fatalf("expected an ordinary want to be walkable, got x=%v stopped=%v", x, stopped)
	}
}
