package types

import (
	"encoding/json"
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
//  1. The OPA LLM ThinkAgent runs locally and writes "directions" to this want's state.
//  2. Progress() syncs these "directions" to the parent Target's state.
//  3. The parent's DispatchThinkerAgent realizes these directions into sibling Wants.
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

// IsAchieved returns true when the OPA planner reports no more directions.
func (o *ItineraryWant) IsAchieved() bool {
	achieved, _ := o.GetStateBool("goal_achieved", false)
	return achieved
}

// Progress orchestrates the planning-to-parent sync.
func (o *ItineraryWant) Progress() {
	// 1. Collect aggregated results from parent Target
	// The parent's DispatchThinker flattens costs and sets into parent's state.
	// We read the fields we care about based on our direction_map.
	directionMapStr := o.GetStringParam("direction_map", "{}")
	var directionMap map[string]DirectionConfig
	json.Unmarshal([]byte(directionMapStr), &directionMap)

	updates := make(map[string]any)
	for _, cfg := range directionMap {
		// Sync CostField if defined
		if cfg.CostField != "" {
			if val, ok := o.GetParentState(cfg.CostField); ok {
				updates[cfg.CostField] = val
			}
		}
		// Sync Sets flags
		for k := range cfg.Sets {
			if val, ok := o.GetParentState(k); ok {
				updates[k] = val
			}
		}
	}
	
	if len(updates) > 0 {
		o.mergeCurrent(updates)
	}

	// 2. Read planned directions written by our own OPA LLM ThinkAgent
	var directions []string
	if raw, ok := o.GetState("directions"); ok {
		if slice, ok := raw.([]string); ok {
			directions = slice
		} else if slice, ok := raw.([]any); ok {
			for _, item := range slice {
				if s, ok := item.(string); ok {
					directions = append(directions, s)
				}
			}
		}
	}

	// 3. Propagate planned directions to parent (Target)
	// The Target's DispatchThinkerAgent will realize these directions into actual Wants.
	if len(directions) > 0 {
		o.SuggestParent(directions)
		// Also record planned count for achievement check
		o.StoreState("planned_count", len(directions))
	}

	// 4. Update goal achievement status
	// If ThinkAgent has run (hash exists) and no more directions are planned, we are done.
	if hash, _ := o.GetStateString("_opa_input_hash", ""); hash != "" {
		if len(directions) == 0 {
			if achieved, _ := o.GetStateBool("goal_achieved", false); !achieved {
				o.StoreState("goal_achieved", true)
				o.StoreLog("[ITINERARY] All goals achieved (no further directions planned)")
			}
		} else {
			// Reset if new directions appear (e.g. after a manual change)
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
