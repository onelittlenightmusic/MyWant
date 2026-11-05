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
	"sort"
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

// GET /api/v1/patterns — the kinds of question asked, and which way of
// answering each goes well (story_ledger.go).
//
//	?q=          a question: the kinds it is one of (else the most like it)
//	?character=  only this character's answers
//	?limit=      at most this many kinds
//
// A kind a want type names in a worked example (robot-qa/…) says so
// (declared); one none names is a worked example to add.
func (s *Server) getStoryPatterns(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	query := storyPatternQuery{question: q.Get("q"), character: q.Get("character"), qas: s.storyQAs()}
	if n, err := strconv.Atoi(q.Get("limit")); err == nil && n > 0 {
		query.limit = n
	}
	patterns := story.patterns(query)
	if patterns == nil {
		patterns = []storyPatternStat{}
	}
	writeDoc(w, r, http.StatusOK, &struct {
		Patterns []storyPatternStat `yaml:"patterns" json:"patterns"`
	}{patterns})
}

// storyQAs is every want type's worked examples (robot-qa/…), as kinds of
// question — read now, so an example added or changed applies to every
// answer already kept.
func (s *Server) storyQAs() []storyQA {
	types, err := s.robotWantTypes()
	if err != nil {
		return nil
	}
	var out []storyQA
	for _, t := range types {
		var keys []string
		for k := range t.Labels {
			if strings.HasPrefix(k, robotQALabel) {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		for _, k := range keys {
			req := strings.Trim(strings.TrimSpace(strings.TrimPrefix(k, robotQALabel)), "「」")
			if req != "" {
				out = append(out, newStoryQA(t.Name, req, strings.Join(strings.Fields(t.Labels[k]), " ")))
			}
		}
	}
	return out
}
