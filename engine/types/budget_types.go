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
// BudgetThinker is started automatically by the framework based on thinkCapabilities in budget.yaml.
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

// IsAchieved returns true when costs exist and budget is not exceeded.
// Reads already-aggregated state written by Progress() to avoid calling aggregate() twice.
func (b *BudgetWant) IsAchieved() bool {
	costsRaw, _ := b.GetState("costs")
	costs, _ := costsRaw.(map[string]any)
	exceeded, _ := b.GetState("budget_exceeded")
	budgetExceeded, _ := exceeded.(bool)
	return len(costs) > 0 && !budgetExceeded
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
