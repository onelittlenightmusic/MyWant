package types

import (
	. "mywant/engine/src"
)

// FibonacciNumbers generates fibonacci sequence numbers
type FibonacciNumbers struct {
	Want
	Count int
}

// NewFibonacciNumbers creates a new fibonacci numbers want
func NewFibonacciNumbers(metadata Metadata, spec WantSpec) interface{} {
	gen := &FibonacciNumbers{
		Want:  Want{},
		Count: 20,
	}

	// Initialize base Want fields
	gen.Init(metadata, spec)

	gen.Count = gen.GetIntParam("count", 20)

	return gen
}

// Exec returns the generalized chain function for the numbers generator
func (g *FibonacciNumbers) Exec() bool {
	defer func() {
		for _, path := range g.GetPaths().Out {
			close(path.Channel)
		}
	}()

	// Read parameters fresh each cycle - enables dynamic changes!
	count := g.GetIntParam("count", 20)

	// Check if already achieved using persistent state
	achieved, _ := g.GetStateBool("achieved", false)

	if achieved {
		return true
	}

	// Mark as achieved in persistent state
	g.StoreState("achieved", true)

	a, b := 0, 1
	for i := 0; i < count; i++ {
		g.SendPacketMulti(a)
		a, b = b, a+b
	}
	return true
}

// GetWant returns the underlying Want
func (g *FibonacciNumbers) GetWant() interface{} {
	return &g.Want
}

// FibonacciFilter filters fibonacci numbers based on criteria
type FibonacciFilter struct {
	Want
	MinValue int
	MaxValue int
	filtered []int
}

// NewFibonacciFilter creates a new fibonacci filter want
func NewFibonacciFilter(metadata Metadata, spec WantSpec) interface{} {
	filter := &FibonacciFilter{
		Want:     Want{},
		MinValue: 0,
		MaxValue: 1000000,
		filtered: make([]int, 0),
	}

	// Initialize base Want fields
	filter.Init(metadata, spec)

	filter.MinValue = filter.GetIntParam("min_value", 0)
	filter.MaxValue = filter.GetIntParam("max_value", 1000000)

	// Set fields for base Want methods
	filter.WantType = "fibonacci filter"
	filter.ConnectivityMetadata = ConnectivityMetadata{
		RequiredInputs:  1,
		RequiredOutputs: 0,
		MaxInputs:       1,
		MaxOutputs:      0,
		WantType:        "fibonacci filter",
		Description:     "Fibonacci number filter (terminal)",
	}

	return filter
}

func (f *FibonacciFilter) GetWant() interface{} {
	return &f.Want
}

// Exec returns the generalized chain function for the filter
func (f *FibonacciFilter) Exec() bool {
	// Read parameters fresh each cycle - enables dynamic changes!
	minValue := f.GetIntParam("min_value", 0)
	maxValue := f.GetIntParam("max_value", 1000000)

	// Validate input channel is available
	in, connectionAvailable := f.GetFirstInputChannel()
	if !connectionAvailable {
		return true
	}

	// Check if already achieved using persistent state
	achieved, _ := f.GetStateBool("achieved", false)
	if achieved {
		return true
	}

	totalProcessed := 0

	// Process all input numbers and filter them
	for i := range in {
		if val, ok := i.(int); ok {
			totalProcessed++
			// Filter based on min/max values
			if val >= minValue && val <= maxValue {
				f.filtered = append(f.filtered, val)
			}
		}
	}

	// Store final state - persist filtered slice and counts using StoreState only
	f.StoreStateMulti(map[string]interface{}{
		"filtered":        f.filtered,
		"count":           len(f.filtered),
		"total_processed": totalProcessed,
	})

	// Mark as achieved in persistent state after processing all inputs and storing state
	f.StoreState("achieved", true)

	return true
}

// RegisterFibonacciWantTypes registers the fibonacci-specific want types with a ChainBuilder
func RegisterFibonacciWantTypes(builder *ChainBuilder) {
	builder.RegisterWantType("fibonacci numbers", NewFibonacciNumbers)
	builder.RegisterWantType("fibonacci filter", NewFibonacciFilter)
}
