package server

import (
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
)

// GET /api/v1/memo/suggestions/{subtype}
// Returns recorded values for a subtype, most-recent first.
// Query param: limit (default 20)
func (s *Server) getMemoSuggestions(w http.ResponseWriter, r *http.Request) {
	subtype := mux.Vars(r)["subtype"]
	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 {
			limit = v
		}
	}
	suggestions := s.memoStore.Suggestions(subtype, limit)
	if suggestions == nil {
		suggestions = []string{}
	}
	s.JSONResponse(w, http.StatusOK, map[string]any{
		"subtype":     subtype,
		"suggestions": suggestions,
	})
}

// GET /api/v1/memo
// Returns all subtypes and their recorded values.
func (s *Server) getMemo(w http.ResponseWriter, r *http.Request) {
	data := s.memoStore.All()
	result := make(map[string][]string, len(data))
	for k, v := range data {
		if v == nil {
			result[k] = []string{}
		} else {
			result[k] = v
		}
	}
	s.JSONResponse(w, http.StatusOK, result)
}

// PUT /api/v1/memo
// Replaces the entire memo with the provided map[string][]string.
func (s *Server) putMemo(w http.ResponseWriter, r *http.Request) {
	var data map[string][]string
	if err := DecodeRequest(r, &data); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "invalid request body", err.Error())
		return
	}
	if err := s.memoStore.Replace(data); err != nil {
		s.JSONError(w, r, http.StatusInternalServerError, "failed to save memo", err.Error())
		return
	}
	s.JSONResponse(w, http.StatusOK, map[string]any{"message": "memo updated"})
}
