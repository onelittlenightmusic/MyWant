package types

import (
	"encoding/json"
	"reflect"

	. "mywant/engine/core"
)

func init() {
	RegisterWantImplementation[ItineraryWant, ItineraryLocals]("itinerary")
}

type ItineraryLocals struct{}

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
	o.CreateInternalMulti(map[string]any{
		"goal_achieved":        false,
		"planned_count":        0,
		"dispatched_count":     0,
		"dispatched_directions": map[string]any{},
		"_opa_input_hash":      "",
		"_last_suggested":      []string{},
		"costs":                map[string]any{},
	})
}

func (o *ItineraryWant) IsAchieved() bool {
	return GetInternal(o, "goal_achieved", false)
}

func (o *ItineraryWant) Progress() {
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
		lastSuggested := GetInternal(o, "_last_suggested", []string{})

		if !reflect.DeepEqual(lastSuggested, directions) {
			o.SuggestParent(directions)
			o.SetInternal("_last_suggested", directions)
			o.SetInternal("planned_count", float64(len(directions)))
		}
	}

	if hash := GetInternal(o, "_opa_input_hash", ""); hash != "" {
		achieved := len(directions) == 0
		o.SetInternal("goal_achieved", achieved)
		
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
