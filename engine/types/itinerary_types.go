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
	if goal, ok := o.Spec.GetParam("goal"); ok && goal != nil {
		o.SetGoal("goal", goal)
	}
	if current, ok := o.Spec.GetParam("current"); ok && current != nil {
		o.SetCurrent("current", current)
	}
	// Promote direction_map param → state so Progress reads from GetCurrent
	o.SetCurrent("direction_map", o.GetStringParam("direction_map", "{}"))
	// Promote OPA config params → state so opa_llm_thinker can read them
	o.SetCurrent("opa_llm_planner_command", o.GetStringParam("opa_llm_planner_command", "opa-llm-planner"))
	o.SetCurrent("policy_dir", o.GetStringParam("policy_dir", ""))
	o.SetCurrent("use_llm", o.GetBoolParam("use_llm", true))
	o.SetCurrent("llm_provider", o.GetStringParam("llm_provider", "anthropic"))
}

func (o *ItineraryWant) IsAchieved() bool {
	return o.GetLocals().GoalAchieved
}

func (o *ItineraryWant) Progress() {
	locals := o.GetLocals()

	// Sync cost/sets fields from parent state into own current snapshot
	directionMapStr := GetCurrent(o, "direction_map", "{}")
	var directionMap map[string]DirectionConfig
	json.Unmarshal([]byte(directionMapStr), &directionMap)

	updates := make(map[string]any)
	for _, cfg := range directionMap {
		if cfg.CostField != "" {
			if val, ok := o.GetParentState(cfg.CostField); ok {
				updates[cfg.CostField] = val
			}
		}
		for k := range cfg.Sets {
			if val, ok := o.GetParentState(k); ok {
				updates[k] = val
			}
		}
	}
	if len(updates) > 0 {
		o.mergeCurrent(updates)
	}

	// Interpret directions and propose dispatch to parent Target
	InterpretDirections(&o.Want)

	// Update goal_achieved
	if GetCurrent(o, "opa_input_hash", "") != "" {
		directions := GetPlan(o, "directions", []string{})
		achieved := len(directions) == 0
		locals.GoalAchieved = achieved
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
