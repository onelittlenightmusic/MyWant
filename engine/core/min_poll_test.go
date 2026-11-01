package mywant

import (
	"testing"
	"time"
)

func TestAtLeastMinPoll(t *testing.T) {
	t.Setenv("MYWANT_MIN_POLL_MS", "")
	if got := atLeastMinPoll(time.Second); got != time.Second {
		t.Fatalf("unset: %v", got)
	}
	t.Setenv("MYWANT_MIN_POLL_MS", "5000")
	if got := atLeastMinPoll(time.Second); got != 5*time.Second {
		t.Fatalf("raised: %v", got)
	}
	if got := atLeastMinPoll(time.Minute); got != time.Minute {
		t.Fatalf("a longer one kept: %v", got)
	}
	t.Setenv("MYWANT_MIN_POLL_MS", "x")
	if got := atLeastMinPoll(time.Second); got != time.Second {
		t.Fatalf("invalid ignored: %v", got)
	}
}
