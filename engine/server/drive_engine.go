package server

import (
	"log"
	"math"
	"sync"
	"time"

	mywant "mywant/engine/core"
)

// baseSpeedCellsPerSec is the movement speed (in canvas grid cells) for a
// character with an effective gear multiplier of 1.0.
const baseSpeedCellsPerSec = 1.0

// driveHeadings remembers each driven character's last resolved heading, so a
// character keeps moving in the same direction on ticks where no "direction"
// want currently targets it. This also gives a later "route"/"redirection"
// phase a natural place to override the heading before it's applied.
//
// Keyed by characterID and touched from driveOneCharacterTick, which today's
// own want engine can call for different characters on different goroutines
// at once (each character's own "character_motion" want runs through the
// normal reconcile loop, not a single shared ticker) — unlike the single
// pre-want-architecture ticker this used to run under, so a map read/write
// here needs its own lock now. See driveMu.
var driveHeadings = map[string]float64{}

// driveMu guards driveHeadings. Used to also guard a "going" ownership map
// shared across characters — removed once going itself moved onto each
// character's own "character_motion" want (see that type's "going" field):
// a character's going/stopped flag is that character's own state now, kept
// with them regardless of which going want (if any) they're currently
// touching, so there is nothing left to steal or leak between characters and
// nothing to carry forward — it was never anyone else's to begin with.
var driveMu sync.Mutex

type driveTarget struct {
	gearMultiplier float64
	hasGear        bool
	dirVectorX     float64
	dirVectorY     float64
	hasDirection   bool
}

// directionVectorOf and gearValueOf read a direction/gear want's persistent
// current-state fields through GetCurrent[float64], not a raw type
// assertion: a value SetCurrent wrote this tick really is a Go float64, but
// one loaded from state.yaml on boot came back through YAML's own decoder,
// which hands an integer-looking scalar back as int, not float64 — so
// `value.(float64)` used to fail silently for exactly the values these want
// types exist to persist (see want.go's prepareForRestart, fixed to stop
// discarding them — this is the next thing that broke once they actually
// survived a restart to be read). GetCurrent[T] coerces either
// representation.
func directionVectorOf(want *mywant.Want) (dx, dy float64) {
	return mywant.GetCurrent(want, "dx", 0.0), mywant.GetCurrent(want, "dy", 0.0)
}

func gearValueOf(want *mywant.Want) float64 {
	return mywant.GetCurrent(want, "value", 1.0)
}

// driveOneCharacterTick resolves and applies one tick of motion for a single
// character — called from that character's own "character_motion" want's
// Progress() (via core.CharacterMotionTick, wired in server.go), which also
// hands over its own want (motionWant) so going can be read straight off
// the one place it's actually kept: the character's own state, not a going
// want shared with — and toggled independently by — anyone else who happens
// to also be near it (see button_occupancy.go's toggleGoingOnStep).
//
// Still has to scan every want to find which direction/gear wants currently
// target this one character for steering and speed — there is no reverse
// index from character to the wants voting on it, only each want's own
// `characters` list — so this is O(wants) per character per tick. Accepted
// for now; see the design note where this architecture was introduced if
// that ever needs revisiting at a much larger character count.
//
// Returns the character's resulting position and true if this tick actually
// moved them — false (with x=y=0) when there was nothing to do (not going,
// or going with no heading ever set) — so the caller (a "character_motion"
// want's Progress()) can mirror the real position into its own state for
// observability without a second lookup back into the cursor store, which
// only this (server) package can reach.
func driveOneCharacterTick(s *Server, characterID string, motionWant *mywant.Want) (x, y float64, moved bool) {
	builder := mywant.GetGlobalChainBuilder()
	if builder == nil {
		return
	}

	target := &driveTarget{gearMultiplier: 1}

	for _, want := range builder.GetWants() {
		switch want.Metadata.Type {
		case "gear":
			if !containsCharacter(want, characterID) {
				continue
			}
			gearVal := gearValueOf(want)
			if !target.hasGear {
				target.gearMultiplier = 1
				target.hasGear = true
			}
			target.gearMultiplier *= gearVal
		case "direction":
			if !containsCharacter(want, characterID) {
				continue
			}
			dxVal, dyVal := directionVectorOf(want)
			// Raw (unnormalized) vector: a want's magnitude acts as its
			// weight when combined with other direction wants targeting
			// the same character.
			target.dirVectorX += dxVal
			target.dirVectorY += dyVal
			target.hasDirection = true
		}
	}

	going := mywant.GetCurrent(motionWant, "going", false)

	driveMu.Lock()
	heading, hasHeading := driveHeadings[characterID]
	if target.hasDirection && (target.dirVectorX != 0 || target.dirVectorY != 0) {
		heading = math.Atan2(target.dirVectorY, target.dirVectorX) * 180 / math.Pi
		if heading < 0 {
			heading += 360
		}
		hasHeading = true
		driveHeadings[characterID] = heading
	}
	driveMu.Unlock()

	dx, dy, moved := resolveMotion(going, heading, hasHeading, target.gearMultiplier, speedOfCharacter(characterID))
	if !moved {
		return 0, 0, false
	}
	x, y = moveDrivenCharacter(s, characterID, dx, dy)
	return x, y, true
}

