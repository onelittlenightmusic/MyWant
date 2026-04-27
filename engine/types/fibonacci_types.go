package types

import (
	. "mywant/engine/core"
)

func init() {
	RegisterWithInit(func() {
		RegisterWantImplementation[FibonacciNumbers, FibonacciNumbersLocals]("fibonacci numbers")
		RegisterWantImplementation[FibonacciFilter, FibonacciFilterLocals]("fibonacci filter")
	})
}

// FibonacciNumbersLocals holds type-specific local state for FibonacciNumbers
type FibonacciNumbersLocals struct{}

// FibonacciNumbers generates fibonacci sequence numbers
type FibonacciNumbers struct {
	Want
}

func (g *FibonacciNumbers) GetLocals() *FibonacciNumbersLocals {
	return CheckLocalsInitialized[FibonacciNumbersLocals](&g.Want)
}

// Initialize resets state before execution begins
func (g *FibonacciNumbers) Initialize() {
	// Promote params → state so Progress/IsAchieved read from GetCurrent
	g.SetCurrent("count", g.GetIntParam("count", 20))
}

// IsAchieved checks if fibonacci generation is complete
func (g *FibonacciNumbers) IsAchieved() bool {
	sentCount := GetCurrent(g, "sent_count", 0)
	count := GetCurrent(g, "count", 20)
	return sentCount >= count
}

// Progress returns the generalized chain function for the numbers generator
func (g *FibonacciNumbers) Progress() {
	count := GetCurrent(g, "count", 20)
	a := GetCurrent(g, "a", 0)
	b := GetCurrent(g, "b", 1)
	sentCount := GetCurrent(g, "sent_count", 0)

	sentCount += 1
	out := NewDataObject("number_value")
	out.Set("value", a)
	g.Provide(out)

	// Calculate achieving percentage
	achievingPercentage := int(float64(sentCount) * 100 / float64(count))

	g.SetCurrent("a", b)
	g.SetCurrent("b", a+b)
	g.SetCurrent("sent_count", sentCount)
	g.SetCurrent("achieving_percentage", achievingPercentage)

	if sentCount >= count {
		// Send end signal
		g.ProvideDone()
		g.SetCurrent("achieving_percentage", 100)
		g.SetCurrent("achieved", true)
		g.SetCurrent("completed", true)
	}

}

// FibonacciFilterLocals holds type-specific local state for FibonacciFilter want
type FibonacciFilterLocals struct {
	filtered []int
}

// FibonacciFilter filters fibonacci numbers based on criteria
type FibonacciFilter struct {
	Want
}

func (f *FibonacciFilter) GetLocals() *FibonacciFilterLocals {
	return CheckLocalsInitialized[FibonacciFilterLocals](&f.Want)
}

// Initialize resets state before execution begins
func (f *FibonacciFilter) Initialize() {
	// Get locals (guaranteed to be initialized by framework)
	locals := f.GetLocals()
	if locals.filtered == nil {
		locals.filtered = make([]int, 0)
	}
	// Promote params → state so Progress reads from GetCurrent
	f.SetCurrent("min_value", f.GetIntParam("min_value", 0))
	f.SetCurrent("max_value", f.GetIntParam("max_value", 1000000))
}

// IsAchieved checks if fibonacci filtering is complete
func (f *FibonacciFilter) IsAchieved() bool {
	return GetCurrent(f, "achieved", false)
}

// Progress returns the generalized chain function for the filter
// Processes one packet per call and returns false to yield control
// Returns true only when end signal (-1) is received
func (f *FibonacciFilter) Progress() {
	locals := f.GetLocals()

	totalProcessed := GetCurrent(f, "total_processed", 0)

	// Restore filtered array from persistent state if it exists
	locals.filtered = GetCurrent(f, "filtered", []int{})

	// Try to receive one packet - wait forever until packet or DONE signal arrives
	_, obj, done, ok := f.UseForeverTyped("number_value")
	if !ok {
		// Channel closed or error
		return
	}

	// Check for end signal
	if done {
		// End signal received - finalize and complete
		f.SetCurrent("filtered", locals.filtered)
		f.SetCurrent("count", len(locals.filtered))
		f.SetCurrent("total_processed", totalProcessed)
		f.SetCurrent("achieved", true)
		f.SetCurrent("achieving_percentage", 100)
		return
	}

	val := GetTyped(obj, "value", 0)
	totalProcessed++
	// Filter based on min/max values from state (promoted in Initialize)
	minValue := GetCurrent(f, "min_value", 0)
	maxValue := GetCurrent(f, "max_value", 1000000)
	if val >= minValue && val <= maxValue {
		locals.filtered = append(locals.filtered, val)
	}

	// Calculate achieving percentage based on processed count
	// Since we don't know the total, use 50% while processing
	achievingPercentage := 50

	// Update state for this packet
	f.SetCurrent("total_processed", totalProcessed)
	f.SetCurrent("filtered", locals.filtered)
	f.SetCurrent("count", len(locals.filtered))
	f.SetCurrent("last_number_processed", val)
	f.SetCurrent("achieving_percentage", achievingPercentage)

	// Yield control - will be called again for next packet
}
