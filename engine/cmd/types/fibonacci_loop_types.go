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

// SeedNumbersLocals holds type-specific local state for SeedNumbers want
type SeedNumbersLocals struct {
	MaxCount int
}

// SeedNumbers provides initial fibonacci seeds (0, 1)
type SeedNumbers struct {
	Want
}

// NewSeedNumbers creates a new seed numbers want
func NewSeedNumbers(metadata Metadata, spec WantSpec) Progressable {
	return &SeedNumbers{*NewWantWithLocals(
		metadata,
		spec,
		&SeedNumbersLocals{},
		"seed numbers",
	)}
}

// IsAchieved checks if seeds have been generated
func (g *SeedNumbers) IsAchieved() bool {
	completed, _ := g.GetStateBool("completed", false)
	return completed
}

// Progress returns the generalized chain function for the seed numbers generator
func (g *SeedNumbers) Progress() {
	maxCount := g.GetIntParam("max_count", 15)
	completed, _ := g.GetStateBool("completed", false)

	if completed {
		return
	}
	g.StoreState("completed", true)
	g.Provide(FibonacciSeed{Value: 0, Position: 0, IsEnd: false})
	g.Provide(FibonacciSeed{Value: 1, Position: 1, IsEnd: false})
	g.Provide(FibonacciSeed{Value: maxCount, Position: -1, IsEnd: true})
	g.StoreState("total_processed", 2)
	g.StoreLog(fmt.Sprintf("Generated initial seeds: 0, 1 (max_count: %d)", maxCount))
}

// FibonacciComputerLocals holds type-specific local state for FibonacciComputer want
type FibonacciComputerLocals struct {
	prev   int
	current int
	position int
	maxCount int
	processed int
	initialized bool
}

// FibonacciComputer computes the next fibonacci number from two using
type FibonacciComputer struct {
	Want
}

// NewFibonacciComputer creates a new fibonacci computer want
func NewFibonacciComputer(metadata Metadata, spec WantSpec) Progressable {
	return &FibonacciComputer{*NewWantWithLocals(
		metadata,
		spec,
		&FibonacciComputerLocals{},
		"fibonacci computer",
	)}
}

// IsAchieved checks if fibonacci computation is complete (end packet received)
func (c *FibonacciComputer) IsAchieved() bool {
	completed, _ := c.GetStateBool("completed", false)
	return completed
}

// Progress returns the generalized chain function for the fibonacci computer
func (c *FibonacciComputer) Progress() {
	in, inputUnavailable := c.GetInputChannel(0)
	if inputUnavailable {
		return
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
			c.Provide(FibonacciSeed{Value: next, Position: position, IsEnd: false})
			prev = current
			current = next
			position++
			processed++
		}
		c.Provide(FibonacciSeed{Value: 0, Position: -1, IsEnd: true})
		c.StoreStateMulti(map[string]any{
			"prev": prev,
			"current": current,
			"position": position,
			"processed": processed,
			"total_processed": processed,
			"completed": true,
		})

		c.StoreLog(fmt.Sprintf("Computed %d fibonacci numbers", processed))
		return
	}
	if !initialized {
		if seed.Position == 0 {
			prev = seed.Value
			c.StoreState("prev", prev)
			return
		} else if seed.Position == 1 {
			current = seed.Value
			initialized = true
			c.StoreStateMulti(map[string]any{
				"current": current,
				"initialized": initialized,
			})
			return
		}
	}
}

// FibonacciMergerLocals holds type-specific local state for FibonacciMerger want
type FibonacciMergerLocals struct {
	seedUsingClosed bool
	computedUsingClosed bool
	processed int
	maxCountReceived bool
}

// FibonacciMerger merges seed values with computed values
type FibonacciMerger struct {
	Want
}

// NewFibonacciMerger creates a new fibonacci merger want
func NewFibonacciMerger(metadata Metadata, spec WantSpec) Progressable {
	return &FibonacciMerger{*NewWantWithLocals(
		metadata,
		spec,
		&FibonacciMergerLocals{},
		"fibonacci merger",
	)}
}

// IsAchieved checks if fibonacci merger is complete (both input channels closed)
func (m *FibonacciMerger) IsAchieved() bool {
	seedUsingClosed, _ := m.GetStateBool("seedUsingClosed", false)
	computedUsingClosed, _ := m.GetStateBool("computedUsingClosed", false)
	return seedUsingClosed && computedUsingClosed
}

// Progress returns the generalized chain function for the fibonacci merger
func (m *FibonacciMerger) Progress() {
	if m.GetInCount() < 2 || m.GetOutCount() < 1 {
		return
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
				m.Provide(fibSeed)
				maxCountReceived = true
				m.StoreState("maxCountReceived", maxCountReceived)
			}
		} else {
			m.Provide(fibSeed) // Send to computer for processing
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

	// End when both using are closed (IsAchieved() will handle completion)
	if seedUsingClosed && computedUsingClosed {
		m.StoreState("total_processed", processed)
		m.StoreLog(fmt.Sprintf("Merged %d fibonacci values", processed))
	}
}

// RegisterFibonacciLoopWantTypes registers the fibonacci loop want types with a ChainBuilder
func RegisterFibonacciLoopWantTypes(builder *ChainBuilder) {
	builder.RegisterWantType("seed numbers", NewSeedNumbers)
	builder.RegisterWantType("fibonacci computer", NewFibonacciComputer)
	builder.RegisterWantType("fibonacci merger", NewFibonacciMerger)
}
