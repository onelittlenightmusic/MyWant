package types

import (
	"fmt"

	. "mywant/engine/src"
)

func init() {
	RegisterWantImplementation[FibonacciNumbers, FibonacciNumbersLocals]("fibonacci numbers")
	RegisterWantImplementation[FibonacciFilter, FibonacciFilterLocals]("fibonacci filter")
}

// FibonacciNumbersLocals holds type-specific local state for FibonacciNumbers
type FibonacciNumbersLocals struct{}

// FibonacciNumbers generates fibonacci sequence numbers
type FibonacciNumbers struct {
	Want
}

func (g *FibonacciNumbers) GetLocals() *FibonacciNumbersLocals {
	return GetLocals[FibonacciNumbersLocals](&g.Want)
}

// Initialize resets state before execution begins
func (g *FibonacciNumbers) Initialize() {
	// No state reset needed for fibonacci wants
}

// IsAchieved checks if fibonacci generation is complete
func (g *FibonacciNumbers) IsAchieved() bool {
	sentCount, _ := g.GetStateInt("sent_count", 0)
	count := g.GetIntParam("count", 20)
	return sentCount >= count
}

// Progress returns the generalized chain function for the numbers generator
func (g *FibonacciNumbers) Progress() {
	count := g.GetIntParam("count", 20)
	a, _ := g.GetStateInt("a", 0)
	b, _ := g.GetStateInt("b", 1)
	sentCount, _ := g.GetStateInt("sent_count", 0)

	sentCount += 1
	g.Provide(a)

	// Calculate achieving percentage
	achievingPercentage := int(float64(sentCount) * 100 / float64(count))

	g.StoreStateMulti(Dict{
		"a":                    b,
		"b":                    a + b,
		"sent_count":           sentCount,
		"achieving_percentage": achievingPercentage,
	})

	if sentCount >= count {
		// Send end signal
		g.ProvideDone()
		g.StoreStateMulti(Dict{
			"final_result":         fmt.Sprintf("Generated %d fibonacci numbers", count),
			"achieving_percentage": 100,
			"achieved":             true,
			"completed":            true, // Explicitly set completed to true
		})
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
	return GetLocals[FibonacciFilterLocals](&f.Want)
}

// Initialize resets state before execution begins
func (f *FibonacciFilter) Initialize() {
	// Get or initialize locals
	locals := f.GetLocals()
	if locals == nil {
		locals = &FibonacciFilterLocals{}
		f.Locals = locals
	}
	if locals.filtered == nil {
		locals.filtered = make([]int, 0)
	}
}

// IsAchieved checks if fibonacci filtering is complete
func (f *FibonacciFilter) IsAchieved() bool {
	achieved, _ := f.GetStateBool("achieved", false)
	return achieved
}

// Progress returns the generalized chain function for the filter
// Processes one packet per call and returns false to yield control
// Returns true only when end signal (-1) is received
func (f *FibonacciFilter) Progress() {
	locals := f.GetLocals()
	if locals == nil {
		f.StoreLog("ERROR: Failed to access FibonacciFilterLocals from Want.Locals")
		return
	}

	totalProcessed, _ := f.GetStateInt("total_processed", 0)

	// Restore filtered array from persistent state if it exists
	filteredVal, _ := f.GetState("filtered")
	if filteredVal != nil {
		if flt, ok := filteredVal.([]int); ok {
			locals.filtered = flt
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
			"filtered":             locals.filtered,
			"count":                len(locals.filtered),
			"total_processed":      totalProcessed,
			"achieved":             true,
			"achieving_percentage": 100,
			"final_result":         fmt.Sprintf("Filtered %d fibonacci numbers (min: %d, max: %d)", len(locals.filtered), f.GetIntParam("min_value", 0), f.GetIntParam("max_value", 1000000)),
		})
		return
	}

	if val, ok := i.(int); ok {
		totalProcessed++
		// Filter based on min/max values from parameters
		minValue := f.GetIntParam("min_value", 0)
		maxValue := f.GetIntParam("max_value", 1000000)
		if val >= minValue && val <= maxValue {
			locals.filtered = append(locals.filtered, val)
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
			"filtered":              locals.filtered,
			"count":                 len(locals.filtered),
			"last_number_processed": val,
			"achieving_percentage":  achievingPercentage,
		})
	}

	// Yield control - will be called again for next packet
}
