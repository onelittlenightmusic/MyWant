package server

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	mywant "mywant/engine/core"
)

// Wants hearing each other (core/want_hear.go): a want's murmur reaches the
// wants that listen for it — spec.hear names its type and timings, and how
// many of the nearest wants of that type on the board it hears (default 1),
// so a want hears what is said near it. Each hearer is handed the murmur and
// woken; it answers in its Progress (SayInReply) or, if its type answers
// nothing, by its voice script told it heard. Every hearing is told to the
// pages as an event (converse), for the board to draw the two talking.

// hearAnswerWait is how long a hearer may take to run before its voice answers
// for it.
const hearAnswerWait = 60 * time.Second

// converseEvent is the event stream's name for a want heard by another.
const converseEvent = "converse"

// wantConverse is one hearing: who said it, who heard, which murmur.
type wantConverse struct {
	From     string `json:"from"`
	To       string `json:"to"`
	MurmurID string `json:"murmurId"`
	At       string `json:"at,omitempty"` // RFC3339
}

// hearMurmur hands a murmur just said to every want that hears it, and wakes
// each.
func (s *Server) hearMurmur(src *mywant.Want, murmurID, text string, out mywant.ResultHistoryEntry, labels map[string]string) {
	if src == nil || murmurID == "" || s.globalBuilder == nil {
		return
	}
	wants := s.globalBuilder.GetWants()
	srcType := src.Metadata.Type
	for _, w := range wants {
		if w.Metadata.ID == src.Metadata.ID || w.Metadata.IsSystemWant {
			continue
		}
		for _, h := range w.GetSpec().Hear {
			if h.Type != srcType || !hearsAt(h.Timings, out.Timings) {
				continue
			}
			if !amongNearest(src, w, wants, srcType, h.Nearest) {
				continue
			}
			m := mywant.HeardMurmur{
				MurmurID: murmurID, FromID: src.Metadata.ID, FromName: src.Metadata.Name, FromType: srcType,
				Timings: out.Timings, Text: text, Result: out.Result, Fields: out.Fields, Labels: labels, At: time.Now(),
			}
			mywant.DeliverHeard(w.Metadata.ID, m)
			c := wantConverse{From: src.Metadata.ID, To: w.Metadata.ID, MurmurID: murmurID, At: time.Now().Format(time.RFC3339)}
			noteConverse(c)
			broadcastSSE(converseEvent, c)
			go s.answerHeard(w.Metadata.ID, m)
			break // heard once, however many of its hear entries match
		}
	}
}

// hearsAt says whether a murmur at `at` (its timings) is one a hear entry
// listens for: any of its timings, or — none named — any murmur. The standard
// "answer" alone counts only when named.
func hearsAt(listen, at []string) bool {
	if len(listen) == 0 {
		return true
	}
	for _, l := range listen {
		for _, t := range at {
			if l == t {
				return true
			}
		}
	}
	return false
}

// amongNearest says whether src is one of the n wants of its type nearest to
// the hearer on the board (by their cells). A want with no cell is as far as
// can be.
func amongNearest(src, hearer *mywant.Want, wants []*mywant.Want, typ string, n int) bool {
	if n <= 0 {
		n = 1
	}
	hx, hy, ok := canvasCell(hearer)
	if !ok {
		return true // nowhere on the board: it hears every one
	}
	type far struct {
		id string
		d  float64
	}
	var all []far
	for _, w := range wants {
		if w.Metadata.Type != typ || w.Metadata.ID == hearer.Metadata.ID {
			continue
		}
		d := math.Inf(1)
		if x, y, ok := canvasCell(w); ok {
			d = math.Hypot(x-hx, y-hy)
		}
		all = append(all, far{w.Metadata.ID, d})
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].d < all[j].d })
	for i := 0; i < len(all) && i < n; i++ {
		if all[i].id == src.Metadata.ID {
			return true
		}
	}
	return false
}

// canvasCell is a want's cell on the board, if it has one.
func canvasCell(w *mywant.Want) (float64, float64, bool) {
	labels := w.GetLabels()
	x, errX := strconv.ParseFloat(labels["mywant.io/canvas-x"], 64)
	y, errY := strconv.ParseFloat(labels["mywant.io/canvas-y"], 64)
	return x, y, errX == nil && errY == nil
}

