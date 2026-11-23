package server

import (
	"math"
	"strconv"

	mywant "mywant/engine/core"
)

// The `characters` / `things` arrays a want carries, and the label that says a
// want has them.
//
// Who is actually IN those arrays at any moment is decided by intersection.go's
// occupancy rule — this is only what it writes with, kept apart because the
// arrays are also written from elsewhere (a want created from YAML naming its
// targets by hand) and are not about standing on anything.

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
	addTargetToWant(want, targetsCharacters, characterID)
}

// addTargetToWant adds id to one of a want's target arrays, if not already in
// it. The same act for a character and for a thing — only which array differs.
func addTargetToWant(want *mywant.Want, field, id string) {
	current := targetIDsOf(want, field)
	for _, got := range current {
		if got == id {
			return
		}
	}
	want.SetCurrent(field, append(append([]string{}, current...), id))
}

// removeTargetFromWant removes id from one of a want's target arrays.
func removeTargetFromWant(want *mywant.Want, field, id string) {
	current := targetIDsOf(want, field)
	out := make([]string, 0, len(current))
	for _, got := range current {
		if got != id {
			out = append(out, got)
		}
	}
	if len(out) != len(current) {
		want.SetCurrent(field, out)
	}
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

// reconcileOccupancy makes what tiles record as stood on agree with where the
// characters are.
//
// A tile's `characters` is kept (persistent), but which tiles each character
// was on (intersectionOn) is only memory: a server started again has forgotten
// it, so a character stepping off afterwards was never seen to leave — the
// tile went on counting them, and a notepad's cover stayed open with nobody
// there. A tile moved under or away from a standing character changes nothing
// either, as only a character's own step is watched.
//
// So now and then, and as the server comes up, every recorded occupant is
// checked against the board: one standing elsewhere (or nowhere) is taken off;
// one still there is put back into memory, so leaving is noticed again.
func (s *Server) reconcileOccupancy() {
	if s.globalBuilder == nil {
		return
	}
	reconcileOccupants(s.globalBuilder.GetWants(), characterCells())
}

// reconcileOccupants is reconcileOccupancy over these wants and these cells.
func reconcileOccupants(wants []*mywant.Want, cells map[string][2]string) {
	intersectionMu.Lock()
	defer intersectionMu.Unlock()
	for _, w := range wants {
		ids := characterIDsOf(w)
		if len(ids) == 0 {
			continue
		}
		x, y := w.GetLabel(canvasLabelX), w.GetLabel(canvasLabelY)
		for _, id := range ids {
			key := mover{moverCharacter, id}.key()
			if c, ok := cells[id]; ok && c[0] == x && c[1] == y && !w.IsArchived() {
				if !containsString(intersectionOn[key], w.Metadata.ID) {
					intersectionOn[key] = append(intersectionOn[key], w.Metadata.ID)
				}
				continue
			}
			removeCharacterFromWant(w, id)
		}
	}
}

// characterCells is the cell every character on the board stands on — the
// live ones where they are, the rest where they were last seen — as the
// canvas labels write a cell.
func characterCells() map[string][2]string {
	cell := func(x, y float64) [2]string {
		return [2]string{strconv.Itoa(int(math.Round(x))), strconv.Itoa(int(math.Round(y)))}
	}
	cursorsMu.RLock()
	defer cursorsMu.RUnlock()
	out := make(map[string][2]string, len(cursors)+len(lastCursorPos))
	for id, e := range lastCursorPos {
		out[id] = cell(e.X, e.Y)
	}
	for id, e := range cursors {
		out[id] = cell(e.X, e.Y)
	}
	return out
}
