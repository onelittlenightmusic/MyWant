package types

import (
	"context"
	"fmt"
	. "mywant/engine/core"
)

const budgetThinkerAgentName = "budget_thinker"

func init() {
	RegisterThinkAgent(budgetThinkerAgentName, budgetThinkerThink)
}

func budgetThinkerThink(ctx context.Context, want *Want) error {
	// ── Phase 0: Initialize Goal ──────────────────────────────────────────
	budget, ok := want.GetGoal("budget_limit")
	if !ok || budget == nil {
		legacyBudget, _ := want.GetStateFloat64("budget", 5000.0)
		want.SetGoal("budget_limit", legacyBudget)
		budget = legacyBudget
	}
	budgetVal := toFloat64(budget)

	cb := GetGlobalChainBuilder()
	if cb == nil { return nil }

	// ── Phase 1: Compute and propagate per-want target budgets ──────────────
	itineraryRaw, hasItinerary := want.GetParentState("itinerary")
	if hasItinerary {
		itinerary, ok := itineraryRaw.(map[string]any)
		if ok && len(itinerary) > 0 {
			activeItems := make(map[string]any)
			for wantName, entry := range itinerary {
				w, found := cb.FindWantByName(wantName)
				if found && (w.Status == WantStatusCancelled || w.Status == WantStatusFailed) {
					continue
				}
				activeItems[wantName] = entry
			}

			if len(activeItems) > 0 {
				perWant := budgetVal / float64(len(activeItems))
				targetBudgets := make(map[string]any, len(activeItems))
				for wantName, entry := range activeItems {
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
			}
		}
	}

	// ── Phase 2: Aggregate reported costs (Current State) ──────────────────
	costsRaw, hasCosts := want.GetParentState("costs")
	costs := map[string]any{}
	if hasCosts {
		if m, ok := costsRaw.(map[string]any); ok {
			for wantName, cost := range m {
				w, found := cb.FindWantByName(wantName)
				if found && (w.Status == WantStatusCancelled || w.Status == WantStatusFailed) {
					continue
				}
				costs[wantName] = cost
			}
		}
	}
	if len(costs) == 0 {
		return nil
	}

	var totalCost float64
	for _, v := range costs {
		totalCost += toFloat64(v)
	}

	remaining := budgetVal - totalCost
	exceeded := totalCost > budgetVal

	// Update GCP Current State (Domain-specific facts)
	want.SetCurrent("total_spent", totalCost)
	want.SetCurrent("remaining_budget", remaining)
	want.SetCurrent("budget_exceeded", exceeded)
	
	summary := fmt.Sprintf("Budget: %.2f, Spent: %.2f, Remaining: %.2f (%d costs reported)",
		budgetVal, totalCost, remaining, len(costs))
	want.SetCurrent("budget_summary", summary)
	want.SetCurrent("costs", costs)

	return nil
}
