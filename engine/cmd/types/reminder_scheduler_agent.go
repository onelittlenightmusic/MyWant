package types

import (
	"context"
	"fmt"
	. "mywant/engine/src"
	"time"
)

// ReminderSchedulerAgent monitors time-based transitions for reminders
// It implements the BackgroundAgent interface
type ReminderSchedulerAgent struct {
	id        string
	wantName  string
	reachingTime time.Time
	eventTime    time.Time
	ticker    *time.Ticker
	done      chan struct{}
	ctx       context.Context
	cancel    context.CancelFunc
}

// NewReminderSchedulerAgent creates a new reminder scheduler agent
func NewReminderSchedulerAgent(wantName string, reachingTime, eventTime time.Time) *ReminderSchedulerAgent {
	return &ReminderSchedulerAgent{
		id:           fmt.Sprintf("reminder-scheduler-%s", wantName),
		wantName:     wantName,
		reachingTime: reachingTime,
		eventTime:    eventTime,
	}
}

// ID returns the unique identifier of this agent
func (r *ReminderSchedulerAgent) ID() string {
	return r.id
}

// Start begins the scheduler goroutine
func (r *ReminderSchedulerAgent) Start(ctx context.Context, w *Want) error {
	r.ctx, r.cancel = context.WithCancel(ctx)
	r.done = make(chan struct{})
	r.ticker = time.NewTicker(10 * time.Second)

	go r.run(w)

	w.StoreLog("[REMINDER-SCHEDULER] Started for want '%s' (ID: %s)\n",
		r.wantName, r.id)
	w.StoreLog("[REMINDER-SCHEDULER] Reaching time: %s, Event time: %s\n",
		r.reachingTime.Format(time.RFC3339), r.eventTime.Format(time.RFC3339))

	return nil
}

// Stop terminates the scheduler goroutine
func (r *ReminderSchedulerAgent) Stop() error {
	if r.cancel != nil {
		r.cancel()
	}
	if r.ticker != nil {
		r.ticker.Stop()
	}
	if r.done != nil {
		<-r.done
	}
	return nil
}

// run is the main scheduler loop
func (r *ReminderSchedulerAgent) run(w *Want) {
	defer close(r.done)

	for {
		select {
		case <-r.ctx.Done():
			w.StoreLog("[REMINDER-SCHEDULER] Stopping scheduler for '%s'\n", r.wantName)
			return

		case <-r.ticker.C:
			r.checkAndTriggerTransitions(w)
		}
	}
}

// checkAndTriggerTransitions checks if time-based transitions should occur
func (r *ReminderSchedulerAgent) checkAndTriggerTransitions(w *Want) {
	now := time.Now()

	// Get current phase from state
	phase, _ := w.GetState("reminder_phase")
	phaseStr := fmt.Sprintf("%v", phase)

	// Check if we should transition to reaching phase
	if phaseStr == ReminderPhaseWaiting && now.After(r.reachingTime) {
		w.StoreLog("[REMINDER-SCHEDULER] Reaching time detected, transitioning to reaching phase\n")
		w.StoreState("reminder_phase", ReminderPhaseReaching)
	}

	// Check if event time has passed while in reaching phase
	if phaseStr == ReminderPhaseReaching && !r.eventTime.IsZero() && now.After(r.eventTime) {
		w.StoreLog("[REMINDER-SCHEDULER] Event time passed\n")
		// Don't auto-transition here; let Progress() handle it based on require_reaction
	}
}
