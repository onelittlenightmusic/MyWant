package types

import (
	"testing"

	. "mywant/engine/core"
)

// The idempotency log exists to stop one request being sent twice. Everything
// here is about the other half of that promise, which it used to break: a
// request that is NOT the same one must never be mistaken for it.

// newCCWant builds a want whose state keys are declared, since SetGoal/SetCurrent
// on an undeclared key is silently ignored — the engine's own rule, and one that
// would otherwise make every assertion here read defaults and pass for the wrong
// reason.
func newCCWant() *Want {
	w := &Want{Metadata: Metadata{ID: "robot", Type: "robot"}}
	w.StateLabels = map[string]StateLabel{
		"session_id":           LabelGoal,
		"auto_request":         LabelGoal,
		"webhook_auto_request": LabelCurrent,
		"cc_message_count":     LabelCurrent,
		"cc_latest_message":    LabelCurrent,
		"request_count":        LabelCurrent,
		"matched_content":      LabelCurrent,
	}
	return w
}

func webhookWant(sessionID, text, stamp string, msgCount, reqCount int) *Want {
	w := newCCWant()
	w.SetGoal("session_id", sessionID)
	w.SetCurrent("webhook_auto_request", text)
	w.SetCurrent("cc_message_count", msgCount)
	w.SetCurrent("request_count", reqCount)
	if stamp != "" {
		w.SetCurrent("cc_latest_message", map[string]any{"text": text, "timestamp": stamp})
	}
	return w
}

// Two different things to say are two different requests. Hashing a counter
// instead of the message meant they could collide, and the loser was dropped in
// silence — forever, since the counter only moves when a send completes.
func TestWebhookRequestIDDiffersPerMessage(t *testing.T) {
	a := deriveClaudeRequestID(webhookWant("s1", "最初のドアを開けて", "2026-08-13T18:48:24+09:00", 4, 8))
	b := deriveClaudeRequestID(webhookWant("s1", "ドアを開けて", "2026-08-13T18:49:55+09:00", 5, 8))
	if a == b {
		t.Errorf("two different messages hashed the same id (%s)", a)
	}
}

// Said twice, deliberately: still two requests, because they are two turns of
// the conversation.
func TestWebhookRequestIDDiffersWhenRepeated(t *testing.T) {
	a := deriveClaudeRequestID(webhookWant("s1", "もう一度", "2026-08-13T18:48:24+09:00", 4, 8))
	b := deriveClaudeRequestID(webhookWant("s1", "もう一度", "2026-08-13T18:50:01+09:00", 5, 8))
	if a == b {
		t.Errorf("the same words said twice hashed one id (%s); each is its own turn", a)
	}
}

// The half that must keep working: one message, sent once. A server restart
// re-derives the id from the message, which has not changed, so the log still
// recognises it.
func TestWebhookRequestIDStableAcrossRestart(t *testing.T) {
	before := deriveClaudeRequestID(webhookWant("s1", "ドアを開けて", "2026-08-13T18:49:55+09:00", 5, 8))
	// A restart loses the in-memory counter — the very thing the old id was
	// built from — but the message survives in the want's state.
	after := deriveClaudeRequestID(webhookWant("s1", "ドアを開けて", "2026-08-13T18:49:55+09:00", 5, 0))
	if before != after {
		t.Errorf("the same message hashed %s before a restart and %s after; it would be sent twice", before, after)
	}
}

// The deadlock, stated directly: request_count could not advance without a
// completed send, and the send was refused because an id built from
// request_count already existed. A message's id must not depend on it at all.
func TestWebhookRequestIDIgnoresRequestCount(t *testing.T) {
	for _, count := range []int{0, 7, 8, 42} {
		got := deriveClaudeRequestID(webhookWant("s1", "ドアを開けて", "2026-08-13T18:49:55+09:00", 5, count))
		want := deriveClaudeRequestID(webhookWant("s1", "ドアを開けて", "2026-08-13T18:49:55+09:00", 5, 0))
		if got != want {
			t.Errorf("request_count=%d changed the message's id (%s vs %s)", count, got, want)
		}
	}
}

// Two people, two sessions, same words: not the same request.
func TestWebhookRequestIDIsPerSession(t *testing.T) {
	a := deriveClaudeRequestID(webhookWant("s1", "ドアを開けて", "2026-08-13T18:49:55+09:00", 5, 8))
	b := deriveClaudeRequestID(webhookWant("s2", "ドアを開けて", "2026-08-13T18:49:55+09:00", 5, 8))
	if a == b {
		t.Errorf("two sessions shared one id (%s)", a)
	}
}

// With nothing queued there is no message to identify, and the autonomous
// derivation — matched content, session state, the counter — takes over
// unchanged.
func TestAutonomousRequestIDUnchanged(t *testing.T) {
	w := newCCWant()
	w.SetGoal("session_id", "s1")
	w.SetGoal("auto_request", "keep going")
	w.SetCurrent("request_count", 3)

	first := deriveClaudeRequestID(w)
	w.SetCurrent("request_count", 4)
	second := deriveClaudeRequestID(w)
	if first == second {
		t.Error("an autonomous trigger's id no longer advances with request_count")
	}
}
