package mywant

import (
	"context"
	"time"
)

// ThinkingAgent implements BackgroundAgent and provides state-reactive thinking loop.
// It runs a ThinkFunc on a ticker interval without a stop condition.
// Structurally similar to PollingAgent but uses ThinkFunc instead of PollFunc.
type ThinkingAgent struct {
	id       string
	interval time.Duration
	think    ThinkFunc
	ticker   *time.Ticker
	done     chan struct{}
	ctx      context.Context
	cancel   context.CancelFunc
	want     *Want
	name     string // Name used for logging and state dumping
}

// NewThinkingAgent creates a new ThinkingAgent.
func NewThinkingAgent(id string, interval time.Duration, name string, think ThinkFunc) *ThinkingAgent {
	return &ThinkingAgent{
		id:       id,
		interval: interval,
		name:     name,
		think:    think,
	}
}

// ID returns the agent's unique identifier.
func (t *ThinkingAgent) ID() string {
	return t.id
}

// Start begins the thinking goroutine.
func (t *ThinkingAgent) Start(ctx context.Context, w *Want) error {
	InfoLog("[ThinkingAgent:%s] Starting for want %s\n", t.id, w.Metadata.Name)
	t.want = w
	t.ctx, t.cancel = context.WithCancel(ctx)
	t.ticker = time.NewTicker(t.interval)
	t.done = make(chan struct{})

	go func() {
		defer t.ticker.Stop()
		defer close(t.done)

		InfoLog("[ThinkingAgent:%s] Goroutine started for want %s\n", t.id, w.Metadata.Name)

		for {
			select {
			case <-t.ctx.Done():
				t.want.StoreLog("[%s] Context cancelled, stopping thinking for %s\n", t.name, w.Metadata.Name)
				return
			case <-t.ticker.C:
				t.BeginProgressCycle()
				err := t.think(t.ctx, t.want)
				t.EndProgressCycle()

				if err != nil {
					t.want.StoreLog("[%s] Error during thinking for %s: %v\n", t.name, w.Metadata.Name, err)
				}
			}
		}
	}()

	return nil
}

// Flush runs the think function once synchronously.
// Called before stopping to ensure any pending state changes are propagated.
func (t *ThinkingAgent) Flush(ctx context.Context) error {
	if t.want == nil {
		return nil
	}
	t.BeginProgressCycle()
	err := t.think(ctx, t.want)
	t.EndProgressCycle()
	return err
}

// Stop gracefully stops the thinking loop.
func (t *ThinkingAgent) Stop() error {
	if t.cancel != nil {
		t.cancel()
	}
	if t.done != nil {
		select {
		case <-t.done:
			// Already done
		case <-time.After(1 * time.Second):
			// Timeout waiting for goroutine to stop
		}
	}
	return nil
}

// BeginProgressCycle wraps want execution for proper state management.
func (t *ThinkingAgent) BeginProgressCycle() {
	if t.want != nil {
		t.want.BeginProgressCycle()
	}
}

// EndProgressCycle dumps agent state changes to the want.
func (t *ThinkingAgent) EndProgressCycle() {
	if t.want != nil {
		t.want.DumpStateForAgent(t.name)
	}
}
