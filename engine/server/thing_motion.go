package server

import (
	"sync"
	"time"

	mywant "mywant/engine/core"
)

// Things that move.
//
// A character is ticked by its own "character_motion" want, through the normal
// reconcile loop. A thing has no want of its own to hang that on, so the
// server runs one ticker for all of them — which is also the cheaper shape:
// the blocked-cell snapshot is built once per tick and shared by every thing
// that moves, where a per-thing want would rebuild it per thing.
//
// Paced to the same interval as a character's, because it is the same board:
// two objects given the same speed should cross it at the same rate whichever
// kind they are.

// thingMotionPositions keeps each moving thing's unrounded position between
// ticks.
//
// A thing's own position label is a whole cell, and it has to stay one — the
// board, the layout and every reader of canvas-x/y are built around cells. But
// a velocity below one cell per tick cannot survive being rounded every tick:
// it would round back to where it started, forever, and the thing would sit
// still no matter what speed it was given. So the fractional position lives
// here and only the rounded value is written out.
//
// Keyed by thing id. An entry is dropped when the thing stops, so the map is
// as small as the number of things actually in motion, and a thing that starts
// again picks up from wherever its label says it is rather than from a stale
// fraction it left behind minutes ago.
var (
	thingMotionMu        sync.Mutex
	thingMotionPositions = map[string][2]float64{}
)

// startThingMotion begins the loop. Called once at server start.
func (s *Server) startThingMotion() {
	go func() {
		ticker := time.NewTicker(thingMotionInterval)
		defer ticker.Stop()
		for range ticker.C {
			s.thingMotionTick()
		}
	}()
}

// thingMotionInterval matches CharacterMotionInterval deliberately — see the
// note above about the same board. Not imported from core: that constant is
// about how often a want paces itself, and this is a ticker of the server's
// own; sharing the number is the point, sharing the declaration would tie two
// unrelated schedules together.
const thingMotionInterval = 250 * time.Millisecond

// thingMotionTick advances every moving thing by one tick.
func (s *Server) thingMotionTick() {
	// Nothing to move things against before the board exists — and reading the
	// want list through a nil builder is a panic in a goroutine nobody is
	// watching.
	if s.thingLabels == nil || s.globalBuilder == nil {
		return
	}
	all := s.thingLabels.All()

	// Built once for the whole tick rather than per thing: it walks every want,
	// and every thing on the board is being stopped by the same walls.
	var blocked map[[2]int]bool
	var allWants []*mywant.Want
	built := false

	moves := make([]thingMove, 0, 4)
	for id, labels := range all {
		b, moving, ok := thingBodyOf(labels)
		if !ok || !moving {
			// Not in motion. Forget what it was carrying between ticks, so
			// starting again begins from where its label says it is and from
			// no heading rather than a stale one.
			thingMotionMu.Lock()
			delete(thingMotionPositions, id)
			thingMotionMu.Unlock()
			forgetThingMotion(id)
			continue
		}

		if !built {
			blocked = s.blockedCellSnapshot()
			built = true
			allWants = s.globalBuilder.GetWants()
		}

		// Steering first: a direction want naming this thing decides where it
		// goes, and its own speed vector is only the magnitude then — two
		// answers to "which way" would be one too many. With nothing steering
		// it, the vector it was given is used whole, which is how a thing moves
		// when nobody is pushing it.
		if dx, dy, steered := steerThing(allWants, id, labels); steered {
			b.dx, b.dy = dx, dy
		}
		if b.dx == 0 && b.dy == 0 {
			continue
		}

		// Carry on from the unrounded position when there is one; otherwise
		// start from the label, which is where this thing actually is.
		thingMotionMu.Lock()
		if prev, ok := thingMotionPositions[id]; ok {
			b.x, b.y = prev[0], prev[1]
		}
		thingMotionMu.Unlock()

		x, y, moved, _ := stepBody(blocked, b)
		if !moved {
			// Held against a wall. Keep the fraction so it is still pressing
			// in the same direction next tick rather than resetting.
			thingMotionMu.Lock()
			thingMotionPositions[id] = [2]float64{x, y}
			thingMotionMu.Unlock()
			continue
		}

		thingMotionMu.Lock()
		thingMotionPositions[id] = [2]float64{x, y}
		thingMotionMu.Unlock()

		// Only write when the CELL changed. A slow thing crosses several ticks
		// per cell, and writing the same label each time would churn the store
		// and wake every browser for a move nobody can see.
		nx, ny := cellLabel(x), cellLabel(y)
		if nx == labels["mywant.io/canvas-x"] && ny == labels["mywant.io/canvas-y"] {
			continue
		}
		if err := s.thingLabels.Set(id, "mywant.io/canvas-x", nx); err != nil {
			continue
		}
		if err := s.thingLabels.Set(id, "mywant.io/canvas-y", ny); err != nil {
			continue
		}
		moves = append(moves, thingMove{ID: id, X: nx, Y: ny})

		// Arriving somewhere can put it on a plate, exactly as a footstep can.
		s.syncThingOccupancy(id, x, y)
	}

	// One frame for the whole tick, carrying the positions themselves.
	//
	// This used to send "thing_changed", the event that means the set of things
	// is different — a world was opened, something was added or thrown away —
	// and which a browser answers by fetching the entire catalog back. That is
	// the right answer to that question and the wrong one to this: a thing that
	// moved one cell is the same thing, and asking for the catalog four times a
	// second buries the move under work nobody asked for. A position is small
	// enough to just say, so it is said here, the way a character's is.
	if len(moves) > 0 {
		go broadcastSSE("thing_moved", moves)
	}
}

// thingMove is one thing's new cell, as the browser needs it: whole cells,
// already formatted the way the labels carry them, so the two cannot disagree.
type thingMove struct {
	ID string `json:"id"`
	X  string `json:"x"`
	Y  string `json:"y"`
}
