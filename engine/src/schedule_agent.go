package mywant

import (
	"context"
	"fmt"
	"time"
)

// SchedulerAgent manages scheduling for a single Want
// It implements the BackgroundAgent interface
type SchedulerAgent struct {
	id        string
	wantID    string           // ID of the Want to schedule
	wantName  string           // Name of the Want to schedule
	builder   *ChainBuilder    // Reference to ChainBuilder for triggering restarts
	ticker    *time.Ticker
	done      chan struct{}
	ctx       context.Context
	cancel    context.CancelFunc
	schedules []ParsedSchedule
	nextExec  time.Time
}

// NewSchedulerAgent creates a new scheduler agent for a target Want
func NewSchedulerAgent(whenSpecs []WhenSpec, wantID, wantName string, builder *ChainBuilder) (*SchedulerAgent, error) {
	if len(whenSpecs) == 0 {
		return nil, fmt.Errorf("no schedule specifications provided")
	}

	schedules := make([]ParsedSchedule, 0, len(whenSpecs))
	for _, spec := range whenSpecs {
		if err := ValidateWhenSpec(spec); err != nil {
			return nil, err
		}

		nextTime, err := CalculateNextExecution(spec, time.Now())
		if err != nil {
			return nil, err
		}

		interval, _ := ParseFrequencyExpression(spec.Every)
		schedules = append(schedules, ParsedSchedule{
			Time:          nextTime,
			Interval:      interval,
			IsAbsolute:    spec.At != "",
			OriginalAt:    spec.At,
			OriginalEvery: spec.Every,
		})
	}

	return &SchedulerAgent{
		id:        fmt.Sprintf("scheduler-%s", wantName),
		wantID:    wantID,
		wantName:  wantName,
		builder:   builder,
		schedules: schedules,
		nextExec:  findEarliestExecution(schedules),
	}, nil
}

// ID returns the unique identifier of this agent
func (s *SchedulerAgent) ID() string {
	return s.id
}

// Start begins the scheduler goroutine
func (s *SchedulerAgent) Start(ctx context.Context, w *Want) error {
	s.ctx, s.cancel = context.WithCancel(ctx)
	s.done = make(chan struct{})
	s.ticker = time.NewTicker(10 * time.Second)

	go s.run()

	InfoLog("[SCHEDULER] Started for Want '%s' (ID: %s) with %d schedule(s)\n",
		s.wantName, s.wantID, len(s.schedules))

	// Log initial schedule details
	for i, sched := range s.schedules {
		atInfo := "no fixed time (from 00:00:00)"
		if sched.IsAbsolute {
			atInfo = fmt.Sprintf("at %s", sched.OriginalAt)
		}
		InfoLog("[SCHEDULER] Schedule %d: %s, every %s, next execution: %s\n",
			i+1, atInfo, sched.OriginalEvery, s.nextExec.Format("2006-01-02 15:04:05"))
	}

	return nil
}

// Stop terminates the scheduler goroutine
func (s *SchedulerAgent) Stop() error {
	if s.cancel != nil {
		s.cancel()
	}
	if s.ticker != nil {
		s.ticker.Stop()
	}
	if s.done != nil {
		<-s.done
	}
	InfoLog("[SCHEDULER] Stopped for Want '%s'\n", s.wantName)
	return nil
}

// run is the main scheduler loop
func (s *SchedulerAgent) run() {
	defer close(s.done)

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-s.ticker.C:
			now := time.Now()
			if now.After(s.nextExec) || now.Equal(s.nextExec) {
				s.triggerRestart(now)
				s.updateNextExecution(now)
			}
		}
	}
}

// triggerRestart triggers Want restart via ChainBuilder
func (s *SchedulerAgent) triggerRestart(now time.Time) {
	InfoLog("[SCHEDULER] Triggering restart for Want '%s' at %s\n",
		s.wantName, now.Format("15:04:05"))

	// Call ChainBuilder to restart the Want
	if err := s.builder.RestartWant(s.wantID); err != nil {
		InfoLog("[SCHEDULER] Failed to restart Want '%s': %v\n", s.wantName, err)
		return
	}

	InfoLog("[SCHEDULER] Successfully restarted Want '%s'\n", s.wantName)
}

// updateNextExecution recalculates the next execution time after a trigger
func (s *SchedulerAgent) updateNextExecution(now time.Time) {
	for i := range s.schedules {
		s.schedules[i].Time = s.schedules[i].Time.Add(s.schedules[i].Interval)
	}
	s.nextExec = findEarliestExecution(s.schedules)

	InfoLog("[SCHEDULER] Next execution for '%s': %s\n",
		s.wantName, s.nextExec.Format("2006-01-02 15:04:05"))
}

// findEarliestExecution finds the earliest execution time among all schedules
func findEarliestExecution(schedules []ParsedSchedule) time.Time {
	if len(schedules) == 0 {
		return time.Time{}
	}
	earliest := schedules[0].Time
	for _, sched := range schedules[1:] {
		if sched.Time.Before(earliest) {
			earliest = sched.Time
		}
	}
	return earliest
}
