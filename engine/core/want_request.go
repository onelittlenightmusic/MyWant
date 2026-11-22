package mywant

import (
	"time"
)

// want_request.go — what a want is asked for, and which asking its answer is to.
//
// Only for a want whose restarts are new requests (its type's requestAsNew): one whose answer is fixed by when it was asked, as a
// transit search's departure is. Every other want restarts as it always has:
// its state reset to what its type starts a run with, its conditions read.
//
// A want is achieved (or failed) FOR a request. Each time it is asked anew —
// made, restarted (a person's 再実行 / 再検索, the engine's own restart: a
// timing's wake, a changed parameter) — its request is the next one, and an
// answer it had is the answer to the one before: kept, shown, but no longer
// what was asked. It is achieved again only once it has answered this one.
//
// The server starting is not asking: a want comes back to the request it had,
// achieved if it had answered it, and nothing is run again. Before this the
// start went through the same reset a restart does (prepareForRestart), and
// "achieved" was only "the field the type reads is set" — so a type either kept
// its answer and could not be asked again (a transit search sat on yesterday's
// route through every 再検索), or let the answer go and asked again at every
// start (it searched again whenever the server came up).
//
//	_request:      {n, requested_at, by}   the request it is on
//	_achieved_for: n                       the request it achieved for
//	_failed_for:   n                       the request it failed for
//
// All three persist; the type's own condition (finalizeWhen, IsAchieved) still
// says whether what it holds is an answer. What this adds is whether that
// answer was given to the request it is on: an agent of its own finished a run
// since it was asked, or the field the condition reads changed since then.

// RequestState holds the request a want is on.
const RequestState = "_request"

// AchievedForState and FailedForState hold the request a want achieved or
// failed for.
const (
	AchievedForState = "_achieved_for"
	FailedForState   = "_failed_for"
)

// What made a request.
const (
	RequestByCreate  = "create"
	RequestByRestart = "restart" // a person, a tool, the API
	RequestByParams  = "params"  // its parameters changed
	RequestByWake    = "wake"    // its own time came (a timing, a schedule)
	RequestByRetry   = "retry"   // a passing failure run again
	RequestByInput   = "input"   // what it uses gave something new
)

// WantRequest is the request a want is on.
type WantRequest struct {
	N           int    `json:"n"`
	RequestedAt string `json:"requested_at"`
	By          string `json:"by"`
	// Started: its run has been prepared (the state reset to what the type
	// starts a run with). A request is prepared once; the server starting
	// again on it resumes it as it was.
	Started bool `json:"started"`
}

func (r WantRequest) toMap() map[string]any {
	return map[string]any{"n": r.N, "requested_at": r.RequestedAt, "by": r.By, "started": r.Started}
}

// RequestsAsNew: a restart of this want is a new request (its type's
// requestAsNew).
func (n *Want) RequestsAsNew() bool {
	return n.WantTypeDefinition != nil && n.WantTypeDefinition.RequestAsNew
}

// Request returns the request the want is on, and whether it has one.
func (n *Want) Request() (WantRequest, bool) {
	v, ok := n.State.Load(RequestState)
	if !ok || v == nil {
		return WantRequest{}, false
	}
	m, ok := v.(map[string]any)
	if !ok {
		return WantRequest{}, false
	}
	r := WantRequest{N: ToInt(m["n"], 0)}
	if r.N <= 0 {
		return WantRequest{}, false
	}
	r.RequestedAt, _ = m["requested_at"].(string)
	r.By, _ = m["by"].(string)
	r.Started, _ = m["started"].(bool)
	return r, true
}

// requestN is the number of the request the want is on, 0 with none.
func (n *Want) requestN() int {
	r, _ := n.Request()
	return r.N
}

// ensureRequest gives a want its first request if it has none: one just made,
// or one kept from before requests were (it is asked once more, and answers
// it, the one time).
func (n *Want) ensureRequest() WantRequest {
	if r, ok := n.Request(); ok {
		return r
	}
	r := WantRequest{N: 1, RequestedAt: time.Now().Format(time.RFC3339), By: RequestByCreate}
	n.StoreState(RequestState, r.toMap())
	return r
}

