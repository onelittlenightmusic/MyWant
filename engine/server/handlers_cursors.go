package server

import (
	"math"
	"net/http"
	"strconv"
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

	// cursorSeq is the highest client-assigned sequence number applied so
	// far for each character — see updateCursor's own seq handling. A
	// client's own rapid-fire position PUTs can arrive at the server out of
	// the order they were sent (ordinary network jitter across concurrent
	// requests); without this, whichever one is *processed* last wins,
	// which is not necessarily the one the client *sent* last, and the
	// character's position visibly regresses to an already-superseded cell.
	// Comparing seq (not arrival order) rejects exactly the stale one.
	// Absent entirely (seq==0, the zero value) for a character nothing has
	// ever sent a seq for — every caller that doesn't send one (drive-tick
	// updates, older clients, the CLI) is untouched by this check.
	cursorSeq = map[string]int64{}

	// Where each character was last seen, kept regardless of how long ago that
	// was. `cursors` answers "who is here now" and is pruned to cursorTTL (8s),
	// which is right for drawing other people's cursors — a cursor that stops
	// reporting should vanish rather than hang there as a ghost.
	//
	// It is wrong for "where is so-and-so". A tab that went quiet for ten
	// seconds has not moved its character to nowhere; the character is exactly
	// where it was left. Placement (canvas-near) asks that second question, and
	// asking it of the pruned map put wants in the corner of the world whenever
	// the browser's last publish had just aged out.
	lastCursorPos = map[string]cursorEntry{} // characterId → last known position
)

// Per-character canvas position, as written into gui_state by the canvas Call
// action and by `mywant-gui i take`. Read by character_want_bridge.go.
const (
	cursorStateXPrefix = "canvas_cursor_x_"
	cursorStateYPrefix = "canvas_cursor_y_"
)

// trimCursorKeyPrefix returns the character id a canvas_cursor_{x,y}_<id> key
// names. The unsuffixed keys (the CursorMan robot cursor) yield no id.
func trimCursorKeyPrefix(key, prefix string) (string, bool) {
	if !strings.HasPrefix(key, prefix) {
		return "", false
	}
	id := strings.TrimPrefix(key, prefix)
	return id, id != ""
}

// cursorCoord coerces a gui_state value to a coordinate. JSON round-trips make
// these float64, but a want's state can also hold them as int after a YAML load.
func cursorCoord(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	}
	return 0, false
}

// formatCanvasCoord renders a coordinate the way a canvas label holds it: a
// whole cell, as a string. Matches what the GUI writes when a tile is dragged.
func formatCanvasCoord(v float64) string {
	return strconv.Itoa(int(math.Round(v)))
}

// hasLiveCursor reports whether someone is currently publishing for a character.
func hasLiveCursor(characterID string) bool {
	cutoff := time.Now().Add(-cursorTTL).UnixMilli()
	cursorsMu.RLock()
	e, ok := cursors[characterID]
	cursorsMu.RUnlock()
	return ok && e.LastSeen >= cutoff
}

// cursorResponse is returned by GET /api/v1/cursors.
type cursorResponse struct {
	CharacterID string        `json:"characterId"`
	DeviceID    string        `json:"deviceId,omitempty"`
	X           float64       `json:"x"`
	Y           float64       `json:"y"`
	Avatar      string        `json:"avatar,omitempty"`
	Color       string        `json:"color,omitempty"`
	Name        string        `json:"name,omitempty"`
	LastSeen    int64         `json:"lastSeen"`
	Effects     []effectEvent `json:"effects,omitempty"`
	EffectType  string        `json:"effectType,omitempty"`
	EffectNonce int64         `json:"effectNonce,omitempty"`
	Message     string        `json:"message,omitempty"`
	MessageAt   int64         `json:"messageAt,omitempty"`
	// Whether somebody is publishing for this character right now. False means
	// this is where they were left — they are still on the board, because a
	// character does not stop existing when the browser showing them is closed.
	Live bool `json:"live"`
}

