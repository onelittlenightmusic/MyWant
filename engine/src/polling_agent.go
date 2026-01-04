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
	id       string
	interval time.Duration
	poll     PollFunc
	ticker   *time.Ticker
	done     chan struct{}
	ctx      context.Context
	cancel   context.CancelFunc
	want     *Want
	name     string // Name used for logging and state dumping
}

// NewPollingAgent creates a new polling agent.
func NewPollingAgent(id string, interval time.Duration, name string, poll PollFunc) *PollingAgent {
	return &PollingAgent{
		id:       id,
		interval: interval,
		name:     name,
		poll:     poll,
	}
}

// ID returns the agent's unique identifier.
func (p *PollingAgent) ID() string {
	return p.id
}

// Start begins the monitoring goroutine.
func (p *PollingAgent) Start(ctx context.Context, w *Want) error {
	p.want = w
	p.ctx, p.cancel = context.WithCancel(ctx)
	p.ticker = time.NewTicker(p.interval)
	p.done = make(chan struct{})

	go func() {
		defer p.ticker.Stop()
		defer close(p.done)

		p.want.StoreLog("[%s] Starting continuous monitoring for want %s\n", p.name, w.Metadata.Name)

		for {
			select {
			case <-p.ctx.Done():
				p.want.StoreLog("[%s] Context cancelled, stopping monitoring for %s\n", p.name, w.Metadata.Name)
				return
			case <-p.ticker.C:
				p.BeginProgressCycle()
				shouldStop, err := p.poll(p.ctx, p.want)
				p.EndProgressCycle()

				if err != nil {
					p.want.StoreLog("[%s] Error during polling for %s: %v\n", p.name, w.Metadata.Name, err)
				}

				if shouldStop {
					p.want.StoreLog("[%s] Termination condition met for %s, stopping monitoring\n", p.name, w.Metadata.Name)
					return
				}
			}
		}
	}()

	return nil
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
