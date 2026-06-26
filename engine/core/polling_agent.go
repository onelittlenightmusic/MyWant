package mywant

import (
	"context"
	"time"
)

// PollFunc defines the signature for the polling logic.
// It returns shouldStop=true if the monitoring should terminate.
type PollFunc func(ctx context.Context, want *Want) (shouldStop bool, err error)

// PollingAgent implements BackgroundAgent and provides common functionality for background polling.
// It handles lifecycle management, ticker synchronization, and Want cycle wrapping.
type PollingAgent struct {
	id        string
	interval  time.Duration
	poll      PollFunc
	ticker    *time.Ticker
	done      chan struct{}
	trigger   chan struct{} // buffered channel for immediate wakeup
	ctx       context.Context
	cancel    context.CancelFunc
	want      *Want
	name      string // Name used for logging and state dumping
	agentType string // Agent type string recorded in AgentHistoryRing
	execID    string // Execution ID of the "running" entry recorded at start
}

// NewPollingAgent creates a new polling agent.
func NewPollingAgent(id string, interval time.Duration, name string, agentType string, poll PollFunc) *PollingAgent {
	return &PollingAgent{
		id:        id,
		interval:  interval,
		name:      name,
		agentType: agentType,
		poll:      poll,
	}
}

// ID returns the agent's unique identifier.
func (p *PollingAgent) ID() string {
	return p.id
}

// Start begins the monitoring goroutine.
func (p *PollingAgent) Start(ctx context.Context, w *Want) error {
	InfoLog("[PollingAgent:%s] Starting for want %s\n", p.id, w.Metadata.Name)
	p.want = w
	p.ctx, p.cancel = context.WithCancel(ctx)
	p.ticker = time.NewTicker(p.interval)
	p.trigger = make(chan struct{}, 1)
	p.done = make(chan struct{})

	go func() {
		defer p.ticker.Stop()
		defer close(p.done)

		InfoLog("[PollingAgent:%s] Goroutine started for want %s\n", p.id, w.Metadata.Name)
		p.want.StoreLog("[%s] Starting continuous monitoring for want %s\n", p.name, w.Metadata.Name)

		runPoll := func() {
			if !p.want.TryStartAgentRun(p.name) {
				return
			}
			p.BeginProgressCycle()
			shouldStop, err := p.poll(p.ctx, p.want)
			p.EndProgressCycle()
			p.want.FinishAgentRun(p.name, false)
			if err != nil {
				p.want.StoreLog("[%s] Error during polling for %s: %v\n", p.name, w.Metadata.Name, err)
				p.want.RecordAgentResult(p.execID, p.name, p.agentType, "error", err.Error())
			}
			if shouldStop {
				p.want.StoreLog("[%s] Termination condition met for %s, stopping monitoring\n", p.name, w.Metadata.Name)
				w.backgroundMutex.Lock()
				delete(w.backgroundAgents, p.id)
				w.backgroundMutex.Unlock()
				p.cancel()
			}
		}

		for {
			select {
			case <-p.ctx.Done():
				p.want.StoreLog("[%s] Context cancelled, stopping monitoring for %s\n", p.name, w.Metadata.Name)
				return
			case <-p.trigger:
				runPoll()
			case <-p.ticker.C:
				runPoll()
			}
		}
	}()

	return nil
}

// Trigger signals the agent to run its poll function immediately, without
// waiting for the next ticker tick. Non-blocking: skipped if already pending.
func (p *PollingAgent) Trigger() {
	select {
	case p.trigger <- struct{}{}:
	default:
	}
}

// Stop gracefully stops the monitoring.
func (p *PollingAgent) Stop() error {
	if p.cancel != nil {
		p.cancel()
	}
	if p.done != nil {
		select {
		case <-p.done:
			// Already done
		case <-time.After(1 * time.Second):
			// Timeout waiting for goroutine to stop
		}
	}
	return nil
}

// BeginProgressCycle wraps want execution for proper state management.
func (p *PollingAgent) BeginProgressCycle() {
	if p.want != nil {
		p.want.BeginProgressCycle()
	}
}

// EndProgressCycle dumps agent state changes to the want.
func (p *PollingAgent) EndProgressCycle() {
	if p.want != nil {
		// Use the agent name or a generic "MonitorAgent" for history tracking
		p.want.DumpStateForAgent(p.name)
	}
}
