package server

import (
	"net/http"

	mywant "mywant/engine/core"

	"github.com/gorilla/mux"
)

// listCharacters returns all characters.
func (s *Server) listCharacters(w http.ResponseWriter, r *http.Request) {
	characters := mywant.ListCharacters()
	if characters == nil {
		characters = []mywant.Character{}
	}
	s.JSONResponse(w, http.StatusOK, map[string]any{
		"characters": characters,
		"count":      len(characters),
	})
}

// getCharacter returns a single character by ID.
func (s *Server) getCharacter(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	c, ok := mywant.GetCharacter(id)
	if !ok {
		s.JSONError(w, r, http.StatusNotFound, "Character not found", id)
		return
	}
	s.JSONResponse(w, http.StatusOK, c)
}

// createCharacter adds a new character.
func (s *Server) createCharacter(w http.ResponseWriter, r *http.Request) {
	var c mywant.Character
	if err := DecodeRequest(r, &c); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	if c.Name == "" {
		s.JSONError(w, r, http.StatusBadRequest, "name is required", "")
		return
	}
	created := mywant.AddCharacter(c)
	s.JSONResponse(w, http.StatusCreated, created)
}

// updateCharacter replaces a character's editable fields (name, avatar, color).
// AssignedDeviceIDs are preserved server-side and not overwritten by this endpoint.
func (s *Server) updateCharacter(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var updated mywant.Character
	if err := DecodeRequest(r, &updated); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	if !mywant.UpdateCharacter(id, updated) {
		s.JSONError(w, r, http.StatusNotFound, "Character not found", id)
		return
	}
	c, _ := mywant.GetCharacter(id)
	s.JSONResponse(w, http.StatusOK, c)
}

// deleteCharacter removes a character. Assigned devices become unassigned automatically
// because AssignedDeviceIDs are stored on the character record.
func (s *Server) deleteCharacter(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	if !mywant.DeleteCharacter(id) {
		s.JSONError(w, r, http.StatusNotFound, "Character not found", id)
		return
	}
	s.JSONResponse(w, http.StatusOK, map[string]string{"message": "deleted", "id": id})
}

// assignDevicesToCharacter sets the device assignment for a character atomically.
// Any device in the request body is removed from its previous character first.
// Body: { "deviceIds": ["device-uuid-1", ...] }
func (s *Server) assignDevicesToCharacter(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var req struct {
		DeviceIDs []string `json:"deviceIds"`
	}
	if err := DecodeRequest(r, &req); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	if req.DeviceIDs == nil {
		req.DeviceIDs = []string{}
	}
	c, ok := mywant.AssignDevicesToCharacter(id, req.DeviceIDs)
	if !ok {
		s.JSONError(w, r, http.StatusNotFound, "Character not found", id)
		return
	}
	s.JSONResponse(w, http.StatusOK, c)
}

// pruneCharacterDevices removes stale device IDs from all character assignments.
// Called by the frontend's device heartbeat cleanup path.
// Body: { "deviceIds": ["stale-device-id", ...] }
func (s *Server) pruneCharacterDevices(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DeviceIDs []string `json:"deviceIds"`
	}
	if err := DecodeRequest(r, &req); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	mywant.PruneCharacterDevices(req.DeviceIDs)
	s.JSONResponse(w, http.StatusOK, map[string]string{"message": "pruned"})
}
