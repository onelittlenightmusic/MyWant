package main

import (
	"fmt"
	"gochain/chain"
)

// FibonacciGenerator generates fibonacci sequence numbers
type FibonacciGenerator struct {
	Want
	Count int
	paths Paths
}

// NewFibonacciGenerator creates a new fibonacci generator want
func NewFibonacciGenerator(metadata Metadata, params map[string]interface{}) *FibonacciGenerator {
	gen := &FibonacciGenerator{
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

// CreateFunction returns the generalized chain function for the generator
func (g *FibonacciGenerator) CreateFunction() func(using []chain.Chan, outputs []chain.Chan) bool {
	return func(using []chain.Chan, outputs []chain.Chan) bool {
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
func (g *FibonacciGenerator) GetWant() *Want {
	return &g.Want
}

// FibonacciFilter filters fibonacci numbers based on criteria
type FibonacciFilter struct {
	Want
	MinValue int
	MaxValue int
	paths    Paths
}

// NewFibonacciFilter creates a new fibonacci filter want
func NewFibonacciFilter(metadata Metadata, params map[string]interface{}) *FibonacciFilter {
	filter := &FibonacciFilter{
		Want: Want{
			Metadata: metadata,
			Spec:     WantSpec{Params: params},
			Stats:    WantStats{},
			Status:   WantStatusIdle,
			State:    make(map[string]interface{}),
		},
		MinValue: 0,
		MaxValue: 1000000,
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
func (f *FibonacciFilter) CreateFunction() func(using []chain.Chan, outputs []chain.Chan) bool {
	return func(using []chain.Chan, outputs []chain.Chan) bool {
		if len(using) == 0 || len(outputs) == 0 {
			return true
		}
		in := using[0]
		out := outputs[0]
		
		for i := range in {
			if val, ok := i.(int); ok {
				// Filter based on min/max values
				if val >= f.MinValue && val <= f.MaxValue {
					out <- val
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
		close(out)
		return true
	}
}

// GetWant returns the underlying Want
func (f *FibonacciFilter) GetWant() *Want {
	return &f.Want
}

// FibonacciSink collects and displays fibonacci numbers
type FibonacciSink struct {
	Want
	numbers []int
	paths   Paths
}

// NewFibonacciSink creates a new fibonacci sink want
func NewFibonacciSink(metadata Metadata, params map[string]interface{}) *FibonacciSink {
	sink := &FibonacciSink{
		Want: Want{
			Metadata: metadata,
			Spec:     WantSpec{Params: params},
			Stats:    WantStats{},
			Status:   WantStatusIdle,
			State:    make(map[string]interface{}),
		},
		numbers: make([]int, 0),
	}
	
	return sink
}

// CreateFunction returns the generalized chain function for the sink
func (s *FibonacciSink) CreateFunction() func(using []chain.Chan, outputs []chain.Chan) bool {
	return func(using []chain.Chan, outputs []chain.Chan) bool {
		if len(using) == 0 {
			return true
		}
		in := using[0]
		
		fmt.Println("Fibonacci numbers:")
		for i := range in {
			if val, ok := i.(int); ok {
				s.numbers = append(s.numbers, val)
				fmt.Printf("%d ", val)
				if s.Stats == nil {
					s.Stats = make(WantStats)
				}
				if val, exists := s.Stats["total_processed"]; exists {
					s.Stats["total_processed"] = val.(int) + 1
				} else {
					s.Stats["total_processed"] = 1
				}
			}
		}
		fmt.Printf("\n\nTotal fibonacci numbers collected: %d\n", len(s.numbers))
		s.StoreState("numbers", s.numbers)
		s.StoreState("count", len(s.numbers))
		return true
	}
}

// GetWant returns the underlying Want
func (s *FibonacciSink) GetWant() *Want {
	return &s.Want
}

// RegisterFibonacciWantTypes registers the fibonacci-specific want types with a ChainBuilder
func RegisterFibonacciWantTypes(builder *ChainBuilder) {
	builder.RegisterWantType("fibonacci_generator", func(metadata Metadata, spec WantSpec) interface{} {
		return NewFibonacciGenerator(metadata, spec.Params)
	})
	
	builder.RegisterWantType("fibonacci_filter", func(metadata Metadata, spec WantSpec) interface{} {
		return NewFibonacciFilter(metadata, spec.Params)
	})
	
	builder.RegisterWantType("fibonacci_sink", func(metadata Metadata, spec WantSpec) interface{} {
		return NewFibonacciSink(metadata, spec.Params)
	})
}