package types

import (
	"reflect"

	. "mywant/engine/core"
)

func init() {
	RegisterWantImplementation[ItineraryWant, ItineraryLocals]("itinerary")
}

// ItineraryLocals holds type-specific local state (none needed).
type ItineraryLocals struct{}

// ItineraryWant implements the core travel planning logic using OPA and LLM.
//
// New Simplified Design:
//  1. The OPA LLM ThinkAgent runs locally and writes "actions" to this want's state.
//  2. Progress() syncs these "actions" to the parent Target's state.
//  3. The parent's DispatchThinkerAgent realizes these actions into sibling Wants.
//  4. Progress() collects aggregated costs from the parent to feed back into planning.
type ItineraryWant struct {
	Want
}

func (o *ItineraryWant) GetLocals() *ItineraryLocals {
	return CheckLocalsInitialized[ItineraryLocals](&o.Want)
}

// Initialize copies goal/current from params to state.
func (o *ItineraryWant) Initialize() {
	if goal, ok := o.Spec.Params["goal"]; ok && goal != nil {
		o.StoreState("goal", goal)
	}
	if current, ok := o.Spec.Params["current"]; ok && current != nil {
		o.StoreState("current", current)
	}
}

// IsAchieved returns true when the OPA planner reports no more actions.
func (o *ItineraryWant) IsAchieved() bool {
	achieved, _ := o.GetStateBool("goal_achieved", false)
	return achieved
}

// Progress orchestrates the planning-to-parent sync.
func (o *ItineraryWant) Progress() {
	// 1. Collect aggregated costs and sets from parent Target
	// The parent's DispatchThinker flattens these into parent's state for us.
	if rawSets, ok := o.GetParentState("sets"); ok {
		if sets, ok := rawSets.(map[string]any); ok {
			o.mergeCurrent(sets)
		}
	}
	if rawCosts, ok := o.GetParentState("costs"); ok {
		if costs, ok := rawCosts.(map[string]any); ok {
			o.mergeCurrent(costs)
		}
	}

	// 2. Read planned actions written by our own OPA LLM ThinkAgent
	var actions []string
	if raw, ok := o.GetState("actions"); ok {
		if slice, ok := raw.([]string); ok {
			actions = slice
		} else if slice, ok := raw.([]any); ok {
			for _, item := range slice {
				if s, ok := item.(string); ok {
					actions = append(actions, s)
				}
			}
		}
	}

	// 3. Propagate planned actions to parent (Target)
	// The Target's DispatchThinkerAgent will realize these actions into actual Wants.
	if len(actions) > 0 {
		o.StoreParentState("actions", actions)
		// Also record planned count for achievement check
		o.StoreState("planned_count", len(actions))
	}

	// 4. Update goal achievement status
	// If ThinkAgent has run (hash exists) and no more actions are planned, we are done.
	if hash, _ := o.GetStateString("_opa_input_hash", ""); hash != "" {
		if len(actions) == 0 {
			if achieved, _ := o.GetStateBool("goal_achieved", false); !achieved {
				o.StoreState("goal_achieved", true)
				o.StoreLog("[ITINERARY] All goals achieved (no further actions planned)")
			}
		} else {
			// Reset if new actions appear (e.g. after a manual change)
			o.StoreState("goal_achieved", false)
		}
	}
}

// mergeCurrent merges updates into own "current" state so ThinkAgent replans.
func (o *ItineraryWant) mergeCurrent(updates map[string]any) {
	if len(updates) == 0 {
		return
	}
	current := map[string]any{}
	if raw, ok := o.GetState("current"); ok {
		if m, ok := raw.(map[string]any); ok {
			for k, v := range m {
				current[k] = v
			}
		}
	}
	
	// Apply updates
	changed := false
	for k, v := range updates {
		// Use reflect.DeepEqual to safely compare complex types like maps
		if !reflect.DeepEqual(current[k], v) {
			current[k] = v
			changed = true
		}
	}
	
	if changed {
		o.StoreState("current", current)
	}
}
