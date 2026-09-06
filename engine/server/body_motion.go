package server

import (
	"math"
	"strconv"
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
// position is KEPT. A character's lives in the ephemeral cursor store (floats,
// broadcast over SSE, owned by the server for the length of a push); a thing's
// lives on its own labels, which is where a thing's position has always lived.
// Neither of those is a fact about motion.

// Label keys a thing's own motion is kept under, alongside the canvas-x/y it
// has always had. Prefixed like every other board label so a thing carries its
// movement the same way it carries its place.
const (
	thingSpeedXLabel = "mywant.io/speed-x"
	thingSpeedYLabel = "mywant.io/speed-y"
	thingMovingLabel = "mywant.io/moving"
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

// cellLabel formats a resolved coordinate the way a canvas label has always
// carried one: a whole cell.
//
// The position itself is a float while it is being resolved, because a
// velocity smaller than a cell has to be able to accumulate — rounding every
// tick would leave anything slower than one cell per tick permanently still.
// See thingMotion, which keeps the unrounded position between ticks and only
// rounds on the way out.
func cellLabel(v float64) string {
	return strconv.Itoa(int(math.Round(v)))
}
