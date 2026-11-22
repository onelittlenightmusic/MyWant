package server

// A character's story in figures, for the characters page's sidebar: what it
// did (each kind of event), when (the parts of the day), what it looked at
// most (want_looked_at.go), and where it goes at each part of the day
// (character_home.go: its ask ledger in characters.yaml).
//
//	GET /api/v1/characters/{id}/story/stats[?days=30]

import (
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/gorilla/mux"
)

type storyStatsWant struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
	wantLook
}

type storyStatsPart struct {
	Part   string `json:"part"`
	Events int    `json:"events"`
	Home   string `json:"home,omitempty"`
	Kind   string `json:"kind,omitempty"`
	Asks   int    `json:"asks,omitempty"`
	Of     int    `json:"of,omitempty"`
}

func (s *Server) getStoryStats(w http.ResponseWriter, r *http.Request) {
	ch := mux.Vars(r)["id"]
	if !storyCharacterOK.MatchString(ch) {
		s.JSONError(w, r, http.StatusBadRequest, "bad character", ch)
		return
	}
	days := 30
	if v, err := strconv.Atoi(r.URL.Query().Get("days")); err == nil && v > 0 && v <= 365 {
		days = v
	}
	now := time.Now()
	events, err := story.read(ch, storyQuery{since: now.AddDate(0, 0, -days), until: now.Add(time.Minute)})
	if err != nil {
		s.JSONError(w, r, http.StatusInternalServerError, "could not read the story", err.Error())
		return
	}
	kinds := map[string]int{}
	byPart := map[string]int{}
	for _, ev := range events {
		kinds[ev.Kind]++
		if t, err := time.Parse(time.RFC3339, ev.At); err == nil {
			byPart[dayPart(t)]++
		}
	}
	var parts []storyStatsPart
	for _, p := range dayParts {
		row := storyStatsPart{Part: p.Name, Events: byPart[p.Name]}
		if home, _ := homeIn(ch, p.Name, now.Location()); home.Home != "" {
			row.Home, row.Kind, row.Asks, row.Of = home.Home, home.Kind, home.Asks, home.Of
		}
		parts = append(parts, row)
	}
	var wants []storyStatsWant
	for id, l := range characterLooks(ch, now, days) {
		row := storyStatsWant{ID: id, wantLook: l}
		if want := s.findWantByIDInAll(id); want != nil {
			row.Name, row.Type = want.Metadata.Name, want.Metadata.Type
		} else {
			continue // gone from the board
		}
		wants = append(wants, row)
	}
	sort.Slice(wants, func(i, j int) bool { return wants[i].Score > wants[j].Score })
	if len(wants) > 10 {
		wants = wants[:10]
	}
	s.JSONResponse(w, http.StatusOK, map[string]any{
		"character": ch, "days": days, "events": len(events),
		"kinds": kinds, "parts": parts, "wants": wants, "now": dayPart(now),
	})
}
