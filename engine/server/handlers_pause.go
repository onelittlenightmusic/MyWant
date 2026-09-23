package server

import (
	"net/http"
)

// systemPauseResponse is what GET/PUT /api/v1/system/pause return.
type systemPauseResponse struct {
	Paused bool `json:"paused"`
}

// getSystemPause handles GET /api/v1/system/pause — whether every want is held
// by the global pause (see ChainBuilder.Suspend).
func (s *Server) getSystemPause(w http.ResponseWriter, r *http.Request) {
	s.JSONResponse(w, http.StatusOK, systemPauseResponse{Paused: s.globalBuilder.IsSuspended()})
}

// setSystemPause handles PUT /api/v1/system/pause {"paused": bool}.
//
// The emergency stop behind the control pill every tab carries: pausing holds
// every non-system want and its background agents on their next tick, resuming
// lets them go on where they were. Announced over SSE as "system_pause" so
// every open GUI flips its pill at once instead of on its next poll.
func (s *Server) setSystemPause(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Paused *bool `json:"paused"`
	}
	if err := DecodeRequest(r, &req); err != nil || req.Paused == nil {
		s.JSONError(w, r, http.StatusBadRequest, `body must be {"paused": true|false}`, "")
		return
	}
	if *req.Paused {
		s.globalBuilder.Suspend()
	} else {
		s.globalBuilder.Resume()
	}
	resp := systemPauseResponse{Paused: s.globalBuilder.IsSuspended()}
	broadcastSSE("system_pause", resp)
	s.JSONResponse(w, http.StatusOK, resp)
}
