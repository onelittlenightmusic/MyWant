package main

import (
	"fmt"
	. "mywant/src"
)

// FibonacciSeed represents a fibonacci sequence value with its position
type FibonacciSeed struct {
	Value    int
	Position int
	IsEnd    bool
}

// SeedNumbers provides initial fibonacci seeds (0, 1)
type SeedNumbers struct {
	Want
	MaxCount int
	paths    Paths
}

// NewSeedNumbers creates a new seed numbers want
func NewSeedNumbers(metadata Metadata, params map[string]interface{}) *SeedNumbers {
	gen := &SeedNumbers{
		Want: Want{
			Metadata: metadata,
			Spec:     WantSpec{Params: params},
			// Stats field removed - using State instead
			Status: WantStatusIdle,
			State:  make(map[string]interface{}),
		},
		MaxCount: 15,
	}

	if c, ok := params["max_count"]; ok {
		if ci, ok := c.(int); ok {
			gen.MaxCount = ci
		} else if cf, ok := c.(float64); ok {
			gen.MaxCount = int(cf)
		}
	}

	return gen
}

// InitializePaths initializes the paths for this seed numbers generator
func (g *SeedNumbers) InitializePaths(inCount, outCount int) {
	g.paths.In = make([]PathInfo, inCount)
	g.paths.Out = make([]PathInfo, outCount)
}

// GetConnectivityMetadata returns connectivity requirements for seed numbers generator
func (g *SeedNumbers) GetConnectivityMetadata() ConnectivityMetadata {
	return ConnectivityMetadata{
		RequiredInputs:  0,
		RequiredOutputs: 1,
		MaxInputs:       0,
		MaxOutputs:      -1,
		WantType:        "seed_numbers",
		Description:     "Fibonacci seed generator",
	}
}

// GetStats returns the stats for this seed numbers generator
func (g *SeedNumbers) GetStats() map[string]interface{} {
	return g.State
}

// Process processes using enhanced paths
func (g *SeedNumbers) Process(paths Paths) bool {
	g.paths = paths
	return false
}

// GetType returns the want type
func (g *SeedNumbers) GetType() string {
	return "seed_numbers"
}

// GetWant returns the embedded Want
func (g *SeedNumbers) GetWant() *Want {
	return &g.Want
}

// Exec returns the generalized chain function for the seed numbers generator
func (g *SeedNumbers) Exec(using []Chan, outputs []Chan) bool {
	// Read parameters fresh each cycle - enables dynamic changes!
	maxCount := 15
	if c, ok := g.Spec.Params["max_count"]; ok {
		if ci, ok := c.(int); ok {
			maxCount = ci
		} else if cf, ok := c.(float64); ok {
			maxCount = int(cf)
		}
	}

	// Check if already completed using persistent state
	completed, _ := g.State["completed"].(bool)

	if len(outputs) == 0 {
		return true
	}
	out := outputs[0]

	if completed {
		return true
	}

	// Mark as completed in persistent state
	g.State["completed"] = true

	// Send initial seeds: 0 and 1
	out <- FibonacciSeed{Value: 0, Position: 0, IsEnd: false}
	out <- FibonacciSeed{Value: 1, Position: 1, IsEnd: false}

	// Send end marker with max count info
	out <- FibonacciSeed{Value: maxCount, Position: -1, IsEnd: true}

	if g.State == nil {
		g.State = make(map[string]interface{})
	}
	g.State["total_processed"] = 2
	fmt.Printf("[SEED] Generated initial seeds: 0, 1 (max_count: %d)\n", maxCount)
	return true
}

// FibonacciComputer computes the next fibonacci number from two using
type FibonacciComputer struct {
	Want
	paths Paths
}

// NewFibonacciComputer creates a new fibonacci computer want
func NewFibonacciComputer(metadata Metadata, params map[string]interface{}) *FibonacciComputer {
	return &FibonacciComputer{
		Want: Want{
			Metadata: metadata,
			Spec:     WantSpec{Params: params},
			// Stats field removed - using State instead
			Status: WantStatusIdle,
			State:  make(map[string]interface{}),
		},
	}
}

