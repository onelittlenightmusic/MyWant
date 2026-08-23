package server

import (
	"math"

	mywant "mywant/engine/core"
)

// ── The one path every character move takes ───────────────────────────────────
//
// A character's position changes for exactly two reasons today — a cursor PUT
// (somebody pressed a key, or dragged, or warped) and a drive tick (a "going"
// want moving them under their own steam) — and until this file existed those
// two shared nothing. Collision lived entirely in the browser: WantCanvas kept
// a set of wall cells and simply declined to send a PUT that would step into
// one. That worked for the path that goes through the browser and did nothing
// at all for the one that doesn't, so a character told to keep going walked
// straight through walls.
//
// The fix is not "add the same check to the drive engine too" — that is how the
// two paths drifted apart in the first place, and the third writer added later
// would drift again. Both callers now resolve their move through
// resolveMove, and it is the only place that decides where a character
// actually ends up. A new mover gets the check by virtue of having to ask.
//
// The browser keeps its own check, and should: it needs an answer in the same
// frame the key is pressed, without a round trip. It is now an optimisation —
// the thing that makes walking feel immediate — rather than the only thing
// standing between a character and a wall.

// doorTypes are the want types that are a doorway rather than a plain tile.
// Mirrors DOOR_TYPES in web/src/components/dashboard/floorPlan.ts.
var doorTypes = map[string]bool{"door": true, "rpg_door": true}

// bumpEffectType is raised on a character's own cursor whenever a move of
// theirs was stopped by something solid.
//
// Running into a wall has always made a noise; it just used to be made
// entirely in the browser, by the same code that refused the step (see
// playCollisionSoundRef in WantCanvas). A move the server stops — a "going"
// want walking someone into a wall — has no such moment in the browser to
// hang a sound on, so the server has to say it happened. It rides the
// existing per-cursor effects list rather than a field of its own because
// that list already solves the hard part: a burst of them survives the
// client's own snapshot coalescing, and each is replayed once by nonce.
//
// Raised every tick the character is held against the wall, deliberately.
// The client turns a repeat in the same place into a footstep rather than a
// second knock (you are not meeting the wall again, you are walking on the
// spot against it) — so "per event" here means per blocked tick, and what
// that sounds like stays the browser's call, exactly as before.
const bumpEffectType = "bump"

// blocksMovement reports whether a want makes the cells it covers impassable.
//
// A wall always does. A door does only while it is locked — an unlocked door
// is a doorway, which is the whole point of it — which is why this reads the
// want's live `locked` state rather than deciding from the type alone.
func blocksMovement(w *mywant.Want) bool {
	if w.Metadata.Type == "wall" {
		return true
	}
	if !doorTypes[w.Metadata.Type] {
		return false
	}
	locked, ok := w.GetCurrent("locked")
	if !ok {
		return false
	}
	b, isBool := locked.(bool)
	return isBool && b
}

// blockedCells is the set of grid cells nothing may stand on.
//
// Whole footprints, not anchors: a wall stretched across four cells blocks
// four cells, and checking only its anchor left three of them walk-through.
// markWantOccupied already reads the canvas labels and expands the span, so
// this is the same geometry the layout itself uses rather than a second copy
// of it.
func blockedCells(allWants []*mywant.Want) map[[2]int]bool {
	cells := map[[2]int]bool{}
	for _, w := range allWants {
		if blocksMovement(w) {
			markWantOccupied(w, cells)
		}
	}
	return cells
}

// blockedCellSnapshot builds the blocked set from the live want list.
//
// Taken by callers BEFORE they lock cursorsMu: it walks every want and reads
// their labels, which is not work to do while holding the cursor lock, and
// resolving the move inside that lock afterwards is what keeps the check and
// the write atomic with respect to each other.
func (s *Server) blockedCellSnapshot() map[[2]int]bool {
	if s.globalBuilder == nil {
		return nil
	}
	return blockedCells(s.globalBuilder.GetWants())
}

// gridCellOf rounds a continuous position to the grid cell it stands on.
func gridCellOf(x, y float64) [2]int {
	return [2]int{int(math.Round(x)), int(math.Round(y))}
}

// resolveMove answers where a character actually ends up, given where they
// are and where something wants to put them.
//
// `continuous` is the difference between walking and arriving. A character
// crossing the board on their own legs passes through every cell on the way
// and cannot pass through a wall, so the whole segment is checked — otherwise
// a fast enough step (gear multiplied, several cells per tick) jumps clean
// over a one-cell wall without ever landing on it. A warp is not travel: it
// does not visit the cells in between, so only where it lands is anyone's
// business. Checking the path for those would make a wall block a teleport
// that never touched it.
//
// Either way the destination itself is always checked, so nothing can come to
// rest inside a wall by any route.
func resolveMove(blocked map[[2]int]bool, fromX, fromY, toX, toY float64, continuous bool) (x, y float64, stopped bool) {
	if len(blocked) == 0 {
		return toX, toY, false
	}

	if !continuous {
		if blocked[gridCellOf(toX, toY)] {
			return fromX, fromY, true
		}
		return toX, toY, false
	}

	dx, dy := toX-fromX, toY-fromY
	dist := math.Hypot(dx, dy)
	if dist == 0 {
		return toX, toY, false
	}

	// Sampled at half a cell so no cell on the way can be stepped over, and
	// the last sample that stood somewhere legal is where they stop — walking
	// into a wall leaves you against it, not back where you started.
	steps := int(math.Ceil(dist / 0.5))
	lastX, lastY := fromX, fromY
	for i := 1; i <= steps; i++ {
		t := float64(i) / float64(steps)
		px, py := fromX+dx*t, fromY+dy*t
		if blocked[gridCellOf(px, py)] {
			return lastX, lastY, true
		}
		lastX, lastY = px, py
	}
	return toX, toY, false
}
