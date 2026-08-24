package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
)

// putCursorSeq issues one PUT /api/v1/cursors/:characterId against a bare
// Server, the same way updateCursor's own callers do — through the real
// handler, not by poking cursors/cursorSeq directly, so this exercises the
// actual decode-then-check order.
func putCursorSeq(t *testing.T, s *Server, characterID string, x, y float64, seq int64) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"x": x, "y": y, "seq": seq})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/cursors/"+characterID, bytes.NewReader(body))
	req = mux.SetURLVars(req, map[string]string{"characterId": characterID})
	w := httptest.NewRecorder()
	s.updateCursor(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("PUT seq=%d: expected 204, got %d: %s", seq, w.Code, w.Body.String())
	}
}

func resetCursorState() {
	cursorsMu.Lock()
	cursors = map[string]cursorEntry{}
	cursorSeq = map[string]int64{}
	cursorsMu.Unlock()
}

// This is the actual bug: a client's own rapid-fire position PUTs can reach
// the server out of the order they were sent — ordinary network jitter
// across concurrent requests, nothing exotic — and without a sequence
// number, whichever one is *processed* last simply overwrites the
// character's position, which is not necessarily the one the client sent
// last. The character's own cursor then visibly regresses to a cell it has
// already walked past.
func TestUpdateCursorRejectsOutOfOrderSeq(t *testing.T) {
	resetCursorState()
	s := &Server{}
	const char = "chr-seq-test"

	putCursorSeq(t, s, char, 3, 0, 3) // "step 3" arrives first
	putCursorSeq(t, s, char, 2, 0, 2) // "step 2", sent earlier, arrives late

	cursorsMu.RLock()
	got := cursors[char]
	cursorsMu.RUnlock()
	if got.X != 3 {
		t.Fatalf("expected the out-of-order seq=2 PUT to be rejected, position still at x=3, got x=%v", got.X)
	}
}

// A seq that keeps climbing is the normal case and must keep applying
// normally — this isn't a one-shot latch, every genuinely newer PUT still
// has to land.
func TestUpdateCursorAppliesIncreasingSeq(t *testing.T) {
	resetCursorState()
	s := &Server{}
	const char = "chr-seq-test-2"

	putCursorSeq(t, s, char, 1, 0, 1)
	putCursorSeq(t, s, char, 2, 0, 2)
	putCursorSeq(t, s, char, 3, 0, 3)

	cursorsMu.RLock()
	got := cursors[char]
	cursorsMu.RUnlock()
	if got.X != 3 {
		t.Fatalf("expected the latest seq=3 PUT to apply, got x=%v", got.X)
	}
}

// seq=0 (a caller that never sends one — an older client, the CLI,
// anything else hitting this endpoint) must never be treated as stale: the
// check is opt-in per request, not a hard requirement to move at all.
func TestUpdateCursorIgnoresOrderingWithoutSeq(t *testing.T) {
	resetCursorState()
	s := &Server{}
	const char = "chr-seq-test-3"

	putCursorSeq(t, s, char, 5, 0, 0)
	putCursorSeq(t, s, char, 6, 0, 0)

	cursorsMu.RLock()
	got := cursors[char]
	cursorsMu.RUnlock()
	if got.X != 6 {
		t.Fatalf("expected the second no-seq PUT to apply normally, got x=%v", got.X)
	}
}

// A PUT whose position lost the race still delivers what else it carries.
//
// The stale-seq check used to answer 204 and apply none of the request. That is
// right for a position — a newer one has already landed — but a message is an
// event, not state: it has never been said before, whatever seq it arrived
// under. So speaking while walking could silently swallow the words, which is
// the same loss the blocked-move path takes care to avoid.
func TestStaleSeqStillDeliversItsMessage(t *testing.T) {
	resetCursorState()
	s := &Server{}

	putCursorSeq(t, s, "chr", 5, 5, 10)

	// Sent before that one, delivered after it, and carrying a remark.
	body, _ := json.Marshal(map[string]any{
		"x": 1, "y": 1, "seq": 9, "message": "wait for me", "messageAt": 1234,
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/cursors/chr", bytes.NewReader(body))
	req = mux.SetURLVars(req, map[string]string{"characterId": "chr"})
	w := httptest.NewRecorder()
	s.updateCursor(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}

	cursorsMu.RLock()
	got := cursors["chr"]
	cursorsMu.RUnlock()

	if got.X != 5 || got.Y != 5 {
		t.Errorf("position moved back to (%v,%v); the newer PUT's (5,5) should stand", got.X, got.Y)
	}
	if got.Message != "wait for me" {
		t.Errorf("message = %q, want it delivered despite the stale position", got.Message)
	}
}
