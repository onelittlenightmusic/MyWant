package mywant

import (
	"context"
	"fmt"
	"testing"
)

// A restart for a request, and a call it cuts short, both name the request.
func TestRestartNamesItsRequest(t *testing.T) {
	w := &Want{Metadata: Metadata{Name: "w"}}
	w.RestartWantBy("q7")
	w.stopAgents("before restart")
	if got := w.StopReason(); got != "restart by q7" {
		t.Fatalf("stop reason: %q", got)
	}
	err := fmt.Errorf("skill cancelled (its want stopped or restarted): %w", context.Canceled)
	if got := callFailure(w, err); got != err.Error()+" (restart by q7)" {
		t.Fatalf("failed line: %q", got)
	}

	// Named once: the next restart, of the engine's own accord, does not
	// borrow it.
	w.stopAgents("on restart")
	if got := w.StopReason(); got != "restart" {
		t.Fatalf("second stop: %q", got)
	}
	w.stopAgents("on archive")
	if got := w.StopReason(); got != "archive" {
		t.Fatalf("archive: %q", got)
	}

	// A failure that is not a cancel says nothing about stopping.
	if got := callFailure(w, fmt.Errorf("boom")); got != "boom" {
		t.Fatalf("plain failure: %q", got)
	}
}

func TestNewRequestIDIsDistinct(t *testing.T) {
	a, b := NewRequestID("q"), NewRequestID("q")
	if a == b || a[0] != 'q' {
		t.Fatalf("%q %q", a, b)
	}
}
