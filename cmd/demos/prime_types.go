package main

import (
	. "mywant/src"
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
			// Stats field removed - using State instead
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

// Exec returns the generalized chain function for the numbers generator
func (g *PrimeNumbers) Exec(using []Chan, outputs []Chan) bool {
	// Read parameters fresh each cycle - enables dynamic changes!
	start := 2
	if s, ok := g.Spec.Params["start"]; ok {
		if si, ok := s.(int); ok {
			start = si
		} else if sf, ok := s.(float64); ok {
			start = int(sf)
		}
	}

	end := 100
	if e, ok := g.Spec.Params["end"]; ok {
		if ei, ok := e.(int); ok {
			end = ei
		} else if ef, ok := e.(float64); ok {
			end = int(ef)
		}
	}

	// Check if already completed using persistent state
	completed, _ := g.State["completed"].(bool)

	if len(outputs) == 0 {
		return true
	}
	out := outputs[0]

	if completed {
		return true
	}

	// Mark as completed in persistent state
	g.State["completed"] = true

	for i := start; i <= end; i++ {
		out <- i
	}
	close(out)
	return true
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
			// Stats field removed - using State instead
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

// Exec returns the generalized chain function for the filter
func (f *PrimeSequence) Exec(using []Chan, outputs []Chan) bool {
	// Read parameters fresh each cycle - enables dynamic changes!
	// Note: prime parameter available but not used in current implementation
	if len(using) == 0 {
		return true
	}
	in := using[0]

	var out Chan
	if len(outputs) > 0 {
		out = outputs[0]
	}

	// Get persistent foundPrimes slice or create new one
	foundPrimes, _ := f.State["foundPrimes"].([]int)
	if foundPrimes == nil {
		foundPrimes = make([]int, 0)
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
				for _, prime := range foundPrimes {
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
				foundPrimes = append(foundPrimes, val)
				f.State["foundPrimes"] = foundPrimes
				if out != nil {
					out <- val
				}
				// Update live state immediately when prime is found
				f.StoreState("foundPrimes", foundPrimes)
				f.StoreState("primeCount", len(foundPrimes))
				f.StoreState("lastPrimeFound", val)
			}

			if f.State == nil {
				f.State = make(map[string]interface{})
			}
			if val, exists := f.State["total_processed"]; exists {
				f.State["total_processed"] = val.(int) + 1
			} else {
				f.State["total_processed"] = 1
			}

			// Update live state for each processed number
			f.StoreState("total_processed", f.State["total_processed"])
			f.StoreState("last_number_processed", val)
		}
	}
	if out != nil {
		close(out)
	}

	// Store found primes in state for collection
	f.State["foundPrimes"] = foundPrimes
	f.StoreState("foundPrimes", foundPrimes)
	f.StoreState("primeCount", len(foundPrimes))

	return true
}

// InitializePaths initializes the paths for this sequence
func (f *PrimeSequence) InitializePaths(inCount int, outCount int) {
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
	return f.State
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


// PrimeSink collects and displays results
type PrimeSink struct {
	Want
	Received int
	paths    Paths
}

// NewPrimeSink creates a new prime sink want
func NewPrimeSink(metadata Metadata, spec WantSpec) *PrimeSink {
	return &PrimeSink{
		Want: Want{
			Metadata: metadata,
			Spec:     spec,
			// Stats field removed - using State instead
			Status:   WantStatusIdle,
			State:    make(map[string]interface{}),
		},
		Received: 0,
	}
}

// Exec returns the generalized chain function for the sink
func (s *PrimeSink) Exec(using []Chan, outputs []Chan) bool {
	if len(using) == 0 {
		return true
	}

	// Use persistent state for received count
	received, _ := s.State["received"].(int)

	primes := make([]int, 0)
	for val := range using[0] {
		if prime, ok := val.(int); ok {
			primes = append(primes, prime)
			received++
		}
	}

	// Update persistent state
	s.State["received"] = received

	// Store collected primes in state
	s.StoreState("primes", primes)
	s.StoreState("total_received", received)

	if s.State == nil {
		s.State = make(map[string]interface{})
	}
	s.State["total_processed"] = received

	return true
}

// GetWant returns the underlying Want
func (s *PrimeSink) GetWant() *Want {
	return &s.Want
}

// RegisterPrimeWantTypes registers the prime-specific want types with a ChainBuilder
func RegisterPrimeWantTypes(builder *ChainBuilder) {
	builder.RegisterWantType("prime_numbers", func(metadata Metadata, spec WantSpec) interface{} {
		return NewPrimeNumbers(metadata, spec.Params)
	})
	
	builder.RegisterWantType("prime_sequence", func(metadata Metadata, spec WantSpec) interface{} {
		return NewPrimeSequence(metadata, spec.Params)
	})
	
	// Register sink type for collecting results
	builder.RegisterWantType("sink", func(metadata Metadata, spec WantSpec) interface{} {
		return NewPrimeSink(metadata, spec)
	})
}