package types

import (
	. "mywant/engine/src"
)

// PrimeNumbersLocals holds type-specific local state for PrimeNumbers want
type PrimeNumbersLocals struct {
	Start int
	End   int
}

// PrimeNumbers creates numbers and sends them downstream
type PrimeNumbers struct {
	Want
}

// NewPrimeNumbers creates a new prime numbers want
func NewPrimeNumbers(metadata Metadata, spec WantSpec) Progressable {
	return &PrimeNumbers{*NewWantWithLocals(
		metadata,
		spec,
		&PrimeNumbersLocals{},
		"prime numbers",
	)}
}

// IsAchieved checks if prime number generation is complete
func (g *PrimeNumbers) IsAchieved() bool {
	start := g.GetIntParam("start", 1)
	end := g.GetIntParam("end", 100)
	currentNumber, _ := g.GetStateInt("current_number", start)
	return currentNumber >= end
}

// Progress returns the generalized chain function for the numbers generator
func (g *PrimeNumbers) Progress() {
	start := g.GetIntParam("start", 1)
	end := g.GetIntParam("end", 100)
	currentNumber, _ := g.GetStateInt("current_number", start)

	currentNumber += 1

	g.Provide(currentNumber)
	if currentNumber >= end {
		// Send end signal
		g.Provide(-1)
	}

	g.StoreState("current_number", currentNumber)
}

// PrimeSequenceLocals holds type-specific local state for PrimeSequence want
type PrimeSequenceLocals struct {
	Prime       int
	foundPrimes []int
}

// PrimeSequence filters out multiples of a prime number
type PrimeSequence struct {
	Want
}

// NewPrimeSequence creates a new prime sequence want
func NewPrimeSequence(metadata Metadata, spec WantSpec) Progressable {
	return &PrimeSequence{*NewWantWithLocals(
		metadata,
		spec,
		&PrimeSequenceLocals{
			foundPrimes: make([]int, 0),
		},
		"prime sequence",
	)}
}

// IsAchieved checks if prime sequence filtering is complete
func (f *PrimeSequence) IsAchieved() bool {
	achieved, _ := f.GetStateBool("achieved", false)
	return achieved
}

// Progress returns the generalized chain function for the filter
// Processes one packet per call and returns false to yield control
// Returns true only when end signal (-1) is received
func (f *PrimeSequence) Progress() {
	locals, ok := f.Locals.(*PrimeSequenceLocals)
	if !ok {
		f.StoreLog("ERROR: Failed to access PrimeSequenceLocals from Want.Locals")
		return
	}

	achieved, _ := f.GetStateBool("achieved", false)
	if achieved {
		return
	}

	totalProcessedVal, _ := f.GetState("total_processed")
	totalProcessed := 0
	if totalProcessedVal != nil {
		if tp, ok := totalProcessedVal.(int); ok {
			totalProcessed = tp
		}
	}

	// Restore foundPrimes from persistent state if it exists
	foundPrimesVal, _ := f.GetState("foundPrimes")
	if foundPrimesVal != nil {
		if fp, ok := foundPrimesVal.([]int); ok {
			locals.foundPrimes = fp
		}
	}

	// Try to receive one packet with timeout
	_, i, ok := f.Use(5000) // 5000ms timeout per packet
	if !ok {
		// No packet available, yield control
		return
	}

	if val, ok := i.(int); ok {
		// Check for end signal
		if val == -1 {
			// End signal received - finalize and complete
			f.StoreStateMulti(map[string]interface{}{
				"foundPrimes":    locals.foundPrimes,
				"primeCount":     len(locals.foundPrimes),
				"total_processed": totalProcessed,
				"achieved":       true,
			})
			return
		}

		totalProcessed++
		isPrime := true

		// Special cases: 1 is not prime, 2 is prime
		if val < 2 {
			isPrime = false
		} else if val == 2 {
			isPrime = true
		} else {
			for _, prime := range locals.foundPrimes {
				if prime*prime > val {
					break // No need to check beyond sqrt(val)
				}
				if val%prime == 0 {
					isPrime = false
					break
				}
			}
		}

		// If it's prime, add to memoized primes
		if isPrime {
			locals.foundPrimes = append(locals.foundPrimes, val)
		}

		// Update state for this packet
		f.StoreStateMulti(map[string]interface{}{
			"total_processed":       totalProcessed,
			"last_number_processed": val,
			"foundPrimes":           locals.foundPrimes,
			"primeCount":            len(locals.foundPrimes),
		})
	}

	// Yield control - will be called again for next packet
}

// // PrimeSink collects and displays results type PrimeSink struct { Want Received int
// paths    Paths }

// // NewPrimeSink creates a new prime sink want func NewPrimeSink(metadata Metadata, spec WantSpec) interface{} { sink := &PrimeSink{ Want:     Want{},
// Received: 0, }

// // Initialize base Want fields sink.Init(metadata, spec)

// // Set fields for base Want methods sink.WantType = "prime sink" sink.ConnectivityMetadata = nil // ConnectivityMetadata loaded from YAML

// func (s *PrimeSink) GetWant() interface{} { return &s.Want }

// // Exec returns the generalized chain function for the sink func (s *PrimeSink) Exec() { // Validate input channel is available in, connectionAvailable := s.GetFirstInputChannel()
// if !connectionAvailable { return true }

// // Check if already achieved using persistent state achieved, _ := s.GetStateBool("achieved", false) if achieved { return true
// 	}

// // Mark as achieved in persistent state s.StoreState("achieved", true)

// // Use persistent state for received count received, _ := s.State["received"].(int)

// primes := make([]int, 0) for val := range in { if prime, ok := val.(int); ok { primes = append(primes, prime)
// received++ } }

// // Update persistent state s.State["received"] = received

// // Store collected primes in state s.StoreStateMulti(map[string]interface{}{ "primes":         primes, "total_received": received,
// 	})

// if s.State == nil { s.State = make(map[string]interface{}) } s.State["total_processed"] = received

// RegisterPrimeWantTypes registers the prime-specific want types with a ChainBuilder
func RegisterPrimeWantTypes(builder *ChainBuilder) {
	builder.RegisterWantType("prime numbers", NewPrimeNumbers)
	builder.RegisterWantType("prime sequence", NewPrimeSequence)
}
