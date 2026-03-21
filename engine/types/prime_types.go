package types

import (
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
	return CheckLocalsInitialized[PrimeNumbersLocals](&g.Want)
}

// Initialize resets state before execution begins
func (g *PrimeNumbers) Initialize() {
	// Promote params → state so Progress/IsAchieved read from GetCurrent
	g.SetCurrent("start", g.GetIntParam("start", 1))
	g.SetCurrent("end", g.GetIntParam("end", 100))
}

// IsAchieved checks if prime number generation is complete
func (g *PrimeNumbers) IsAchieved() bool {
	start := GetCurrent(g, "start", 1)
	end := GetCurrent(g, "end", 100)
	currentNumber := GetCurrent(g, "current_number", start-1)
	return currentNumber >= end
}

// Progress returns the generalized chain function for the numbers generator
func (g *PrimeNumbers) Progress() {
	start := GetCurrent(g, "start", 1)
	end := GetCurrent(g, "end", 100)
	currentNumber := GetCurrent(g, "current_number", start-1) // Start from start-1 so first iteration sends start

	currentNumber += 1

	out := NewDataObject("number_value")
	out.Set("value", currentNumber)
	g.Provide(out)

	// Calculate achieving percentage
	totalCount := end - start + 1
	if totalCount <= 0 {
		totalCount = 1
	}
	currentProgress := currentNumber - start + 1
	currentProgress = max(currentProgress, 0)
	achievingPercentage := min(int(float64(currentProgress)*100/float64(totalCount)), 100)

	g.SetCurrent("current_number", currentNumber)
	g.SetCurrent("achieving_percentage", achievingPercentage)

	if currentNumber >= end {
		// Send end signal
		g.ProvideDone()
		g.SetCurrent("achieving_percentage", 100)
		g.SetCurrent("achieved", true)
		g.SetCurrent("completed", true)
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
	return CheckLocalsInitialized[PrimeSequenceLocals](&f.Want)
}

// Initialize resets state before execution begins
func (f *PrimeSequence) Initialize() {
	// Get locals (guaranteed to be initialized by framework)
	locals := f.GetLocals()
	if locals.foundPrimes == nil {
		locals.foundPrimes = make([]int, 0)
	}
}

// IsAchieved checks if prime sequence filtering is complete
func (f *PrimeSequence) IsAchieved() bool {
	return GetCurrent(f, "achieved", false)
}

// Progress returns the generalized chain function for the filter
// Processes one packet per call and returns false to yield control
// Returns true only when end signal (-1) is received
func (f *PrimeSequence) Progress() {
	locals := f.GetLocals()

	if GetCurrent(f, "achieved", false) {
		return
	}

	totalProcessed := GetCurrent(f, "total_processed", 0)

	// Restore foundPrimes from persistent state if it exists
	locals.foundPrimes = GetCurrent(f, "foundPrimes", []int{})

	// Try to receive one packet - wait forever until packet or DONE signal arrives
	_, obj, done, ok := f.UseForeverTyped("number_value")
	if !ok {
		// Channel closed or error
		return
	}

	// Check for end signal
	if done {
		// End signal received - finalize and complete
		f.SetCurrent("foundPrimes", locals.foundPrimes)
		f.SetCurrent("primeCount", len(locals.foundPrimes))
		f.SetCurrent("total_processed", totalProcessed)
		f.SetCurrent("achieved", true)
		f.SetCurrent("achieving_percentage", 100)
		f.ProvideDone()
		return
	}

	val := GetTyped(obj, "value", 0)
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

	// Update state for this packet
	f.SetCurrent("total_processed", totalProcessed)
	f.SetCurrent("last_number_processed", val)
	f.SetCurrent("foundPrimes", locals.foundPrimes)
	f.SetCurrent("primeCount", len(locals.foundPrimes))
	f.SetCurrent("achieving_percentage", achievingPercentage)

	// Yield control - will be called again for next packet
}
