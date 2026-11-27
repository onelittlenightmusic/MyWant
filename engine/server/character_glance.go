package server

// character_glance.go — a character at a glance, as a small widget shows them.
//
// GET /api/v1/characters/{id}/glance
//
//	{id, name, avatar, color, mood, intensity, place, line, lineAt, lineKind}
//
// Who they are, how they feel (character_memory.go), where they stand — the
// name of what is on their cell, a want before a thing — and the last thing
// they said; with nothing said since the server started (what is said is kept
// in memory, speech_log.go), the newest thing they remember hearing, as
// heard. All of it is theirs: nothing here is about the board beyond what
// they stand on and what they heard.

import (
	"net/http"
	"time"

	mywant "mywant/engine/core"

	"github.com/gorilla/mux"
)

// CharacterGlance is a character at a glance.
type CharacterGlance struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Avatar    string  `json:"avatar"`
	Color     string  `json:"color"`
	Mood      string  `json:"mood"`
	Intensity float64 `json:"intensity"`
	// Place: what they stand on, by name ("" when nothing, or unknown);
	// Near: true when it is beside them rather than under them.
	Place string `json:"place,omitempty"`
	Near  bool   `json:"near,omitempty"`
	// Line: the last thing they said, or (LineKind "heard") the newest thing
	// they remember hearing. LineAt: when, RFC3339.
	Line     string `json:"line,omitempty"`
	LineAt   string `json:"lineAt,omitempty"`
	LineKind string `json:"lineKind,omitempty"`
}

// getCharacterGlance handles GET /api/v1/characters/{id}/glance.
func (s *Server) getCharacterGlance(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	c, ok := mywant.GetCharacter(id)
	if !ok {
		s.JSONError(w, r, http.StatusNotFound, "Character not found", id)
		return
	}
	g := CharacterGlance{ID: c.ID, Name: c.Name, Avatar: c.Avatar, Color: c.Color}
	m := memories.get(c.ID)
	g.Mood, g.Intensity = m.Mood, m.Intensity
	g.Place, g.Near = s.placeOf(*c)
	if e, ok := lastSpeechOf(c.ID); ok {
		g.Line, g.LineAt, g.LineKind = e.Text, time.UnixMilli(e.At).Format(time.RFC3339), "said"
		if e.Kind != "" {
			g.LineKind = e.Kind
		}
	} else if len(m.Memories) > 0 {
		g.Line, g.LineAt, g.LineKind = m.Memories[0].Say, m.Memories[0].At, "heard"
	}
	s.JSONResponse(w, http.StatusOK, g)
}

// placeOf is the name of what a character stands on — a want before a thing —
// else of what is beside them.
func (s *Server) placeOf(c mywant.Character) (string, bool) {
	if s.globalBuilder == nil {
		return "", false
	}
	x, y, ok := characterCell(c, s.globalBuilder.GetWants())
	if !ok {
		return "", false
	}
	tiles, err := s.robotTiles()
	if err != nil {
		return "", false
	}
	best := func(near bool) string {
		name := ""
		for _, t := range tiles {
			if characterWantTypes[t.typ] {
				continue // a person's mouth, not a place
			}
			d := absInt(t.x-x) + absInt(t.y-y)
			if (!near && d != 0) || (near && (d == 0 || absInt(t.x-x) > 1 || absInt(t.y-y) > 1)) {
				continue
			}
			if t.cardKind == "want" {
				return t.name
			}
			if name == "" {
				name = t.name
			}
		}
		return name
	}
	if on := best(false); on != "" {
		return on, false
	}
	if by := best(true); by != "" {
		return by, true
	}
	return "", false
}

// lastSpeechOf is the last thing a character said, if any is kept.
func lastSpeechOf(id string) (SpeechEntry, bool) {
	speechMu.RLock()
	defer speechMu.RUnlock()
	for i := len(speechLog) - 1; i >= 0; i-- {
		if speechLog[i].CharacterID == id {
			return speechLog[i], true
		}
	}
	return SpeechEntry{}, false
}
