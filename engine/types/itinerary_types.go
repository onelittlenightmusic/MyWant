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
}

func (o *ItineraryWant) IsAchieved() bool {
	achieved, _ := o.GetInternal("goal_achieved")
	return achieved != nil && achieved.(bool)
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

	var directions []string
	if raw, ok := o.GetState("directions"); ok {
		if slice, ok := raw.([]string); ok { directions = slice } else if slice, ok := raw.([]any); ok {
			for _, item := range slice { if s, ok := item.(string); ok { directions = append(directions, s) } }
		}
	}

	if len(directions) > 0 {
		o.SuggestParent(directions)
		o.SetInternal("planned_count", float64(len(directions)))
	}

	if hash, _ := o.GetStateString("_opa_input_hash", ""); hash != "" {
		if len(directions) == 0 {
			o.SetInternal("goal_achieved", true)
		} else {
			o.SetInternal("goal_achieved", false)
		}
	}
}

func (o *ItineraryWant) mergeCurrent(updates map[string]any) {
	if len(updates) == 0 { return }
	current := map[string]any{}
	if raw, ok := o.GetCurrent("current"); ok {
		if m, ok := raw.(map[string]any); ok { for k, v := range m { current[k] = v } }
	}
	
	changed := false
	for k, v := range updates {
		if !reflect.DeepEqual(current[k], v) { current[k] = v; changed = true }
	}
	
	if changed { o.SetCurrent("current", current) }
}
