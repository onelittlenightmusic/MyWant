package types

import (
	"encoding/json"
	"fmt"
	"time"

	. "mywant/engine/core"
)

func init() {
	RegisterWantImplementation[ItineraryWant, ItineraryLocals]("itinerary")
}

// ItineraryLocals holds type-specific local state (none needed).
type ItineraryLocals struct{}

// ItineraryAction defines how to handle a specific OPA planner action.
type ItineraryAction struct {
	// Type is the want type to create (e.g. "hotel", "restaurant")
	Type string `json:"type"`
	// Params are passed to the created want's spec.params
	Params map[string]any `json:"params,omitempty"`
	// Sets are applied to own "current" state when the want completes,
	// causing the OPA ThinkAgent to replan against the updated state.
	Sets map[string]any `json:"sets,omitempty"`
	// CostField, if set, reads the "cost" field from the completed want's state
	// and writes it into own "current" state under this key.
	// This allows OPA policies to reason about actual booking costs.
	CostField string `json:"cost_field,omitempty"`
	// CancelsAction, if set, looks up the previously completed want for that
	// action and signals it to self-cancel before the new want is dispatched.
	// The itinerary waits until the old want's status is WantStatusCancelled,
	// then dispatches the new (reduce) want.
	CancelsAction string `json:"cancels_action,omitempty"`
}

// ItineraryWant combines OPA planning with dynamic want dispatch.
//
// Flow:
//  1. The OPA LLM ThinkAgent runs (via opa_llm_planning capability) and writes
//     "actions" to this want's own state.
//  2. Progress() reads those actions and creates the corresponding travel wants.
//  3. When each travel want completes, Progress() updates own "current" state.
//  4. The ThinkAgent detects the state change and replans.
//  5. When the plan is empty, IsAchieved returns true.
type ItineraryWant struct {
	Want
}

func (o *ItineraryWant) GetLocals() *ItineraryLocals {
	return CheckLocalsInitialized[ItineraryLocals](&o.Want)
}

// Initialize copies goal/current from params to state.
// Always overwrites because initialValue: {} from YAML is a non-nil empty map,
// so checking for nil is not sufficient to detect "not yet set".
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

