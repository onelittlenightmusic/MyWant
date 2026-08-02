package server

import (
	"net/http"
	"sync"
	"time"

	mywant "mywant/engine/core"

	"github.com/gorilla/mux"
)

// Kata progress is derived, not stored, so nothing notices a kata being
// completed until something evaluates it. A want placed on any page can finish
// one, and the user may well be looking at a different section — so the server
// re-evaluates shortly after wants change and announces what became held over
// the same SSE hub every other section uses. Clients then mark their menu
// without polling.
var (
	kataAnnounceMu    sync.Mutex
	kataAnnounceTimer *time.Timer
)

// kataAnnounceDelay lets a burst of want changes settle into one evaluation.
const kataAnnounceDelay = 1500 * time.Millisecond

// ScheduleKataAnnounce re-evaluates kata after a short quiet period and
// broadcasts "kata_held" with the ids of any that became held. Safe to call on
// every want change; repeated calls collapse into one evaluation.
func (s *Server) ScheduleKataAnnounce() {
	kataAnnounceMu.Lock()
	defer kataAnnounceMu.Unlock()
	if kataAnnounceTimer != nil {
		kataAnnounceTimer.Stop()
	}
	kataAnnounceTimer = time.AfterFunc(kataAnnounceDelay, func() {
		_, _, newly := s.evaluateKataTracking()
		if len(newly) == 0 {
			return
		}
		broadcastSSE("kata_held", newly)
	})
}

// listKata returns every belt and every kata with its live standing.
func (s *Server) listKata(w http.ResponseWriter, r *http.Request) {
	levels, kata := s.evaluateKata()
	s.JSONResponse(w, http.StatusOK, map[string]any{
		"levels": levels,
		"kata":   kata,
		"count":  len(kata),
	})
}

// getKata returns a single kata's standing.
func (s *Server) getKata(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	_, kata := s.evaluateKata()
	for _, k := range kata {
		if k.KataID == id {
			s.JSONResponse(w, http.StatusOK, k)
			return
		}
	}
	s.JSONError(w, r, http.StatusNotFound, "Kata not found", id)
}

// listKataRecords returns the practice log — every time a kata was 極まった.
func (s *Server) listKataRecords(w http.ResponseWriter, r *http.Request) {
	records := mywant.ListKataRecords()
	if records == nil {
		records = []mywant.KataRecord{}
	}
	s.JSONResponse(w, http.StatusOK, map[string]any{
		"records": records,
		"count":   len(records),
	})
}
