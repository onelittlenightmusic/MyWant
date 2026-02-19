package types

import (
	"fmt"
	. "mywant/engine/core"
)

func init() {
	RegisterWantImplementation[BudgetWant, BudgetWantLocals]("budget")
}

// BudgetWantLocals holds type-specific local state for BudgetWant
type BudgetWantLocals struct{}

// BudgetWant reads costs aggregated in the parent (Target) State via MergeParentState
// and computes total/remaining budget.
type BudgetWant struct {
	Want
}

// Initialize sets up initial budget state
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

// aggregate reads costs from the parent Target's State and updates own state.
// Returns (costs map, allReported bool).
func (b *BudgetWant) aggregate() (map[string]any, bool) {
	budget, _ := b.GetStateFloat64("budget", 5000.0)

	// Read costs written by child agents via MergeParentState → Target.MergeState
	costsRaw, hasCosts := b.GetParentState("costs")
	costs := map[string]any{}
	if hasCosts {
		if m, ok := costsRaw.(map[string]any); ok {
			costs = m
		}
	}

	var totalCost float64
	for _, v := range costs {
		switch c := v.(type) {
		case float64:
			totalCost += c
		case int:
			totalCost += float64(c)
		}
	}

	remaining := budget - totalCost
	exceeded := totalCost > budget

	b.StoreStateMulti(Dict{
		"costs":            costs,
		"total_cost":       totalCost,
		"remaining_budget": remaining,
		"budget_exceeded":  exceeded,
		"budget_summary": fmt.Sprintf("Budget: %.2f, Spent: %.2f, Remaining: %.2f (%d costs reported)",
			budget, totalCost, remaining, len(costs)),
	})

	// allReported: at least one cost entry and budget not exceeded
	return costs, len(costs) > 0 && !exceeded
}

// Progress reads costs from parent State and updates budget summary
func (b *BudgetWant) Progress() {
	b.aggregate()
}

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