// answerHeard wakes the hearer, waits for it to run, and — when it said
// nothing in answer to the murmur itself — answers by its voice. What it
// heard is then let go.
func (s *Server) answerHeard(id string, m mywant.HeardMurmur) {
	defer mywant.TakeHeard(id)
	if err := s.globalBuilder.RestartWant(id); err != nil {
		return
	}
	s.robotWaitSettled(id, hearAnswerWait)
	w := s.findWantByIDInAll(id)
	if w == nil {
		return
	}
	if replied, _ := w.GetStateString(mywant.SayReplyToState, ""); replied == m.MurmurID {
		// Its Progress answered it: kept as its murmur all the same.
		said, _ := w.GetStateString("say", "")
		s.noteReply(w, m, said)
		return
	}
	line := s.heardVoice(w, m)
	if line == "" {
		return
	}
	w.StoreState(mywant.SayReplyToState, m.MurmurID)
	w.StoreState("say", line)
	s.noteReply(w, m, line)
}

// noteReply keeps an answer to a murmur as a murmur of the hearer's own, its
// labels saying why: it heard (reason), whose murmur (reply_to, from_want_id).
// Not handed on to the wants that hear the hearer: a conversation is the two.
func (s *Server) noteReply(w *mywant.Want, m mywant.HeardMurmur, line string) {
	if s.notifications == nil || line == "" {
		return
	}
	_ = s.notifications.Record(NotificationEntry{
		ID:         "notif-" + time.Now().UTC().Format("20060102T150405.000000000"),
		Message:    line,
		Kind:       "alert",
		TargetType: "want",
		TargetID:   w.Metadata.ID,
		Labels: map[string]string{
			"reason": "heard", "reply_to": m.MurmurID, "from_want_id": m.FromID,
		},
	})
}

// heardVoice is the hearer's answer by its type's voice script, told it heard
// — or, with no voice of its own, a plain word that it did.
func (s *Server) heardVoice(w *mywant.Want, m mywant.HeardMurmur) string {
	script := ""
	if s.wantTypeLoader != nil {
		script = s.wantTypeLoader.GetVoiceScript(w.GetMetadata().Type)
	}
	if script != "" {
		ev := mywant.VoiceEvent{Kind: mywant.VoiceEventHeard, Heard: &m}
		if line := mywant.WantNewsVoice(context.Background(), w, script, ev); line.Say != "" {
			return line.Say
		}
		return ""
	}
	return fmt.Sprintf("「%s」の話、聞いたよ", m.FromName)
}

// murmurLabels is what a murmur is about in set words (core MurmurLabelTime,
// MurmurLabelPlace): its voice's, else read off the want — the time a field of
// its type marked a date holds (in its answer, else its state), the place its
// first place parameter names. Nothing found, no label.
func (s *Server) murmurLabels(w *mywant.Want, out mywant.ResultHistoryEntry, voiced map[string]string) map[string]string {
	labels := map[string]string{}
	def := w.WantTypeDefinition
	if def != nil {
		for _, sd := range def.State {
			if sd.SubType != "date" && sd.SubType != "datetime" {
				continue
			}
			v, ok := out.Fields[sd.Name]
			if !ok {
				v, _ = w.GetStateString(sd.Name, "")
			}
			if at, ok := parseWhen(v); ok {
				labels[mywant.MurmurLabelTime] = at.Format(time.RFC3339)
				break
			}
		}
		for _, pk := range placeParams(def) {
			if v, _ := w.Spec.Params[pk[0]].(string); strings.TrimSpace(v) != "" {
				labels[mywant.MurmurLabelPlace] = strings.TrimSpace(v)
				break
			}
		}
	}
	for k, v := range voiced {
		labels[k] = v
	}
	if len(labels) == 0 {
		return nil
	}
	return labels
}

// whenRe finds a date and time written in a value: 2026-11-22T10:00(+09:00),
// 2026-11-22 10:00.
var whenRe = regexp.MustCompile(`\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}(?::\d{2})?(?:Z|[+-]\d{2}:\d{2})?`)

