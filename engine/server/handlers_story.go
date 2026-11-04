package server

// GET /api/v1/characters/{id}/story — a character's story (story.go).
//
//	?since=  RFC 3339, or a duration back from now (1h, 30m, 2h45m); default 24h
//	?until=  RFC 3339; default now
//	?kind=   kinds, comma-separated (want.arrive,chat.answer)
//	?limit=  the last this many; default 500, at most 5000
//
// Events oldest first, as JSON, or YAML with ?format=yaml (writeDoc).

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	ws "github.com/onelittlenightmusic/want-spec"
)

const (
	storyListDefault = 500
	storyListMax     = 5000
)

// storyTime reads a since or until: RFC 3339, or a duration back from now.
func storyTime(v string, now time.Time, def time.Time) (time.Time, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return def, true
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t, true
	}
	if d, err := time.ParseDuration(v); err == nil && d > 0 {
		return now.Add(-d), true
	}
	return time.Time{}, false
}

func (s *Server) getCharacterStory(w http.ResponseWriter, r *http.Request) {
	character := mux.Vars(r)["id"]
	q := r.URL.Query()
	now := time.Now()
	since, ok := storyTime(q.Get("since"), now, now.Add(-24*time.Hour))
	if !ok {
		http.Error(w, "since: RFC 3339 or a duration (1h)", http.StatusBadRequest)
		return
	}
	until, ok := storyTime(q.Get("until"), now, now.Add(time.Second))
	if !ok {
		http.Error(w, "until: RFC 3339", http.StatusBadRequest)
		return
	}
	query := storyQuery{since: since, until: until, limit: storyListDefault}
	if n, err := strconv.Atoi(q.Get("limit")); err == nil && n > 0 {
		query.limit = min(n, storyListMax)
	}
	if k := strings.TrimSpace(q.Get("kind")); k != "" {
		query.kinds = map[string]bool{}
		for _, kind := range strings.Split(k, ",") {
			if kind = strings.TrimSpace(kind); kind != "" {
				query.kinds[kind] = true
			}
		}
	}
	events, err := story.read(character, query)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if events == nil {
		events = []ws.StoryEvent{}
	}
	writeDoc(w, r, http.StatusOK, &ws.StoryList{Events: events})
}
