package mywant

import (
	"context"
	"strings"
	"testing"
	"time"
)

// A call's context ending says which of the two ended it: its own timeout, or
// its want stopping.
func TestCallEndedSaysWhy(t *testing.T) {
	timed, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	<-timed.Done()
	if got := callEnded("skill", timed).Error(); !strings.HasPrefix(got, "skill timed out") {
		t.Fatalf("deadline: %q", got)
	}

	parent, stop := context.WithCancel(context.Background())
	inner, cancel2 := context.WithTimeout(parent, time.Hour)
	defer cancel2()
	stop() // the want's agents stopped
	<-inner.Done()
	if got := callEnded("skill", inner).Error(); !strings.HasPrefix(got, "skill cancelled") {
		t.Fatalf("cancel: %q", got)
	}
}

// Through a real call: a skill still running when its want stops (a restart
// cancels the agents' context) says cancelled; one that runs past its own
// timeout says timed out.
func TestMRSCallEndedSaysWhy(t *testing.T) {
	mrsNeedPython(t)
	script := mrsTestScript(t, t.TempDir()+"/slow", "import time, json\ntime.sleep(5)\nprint(json.dumps({'ok': True}))\n")

	// The want's agents stopped, half a second in.
	agents, stop := context.WithCancel(context.Background())
	skill, cancel := context.WithTimeout(agents, 20*time.Second)
	defer cancel()
	time.AfterFunc(500*time.Millisecond, stop)
	_, err := RunMRSScript(skill, script, MRSRunOptions{})
	if err == nil || !strings.HasPrefix(err.Error(), "skill cancelled") {
		t.Fatalf("want stopped mid-call: got %v", err)
	}

	// Its own timeout ran out.
	skill2, cancel2 := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel2()
	_, err = RunMRSScript(skill2, script, MRSRunOptions{})
	if err == nil || !strings.HasPrefix(err.Error(), "skill timed out") {
		t.Fatalf("timeout: got %v", err)
	}
}
