package server

import (
	"net/http"
	"time"

	"github.com/gorilla/mux"

	mywant "mywant/engine/core"
)

// A want's answers — its result history, the pages its card turns back
// through and the balls beside its tile — read and edited by hand
// (core/result_history_crud.go). Each is named by its key: its id, else when
// it came (as the GUI's outputEntryKey names it).
//
//	GET    /api/v1/wants/{id}/results          — all, oldest first
//	POST   /api/v1/wants/{id}/results          — add one {result, fields?, about?, timestamp?}
//	GET    /api/v1/wants/{id}/results/{key}    — one
//	PUT    /api/v1/wants/{id}/results/{key}    — change one {result?, fields?, about?}
//	DELETE /api/v1/wants/{id}/results/{key}    — let one go (not brought back)

type resultHistoryBody struct {
	Result    any            `json:"result"`
	Fields    map[string]any `json:"fields"`
	About     *string        `json:"about"`
	Timestamp *time.Time     `json:"timestamp"`
}

func (s *Server) resultHistoryWant(w http.ResponseWriter, r *http.Request) *mywant.Want {
	id := mux.Vars(r)["id"]
	want := s.findWantByIDInAll(id)
	if want == nil {
		s.JSONError(w, r, http.StatusNotFound, "no such want", id)
	}
	return want
}

func (s *Server) listResultHistory(w http.ResponseWriter, r *http.Request) {
	want := s.resultHistoryWant(w, r)
	if want == nil {
		return
	}
	list := want.ResultHistoryEntries()
	if list == nil {
		list = []mywant.ResultHistoryEntry{}
	}
	s.JSONResponse(w, http.StatusOK, map[string]any{"results": list})
}

func (s *Server) getResultHistory(w http.ResponseWriter, r *http.Request) {
	want := s.resultHistoryWant(w, r)
	if want == nil {
		return
	}
	key := mux.Vars(r)["key"]
	for _, e := range want.ResultHistoryEntries() {
		if mywant.ResultHistoryKey(e) == key {
			s.JSONResponse(w, http.StatusOK, e)
			return
		}
	}
	s.JSONError(w, r, http.StatusNotFound, "no such answer", key)
}

func (s *Server) addResultHistory(w http.ResponseWriter, r *http.Request) {
	want := s.resultHistoryWant(w, r)
	if want == nil {
		return
	}
	var body resultHistoryBody
	if err := DecodeRequest(r, &body); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "invalid request body", err.Error())
		return
	}
	if body.Result == nil {
		s.JSONError(w, r, http.StatusBadRequest, "result is required", "")
		return
	}
	e := mywant.ResultHistoryEntry{Result: body.Result, Fields: body.Fields}
	if body.About != nil {
		e.About = *body.About
	}
	if body.Timestamp != nil {
		e.Timestamp = *body.Timestamp
	}
	s.JSONResponse(w, http.StatusCreated, want.AddResultHistory(e))
}

func (s *Server) updateResultHistory(w http.ResponseWriter, r *http.Request) {
	want := s.resultHistoryWant(w, r)
	if want == nil {
		return
	}
	var body resultHistoryBody
	if err := DecodeRequest(r, &body); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "invalid request body", err.Error())
		return
	}
	key := mux.Vars(r)["key"]
	e, ok := want.UpdateResultHistory(key, func(e *mywant.ResultHistoryEntry) {
		if body.Result != nil {
			e.Result = body.Result
		}
		if body.Fields != nil {
			e.Fields = body.Fields
		}
		if body.About != nil {
			e.About = *body.About
		}
		if body.Timestamp != nil {
			e.Timestamp = *body.Timestamp
		}
	})
	if !ok {
		s.JSONError(w, r, http.StatusNotFound, "no such answer", key)
		return
	}
	s.JSONResponse(w, http.StatusOK, e)
}

func (s *Server) deleteResultHistory(w http.ResponseWriter, r *http.Request) {
	want := s.resultHistoryWant(w, r)
	if want == nil {
		return
	}
	key := mux.Vars(r)["key"]
	if !want.DeleteResultHistory(key) {
		s.JSONError(w, r, http.StatusNotFound, "no such answer", key)
		return
	}
	s.JSONResponse(w, http.StatusOK, map[string]any{"deleted": key})
}
