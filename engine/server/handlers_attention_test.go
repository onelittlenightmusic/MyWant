package server

import (
	"testing"
	"time"
)

// A failed run files the tab; a needs-human verdict replaces it; the next
// clean run of the same URL clears it.
func TestRecordWebRunOutcome(t *testing.T) {
	const url = "https://example.com/inbox"
	t.Cleanup(func() { delete(webAttention, url) })

	recordWebRunOutcome(url, browserRunResult{Error: "selector not found"})
	if got := webAttention[url]; got.Kind != "web_failed" || got.URL != url {
		t.Fatalf("after error: %+v", got)
	}
	recordWebRunOutcome(url, browserRunResult{Result: map[string]any{"needs_human": "login"}})
	if got := webAttention[url]; got.Kind != "needs_human" || got.Detail != "login" {
		t.Fatalf("after needs_human: %+v", got)
	}
	recordWebRunOutcome(url, browserRunResult{Result: map[string]any{"ok": true}})
	if _, still := webAttention[url]; still {
		t.Fatalf("a clean run should clear the tab")
	}
	recordWebRunOutcome("", browserRunResult{Error: "x"})
	if _, filed := webAttention[""]; filed {
		t.Fatalf("a run with no URL has no tab to file")
	}
}

// An alert keeps the moment it began for as long as it holds — so it is the
// same item on every read — and starts afresh after it stops.
func TestAlertBegan(t *testing.T) {
	const id = "alert:w1:雨で中止"
	t.Cleanup(func() { forgetAlertsNotIn(nil) })

	t0 := time.UnixMilli(1_000_000)
	if got := alertBegan(id, t0); got != t0.UnixMilli() {
		t.Fatalf("first sight: %d", got)
	}
	if got := alertBegan(id, t0.Add(time.Minute)); got != t0.UnixMilli() {
		t.Fatalf("still holding should keep its start: %d", got)
	}
	forgetAlertsNotIn(map[string]bool{})
	t1 := t0.Add(time.Hour)
	if got := alertBegan(id, t1); got != t1.UnixMilli() {
		t.Fatalf("holding again is a new item: %d", got)
	}
}
