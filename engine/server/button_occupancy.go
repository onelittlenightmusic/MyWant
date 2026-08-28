package server

import (
	"math"
	"strconv"
	"sync"
	"time"

	mywant "mywant/engine/core"
)

// ── form-type:button occupancy ────────────────────────────────────────────────
//
// A want type opts into "step on me" targeting by setting the label
// `form-type: button` on its metadata (the same label the GUI reads to draw it
// as a round, sinkable tile — see wantForm.ts on the frontend). Any such want
// that also carries a `characters` current-state array (direction/going/gear
// all do) gets that array kept in sync with who is physically standing on its
// canvas tile: a character's ID is added the moment they step onto the tile,
// and removed the moment they step off. Nothing else changes — the existing
// per-want-type effect logic (drive_engine.go's vote counting, vector sum,
// multiplier) just keeps reading `characters` as it always has, now populated
// by footsteps instead of by YAML.
//
// This is deliberately generic rather than drive-specific: it is keyed only on
// the form-type label, not on a whitelist of type names, so any future
// want type that wants "targets whoever's standing on me" gets it for free by
// adding the label and a `characters` state field.

var (
	buttonOccupancyMu sync.Mutex
	// characterOnButton remembers, for each character, the ID of the
	// form-type:button want they were standing on as of the last position
	// update — "" if none. Touched from both HTTP handler goroutines
	// (ordinary movement) and each character's own "character_motion" want's
	// goroutine (driven movement), so it needs its own lock.
	characterOnButton = map[string]string{}
)

// syncButtonOccupancy re-checks which form-type:button want (if any) sits on
// the cell a character just moved to, and adds/removes that character from
// the want's `characters` current-state array accordingly. Called from every
// place a character's position is written: the ordinary cursor PUT handler
// and the drive engine's own moveDrivenCharacter.
func (s *Server) syncButtonOccupancy(characterID string, newX, newY float64) {
	if s.globalBuilder == nil {
		return
	}
	applyButtonOccupancy(characterID, newX, newY, s.globalBuilder.GetWants(), s.isButtonFormType)
}

// applyButtonOccupancy holds the actual transition logic, taking the want
// list and the form-type check as plain parameters rather than reaching into
// the Server — so it's exercised directly in tests without standing up a real
// ChainBuilder/WantTypeLoader.
func applyButtonOccupancy(characterID string, newX, newY float64, allWants []*mywant.Want, isButton func(typeName string) bool) {
	rx, ry := strconv.Itoa(int(math.Round(newX))), strconv.Itoa(int(math.Round(newY)))

	buttonOccupancyMu.Lock()
	defer buttonOccupancyMu.Unlock()

	prevWantID := characterOnButton[characterID]
	newWant := findButtonWantAtCell(rx, ry, allWants, isButton)
	newWantID := ""
	if newWant != nil {
		newWantID = newWant.Metadata.ID
	}
	if newWantID == prevWantID {
		return // standing exactly where they were; no transition to apply
	}

	if prevWantID != "" {
		for _, w := range allWants {
			if w.Metadata.ID == prevWantID {
				removeCharacterFromWant(w, characterID)
				break
			}
		}
	}
	if newWant != nil {
		addCharacterToWant(newWant, characterID)
		toggleGoingOnStep(newWant, characterID, allWants)
	}
	characterOnButton[characterID] = newWantID
}

// toggleGoingOnStep asks a "going" want to flip the *stepping character's own*
// going/stopped flag the instant a footstep lands on it — a pressure plate, not
// a switch someone else has to reach into the sidebar and throw.
//
// Asks rather than does. It used to write the character's flag here, which made
// two implementations of one instruction: this one knew who had stepped, and
// the webhook one in engine/types knew how to interpret an action, and neither
// knew the other's half — so the card's toggle applied to nobody. A footstep is
// now the same instruction the card sends, queued on the same want and carried
// out by the same Progress (see going_types.go), and this is only the door it
// comes in by.
//
// "toggle" rather than "going" so a second footstep stops what the first one
// started for *that character* — the "step on it again to turn it off" a real
// pressure plate reads as. Stepping *off* does not touch it either way: going,
// once set, stays set regardless of where the character wanders next.
func toggleGoingOnStep(want *mywant.Want, characterID string, allWants []*mywant.Want) {
	if want.Metadata.Type != "going" || characterID == "" {
		return
	}
	want.AppendState("webhook_queue", map[string]any{
		"payload":    map[string]any{"action": "toggle", "character_id": characterID},
		"receivedAt": time.Now().Format(time.RFC3339Nano),
	})
}

// findButtonWantAtCell returns the form-type:button want sitting at (x, y)
// (as canvas-x/canvas-y label strings), or nil if there isn't one.
func findButtonWantAtCell(x, y string, allWants []*mywant.Want, isButton func(typeName string) bool) *mywant.Want {
	for _, want := range allWants {
		if want.GetLabel(canvasLabelX) != x || want.GetLabel(canvasLabelY) != y {
			continue
		}
		if !isButton(want.Metadata.Type) {
			continue
		}
		return want
	}
	return nil
}

// isButtonFormType reports whether a want type's definition declares
// `form-type: button`. Reads the live type definition rather than a
// hardcoded list, so this applies to any want type carrying the label —
// today that's direction/going/gear, but nothing here knows their names.
func (s *Server) isButtonFormType(typeName string) bool {
	if s.wantTypeLoader == nil {
		return false
	}
	def := s.wantTypeLoader.GetDefinition(typeName)
	return def != nil && def.Metadata.Labels["form-type"] == "button"
}

// addCharacterToWant adds characterID to a want's `characters` current state
// if it isn't already there.
func addCharacterToWant(want *mywant.Want, characterID string) {
	current := characterIDsOf(want)
	for _, id := range current {
		if id == characterID {
			return
		}
	}
	want.SetCurrent("characters", append(append([]string{}, current...), characterID))
}

// removeCharacterFromWant removes characterID from a want's `characters`
// current state, if present.
func removeCharacterFromWant(want *mywant.Want, characterID string) {
	current := characterIDsOf(want)
	out := make([]string, 0, len(current))
	for _, id := range current {
		if id != characterID {
			out = append(out, id)
		}
	}
	if len(out) != len(current) {
		want.SetCurrent("characters", out)
	}
}