// Progress reads actions from own state (written by ThinkAgent) and
// dispatches the corresponding travel wants dynamically.
func (o *ItineraryWant) Progress() {
	// 1. Read actions written by OPA LLM ThinkAgent
	actionsRaw, ok := o.GetState("actions")
	if !ok || actionsRaw == nil {
		return // ThinkAgent hasn't run yet
	}
	actions := anyToStringSlice(actionsRaw)

	// 2. Empty actions → only consider goal achieved if ThinkAgent has run at least once.
	// _opa_input_hash is set by the ThinkAgent on its first successful run.
	// Without this guard, Progress() would see initialValue:[] and immediately set goal_achieved.
	if len(actions) == 0 {
		hash, _ := o.GetStateString("_opa_input_hash", "")
		if hash == "" {
			o.StoreLog("[ITINERARY] Waiting for OPA ThinkAgent first run...")
			return
		}
		if achieved, _ := o.GetStateBool("goal_achieved", false); !achieved {
			o.StoreState("goal_achieved", true)
			o.StoreLog("[ITINERARY] Goal achieved!")
		}
		return
	}

	// 3. Parse action_map param
	actionMapStr := o.GetStringParam("action_map", "{}")
	var actionMap map[string]ItineraryAction
	if err := json.Unmarshal([]byte(actionMapStr), &actionMap); err != nil {
		o.StoreLog("[ITINERARY] ERROR parsing action_map: %v", err)
		return
	}

	cb := GetGlobalChainBuilder()
	if cb == nil {
		return
	}

	// 4. Load in-flight action → wantID mapping.
	// Completed actions are stored as "done" (sentinel) rather than being deleted,
	// preventing re-dispatch across Progress cycles until the ThinkAgent replans.
	const doneMarker = "__done__"
	dispatched := itineraryLoadDispatched(&o.Want)

	// 4a. Clear "done" markers when OPA has produced a new plan.
	// This happens when _opa_input_hash differs from the hash we last saw.
	currentHash, _ := o.GetStateString("_opa_input_hash", "")
	lastSeenHash, _ := o.GetStateString("_dispatched_plan_hash", "")
	if currentHash != lastSeenHash && currentHash != "" {
		for action, wantID := range dispatched {
			if wantID == doneMarker {
				delete(dispatched, action)
			}
		}
		o.StoreState("_dispatched_plan_hash", currentHash)
	}

	// 5. Check completion of already-dispatched wants.
	// On completion, mark as "done" (not deleted) so subsequent Progress cycles
	// skip re-dispatch until the ThinkAgent produces a new plan.
	// Also save the completed want's ID so reduce actions can pass it to agents.
	completedIDs := itineraryLoadCompletedIDs(&o.Want)
	for action, wantID := range dispatched {
		if wantID == doneMarker {
			continue // Already completed and marked
		}
		for _, w := range cb.GetWants() {
			if w.Metadata.ID != wantID {
				continue
			}
			completed, _ := w.GetStateBool("completed", false)
			if !completed {
				break
			}
			// Apply completion state updates so the ThinkAgent can replan
			if cfg, exists := actionMap[action]; exists {
				o.mergeCurrent(cfg.Sets)
				// If cost_field is configured, read the actual cost from the completed
				// want's state and store it in own "current" so OPA can see it.
				if cfg.CostField != "" {
					if costRaw, ok := w.GetState("cost"); ok {
						if cost := toFloat64(costRaw); cost > 0 {
							o.mergeCurrent(map[string]any{cfg.CostField: cost})
							o.StoreLog("[ITINERARY] Action '%s' cost %.2f → current.%s", action, cost, cfg.CostField)
						}
					}
				}
			}
			completedIDs[action] = wantID // Remember ID for potential future cancel
			dispatched[action] = doneMarker // Mark as done, not deleted
			o.StoreLog("[ITINERARY] Action '%s' completed (want: %s)", action, wantID)
			break
		}
	}
	itinerarySaveCompletedIDs(&o.Want, completedIDs)

	// 6. Dispatch new wants for actions not yet in-flight (or already done).
	for _, action := range actions {
		if v, already := dispatched[action]; already && v != doneMarker {
			continue // In-flight
		}
		if dispatched[action] == doneMarker {
			continue // Completed — wait for ThinkAgent to replan
		}
		cfg, exists := actionMap[action]
		if !exists {
			o.StoreLog("[ITINERARY] WARNING: no mapping for action '%s'", action)
			continue
		}
		// Copy params without mutating action_map config.
		params := make(map[string]any, len(cfg.Params))
		for k, v := range cfg.Params {
			params[k] = v
		}
		// inheritedSeries/inheritedVersion are set when this action replaces a cancelled want.
		// Non-replacement wants leave them zero, and addWant() will assign fresh values.
		var inheritedSeries string
		var inheritedVersion int
		// If this action cancels a previous one, ensure the old want self-cancels first.
		if cfg.CancelsAction != "" {
			if oldWantID, ok := completedIDs[cfg.CancelsAction]; ok && oldWantID != "" {
				cancelKey := "_cancel_pending_" + action
				cancelPendingID, _ := o.GetStateString(cancelKey, "")

				if cancelPendingID == "" {
					// First time: write _cancel_requested to old want's state and restart it.
					for _, w := range cb.GetWants() {
						if w.Metadata.ID == oldWantID {
							w.StoreState("_cancel_requested", true)
							break
						}
					}
					if err := cb.RestartWant(oldWantID); err != nil {
						o.StoreLog("[ITINERARY] ERROR triggering cancel for '%s': %v", oldWantID, err)
					} else {
						o.StoreState(cancelKey, oldWantID)
						o.StoreLog("[ITINERARY] Requested self-cancel for action '%s' (want: %s)", action, oldWantID)
					}
					itinerarySaveDispatched(&o.Want, dispatched)
					itinerarySaveCompletedIDs(&o.Want, completedIDs)
					continue // Skip dispatch this tick; check cancel status next tick
				}

				// Cancel is in progress: check if old want has self-cancelled.
				oldCancelled := false
				for _, w := range cb.GetWants() {
					if w.Metadata.ID == cancelPendingID {
						if w.GetStatus() == WantStatusCancelled {
							oldCancelled = true
						}
						break
					}
				}
				if !oldCancelled {
					o.StoreLog("[ITINERARY] Waiting for want '%s' to self-cancel (action: %s)...", cancelPendingID, action)
					itinerarySaveDispatched(&o.Want, dispatched)
					itinerarySaveCompletedIDs(&o.Want, completedIDs)
					continue // Still waiting; skip dispatch
				}

				// Old want has self-cancelled — clear the pending marker and fall through to dispatch.
				o.StoreState(cancelKey, "")
				o.StoreLog("[ITINERARY] Old want '%s' cancelled; dispatching new want for action '%s'", cancelPendingID, action)
				// Inherit series and bump version so the new want is visibly linked to its predecessor.
				for _, w := range cb.GetWants() {
					if w.Metadata.ID == cancelPendingID {
						inheritedSeries = w.Metadata.Series
						inheritedVersion = w.Metadata.Version
						break
					}
				}
			}
		}
		wantID := fmt.Sprintf("itinerary-%s-%d", action, time.Now().UnixNano())
		// Use the itinerary's own controller (target) as the owner of dispatched wants,
		// so they appear as siblings under the same target in the UI.
		// Fall back to the itinerary itself if no parent controller exists.
		ownerRef := OwnerReference{
			APIVersion:         "mywant/v1",
			Kind:               "Want",
			Name:               o.Metadata.Name,
			ID:                 o.Metadata.ID,
			Controller:         true,
			BlockOwnerDeletion: true,
		}
		for _, ref := range o.Metadata.OwnerReferences {
			if ref.Controller && ref.Kind == "Want" {
				ownerRef = ref
				break
			}
		}
		ownerInstanceID := fmt.Sprintf("%s-%s", ownerRef.Name, ownerRef.ID)
		newWant := &Want{
			Metadata: Metadata{
				ID:   wantID,
				Name: fmt.Sprintf("%s-%s", action, o.Metadata.Name),
				Type: cfg.Type,
				Labels: map[string]string{
					"itinerary":  o.Metadata.ID,
					"action":     action,
					"owner":      "child",
					"owner-name": ownerInstanceID,
				},
				OwnerReferences: []OwnerReference{ownerRef},
				// Replacement wants inherit Series and get Version+1; fresh wants get zero
				// values here and addWant() assigns a new Series with Version=1.
				Series:  inheritedSeries,
				Version: inheritedVersion + 1,
			},
			Spec: WantSpec{Params: params},
		}
		if err := cb.QueueWantAdd([]*Want{newWant}); err != nil {
			o.StoreLog("[ITINERARY] ERROR queuing want for '%s': %v", action, err)
			continue
		}
		dispatched[action] = wantID
		o.StoreLog("[ITINERARY] Dispatched '%s' (type: %s) for action '%s'", wantID, cfg.Type, action)
	}

	itinerarySaveDispatched(&o.Want, dispatched)
	o.StoreState("planned_count", len(actions))
	inFlight := 0
	for _, v := range dispatched {
		if v != doneMarker {
			inFlight++
		}
	}
	o.StoreState("dispatched_count", inFlight)
}

