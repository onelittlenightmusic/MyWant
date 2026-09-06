package server

import (
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
