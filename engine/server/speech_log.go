package server

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	mywant "mywant/engine/core"
)

// Who said what, in the order it was said.
//
// Each browser could work this out for itself — the messages are in the wants
// and the cursors, and a tab watching both can see them arrive. But then the
// conversation exists only where somebody was already watching: a tab opened a
// moment later shows an empty column, two tabs can disagree about the order
// when their polls interleave differently, and nothing outside the browser can
// read it at all. An ordering is a fact about the world, so the world keeps it.
//
// Deliberately small and in memory. This is the last few things that were said,
// for a panel that shows the last few things that were said — the durable record
// of a conversation is the chat want's own message history, and the work log
// keeps the archive. Losing this on restart costs a column that repopulates as
// soon as anyone speaks.

// SpeechEntry is one thing a character said.
type SpeechEntry struct {
	CharacterID string `json:"characterId"`
	Text        string `json:"text"`
	// Unix ms. The sort key, and what a client's expiry counts from.
	At int64 `json:"at"`
	// How it was said: "say" (a cursor message, e.g. `mywant gui i say`) or
	// "chat" (posted to the character's chat want). Kept because they are
	// different acts even though they land in the same column.
	Source string `json:"source"`
}

const speechLogMax = 200

var (
	speechMu  sync.RWMutex
	speechLog []SpeechEntry // oldest first
)

// recordSpeech appends an utterance. Empty text is not an utterance.
func recordSpeech(characterID, text, source string) {
	if characterID == "" || text == "" {
		return
	}
	speechMu.Lock()
	speechLog = append(speechLog, SpeechEntry{
		CharacterID: characterID,
		Text:        text,
		At:          time.Now().UnixMilli(),
		Source:      source,
	})
	if len(speechLog) > speechLogMax {
		speechLog = speechLog[len(speechLog)-speechLogMax:]
	}
	speechMu.Unlock()
}

// listSpeech handles GET /api/v1/speech — the recent conversation, oldest
// first, so a client can render it top to bottom without re-sorting.
//
// ?limit=N trims to the newest N, still in order. ?since=<unix ms> returns only
// what was said after a moment the caller already has, which is what makes this
// pollable without re-reading the whole column.
func (s *Server) listSpeech(w http.ResponseWriter, r *http.Request) {
	limit := speechLogMax
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n < speechLogMax {
			limit = n
		}
	}
	var since int64
	if v := r.URL.Query().Get("since"); v != "" {
		since, _ = strconv.ParseInt(v, 10, 64)
	}

	speechMu.RLock()
	out := make([]SpeechEntry, 0, len(speechLog))
	for _, e := range speechLog {
		if e.At > since {
			out = append(out, e)
		}
	}
	speechMu.RUnlock()

	if len(out) > limit {
		out = out[len(out)-limit:]
	}
	s.JSONResponse(w, http.StatusOK, out)
}

// characterIDForChatWant answers whose voice a chat want is, so a message
// posted to it can be recorded against a person rather than against a want.
//
// The want's own parameter is asked first because it is what the want was
// created with; the character's binding is the fallback for one that was
// pointed at a character by hand.
func characterIDForChatWant(want *mywant.Want) string {
	if want == nil {
		return ""
	}
	if id, ok := want.Spec.Params["character_id"].(string); ok && id != "" {
		return id
	}
	wantID := want.Metadata.ID
	for _, c := range mywant.ListCharacters() {
		if c.AuraCardWantID == wantID {
			return c.ID
		}
	}
	return ""
}
