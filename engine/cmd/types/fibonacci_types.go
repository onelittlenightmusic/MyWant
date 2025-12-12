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
	)

	locals := want.Locals.(*FibonacciNumbersLocals)
	locals.Count = want.GetIntParam("count", 20)

	return &FibonacciNumbers{*want}
}

// Exec returns the generalized chain function for the numbers generator
func (g *FibonacciNumbers) Exec() bool {
	count := g.GetIntParam("count", 20)
	a, _ := g.GetStateInt("a", 0)
	b, _ := g.GetStateInt("b", 1)
	sentCount, _ := g.GetStateInt("sent_count", 0)

	if sentCount >= count {
		// Send end signal
		g.SendPacketMulti(-1)
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
	)

	locals := want.Locals.(*FibonacciFilterLocals)
	locals.MinValue = want.GetIntParam("min_value", 0)
	locals.MaxValue = want.GetIntParam("max_value", 1000000)
	locals.filtered = make([]int, 0)

	return want
}

// Exec returns the generalized chain function for the filter
// Processes one packet per call and returns false to yield control
// Returns true only when end signal (-1) is received
func (f *FibonacciFilter) Exec() bool {
	locals, ok := f.Locals.(*FibonacciFilterLocals)
	if !ok {
		f.StoreLog("ERROR: Failed to access FibonacciFilterLocals from Want.Locals")
		return true
	}

	// Check if already achieved from previous execution
	achieved, _ := f.GetStateBool("achieved", false)
	if achieved {
		return true
	}

	totalProcessedVal, _ := f.GetState("total_processed")
	totalProcessed := 0
	if totalProcessedVal != nil {
		if tp, ok := totalProcessedVal.(int); ok {
			totalProcessed = tp
		}
	}

	// Restore filtered array from persistent state if it exists
	filteredVal, _ := f.GetState("filtered")
	if filteredVal != nil {
		if flt, ok := filteredVal.([]int); ok {
			locals.filtered = flt
		}
	}

	// Try to receive one packet with timeout
	_, i, ok := f.ReceiveFromAnyInputChannel(5000) // 5000ms timeout per packet
	if !ok {
		// No packet available, yield control
		return false
	}

	if val, ok := i.(int); ok {
		// Check for end signal
		if val == -1 {
			// End signal received - finalize and complete
			f.StoreStateMulti(map[string]interface{}{
				"filtered":        locals.filtered,
				"count":           len(locals.filtered),
				"total_processed": totalProcessed,
				"achieved":        true,
			})
			return true
		}

		totalProcessed++
		// Filter based on min/max values
		if val >= locals.MinValue && val <= locals.MaxValue {
			locals.filtered = append(locals.filtered, val)
		}

		// Update state for this packet
		f.StoreStateMulti(map[string]interface{}{
			"total_processed":       totalProcessed,
			"filtered":              locals.filtered,
			"count":                 len(locals.filtered),
			"last_number_processed": val,
		})
	}

	// Yield control - will be called again for next packet
	return false
}

// RegisterFibonacciWantTypes registers the fibonacci-specific want types with a ChainBuilder
func RegisterFibonacciWantTypes(builder *ChainBuilder) {
	builder.RegisterWantType("fibonacci numbers", NewFibonacciNumbers)
	builder.RegisterWantType("fibonacci filter", NewFibonacciFilter)
}