// NewRequest asks the want anew: its answer, if it had one, is to the request
// before. `by` is what asked (RequestByRestart, RequestByWake, …).
func (n *Want) NewRequest(by string) WantRequest {
	if !n.RequestsAsNew() {
		return WantRequest{}
	}
	prev, _ := n.Request()
	r := WantRequest{N: prev.N + 1, RequestedAt: time.Now().Format(time.RFC3339), By: by}
	n.StoreState(RequestState, r.toMap())
	n.StoreLog("[REQUEST] Asked anew (#%d, by %s)", r.N, by)
	return r
}

// markRequestStarted: the request's run is prepared.
func (n *Want) markRequestStarted(r WantRequest) {
	r.Started = true
	n.StoreState(RequestState, r.toMap())
}

// noteAgentRan: an agent of this want finished a run (a do agent's, a
// monitor's tick) — an answer, or a failure, to the request it is on.
func (n *Want) noteAgentRan() {
	n.answeredRequest.Store(int64(n.requestN()))
}

// gateFields names the fields a want's own conditions read, if its type says
// (finalizeWhen): an answer to this request may come as a change of one.
type gateFields interface {
	achieveGateField() string
	failGateField() string
}

// answeredNow says whether the want has answered the request it is on: an
// agent of its own finished a run since it was asked, or `field` (the one its
// condition reads) changed since then. A want with no agents and no field it
// is known to read is answered by whatever it does itself.
func (n *Want) answeredNow(field string) bool {
	req := n.requestN()
	if req > 0 && n.answeredRequest.Load() == int64(req) {
		return true
	}
	if field != "" {
		// Changed since it was asked — and in this run of it: a value read
		// back from the memory file as the server starts was written then,
		// and is no answer.
		r, ok := n.Request()
		if !ok {
			return false
		}
		at, err := time.Parse(time.RFC3339, r.RequestedAt)
		if err != nil {
			return false
		}
		if started := time.Unix(0, n.runStartedAt.Load()); started.After(at) {
			at = started
		}
		ts, ok := n.stateTimestamps.Load(field)
		t, isTime := ts.(time.Time)
		return ok && isTime && t.After(at)
	}
	return len(n.Spec.Requires) == 0
}

// forRequest says whether `stateKey` holds the request the want is on.
func (n *Want) forRequest(stateKey string) bool {
	req := n.requestN()
	if req == 0 {
		return false
	}
	v, ok := n.State.Load(stateKey)
	return ok && ToInt(v, 0) == req
}

// AchievedNow: what the want holds is an answer (`cond`, its type's
// condition) and an answer to the request it is on. The first time both hold
// it is recorded (_achieved_for), so a server starting again finds it.
func (n *Want) AchievedNow(cond bool) bool {
	if !cond || !n.RequestsAsNew() {
		return cond
	}
	if n.forRequest(AchievedForState) {
		return true
	}
	field := ""
	if g, ok := n.progressable.(gateFields); ok {
		field = g.achieveGateField()
	}
	if !n.answeredNow(field) {
		return false
	}
	if req := n.requestN(); req > 0 {
		n.StoreState(AchievedForState, req)
	}
	return true
}

// FailedNow is AchievedNow for failing: a failure the want holds counts once it
// is the request it is on that failed — an error kept from the request before
// does not fail it again.
func (n *Want) FailedNow(cond bool) bool {
	if !cond || !n.RequestsAsNew() {
		return cond
	}
	if n.forRequest(FailedForState) {
		return true
	}
	field := ""
	if g, ok := n.progressable.(gateFields); ok {
		field = g.failGateField()
	}
	if !n.answeredNow(field) {
		return false
	}
	if req := n.requestN(); req > 0 {
		n.StoreState(FailedForState, req)
	}
	return true
}

// achievedNow / failedNow read the want's own conditions.
func (n *Want) achievedNow() bool {
	return n.progressable != nil && n.AchievedNow(n.progressable.IsAchieved())
}

func (n *Want) failedNow() bool {
	f, ok := n.progressable.(Failable)
	return ok && n.FailedNow(f.IsFailed())
}
