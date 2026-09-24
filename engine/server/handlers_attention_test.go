package server

import "testing"

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