// parseWhen reads a time out of a value, in local time when it names no zone.
func parseWhen(v any) (time.Time, bool) {
	str, _ := v.(string)
	m := whenRe.FindString(str)
	if m == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04", "2006-01-02 15:04", "2006-01-02T15:04:05", "2006-01-02 15:04:05"} {
		if t, err := time.ParseInLocation(layout, m, time.Local); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// interpretHeard finds what an ask means in a murmur that carries no label for
// it (core HearInterpret): its candidates — the times written in it, or the
// places (its own named values, the hearer's place parameters) — read by Laya
// with the murmur's words, else the first of them.
func (s *Server) interpretHeard(hearer *mywant.Want, m mywant.HeardMurmur, ask string) (mywant.HeardChoice, bool) {
	cands := heardCandidates(hearer, m, ask)
	if len(cands) == 0 {
		return mywant.HeardChoice{}, false
	}
	// Asked through the same queue as every decision Laya makes (decideMany:
	// whichever worker is free takes it); no worker, the first candidate.
	if len(cands) > 1 {
		criteria := map[string]string{}
		for _, c := range cands {
			criteria[c] = c
		}
		what := map[string]string{mywant.HearWhen: "この知らせが話している時刻はどれか", mywant.HearWhere: "この知らせが話している場所はどれか"}[ask]
		if what == "" {
			what = "この知らせの「" + ask + "」はどれか"
		}
		text := fmt.Sprintf("出来事: 「%s」が知らせた。%s", m.FromName, m.Text)
		if a, _, err := (&robotDecider{}).decide(text, map[string]decisionQuestion{ask: {Type: "choice", Instructions: what, Criteria: criteria}}); err == nil {
			if ans, ok := a[ask]; ok && ans.Choice != "" {
				return mywant.HeardChoice{Choice: ans.Choice, Probabilities: ans.Probabilities}, true
			}
		}
	}
	return mywant.HeardChoice{Choice: cands[0], Probabilities: map[string]float64{cands[0]: 1}}, true
}

// heardCandidates are the values an ask could mean in a murmur, likeliest
// first: for when, every time written in its answer, fields and words; for
// where, the values of its answer's place-like keys, then the hearer's own
// place parameters.
func heardCandidates(hearer *mywant.Want, m mywant.HeardMurmur, ask string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(v string) {
		if v = strings.TrimSpace(v); v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	var walk func(v any, key string)
	switch ask {
	case mywant.HearWhen:
		walk = func(v any, _ string) {
			switch x := v.(type) {
			case string:
				if at, ok := parseWhen(x); ok {
					add(at.Format(time.RFC3339))
				}
			case map[string]any:
				for _, y := range x {
					walk(y, "")
				}
			case []any:
				for _, y := range x {
					walk(y, "")
				}
			}
		}
	case mywant.HearWhere:
		walk = func(v any, key string) {
			switch x := v.(type) {
			case string:
				if placeKey.MatchString(key) {
					add(x)
				}
			case map[string]any:
				for k, y := range x {
					walk(y, k)
				}
			case []any:
				for _, y := range x {
					walk(y, key)
				}
			}
		}
	default:
		return nil
	}
	walk(m.Result, "")
	for k, v := range m.Fields {
		walk(v, k)
	}
	if ask == mywant.HearWhen {
		walk(m.Text, "")
	}
	if ask == mywant.HearWhere && hearer != nil {
		for _, pk := range placeParams(hearer.WantTypeDefinition) {
			if v, _ := hearer.Spec.Params[pk[0]].(string); v != "" {
				add(v)
			}
		}
	}
	return out
}

// placeKey is a key whose value is a place.
var placeKey = regexp.MustCompile(`(?i)^(place|location|store|station|city|at|to|from|where|venue|area|shop)$|_(place|location|store|station|city)$`)

// hearWant handles POST /api/v1/wants/{id}/hear — body {asks: ["when",
// "where"]}: what the want heard, each murmur with what the asks mean in it
// (core Interpret: its labels first, else Laya). A plugin script's
// current().hear(*asks).
func (s *Server) hearWant(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		s.handleOptions(w, r)
		return
	}
	id := mux.Vars(r)["id"]
	var body struct {
		Asks []string `json:"asks"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	want, _, found := s.globalBuilder.FindWantByID(id)
	if !found {
		s.JSONError(w, r, http.StatusNotFound, "want not found", id)
		return
	}
	heard := want.Hear(body.Asks...)
	if heard == nil {
		heard = []mywant.HeardMurmur{}
	}
	s.JSONResponse(w, http.StatusOK, map[string]any{"heard": heard})
}
