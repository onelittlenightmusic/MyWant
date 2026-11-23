package server

import (
	"context"
	"net/http"

	"github.com/gorilla/mux"
	mywant "mywant/engine/core"
)

// voiceEventKinds are the events the board may tell a want of — what happened
// to it on the board that it should say something about. The words are the
// default lines' (core/want_voice_defaults.go, by language): the board names
// the event and the wants it is about, never the words.
var voiceEventKinds = map[string]bool{
	mywant.VoiceEventHearGained: true,
	mywant.VoiceEventHearFar:    true,
	mywant.VoiceEventHearTaken:  true,
}

// wantVoiceEvent handles POST /api/v1/wants/{id}/voice-event —
// {kind, from_id, to_id}: the want says, out loud over its tile, what it makes
// of the event (a moment of kind speech, aloud). Answers {say}.
func (s *Server) wantVoiceEvent(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		s.handleOptions(w, r)
		return
	}
	var body struct {
		Kind   string `json:"kind"`
		FromID string `json:"from_id"`
		ToID   string `json:"to_id"`
	}
	if err := DecodeRequest(r, &body); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "invalid request body", err.Error())
		return
	}
	if !voiceEventKinds[body.Kind] {
		s.JSONError(w, r, http.StatusBadRequest, "unknown voice event", body.Kind)
		return
	}
	want := s.findWantByIDInAll(mux.Vars(r)["id"])
	if want == nil {
		s.JSONError(w, r, http.StatusNotFound, "want not found", mux.Vars(r)["id"])
		return
	}
	vars := map[string]string{}
	for k, id := range map[string]string{"from": body.FromID, "to": body.ToID} {
		if id == "" {
			continue
		}
		if other := s.findWantByIDInAll(id); other != nil {
			vars[k] = other.Metadata.Name
		}
	}
	// Said in the default words alone: a type's voice script speaks of its
	// own answers, and one not written for these events would answer with
	// them (a weather's forecast for "too far to hear").
	line := mywant.WantNewsVoice(context.Background(), want, "", mywant.VoiceEvent{Kind: body.Kind, Vars: vars})
	if line.Say != "" {
		m := speechMoment("want", want.Metadata.ID, line.Say, line.Mood)
		m.Aloud = true
		moments.add(m)
	}
	s.JSONResponse(w, http.StatusOK, map[string]any{"say": line.Say, "mood": line.Mood})
}
