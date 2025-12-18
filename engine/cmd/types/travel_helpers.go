package types

import (
	. "mywant/engine/src"
)

// TravelProgressHelper provides common progress logic for all travel wants
// This reduces code duplication across RestaurantWant, HotelWant, and BuffetWant
type TravelProgressHelper struct {
	Want                 *Want
	TryAgentExecutionFn  func() any // Returns schedule or nil
	SetScheduleFn        func(schedule any)
	GenerateScheduleFn   func() *TravelSchedule
	ServiceType          string // "restaurant", "hotel", or "buffet"
}

// IsAchievedBase implements common IsAchieved logic for all travel wants
func (h *TravelProgressHelper) IsAchievedBase() bool {
	attempted, _ := h.Want.GetStateBool("attempted", false)
	return attempted
}

// CalculateAchievingPercentageBase returns progress percentage
func (h *TravelProgressHelper) CalculateAchievingPercentageBase() int {
	attempted, _ := h.Want.GetStateBool("attempted", false)
	if attempted {
		return 100
	}
	return 0
}

// ProgressBase implements common progress logic for all travel wants
// Specific implementations call this from their Progress() method
func (h *TravelProgressHelper) ProgressBase() {
	attempted, _ := h.Want.GetStateBool("attempted", false)
	_, connectionAvailable := h.Want.GetFirstOutputChannel()

	if attempted {
		return
	}
	h.Want.StoreState("attempted", true)

	// Try to use agent system if available - agent completely overrides normal execution
	if h.TryAgentExecutionFn != nil {
		if agentSchedule := h.TryAgentExecutionFn(); agentSchedule != nil {
			// Use the agent's schedule result
			if h.SetScheduleFn != nil {
				h.SetScheduleFn(agentSchedule)
			}
			if connectionAvailable {
				// Schedule will be sent via subsequent Provide() call in specific implementation
			}
			return
		}
	}

	// Generate and provide schedule if no agent execution
	if h.GenerateScheduleFn != nil {
		schedule := h.GenerateScheduleFn()
		if schedule != nil && connectionAvailable {
			h.Want.Provide(schedule)
		}
	}
}

// ExtractLocals is a helper to safely extract locals from a Want
// Returns the locals and an error message if the type assertion fails
func ExtractLocals[T any](want *Want) (T, bool) {
	var zero T
	if want.Locals == nil {
		return zero, false
	}
	locals, ok := want.Locals.(T)
	return locals, ok
}

// TravelLocalsBase provides common properties for all travel locals types
type TravelLocalsBase struct {
	Kind string
}

// GetKind returns the type of travel service
func (t TravelLocalsBase) GetKind() string {
	return t.Kind
}
