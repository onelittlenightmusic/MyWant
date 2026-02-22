package types

import (
	"context"
	"fmt"
	. "mywant/engine/core"
	"time"
)

const budgetThinkerAgentName = "budget_thinker"

func init() {
	RegisterThinkAgent(budgetThinkerAgentName, budgetThinkerThink)
}

// budgetThinkerThink runs in two phases on each tick:
//
//  1. Target budget allocation: reads "itinerary" from the parent coordinator state,
//     divides total budget evenly, and writes "target_budgets" back to the parent so
//     each travel want's ConditionThinker can pick up its allocation.
//
//  2. Cost aggregation: reads "costs" from the parent coordinator state and updates
//     the budget want's own state (total_cost, remaining_budget, budget_summary).
func budgetThinkerThink(ctx context.Context, want *Want) error {
	budget, _ := want.GetStateFloat64("budget", 5000.0)

	// ── Phase 1: Compute and propagate per-want target budgets ──────────────
	itineraryRaw, hasItinerary := want.GetParentState("itinerary")
	if hasItinerary {
		itinerary, ok := itineraryRaw.(map[string]any)
		if ok && len(itinerary) > 0 {
			perWant := budget / float64(len(itinerary))
			targetBudgets := make(map[string]any, len(itinerary))
			for wantName, entry := range itinerary {
				wantType := ""
				if entryMap, ok := entry.(map[string]any); ok {
					wantType, _ = entryMap["want_type"].(string)
				}
				targetBudgets[wantName] = map[string]any{
					"want_type":     wantType,
					"want_name":     wantName,
					"target_budget": perWant,
				}
			}
			want.MergeParentState(map[string]any{"target_budgets": targetBudgets})
			want.StoreLog("[BudgetThinker] Allocated %.2f per want across %d itinerary items", perWant, len(itinerary))
		}
	}

	// ── Phase 2: Aggregate reported costs ───────────────────────────────────
	costsRaw, hasCosts := want.GetParentState("costs")
	costs := map[string]any{}
	if hasCosts {
		if m, ok := costsRaw.(map[string]any); ok {
			costs = m
		}
	}
	if len(costs) == 0 {
		return nil
	}

	var totalCost float64
	for _, v := range costs {
		totalCost += toFloat64(v)
	}

	remaining := budget - totalCost
	exceeded := totalCost > budget

	want.StoreStateMulti(Dict{
		"costs":            costs,
		"total_cost":       totalCost,
		"remaining_budget": remaining,
		"budget_exceeded":  exceeded,
		"budget_summary": fmt.Sprintf("Budget: %.2f, Spent: %.2f, Remaining: %.2f (%d costs reported)",
			budget, totalCost, remaining, len(costs)),
	})
	want.StoreLog("[BudgetThinker] Updated: total=%.2f, remaining=%.2f, %d costs", totalCost, remaining, len(costs))
	return nil
}

// startBudgetThinker starts the BudgetThinker BackgroundAgent for a budget want.
func startBudgetThinker(want *Want) {
	thinkerID := "budget-thinker-" + want.Metadata.ID
	if _, exists := want.GetBackgroundAgent(thinkerID); exists {
		return // Already running
	}

	reg := want.GetAgentRegistry()
	if reg == nil {
		return
	}

	agent, ok := reg.GetAgent(budgetThinkerAgentName)
	if !ok {
		return
	}

	thinkAgent, ok := agent.(*ThinkAgent)
	if !ok {
		return
	}

	tAgent := NewThinkingAgent(thinkerID, 2*time.Second, "BudgetThinker", thinkAgent.Think)
	if err := want.AddBackgroundAgent(tAgent); err != nil {
		want.StoreLog("[BudgetThinker] Failed to start: %v", err)
	}
}
