package server

import (
	"net/http"
	"strings"
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

// effectEvent is one effect firing, carried on the cursor so a burst of them
// survives to the client. A single scalar effectType/nonce loses rapid repeats:
// the client's setState coalesces snapshots, so only the last nonce is seen and
// only one animation plays. A short list lets one snapshot piggyback every
// recent fire, and the client replays each nonce it hasn't seen. Trimmed by age
// (effectTTL) so it stays tiny and old fires don't replay on a fresh connect.
type effectEvent struct {
	Type  string  `json:"type"`
	Nonce int64   `json:"nonce"` // Unix ms, monotonic across client and server sources
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
}

const effectTTL = 3 * time.Second

// trimEffects drops events older than effectTTL.
func trimEffects(prev []effectEvent) []effectEvent {
	cutoff := time.Now().Add(-effectTTL).UnixMilli()
	out := make([]effectEvent, 0, len(prev))
	for _, e := range prev {
		if e.Nonce >= cutoff {
			out = append(out, e)
		}
	}
	return out
}

// appendNoop just ages the list (used on effect-less position PUTs).
func appendNoop(prev []effectEvent) []effectEvent { return trimEffects(prev) }

// appendEffect ages the list and adds ev.
func appendEffect(prev []effectEvent, ev effectEvent) []effectEvent {
	return append(trimEffects(prev), ev)
}

// splitEffectTypes splits a comma-separated effectType into non-empty types.
func splitEffectTypes(s string) []string {
	out := []string{}
	for _, t := range strings.Split(s, ",") {
		if t = strings.TrimSpace(t); t != "" {
			out = append(out, t)
		}
	}
	return out
}

type cursorEntry struct {
	DeviceID    string        `json:"deviceId,omitempty"`
	X           float64       `json:"x"`
	Y           float64       `json:"y"`
	Avatar      string        `json:"avatar,omitempty"`
	Color       string        `json:"color,omitempty"`
	Name        string        `json:"name,omitempty"`
	LastSeen    int64         `json:"lastSeen"` // Unix ms
	Effects     []effectEvent `json:"effects,omitempty"`
	EffectType  string        `json:"effectType,omitempty"`
	EffectNonce int64         `json:"effectNonce,omitempty"`
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
	Name        string        `json:"name,omitempty"`
	LastSeen    int64         `json:"lastSeen"`
	Effects     []effectEvent `json:"effects,omitempty"`
	EffectType  string        `json:"effectType,omitempty"`
	EffectNonce int64         `json:"effectNonce,omitempty"`
	Message     string        `json:"message,omitempty"`
	MessageAt   int64         `json:"messageAt,omitempty"`
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
			Effects:     e.Effects,
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

	// Carry the recent-effects list across position PUTs (which say nothing
	// about effects), appending any newly fired ones so a rapid burst all rides
	// the snapshot. effectType may be comma-separated (several effects fired by
	// one press).
	effects := appendNoop(prev.Effects)
	if body.EffectType != "" {
		nonce := body.EffectNonce
		if nonce == 0 {
			nonce = time.Now().UnixMilli()
		}
		for _, t := range splitEffectTypes(body.EffectType) {
			effects = appendEffect(effects, effectEvent{Type: t, Nonce: nonce, X: body.X, Y: body.Y})
		}
	}

	cursors[characterID] = cursorEntry{
		DeviceID:    body.DeviceID,
		X:           body.X,
		Y:           body.Y,
		Avatar:      body.Avatar,
		Color:       body.Color,
		Name:        body.Name,
		LastSeen:    time.Now().UnixMilli(),
		Effects:     effects,
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

// FireCharacterEffect plays an effect on a character's cursor by bumping its
// EffectType/EffectNonce in place (keeping its current position) and
// broadcasting the cursor roster — the same path an X-press takes, so every
// client animates the effect at that character's cursor. Used by server-side
// triggers such as a place_arrival geofence, which have no client of their own.
// No-op if the character has no live cursor entry.
func FireCharacterEffect(characterID, effectType string) {
	if characterID == "" || effectType == "" {
		return
	}
	cursorsMu.Lock()
	e, ok := cursors[characterID]
	if !ok {
		cursorsMu.Unlock()
		return // character has no cursor on screen — nothing to animate
	}
	// Wall-clock ms, the same unit the client dispatcher uses, so the frontend's
	// monotonic per-event guard stays consistent whichever source fired.
	nonce := time.Now().UnixMilli()
	e.Effects = appendEffect(e.Effects, effectEvent{Type: effectType, Nonce: nonce, X: e.X, Y: e.Y})
	e.EffectType = effectType
	e.EffectNonce = nonce
	e.LastSeen = time.Now().UnixMilli()
	cursors[characterID] = e
	cursorsMu.Unlock()

	go broadcastSSE("cursor", snapshotCursors())
}
