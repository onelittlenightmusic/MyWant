package server

import (
	"sync"
	"time"
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
	if s.thingLabels == nil {
		return
	}
	all := s.thingLabels.All()

	// Built once for the whole tick rather than per thing: it walks every want,
	// and every thing on the board is being stopped by the same walls.
	var blocked map[[2]int]bool
	built := false

	changed := make([]string, 0, 4)
	for id, labels := range all {
		b, moving, ok := thingBodyOf(labels)
		if !ok || !moving || (b.dx == 0 && b.dy == 0) {
			// Not in motion. Forget any fraction it was carrying, so starting
			// again begins from where its label actually says it is.
			thingMotionMu.Lock()
			delete(thingMotionPositions, id)
			thingMotionMu.Unlock()
			continue
		}

		if !built {
			blocked = s.blockedCellSnapshot()
			built = true
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
		changed = append(changed, id)
	}

	for _, id := range changed {
		go broadcastSSE("thing_changed", id)
	}
}
