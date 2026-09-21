package server

import (
	"net/http"

	"github.com/gorilla/mux"
	"mywant/engine/types"
)

// Stopping the robot, and forgetting what it was told.
//
// Both are things a person does to a conversation that is already running, and
// neither can be done by writing state: the Do agent is blocked inside the
// agent's own answer and will not look at the want again until it comes back
// (see fm_server.go). So they reach the live process directly.
//
// Only the on-device provider can be reached this way. claude_code and gemini
// run a CLI of their own per request and have no resident session to interrupt
// — asked to stop one of those, this says so rather than reporting a stop that
// did not happen.

// interruptWantChat stops the turn the agent is in the middle of.
func (s *Server) interruptWantChat(w http.ResponseWriter, r *http.Request) {
	want := s.findWantByIDOrName(mux.Vars(r)["id"])
	if want == nil {
		s.JSONError(w, r, http.StatusNotFound, "want not found", "")
		return
	}
	if stopped := types.StopOnDeviceAgent(); !stopped {
		// Not an error: pressing stop on a turn that has just finished is a
		// normal thing to do, and the screen it was pressed on is a poll or
		// two behind the want.
		s.JSONResponse(w, http.StatusOK, map[string]any{
			"stopped": false,
			"message": "nothing was running",
		})
		return
	}
	want.SetCurrent("cc_streaming_text", "")
	s.JSONResponse(w, http.StatusOK, map[string]any{"stopped": true})
}

// clearWantChatSession forgets the conversation the agent is holding.
//
// What it forgets is the agent's own transcript — what it can refer back to.
// The chat on screen is the want's state and is left alone: the person asked
// the robot to forget, not to erase what they said.
func (s *Server) clearWantChatSession(w http.ResponseWriter, r *http.Request) {
	want := s.findWantByIDOrName(mux.Vars(r)["id"])
	if want == nil {
		s.JSONError(w, r, http.StatusNotFound, "want not found", "")
		return
	}
	cleared, err := types.ClearOnDeviceSession()
	if err != nil {
		s.JSONError(w, r, http.StatusConflict, err.Error(), "")
		return
	}
	s.JSONResponse(w, http.StatusOK, map[string]any{"cleared": cleared})
}
