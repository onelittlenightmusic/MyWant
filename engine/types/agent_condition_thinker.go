package types

import (
	"context"
	. "mywant/engine/core"
)

const conditionThinkerAgentName = "condition_thinker"

func init() {
	RegisterThinkAgent(conditionThinkerAgentName, conditionThinkerThink)
}

// conditionThinkerThink runs in three phases on each tick:
//
//  1. Itinerary registration (one-shot): adds own {want_type, want_name} entry to the
//     parent coordinator's "itinerary" map so BudgetThinker can allocate per-want budgets.
//     If no parent exists, sets good_to_reserve=true immediately.
//
//  2. Target budget check: once BudgetThinker has written "target_budgets" to the
//     parent state, reads own allocation, stores it, and sets good_to_reserve=true.
//
//  3. Cost propagation (existing): after reservation completes, propagates the final
//     cost to the parent's "costs" map for BudgetThinker aggregation.
func conditionThinkerThink(ctx context.Context, want *Want) error {
	// ── Phase 1: Register in parent itinerary (only once) ──────────────────
	registered, _ := want.GetStateBool("_thinker_itinerary_registered", false)
	if !registered {
		if !want.HasParent() {
			// Standalone want (no coordinator) – approve immediately
			want.StoreState("good_to_reserve", true)
			want.StoreState("_thinker_good_to_reserve_set", true)
		} else {
			want.MergeParentState(map[string]any{
				"itinerary": map[string]any{
					want.Metadata.Name: map[string]any{
						"want_type": want.Metadata.Type,
						"want_name": want.Metadata.Name,
					},
				},
			})
			want.StoreLog("[ConditionThinker] Registered in itinerary: %s (%s)", want.Metadata.Name, want.Metadata.Type)
		}
		want.StoreState("_thinker_itinerary_registered", true)
	}

	// ── Phase 2: Check for own target budget allocation ────────────────────
	goodToReserveSet, _ := want.GetStateBool("_thinker_good_to_reserve_set", false)
	if !goodToReserveSet {
		targetBudgetsRaw, hasTB := want.GetParentState("target_budgets")
		if hasTB {
			if tb, found := extractTargetBudget(targetBudgetsRaw, want.Metadata.Name); found {
				want.StoreState("target_budget", tb)
				want.StoreState("good_to_reserve", true)
				want.StoreState("_thinker_good_to_reserve_set", true)
				want.StoreLog("[ConditionThinker] Target budget allocated: %.2f → good_to_reserve=true", tb)
			}
			// else: target_budgets exists but our entry not yet added — wait for next tick
		} else {
			// target_budgets key absent: parent may have no BudgetThinker (e.g. no budget want).
			// Count ticks since itinerary registration; self-approve after a short wait so
			// coordinators without budget tracking still allow agents to run.
			ticksWaited, _ := want.GetStateInt("_thinker_ticks_waited", 0)
			ticksWaited++
			want.StoreState("_thinker_ticks_waited", ticksWaited)
			if ticksWaited >= 3 {
				want.StoreState("good_to_reserve", true)
				want.StoreState("_thinker_good_to_reserve_set", true)
				want.StoreLog("[ConditionThinker] No budget coordination after %d ticks → good_to_reserve=true", ticksWaited)
			}
		}
	}

	// ── Phase 3: Cost propagation ──────────────────────────────────────────
	costRaw, exists := want.GetState("cost")
	if !exists || costRaw == nil {
		return nil
	}
	cost := toFloat64(costRaw)
	if cost == 0 {
		return nil
	}
	lastRaw, _ := want.GetState("_thinker_last_reported_cost")
	if last := toFloat64(lastRaw); last == cost {
		return nil
	}
	want.MergeParentState(map[string]any{
		"costs": map[string]any{want.Metadata.Name: cost},
	})
	want.StoreState("_thinker_last_reported_cost", cost)
	want.StoreLog("[ConditionThinker] Propagated cost %.2f to parent want", cost)
	return nil
}

// toFloat64 converts various numeric types to float64.
func toFloat64(v any) float64 {
	if v == nil {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case int32:
		return float64(n)
	case uint:
		return float64(n)
	case uint64:
		return float64(n)
	}
	return 0
}

// extractTargetBudget looks up the target_budget for wantName in the target_budgets map.
// target_budgets is structured as: map[wantName]map[string]any{"target_budget": float64, ...}
func extractTargetBudget(targetBudgetsRaw any, wantName string) (float64, bool) {
	tb, ok := targetBudgetsRaw.(map[string]any)
	if !ok {
		return 0, false
	}
	entry, ok := tb[wantName]
	if !ok {
		return 0, false
	}
	entryMap, ok := entry.(map[string]any)
	if !ok {
		return 0, false
	}
	budget := toFloat64(entryMap["target_budget"])
	return budget, budget > 0
}
