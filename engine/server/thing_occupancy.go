package server

import (
	"math"
	"strconv"
	"sync"

	mywant "mywant/engine/core"
)

// A thing standing on a want.
//
// The character version of this (button_occupancy.go) says it is deliberately
// generic — keyed on the form-type label rather than a whitelist of types, so
// any want that wants "targets whoever is standing on me" gets it by adding a
// label. It was generic in every way except one: only a character could be the
// whoever. A thing has a position on the same board and can be pushed across
// it, so it can stand on a plate exactly as a character can, and the wants that
// react to being stood on have no reason to care which arrived.
//
// So the same transition, against the `things` array instead of `characters`,
// and with a thing's own equivalents of the two things a footstep does: a going
// want turns its moving flag on, and a direction want steers it (that half
// needs nothing here — thingMotionTick reads the same vote the character tick
// does, once the thing is named in the array).
var (
	thingOnButtonMu sync.Mutex
	// Which want each thing is standing on, so a transition can be told from
	// standing still. Mirrors characterOnButton.
	thingOnButton = map[string]string{}
)

// syncThingOccupancy re-checks which form-type:button want (if any) sits on the
// cell a thing just moved to, and adds or removes it from that want's `things`
// array. Called from the one place a thing's position is written by motion —
// thingMotionTick — for the same reason the character one is called from every
// place a character's is.
func (s *Server) syncThingOccupancy(thingID string, newX, newY float64) {
	if s.globalBuilder == nil {
		return
	}
	applyThingOccupancy(s, thingID, newX, newY, s.globalBuilder.GetWants(), s.isButtonFormType)
}

func applyThingOccupancy(
	s *Server, thingID string, newX, newY float64,
	allWants []*mywant.Want, isButton func(typeName string) bool,
) {
	rx, ry := strconv.Itoa(int(math.Round(newX))), strconv.Itoa(int(math.Round(newY)))

	thingOnButtonMu.Lock()
	defer thingOnButtonMu.Unlock()

	prevWantID := thingOnButton[thingID]
	newWant := findButtonWantAtCell(rx, ry, allWants, isButton)
	newWantID := ""
	if newWant != nil {
		newWantID = newWant.Metadata.ID
	}
	if newWantID == prevWantID {
		return // still on whatever it was on; no transition to apply
	}

	if prevWantID != "" {
		for _, w := range allWants {
			if w.Metadata.ID == prevWantID {
				removeTargetFromWant(w, targetsThings, thingID)
				break
			}
		}
	}
	if newWant != nil {
		addTargetToWant(newWant, targetsThings, thingID)
		s.toggleThingGoingOnStep(newWant, thingID)
	}
	thingOnButton[thingID] = newWantID
}

// toggleThingGoingOnStep is a going want turning a thing's motion on as it
// arrives — the same pressure plate a character steps on.
//
// Written straight to the thing's label rather than queued as a webhook the way
// the character version is, and the difference is real rather than a shortcut:
// a character's going lives on a want (their own "character_motion"), so the
// instruction has to reach that want's Progress to be carried out, and queueing
// is how it gets there without a second implementation. A thing's moving flag
// is a label, and a label is written by writing it. There is no other half to
// keep in step with.
//
// "toggle" for the same reason: stepping onto the plate a second time stops
// what the first arrival started, which is what a pressure plate reads as.
func (s *Server) toggleThingGoingOnStep(want *mywant.Want, thingID string) {
	if want.Metadata.Type != "going" || thingID == "" || s.thingLabels == nil {
		return
	}
	labels := s.thingLabels.Get(thingID)
	next := "true"
	if labels[thingMovingLabel] == "true" {
		next = "false"
	}
	_ = s.thingLabels.Set(thingID, thingMovingLabel, next)
	go broadcastSSE("thing_changed", thingID)
}

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