// InitializePaths initializes the paths for this computer
func (c *FibonacciComputer) InitializePaths(inCount, outCount int) {
	c.paths.In = make([]PathInfo, inCount)
	c.paths.Out = make([]PathInfo, outCount)
}

// GetConnectivityMetadata returns connectivity requirements for fibonacci computer
func (c *FibonacciComputer) GetConnectivityMetadata() ConnectivityMetadata {
	return ConnectivityMetadata{
		RequiredInputs:  1,
		RequiredOutputs: 1,
		MaxInputs:       1,
		MaxOutputs:      -1,
		WantType:        "fibonacci_computer",
		Description:     "Fibonacci number computer",
	}
}

// GetStats returns the stats for this computer
func (c *FibonacciComputer) GetStats() map[string]interface{} {
	return c.State
}

// Process processes using enhanced paths
func (c *FibonacciComputer) Process(paths Paths) bool {
	c.paths = paths
	return false
}

// GetType returns the want type
func (c *FibonacciComputer) GetType() string {
	return "fibonacci_computer"
}

// GetWant returns the embedded Want
func (c *FibonacciComputer) GetWant() *Want {
	return &c.Want
}

// Exec returns the generalized chain function for the fibonacci computer
func (c *FibonacciComputer) Exec(using []Chan, outputs []Chan) bool {
	if len(using) == 0 || len(outputs) == 0 {
		return true
	}
	in := using[0]
	out := outputs[0]

	// Initialize persistent state variables
	prev, _ := c.State["prev"].(int)
	current, _ := c.State["current"].(int)
	if current == 0 {
		current = 1
	}
	position, _ := c.State["position"].(int)
	if position == 0 {
		position = 2
	}
	maxCount, _ := c.State["maxCount"].(int)
	if maxCount == 0 {
		maxCount = 15
	}
	processed, _ := c.State["processed"].(int)
	initialized, _ := c.State["initialized"].(bool)

	seed := (<-in).(FibonacciSeed)

	if seed.IsEnd {
		maxCount = seed.Value
		c.State["maxCount"] = maxCount
		fmt.Printf("[COMPUTER] Received max count: %d\n", maxCount)

		// After getting max count, start computing all remaining fibonacci numbers
		for position < maxCount {
			next := prev + current
			out <- FibonacciSeed{Value: next, Position: position, IsEnd: false}
			prev = current
			current = next
			position++
			processed++
		}

		// Send end signal
		out <- FibonacciSeed{Value: 0, Position: -1, IsEnd: true}

		// Update persistent state
		c.State["prev"] = prev
		c.State["current"] = current
		c.State["position"] = position
		c.State["processed"] = processed
		c.State["total_processed"] = processed

		fmt.Printf("[COMPUTER] Computed %d fibonacci numbers\n", processed)
		return true
	}

	// Initialize with first two seeds
	if !initialized {
		if seed.Position == 0 {
			prev = seed.Value
			c.State["prev"] = prev
			return false
		} else if seed.Position == 1 {
			current = seed.Value
			initialized = true
			c.State["current"] = current
			c.State["initialized"] = initialized
			return false
		}
	}

	return false
}

// FibonacciMerger merges seed values with computed values
type FibonacciMerger struct {
	Want
	paths Paths
}

// NewFibonacciMerger creates a new fibonacci merger want
func NewFibonacciMerger(metadata Metadata, params map[string]interface{}) *FibonacciMerger {
	return &FibonacciMerger{
		Want: Want{
			Metadata: metadata,
			Spec:     WantSpec{Params: params},
			// Stats field removed - using State instead
			Status: WantStatusIdle,
			State:  make(map[string]interface{}),
		},
	}
}

// InitializePaths initializes the paths for this merger
func (m *FibonacciMerger) InitializePaths(inCount, outCount int) {
	m.paths.In = make([]PathInfo, inCount)
	m.paths.Out = make([]PathInfo, outCount)
}

