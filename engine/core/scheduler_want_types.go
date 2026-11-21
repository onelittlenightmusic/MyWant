package mywant

import (
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// SchedulerWant is a System Want that runs every scheduled Want when its time
// comes: spec.when, its type's default-every, its own wake (WakeAt).
//
// It keeps them in one book (scheduleBook), earliest first, and waits in its
// Progress for whichever comes first: the earliest time, or word that a
// want's schedule may have changed (scheduleChanged — a want added, deleted
// or its spec changed, a wake written). Nothing is looked at meanwhile; a
// pass with nothing to do waits up to schedulerMaxWait and returns, so the
// progression loop can still stop or suspend it.
type SchedulerWant struct {
	Want
	builder *ChainBuilder // Reference to ChainBuilder for coordinating restarts
	// Wall-clock second last written to the last_scan_time state field. The
	// scheduler passes many times a minute, and any real state change makes
	// the stats writer rebuild and rewrite the entire memory file, so it is
	// written once a minute.
	lastScanStored int64
	book           *scheduleBook
	changed        chan string // want IDs whose schedule may have changed
	nextSweep      time.Time   // when the targets are next looked after
}

// schedulerScanTimeResolution is how coarsely last_scan_time is recorded. It
// exists to show the scheduler is alive, which a minute answers as well as a
// second, and a minute costs 60× less persistence churn.
const schedulerScanTimeResolution = 60

// schedulerMaxWait is the longest a pass waits with nothing to do.
const schedulerMaxWait = time.Second

// targetSweepEvery is how often the targets are looked after (sweepTargets),
// besides when one is added.
const targetSweepEvery = 5 * time.Second

// The scheduler's ear: scheduleChanged sends a want's ID to it. Before the
// scheduler is up nothing is sent — it reads every want when it starts. A
// full ear asks for every want to be read again (scheduleAll).
var (
	scheduleEarMu sync.RWMutex
	scheduleEar   chan string
	scheduleAll   atomic.Bool
)

// scheduleChanged tells the scheduler a want's schedule may have changed: it
// was added or deleted, its spec changed, or it wrote its wake_at.
func scheduleChanged(id string) {
	scheduleEarMu.RLock()
	ear := scheduleEar
	scheduleEarMu.RUnlock()
	if ear == nil || id == "" {
		return
	}
	select {
	case ear <- id:
	default:
		scheduleAll.Store(true)
	}
}

// NewSchedulerWant creates a new Scheduler Want
func NewSchedulerWant() *SchedulerWant {
	return &SchedulerWant{}
}

// SetChainBuilder sets the ChainBuilder reference for this scheduler
func (s *SchedulerWant) SetChainBuilder(builder *ChainBuilder) {
	s.builder = builder
}

// Progress waits for the first schedule to come, or a change, and runs what
// has come.
func (s *SchedulerWant) Progress() {
	if s.builder == nil {
		return // ChainBuilder not yet set
	}
	if s.book == nil {
		s.start()
	}

	wait := schedulerMaxWait
	if at, ok := s.book.next(); ok {
		wait = min(wait, max(time.Until(at), 0))
	}
	wait = min(wait, max(time.Until(s.nextSweep), 0))
	timer := time.NewTimer(wait)
	select {
	case id := <-s.changed:
		s.refresh(id)
	case <-timer.C:
	}
	timer.Stop()
	for drained := false; !drained; {
		select {
		case id := <-s.changed:
			s.refresh(id)
		default:
			drained = true
		}
	}
	if scheduleAll.Swap(false) {
		s.refreshAll()
	}

	now := time.Now()
	for _, id := range s.book.due(now) {
		name := s.book.name(id)
		InfoLog("[SCHEDULER] Triggering restart for Want '%s' at %s\n", name, now.Format("15:04:05"))
		if err := s.builder.RestartWant(id); err != nil {
			InfoLog("[SCHEDULER] Failed to restart Want '%s': %v\n", name, err)
		}
	}
	if !now.Before(s.nextSweep) {
		s.sweepTargets()
		s.nextSweep = now.Add(targetSweepEvery)
	}

	// Update scheduler statistics in state. The count is idempotent — StoreState
	// drops a write that changes nothing — but the scan time is not, so it is
	// only refreshed once every schedulerScanTimeResolution seconds.
	s.storeState("total_scheduled_wants", s.book.count())
	if unix := now.Unix(); unix-s.lastScanStored >= schedulerScanTimeResolution {
		s.lastScanStored = unix
		s.storeState("last_scan_time", unix)
	}
}

// start opens the ear, then reads every want — in that order, so a change in
// between is not missed (reading a want twice changes nothing).
func (s *SchedulerWant) start() {
	s.book = newScheduleBook()
	s.changed = make(chan string, 1024)
	scheduleEarMu.Lock()
	scheduleEar = s.changed
	scheduleEarMu.Unlock()
	s.refreshAll()
	s.retryPassingFailures()
}

// retryPassingFailures runs again, once, the wants found failed for a
// passing reason (want_retry.go) with no wake of their own to come — those
// that failed before the server stopped, or before failures were run again
// at all. Failing again, each is run again on its own from there.
func (s *SchedulerWant) retryPassingFailures() {
	now := time.Now()
	for _, w := range s.builder.GetWants() {
		if w.GetStatus() != WantStatusFailed {
			continue
		}
		errText, _ := w.GetStateString(StateFieldError, "")
		if !IsTransientFailure(errText) {
			continue
		}
		if at, ok := wakeOf(w); ok && at.After(now) {
			continue // its own wake will run it
		}
		InfoLog("[RETRY] Running '%s' again: it failed for a passing reason (%s)\n", w.Metadata.Name, firstLineOf(errText))
		if err := s.builder.RestartWant(w.Metadata.ID); err != nil {
			InfoLog("[RETRY] Could not run '%s' again: %v\n", w.Metadata.Name, err)
		}
	}
}

// refreshAll reads every want's schedule.
func (s *SchedulerWant) refreshAll() {
	s.builder.wantsMu.RLock()
	ids := make([]string, 0, len(s.builder.wants))
	for id := range s.builder.wants {
		ids = append(ids, id)
	}
	s.builder.wantsMu.RUnlock()
	known := map[string]bool{}
	for _, id := range ids {
		known[id] = true
		s.refresh(id)
	}
	for id := range s.book.wants {
		if !known[id] {
			s.book.remove(id)
		}
	}
}

// refresh reads one want's schedule into the book; a want gone, or the
// scheduler itself, is taken out. A target added is looked after at once.
func (s *SchedulerWant) refresh(id string) {
	s.builder.wantsMu.RLock()
	rw, ok := s.builder.wants[id]
	s.builder.wantsMu.RUnlock()
	if !ok || rw.want == nil || rw.want.Metadata.Type == "scheduler" {
		s.book.remove(id)
		return
	}
	if isTargetWant(rw.want) {
		s.lookAfterTarget(id, rw)
		return
	}
	wake, _ := wakeOf(rw.want)
	s.book.put(id, rw.want.Metadata.Name, scheduleOf(rw.want), wake, time.Now())
}

func isTargetWant(w *Want) bool {
	return w.Metadata.Type == "target" || w.Metadata.Type == "custom_target"
}

// sweepTargets looks after every target: a target always needs a progression
// loop, schedule or not.
func (s *SchedulerWant) sweepTargets() {
	s.builder.wantsMu.RLock()
	targets := map[string]*runtimeWant{}
	for id, rw := range s.builder.wants {
		if rw.want != nil && isTargetWant(rw.want) {
			targets[id] = rw
		}
	}
	s.builder.wantsMu.RUnlock()
	for id, rw := range targets {
		s.lookAfterTarget(id, rw)
	}
}

// lookAfterTarget starts a target's loop. After a server restart, targets
// restored in "reaching" state have no active goroutine: reset to idle so
// startWant can restart the loop.
func (s *SchedulerWant) lookAfterTarget(id string, rw *runtimeWant) {
	if IsReachingStatus(rw.want.GetStatus()) && !rw.want.goroutineActive.Load() {
		rw.want.RestartWant()
	}
	s.builder.startWant(id, rw)
}

// Initialize resets state before execution begins
func (s *SchedulerWant) Initialize() {
	// No state reset needed for scheduler wants
}

// IsAchieved always returns false since the scheduler runs continuously
func (s *SchedulerWant) IsAchieved() bool {
	return false
}

// RegisterSchedulerWantTypes registers the scheduler want type with the ChainBuilder
func RegisterSchedulerWantTypes(builder *ChainBuilder) {
	builder.RegisterWantType("scheduler", func(m Metadata, spec WantSpec) Progressable {
		schedulerWant := NewSchedulerWant()
		schedulerWant.Want = Want{Metadata: m, Spec: spec}
		schedulerWant.SetChainBuilder(builder)
		return schedulerWant
	})
}

// defaultEveryLabel is a want type's label for how often its wants run again
// when their own spec says nothing (spec.when) — "1h", "30m", "1d". A want that
// is achieved on its first answer (smartgolf's reservation check) stops its
// monitor there; the schedule restarts it, so it answers again an hour later.
const defaultEveryLabel = "default-every"

// scheduleOf is a want's schedule: its own spec.when, else its type's
// default-every.
func scheduleOf(want *Want) []WhenSpec {
	if len(want.Spec.When) > 0 {
		return want.Spec.When
	}
	if def := want.WantTypeDefinition; def != nil {
		if every := strings.TrimSpace(def.Metadata.Labels[defaultEveryLabel]); every != "" {
			return []WhenSpec{{Every: every}}
		}
	}
	return nil
}
