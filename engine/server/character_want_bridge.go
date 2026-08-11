package server

import (
	mywant "mywant/engine/core"
)

// ── Characters that are played by a want ─────────────────────────────────────
//
// The robot is two things at once, and both are real. It is a character in the
// registry — so it can be called, ridden, spoken to, and drawn with an avatar
// and a colour — and it is a want on the canvas: the coding agent, which walks
// around under its own steam and whose card is its chat UI. They are two views
// of one thing, bound together by the character's AuraCardWantID.
//
// What the canvas draws for it is the want. There is no cursor for the robot
// and there cannot be one: a cursor is presence, something a live browser keeps
// republishing (see cursorTTL), and the robot has no browser. So an instruction
// aimed at the *character* has to land on the *want*, or it lands nowhere —
// which is exactly what calling the robot used to do. It wrote a position into
// gui_state that nothing on screen was reading.
//
// This is that translation, and it lives here rather than in the GUI because
// every caller deserves it without knowing the secret: the canvas action
// bubble, `mywant-gui i take`, and any agent driving the same state keys all
// speak to the character and all reach the want.

// characterWantTypes are the want types that ARE a character rather than a
// thing on the board. Mirrors isCharacterWant() in
// web/src/components/dashboard/canvasTileGeometry.ts, which is what makes the
// canvas draw these as a free-floating avatar token instead of a tile — keep
// the two in step.
var characterWantTypes = map[string]bool{
	"robot": true,
}

// applyCharacterCursorToWant turns "this character is now at (x, y)" into a
// move of the want that plays them, for the characters that have one.
//
// Driven off the x key and reading y beside it in the same merge: the two are
// always written together (they describe one cell), and taking them one at a
// time would move the want to a half-updated position.
//
// The unsuffixed canvas_cursor_x / canvas_cursor_y are deliberately not
// matched: those are the CursorMan robot cursor (`mywant-gui i set`), which is
// not a character.
func (s *Server) applyCharacterCursorToWant(updates map[string]any) {
	for key, val := range updates {
		charID, ok := trimCursorKeyPrefix(key, cursorStateXPrefix)
		if !ok {
			continue
		}
		x, okX := cursorCoord(val)
		y, okY := cursorCoord(updates[cursorStateYPrefix+charID])
		if !okX || !okY {
			continue
		}
		// Somebody is publishing a live cursor for this character: they are a
		// player with a tab open, this write is that tab reporting where it
		// already is, and there is no want standing in for them. Nothing to do.
		if hasLiveCursor(charID) {
			continue
		}
		s.moveCharacterWant(charID, x, y)
	}
}

// moveCharacterWant places the want that plays characterID at (x, y). No-op
// unless the character is bound to a want and that want is one of the types
// that represents a character — an aura card can be any want a player has
// starred, and dragging someone's weather tile across the board because they
// were called would be a surprise.
func (s *Server) moveCharacterWant(characterID string, x, y float64) {
	c, ok := mywant.GetCharacter(characterID)
	if !ok || c.AuraCardWantID == "" || s.globalBuilder == nil {
		return
	}
	want, _, found := s.globalBuilder.FindWantByID(c.AuraCardWantID)
	if !found || want == nil || !characterWantTypes[want.Metadata.Type] {
		return
	}
	if want.Metadata.Labels == nil {
		want.Metadata.Labels = map[string]string{}
	}
	nx, ny := formatCanvasCoord(x), formatCanvasCoord(y)
	if want.Metadata.Labels[canvasLabelX] == nx && want.Metadata.Labels[canvasLabelY] == ny {
		return // already there; don't churn a save and a broadcast
	}
	want.Metadata.Labels[canvasLabelX] = nx
	want.Metadata.Labels[canvasLabelY] = ny
	s.globalBuilder.UpdateWant(want)
	s.globalBuilder.TriggerSave()
	go broadcastSSE("want_changed", []string{want.Metadata.ID})
}
