package types

import (
	. "mywant/engine/src"
)

// FibonacciNumbersLocals holds type-specific local state for FibonacciNumbers want
type FibonacciNumbersLocals struct {
	Count int
}

// FibonacciNumbers generates fibonacci sequence numbers
type FibonacciNumbers struct {
	Want
}

// NewFibonacciNumbers creates a new fibonacci numbers want
func NewFibonacciNumbers(metadata Metadata, spec WantSpec) interface{} {
	want := NewWant(
		metadata,
		spec,
		func() WantLocals { return &FibonacciNumbersLocals{} },
		ConnectivityMetadata{
			RequiredInputs:  0,
			RequiredOutputs: 1,
			MaxInputs:       0,
			MaxOutputs:      -1,
			WantType:        "fibonacci numbers",
			Description:     "Fibonacci sequence generator",
		},
		"fibonacci numbers",
	).(*Want)

	locals := want.Locals.(*FibonacciNumbersLocals)
	locals.Count = want.GetIntParam("count", 20)

	return want
}

// Exec returns the generalized chain function for the numbers generator
func (g *FibonacciNumbers) Exec() bool {
	count := g.GetIntParam("count", 20)
	a, _ := g.GetStateInt("a", 0)
	b, _ := g.GetStateInt("b", 1)
	sentCount, _ := g.GetStateInt("sent_count", 0)

	if sentCount >= count {
		return true
	}

	g.SendPacketMulti(a)
	g.StoreStateMulti(map[string]interface{}{
		"a":          b,
		"b":          a + b,
		"sent_count": sentCount + 1,
	})

	return false
}

// FibonacciFilterLocals holds type-specific local state for FibonacciFilter want
type FibonacciFilterLocals struct {
	MinValue int
	MaxValue int
	filtered []int
}

// FibonacciFilter filters fibonacci numbers based on criteria
type FibonacciFilter struct {
	Want
}

// NewFibonacciFilter creates a new fibonacci filter want
func NewFibonacciFilter(metadata Metadata, spec WantSpec) interface{} {
	want := NewWant(
		metadata,
		spec,
		func() WantLocals { return &FibonacciFilterLocals{} },
		ConnectivityMetadata{
			RequiredInputs:  1,
			RequiredOutputs: 0,
			MaxInputs:       1,
			MaxOutputs:      0,
			WantType:        "fibonacci filter",
			Description:     "Fibonacci number filter (terminal)",
		},
		"fibonacci filter",
	).(*Want)

	locals := want.Locals.(*FibonacciFilterLocals)
	locals.MinValue = want.GetIntParam("min_value", 0)
	locals.MaxValue = want.GetIntParam("max_value", 1000000)
	locals.filtered = make([]int, 0)

	return want
}

// Exec returns the generalized chain function for the filter
func (f *FibonacciFilter) Exec() bool {
	locals, ok := f.Locals.(*FibonacciFilterLocals)
	if !ok {
		f.StoreLog("ERROR: Failed to access FibonacciFilterLocals from Want.Locals")
		return true
	}

	achieved, _ := f.GetStateBool("achieved", false)
	if achieved {
		return true
	}

	totalProcessed := 0
	_, i, ok := f.ReceiveFromAnyInputChannel(100)
	if !ok {
		f.StoreState("achieved", true)

		return true
	}

	if val, ok := i.(int); ok {
		totalProcessed++
		// Filter based on min/max values
		if val >= locals.MinValue && val <= locals.MaxValue {
			locals.filtered = append(locals.filtered, val)
		}
	}
	f.StoreStateMulti(map[string]interface{}{
		"filtered":        locals.filtered,
		"count":           len(locals.filtered),
		"total_processed": totalProcessed,
	})

	return false
}

// RegisterFibonacciWantTypes registers the fibonacci-specific want types with a ChainBuilder
func RegisterFibonacciWantTypes(builder *ChainBuilder) {
	builder.RegisterWantType("fibonacci numbers", NewFibonacciNumbers)
	builder.RegisterWantType("fibonacci filter", NewFibonacciFilter)
}
