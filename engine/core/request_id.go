package mywant

import (
	"context"
	"errors"
	"strconv"
	"sync/atomic"
)

// A config change's request ID, put on the log lines that already exist, so
// why a want restarted can be traced back afterwards: the server gives each
// PUT / PATCH of a want an ID and puts it on that request's API log entry;
// the restart the change causes names it, and so does a call that restart
// cuts short —
//
//	API log:  PATCH /api/v1/wants/{id} … requestId q1a7
//	[足止めの塔] [RESTART] Want '足止めの塔' restarting execution (by q1a7)
//	[足止めの塔] [MRS-MONITOR:battle_turret_agent] failed: skill cancelled … (restart by q1a7)
//
// A counter rather than a random string: cheap, and in order. It starts over
// with the server, so an ID is read with the time on its line.
var requestSeq atomic.Uint64

// NewRequestID is the next ID, starting with prefix.
func NewRequestID(prefix string) string {
	return prefix + strconv.FormatUint(requestSeq.Add(1), 36)
}

// SetChangeBy records the request this config came in with, for the restart
// it causes to name (applyWantChanges).
func (n *Want) SetChangeBy(requestID string) { n.changeBy = requestID }

// ChangeBy is the request SetChangeBy recorded, or "".
func (n *Want) ChangeBy() string { return n.changeBy }

// noteStop records why the want's agents are about to be stopped, for a call
// that stop cuts short to say (StopReason). A restart takes the request it was
// for (RestartWantBy), once.
func (n *Want) noteStop(reason string) {
	if reason == "restart" {
		if by := n.restartBy.Swap(nil); by != nil {
			reason = "restart by " + *by
		}
	}
	n.stopReason.Store(&reason)
}

// StopReason is why the want's agents were last stopped, or "".
func (n *Want) StopReason() string {
	if r := n.stopReason.Load(); r != nil {
		return *r
	}
	return ""
}

// callFailure is the "failed" line's tail for a call: the error, and when the
// want stopping cut it short, why it stopped.
func callFailure(want *Want, err error) string {
	if errors.Is(err, context.Canceled) {
		if r := want.StopReason(); r != "" {
			return err.Error() + " (" + r + ")"
		}
	}
	return err.Error()
}
