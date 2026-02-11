package types

import (
	"fmt"

	. "mywant/engine/core"
)

func init() {
	RegisterWantImplementation[PrimeNumbers, PrimeNumbersLocals]("prime numbers")
	RegisterWantImplementation[PrimeSequence, PrimeSequenceLocals]("prime sequence")
}

// PrimeNumbersLocals holds type-specific local state for PrimeNumbers want
type PrimeNumbersLocals struct {
	Start int
	End   int
}

// PrimeNumbers creates numbers and sends them downstream
type PrimeNumbers struct {
	Want
}

func (g *PrimeNumbers) GetLocals() *PrimeNumbersLocals {
	return GetLocals[PrimeNumbersLocals](&g.Want)
}

// Initialize resets state before execution begins
func (g *PrimeNumbers) Initialize() {
	// No state reset needed for prime wants
}

// IsAchieved checks if prime number generation is complete
func (g *PrimeNumbers) IsAchieved() bool {
	start := g.GetIntParam("start", 1)
	end := g.GetIntParam("end", 100)
	currentNumber, _ := g.GetStateInt("current_number", start-1)
	return currentNumber >= end
}

// Progress returns the generalized chain function for the numbers generator
func (g *PrimeNumbers) Progress() {
	start := g.GetIntParam("start", 1)
	end := g.GetIntParam("end", 100)
	currentNumber, _ := g.GetStateInt("current_number", start-1) // Start from start-1 so first iteration sends start

	currentNumber += 1

	g.Provide(currentNumber)

	// Calculate achieving percentage
	totalCount := end - start + 1
	if totalCount <= 0 {
		totalCount = 1
	}
	currentProgress := currentNumber - start + 1
	if currentProgress < 0 {
		currentProgress = 0
	}
	achievingPercentage := int(float64(currentProgress) * 100 / float64(totalCount))
	if achievingPercentage > 100 {
		achievingPercentage = 100
	}

	g.StoreStateMulti(Dict{
		"current_number":       currentNumber,
		"achieving_percentage": achievingPercentage,
	})

	if currentNumber >= end {
		// Send end signal
		g.ProvideDone()
		g.StoreStateMulti(Dict{
			"final_result":         fmt.Sprintf("Generated %d numbers from %d to %d", end-start+1, start, end),
			"achieving_percentage": 100,
			"achieved":             true,
			"completed":            true, // Explicitly set completed to true
		})
	}
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

func (f *PrimeSequence) GetLocals() *PrimeSequenceLocals {
	return GetLocals[PrimeSequenceLocals](&f.Want)
}

// Initialize resets state before execution begins
func (f *PrimeSequence) Initialize() {
	// Get or initialize locals
	locals := f.GetLocals()
	if locals == nil {
		locals = &PrimeSequenceLocals{}
		f.Locals = locals
	}
	if locals.foundPrimes == nil {
		locals.foundPrimes = make([]int, 0)
	}
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
	locals := CheckLocalsInitialized[PrimeSequenceLocals](&f.Want)

	achieved, _ := f.GetStateBool("achieved", false)
	if achieved {
		return
	}

	totalProcessed, _ := f.GetStateInt("total_processed", 0)

	// Restore foundPrimes from persistent state if it exists
	foundPrimesVal, _ := f.GetState("foundPrimes")
	if foundPrimesVal != nil {
		// Handle both direct []int and interface{} from JSON deserialization
		switch v := foundPrimesVal.(type) {
		case []int:
			locals.foundPrimes = v
		case []interface{}:
			// Convert []interface{} to []int (from JSON deserialization)
			locals.foundPrimes = make([]int, 0, len(v))
			for _, item := range v {
				if num, ok := item.(float64); ok {
					locals.foundPrimes = append(locals.foundPrimes, int(num))
				} else if num, ok := item.(int); ok {
					locals.foundPrimes = append(locals.foundPrimes, num)
				}
			}
		}
	}

	// Try to receive one packet - wait forever until packet or DONE signal arrives
	_, i, done, ok := f.UseForever()
	if !ok {
		// Channel closed or error
		return
	}

	// Check for end signal
	if done {
		// End signal received - finalize and complete
		f.StoreStateMulti(Dict{
			"foundPrimes":          locals.foundPrimes,
			"primeCount":           len(locals.foundPrimes),
			"total_processed":      totalProcessed,
			"achieved":             true,
			"achieving_percentage": 100,
			"final_result":         fmt.Sprintf("Found %d prime numbers", len(locals.foundPrimes)),
		})
		f.ProvideDone()
		return
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

		// Calculate achieving percentage based on processed count
		// Since we don't know the total, use 50% while processing
		achievingPercentage := 50
		if totalProcessed > 0 {
			achievingPercentage = 50 // Partial progress for streaming without count
		}

		// Update state for this packet
		f.StoreStateMulti(Dict{
			"total_processed":       totalProcessed,
			"last_number_processed": val,
			"foundPrimes":           locals.foundPrimes,
			"primeCount":            len(locals.foundPrimes),
			"achieving_percentage":  achievingPercentage,
		})
	}

	// Yield control - will be called again for next packet
}
