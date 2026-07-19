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
	go broadcastSSE("character_changed", created.ID)
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
	go broadcastSSE("character_changed", id)
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
	go broadcastSSE("character_changed", id)
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
	go broadcastSSE("character_changed", id)
	s.JSONResponse(w, http.StatusOK, c)
}

// setCharacterAuraDefault marks (or, with an empty value, clears) a want's
// aura-default selection for a character.
// Body: { "wantId": "want-id", "section": "current", "key": "selected", "value": "..." }
func (s *Server) setCharacterAuraDefault(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var req struct {
		WantID  string `json:"wantId"`
		Section string `json:"section"`
		Key     string `json:"key"`
		Value   string `json:"value"`
	}
	if err := DecodeRequest(r, &req); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	if req.WantID == "" {
		s.JSONError(w, r, http.StatusBadRequest, "wantId is required", "")
		return
	}
	mark := mywant.AuraMark{Section: req.Section, Key: req.Key, Value: req.Value}
	c, ok := mywant.SetCharacterAuraDefault(id, req.WantID, mark)
	if !ok {
		s.JSONError(w, r, http.StatusNotFound, "Character not found", id)
		return
	}
	go broadcastSSE("character_changed", id)
	s.JSONResponse(w, http.StatusOK, c)
}

// setCharacterAuraCardWant sets (or, with an empty wantId, clears) the want a
// character has bookmarked as their aura card.
// Body: { "wantId": "want-id" }
func (s *Server) setCharacterAuraCardWant(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var req struct {
		WantID string `json:"wantId"`
	}
	if err := DecodeRequest(r, &req); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	c, ok := mywant.SetCharacterAuraCardWant(id, req.WantID)
	if !ok {
		s.JSONError(w, r, http.StatusNotFound, "Character not found", id)
		return
	}
	go broadcastSSE("character_changed", id)
	s.JSONResponse(w, http.StatusOK, c)
}

// setCharacterDesign sets the tile/aura design-plugin ids for a character.
// Body: { "tile_design": "forest", "aura_design": "forest" } (either may be "" = inherit).
func (s *Server) setCharacterDesign(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var req struct {
		TileDesign string `json:"tile_design"`
		AuraDesign string `json:"aura_design"`
	}
	if err := DecodeRequest(r, &req); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	c, ok := mywant.SetCharacterDesign(id, req.TileDesign, req.AuraDesign)
	if !ok {
		s.JSONError(w, r, http.StatusNotFound, "Character not found", id)
		return
	}
	go broadcastSSE("character_changed", id)
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
	go broadcastSSE("character_changed", req.DeviceIDs)
	s.JSONResponse(w, http.StatusOK, map[string]string{"message": "pruned"})
}
