package types

import (
	"encoding/json"
	"reflect"

	. "mywant/engine/core"
)

func init() {
	RegisterWantImplementation[ItineraryWant, ItineraryLocals]("itinerary")
}

type ItineraryLocals struct {
	GoalAchieved         bool           `mywant:"internal,goal_achieved"`
	PlannedCount         int            `mywant:"internal,planned_count"`
	DispatchedCount      int            `mywant:"internal,dispatched_count"`
	DispatchedDirections map[string]any `mywant:"internal,dispatched_directions"`
	LastSuggested        []string       `mywant:"internal,_last_suggested"`
	// OpaInputHash (_opa_input_hash) and Costs (costs) are intentionally excluded:
	// they are written by external agents/wants (OPA thinker, BudgetWant via
	// MergeParentState). Including them here would cause SyncLocalsState(false)
	// to overwrite externally-set values with stale data every cycle.
}

type ItineraryWant struct {
	Want
}

func (o *ItineraryWant) GetLocals() *ItineraryLocals {
	return CheckLocalsInitialized[ItineraryLocals](&o.Want)
}

func (o *ItineraryWant) Initialize() {
	if goal, ok := o.Spec.Params["goal"]; ok && goal != nil {
		o.SetGoal("goal", goal)
	}
	if current, ok := o.Spec.Params["current"]; ok && current != nil {
		o.SetCurrent("current", current)
	}
}

func (o *ItineraryWant) IsAchieved() bool {
	return o.GetLocals().GoalAchieved
}

func (o *ItineraryWant) Progress() {
	locals := o.GetLocals()
	directionMapStr := o.GetStringParam("direction_map", "{}")
	var directionMap map[string]DirectionConfig
	json.Unmarshal([]byte(directionMapStr), &directionMap)

	updates := make(map[string]any)
	for _, cfg := range directionMap {
		if cfg.CostField != "" {
			if val, ok := o.GetParentState(cfg.CostField); ok { updates[cfg.CostField] = val }
		}
		for k := range cfg.Sets {
			if val, ok := o.GetParentState(k); ok { updates[k] = val }
		}
	}

	if len(updates) > 0 { o.mergeCurrent(updates) }

	directions := GetPlan(o, "directions", []string{})

	if len(directions) > 0 {
		if !reflect.DeepEqual(locals.LastSuggested, directions) {
			o.SuggestParent(directions)
			locals.LastSuggested = directions
			locals.PlannedCount = len(directions)
		}
	}

	if GetCurrent(o, "opa_input_hash", "") != "" {
		achieved := len(directions) == 0
		locals.GoalAchieved = achieved
		
		// Propagation: inform parent that this planner has finished its goal
		o.MergeParentState(map[string]any{"goal_achieved": achieved})
	}
}

func (o *ItineraryWant) mergeCurrent(updates map[string]any) {
	if len(updates) == 0 { return }
	current := GetCurrent(o, "current", map[string]any{})
	
	changed := false
	for k, v := range updates {
		if !reflect.DeepEqual(current[k], v) { current[k] = v; changed = true }
	}
	
	if changed { o.SetCurrent("current", current) }
}
