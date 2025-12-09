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
	gen.Init(metadata, spec)

	gen.Count = gen.GetIntParam("count", 20)

	return gen
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
	filter.Init(metadata, spec)

	filter.MinValue = filter.GetIntParam("min_value", 0)
	filter.MaxValue = filter.GetIntParam("max_value", 1000000)
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

// Exec returns the generalized chain function for the filter
func (f *FibonacciFilter) Exec() bool {
	minValue := f.GetIntParam("min_value", 0)
	maxValue := f.GetIntParam("max_value", 1000000)
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
		if val >= minValue && val <= maxValue {
			f.filtered = append(f.filtered, val)
		}
	}
	f.StoreStateMulti(map[string]interface{}{
		"filtered":        f.filtered,
		"count":           len(f.filtered),
		"total_processed": totalProcessed,
	})

	return false
}

// RegisterFibonacciWantTypes registers the fibonacci-specific want types with a ChainBuilder
func RegisterFibonacciWantTypes(builder *ChainBuilder) {
	builder.RegisterWantType("fibonacci numbers", NewFibonacciNumbers)
	builder.RegisterWantType("fibonacci filter", NewFibonacciFilter)
}
