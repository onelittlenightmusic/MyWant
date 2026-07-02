package server

import (
	"log"
	"math"
	"time"

	mywant "mywant/engine/core"
)

// driveEngineTick is how often the drive engine recomputes character movement.
const driveEngineTick = 1 * time.Second

// baseSpeedCellsPerSec is the movement speed (in canvas grid cells) for a
// character with an effective gear multiplier of 1.0.
const baseSpeedCellsPerSec = 1.0

// driveHeadings remembers each driven character's last resolved heading, so a
// character keeps moving in the same direction on ticks where no "direction"
// want currently targets it. This also gives a later "route"/"redirection"
// phase a natural place to override the heading before it's applied.
// Only ever read/written from driveEngineTickOnce, which runs sequentially
// on a single ticker goroutine, so no locking is needed.
var driveHeadings = map[string]float64{}

// startDriveEngine launches the background goroutine that moves characters
// targeted by "going"/"gear"/"direction" wants once per driveEngineTick.
func startDriveEngine() {
	go func() {
		ticker := time.NewTicker(driveEngineTick)
		defer ticker.Stop()
		for range ticker.C {
			driveEngineTickOnce()
		}
	}()
}

type driveTarget struct {
	goingVotes     []bool
	gearMultiplier float64
	hasGear        bool
	dirVectorX     float64
	dirVectorY     float64
	hasDirection   bool
}

// driveEngineTickOnce enumerates all going/gear/direction wants, resolves the
// combined motion for every targeted character, and pushes updated positions
// into the shared cursor store.
func driveEngineTickOnce() {
	builder := mywant.GetGlobalChainBuilder()
	if builder == nil {
		return
	}

	targets := map[string]*driveTarget{}
	getTarget := func(characterID string) *driveTarget {
		t, ok := targets[characterID]
		if !ok {
			t = &driveTarget{gearMultiplier: 1}
			targets[characterID] = t
		}
		return t
	}

	for _, want := range builder.GetWants() {
		switch want.Metadata.Type {
		case "going":
			going, _ := want.GetCurrent("going")
			goingBool, _ := going.(bool)
			for _, charID := range characterIDsOf(want) {
				t := getTarget(charID)
				t.goingVotes = append(t.goingVotes, goingBool)
			}
		case "gear":
			value, _ := want.GetCurrent("value")
			gearVal, ok := value.(float64)
			if !ok {
				gearVal = 1
			}
			for _, charID := range characterIDsOf(want) {
				t := getTarget(charID)
				if !t.hasGear {
					t.gearMultiplier = 1
					t.hasGear = true
				}
				t.gearMultiplier *= gearVal
			}
		case "direction":
			degrees, _ := want.GetCurrent("degrees")
			degVal, ok := degrees.(float64)
			if !ok {
				degVal = 0
			}
			rad := degVal * math.Pi / 180
			for _, charID := range characterIDsOf(want) {
				t := getTarget(charID)
				t.dirVectorX += math.Cos(rad)
				t.dirVectorY += math.Sin(rad)
				t.hasDirection = true
			}
		}
	}

	for charID, t := range targets {
		going := resolveGoing(t.goingVotes)

		heading, hasHeading := driveHeadings[charID]
		if t.hasDirection && (t.dirVectorX != 0 || t.dirVectorY != 0) {
			heading = math.Atan2(t.dirVectorY, t.dirVectorX) * 180 / math.Pi
			if heading < 0 {
				heading += 360
			}
			hasHeading = true
		}
		if !hasHeading {
			heading = 0
		}
		driveHeadings[charID] = heading

		if !going {
			continue
		}

		distance := t.gearMultiplier * baseSpeedCellsPerSec * driveEngineTick.Seconds()
		rad := heading * math.Pi / 180
		dx := distance * math.Cos(rad)
		dy := distance * math.Sin(rad)

		moveDrivenCharacter(charID, dx, dy)
	}
}

// resolveGoing applies the "stopped wins" priority rule across every going
// want targeting a character: if any vote is false (stopped), the character
// is stopped, regardless of how many wants vote to go.
func resolveGoing(votes []bool) bool {
	if len(votes) == 0 {
		return false
	}
	for _, v := range votes {
		if !v {
			return false
		}
	}
	return true
}

// characterIDsOf reads a want's "characters" current-state array (mirrored
// there from the "characters" parameter at Initialize time).
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
// the updated snapshot over SSE.
func moveDrivenCharacter(characterID string, dx, dy float64) {
	character, ok := mywant.GetCharacter(characterID)
	if !ok {
		log.Printf("[DriveEngine] unknown character %q referenced by a drive want; skipping", characterID)
		return
	}

	cursorsMu.Lock()
	entry := cursors[characterID]
	entry.X += dx
	entry.Y += dy
	entry.Avatar = character.Avatar
	entry.Color = character.Color
	entry.Name = character.Name
	entry.LastSeen = time.Now().UnixMilli()
	cursors[characterID] = entry
	cursorsMu.Unlock()

	go broadcastSSE("cursor", snapshotCursors())
}
