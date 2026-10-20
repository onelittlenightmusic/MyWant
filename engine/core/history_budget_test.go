package mywant

import (
	"strings"
	"testing"
	"time"
)

// A history keeps its newest entries while they fit its budget, and always the
// newest one; the answers are kept whatever their size.
func TestHistoryKeepsWithinBudget(t *testing.T) {
	h := NewHistoryManager()
	big := strings.Repeat("x", 30<<10)
	for i := 0; i < 5; i++ {
		h.StateHistoryRing.Append(StateHistoryEntry{Timestamp: time.Now(), StateValue: map[string]any{"responses": []any{big, i}}})
		h.ResultHistoryRing.Append(ResultHistoryEntry{Timestamp: time.Now(), Result: big + string(rune('a'+i))})
	}
	got := h.GetHistory()
	if n := len(got.StateHistory); n != 2 {
		t.Errorf("kept %d 30KB state entries in a 64KB budget, want 2", n)
	}
	last := got.StateHistory[len(got.StateHistory)-1].StateValue.(map[string]any)["responses"].([]any)[1]
	if last != 4 {
		t.Errorf("newest kept is #%v, want #4", last)
	}
	if n := len(got.ResultHistory); n != 5 {
		t.Errorf("kept %d answers, want all 5", n)
	}

	huge := NewHistoryManager()
	huge.LogHistoryRing.Append(LogHistoryEntry{Timestamp: time.Now(), Logs: strings.Repeat("y", 100<<10)})
	if n := len(huge.GetHistory().LogHistory); n != 1 {
		t.Errorf("an entry over budget on its own was dropped (%d kept)", n)
	}
}
