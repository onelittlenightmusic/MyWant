package types

import (
	. "mywant/engine/src"
)

// FibonacciNumbers generates fibonacci sequence numbers
type FibonacciNumbers struct {
	Want
	Count int
	paths Paths
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
	// Read parameters fresh each cycle - enables dynamic changes!
	count := g.GetIntParam("count", 20)

	// Check if already completed using persistent state
	completed, _ := g.GetStateBool("completed", false)

	// Validate output channel is available
	out, connectionAvailable := g.GetFirstOutputChannel()
	if !connectionAvailable {
		return true
	}

	if completed {
		return true
	}

	// Mark as completed in persistent state
	g.StoreState("completed", true)

	a, b := 0, 1
	for i := 0; i < count; i++ {
		out <- a
		a, b = b, a+b
	}
	close(out)
	return true
}

// GetWant returns the underlying Want
func (g *FibonacciNumbers) GetWant() interface{} {
	return &g.Want
}

// FibonacciSequence filters fibonacci numbers based on criteria
type FibonacciSequence struct {
	Want
	MinValue int
	MaxValue int
	filtered []int
	paths    Paths
}

// NewFibonacciSequence creates a new fibonacci sequence want
func NewFibonacciSequence(metadata Metadata, spec WantSpec) interface{} {
	filter := &FibonacciSequence{
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
	filter.WantType = "fibonacci sequence"
	filter.ConnectivityMetadata = ConnectivityMetadata{
		RequiredInputs:  1,
		RequiredOutputs: 0,
		MaxInputs:       1,
		MaxOutputs:      0,
		WantType:        "fibonacci sequence",
		Description:     "Fibonacci number sequence filter (terminal)",
	}

	return filter
}

func (f *FibonacciSequence) GetWant() interface{} {
	return &f.Want
}

// Exec returns the generalized chain function for the filter
func (f *FibonacciSequence) Exec() bool {
	// Read parameters fresh each cycle - enables dynamic changes!
	minValue := f.GetIntParam("min_value", 0)

	maxValue := f.GetIntParam("max_value", 1000000)

	// Validate input channel is available
	in, connectionAvailable := f.GetFirstInputChannel()
	if !connectionAvailable {
		return true
	}

	// Check if already completed using persistent state
	completed, _ := f.GetStateBool("completed", false)
	if completed {
		return true
	}

	// Mark as completed in persistent state
	f.StoreState("completed", true)

	// Use local field to track filtered numbers
	f.filtered = make([]int, 0)
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

	// Close any output channels (though this should be the end point)
	for i := 0; i < f.paths.GetOutCount(); i++ {
		close(f.paths.Out[i].Channel)
	}

	// Store final state - persist filtered slice and counts
	f.State["filtered"] = f.filtered
	f.StoreStateMulti(map[string]interface{}{
		"filtered":        f.filtered,
		"count":           len(f.filtered),
		"total_processed": totalProcessed,
	})


	return true
}

// FibonacciFilter is an alias for FibonacciSequence when used in recipes
func NewFibonacciFilter(metadata Metadata, spec WantSpec) interface{} {
	// Reuse FibonacciSequence implementation but with "fibonacci filter" type
	filter := &FibonacciSequence{
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

// RegisterFibonacciWantTypes registers the fibonacci-specific want types with a ChainBuilder
func RegisterFibonacciWantTypes(builder *ChainBuilder) {
	builder.RegisterWantType("fibonacci numbers", NewFibonacciNumbers)
	builder.RegisterWantType("fibonacci sequence", NewFibonacciSequence)
	builder.RegisterWantType("fibonacci filter", NewFibonacciFilter)
}
