package types

import (
	. "mywant/engine/src"
)

// PrimeNumbers creates numbers and sends them downstream
type PrimeNumbers struct {
	Want
	Start int
	End   int
	paths Paths
}

// NewPrimeNumbers creates a new prime numbers want
func NewPrimeNumbers(metadata Metadata, spec WantSpec) interface{} {
	gen := &PrimeNumbers{
		Want:  Want{},
		Start: 2,
		End:   100,
	}
	gen.Init(metadata, spec)

	gen.Start = gen.GetIntParam("start", 2)
	gen.End = gen.GetIntParam("end", 100)

	return gen
}

// Exec returns the generalized chain function for the numbers generator
func (g *PrimeNumbers) Exec() bool {
	start := g.GetIntParam("start", 2)
	end := g.GetIntParam("end", 100)
	currentNumber, exists := g.GetStateInt("current_number", start)
	if !exists {
		currentNumber = start
	}

	if currentNumber > end {
		return true
	}

	g.SendPacketMulti(currentNumber)
	g.StoreState("current_number", currentNumber+1)

	return false
}

// PrimeSequence filters out multiples of a prime number
type PrimeSequence struct {
	Want
	Prime       int
	foundPrimes []int
	paths       Paths
}

// NewPrimeSequence creates a new prime sequence want
func NewPrimeSequence(metadata Metadata, spec WantSpec) interface{} {
	filter := &PrimeSequence{
		Want:        Want{},
		Prime:       2,
		foundPrimes: make([]int, 0),
	}
	filter.Init(metadata, spec)

	filter.Prime = filter.GetIntParam("prime", 2)
	filter.WantType = "prime sequence"
	filter.ConnectivityMetadata = ConnectivityMetadata{
		RequiredInputs:  1,
		RequiredOutputs: 0,
		MaxInputs:       1,
		MaxOutputs:      -1,
		WantType:        "prime sequence",
		Description:     "Prime number sequence",
	}

	return filter
}

// Exec returns the generalized chain function for the filter
func (f *PrimeSequence) Exec() bool {
	achieved, _ := f.GetStateBool("achieved", false)
	if achieved {
		return true
	}
	f.StoreState("achieved", true)
	foundPrimesVal, exists := f.GetState("foundPrimes")
	foundPrimes := make([]int, 0)
	if exists {
		if fp, ok := foundPrimesVal.([]int); ok {
			foundPrimes = fp
		}
	}
	totalProcessedVal, _ := f.GetState("total_processed")
	totalProcessed := 0
	if totalProcessedVal != nil {
		if tp, ok := totalProcessedVal.(int); ok {
			totalProcessed = tp
		}
	}

	for {
		_, i, ok := f.ReceiveFromAnyInputChannel(100)
		if !ok {
			break
		}

		if val, ok := i.(int); ok {
			totalProcessed++
			isPrime := true

			// Special cases: 1 is not prime, 2 is prime
			if val < 2 {
				isPrime = false
			} else if val == 2 {
				isPrime = true
			} else {
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
				// if f.paths.GetOutCount() > 0 { f.SendPacketMulti(val) } Update live state immediately when prime is found
				// f.StoreStateMulti(map[string]interface{}{ "foundPrimes":    foundPrimes, "primeCount":     len(foundPrimes), "lastPrimeFound": val,
				// })
			}
			f.StoreStateMulti(map[string]interface{}{
				"total_processed":       totalProcessed,
				"last_number_processed": val,
			})
		}
	}
	f.StoreStateMulti(map[string]interface{}{
		"foundPrimes":    foundPrimes,
		"primeCount":     len(foundPrimes),
		"total_processed": totalProcessed,
	})

	return true
}

// // PrimeSink collects and displays results type PrimeSink struct { Want Received int
// paths    Paths }

// // NewPrimeSink creates a new prime sink want func NewPrimeSink(metadata Metadata, spec WantSpec) interface{} { sink := &PrimeSink{ Want:     Want{},
// Received: 0, }

// // Initialize base Want fields sink.Init(metadata, spec)

// // Set fields for base Want methods sink.WantType = "prime sink" sink.ConnectivityMetadata = ConnectivityMetadata{ RequiredInputs:  1,
// RequiredOutputs: 0, MaxInputs:       -1, MaxOutputs:      0, WantType:        "prime sink",
// Description:     "Prime sink/collector", }

// func (s *PrimeSink) GetWant() interface{} { return &s.Want }

// // Exec returns the generalized chain function for the sink func (s *PrimeSink) Exec() bool { // Validate input channel is available in, connectionAvailable := s.GetFirstInputChannel()
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