// snapshotCursors returns all non-stale cursor entries as a response slice.
// Shared by listCursors (HTTP GET) and the SSE broadcast after updateCursor.
// Every character the board should draw: those being played right now, and
// those merely standing where they were left.
//
// It used to be only the first kind. Presence expires after cursorTTL, so a
// character whose browser closed — or whose tab simply went quiet for eight
// seconds — vanished from the board mid-game, and a cast of characters was only
// ever as visible as the number of browsers open at that moment. Existing and
// being watched are different things, and only one of them should decide
// whether somebody is on the board.
//
// The distinction is kept rather than erased: each entry says whether it is
// live, so a client can draw a character who is present differently from one
// who is only standing there. Everything else — the effects, the message, the
// device — belongs to a live entry and is left empty on the others, which is
// what they are: a position and a person, with nobody driving.
func snapshotCursors() []cursorResponse {
	cutoff := time.Now().Add(-cursorTTL).UnixMilli()
	cursorsMu.RLock()
	result := make([]cursorResponse, 0, len(cursors))
	seen := make(map[string]bool, len(cursors))
	for charID, e := range cursors {
		if e.LastSeen < cutoff {
			continue
		}
		seen[charID] = true
		result = append(result, cursorResponse{
			Live:        true,
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
	// ...and everybody else, where they were last seen. Read under the same
	// lock so a character cannot appear twice by going live between the two
	// halves.
	for charID, e := range lastCursorPos {
		if seen[charID] || drawnFromOwnWant(charID) {
			continue
		}
		result = append(result, cursorResponse{
			CharacterID: charID,
			X:           e.X,
			Y:           e.Y,
			Avatar:      e.Avatar,
			Color:       e.Color,
			Name:        e.Name,
			LastSeen:    e.LastSeen,
		})
	}
	cursorsMu.RUnlock()
	return result
}

// seedLastKnownPositions fills the durable map from where the canvas last
// recorded everybody, so characters are on the board from the moment the server
// is up rather than from the moment somebody opens a browser and republishes.
//
// gui_state is the only thing that outlives a restart here: the presence map is
// memory, and a character nobody is playing publishes nothing to refill it.
func seedLastKnownPositions(state map[string]any) {
	if state == nil {
		return
	}
	cursorsMu.Lock()
	defer cursorsMu.Unlock()
	for _, c := range mywant.ListCharacters() {
		if _, known := lastCursorPos[c.ID]; known {
			continue
		}
		x, okX := cursorCoord(state[cursorStateXPrefix+c.ID])
		y, okY := cursorCoord(state[cursorStateYPrefix+c.ID])
		if !okX || !okY {
			continue
		}
		lastCursorPos[c.ID] = cursorEntry{
			X: x, Y: y, Avatar: c.Avatar, Color: c.Color, Name: c.Name,
			// Long ago on purpose: this is a position, not a sighting, and
			// nothing should read it as somebody having just been here.
			LastSeen: 0,
		}
	}
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
		// Seq, if the client sends one, orders that client's own PUTs for
		// this character — see cursorSeq's own comment. Zero (the default
		// for a caller that omits it) means "no ordering information",
		// never treated as stale.
		Seq int64 `json:"seq,omitempty"`
	}
	if err := DecodeRequest(r, &body); err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "invalid request body", err.Error())
		return
	}

	cursorsMu.Lock()
	if body.Seq > 0 && body.Seq <= cursorSeq[characterID] {
		// A later PUT from the same client already landed and applied —
		// this one represents a position the client itself has already
		// moved past. Nothing in it (position or metadata) is more current
		// than what's already there; apply none of it.
		cursorsMu.Unlock()
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if body.Seq > 0 {
		cursorSeq[characterID] = body.Seq
	}
	prev := cursors[characterID]
	// Somebody who says something without stamping it said it now.
	//
	// The test below is "does this carry a messageAt we have not seen", which
	// silently answered no for a client that sends none: `mywant gui i say`
	// sends the words and no timestamp, so its utterances compared 0 against 0
	// and were taken for a repeat of themselves — never recorded, never spoken.
	// Stamping here is what makes them utterances; browsers already stamp their
	// own, and are unaffected.
	if body.Message != "" && body.MessageAt == 0 {
		if body.Message == prev.Message {
			// The same words, carried along by an ordinary position update —
			// a keepalive, a step, an echo. Keep the moment they were first
			// said, or every republish would count as saying it again and the
			// conversation would fill with copies of one remark.
			body.MessageAt = prev.MessageAt
		} else {
			body.MessageAt = time.Now().UnixMilli()
		}
	}
	// Only the FIRST PUT carrying a given messageAt is a real utterance — that's
	// the one worth archiving. Later PUTs just carry it along.
	// Newly said means said LATER, not merely said differently.
	//
	// A tab republishes what its character is saying on every keepalive, from a
	// copy it adopted earlier. If that copy has fallen behind — the character
	// has said two more things since — the republish carries an old message
	// with an old timestamp, and "different from the current one" counted that
	// as a fresh utterance: an old remark reappeared at the end of the
	// conversation, after the newer ones it had already been replaced by.
	isNewMessage := body.Message != "" && body.MessageAt > prev.MessageAt

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
	lastCursorPos[characterID] = cursors[characterID]
	cursorsMu.Unlock()

	// The want bound to this character follows them, so the card is reachable
	// from where they are. The same mover the robot has always used — it is
	// gated on the want being one that stands for a character, so a starred
	// weather tile is not dragged around by its owner walking.
	s.moveCharacterWant(characterID, body.X, body.Y)

	// Add/remove this character from any form-type:button want's `characters`
	// list depending on whether they just stepped onto or off of its tile.
	s.syncButtonOccupancy(characterID, body.X, body.Y)

	// A new utterance joins the conversation record. Only the first PUT carrying
	// a given messageAt — the later ones are the same words being carried along
	// by position updates, not somebody saying it again.
	if isNewMessage {
		recordSpeech(characterID, body.Message, "say")
		// ...and into their own chat window, which is the same conversation
		// seen from their card rather than from the board.
		s.appendToCharacterChat(characterID, body.Message)
		// ...and if it was addressed to the robot, the robot hears it. Said in
		// the room either way — see forwardToRobotIfAddressed.
		s.forwardToRobotIfAddressed(characterID, body.Message)
	}

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

// rememberCharacterPosition records where a character now is, for a character
// nobody is playing.
//
// The durable roster is normally fed by the browser publishing a cursor. A
// character without one is moved by other means — called across the board,
// taken by `mywant-gui i take`, driven by an agent — and those write the
// position into gui_state instead. This is how that reaches the roster the
// board actually draws from.
//
// The identity fields come from the character store rather than from whatever
// was last published, so somebody who has never had a browser open still
// arrives with their own face and colour.
func rememberCharacterPosition(characterID string, x, y float64) {
	c, ok := mywant.GetCharacter(characterID)
	if !ok || drawnFromOwnWant(characterID) {
		return
	}
	cursorsMu.Lock()
	prev := lastCursorPos[characterID]
	prev.X, prev.Y = x, y
	prev.Avatar, prev.Color, prev.Name = c.Avatar, c.Color, c.Name
	lastCursorPos[characterID] = prev
	cursorsMu.Unlock()
	// Everyone watching sees them arrive, rather than on whoever's next poll.
	go broadcastSSE("cursor", snapshotCursors())
}

// drawnFromOwnWant reports whether the board draws this character from a want
// of their own rather than from their cursor — the robot, which has no browser
// and so no cursor to be drawn from.
//
// Such a character must stay out of the roster entirely. Their want is where
// they are, and a second copy in the roster can only ever disagree with it:
// the want moves when they wander, the roster copy moves when they are called,
// so the two drift apart and the board draws the same robot in two places. The
// one you walked up to is then not the one that moves, which reads as the robot
// flying off the moment you reach it.
func drawnFromOwnWant(characterID string) bool {
	c, ok := mywant.GetCharacter(characterID)
	if !ok || c.AuraCardWantID == "" {
		return false
	}
	want, _, found := mywant.GetGlobalChainBuilder().FindWantByID(c.AuraCardWantID)
	if !found || want == nil {
		return false
	}
	// The same table the mover uses, minus the chat wants: those stand on their
	// character's cell and are drawn by nothing, so their character is still
	// drawn from the cursor and still belongs in the roster.
	return want.Metadata.Type == "robot"
}
