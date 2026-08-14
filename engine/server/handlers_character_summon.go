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
	if _, ok := mywant.GetCharacter(id); !ok {
		s.JSONError(w, r, http.StatusNotFound, "Character not found", id)
		return
	}

	var req summonRequest
	if err := DecodeRequest(r, &req); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	// Asking is asking, whether or not anybody is there to hear it yet.
	//
	// This used to look for a live device first and move the character when it
	// found none, which made consent depend on being caught at the right
	// moment: a character nobody had a tab open for was simply dragged. An
	// invitation can wait — it sits in the queue until whoever picks that
	// character up answers it, on whatever device they turn up with, which may
	// not have existed when it was sent. Presence has no business deciding
	// whether permission is asked for.
	//
	// It is also the more honest reading of the two words: call asks, take
	// takes. Anyone who wants the old behaviour asks for it by not inviting.
	// An agent has no consent to give. The robot is a program with a character's
	// face — there is no one behind it to be interrupted, no camera to yank, and
	// nobody who will ever open a tab to answer, so an invitation addressed to it
	// would sit in the queue forever while the thing it asked for never happened.
	// Calling it is simply moving it, which is what calling a program means.
	if req.Invite && !isAgentCharacter(id) {
		from, _ := mywant.GetCharacter(req.From)
		fromName := ""
		if from != nil {
			fromName = from.Name
		}
		s.appendCallInvite(id, req.From, fromName, req.X, req.Y, req.URL)
		s.JSONResponse(w, http.StatusOK, summonResponse{Outcome: "invited", X: req.X, Y: req.Y})
		return
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

// appendCallInvite asks a browser whether its character will come.
func (s *Server) appendCallInvite(toCharacterID, fromID, fromName string, x, y float64, url string) {
	want := s.findWantByIDInAll(guiStateWantID)
	if want == nil {
		return
	}
	now := time.Now().UnixMilli()
	action := map[string]any{
		"id":   fmt.Sprintf("call-%d-%s", now, toCharacterID),
		"type": "call-invite",
		// Addressed to the person, not to a screen. Which device answers is
		// whichever one is playing them when they get to it, and that one need
		// not exist yet — see the queue this lands in (pendingDeviceActions),
		// which is read by every client and outlives any of them.
		"to_character_id":     toCharacterID,
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
			"to_character_id":   toCharacterID,
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
			"to_character_id":   stringField(invite, "to_character_id"),
			"x":                 invite["x"],
			"y":                 invite["y"],
			"url":               stringField(invite, "url"),
		},
	})
}

// isAgentCharacter reports whether this character is run by a program rather
// than played by a person.
//
// Asked of the want bound to them: the robot's is its coding agent, which is
// the whole of the difference — a chat want is a mailbox and its character is
// still somebody. Kept next to the summon rather than shared with the drawing
// rule it resembles, because they answer different questions and will not
// always agree: one is about who can consent, the other about what to draw.
func isAgentCharacter(characterID string) bool {
	c, ok := mywant.GetCharacter(characterID)
	if !ok || c.AuraCardWantID == "" {
		return false
	}
	want, _, found := mywant.GetGlobalChainBuilder().FindWantByID(c.AuraCardWantID)
	return found && want != nil && want.Metadata.Type == "robot"
}
