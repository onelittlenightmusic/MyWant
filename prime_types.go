package main

import (
	"fmt"
	"gochain/chain"
)

// PrimeGenerator creates numbers and sends them downstream
type PrimeGenerator struct {
	Node
	Start int
	End   int
	paths Paths
}

// NewPrimeGenerator creates a new prime generator node
func NewPrimeGenerator(metadata Metadata, params map[string]interface{}) *PrimeGenerator {
	gen := &PrimeGenerator{
		Node: Node{
			Metadata: metadata,
			Spec:     NodeSpec{Params: params},
			Stats:    NodeStats{},
			Status:   NodeStatusIdle,
			State:    make(map[string]interface{}),
		},
		Start: 2,
		End:   100,
	}
	
	if s, ok := params["start"]; ok {
		if si, ok := s.(int); ok {
			gen.Start = si
		} else if sf, ok := s.(float64); ok {
			gen.Start = int(sf)
		}
	}
	if e, ok := params["end"]; ok {
		if ei, ok := e.(int); ok {
			gen.End = ei
		} else if ef, ok := e.(float64); ok {
			gen.End = int(ef)
		}
	}
	
	return gen
}

// CreateFunction returns the generalized chain function for the generator
func (g *PrimeGenerator) CreateFunction() func(inputs []chain.Chan, outputs []chain.Chan) bool {
	return func(inputs []chain.Chan, outputs []chain.Chan) bool {
		if len(outputs) == 0 {
			return true
		}
		out := outputs[0]
		for i := g.Start; i <= g.End; i++ {
			out <- i
		}
		close(out)
		return true
	}
}

// GetNode returns the underlying Node
func (g *PrimeGenerator) GetNode() *Node {
	return &g.Node
}

// PrimeFilter filters out multiples of a prime number
type PrimeFilter struct {
	Node
	Prime int
	foundPrimes []int
	paths Paths
}

// NewPrimeFilter creates a new prime filter node
func NewPrimeFilter(metadata Metadata, params map[string]interface{}) *PrimeFilter {
	filter := &PrimeFilter{
		Node: Node{
			Metadata: metadata,
			Spec:     NodeSpec{Params: params},
			Stats:    NodeStats{},
			Status:   NodeStatusIdle,
			State:    make(map[string]interface{}),
		},
		Prime: 2,
		foundPrimes: make([]int, 0),
	}
	
	if p, ok := params["prime"]; ok {
		if pi, ok := p.(int); ok {
			filter.Prime = pi
		} else if pf, ok := p.(float64); ok {
			filter.Prime = int(pf)
		}
	}
	
	return filter
}

// CreateFunction returns the generalized chain function for the filter
func (f *PrimeFilter) CreateFunction() func(inputs []chain.Chan, outputs []chain.Chan) bool {
	return func(inputs []chain.Chan, outputs []chain.Chan) bool {
		if len(inputs) == 0 || len(outputs) == 0 {
			return true
		}
		in := inputs[0]
		out := outputs[0]
		
		for i := range in {
			if val, ok := i.(int); ok {
				isPrime := true
				
				// Special cases: 1 is not prime, 2 is prime
				if val < 2 {
					isPrime = false
				} else if val == 2 {
					isPrime = true
				} else {
					// Check if val can be divided by any previously found prime
					for _, prime := range f.foundPrimes {
						if prime*prime > val {
							break // No need to check beyond sqrt(val)
						}
						if val%prime == 0 {
							isPrime = false
							break
						}
					}
				}
				
				// If it's prime, add to memoized primes and pass through
				if isPrime {
					f.foundPrimes = append(f.foundPrimes, val)
					out <- val
				}
				
				f.Stats.TotalProcessed++
			}
		}
		close(out)
		return true
	}
}

// GetNode returns the underlying Node
func (f *PrimeFilter) GetNode() *Node {
	return &f.Node
}

// PrimeSink collects and displays prime numbers
type PrimeSink struct {
	Node
	primes []int
	paths  Paths
}

// NewPrimeSink creates a new prime sink node
func NewPrimeSink(metadata Metadata, params map[string]interface{}) *PrimeSink {
	sink := &PrimeSink{
		Node: Node{
			Metadata: metadata,
			Spec:     NodeSpec{Params: params},
			Stats:    NodeStats{},
			Status:   NodeStatusIdle,
			State:    make(map[string]interface{}),
		},
		primes: make([]int, 0),
	}
	
	return sink
}

// CreateFunction returns the generalized chain function for the sink
func (s *PrimeSink) CreateFunction() func(inputs []chain.Chan, outputs []chain.Chan) bool {
	return func(inputs []chain.Chan, outputs []chain.Chan) bool {
		if len(inputs) == 0 {
			return true
		}
		in := inputs[0]
		
		for i := range in {
			if val, ok := i.(int); ok {
				s.primes = append(s.primes, val)
				fmt.Printf("%d\n", val)
				s.Stats.TotalProcessed++
			}
		}
		s.StoreState("primes", s.primes)
		s.StoreState("count", len(s.primes))
		return true
	}
}

// GetNode returns the underlying Node
func (s *PrimeSink) GetNode() *Node {
	return &s.Node
}

// RegisterPrimeNodeTypes registers the prime-specific node types with a ChainBuilder
func RegisterPrimeNodeTypes(builder *ChainBuilder) {
	builder.RegisterNodeType("prime_generator", func(metadata Metadata, params map[string]interface{}) interface{} {
		return NewPrimeGenerator(metadata, params)
	})
	
	builder.RegisterNodeType("prime_filter", func(metadata Metadata, params map[string]interface{}) interface{} {
		return NewPrimeFilter(metadata, params)
	})
	
	builder.RegisterNodeType("prime_sink", func(metadata Metadata, params map[string]interface{}) interface{} {
		return NewPrimeSink(metadata, params)
	})
}