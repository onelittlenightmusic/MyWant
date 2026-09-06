package server

import (
	"math"
	"strconv"
	"sync"

	mywant "mywant/engine/core"
)

// How a thing in motion decides where it is going.
//
// Whether it is standing on anything, and what that means, is intersection.go's
// business — the same rules a character's footstep goes through. This is only
// the half that is about the thing itself: how fast, and which way.

// thingSpeedOf resolves how far a thing travels in one tick, from its own
// labels: the magnitude of the speed vector it was given, or the board's base
// speed when it has none.
//
// Only the magnitude, because when a direction want is steering the thing the
// heading comes from the vote and the vector's own angle would be a second,
// disagreeing answer to the same question. With no direction want the vector is
// used whole (see thingMotionTick), and this is not consulted at all.
func thingSpeedOf(labels map[string]string) float64 {
	dx, _ := strconv.ParseFloat(labels[thingSpeedXLabel], 64)
	dy, _ := strconv.ParseFloat(labels[thingSpeedYLabel], 64)
	if m := math.Hypot(dx, dy); m > 0 {
		return m
	}
	return baseSpeedCellsPerSec
}

// thingHeadings remembers each thing's last resolved heading, so it keeps going
// the same way on ticks where no direction want currently names it — exactly
// what driveHeadings does for a character, and for the same reason: a vote that
// briefly drops out is not an instruction to stop.
var (
	thingHeadingMu sync.Mutex
	thingHeadings  = map[string]float64{}
)

// steerThing turns the direction/gear wants currently naming a thing into this
// tick's velocity, or reports that none of them are.
func steerThing(allWants []*mywant.Want, thingID string, labels map[string]string) (dx, dy float64, steered bool) {
	target := collectDriveInputs(allWants, targetsThings, thingID)

	thingHeadingMu.Lock()
	heading, hasHeading := thingHeadings[thingID]
	if target.hasDirection && (target.dirVectorX != 0 || target.dirVectorY != 0) {
		heading = math.Atan2(target.dirVectorY, target.dirVectorX) * 180 / math.Pi
		if heading < 0 {
			heading += 360
		}
		hasHeading = true
		thingHeadings[thingID] = heading
	}
	thingHeadingMu.Unlock()

	if !hasHeading {
		return 0, 0, false
	}
	// The same resolver the character tick uses, so a thing and a character
	// given the same heading, gear and speed move identically.
	dx, dy, moved := resolveMotion(true, heading, hasHeading, target.gearMultiplier, thingSpeedOf(labels))
	return dx, dy, moved
}

// forgetThingMotion drops what a stopped thing was carrying between ticks.
func forgetThingMotion(thingID string) {
	thingHeadingMu.Lock()
	delete(thingHeadings, thingID)
	thingHeadingMu.Unlock()
}