// mergeCurrent merges sets into own "current" state so ThinkAgent replans.
func (o *ItineraryWant) mergeCurrent(sets map[string]any) {
	if len(sets) == 0 {
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
	for k, v := range sets {
		current[k] = v
	}
	o.StoreState("current", current)
	o.StoreLog("[ITINERARY] Updated current: %v", sets)
}

// anyToStringSlice converts []any or []string to []string.
func anyToStringSlice(v any) []string {
	switch val := v.(type) {
	case []string:
		return val
	case []any:
		out := make([]string, 0, len(val))
		for _, item := range val {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func itineraryLoadDispatched(w *Want) map[string]string {
	result := map[string]string{}
	raw, ok := w.GetState("dispatched_actions")
	if !ok || raw == nil {
		return result
	}
	switch v := raw.(type) {
	case map[string]string:
		return v
	case map[string]any:
		for k, val := range v {
			if s, ok2 := val.(string); ok2 {
				result[k] = s
			}
		}
	}
	return result
}

func itinerarySaveDispatched(w *Want, dispatched map[string]string) {
	asAny := make(map[string]any, len(dispatched))
	for k, v := range dispatched {
		asAny[k] = v
	}
	w.StoreState("dispatched_actions", asAny)
}

// itineraryLoadCompletedIDs loads the action→wantID map for already-completed actions.
// This allows reduce/cancel actions to locate the old want and pass its ID to agents.
func itineraryLoadCompletedIDs(w *Want) map[string]string {
	result := map[string]string{}
	raw, ok := w.GetState("_completed_want_ids")
	if !ok || raw == nil {
		return result
	}
	switch v := raw.(type) {
	case map[string]string:
		return v
	case map[string]any:
		for k, val := range v {
			if s, ok2 := val.(string); ok2 {
				result[k] = s
			}
		}
	}
	return result
}

func itinerarySaveCompletedIDs(w *Want, completedIDs map[string]string) {
	asAny := make(map[string]any, len(completedIDs))
	for k, v := range completedIDs {
		asAny[k] = v
	}
	w.StoreState("_completed_want_ids", asAny)
}
