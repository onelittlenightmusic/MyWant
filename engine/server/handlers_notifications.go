package server

import (
	"encoding/json"
	"net/http"
	"strconv"
)

// ── Notifications ─────────────────────────────────────────────────────────────
//
// The GUI speaks every transient notice through the robot's speech bubble; each
// one is posted here so the Logs page can show what the user was told, on any
// device. See NotificationStore.

// recordNotification handles POST /api/v1/notifications
func (s *Server) recordNotification(w http.ResponseWriter, r *http.Request) {
	var entry NotificationEntry
	if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "invalid notification payload", err.Error())
		return
	}
	if entry.Message == "" {
		s.JSONError(w, r, http.StatusBadRequest, "notification message is required", "")
		return
	}
	// The client supplies neither — stamping is the server's job, so entries
	// stay ordered by one clock even when devices disagree about the time.
	entry.ID = ""
	entry.At = ""

	if err := s.notifications.Record(entry); err != nil {
		s.JSONError(w, r, http.StatusInternalServerError, "failed to record notification", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// getNotifications handles GET /api/v1/notifications[?limit=N] (most-recent first)
func (s *Server) getNotifications(w http.ResponseWriter, r *http.Request) {
	limit := 0
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	entries := s.notifications.All(limit)
	s.JSONResponse(w, http.StatusOK, map[string]any{"notifications": entries, "count": len(entries)})
}

// clearNotifications handles DELETE /api/v1/notifications
func (s *Server) clearNotifications(w http.ResponseWriter, r *http.Request) {
	if err := s.notifications.Clear(); err != nil {
		s.JSONError(w, r, http.StatusInternalServerError, "failed to clear notifications", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
