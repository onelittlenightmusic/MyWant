package server

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	mywant "mywant/engine/core"
)

// What just happened on the board, shown for a moment: one event, `moment`,
// for every page.
//
// A mark popping over a tile or a character — ♥ a connection made, ！ a value
// arriving through one, ？ something going wrong — and, as they move here, an
// effect landing on a want and a line being said. None of it is state a page
// reads: it is drawn for a second and gone, and a page that was not watching
// has missed nothing. So each moment carries what drawing it needs and no
// more,
//
//	{"moments": [{"kind": "emote", "on": "want", "id": "want-…", "mark": "bang"}, …]}
//
// and never sends a page back for the want it is about. Moments arriving
// close together are one event (momentWindow), the same mark on the same
// thing is said once, and again only after momentRepeatAfter — a want that
// fails every second is one ？ every few seconds, not a stream of them.

// momentWindow is how long moments gather before they are sent.
const momentWindow = 150 * time.Millisecond

// momentRepeatAfter is how soon the same mark on the same thing is shown
// again.
const momentRepeatAfter = 2 * time.Second

// The marks.
const (
	markHeart    = "heart"
	markBang     = "bang"
	markQuestion = "question"
)

// moment is one thing to draw. Which fields it carries is its kind's:
//
//	emote   on (want | thing | character), id, mark
//	impact  type (attack | mend | freeze | feed), from, on — the wants, by
//	        name, as the page draws them — and value (a feed's, shortened)
//	speech  by (want | character), id, text, at (unix ms), mood, and the
//	        translations already kept, by language (translate.go)
type moment struct {
	Kind         string            `json:"kind"`
	On           string            `json:"on,omitempty"`
	ID           string            `json:"id,omitempty"`
	Mark         string            `json:"mark,omitempty"`
	Type         string            `json:"type,omitempty"`
	From         string            `json:"from,omitempty"`
	Value        string            `json:"value,omitempty"`
	By           string            `json:"by,omitempty"`
	Text         string            `json:"text,omitempty"`
	Mood         string            `json:"mood,omitempty"`
	At           int64             `json:"at,omitempty"`
	Translations map[string]string `json:"translations,omitempty"`
}

// key is what makes two moments the same one.
func (m moment) key() string {
	return m.Kind + "\x00" + m.On + "\x00" + m.ID + "\x00" + m.Mark + "\x00" + m.Type + "\x00" + m.From +
		"\x00" + m.By + "\x00" + m.Text
}

// target is what it is about — a moment with none is not sent.
func (m moment) target() string {
	if m.Kind == "impact" {
		return m.On
	}
	return m.ID
}

// repeatAfter is how soon the same moment is sent again. A mark is held back
// for a while; an effect is not — a battle lands one several times a second,
// and the page spaces each kind by its own measure (impactStore) — nor is a
// line, which is said again only after it was blanked (its writer's rule).
func (m moment) repeatAfter() time.Duration {
	if m.Kind == "impact" || m.Kind == "speech" {
		return 0
	}
	return momentRepeatAfter
}

type momentHub struct {
	mu      sync.Mutex
	pending []moment
	queued  map[string]bool
	shown   map[string]time.Time // key → when it was last sent
	timer   *time.Timer
	// Seams for the tests.
	window time.Duration
	now    func() time.Time
	send   func(moments []moment)
}

func newMomentHub() *momentHub {
	return &momentHub{
		queued: map[string]bool{},
		shown:  map[string]time.Time{},
		window: momentWindow,
		now:    time.Now,
		send:   func(ms []moment) { broadcastSSE("moment", map[string]any{"moments": ms}) },
	}
}

var moments = newMomentHub()

// add queues moments for the next send: one already waiting is not queued
// twice, and one sent within momentRepeatAfter is not sent again.
func (h *momentHub) add(ms ...moment) {
	h.mu.Lock()
	defer h.mu.Unlock()
	now := h.now()
	for _, m := range ms {
		if m.target() == "" {
			continue
		}
		k := m.key()
		if h.queued[k] {
			continue
		}
		if at, ok := h.shown[k]; ok && now.Sub(at) < m.repeatAfter() {
			continue
		}
		h.queued[k] = true
		h.pending = append(h.pending, m)
	}
	if len(h.pending) > 0 && h.timer == nil {
		h.timer = time.AfterFunc(h.window, h.flush)
	}
}

func (h *momentHub) flush() {
	h.mu.Lock()
	out := h.pending
	h.pending, h.queued, h.timer = nil, map[string]bool{}, nil
	now := h.now()
	for _, m := range out {
		if m.repeatAfter() > 0 {
			h.shown[m.key()] = now
		}
	}
	// Forget what is too old to hold anything back, so the map stays the size
	// of the last few seconds.
	for k, at := range h.shown {
		if now.Sub(at) >= momentRepeatAfter {
			delete(h.shown, k)
		}
	}
	h.mu.Unlock()
	if len(out) > 0 {
		h.send(out)
	}
}

