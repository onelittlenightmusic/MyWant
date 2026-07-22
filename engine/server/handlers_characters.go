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

// setCharacterAuraDefault marks (or, with an empty value, clears) an aura mark
// for a character. It accepts either of two shapes:
//
//   - BINDING (want-field shorthand): { wantId, section, key, value, mode }.
//     The caller names the want it is marking; what gets stored is that want's
//     *type* plus section/key — an address that means the same thing in any
//     install. The instance ID is only used to look the type up here.
//
//   - DEFINITION (explicit target): { target: {kind, name, path}, value }.
//     value may be an object — the definition the target's name resolves to
//     (e.g. a place → {lat, lng, radius}). Nothing is applied to a want.
//
// value is decoded as an arbitrary JSON value so a definition can carry an
// object while a binding carries a scalar. An empty value clears the mark.
func (s *Server) setCharacterAuraDefault(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var req struct {
		// binding shorthand
		WantID  string `json:"wantId"`
		Section string `json:"section"`
		Key     string `json:"key"`
		Mode    string `json:"mode"`
		// explicit target (definitions, and any non-want-field mark)
		Target *mywant.AuraTarget `json:"target"`
		// value: scalar for a binding, object for a definition
		Value any `json:"value"`
	}
	if err := DecodeRequest(r, &req); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	var target mywant.AuraTarget
	switch {
	case req.Target != nil:
		target = *req.Target
	case req.WantID != "":
		want, _, found := s.globalBuilder.FindWantByID(req.WantID)
		if !found {
			s.JSONError(w, r, http.StatusNotFound, "Want not found", req.WantID)
			return
		}
		if want.Metadata.Type == "" {
			s.JSONError(w, r, http.StatusBadRequest, "Want has no type to address the mark to", req.WantID)
			return
		}
		target = mywant.AuraTarget{
			Kind: mywant.AuraTargetKindWantType,
			Name: want.Metadata.Type,
			Path: req.Section + "/" + req.Key,
		}
	default:
		s.JSONError(w, r, http.StatusBadRequest, "Either target or wantId is required", "")
		return
	}
	if !target.Valid() {
		s.JSONError(w, r, http.StatusBadRequest, "Invalid aura target (kind and name are required)", "")
		return
	}

	mark := mywant.AuraMark{Target: target, Value: req.Value, Mode: req.Mode}
	c, ok := mywant.SetCharacterAuraDefault(id, mark)
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
