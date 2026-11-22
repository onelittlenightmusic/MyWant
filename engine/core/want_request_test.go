package mywant

import "testing"

func requestWant(asNew bool) *Want {
	w := &Want{Metadata: Metadata{ID: "w1", Name: "w1"}}
	w.WantTypeDefinition = &WantTypeDefinition{RequestAsNew: asNew}
	w.Spec.Requires = []string{"transit_search"}
	return w
}

func TestRequestAchievedFor(t *testing.T) {
	w := requestWant(true)
	r := w.ensureRequest()
	if r.N != 1 || r.Started {
		t.Fatalf("first request = %+v, want #1 not started", r)
	}

	// An answer held but not given to this request: not achieved.
	if w.AchievedNow(true) {
		t.Fatal("achieved before any agent answered this request")
	}
	// An agent finished a run for it: achieved, and recorded.
	w.noteAgentRan()
	if !w.AchievedNow(true) {
		t.Fatal("not achieved after its agent answered")
	}
	if !w.forRequest(AchievedForState) {
		t.Fatal("achievement not recorded for request #1")
	}

	// The server starting again: the same request, its record kept, no agent
	// run yet in this process — still achieved, nothing to run.
	w.markRequestStarted(r)
	boot := requestWant(true)
	w.State.Range(func(k, v any) bool { boot.State.Store(k, v); return true })
	if r, _ := boot.Request(); !r.Started || r.N != 1 {
		t.Fatalf("request after boot = %+v, want #1 started (resumed)", r)
	}
	if !boot.AchievedNow(true) {
		t.Fatal("a server start lost the achievement of the request it was on")
	}

	// Asked anew: the answer it holds is to the request before.
	nr := boot.NewRequest(RequestByRestart)
	if nr.N != 2 || nr.Started || nr.By != RequestByRestart {
		t.Fatalf("new request = %+v, want #2 by restart, not started", nr)
	}
	if boot.AchievedNow(true) {
		t.Fatal("still achieved after being asked anew, before answering")
	}
	boot.noteAgentRan()
	if !boot.AchievedNow(true) {
		t.Fatal("not achieved after answering the new request")
	}
}

func TestRequestFailedFor(t *testing.T) {
	w := requestWant(true)
	w.ensureRequest()
	w.noteAgentRan()
	if !w.FailedNow(true) {
		t.Fatal("a failure of this request did not fail it")
	}
	// Asked anew: the error kept from before does not fail it again.
	w.NewRequest(RequestByRetry)
	if w.FailedNow(true) {
		t.Fatal("an error from the request before failed the new one")
	}
}

func TestRequestNotAsNew(t *testing.T) {
	w := requestWant(false)
	if r := w.NewRequest(RequestByRestart); r.N != 0 {
		t.Fatalf("a type without requestAsNew was given request %+v", r)
	}
	if _, ok := w.Request(); ok {
		t.Fatal("a type without requestAsNew holds a request")
	}
	// Its conditions alone decide, as they always have.
	if !w.AchievedNow(true) || w.AchievedNow(false) || !w.FailedNow(true) {
		t.Fatal("a type without requestAsNew is not decided by its own conditions")
	}
}