// speedOfCharacter resolves a character's own configured speed, falling
// back to baseSpeedCellsPerSec when the character has none set (0) or isn't
// known at all.
func speedOfCharacter(characterID string) float64 {
	if character, ok := mywant.GetCharacter(characterID); ok && character.Speed > 0 {
		return character.Speed
	}
	return baseSpeedCellsPerSec
}

// resolveMotion turns one character's already-collected going/heading/gear
// inputs into a single tick's motion, in whole grid cells. Pure, so the
// whole-cell-per-tick rule and the "no heading yet means stand still" rule
// stay unit-testable without a running engine.
//
// Deliberately not scaled by how often the tick fires: distance is
// `speed * gearMultiplier` cells every tick, full stop, so `speed` reads as
// "cells per tick" rather than "cells per second". A cells-per-second
// version existed briefly — it kept the same overall rate at any tick
// interval, which sounds right until the tick interval gets shorter than a
// whole cell's worth of movement: each tick then advances a *fraction* of a
// cell, so a driven character glides between grid lines instead of hopping
// across them, on a board built entirely around the hop. Whole-cell steps
// are the point; how often driveOneCharacterTick runs controls how often one
// happens, and that alone is "faster" or "slower" here.
func resolveMotion(going bool, heading float64, hasHeading bool, gearMultiplier, speed float64) (dx, dy float64, moved bool) {
	if !going {
		return 0, 0, false
	}
	// A going character with nobody currently or ever telling it which way
	// to go must not move at all — not "east", which is just where an unset
	// float64 happens to point. Stepping onto a bare going plate, with no
	// direction want anywhere in the mix, used to send the character
	// marching off it on its own; standing still until an actual heading
	// exists is what "going" without a direction should mean. Once a
	// direction want *has* targeted this character at least once, its last
	// resolved heading persists (see driveHeadings) and keeps steering
	// movement even on a tick where the vote briefly drops out.
	if !hasHeading {
		return 0, 0, false
	}
	distance := gearMultiplier * speed
	rad := heading * math.Pi / 180
	return distance * math.Cos(rad), distance * math.Sin(rad), true
}

// containsCharacter reports whether characterID is in a want's `characters`
// current-state array.
func containsCharacter(want *mywant.Want, characterID string) bool {
	for _, id := range characterIDsOf(want) {
		if id == characterID {
			return true
		}
	}
	return false
}

// characterIDsOf reads a want's "characters" current-state array (mirrored
// there from the "characters" parameter at Initialize time, or from
// footstep occupancy — see button_occupancy.go).
func characterIDsOf(want *mywant.Want) []string {
	raw, ok := want.GetCurrent("characters")
	if !ok {
		return nil
	}
	switch v := raw.(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

// moveDrivenCharacter applies a relative (dx, dy) movement (in grid-cell
// units) to a character's position in the shared ephemeral cursor store,
// enriching the entry with the character's display fields, and broadcasts
// the updated snapshot over SSE. Also re-checks which form-type:button want
// (if any) the character now stands on, since a "going" push is exactly the
// kind of move that can walk them onto or off of one.
func moveDrivenCharacter(s *Server, characterID string, dx, dy float64) (newX, newY float64) {
	character, ok := mywant.GetCharacter(characterID)
	if !ok {
		log.Printf("[DriveEngine] unknown character %q referenced by a drive want; skipping", characterID)
		return 0, 0
	}

	// Built before the lock — it walks every want — so the check and the write
	// below stay atomic with respect to each other.
	blocked := s.blockedCellSnapshot()

	cursorsMu.Lock()
	entry := cursors[characterID]
	// This is walking, so the whole segment is verified, not just where it
	// lands: a gear-multiplied push covers several cells in one tick and
	// would otherwise step clean over a wall instead of into it. Being
	// stopped is not an error — a character held against a wall simply does
	// not advance this tick, and keeps trying on the next one.
	var stopped bool
	entry.X, entry.Y, stopped = resolveMove(blocked, entry.X, entry.Y, entry.X+dx, entry.Y+dy, true)
	// Walking into something solid makes a noise, and this is the one path
	// where the browser never gets the chance to make it itself — nothing
	// here started in a keypress. See bumpEffectType.
	if stopped {
		entry.Effects = appendEffect(entry.Effects, effectEvent{
			Type: bumpEffectType, Nonce: time.Now().UnixMilli(), X: entry.X, Y: entry.Y,
		})
	} else {
		entry.Effects = appendNoop(entry.Effects)
	}
	entry.Avatar = character.Avatar
	entry.Color = character.Color
	entry.Name = character.Name
	entry.LastSeen = time.Now().UnixMilli()
	cursors[characterID] = entry
	newX, newY = entry.X, entry.Y
	cursorsMu.Unlock()

	s.syncButtonOccupancy(characterID, newX, newY)

	go broadcastSSE("cursor", snapshotCursors())
	return newX, newY
}
