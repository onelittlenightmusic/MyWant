package server

import (
	"math"
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

// thingMotionState is where a thing in flight actually is.
//
// In memory, not on the thing's labels, and that is the whole design. A label
// write rewrites the label file — every label, every time (see
// ThingLabelStore.Set) — so a position written per tick would be a full disk
// round trip four times a second per moving thing. A character's live position
// does not touch disk either; it lives in the cursor store and is broadcast.
// This is the same answer to the same problem.
//
// It also lets the position be a real number. A cell is where a thing RESTS —
// the board, the layout and every "which thing is on this square" question are
// about cells — but nothing in between has to be, and rounding every tick
// meant a thing moving slower than a cell per tick rounded back to where it
// started, forever. The fraction is the motion.
//
// The entry is created when a thing starts moving and dropped when it stops,
// so the map is as small as the number of things actually in flight, and a
// thing that starts again picks up from its label rather than from a stale
// fraction it left behind minutes ago.
type thingMotionState struct {
	x, y float64
	// Velocity in cells per TICK, which is what stepBody adds to a position.
	// The same units the character mover uses, misleadingly named "speed in
	// cells per second" over there; keeping them identical is what makes a
	// thing and a character given the same numbers move alike.
	vx, vy float64
}

var (
	thingMotionMu     sync.Mutex
	thingMotionStates = map[string]*thingMotionState{}
)

// thingMotionInterval matches CharacterMotionInterval deliberately — see the
// note above about the same board. Not imported from core: that constant is
// about how often a want paces itself, and this is a ticker of the server's
// own; sharing the number is the point, sharing the declaration would tie two
// unrelated schedules together.
const thingMotionInterval = 250 * time.Millisecond

// Friction.
//
// A thing shoved across a board slows down and stops. Without that, "moving"
// was a switch rather than a shove: anything given a speed travelled in a
// straight line until it met a wall, and the only way to stop it was to reach
// in and turn it off. A throw should land.
//
// Expressed as the fraction of speed KEPT after one second, not per tick, so
// the number means something on its own and does not silently change if the
// tick rate ever does. A thing shoved at one cell per tick coasts about three
// cells before it settles — far enough to read as a throw, short enough that
// the board does not fill up with drifting objects.
//
// Overridable per thing with `mywant.io/damping`: 1 is frictionless — ice, or
// anything meant to keep going until something stops it — and 0 is a thing
// that does not slide at all.
const (
	thingDampingPerSecond = 0.25
	thingDampingLabel     = "mywant.io/damping"
	// Below this (in cells per tick) a thing is not moving, it is drifting
	// imperceptibly, and it is put down properly instead.
	thingRestSpeed = 0.02
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
	rested := make([]string, 0, 2)

	for id, labels := range all {
		b, moving, ok := thingBodyOf(labels)
		if !ok || !moving {
			// Stopped from outside — somebody turned the flag off, or took the
			// thing off the board — rather than by running out of speed. Put it
			// down where it actually is first: the label still says where the
			// throw BEGAN, and dropping the flight state without writing would
			// teleport it back there.
			if x, y, flying := landThing(id); flying {
				s.putThingDown(id, x, y)
				rested = append(rested, id)
			}
			continue
		}

		if !built {
			blocked = s.blockedCellSnapshot()
			built = true
			allWants = s.globalBuilder.GetWants()
		}

		st := beginOrContinueFlight(id, b)

		// A want steering this thing is a motor, not a shove: it sets the
		// velocity outright every tick, so friction never accumulates against
		// it and a driven thing holds its speed. Friction is for what is
		// coasting on its own momentum, which is the only thing that should
		// come to a stop by itself.
		//
		// `decay` is also what the NEXT step will be scaled by, which is what
		// makes the velocity reported below a prediction rather than a guess —
		// see thingMove.
		decay := 1.0
		if dx, dy, steered := steerThing(allWants, id, labels); steered {
			st.vx, st.vy = dx, dy
		} else {
			decay = math.Pow(dampingOf(labels), thingMotionInterval.Seconds())
			st.vx *= decay
			st.vy *= decay
		}

		x, y, moved, _ := stepBody(blocked, body{x: st.x, y: st.y, dx: st.vx, dy: st.vy})

		// Two ways to be finished: slowed to nothing, or held against a wall.
		// Both are the thing coming to rest, and both end the same way — put it
		// down where it is and say so — because a thing pressed motionless
		// against a wall for the rest of the session is not in flight.
		if math.Hypot(st.vx, st.vy) < thingRestSpeed || !moved {
			landThing(id)
			s.putThingDown(id, x, y)
			rested = append(rested, id)
			// A last frame, at rest. Without it a browser filling the gaps
			// between frames would carry on past the true stopping point on the
			// velocity it was last told about, and only be corrected by the
			// next thing that happened to refetch. A zero velocity is the
			// instruction to stop predicting.
			moves = append(moves, thingMove{ID: id, X: x, Y: y})
			continue
		}

		st.x, st.y = x, y
		moves = append(moves, thingMove{
			ID: id, X: x, Y: y,
			VX: st.vx * decay, VY: st.vy * decay,
		})

		// Arriving somewhere can put it on a plate, exactly as a footstep can.
		s.syncThingOccupancy(id, x, y)
	}

	// One frame for the whole tick, carrying the positions themselves.
	//
	// This used to send "thing_changed", the event that means the set of things
	// is different — a world was opened, something was added or thrown away —
	// and which a browser answers by fetching the entire catalog back. That is
	// the right answer to that question and the wrong one to this: a thing that
	// moved is the same thing, and asking for the catalog four times a second
	// buries the move under work nobody asked for. A position is small enough
	// to just say, so it is said here, the way a character's is.
	if len(moves) > 0 {
		go broadcastSSE("thing_moved", moves)
	}
	// Coming to rest is a different event: the moving flag changed, which is a
	// fact about the thing rather than about where it is.
	for _, id := range rested {
		go broadcastSSE("thing_changed", id)
	}
}

// beginOrContinueFlight returns the thing's live motion state, seeding it from
// its labels the first tick of a flight.
func beginOrContinueFlight(id string, b body) *thingMotionState {
	thingMotionMu.Lock()
	defer thingMotionMu.Unlock()
	if st, ok := thingMotionStates[id]; ok {
		return st
	}
	st := &thingMotionState{x: b.x, y: b.y, vx: b.dx, vy: b.dy}
	thingMotionStates[id] = st
	return st
}

// landThing ends a flight, reporting where the thing had got to. Reports false
// for a thing that was not in flight, so a thing sitting still is not put down
// again every tick.
//
// Also drops the heading it was steering by: starting again begins from where
// its label says it is, with no heading, rather than from a stale one.
func landThing(id string) (x, y float64, flying bool) {
	thingMotionMu.Lock()
	st, ok := thingMotionStates[id]
	if ok {
		x, y = st.x, st.y
		delete(thingMotionStates, id)
	}
	thingMotionMu.Unlock()
	forgetThingMotion(id)
	return x, y, ok
}

// putThingDown writes where a thing ended up onto its labels.
//
// The only time flight touches the label file. Everything in between was in
// memory (see thingMotionState), and this is the one write that has to survive
// a restart — where the thing ENDED UP. The coordinates keep their fraction: a
// thing is allowed to come to rest between cells, and rounding here would
// teleport it on the last tick of every throw.
//
// The speed labels are left alone on purpose. They are the shove it was given,
// not the speed it has now, so setting moving again repeats the throw instead
// of needing the vector set a second time.
func (s *Server) putThingDown(id string, x, y float64) {
	if s.thingLabels != nil {
		_ = s.thingLabels.Set(id, "mywant.io/canvas-x", posLabel(x))
		_ = s.thingLabels.Set(id, "mywant.io/canvas-y", posLabel(y))
		_ = s.thingLabels.Set(id, thingMovingLabel, "false")
	}
	// Where it stopped counts as an arrival: a thing that slid onto a plate and
	// settled there is standing on it.
	s.syncThingOccupancy(id, x, y)
}

// dampingOf reads how slippery a thing is, as the fraction of speed kept per
// second. Anything outside 0..1 is not a damping factor — a value above one is
// a thing that accelerates itself — so the standard rate stands in.
func dampingOf(labels map[string]string) float64 {
	raw, ok := labels[thingDampingLabel]
	if !ok {
		return thingDampingPerSecond
	}
	v, err := parseFloatLabel(raw)
	if err != nil || v < 0 || v > 1 {
		return thingDampingPerSecond
	}
	return v
}

// thingMove is one thing's new position, as the browser needs it.
//
// Numbers rather than the cell strings the labels carry, because in flight
// there is no cell — the whole point of keeping the position in memory is that
// it is allowed to be between two of them.
//
// The velocity is here so the browser can fill in the gaps. Frames arrive four
// times a second because that is how often the board is recalculated, and that
// is not often enough to look like sliding; with a velocity the browser can
// carry the thing along between them and draw as often as it likes.
//
// It is the velocity of the NEXT step, not the one just taken — st.vx has
// already been damped for this tick, so damping it once more is exactly what
// the next tick will do to it. That distinction is the whole difference
// between a prediction that lands on the following frame and one that
// overshoots it by a damping step and gets visibly yanked back. A steered
// thing is not damped at all, so for it the two are the same number.
//
// Zero means stopped: the last frame of a flight carries no velocity, which is
// how a browser is told to stop filling.
type thingMove struct {
	ID string  `json:"id"`
	X  float64 `json:"x"`
	Y  float64 `json:"y"`
	// Cells per tick — the same units the speed labels use.
	VX float64 `json:"vx"`
	VY float64 `json:"vy"`
}

// liveThingPositions returns where the things currently in flight actually are,
// for anything that reads positions off the labels — which, mid-flight, are
// where the thing was when it last came to rest.
//
// Without this a page loading while something is sliding draws it at its old
// cell until the next frame arrives, and worse, any refetch during the flight
// yanks it back there.
func liveThingPositions() map[string][2]float64 {
	thingMotionMu.Lock()
	defer thingMotionMu.Unlock()
	if len(thingMotionStates) == 0 {
		return nil
	}
	out := make(map[string][2]float64, len(thingMotionStates))
	for id, st := range thingMotionStates {
		out[id] = [2]float64{st.x, st.y}
	}
	return out
}

// overlayThingMotion rewrites the canvas coordinates of anything in flight to
// where it is now. Copies the map it edits: All() hands out the store's own
// label maps, and a reader must not write through them.
func overlayThingMotion(labels map[string]map[string]string) map[string]map[string]string {
	live := liveThingPositions()
	if len(live) == 0 {
		return labels
	}
	out := make(map[string]map[string]string, len(labels))
	for id, l := range labels {
		pos, moving := live[id]
		if !moving {
			out[id] = l
			continue
		}
		copied := make(map[string]string, len(l))
		for k, v := range l {
			copied[k] = v
		}
		copied["mywant.io/canvas-x"] = posLabel(pos[0])
		copied["mywant.io/canvas-y"] = posLabel(pos[1])
		out[id] = copied
	}
	return out
}
