package server

import (
	"net/http"
	"time"

	"github.com/gorilla/mux"
	mywant "mywant/engine/core"
	"mywant/engine/types"
)

// Stopping the robot, and forgetting what it was told.
//
// Both are things a person does to a conversation that is already running, and
// neither can be done by writing state: the Do agent is blocked inside the
// agent's own answer and will not look at the want again until it comes back
// (see fm_server.go). So they reach the live process directly.
//
// Only the on-device provider can be stopped this way. claude_code and gemini
// run a CLI of their own per request and have no resident session to interrupt
// — asked to stop one of those, this says so rather than reporting a stop that
// did not happen. Forgetting reaches all of them; see clearWantChatSession.

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
// the robot to forget, not to erase what they said. Where it was forgotten is
// written down instead (cc_session_resets), so the thread can show the break:
// a history that runs on unmarked past a reset reads as one conversation, and
// the answers after it look like they ignored everything before.
//
// Every provider, not only the on-device one. claude_code and gemini hold their
// conversation in a session the CLI resumes by id, so forgetting it is dropping
// the id — the next message starts a new session. Apple FM keeps its session in
// the resident process, which is reset directly as before.
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
	if mywant.GetGoal(want, "session_id", "") != "" || mywant.GetCurrent(want, "session_id", "") != "" {
		want.SetGoal("session_id", "")
		want.SetCurrent("session_id", "")
		want.SetCurrent("current_session_state", "waiting_for_first_message")
		cleared = true
	}
	// Marked whether or not anything was held: pressing reset is the person
	// saying the conversation ends here, and the next message is the first of a
	// new one either way.
	resets := mywant.GetCurrent(want, "cc_session_resets", []any{})
	resets = append(resets, time.Now().Format(time.RFC3339))
	if len(resets) > maxSessionResetMarks {
		resets = resets[len(resets)-maxSessionResetMarks:]
	}
	want.SetCurrent("cc_session_resets", resets)
	s.JSONResponse(w, http.StatusOK, map[string]any{"cleared": cleared})
}

// maxSessionResetMarks keeps the reset marks about as long as the history they
// sit in (cc_messages is a short FIFO); a mark older than every message left
// has nothing to divide.
const maxSessionResetMarks = 20
