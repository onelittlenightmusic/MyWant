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

func TestWakeIsOneEntryInTheSchedule(t *testing.T) {
	a, err := NewSchedulerAgent(nil, "w1", "golf", nil)
	if err != nil {
		t.Fatalf("an agent for a wake alone: %v", err)
	}
	now := time.Now()
	at := now.Add(time.Minute).Truncate(time.Second)
	a.setWake(at)
	if !a.nextExec.Equal(at) || len(a.schedules) != 1 {
		t.Fatalf("wake not scheduled: next %v, %d entries", a.nextExec, len(a.schedules))
	}
	a.setWake(at.Add(time.Hour)) // replaced, not added
	if len(a.schedules) != 1 || !a.nextExec.Equal(at.Add(time.Hour)) {
		t.Fatalf("replace: next %v, %d entries", a.nextExec, len(a.schedules))
	}
	a.updateNextExecution(at.Add(2 * time.Hour)) // came: done once, dropped
	if len(a.schedules) != 0 {
		t.Fatalf("wake still scheduled after it came: %v", a.schedules)
	}
	a.setWake(time.Time{})
	a.setWake(now.Add(-time.Hour)) // long gone: not put in
	if len(a.schedules) != 0 {
		t.Fatalf("an hour-old wake scheduled: %v", a.schedules)
	}
}

func TestWakeBesideAnEverySchedule(t *testing.T) {
	a, err := NewSchedulerAgent([]WhenSpec{{Every: "1h"}}, "w1", "golf", nil)
	if err != nil {
		t.Fatal(err)
	}
	every := a.nextExec
	wake := time.Now().Add(time.Second).Truncate(time.Second)
	if !wake.Before(every) {
		t.Skip("the hour turns within a second")
	}
	a.setWake(wake)
	if !a.nextExec.Equal(wake) {
		t.Fatalf("next %v, want the wake %v", a.nextExec, wake)
	}
	a.updateNextExecution(wake)
	if len(a.schedules) != 1 || !a.nextExec.Equal(every) {
		t.Fatalf("after the wake: next %v (%d entries), want the hourly %v", a.nextExec, len(a.schedules), every)
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
