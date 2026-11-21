package mywant

import (
	"regexp"
	"strings"
	"time"
)

// A want that failed for a reason that passes by itself — the network down a
// moment, a name not resolving, a server timing out or answering 5xx — is run
// again on its own (its wake, WakeAt): in a minute, five, fifteen, thirty,
// then every hour until it answers. Meanwhile it is not something a person
// has to see (the server's attention leaves it out) unless it has been
// failing for retryAttentionAfter. A failure only a person can mend — its
// settings wrong, its type broken — is not run again and is attention at once.
//
// Kept in its state, apart from its type's fields, so a restart of the want or
// the server keeps it: RetryCountState how many times it has been run again,
// FailingSinceState when it began to fail (RFC3339), RetryingState whether it
// is being run again. A success clears them.

const (
	RetryCountState   = "retry_count"
	FailingSinceState = "failing_since"
	RetryingState     = "retrying"
)

// retryBackoff is how long after each failure the want is run again; after
// the last, every last.
var retryBackoff = []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute, 30 * time.Minute, time.Hour}

// RetryAttentionAfter is how long a passing failure may last before a person
// is told of it (the attention list): an hour of trying is not passing.
const RetryAttentionAfter = time.Hour

// transientFailure matches the error of a failure that passes by itself.
var transientFailure = regexp.MustCompile(`(?i)` + strings.Join([]string{
	`no such host`, `name does not resolve`, `name or service not known`,
	`temporary failure in name resolution`, `server misbehaving`,
	`connection refused`, `connection reset`, `broken pipe`, `network is unreachable`,
	`no route to host`, `timeout`, `timed out`, `deadline exceeded`, `tls handshake`,
	`unexpected eof`, `\b50[234]\b`, `bad gateway`, `service unavailable`, `gateway timeout`,
	`too many requests`, `\b429\b`, `is not answering`, `接続できません`,
}, "|"))

// IsTransientFailure says whether an error is of the kind that passes by itself.
func IsTransientFailure(err string) bool {
	return strings.TrimSpace(err) != "" && transientFailure.MatchString(err)
}

// noteFailed is a want just failed: a passing failure is run again later, the
// count and when it began kept; any other is left for a person.
func (n *Want) noteFailed() {
	errText, _ := n.GetStateString(StateFieldError, "")
	if !IsTransientFailure(errText) {
		// Only said when it was being run again: a want that never was has
		// nothing to change (and a write would be a change for nothing).
		if retrying, _ := n.GetStateBool(RetryingState, false); retrying {
			n.StoreState(RetryingState, false)
		}
		return
	}
	now := time.Now()
	if since, _ := n.GetStateString(FailingSinceState, ""); since == "" {
		n.StoreState(FailingSinceState, now.Format(time.RFC3339))
	}
	count, _ := n.GetStateInt(RetryCountState, 0)
	wait := retryBackoff[len(retryBackoff)-1]
	if count < len(retryBackoff) {
		wait = retryBackoff[count]
	}
	n.StoreState(RetryCountState, count+1)
	n.StoreState(RetryingState, true)
	n.WakeAt(now.Add(wait))
	n.StoreLog("[RETRY] failed for a passing reason; run again in %s (try %d): %s", wait, count+1, firstLineOf(errText))
}

// noteAnswered is a want that answered: whatever failing it was doing is over.
func (n *Want) noteAnswered() {
	if v, ok := n.State.Load(RetryingState); !ok || v == nil {
		if c, _ := n.GetStateInt(RetryCountState, 0); c == 0 {
			return
		}
	}
	n.StoreState(RetryCountState, 0)
	n.StoreState(FailingSinceState, "")
	n.StoreState(RetryingState, false)
}

// StillRetrying says whether a failed want is being run again for a passing
// failure that has not yet lasted RetryAttentionAfter — not yet a person's.
func StillRetrying(w *Want, now time.Time) bool {
	retrying, _ := w.GetStateBool(RetryingState, false)
	if !retrying {
		return false
	}
	since, _ := w.GetStateString(FailingSinceState, "")
	at, err := time.Parse(time.RFC3339, since)
	if err != nil {
		return true
	}
	return now.Sub(at) < RetryAttentionAfter
}

func firstLineOf(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 160 {
		s = s[:160]
	}
	return s
}