// GetConnectivityMetadata returns connectivity requirements for fibonacci merger
func (m *FibonacciMerger) GetConnectivityMetadata() ConnectivityMetadata {
	return ConnectivityMetadata{
		RequiredInputs:  2,
		RequiredOutputs: 1, // Only to computer
		MaxInputs:       2,
		MaxOutputs:      -1,
		WantType:        "fibonacci_merger",
		Description:     "Fibonacci sequence merger",
	}
}

// GetStats returns the stats for this merger
func (m *FibonacciMerger) GetStats() map[string]interface{} {
	return m.State
}

// Process processes using enhanced paths
func (m *FibonacciMerger) Process(paths Paths) bool {
	m.paths = paths
	return false
}

// GetType returns the want type
func (m *FibonacciMerger) GetType() string {
	return "fibonacci_merger"
}

// GetWant returns the embedded Want
func (m *FibonacciMerger) GetWant() *Want {
	return &m.Want
}

// Exec returns the generalized chain function for the fibonacci merger
func (m *FibonacciMerger) Exec(using []Chan, outputs []Chan) bool {
	if len(using) < 2 || len(outputs) < 1 {
		return true
	}

	// Use persistent state for closure variables
	seedUsingClosed, _ := m.State["seedUsingClosed"].(bool)
	computedUsingClosed, _ := m.State["computedUsingClosed"].(bool)
	processed, _ := m.State["processed"].(int)
	maxCountReceived, _ := m.State["maxCountReceived"].(bool)

	seedIn := using[0]        // From seed generator
	computedIn := using[1]    // From fibonacci computer (feedback loop)
	computerOut := outputs[0] // To fibonacci computer

	// Handle both using with blocking select
	select {
	case seed := <-seedIn:
		fibSeed := seed.(FibonacciSeed)
		if fibSeed.IsEnd {
			seedUsingClosed = true
			m.State["seedUsingClosed"] = seedUsingClosed
			if !maxCountReceived {
				// Forward end signal to computer to set max count
				computerOut <- fibSeed
				maxCountReceived = true
				m.State["maxCountReceived"] = maxCountReceived
			}
		} else {
			computerOut <- fibSeed                                      // Send to computer for processing
			fmt.Printf("F(%d) = %d\n", fibSeed.Position, fibSeed.Value) // Display directly
			processed++
			m.State["processed"] = processed
		}

	case computed := <-computedIn:
		fibSeed := computed.(FibonacciSeed)
		if fibSeed.IsEnd {
			computedUsingClosed = true
			m.State["computedUsingClosed"] = computedUsingClosed
		} else {
			// Display computed values directly
			fmt.Printf("F(%d) = %d\n", fibSeed.Position, fibSeed.Value)
			processed++
			m.State["processed"] = processed
		}
	}

	// End when both using are closed
	if seedUsingClosed && computedUsingClosed {
		if m.State == nil {
			m.State = make(map[string]interface{})
		}
		m.State["total_processed"] = processed
		fmt.Printf("[MERGER] Merged %d fibonacci values\n", processed)
		return true
	}

	return false
}

// RegisterFibonacciLoopWantTypes registers the fibonacci loop want types with a ChainBuilder
func RegisterFibonacciLoopWantTypes(builder *ChainBuilder) {
	// Register seed numbers type
	builder.RegisterWantType("seed_numbers", func(metadata Metadata, spec WantSpec) interface{} {
		return NewSeedNumbers(metadata, spec.Params)
	})

	// Register fibonacci computer type
	builder.RegisterWantType("fibonacci_computer", func(metadata Metadata, spec WantSpec) interface{} {
		return NewFibonacciComputer(metadata, spec.Params)
	})

	// Register fibonacci merger type
	builder.RegisterWantType("fibonacci_merger", func(metadata Metadata, spec WantSpec) interface{} {
		return NewFibonacciMerger(metadata, spec.Params)
	})
}
