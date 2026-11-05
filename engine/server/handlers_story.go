package server

// GET /api/v1/characters/{id}/story — a character's story (story.go).
//
//	?since=  RFC 3339, or a duration back from now (1h, 30m, 2h45m); default 24h
//	?until=  RFC 3339; default now
//	?kind=   kinds, comma-separated (want.arrive,chat.answer)
//	?limit=  the last this many; default 500, at most 5000
//
// Events oldest first, as JSON, or YAML with ?format=yaml (writeDoc).
//
// GET /api/v1/characters/{id}/digests — its days, one digest each
// (story_digest.go), oldest first; today's worked out as it stands.
//
//	?since=  YYYY-MM-DD, or days back from today (7d, 30d); default 7d
//	?until=  YYYY-MM-DD; default today

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

// storyDigestList is GET /characters/{id}/digests.
type storyDigestList struct {
	Digests []ws.StoryDigest `yaml:"digests" json:"digests"`
}

// storyDayOf reads a day: YYYY-MM-DD, or "<n>d" back from today.
func storyDayOf(v string, today time.Time, def string) (string, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return def, true
	}
	if _, err := time.ParseInLocation("2006-01-02", v, time.Local); err == nil {
		return v, true
	}
	if n, err := strconv.Atoi(strings.TrimSuffix(v, "d")); err == nil && strings.HasSuffix(v, "d") && n >= 0 && n <= 3660 {
		return today.AddDate(0, 0, -n).Format("2006-01-02"), true
	}
	return "", false
}

func (s *Server) getCharacterDigests(w http.ResponseWriter, r *http.Request) {
	character := mux.Vars(r)["id"]
	q := r.URL.Query()
	today := time.Now().In(time.Local)
	since, ok := storyDayOf(q.Get("since"), today, today.AddDate(0, 0, -7).Format("2006-01-02"))
	if !ok {
		http.Error(w, "since: YYYY-MM-DD or days back (7d)", http.StatusBadRequest)
		return
	}
	until, ok := storyDayOf(q.Get("until"), today, today.Format("2006-01-02"))
	if !ok {
		http.Error(w, "until: YYYY-MM-DD or days back (0d)", http.StatusBadRequest)
		return
	}
	digests, err := story.digests(character, since, until)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if digests == nil {
		digests = []ws.StoryDigest{}
	}
	writeDoc(w, r, http.StatusOK, &storyDigestList{Digests: digests})
}
