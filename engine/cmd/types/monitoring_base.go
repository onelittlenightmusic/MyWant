package types

import (
	"context"
	"log"
	"time"

	. "mywant/engine/src"
)

// PollFunc defines the signature for the polling logic.
// It returns shouldStop=true if the monitoring should terminate.
type PollFunc func(ctx context.Context, want *Want) (shouldStop bool, err error)

// BaseMonitoringAgent provides common functionality for background polling agents.
// It handles lifecycle management, ticker synchronization, and Want cycle wrapping.
type BaseMonitoringAgent struct {
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

// NewBaseMonitoringAgent creates a new base monitoring agent.
func NewBaseMonitoringAgent(id string, interval time.Duration, name string, poll PollFunc) *BaseMonitoringAgent {
	return &BaseMonitoringAgent{
		id:       id,
		interval: interval,
		name:     name,
		poll:     poll,
	}
}

// ID returns the agent's unique identifier.
func (b *BaseMonitoringAgent) ID() string {
	return b.id
}

// Start begins the monitoring goroutine.
func (b *BaseMonitoringAgent) Start(ctx context.Context, w *Want) error {
	b.want = w
	b.ctx, b.cancel = context.WithCancel(ctx)
	b.ticker = time.NewTicker(b.interval)
	b.done = make(chan struct{})

	go func() {
		defer b.ticker.Stop()
		defer close(b.done)

		log.Printf("[%s] Starting continuous monitoring for want %s", b.name, w.Metadata.Name)

		for {
			select {
			case <-b.ctx.Done():
				log.Printf("[%s] Context cancelled, stopping monitoring for %s", b.name, w.Metadata.Name)
				return
			case <-b.ticker.C:
				b.BeginProgressCycle()
				shouldStop, err := b.poll(b.ctx, b.want)
				b.EndProgressCycle()

				if err != nil {
					log.Printf("[%s] Error during polling for %s: %v", b.name, w.Metadata.Name, err)
				}

				if shouldStop {
					log.Printf("[%s] Termination condition met for %s, stopping monitoring", b.name, w.Metadata.Name)
					return
				}
			}
		}
	}()

	return nil
}

// Stop gracefully stops the monitoring.
func (b *BaseMonitoringAgent) Stop() error {
	if b.cancel != nil {
		b.cancel()
	}
	if b.done != nil {
		select {
		case <-b.done:
			// Already done
		case <-time.After(1 * time.Second):
			// Timeout waiting for goroutine to stop
		}
	}
	return nil
}

// BeginProgressCycle wraps want execution for proper state management.
func (b *BaseMonitoringAgent) BeginProgressCycle() {
	if b.want != nil {
		b.want.BeginProgressCycle()
	}
}

// EndProgressCycle dumps agent state changes to the want.
func (b *BaseMonitoringAgent) EndProgressCycle() {
	if b.want != nil {
		b.want.DumpStateForAgent(b.name)
	}
}
