package mywant

import (
	"testing"
	"time"
)

func TestScheduleOfFallsBackToTypeDefaultEvery(t *testing.T) {
	def := &WantTypeDefinition{}
	def.Metadata.Labels = map[string]string{defaultEveryLabel: " 1h "}

	w := &Want{WantTypeDefinition: def}
	if got := scheduleOf(w); len(got) != 1 || got[0].Every != "1h" {
		t.Fatalf("type default: got %+v, want every 1h", got)
	}

	w.Spec.When = []WhenSpec{{Every: "10m"}}
	if got := scheduleOf(w); len(got) != 1 || got[0].Every != "10m" {
		t.Fatalf("own spec.when wins: got %+v", got)
	}

	if got := scheduleOf(&Want{}); got != nil {
		t.Fatalf("no schedule: got %+v", got)
	}
}

func TestBookWakeIsDoneOnce(t *testing.T) {
	b := newScheduleBook()
	now := time.Now()
	at := now.Add(time.Minute).Truncate(time.Second)
	b.put("w1", "golf", nil, at, now)
	if next, ok := b.next(); !ok || !next.Equal(at) {
		t.Fatalf("next = %v %v, want the wake %v", next, ok, at)
	}
	b.put("w1", "golf", nil, at.Add(time.Hour), now) // replaced, not added
	if next, _ := b.next(); !next.Equal(at.Add(time.Hour)) {
		t.Fatalf("replace: next %v", next)
	}
	if got := b.due(at.Add(30 * time.Minute)); len(got) != 0 {
		t.Fatalf("due before its time: %v", got)
	}
	if got := b.due(at.Add(2 * time.Hour)); len(got) != 1 || got[0] != "w1" {
		t.Fatalf("due = %v, want w1", got)
	}
	if _, ok := b.next(); ok {
		t.Fatal("a wake done is still to come")
	}
	if got := b.due(at.Add(3 * time.Hour)); len(got) != 0 {
		t.Fatalf("done twice: %v", got)
	}
	b.put("w1", "golf", nil, time.Time{}, now) // the restart took it back
	if b.count() != 0 || len(b.wants) != 0 {
		t.Fatalf("nothing to come, yet booked: %v", b.wants)
	}
	b.put("w2", "old", nil, now.Add(-time.Hour), now) // long gone: not booked
	if _, ok := b.next(); ok {
		t.Fatal("an hour-old wake booked")
	}
}

func TestBookEveryGoesOnAndWakeBesideIt(t *testing.T) {
	b := newScheduleBook()
	now := time.Date(2026, 11, 21, 10, 0, 30, 0, time.Local)
	b.put("w1", "golf", []WhenSpec{{Every: "1h"}}, time.Time{}, now)
	every, _ := b.next()
	if !every.Equal(time.Date(2026, 11, 21, 11, 0, 0, 0, time.Local)) {
		t.Fatalf("every 1h: next %v", every)
	}
	wake := now.Add(10 * time.Minute)
	b.put("w1", "golf", []WhenSpec{{Every: "1h"}}, wake, now)
	if next, _ := b.next(); !next.Equal(wake) {
		t.Fatalf("next %v, want the wake %v", next, wake)
	}
	b.due(wake)
	if next, _ := b.next(); !next.Equal(every) {
		t.Fatalf("after the wake: next %v, want the hourly %v", next, every)
	}
	b.due(every)
	if next, _ := b.next(); !next.Equal(every.Add(time.Hour)) {
		t.Fatalf("after the hour: next %v, want %v", next, every.Add(time.Hour))
	}
	// The same schedule read again changes nothing.
	b.put("w1", "golf", []WhenSpec{{Every: "1h"}}, wake, every)
	if next, _ := b.next(); !next.Equal(every.Add(time.Hour)) {
		t.Fatalf("read again moved it: %v", next)
	}
}

func TestBookEarliestFirstAcrossWants(t *testing.T) {
	b := newScheduleBook()
	now := time.Now()
	b.put("a", "a", nil, now.Add(3*time.Minute), now)
	b.put("b", "b", nil, now.Add(1*time.Minute), now)
	b.put("c", "c", nil, now.Add(2*time.Minute), now)
	b.remove("b")
	if got := b.due(now.Add(150 * time.Second)); len(got) != 1 || got[0] != "c" {
		t.Fatalf("due = %v, want c only", got)
	}
	if next, _ := b.next(); !next.Equal(now.Add(3 * time.Minute)) {
		t.Fatalf("next %v, want a's", next)
	}
}

func TestScheduleChangedReachesTheScheduler(t *testing.T) {
	ear := make(chan string, 1)
	scheduleEarMu.Lock()
	scheduleEar = ear
	scheduleEarMu.Unlock()
	defer func() { scheduleEarMu.Lock(); scheduleEar = nil; scheduleEarMu.Unlock(); scheduleAll.Store(false) }()

	w := &Want{Metadata: Metadata{ID: "want-sched-ear"}}
	w.WakeAt(time.Now().Add(time.Hour))
	if got := <-ear; got != "want-sched-ear" {
		t.Fatalf("heard %q", got)
	}
	scheduleChanged("x")
	scheduleChanged("y") // the ear is full: every want is to be read again
	if !scheduleAll.Load() {
		t.Fatal("a full ear did not ask for all")
	}
}

func TestRestartTakesBackAPassedWake(t *testing.T) {
	w := &Want{Metadata: Metadata{ID: "want-wake-restart"}}
	w.State.Store(WakeAtState, time.Now().Add(-time.Minute).Format(time.RFC3339))
	w.prepareForRestart()
	if v, _ := w.State.Load(WakeAtState); v != "" {
		t.Fatalf("passed wake still %v after a restart", v)
	}
	future := time.Now().Add(time.Hour).Format(time.RFC3339)
	w.State.Store(WakeAtState, future)
	w.prepareForRestart()
	if v, _ := w.State.Load(WakeAtState); v != future {
		t.Fatalf("wake to come changed to %v by a restart", v)
	}
}
