package server

import (
	"net/http"
	"strconv"

	mywant "mywant/engine/core"

	"github.com/gorilla/mux"
)

// What a character chose from a keyboard's suggestions for what they typed
// (pick_store.go): the keyboards send each choice, and read the newest back
// to offer the same choice again and to show Apple FM how this person
// chooses.
//
//	POST   /api/v1/characters/{id}/picks   body: {"picks": [Pick…]}
//	GET    /api/v1/characters/{id}/picks?limit=200   newest first
//	DELETE /api/v1/characters/{id}/picks

type addPicksRequest struct {
	Picks []mywant.Pick `json:"picks"`
}

func (s *Server) addCharacterPicks(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	if _, ok := mywant.GetCharacter(id); !ok {
		s.JSONError(w, r, http.StatusNotFound, "Character not found", id)
		return
	}
	var req addPicksRequest
	if err := DecodeRequest(r, &req); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	var picks []mywant.Pick
	for _, p := range req.Picks {
		// A choice is a choice of something: letters kept as typed are not sent.
		if p.Typed != "" && p.Chose.Name != "" && p.Chose.Text != "" {
			picks = append(picks, p)
		}
	}
	if len(picks) == 0 {
		s.JSONError(w, r, http.StatusBadRequest, "no picks", "each pick needs typed, chose.name and chose.text")
		return
	}
	if err := mywant.AddPicks(id, picks); err != nil {
		s.JSONError(w, r, http.StatusInternalServerError, "Could not record picks", err.Error())
		return
	}
	s.JSONResponse(w, http.StatusCreated, map[string]any{"added": len(picks)})
}

func (s *Server) listCharacterPicks(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	if _, ok := mywant.GetCharacter(id); !ok {
		s.JSONError(w, r, http.StatusNotFound, "Character not found", id)
		return
	}
	limit := 200
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 {
		limit = v
	}
	picks, err := mywant.ListPicks(id, limit)
	if err != nil {
		s.JSONError(w, r, http.StatusInternalServerError, "Could not read picks", err.Error())
		return
	}
	if picks == nil {
		picks = []mywant.Pick{}
	}
	s.JSONResponse(w, http.StatusOK, map[string]any{"picks": picks, "count": len(picks)})
}

func (s *Server) clearCharacterPicks(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	if _, ok := mywant.GetCharacter(id); !ok {
		s.JSONError(w, r, http.StatusNotFound, "Character not found", id)
		return
	}
	if err := mywant.ClearPicks(id); err != nil {
		s.JSONError(w, r, http.StatusInternalServerError, "Could not clear picks", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
