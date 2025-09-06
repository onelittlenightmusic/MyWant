package main

import (
	. "mywant"
)

// PrimeNumbers creates numbers and sends them downstream
type PrimeNumbers struct {
	Want
	Start int
	End   int
	paths Paths
}

// NewPrimeNumbers creates a new prime numbers want
func NewPrimeNumbers(metadata Metadata, params map[string]interface{}) *PrimeNumbers {
	gen := &PrimeNumbers{
		Want: Want{
			Metadata: metadata,
			Spec:     WantSpec{Params: params},
			Stats:    WantStats{},
			Status:   WantStatusIdle,
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

// CreateFunction returns the generalized chain function for the numbers generator
func (g *PrimeNumbers) CreateFunction() func(using []Chan, outputs []Chan) bool {
	return func(using []Chan, outputs []Chan) bool {
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

// GetWant returns the underlying Want
func (g *PrimeNumbers) GetWant() *Want {
	return &g.Want
}

// PrimeSequence filters out multiples of a prime number
type PrimeSequence struct {
	Want
	Prime int
	foundPrimes []int
	paths Paths
}

// NewPrimeSequence creates a new prime sequence want
func NewPrimeSequence(metadata Metadata, params map[string]interface{}) *PrimeSequence {
	filter := &PrimeSequence{
		Want: Want{
			Metadata: metadata,
			Spec:     WantSpec{Params: params},
			Stats:    WantStats{},
			Status:   WantStatusIdle,
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
func (f *PrimeSequence) CreateFunction() func(using []Chan, outputs []Chan) bool {
	return func(using []Chan, outputs []Chan) bool {
		if len(using) == 0 {
			return true
		}
		in := using[0]
		
		var out Chan
		if len(outputs) > 0 {
			out = outputs[0]
		}
		
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
					if out != nil {
						out <- val
					}
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
		if out != nil {
			close(out)
		}
		
		// Store found primes in state for collection
		f.StoreState("foundPrimes", f.foundPrimes)
		f.StoreState("primeCount", len(f.foundPrimes))
		
		return true
	}
}

// InitializePaths initializes the paths for this sequence
func (f *PrimeSequence) InitializePaths(inCount, outCount int) {
	f.paths.In = make([]PathInfo, inCount)
	f.paths.Out = make([]PathInfo, outCount)
}

// GetConnectivityMetadata returns connectivity requirements for prime sequence
func (f *PrimeSequence) GetConnectivityMetadata() ConnectivityMetadata {
	return ConnectivityMetadata{
		RequiredInputs:  1,
		RequiredOutputs: 0, // Optional output
		MaxInputs:       1,
		MaxOutputs:      -1,
		WantType:        "prime_sequence",
		Description:     "Prime number sequence",
	}
}

// GetStats returns the stats for this sequence
func (f *PrimeSequence) GetStats() map[string]interface{} {
	return f.Stats
}

// Process processes using enhanced paths
func (f *PrimeSequence) Process(paths Paths) bool {
	f.paths = paths
	return false
}

// GetType returns the want type
func (f *PrimeSequence) GetType() string {
	return "prime_sequence"
}

// GetWant returns the underlying Want
func (f *PrimeSequence) GetWant() *Want {
	return &f.Want
}


// RegisterPrimeWantTypes registers the prime-specific want types with a ChainBuilder
func RegisterPrimeWantTypes(builder *ChainBuilder) {
	builder.RegisterWantType("prime_numbers", func(metadata Metadata, spec WantSpec) interface{} {
		return NewPrimeNumbers(metadata, spec.Params)
	})
	
	builder.RegisterWantType("prime_sequence", func(metadata Metadata, spec WantSpec) interface{} {
		return NewPrimeSequence(metadata, spec.Params)
	})
}