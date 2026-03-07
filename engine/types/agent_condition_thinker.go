package types

import (
	"context"
	. "mywant/engine/core"
)

const conditionThinkerAgentName = "condition_thinker"

func init() {
	RegisterThinkAgent(conditionThinkerAgentName, conditionThinkerThink)
}

func conditionThinkerThink(ctx context.Context, want *Want) error {
	// Register internal fields on first tick (idempotent)
	want.CreateInternal("thinker.goal_initialized", false)
	want.CreateInternal("thinker.itinerary_done", false)
	want.CreateInternal("thinker.plan_set", false)
	want.CreateInternal("thinker.ticks_waited", 0)

	// ── Phase 1: Initialize Goal & Register in parent itinerary ──────────
	if !GetInternal(want, "thinker.goal_initialized", false) {
		// Initialize the standard goal for any reservation-based want
		want.SetGoal("reservation_status", "confirmed")
		want.SetInternal("thinker.goal_initialized", true)
	}

	if !GetInternal(want, "thinker.itinerary_done", false) {
		if !want.HasParent() {
			// Standalone want (no coordinator) – approve immediately
			want.SetPlan("execute_booking", true)
			want.SetPredefined("good_to_reserve", true)
			want.SetInternal("thinker.plan_set", true)
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
		want.SetInternal("thinker.itinerary_done", true)
	}

	// ── Phase 2: Planning (Budget check & Approve execution) ──────────────
	if !GetInternal(want, "thinker.plan_set", false) {
		targetBudgetsRaw := GetParentState(want, "target_budgets", map[string]any{})
		if len(targetBudgetsRaw) > 0 {
			if tb, found := extractTargetBudget(targetBudgetsRaw, want.Metadata.Name); found {
				want.SetCurrent("budget_limit", tb)
				want.SetPlan("execute_booking", true)
				want.SetPredefined("good_to_reserve", true)
				want.SetInternal("thinker.plan_set", true)
				want.StoreLog("[ConditionThinker] Target budget allocated: %.2f → plan.execute_booking=true", tb)
			}
		} else {
			ticksWaited := GetInternal(want, "thinker.ticks_waited", 0)
			ticksWaited++
			want.SetInternal("thinker.ticks_waited", ticksWaited)
			if ticksWaited >= 3 {
				want.SetPlan("execute_booking", true)
				want.SetPredefined("good_to_reserve", true)
				want.SetInternal("thinker.plan_set", true)
				want.StoreLog("[ConditionThinker] No budget coordination after %d ticks → plan.execute_booking=true", ticksWaited)
			}
		}
	}

	// ── Phase 3: Cost propagation (Current State -> Parent) ────────────────
	if GetInternal(want, "_cancelled", false) || want.Status == WantStatusCancelled {
		want.MergeParentState(map[string]any{
			"costs": map[string]any{want.Metadata.Name: 0.0},
		})
		return nil
	}

	// Try to get cost from GCP 'current.actual_cost'
	cost := GetCurrent(want, "actual_cost", 0.0)
	if cost == 0 {
		cost = GetState(want, "cost", 0.0)
		if cost != 0 {
			want.SetCurrent("actual_cost", cost)
		}
	}

	if cost == 0 {
		return nil
	}

	want.MergeParentState(map[string]any{
		"costs": map[string]any{want.Metadata.Name: cost},
	})
	return nil
}

// toFloat64 converts various numeric types to float64.
func toFloat64(v any) float64 {
	if v == nil { return 0 }
	switch n := v.(type) {
	case float64: return n
	case float32: return float64(n)
	case int: return float64(n)
	case int64: return float64(n)
	case int32: return float64(n)
	case uint: return float64(n)
	case uint64: return float64(n)
	}
	return 0
}

func extractTargetBudget(targetBudgetsRaw any, wantName string) (float64, bool) {
	tb, ok := targetBudgetsRaw.(map[string]any)
	if !ok { return 0, false }
	entry, ok := tb[wantName]
	if !ok { return 0, false }
	entryMap, ok := entry.(map[string]any)
	if !ok { return 0, false }
	budget := toFloat64(entryMap["target_budget"])
	return budget, budget > 0
}
