package server

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	mywant "mywant/engine/core"
)

// Moving somebody, as one thing the server knows how to do.
//
// The GUI's Call and the CLI's `take` were two implementations of one idea, and
// they had drifted apart in every way two implementations can: Call wrote the
// character's position into gui_state and invited a player who was present to
// consent, `take` wrote canvas labels onto a want directly and asked nobody. So
// the same instruction produced different effects — after a `take`, a character
// stayed where they were while the want bound to them was dragged off alone —
// and every fix had to be made twice or it was not made at all.
//
// One endpoint, then, and the only genuine difference between the two callers
// is a flag: whether to ask first. That is a policy about manners, which
// belongs to whoever is doing the summoning; where a character IS belongs here.

// summonRequest is the body of POST /api/v1/characters/{id}/summon.
type summonRequest struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	// Invite asks the target's browser to accept before anything moves, when
	// there is a browser to ask. Moving somebody who is present without their
	// say-so also yanks their camera mid-action; a character with nobody at the
	// controls has nobody to ask, and is moved directly.
	Invite bool `json:"invite,omitempty"`
	// From names who is doing the summoning, for the invitation to say.
	From string `json:"from,omitempty"`
	// URL invites them onto a web page instead of a cell. Only meaningful
	// alongside Invite — there is nothing to open without a browser.
	URL string `json:"url,omitempty"`
}

type summonResponse struct {
	// "invited" (asked, awaiting an answer) or "moved" (done).
	Outcome string  `json:"outcome"`
	X       float64 `json:"x"`
	Y       float64 `json:"y"`
}

func (s *Server) summonCharacter(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	target, ok := mywant.GetCharacter(id)
	if !ok {
		s.JSONError(w, r, http.StatusNotFound, "Character not found", id)
		return
	}

	var req summonRequest
	if err := DecodeRequest(r, &req); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if req.Invite {
		if deviceID := s.liveDeviceOf(target); deviceID != "" {
			from, _ := mywant.GetCharacter(req.From)
			fromName := ""
			if from != nil {
				fromName = from.Name
			}
			s.appendCallInvite(deviceID, req.From, fromName, req.X, req.Y, req.URL)
			s.JSONResponse(w, http.StatusOK, summonResponse{Outcome: "invited", X: req.X, Y: req.Y})
			return
		}
	}

	s.moveCharacterTo(id, req.X, req.Y)
	s.JSONResponse(w, http.StatusOK, summonResponse{Outcome: "moved", X: req.X, Y: req.Y})
}

// moveCharacterTo puts a character on a cell, and everything that follows from
// that follows from here: the durable roster the board draws from, the want
// bound to them, and the gui_state field a reloading tab reads back.
//
// Written through gui_state rather than beside it, because that is where a
// character's position has always lived and where every client already looks
// for it. The bridge that turns such a write into the rest (see
// applyCharacterCursorToWant) is the same one an ordinary gui_state PUT goes
// through, so a summon and a hand-written position cannot diverge.
func (s *Server) moveCharacterTo(characterID string, x, y float64) {
	updates := map[string]any{
		cursorStateXPrefix + characterID: x,
		cursorStateYPrefix + characterID: y,
	}
	s.applyCharacterCursorToWant(updates)

	want := s.findWantByIDInAll(guiStateWantID)
	if want == nil {
		return
	}
	for key, val := range updates {
		want.StoreState(key, val)
		if !mywant.Contains(want.ProvidedStateFields, key) {
			want.ProvidedStateFields = append(want.ProvidedStateFields, key)
		}
	}
	resp := guiStateResponse{Seq: nextGUIStateSeq(), State: s.guiStateWithConfig(want)}
	go broadcastSSE("gui_state", resp)
}

// liveDeviceOf returns the device somebody is playing this character on, or ""
// when nobody is at the controls.
//
// Asked of the cursor registry first, because that is what being played
// actually is: a browser publishing where this character is, from a device it
// names as it does so. The assignment list is a different fact — which devices
// a character has been GIVEN — and it is routinely empty for a character
// somebody is playing right now, which made every call to a present player
// skip the invitation and simply move them. Whoever is driving is the one to
// ask, and the driver says which device they are driving from.
func (s *Server) liveDeviceOf(c *mywant.Character) string {
	cursorsMu.RLock()
	e, ok := cursors[c.ID]
	cursorsMu.RUnlock()
	if ok && hasLiveCursor(c.ID) && e.DeviceID != "" {
		return e.DeviceID
	}

	// Nobody is publishing, but a device may still be assigned and listening —
	// a tab that has gone quiet without closing.
	want := s.findWantByIDInAll(guiStateWantID)
	if want == nil || len(c.AssignedDeviceIDs) == 0 {
		return ""
	}
	raw, ok := want.GetAllState()["devices"].([]any)
	if !ok {
		return ""
	}
	cutoff := time.Now().UnixMilli() - deviceLiveMs
	for _, id := range c.AssignedDeviceIDs {
		for _, d := range raw {
			m, ok := d.(map[string]any)
			if !ok || stringField(m, "id") != id {
				continue
			}
			if seen, ok := m["lastSeen"].(float64); ok && int64(seen) >= cutoff {
				return id
			}
		}
	}
	return ""
}

