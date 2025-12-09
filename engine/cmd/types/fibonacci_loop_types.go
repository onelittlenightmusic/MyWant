package types

import (
	"fmt"
	. "mywant/engine/src"
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
func NewSeedNumbers(metadata Metadata, spec WantSpec) interface{} {
	gen := &SeedNumbers{
		Want:     Want{},
		MaxCount: 15,
	}
	gen.Init(metadata, spec)

	// Extract max_count parameter with automatic type conversion
	gen.MaxCount = gen.GetIntParam("max_count", 15)
	gen.WantType = "seed numbers"
	gen.ConnectivityMetadata = ConnectivityMetadata{
		RequiredInputs:  0,
		RequiredOutputs: 1,
		MaxInputs:       0,
		MaxOutputs:      -1,
		WantType:        "seed numbers",
		Description:     "Fibonacci seed generator",
	}

	return gen
}

// Exec returns the generalized chain function for the seed numbers generator
func (g *SeedNumbers) Exec() bool {
	maxCount := g.GetIntParam("max_count", 15)
	completed, _ := g.GetStateBool("completed", false)

	if completed {
		return true
	}
	g.StoreState("completed", true)
	g.SendPacketMulti(FibonacciSeed{Value: 0, Position: 0, IsEnd: false})
	g.SendPacketMulti(FibonacciSeed{Value: 1, Position: 1, IsEnd: false})
	g.SendPacketMulti(FibonacciSeed{Value: maxCount, Position: -1, IsEnd: true})
	g.StoreState("total_processed", 2)
	g.StoreLog(fmt.Sprintf("Generated initial seeds: 0, 1 (max_count: %d)", maxCount))
	return true
}

// FibonacciComputer computes the next fibonacci number from two using
type FibonacciComputer struct {
	Want
	paths Paths
}

// NewFibonacciComputer creates a new fibonacci computer want
func NewFibonacciComputer(metadata Metadata, spec WantSpec) interface{} {
	computer := &FibonacciComputer{
		Want: Want{},
	}
	computer.Init(metadata, spec)
	computer.WantType = "fibonacci computer"
	computer.ConnectivityMetadata = ConnectivityMetadata{
		RequiredInputs:  1,
		RequiredOutputs: 1,
		MaxInputs:       1,
		MaxOutputs:      -1,
		WantType:        "fibonacci computer",
		Description:     "Fibonacci number computer",
	}

	return computer
}

// Exec returns the generalized chain function for the fibonacci computer
func (c *FibonacciComputer) Exec() bool {
	in, inputUnavailable := c.GetInputChannel(0)
	if inputUnavailable {
		return true
	}
	prev, _ := c.GetStateInt("prev", 0)
	current, _ := c.GetStateInt("current", 0)
	if current == 0 {
		current = 1
	}
	position, _ := c.GetStateInt("position", 0)
	if position == 0 {
		position = 2
	}
	maxCount, _ := c.GetStateInt("maxCount", 0)
	if maxCount == 0 {
		maxCount = 15
	}
	processed, _ := c.GetStateInt("processed", 0)
	initialized, _ := c.GetStateBool("initialized", false)

	seed := (<-in).(FibonacciSeed)

	if seed.IsEnd {
		maxCount = seed.Value
		c.StoreState("maxCount", maxCount)
		c.StoreLog(fmt.Sprintf("Received max count: %d", maxCount))

		// After getting max count, start computing all remaining fibonacci numbers
		for position < maxCount {
			next := prev + current
			c.SendPacketMulti(FibonacciSeed{Value: next, Position: position, IsEnd: false})
			prev = current
			current = next
			position++
			processed++
		}
		c.SendPacketMulti(FibonacciSeed{Value: 0, Position: -1, IsEnd: true})
		c.StoreStateMulti(map[string]interface{}{
			"prev": prev,
			"current": current,
			"position": position,
			"processed": processed,
			"total_processed": processed,
		})

		c.StoreLog(fmt.Sprintf("Computed %d fibonacci numbers", processed))
		return true
	}
	if !initialized {
		if seed.Position == 0 {
			prev = seed.Value
			c.StoreState("prev", prev)
			return false
		} else if seed.Position == 1 {
			current = seed.Value
			initialized = true
			c.StoreStateMulti(map[string]interface{}{
				"current": current,
				"initialized": initialized,
			})
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
func NewFibonacciMerger(metadata Metadata, spec WantSpec) interface{} {
	merger := &FibonacciMerger{
		Want: Want{},
	}
	merger.Init(metadata, spec)
	merger.WantType = "fibonacci merger"
	merger.ConnectivityMetadata = ConnectivityMetadata{
		RequiredInputs:  2,
		RequiredOutputs: 1,
		MaxInputs:       2,
		MaxOutputs:      1,
		WantType:        "fibonacci merger",
		Description:     "Fibonacci merger",
	}

	return merger
}

// Exec returns the generalized chain function for the fibonacci merger
func (m *FibonacciMerger) Exec() bool {
	if m.paths.GetInCount() < 2 || m.paths.GetOutCount() < 1 {
		return true
	}

	// Use persistent state for closure variables
	seedUsingClosed, _ := m.GetStateBool("seedUsingClosed", false)
	computedUsingClosed, _ := m.GetStateBool("computedUsingClosed", false)
	processed, _ := m.GetStateInt("processed", 0)
	maxCountReceived, _ := m.GetStateBool("maxCountReceived", false)

	seedIn, _ := m.GetInputChannel(0)     // From seed generator
	computedIn, _ := m.GetInputChannel(1) // From fibonacci computer (feedback loop)
	select {
	case seed := <-seedIn:
		fibSeed := seed.(FibonacciSeed)
		if fibSeed.IsEnd {
			seedUsingClosed = true
			m.StoreState("seedUsingClosed", seedUsingClosed)
			if !maxCountReceived {
				// Forward end signal to computer to set max count
				m.SendPacketMulti(fibSeed)
				maxCountReceived = true
				m.StoreState("maxCountReceived", maxCountReceived)
			}
		} else {
			m.SendPacketMulti(fibSeed) // Send to computer for processing
			m.StoreLog(fmt.Sprintf("F(%d) = %d", fibSeed.Position, fibSeed.Value)) // Display directly
			processed++
			m.StoreState("processed", processed)
		}

	case computed := <-computedIn:
		fibSeed := computed.(FibonacciSeed)
		if fibSeed.IsEnd {
			computedUsingClosed = true
			m.StoreState("computedUsingClosed", computedUsingClosed)
		} else {
			// Display computed values directly
			m.StoreLog(fmt.Sprintf("F(%d) = %d", fibSeed.Position, fibSeed.Value))
			processed++
			m.StoreState("processed", processed)
		}
	}

	// End when both using are closed
	if seedUsingClosed && computedUsingClosed {
		m.StoreState("total_processed", processed)
		m.StoreLog(fmt.Sprintf("Merged %d fibonacci values", processed))
		return true
	}

	return false
}

// RegisterFibonacciLoopWantTypes registers the fibonacci loop want types with a ChainBuilder
func RegisterFibonacciLoopWantTypes(builder *ChainBuilder) {
	builder.RegisterWantType("seed numbers", NewSeedNumbers)
	builder.RegisterWantType("fibonacci computer", NewFibonacciComputer)
	builder.RegisterWantType("fibonacci merger", NewFibonacciMerger)
}
