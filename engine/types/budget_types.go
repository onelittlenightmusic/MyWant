package types

import (
	. "mywant/engine/core"
)

func init() {
	RegisterWantImplementation[BudgetWant, BudgetWantLocals]("budget")
}

// BudgetWantLocals holds type-specific local state for BudgetWant
type BudgetWantLocals struct{}

// BudgetWant tracks travel budget by aggregating costs reported by child wants.
// Aggregation is delegated to BudgetThinker (ThinkAgent) which reads costs from
// the parent coordinator state and updates own state every 2 seconds.
type BudgetWant struct {
	Want
}

// Initialize sets up initial budget state.
func (b *BudgetWant) Initialize() {
	budget := b.GetFloatParam("budget", 5000.0)
	currency := b.GetStringParam("currency", "USD")
	b.StoreStateMulti(Dict{
		"budget":               budget,
		"currency":             currency,
		"costs":                map[string]any{},
		"total_cost":           0.0,
		"remaining_budget":     budget,
		"budget_exceeded":      false,
		"achieving_percentage": 0,
	})
}

// Progress is a no-op: BudgetThinker handles all state updates asynchronously.
func (b *BudgetWant) Progress() {}

// IsAchieved returns true when all itinerary items have reported non-zero costs
// and the budget has not been exceeded.
//
// Waiting for all items prevents premature termination of BudgetThinker:
// if the budget want achieved as soon as any cost appeared, BudgetThinker would
// be stopped before processing later cost updates (e.g. a hotel booking that
// arrives after the restaurant has already reported its cost).
func (b *BudgetWant) IsAchieved() bool {
	costsRaw, _ := b.GetState("costs")
	costs, _ := costsRaw.(map[string]any)
	exceeded, _ := b.GetState("budget_exceeded")
	budgetExceeded, _ := exceeded.(bool)
	if len(costs) == 0 || budgetExceeded {
		return false
	}

	// If this budget want has a parent coordinator, wait until every item in the
	// coordinator's itinerary has reported a non-zero cost.  This ensures
	// BudgetThinker stays alive long enough to aggregate all final costs.
	itineraryRaw, hasItinerary := b.GetParentState("itinerary")
	if hasItinerary {
		if itinerary, ok := itineraryRaw.(map[string]any); ok && len(itinerary) > 0 {
			parentCostsRaw, _ := b.GetParentState("costs")
			parentCosts, _ := parentCostsRaw.(map[string]any)
			if len(parentCosts) < len(itinerary) {
				// Not all itinerary items have reported costs yet.
				return false
			}
		}
	}

	return true
}

// CalculateAchievingPercentage returns progress percentage
func (b *BudgetWant) CalculateAchievingPercentage() int {
	if b.IsAchieved() {
		return 100
	}
	costsRaw, _ := b.GetState("costs")
	if m, ok := costsRaw.(map[string]any); ok && len(m) > 0 {
		return 50
	}
	return 0
}