// emote is a mark over a want, a thing or a character.
func emote(on, id, mark string) moment { return moment{Kind: "emote", On: on, ID: id, Mark: mark} }

// setupMoments listens for the moments wants make, and adds the endpoint a
// page says its own through.
func (s *Server) setupMoments(api *mux.Router) {
	mywant.OnWantMoment = s.wantMoment
	api.HandleFunc("/moments", s.postMoments).Methods("POST", "OPTIONS")
}

// wantMoment turns what a want just did into the moments it is shown as.
func (s *Server) wantMoment(n *mywant.Want, key string, value, previous any) {
	id := n.Metadata.ID
	switch key {
	case mywant.MomentStatusKey:
		// Gone wrong: ？ over it.
		if st, ok := value.(mywant.WantStatus); ok && st == mywant.WantStatusFailed {
			moments.add(emote("want", id, markQuestion))
		}
	case "error":
		// An error it did not have before.
		if momentText(value) != "" && momentText(previous) == "" {
			moments.add(emote("want", id, markQuestion))
		}
	case "say":
		// A line said over its tile. A blank is the want falling silent, and
		// not a line.
		if text := strings.TrimSpace(momentText(value)); text != "" {
			moments.add(speechMoment("want", id, text, ""))
		}
	case mywant.ImpactField:
		// What it did lands on each want it reaches — drawn there (impact),
		// and a value handed on (feed) is ！ over them too.
		imp, _ := value.(map[string]any)
		kind, _ := imp["kind"].(string)
		if imp == nil || !impactKinds[kind] {
			return
		}
		val, _ := imp["value"].(string)
		for _, name := range impactTargets(imp["to"]) {
			moments.add(moment{Kind: "impact", Type: kind, From: n.Metadata.Name, On: name, Value: val})
			if kind != mywant.ImpactFeed {
				continue
			}
			if w, ok := s.findWantByNameOrID(name); ok {
				moments.add(emote("want", w.Metadata.ID, markBang))
			}
		}
	}
}

// impactKinds are the effects the page draws (components/impact/effects).
var impactKinds = map[string]bool{"attack": true, "mend": true, "freeze": true, mywant.ImpactFeed: true}

// impactTargets is an impact's `to`, as names.
func impactTargets(v any) []string {
	switch t := v.(type) {
	case []string:
		return t
	case []any:
		out := make([]string, 0, len(t))
		for _, x := range t {
			if s, ok := x.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// momentText is a state value as words, "" for none.
func momentText(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

// findWantByNameOrID is the running want called name.
func (s *Server) findWantByNameOrID(name string) (*mywant.Want, bool) {
	if s.globalBuilder == nil {
		return nil, false
	}
	if w, ok := s.globalBuilder.FindWantByName(name); ok {
		return w, true
	}
	if w, _, ok := s.globalBuilder.FindWantByID(name); ok {
		return w, true
	}
	return nil, false
}

// speechMoment is a line said, with the translations already kept for it
// in the languages people read the board in — one not kept yet is looked up
// by the page later (translations/lookup), never waited for here.
func speechMoment(by, id, text, mood string) moment {
	m := moment{Kind: "speech", By: by, ID: id, Text: text, Mood: mood, At: time.Now().UnixMilli()}
	if translations != nil {
		for _, lang := range translations.languages() {
			if t := translated(text, lang); t != text {
				if m.Translations == nil {
					m.Translations = map[string]string{}
				}
				m.Translations[lang] = t
			}
		}
	}
	return m
}

// connectedMoments: a connection made — ♥ over both ends, and over the
// character who made it when the page said who that was.
func connectedMoments(sourceWant, sourceThing, targetWant, characterID string) {
	var ms []moment
	if sourceWant != "" {
		ms = append(ms, emote("want", sourceWant, markHeart))
	}
	if sourceThing != "" {
		ms = append(ms, emote("thing", sourceThing, markHeart))
	}
	if targetWant != "" {
		ms = append(ms, emote("want", targetWant, markHeart))
	}
	if characterID != "" {
		ms = append(ms, emote("character", characterID, markHeart))
	}
	moments.add(ms...)
}

// postMoments is POST /api/v1/moments — a mark a page knows of first (a
// character's own press that came to nothing), said to every page:
//
//	{"moments": [{"kind": "emote", "on": "character", "id": "chr-…", "mark": "question"}]}
//
// Only marks: an effect or a line is the engine's to say.
func (s *Server) postMoments(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Moments []moment `json:"moments"`
	}
	if err := DecodeRequest(r, &req); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "Invalid request", err.Error())
		return
	}
	var ok []moment
	for _, m := range req.Moments {
		if m.Kind != "emote" || (m.Mark != markHeart && m.Mark != markBang && m.Mark != markQuestion) ||
			(m.On != "want" && m.On != "thing" && m.On != "character") {
			continue
		}
		ok = append(ok, emote(m.On, m.ID, m.Mark))
	}
	moments.add(ok...)
	s.JSONResponse(w, http.StatusOK, map[string]int{"accepted": len(ok)})
}
