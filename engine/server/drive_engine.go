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

// driveGoingOwner remembers, for each character, the ID of the "going" want
// that last claimed it by footstep (its `characters` array — see
// button_occupancy.go). A character keeps reading *that* want's live
// going/stopped state every tick even after walking off its tile, so the
// want's own card — toggled from the sidebar, not by anyone standing on it —
// keeps controlling a character it once claimed. Footstep only transfers
// ownership to a *different* going want; walking off with nothing new
// claimed leaves the previous owner in charge.
// Only ever read/written from driveEngineTickOnce — see driveHeadings.
var driveGoingOwner = map[string]string{}

// startDriveEngine launches the background goroutine that moves characters
// targeted by "going"/"gear"/"direction" wants once per driveEngineTick.
// Takes the Server so a moved character's new cell can be checked against
// form-type:button wants (see syncButtonOccupancy in button_occupancy.go).
func startDriveEngine(s *Server) {
	go func() {
		ticker := time.NewTicker(driveEngineTick)
		defer ticker.Stop()
		for range ticker.C {
			driveEngineTickOnce(s)
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
func driveEngineTickOnce(s *Server) {
	builder := mywant.GetGlobalChainBuilder()
	if builder == nil {
		return
	}

	targets := map[string]*driveTarget{}
	goingWants := map[string]*mywant.Want{}
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
			goingWants[want.Metadata.ID] = want
			going, _ := want.GetCurrent("going")
			goingBool, _ := going.(bool)
			for _, charID := range characterIDsOf(want) {
				t := getTarget(charID)
				t.goingVotes = append(t.goingVotes, goingBool)
				driveGoingOwner[charID] = want.Metadata.ID
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
			dxCur, _ := want.GetCurrent("dx")
			dyCur, _ := want.GetCurrent("dy")
			dxVal, ok1 := dxCur.(float64)
			dyVal, ok2 := dyCur.(float64)
			if !ok1 {
				dxVal = 0
			}
			if !ok2 {
				dyVal = 0
			}
			for _, charID := range characterIDsOf(want) {
				t := getTarget(charID)
				// Raw (unnormalized) vector: a want's magnitude acts as its
				// weight when combined with other direction wants targeting
				// the same character.
				t.dirVectorX += dxVal
				t.dirVectorY += dyVal
				t.hasDirection = true
			}
		}
	}

	// A character whose last footstep claimed a going want has no entry in
	// targets at all once it walks off — the want loop above only ever
	// creates one for a character it actually saw on a want's `characters`
	// array this tick. Give it one, carrying a live vote read straight from
	// its owner's current going/stopped state, not a snapshot from whenever
	// it left.
	carryForwardGoingTargets(targets, driveGoingOwner, goingWants)

	moves := resolveDriveTick(targets, driveHeadings, driveEngineTick.Seconds(), func(charID string) float64 {
		if character, ok := mywant.GetCharacter(charID); ok {
			return character.Speed
		}
		return 0
	}, baseSpeedCellsPerSec)

	for charID, m := range moves {
		moveDrivenCharacter(s, charID, m.dx, m.dy)
	}
}

// carryForwardGoingTargets gives every character with a going-want owner
// (see driveGoingOwner) a synthetic vote for this tick, read live from that
// want's current going/stopped state — but only when it doesn't already have
// a *going* vote this tick, which must not be overridden by a stale owner.
//
// That is deliberately not the same as "already has a targets entry at
// all": standing on a direction or gear want (no going want in sight) also
// creates one, for its own dx/dy or multiplier — and skipping the carried-
// forward vote whenever any entry exists made touching a direction want stop
// a character that was going, by leaving its goingVotes empty for the tick.
// Reuses that entry instead of replacing it, so a character standing on
// direction/gear *and* carrying forward a going vote keeps both.
//
// Deletes the owner entry once its want no longer exists, since there's then
// nothing left to read.
func carryForwardGoingTargets(targets map[string]*driveTarget, owner map[string]string, goingWants map[string]*mywant.Want) {
	for charID, wantID := range owner {
		if t, ok := targets[charID]; ok && len(t.goingVotes) > 0 {
			continue
		}
		want, ok := goingWants[wantID]
		if !ok {
			delete(owner, charID)
			continue
		}
		going, _ := want.GetCurrent("going")
		goingBool, _ := going.(bool)
		t, ok := targets[charID]
		if !ok {
			t = &driveTarget{gearMultiplier: 1}
			targets[charID] = t
		}
		t.goingVotes = append(t.goingVotes, goingBool)
	}
}

// resolvedMove is one character's fully-resolved motion for a tick, in grid
// cells.
type resolvedMove struct {
	dx, dy float64
}

// resolveDriveTick turns this tick's collected want votes into per-character
// motion. Pure aside from updating headings in place, the same way the
// per-tick loop always has — heading persists across ticks by design (see
// its own comment), so the update belongs here and not in the caller. Going
// needs no such treatment here: carryForwardGoingTargets already guarantees
// every target has at least one vote, live or carried forward, so plain
// resolveGoing is enough.
//
// speedOf resolves a character's own configured speed (0 if it has none, or
// the character isn't known); baseSpeed is the fallback in that case.
func resolveDriveTick(
	targets map[string]*driveTarget,
	headings map[string]float64,
	tickSeconds float64,
	speedOf func(charID string) float64,
	baseSpeed float64,
) map[string]resolvedMove {
	moves := map[string]resolvedMove{}
	for charID, t := range targets {
		going := resolveGoing(t.goingVotes)

		heading, hasHeading := headings[charID]
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
		headings[charID] = heading

		if !going {
			continue
		}

		speed := speedOf(charID)
		if speed <= 0 {
			speed = baseSpeed
		}
		distance := t.gearMultiplier * speed * tickSeconds
		rad := heading * math.Pi / 180
		moves[charID] = resolvedMove{dx: distance * math.Cos(rad), dy: distance * math.Sin(rad)}
	}
	return moves
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
// the updated snapshot over SSE. Also re-checks which form-type:button want
// (if any) the character now stands on, since a "going" push is exactly the
// kind of move that can walk them onto or off of one.
func moveDrivenCharacter(s *Server, characterID string, dx, dy float64) {
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
	newX, newY := entry.X, entry.Y
	cursorsMu.Unlock()

	s.syncButtonOccupancy(characterID, newX, newY)

	go broadcastSSE("cursor", snapshotCursors())
}
