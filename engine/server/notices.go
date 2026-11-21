package server

// Notices rated: a want's notice, shown as the device's own notification
// (the app, the browser extension), carries 👍 and 👎. A rating is what the
// person said of it — worth telling (news) or not (routine) — and is kept as
// the label of the decision Laya made about it (robot-initiative-labels.jsonl,
// by: person), so it is learnt from when the robot is retrained, with the
// labels its chat and the notices opened give (robot_initiative_learn.go).
// A notice Laya was not asked about is entered as a decision of its own, so
// its rating still teaches.
//
// A 👎 may come with an offer to mute what said it: its timing, on that want
// (spec.voice.notify without it) — when it is not the first 👎 for that timing
// lately with no 👍 since, or when Laya had already thought it routine.
//
//	POST /api/v1/notices/{id}/rate {"rating": "up" | "down"}
//	  → {label, suggestMute?: {wantId, want, timings, text}}
//	POST /api/v1/notices/{id}/mute → {wantId, notify}

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	mywant "mywant/engine/core"
)

// noticeRecord is a notice as it was told, kept to be rated.
type noticeRecord struct {
	ID       string            `json:"id"`
	At       string            `json:"at"`
	WantID   string            `json:"wantId"`
	Want     string            `json:"want"`
	WantType string            `json:"wantType,omitempty"`
	Text     string            `json:"text"`
	Moment   string            `json:"moment,omitempty"`
	Timings  []string          `json:"timings,omitempty"`
	Labels   map[string]string `json:"labels,omitempty"`
	News     *decisionAnswer   `json:"news,omitempty"` // what Laya thought of it, when asked
}

// noticeRating is one rating given.
type noticeRating struct {
	ID      string   `json:"id"` // the notice's
	At      string   `json:"at"`
	WantID  string   `json:"wantId"`
	Timings []string `json:"timings,omitempty"`
	Rating  string   `json:"rating"` // up | down
}

// muteOfferAfter is how far back a 👎 for the same timing counts towards
// offering to mute it.
const muteOfferAfter = 30 * 24 * time.Hour

var noticesMu sync.Mutex

func noticesFile() string       { return initiativeHomeFile("notices.jsonl") }
func noticeRatingsFile() string { return initiativeHomeFile("notice-ratings.jsonl") }

func noteNotice(n noticeRecord) {
	if n.At == "" {
		n.At = time.Now().Format(time.RFC3339)
	}
	noticesMu.Lock()
	defer noticesMu.Unlock()
	_ = appendJSONLine(noticesFile(), n)
}

func findNotice(id string) (noticeRecord, bool) {
	var found noticeRecord
	ok := false
	readJSONLines(noticesFile(), func(n noticeRecord) {
		if n.ID == id {
			found, ok = n, true
		}
	})
	return found, ok
}

// timingReasonLabels say why an answer was told: at which timings, the
// moment in its type's words, what made them come, and the answer.
func timingReasonLabels(w *mywant.Want, out mywant.ResultHistoryEntry, tell mywant.Telling, moment string) map[string]string {
	labels := map[string]string{"reason": "timing", "event": "output"}
	timings := tell.Timings
	if len(timings) == 0 {
		timings = out.Timings
	}
	if len(timings) > 0 {
		labels["timings"] = strings.Join(timings, ",")
	}
	if moment != "" {
		labels["trigger"] = moment
	}
	if c := w.TimingConditions(timings); len(c) > 0 {
		labels["condition"] = strings.Join(c, ",")
	}
	if out.ID != "" {
		labels["output_id"] = out.ID
	}
	return labels
}

