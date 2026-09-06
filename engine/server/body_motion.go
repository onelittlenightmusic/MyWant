package server

import (
	"strconv"
	"strings"
)

// Motion, for anything on the board that has a position.
//
// A character was the only thing that could move, so everything about moving
// lived in the character path: the speed, the going flag, the collision
// check, the write-back. None of that is actually about characters. A thing
// sitting on a cell is the same kind of object — it has a position, it can be
// given a velocity, and if it is going to travel it has to be stopped by the
// same walls. So the part that is genuinely shared is named here and both
// callers go through it: driveOneCharacterTick for a character, thingMotion
// for a thing.
//
// What stays with each caller is only what genuinely differs — where the
// position is KEPT. A character's lives in the ephemeral cursor store; a
// thing's rests on its own labels and, while it is actually in flight, in the
// motion store (see thingMotionState) for the same reason a character's is
// ephemeral: a live position is not something to write to disk four times a
// second. Neither of those is a fact about motion.

// Label keys a thing's own motion is kept under, alongside the canvas-x/y it
// has always had. Prefixed like every other board label so a thing carries its
// movement the same way it carries its place.
const (
	thingSpeedXLabel = "mywant.io/speed-x"
	thingSpeedYLabel = "mywant.io/speed-y"
	thingMovingLabel = "mywant.io/moving"
	// The pin: whether a thing is on the board at all. Three states, and the
	// absent one matters — no label means nobody has answered and the board's
	// own rule decides. "false" is a thing taken off by hand, which is also
	// what a thing's archive is (see intersection.go's trash rule).
	thingCanvasPinLabel = "mywant.io/canvas"
)

// body is what the mover needs to know about anything it moves: where it is
// and how fast it is going. Deliberately not an interface over characters and
// things — they keep their positions in places with nothing in common, and an
// interface would only be a way to pretend otherwise. A plain value, filled in
// by whoever holds the real state and read back by them afterwards.
type body struct {
	x, y   float64
	dx, dy float64
}

// stepBody advances one body by one tick and answers where it ends up.
//
// The whole of "recalculate a position" for everything on this board. It is
// the character mover's own rule, unchanged and now shared: the segment is
// verified rather than only the destination, because a fast body covers
// several cells in a tick and would otherwise step clean over a wall instead
// of into it. Being stopped is not an error — something held against a wall
// simply does not advance this tick and keeps trying on the next.
//
// A body with no velocity is not moved and not reported as stopped: standing
// still is not a collision, and treating it as one would have every idle thing
// on the board making a bump noise every tick.
func stepBody(blocked map[[2]int]bool, b body) (x, y float64, moved, stopped bool) {
	if b.dx == 0 && b.dy == 0 {
		return b.x, b.y, false, false
	}
	x, y, stopped = resolveMove(blocked, b.x, b.y, b.x+b.dx, b.y+b.dy, true)
	moved = x != b.x || y != b.y
	return x, y, moved, stopped
}

// thingBodyOf reads a thing's position and velocity off its labels.
//
// `moving` gates the velocity rather than being handed on: a thing that has
// been given a speed and then told to stop should keep the speed it was given,
// so that starting it again resumes what it was doing rather than needing the
// vector set a second time. Same shape as a character's own going flag, which
// likewise does not erase the heading it was steering by.
func thingBodyOf(labels map[string]string) (b body, moving bool, ok bool) {
	x, errX := strconv.ParseFloat(labels["mywant.io/canvas-x"], 64)
	y, errY := strconv.ParseFloat(labels["mywant.io/canvas-y"], 64)
	if errX != nil || errY != nil {
		// Not placed on the board at all. Nothing to move.
		return body{}, false, false
	}
	moving = labels[thingMovingLabel] == "true"
	dx, _ := strconv.ParseFloat(labels[thingSpeedXLabel], 64)
	dy, _ := strconv.ParseFloat(labels[thingSpeedYLabel], 64)
	if !moving {
		dx, dy = 0, 0
	}
	return body{x: x, y: y, dx: dx, dy: dy}, moving, true
}

// posLabel formats a coordinate for a label.
//
// A canvas label used to carry a whole cell and nothing else. It does not have
// to: a cell is where a thing RESTS, and every "which thing is on this square"
// question rounds anyway, but nothing says a thing must come to rest on the
// grid. Keeping the fraction is what lets something drift to a halt just past
// a cell instead of snapping onto it.
//
// Three decimals, trailing zeros trimmed. Enough that a board this size cannot
// tell the difference, short enough that a label stays readable, and rounded
// rather than shortest-representation so a position never comes out as
// "6.700000000000001".
func posLabel(v float64) string {
	s := strconv.FormatFloat(v, 'f', 3, 64)
	s = strings.TrimRight(s, "0")
	return strings.TrimSuffix(s, ".")
}

// parseFloatLabel reads a number off a label.
func parseFloatLabel(raw string) (float64, error) {
	return strconv.ParseFloat(strings.TrimSpace(raw), 64)
}
