package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
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

// ── per-want alerts ─────────────────────────────────────────────────────────
//
// A "want alert" is a deliberate, persistent notice tied to one want: it badges
// the want's tile and minimap cell until the want is opened. want_achieved
// raises one automatically (see RegisterAchievementCallback); a want agent or
// the system can raise one directly via POST /wants/{id}/notify.

// notifyWant handles POST /api/v1/wants/{id}/notify — body {message, title?}.
func (s *Server) notifyWant(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var body struct {
		Message string `json:"message"`
		Title   string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Message == "" {
		s.JSONError(w, r, http.StatusBadRequest, "message is required", "")
		return
	}
	if err := s.notifications.Record(NotificationEntry{
		Message:    body.Message,
		Kind:       "alert",
		TargetType: "want",
		TargetID:   id,
	}); err != nil {
		s.JSONError(w, r, http.StatusInternalServerError, "failed to record", err.Error())
		return
	}

	title := body.Title
	if title == "" {
		if want, _, found := s.globalBuilder.FindWantByID(id); found && want.Metadata.Name != "" {
			title = want.Metadata.Name
		} else {
			title = "MyWant"
		}
	}
	s.sendWantPush(id, title, body.Message, "/w/"+id)

	s.JSONResponse(w, http.StatusCreated, map[string]any{"ok": true})
}

// getWantNotifications handles GET /api/v1/wants/{id}/notifications[?limit=N].
func (s *Server) getWantNotifications(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	limit := 0
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	entries, unread := s.notifications.ForWant(id, limit)
	s.JSONResponse(w, http.StatusOK, map[string]any{"notifications": entries, "unread": unread})
}

// markWantNotificationsRead handles POST /api/v1/wants/{id}/notifications/read.
func (s *Server) markWantNotificationsRead(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	n, err := s.notifications.MarkWantRead(id)
	if err != nil {
		s.JSONError(w, r, http.StatusInternalServerError, "failed to mark read", err.Error())
		return
	}
	s.JSONResponse(w, http.StatusOK, map[string]any{"read": n})
}

// getUnreadWantCounts handles GET /api/v1/notifications/unread-counts.
func (s *Server) getUnreadWantCounts(w http.ResponseWriter, r *http.Request) {
	counts := s.notifications.UnreadWantCounts()
	total := 0
	for _, c := range counts {
		total += c
	}
	s.JSONResponse(w, http.StatusOK, map[string]any{"counts": counts, "total": total})
}