// rateNotice handles POST /api/v1/notices/{id}/rate.
func (s *Server) rateNotice(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		s.handleOptions(w, r)
		return
	}
	id := mux.Vars(r)["id"]
	var body struct {
		Rating string `json:"rating"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || (body.Rating != "up" && body.Rating != "down") {
		s.JSONError(w, r, http.StatusBadRequest, `want {"rating": "up" | "down"}`, "")
		return
	}
	n, ok := findNotice(id)
	if !ok {
		s.JSONError(w, r, http.StatusNotFound, "notice not found", id)
		return
	}
	gold := "news"
	if body.Rating == "down" {
		gold = "routine"
	}
	decision := s.noticeDecisionID(n)
	l := initiativeLabel{ID: decision, At: time.Now().Format(time.RFC3339), Gold: map[string]string{newsQuestionID: gold},
		By: "person", Reason: "the person rated the notice " + body.Rating}
	noticesMu.Lock()
	err := appendJSONLine(initiativeHomeFile("robot-initiative-labels.jsonl"), l)
	if err == nil {
		err = appendJSONLine(noticeRatingsFile(), noticeRating{ID: id, At: l.At, WantID: n.WantID, Timings: n.Timings, Rating: body.Rating})
	}
	noticesMu.Unlock()
	if err != nil {
		s.JSONError(w, r, http.StatusInternalServerError, "failed to keep the rating", err.Error())
		return
	}
	// Taught with the rest, once enough have gathered (as the chat's are).
	if ri := s.initiative; ri != nil {
		go s.robotMaybeRetrain(ri)
	}
	resp := map[string]any{"label": l}
	if body.Rating == "down" {
		if offer := s.muteOffer(n); offer != nil {
			resp["suggestMute"] = offer
		}
	}
	s.JSONResponse(w, http.StatusOK, resp)
}

// noticeDecisionID is the decision a rating of the notice labels: the one the
// robot recorded for it, else one entered now (its words, the news question).
func (s *Server) noticeDecisionID(n noticeRecord) string {
	logPath := initiativeHomeFile("robot-initiative.jsonl")
	if ri := s.initiative; ri != nil {
		logPath = ri.logPath
	}
	found := ""
	readJSONLines(logPath, func(rec initiativeRecord) {
		if rec.NoticeID == n.ID && rec.ID != "" && rec.State != "" {
			found = rec.ID
		}
	})
	if found != "" {
		return found
	}
	rec := initiativeRecord{
		ID: fmt.Sprintf("init-%d", time.Now().UnixNano()), WantID: n.WantID, At: n.At,
		Event: "rated", Want: n.Want, WantType: n.WantType, State: outputNewsText(n.Want, n.Text, n.Moment),
		By: "person", Did: "none", Push: "sent", NoticeID: n.ID,
		Questions: map[string]decisionQuestion{newsQuestionID: newsQuestion},
	}
	if n.News != nil {
		rec.Answers = map[string]decisionAnswer{newsQuestionID: *n.News}
	}
	_ = appendJSONLine(logPath, rec)
	return rec.ID
}

// muteOffer is the offer to mute what said the notice — its timings on that
// want — after a 👎: when another 👎 for them came lately with no 👍 since,
// or Laya had already thought it routine. Nil when there is nothing to offer
// (no timing, or the want no longer notifies at them).
func (s *Server) muteOffer(n noticeRecord) map[string]any {
	if len(n.Timings) == 0 {
		return nil
	}
	want, _, found := s.globalBuilder.FindWantByID(n.WantID)
	if !found {
		return nil
	}
	notifying := map[string]bool{}
	for _, t := range want.NotifyTimings() {
		notifying[t] = true
	}
	var timings []string
	for _, t := range n.Timings {
		if notifying[t] {
			timings = append(timings, t)
		}
	}
	if len(timings) == 0 {
		return nil
	}
	key := strings.Join(timings, ",")
	downs := recentDowns(n.WantID, timings, time.Now())
	layaRoutine := n.News != nil && n.News.Choice == "routine"
	if downs < 2 && !layaRoutine {
		return nil
	}
	moment := strings.TrimSpace(want.TimingMoments(timings))
	what := "「" + key + "」のとき"
	switch {
	case moment != "":
		what = moment
	case key == "answer": // the standard timing: every new answer
		what = "新しい結果のとき"
	}
	return map[string]any{
		"wantId": n.WantID, "want": n.Want, "timings": timings,
		"text": fmt.Sprintf("%s：%sの通知をミュートしますか？", n.Want, what),
	}
}

// recentDowns is how many 👎 the want's notices at these timings have had
// lately (muteOfferAfter) since the last 👍.
func recentDowns(wantID string, timings []string, now time.Time) int {
	since := now.Add(-muteOfferAfter)
	downs := 0
	readJSONLines(noticeRatingsFile(), func(r noticeRating) {
		at, err := time.Parse(time.RFC3339, r.At)
		if err != nil || at.Before(since) || r.WantID != wantID || !sharesTiming(r.Timings, timings) {
			return
		}
		if r.Rating == "up" {
			downs = 0 // a 👍 since: start again
		} else {
			downs++
		}
	})
	return downs
}

func sharesTiming(a, b []string) bool {
	for _, x := range a {
		for _, y := range b {
			if x == y {
				return true
			}
		}
	}
	return false
}

// muteNotice handles POST /api/v1/notices/{id}/mute: the want stops notifying
// at the notice's timings (spec.voice.notify without them). Its murmurs stay.
func (s *Server) muteNotice(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		s.handleOptions(w, r)
		return
	}
	id := mux.Vars(r)["id"]
	n, ok := findNotice(id)
	if !ok || len(n.Timings) == 0 {
		s.JSONError(w, r, http.StatusNotFound, "notice not found, or said at no timing", id)
		return
	}
	want, _, found := s.globalBuilder.FindWantByID(n.WantID)
	if !found {
		s.JSONError(w, r, http.StatusNotFound, "want not found", n.WantID)
		return
	}
	voice := want.MutedNotify(n.Timings)
	spec := want.GetSpec()
	if spec == nil {
		spec = &mywant.WantSpec{}
	}
	spec.Voice = voice
	// Applied as an edit of the want is (commitWantUpdate), state kept.
	updated := &mywant.Want{Metadata: want.GetMetadata(), Spec: *spec}
	mywant.StoreStateMulti(updated, want.GetAllStateDeep())
	updated.SetChangeBy(mywant.NewRequestID("q"))
	s.globalBuilder.UpdateWant(updated)
	s.globalBuilder.TriggerSave()
	boardWantsChanged(want.Metadata.ID)
	notify := append([]string{}, *voice.Notify...)
	sort.Strings(notify)
	s.JSONResponse(w, http.StatusOK, map[string]any{"wantId": n.WantID, "notify": notify})
}
