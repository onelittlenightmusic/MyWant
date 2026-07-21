package server

import (
	"net/http"
	"sync"
	"time"

	mywant "mywant/engine/core"

	"github.com/gorilla/mux"
)

// ── In-memory cursor store ────────────────────────────────────────────────────

const cursorTTL = 8 * time.Second // entries older than this are excluded from GET

// sayTTL is how long a speech bubble stays up. Mirrors sayTtlMs() in
// web/src/shared/characterPresence.ts.
const sayTTL = 8 * time.Second

type cursorEntry struct {
	DeviceID    string  `json:"deviceId,omitempty"`
	X           float64 `json:"x"`
	Y           float64 `json:"y"`
	Avatar      string  `json:"avatar,omitempty"`
	Color       string  `json:"color,omitempty"`
	Name        string  `json:"name,omitempty"`
	LastSeen    int64   `json:"lastSeen"` // Unix ms
	EffectType  string  `json:"effectType,omitempty"`
	EffectNonce int64   `json:"effectNonce,omitempty"`
	// Speech-bubble text set by `i say` / the canvas Say action. The client
	// keeps resending it on each position PUT while the bubble should stay
	// up, so it expires naturally with the entry's own cursorTTL — there is
	// no separate clear call.
	Message   string `json:"message,omitempty"`
	MessageAt int64  `json:"messageAt,omitempty"` // Unix ms the message was first said
}

var (
	cursorsMu sync.RWMutex
	cursors   = map[string]cursorEntry{} // characterId → entry
)

// cursorResponse is returned by GET /api/v1/cursors.
type cursorResponse struct {
	CharacterID string  `json:"characterId"`
	DeviceID    string  `json:"deviceId,omitempty"`
	X           float64 `json:"x"`
	Y           float64 `json:"y"`
	Avatar      string  `json:"avatar,omitempty"`
	Color       string  `json:"color,omitempty"`
	Name        string  `json:"name,omitempty"`
	LastSeen    int64   `json:"lastSeen"`
	EffectType  string  `json:"effectType,omitempty"`
	EffectNonce int64   `json:"effectNonce,omitempty"`
	Message     string  `json:"message,omitempty"`
	MessageAt   int64   `json:"messageAt,omitempty"`
}

// snapshotCursors returns all non-stale cursor entries as a response slice.
// Shared by listCursors (HTTP GET) and the SSE broadcast after updateCursor.
func snapshotCursors() []cursorResponse {
	cutoff := time.Now().Add(-cursorTTL).UnixMilli()
	cursorsMu.RLock()
	result := make([]cursorResponse, 0, len(cursors))
	for charID, e := range cursors {
		if e.LastSeen < cutoff {
			continue
		}
		result = append(result, cursorResponse{
			CharacterID: charID,
			DeviceID:    e.DeviceID,
			X:           e.X,
			Y:           e.Y,
			Avatar:      e.Avatar,
			Color:       e.Color,
			Name:        e.Name,
			LastSeen:    e.LastSeen,
			EffectType:  e.EffectType,
			EffectNonce: e.EffectNonce,
			Message:     e.Message,
			MessageAt:   e.MessageAt,
		})
	}
	cursorsMu.RUnlock()
	return result
}

// listCursors handles GET /api/v1/cursors
// Returns all cursor positions that have been updated within cursorTTL.
func (s *Server) listCursors(w http.ResponseWriter, r *http.Request) {
	result := snapshotCursors()

	// Lazily prune stale entries (best-effort, no accuracy guarantee).
	cutoff := time.Now().Add(-cursorTTL).UnixMilli()
	go func() {
		cursorsMu.Lock()
		for charID, e := range cursors {
			if e.LastSeen < cutoff {
				delete(cursors, charID)
			}
		}
		cursorsMu.Unlock()
	}()

	if checkETag(w, r, result) {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	s.JSONResponse(w, http.StatusOK, result)
}

// updateCursor handles PUT /api/v1/cursors/:characterId
// Upserts this character's cursor position. Last-write-wins, no locking.
func (s *Server) updateCursor(w http.ResponseWriter, r *http.Request) {
	characterID := mux.Vars(r)["characterId"]
	if characterID == "" {
		s.JSONError(w, r, http.StatusBadRequest, "characterId is required", "")
		return
	}

	var body struct {
		X           float64 `json:"x"`
		Y           float64 `json:"y"`
		DeviceID    string  `json:"deviceId,omitempty"`
		Avatar      string  `json:"avatar,omitempty"`
		Color       string  `json:"color,omitempty"`
		Name        string  `json:"name,omitempty"`
		EffectType  string  `json:"effectType,omitempty"`
		EffectNonce int64   `json:"effectNonce,omitempty"`
		Message     string  `json:"message,omitempty"`
		MessageAt   int64   `json:"messageAt,omitempty"`
	}
	if err := DecodeRequest(r, &body); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "invalid request body", err.Error())
		return
	}

	cursorsMu.Lock()
	prev := cursors[characterID]
	// Only the FIRST PUT carrying a given messageAt is a real utterance — that's
	// the one worth archiving. Later PUTs just carry it along.
	isNewMessage := body.Message != "" && prev.MessageAt != body.MessageAt

	// A live speech bubble survives position updates that say nothing about it.
	// Without this, a message set by one writer is wiped by the next position
	// PUT from another: `mywant-gui i say` publishes the message, the SSE echo
	// makes the speaker's own browser re-sync and re-PUT its position, and that
	// PUT — which knows nothing of the CLI's message — would blank the bubble
	// milliseconds after it appeared.
	message, messageAt := body.Message, body.MessageAt
	if message == "" && prev.Message != "" && time.Since(time.UnixMilli(prev.MessageAt)) < sayTTL {
		message, messageAt = prev.Message, prev.MessageAt
	}

	cursors[characterID] = cursorEntry{
		DeviceID:    body.DeviceID,
		X:           body.X,
		Y:           body.Y,
		Avatar:      body.Avatar,
		Color:       body.Color,
		Name:        body.Name,
		LastSeen:    time.Now().UnixMilli(),
		EffectType:  body.EffectType,
		EffectNonce: body.EffectNonce,
		Message:     message,
		MessageAt:   messageAt,
	}
	cursorsMu.Unlock()

	go broadcastSSE("cursor", snapshotCursors())

	// Log to ~/.mywant/work.log.
	// important=true only when an effect (aura / want interaction) fired or a
	// new Say message was uttered — those are the traces worth keeping for
	// future players. Plain position updates are kept for 1 hour then
	// discarded by rotation.
	mywant.AppendWorkLog(mywant.WorkLogEntry{
		Type:      "cursor",
		Important: body.EffectType != "" || isNewMessage,
		Data: map[string]any{
			"character_id": characterID,
			"device_id":    body.DeviceID,
			"x":            body.X,
			"y":            body.Y,
			"avatar":       body.Avatar,
			"color":        body.Color,
			"name":         body.Name,
			"effect_type":  body.EffectType,
			"effect_nonce": body.EffectNonce,
			"message":      body.Message,
		},
	})

	w.WriteHeader(http.StatusNoContent)

}

// deleteCursor handles DELETE /api/v1/cursors/:characterId
// Called when a device leaves canvas mode so its cursor disappears immediately.
func (s *Server) deleteCursor(w http.ResponseWriter, r *http.Request) {
	characterID := mux.Vars(r)["characterId"]
	cursorsMu.Lock()
	delete(cursors, characterID)
	cursorsMu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}
