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

// budgetThinkerThink runs in two phases on each tick:
func budgetThinkerThink(ctx context.Context, want *Want) error {
	budget, _ := want.GetStateFloat64("budget", 5000.0)
	cb := GetGlobalChainBuilder()
	if cb == nil { return nil }

	// ── Phase 1: Compute and propagate per-want target budgets ──────────────
	itineraryRaw, hasItinerary := want.GetParentState("itinerary")
	if hasItinerary {
		itinerary, ok := itineraryRaw.(map[string]any)
		if ok && len(itinerary) > 0 {
			// Filter to only include active (non-cancelled) wants
			activeItems := make(map[string]any)
			for wantName, entry := range itinerary {
				// Find the actual want to check its status
				w, found := cb.FindWantByName(wantName)
				if found && (w.Status == WantStatusCancelled || w.Status == WantStatusFailed) {
					continue // Skip inactive wants
				}
				activeItems[wantName] = entry
			}

			if len(activeItems) > 0 {
				perWant := budget / float64(len(activeItems))
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
				want.StoreLog("[BudgetThinker] Allocated %.2f per want across %d active itinerary items (out of %d total)", 
					perWant, len(activeItems), len(itinerary))
			}
		}
	}

	// ── Phase 2: Aggregate reported costs ───────────────────────────────────
	costsRaw, hasCosts := want.GetParentState("costs")
	costs := map[string]any{}
	if hasCosts {
		if m, ok := costsRaw.(map[string]any); ok {
			// Filter costs: skip entries from cancelled wants
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