// How recently a device must have checked in to count as somebody being there.
// Matches the GUI's own DEVICE_LIVE_MS, so the two agree about who is present.
const deviceLiveMs = 60_000

// appendCallInvite asks a browser whether its character will come.
func (s *Server) appendCallInvite(deviceID, fromID, fromName string, x, y float64, url string) {
	want := s.findWantByIDInAll(guiStateWantID)
	if want == nil {
		return
	}
	now := time.Now().UnixMilli()
	action := map[string]any{
		"id":                  fmt.Sprintf("call-%d-%s", now, deviceID),
		"type":                "call-invite",
		"device_id":           deviceID,
		"from_character_id":   fromID,
		"from_character_name": fromName,
		"x":                   x,
		"y":                   y,
		"url":                 url,
		"timestamp":           now,
	}
	var actions []any
	if existing, ok := want.GetAllState()["pendingDeviceActions"].([]any); ok {
		actions = existing
	}
	want.StoreState("pendingDeviceActions", append(actions, action))
	mywant.AppendWorkLog(mywant.WorkLogEntry{
		Type:      "call",
		Important: true,
		Data: map[string]any{
			"from_character_id": fromID,
			"to_device_id":      deviceID,
			"x":                 x,
			"y":                 y,
			"url":               url,
		},
	})
	resp := guiStateResponse{Seq: nextGUIStateSeq(), State: s.guiStateWithConfig(want)}
	go broadcastSSE("gui_state", resp)
}

// ── Answering an invitation ──────────────────────────────────────────────────

// respondRequest is the body of POST /api/v1/characters/{id}/summon/respond.
type respondRequest struct {
	InviteID string `json:"inviteId"`
	Accept   bool   `json:"accept"`
}

type respondResponse struct {
	Outcome string  `json:"outcome"` // "moved", "declined", or "open-url"
	X       float64 `json:"x,omitempty"`
	Y       float64 `json:"y,omitempty"`
	URL     string  `json:"url,omitempty"`
}

// respondToSummon answers an invitation, in the one place that knows what
// answering means.
//
// Accepting used to be a thing a browser did to itself: it moved its own cursor
// and forgot the invitation. So the server, having asked, never learned the
// answer — accepted, declined and ignored looked identical from here — the move
// skipped the path every other move takes, and nothing without a browser could
// answer at all. Half of summoning had been unified and this was the other half,
// still living in a client.
//
// A URL invitation is the one part that cannot move here: opening a page is
// something only the browser can do. The answer is still recorded here, and the
// address is handed back for the client to open.
func (s *Server) respondToSummon(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	if _, ok := mywant.GetCharacter(id); !ok {
		s.JSONError(w, r, http.StatusNotFound, "Character not found", id)
		return
	}
	var req respondRequest
	if err := DecodeRequest(r, &req); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	invite := s.takePendingAction(req.InviteID)
	if invite == nil {
		s.JSONError(w, r, http.StatusNotFound, "No such invitation", req.InviteID)
		return
	}

	x, _ := invite["x"].(float64)
	y, _ := invite["y"].(float64)
	url := stringField(invite, "url")

	if !req.Accept {
		s.recordSummonAnswer(invite, "declined")
		s.JSONResponse(w, http.StatusOK, respondResponse{Outcome: "declined"})
		return
	}

	if url != "" {
		s.recordSummonAnswer(invite, "accepted")
		s.JSONResponse(w, http.StatusOK, respondResponse{Outcome: "open-url", URL: url})
		return
	}

	// The same move a summons without an invitation makes — one path, so
	// arriving because you agreed to and arriving because you were pulled leave
	// the board in the same state.
	s.moveCharacterTo(id, x, y)
	s.recordSummonAnswer(invite, "accepted")
	s.JSONResponse(w, http.StatusOK, respondResponse{Outcome: "moved", X: x, Y: y})
}

// takePendingAction removes an action by id and returns it, so an invitation
// cannot be answered twice.
func (s *Server) takePendingAction(id string) map[string]any {
	want := s.findWantByIDInAll(guiStateWantID)
	if want == nil || id == "" {
		return nil
	}
	existing, _ := want.GetAllState()["pendingDeviceActions"].([]any)
	var found map[string]any
	remaining := make([]any, 0, len(existing))
	for _, a := range existing {
		m, ok := a.(map[string]any)
		if ok && stringField(m, "id") == id {
			found = m
			continue
		}
		remaining = append(remaining, a)
	}
	if found == nil {
		return nil
	}
	want.StoreState("pendingDeviceActions", remaining)
	resp := guiStateResponse{Seq: nextGUIStateSeq(), State: s.guiStateWithConfig(want)}
	go broadcastSSE("gui_state", resp)
	return found
}

// recordSummonAnswer keeps the answer, so the person who asked can find out
// what became of it — the whole point of asking rather than pulling.
func (s *Server) recordSummonAnswer(invite map[string]any, answer string) {
	mywant.AppendWorkLog(mywant.WorkLogEntry{
		Type:      "call",
		Important: true,
		Data: map[string]any{
			"answer":            answer,
			"from_character_id": stringField(invite, "from_character_id"),
			"to_device_id":      stringField(invite, "device_id"),
			"x":                 invite["x"],
			"y":                 invite["y"],
			"url":               stringField(invite, "url"),
		},
	})
}
