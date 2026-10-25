package server

import (
	"net/http"

	"github.com/gorilla/mux"
	mywant "mywant/engine/core"
	"mywant/engine/types"
)

// A battle runs itself: each room defense in its own Progress, each of its
// wants in its own (types/room_defense_battle.go, battle_send.go). What the
// server has for it is what only the server knows — where the board's walls
// are, and where each character stands — handed in once, and the 攻撃
// skill's lock-on, which is a connection asked for over the API.
func (s *Server) startBattles() {
	types.BlockedCells = blockedCells
	types.CharacterAt = func(characterID string) (float64, float64, bool) {
		if characterID == "" {
			return 0, 0, false
		}
		cursorsMu.RLock()
		e, ok := cursors[characterID]
		cursorsMu.RUnlock()
		return e.X, e.Y, ok
	}
}

// POST /api/v1/wants/{id}/attack  {"target": "<want id>"}  — "" to stop.
//
// This want attacks that one: its connection becomes the one to the target's
// hp (setAttack), which the battle reads — a character's own want connected
// to an enemy is the 攻撃 skill, a turret's is its aim.
func (s *Server) setWantAttack(w http.ResponseWriter, r *http.Request) {
	if s.globalBuilder == nil {
		s.JSONError(w, r, http.StatusServiceUnavailable, "Server not ready", "")
		return
	}
	attacker, _, found := s.globalBuilder.FindWantByID(mux.Vars(r)["id"])
	if !found {
		s.JSONError(w, r, http.StatusNotFound, "Want not found", "")
		return
	}
	var body struct {
		Target string `json:"target"`
		// Add: one more connection rather than the only one — a heal mends
		// several. Already connected, it is taken off instead.
		Add bool `json:"add"`
	}
	if err := DecodeRequest(r, &body); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "invalid request body", err.Error())
		return
	}
	if body.Target == "" {
		types.SetAttack(attacker, nil)
		s.JSONResponse(w, http.StatusOK, map[string]any{"attacking": ""})
		return
	}
	target, _, found := s.globalBuilder.FindWantByID(body.Target)
	if !found {
		s.JSONError(w, r, http.StatusNotFound, "Target not found", "")
		return
	}
	if !body.Add {
		types.SetAttack(attacker, target)
		s.JSONResponse(w, http.StatusOK, map[string]any{"attacking": target.Metadata.Name})
		return
	}
	var to []*mywant.Want
	removed := false
	if spec := attacker.GetSpec(); spec != nil {
		for _, sub := range spec.StateSubscriptions {
			if sub.WantName == target.Metadata.Name {
				removed = true
				continue
			}
			if t, ok := s.globalBuilder.FindWantByName(sub.WantName); ok {
				to = append(to, t)
			}
		}
	}
	if !removed {
		to = append(to, target)
	}
	types.SetLinks(attacker, to)
	names := make([]string, 0, len(to))
	for _, t := range to {
		names = append(names, t.Metadata.Name)
	}
	s.JSONResponse(w, http.StatusOK, map[string]any{"connected": names, "removed": removed})
}
