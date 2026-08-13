package server

import (
	"log"
	"net/http"
	"strconv"
	"strings"
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

// recordRobotSayFromGUIState notices `mywant gui robot say` going past.
//
// The robot's words arrive as a gui_state write rather than as a cursor message
// or a chat want, because the robot has no browser publishing a cursor and its
// want is a coding agent rather than a mailbox. Three ways of speaking, then —
// but one conversation, so this is where the third one joins it.
//
// Gated on the nonce actually changing. The dashboard rewrites the whole
// gui_state block on every debounce, so the same message goes past again and
// again; the nonce is what `say` bumps and a rewrite does not, and it is
// already what tells the frontend a command is new.
func recordRobotSayFromGUIState(want *mywant.Want, updates map[string]any) {
	message, _ := updates["robot_message"].(string)
	if message == "" {
		return
	}
	nonce := toInt64(updates["robot_nonce"])
	if nonce == 0 || nonce == toInt64(want.GetAllState()["robot_nonce"]) {
		return
	}
	recordSpeech(robotCharacterID, message, "say")
}

// toInt64 coerces a JSON number, which arrives as float64 however it was
// written, without caring which shape it took on the way.
func toInt64(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int64:
		return n
	case int:
		return int64(n)
	}
	return 0
}

// ── Addressing the robot ─────────────────────────────────────────────────────

// robotMention is how you hand something to the robot's coding agent.
//
// Everything anybody types used to go there — the header bubble posted
// straight to the robot's webhook, and nothing else reached it. That made the
// agent the default listener for every remark, including the ones meant for
// the room, and made talking to the room impossible from the one input box
// most people use.
//
// So the rule inverted: everything said is speech, and the agent is addressed
// on purpose. One rule, applied where the utterance is recorded, so it holds
// for the header bubble, `mywant gui i say`, a chat want and anything added
// later without each of them having to remember it.
const robotMention = "@robot"

// forwardToRobotIfAddressed hands an utterance to the robot's coding agent when
// it is addressed to the robot, and does nothing otherwise.
//
// The mention is stripped before forwarding: it is how the message was routed,
// not part of what was asked. A message that is only a mention asks nothing and
// is left as the speech it already is.
//
// The robot never triggers itself. Its own answers go through the speech record
// too (see CharacterSpeaks), and one quoting a mention would otherwise ask the
// agent to answer its own reply.
func (s *Server) forwardToRobotIfAddressed(speakerID, text string) {
	if speakerID == robotCharacterID || !mentionsRobot(text) {
		return
	}
	request := strings.TrimSpace(stripRobotMention(text))
	if request == "" {
		return
	}
	robot := s.findWantByIDOrName(robotCharacterID)
	if robot == nil {
		log.Printf("[Speech] %s addressed the robot, but no robot want exists", speakerID)
		return
	}
	// Written straight into the want's state rather than posted back through
	// our own HTTP endpoint: same destination, one fewer round trip, and no way
	// for the forward to fail because the server is busy answering itself.
	storeWebhookMessage(robot, webhookMessage{
		Sender:    speakerID,
		Text:      request,
		Timestamp: time.Now().Format(time.RFC3339),
	}, ccStateCfg)
	log.Printf("[Speech] %s asked the robot: %s", speakerID, request)
}

// mentionsRobot reports whether an utterance is addressed to the robot.
//
// Matched as a whole word so an address is deliberate: "@robotics" is a subject,
// not a summons. Anywhere in the message, not only at the front, because
// "これ直せる? @robot" is the same request as the other way round.
func mentionsRobot(text string) bool {
	lower := strings.ToLower(text)
	for i := 0; ; {
		j := strings.Index(lower[i:], robotMention)
		if j < 0 {
			return false
		}
		end := i + j + len(robotMention)
		if end == len(lower) || !isMentionChar(rune(lower[end])) {
			return true
		}
		i = end
	}
}

// isMentionChar reports whether a rune would continue a mention, so that the
// character after "@robot" decides whether the mention ended there.
func isMentionChar(r rune) bool {
	return r == '_' || r == '-' ||
		(r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}

// stripRobotMention removes the address, leaving what was actually asked.
func stripRobotMention(text string) string {
	lower := strings.ToLower(text)
	for i := 0; ; {
		j := strings.Index(lower[i:], robotMention)
		if j < 0 {
			return text
		}
		start := i + j
		end := start + len(robotMention)
		if end == len(lower) || !isMentionChar(rune(lower[end])) {
			// Joined with a single space, not by concatenating what is left:
			// removing a mention from the middle of a sentence would otherwise
			// leave the spaces that surrounded it sitting next to each other.
			return strings.TrimSpace(
				strings.TrimSpace(text[:start]) + " " + strings.TrimSpace(text[end:]),
			)
		}
		i = end
	}
}
