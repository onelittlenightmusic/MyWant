package main

import (
	. "mywant/src"
)

// FibonacciNumbers generates fibonacci sequence numbers
type FibonacciNumbers struct {
	Want
	Count int
	paths Paths
}

// NewFibonacciNumbers creates a new fibonacci numbers want
func NewFibonacciNumbers(metadata Metadata, params map[string]interface{}) *FibonacciNumbers {
	gen := &FibonacciNumbers{
		Want: Want{
			Metadata: metadata,
			Spec:     WantSpec{Params: params},
			Stats:    WantStats{},
			Status:   WantStatusIdle,
			State:    make(map[string]interface{}),
		},
		Count: 20,
	}
	
	if c, ok := params["count"]; ok {
		if ci, ok := c.(int); ok {
			gen.Count = ci
		} else if cf, ok := c.(float64); ok {
			gen.Count = int(cf)
		}
	}
	
	return gen
}

// CreateFunction returns the generalized chain function for the numbers generator
func (g *FibonacciNumbers) CreateFunction() func(using []Chan, outputs []Chan) bool {
	return func(using []Chan, outputs []Chan) bool {
		if len(outputs) == 0 {
			return true
		}
		out := outputs[0]
		a, b := 0, 1
		for i := 0; i < g.Count; i++ {
			out <- a
			a, b = b, a+b
		}
		close(out)
		return true
	}
}

// GetWant returns the underlying Want
func (g *FibonacciNumbers) GetWant() *Want {
	return &g.Want
}

// FibonacciSequence filters fibonacci numbers based on criteria
type FibonacciSequence struct {
	Want
	MinValue   int
	MaxValue   int
	filtered   []int
	paths      Paths
}

// NewFibonacciSequence creates a new fibonacci sequence want
func NewFibonacciSequence(metadata Metadata, params map[string]interface{}) *FibonacciSequence {
	filter := &FibonacciSequence{
		Want: Want{
			Metadata: metadata,
			Spec:     WantSpec{Params: params},
			Stats:    WantStats{},
			Status:   WantStatusIdle,
			State:    make(map[string]interface{}),
		},
		MinValue: 0,
		MaxValue: 1000000,
		filtered: make([]int, 0),
	}
	
	if min, ok := params["min_value"]; ok {
		if mini, ok := min.(int); ok {
			filter.MinValue = mini
		} else if minf, ok := min.(float64); ok {
			filter.MinValue = int(minf)
		}
	}
	
	if max, ok := params["max_value"]; ok {
		if maxi, ok := max.(int); ok {
			filter.MaxValue = maxi
		} else if maxf, ok := max.(float64); ok {
			filter.MaxValue = int(maxf)
		}
	}
	
	return filter
}

// CreateFunction returns the generalized chain function for the filter
func (f *FibonacciSequence) CreateFunction() func(using []Chan, outputs []Chan) bool {
	return func(using []Chan, outputs []Chan) bool {
		if len(using) == 0 {
			return true
		}
		in := using[0]
		
		// Process all input numbers and filter them
		for i := range in {
			if val, ok := i.(int); ok {
				// Filter based on min/max values
				if val >= f.MinValue && val <= f.MaxValue {
					f.filtered = append(f.filtered, val)
					// Update state immediately when number is filtered
					f.StoreState("filtered", f.filtered)
					f.StoreState("count", len(f.filtered))
				}
				if f.Stats == nil {
					f.Stats = make(WantStats)
				}
				if val, exists := f.Stats["total_processed"]; exists {
					f.Stats["total_processed"] = val.(int) + 1
				} else {
					f.Stats["total_processed"] = 1
				}
			}
		}
		
		// Close any output channels (though this should be the end point)
		for _, out := range outputs {
			close(out)
		}
		
		// Final state update to ensure consistency (in case no numbers were filtered)
		f.StoreState("filtered", f.filtered)
		f.StoreState("count", len(f.filtered))
		
		// Display collected results
		println("🔢 Filtered fibonacci numbers:", len(f.filtered), "numbers between", f.MinValue, "and", f.MaxValue)
		for i, num := range f.filtered {
			if i < 10 { // Show first 10 numbers
				print(num, " ")
			}
		}
		if len(f.filtered) > 10 {
			println("... (", len(f.filtered)-10, "more)")
		} else {
			println()
		}
		
		return true
	}
}

// InitializePaths initializes the paths for this sequence
func (f *FibonacciSequence) InitializePaths(inCount, outCount int) {
	f.paths.In = make([]PathInfo, inCount)
	f.paths.Out = make([]PathInfo, outCount)
}

// GetConnectivityMetadata returns connectivity requirements for fibonacci sequence
func (f *FibonacciSequence) GetConnectivityMetadata() ConnectivityMetadata {
	return ConnectivityMetadata{
		RequiredInputs:  1,
		RequiredOutputs: 0, // No outputs - this is a terminal want
		MaxInputs:       1,
		MaxOutputs:      0, // Terminal want - no outputs allowed
		WantType:        "fibonacci_sequence",
		Description:     "Fibonacci number sequence filter (terminal)",
	}
}

// GetStats returns the stats for this sequence
func (f *FibonacciSequence) GetStats() map[string]interface{} {
	return f.Stats
}

// Process processes using enhanced paths
func (f *FibonacciSequence) Process(paths Paths) bool {
	f.paths = paths
	return false
}

// GetType returns the want type
func (f *FibonacciSequence) GetType() string {
	return "fibonacci_sequence"
}

// GetWant returns the underlying Want
func (f *FibonacciSequence) GetWant() *Want {
	return &f.Want
}


// RegisterFibonacciWantTypes registers the fibonacci-specific want types with a ChainBuilder
func RegisterFibonacciWantTypes(builder *ChainBuilder) {
	builder.RegisterWantType("fibonacci_numbers", func(metadata Metadata, spec WantSpec) interface{} {
		return NewFibonacciNumbers(metadata, spec.Params)
	})
	
	builder.RegisterWantType("fibonacci_sequence", func(metadata Metadata, spec WantSpec) interface{} {
		return NewFibonacciSequence(metadata, spec.Params)
	})
}