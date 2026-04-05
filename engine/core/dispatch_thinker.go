package mywant

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// DispatchThinkerName is the identifier for the dispatching think agent.
const DispatchThinkerName = "dispatch-thinker"

// DirectionConfig defines how a logical direction maps to a Want.
type DirectionConfig struct {
	Type             string              `json:"type"`
	Params           map[string]any      `json:"params"`
	Sets             map[string]any      `json:"sets"`
	CostField        string              `json:"cost_field"`
	CancelsDirection string              `json:"cancels_direction,omitempty"`
	Using            []map[string]string `json:"using,omitempty"`
}

// DirectionRequest represents a low-level request to create a new child want.
// (Keeping this for internal use by DispatchThinker if needed, or for backward compatibility)
type DirectionRequest struct {
	Direction   string         `json:"direction"`
	Type        string         `json:"type"`
	Name        string         `json:"name"`
	Params      map[string]any `json:"params"`
	Series      string         `json:"series,omitempty"`
	Version     int           `json:"version,omitempty"`
	RequesterID string         `json:"requester_id,omitempty"`
}

// DispatchRequest is a fully-resolved want spec proposed by a planner child want
// to its parent Target for idempotent dispatch.
// The child interprets directions + direction_map; the parent handles AddChildWant.
type DispatchRequest struct {
	Direction        string              `json:"direction"`
	RequesterID      string              `json:"requester_id"`
	Type             string              `json:"type"`
	Params           map[string]any      `json:"params,omitempty"`
	Sets             map[string]any      `json:"sets,omitempty"`
	CostField        string              `json:"cost_field,omitempty"`
	CancelsDirection string              `json:"cancels_direction,omitempty"`
	Using            []map[string]string `json:"using,omitempty"`
}

// InterpretDirections reads the want's "directions" plan state and "direction_map" param,
// converts each direction to a DispatchRequest, and calls ProposeDispatch to write the
// resolved list to the parent state as "desired_dispatch".
// Call this from Progress() in any planner want that uses opa_llm_planning.
func InterpretDirections(w *Want) {
	// Read directions directly from state (written by opa_llm_thinker via SetPlan).
	// Use getState instead of GetPlan to avoid StateLabels dependency.
	var directions []string
	if raw, ok := w.getState("directions"); ok {
		switch v := raw.(type) {
		case []string:
			directions = v
		case []any:
			for _, item := range v {
				if s, ok := item.(string); ok {
					directions = append(directions, s)
				}
			}
		}
	}

	directionMap := make(map[string]DirectionConfig)
	if rawMap, ok := w.Spec.Params["direction_map"]; ok {
		switch v := rawMap.(type) {
		case string:
			json.Unmarshal([]byte(v), &directionMap)
		case map[string]any:
			b, _ := json.Marshal(v)
			json.Unmarshal(b, &directionMap)
		}
	}

	if len(directionMap) == 0 {
		return
	}

	requests := make([]DispatchRequest, 0, len(directions))
	for _, dir := range directions {
		cfg, ok := directionMap[dir]
		if !ok {
			continue
		}
		requests = append(requests, DispatchRequest{
			Direction:        dir,
			RequesterID:      w.Metadata.ID,
			Type:             cfg.Type,
			Params:           cfg.Params,
			Sets:             cfg.Sets,
			CostField:        cfg.CostField,
			CancelsDirection: cfg.CancelsDirection,
			Using:            cfg.Using,
		})
	}
	w.ProposeDispatch(requests)
}

// InterpretDirectionsCoordinator is like InterpretDirections but writes desired_dispatch
// to the want's OWN state instead of the parent's state.
// Use this from coordinator wants (like GoalWant) that are themselves the dispatch target,
// not a planner child that reports up to a parent coordinator.
func InterpretDirectionsCoordinator(w *Want) {
	var directions []string
	if raw, ok := w.getState("directions"); ok {
		switch v := raw.(type) {
		case []string:
			directions = v
		case []any:
			for _, item := range v {
				if s, ok := item.(string); ok {
					directions = append(directions, s)
				}
			}
		}
	}

	// Read direction_map from state (direction_map_json) or Spec.Params["direction_map"].
	// GoalWant stores it in state to avoid concurrent map writes on Spec.Params.
	directionMap := make(map[string]DirectionConfig)
	if raw, ok := w.getState("direction_map_json"); ok {
		if s, ok := raw.(string); ok && s != "" {
			json.Unmarshal([]byte(s), &directionMap)
		}
	}
	if len(directionMap) == 0 {
		if rawMap, ok := w.Spec.Params["direction_map"]; ok {
			switch v := rawMap.(type) {
			case string:
				json.Unmarshal([]byte(v), &directionMap)
			case map[string]any:
				b, _ := json.Marshal(v)
				json.Unmarshal(b, &directionMap)
			}
		}
	}

	if len(directionMap) == 0 {
		return
	}

	requests := make([]DispatchRequest, 0, len(directions))
	for _, dir := range directions {
		cfg, ok := directionMap[dir]
		if !ok {
			continue
		}
		requests = append(requests, DispatchRequest{
			Direction:   dir,
			RequesterID: w.Metadata.ID,
			Type:        cfg.Type,
			Params:      cfg.Params,
			Sets:        cfg.Sets,
			CostField:   cfg.CostField,
			Using:       cfg.Using,
		})
	}
	// Write directly to own state so the DispatchThinker running on this coordinator can read it.
	if requests == nil {
		requests = []DispatchRequest{}
	}
	w.storeState("desired_dispatch", requests)
}

// matchesLabels returns true if all selector key=value pairs are present in labels.
func matchesLabels(labels, selector map[string]string) bool {
	for k, v := range selector {
		if labels[k] != v {
			return false
		}
	}
	return true
}

// NewDispatchThinker creates a ThinkingAgent that monitors the want's "desired_dispatch" state
// (written by child planner wants via ProposeDispatch) and idempotently creates/tracks child wants.
func NewDispatchThinker(id string) *ThinkingAgent {
	return NewThinkingAgent(id, 1*time.Second, DispatchThinkerName, func(ctx context.Context, w *Want) error {
		// 1. Sync provider_state_map unconditionally (before early return).
		if rawMap, ok := w.Spec.Params["provider_state_map"]; ok {
			providerStateMap := make(map[string]string)
			switch v := rawMap.(type) {
			case string:
				json.Unmarshal([]byte(v), &providerStateMap)
			case map[string]any:
				for k, val := range v {
					if s, ok := val.(string); ok {
						providerStateMap[k] = s
					}
				}
			}
			cb := GetGlobalChainBuilder()
			keys := make([]string, 0, len(providerStateMap))
			for _, stateKey := range providerStateMap {
				keys = append(keys, stateKey)
			}
			w.SetCurrent("provider_keys", keys)

			for childName, stateKey := range providerStateMap {
				if cur, ok := w.GetCurrent(stateKey); ok {
					if b, ok := cur.(bool); ok && b {
						continue
					}
				}
				for _, child := range cb.GetWants() {
					// Match by exact name OR prefixed name (owner_types.go forces prefix = parent want name)
					prefixedName := fmt.Sprintf("%s-%s", w.Metadata.Name, childName)
					nameMatch := child.Metadata.Name == childName || child.Metadata.Name == prefixedName
					if nameMatch && w.isOwnerOf(child) {
						if child.Status == WantStatusAchieved {
							w.SetCurrent(stateKey, true)
							w.StoreLog("[%s] Provider '%s' achieved → %s=true", DispatchThinkerName, childName, stateKey)
						}
						break
					}
				}
			}
		}

		// 2. Read desired_dispatch written by child planner want via ProposeDispatch.
		var desiredRequests []DispatchRequest
		if raw, ok := w.getState("desired_dispatch"); ok {
			switch v := raw.(type) {
			case []DispatchRequest:
				desiredRequests = v
			case []any:
				for _, item := range v {
					b, err := json.Marshal(item)
					if err != nil {
						continue
					}
					var req DispatchRequest
					if err := json.Unmarshal(b, &req); err == nil {
						desiredRequests = append(desiredRequests, req)
					}
				}
			}
		}

		if len(desiredRequests) == 0 {
			return nil
		}

		// 3. Load tracking state
		dispatched := make(map[string]string)
		if raw, ok := w.getState("_dispatched_directions"); ok {
			if m, ok := raw.(map[string]string); ok {
				dispatched = m
			} else if m, ok := raw.(map[string]any); ok {
				for k, v := range m {
					if s, ok := v.(string); ok {
						dispatched[k] = s
					}
				}
			}
		}

		completedIDs := make(map[string]string)
		if raw, ok := w.getState("_completed_direction_ids"); ok {
			if m, ok := raw.(map[string]string); ok {
				completedIDs = m
			} else if m, ok := raw.(map[string]any); ok {
				for k, v := range m {
					if s, ok := v.(string); ok {
						completedIDs[k] = s
					}
				}
			}
		}

		const doneMarker = "DONE"
		cb := GetGlobalChainBuilder()

		// Build a lookup map for quick access
		requestByDirection := make(map[string]DispatchRequest, len(desiredRequests))
		for _, req := range desiredRequests {
			requestByDirection[req.Direction] = req
		}

		// 3.5. Reconcile: recover tracking state from existing child wants (e.g. after server restart).
		for _, req := range desiredRequests {
			direction := req.Direction
			if v, already := dispatched[direction]; already && v != "" {
				continue
			}
			namespacedDir := fmt.Sprintf("%s:%s", w.Metadata.Name, direction)
			for _, child := range cb.GetWants() {
				if child.Metadata.Labels["direction"] == namespacedDir && w.isOwnerOf(child) {
					if child.Status == WantStatusAchieved {
						if len(req.Sets) > 0 {
							for k, v := range req.Sets {
								w.storeState(k, v) // bypass schema validation: Sets keys are user-defined
							}
						}
						completedIDs[direction] = child.Metadata.ID
						dispatched[direction] = doneMarker
						w.StoreLog("[%s] Reconciled completed direction '%s' (want: %s)", DispatchThinkerName, direction, child.Metadata.ID)
					} else {
						dispatched[direction] = child.Metadata.ID
						w.StoreLog("[%s] Reconciled in-progress direction '%s' (want: %s)", DispatchThinkerName, direction, child.Metadata.ID)
					}
					break
				}
			}
		}

		// 3.6. If a DONE direction's child want was deleted, reset for re-dispatch.
		for direction, wantID := range dispatched {
			if wantID != doneMarker {
				continue
			}
			completedID, hasCompleted := completedIDs[direction]
			if !hasCompleted || completedID == "" {
				continue
			}
			stillExists := false
			for _, child := range cb.GetWants() {
				if child.Metadata.ID == completedID {
					stillExists = true
					break
				}
			}
			if !stillExists {
				delete(dispatched, direction)
				delete(completedIDs, direction)
				if req, ok := requestByDirection[direction]; ok {
					for k, v := range req.Sets {
						if b, ok := v.(bool); ok && b {
							w.storeState(k, false) // bypass schema validation: Sets keys are user-defined
						}
					}
				}
				w.SetStatus(WantStatusReaching)
				w.StoreLog("[%s] Direction '%s' want was deleted — resetting for re-dispatch", DispatchThinkerName, direction)
			} else {
				for _, child := range cb.GetWants() {
					if child.Metadata.ID != completedID {
						continue
					}
					if child.progressable == nil {
						break
					}
					if !child.progressable.IsAchieved() {
						delete(dispatched, direction)
						delete(completedIDs, direction)
						if req, ok := requestByDirection[direction]; ok {
							for k, v := range req.Sets {
								if b, ok := v.(bool); ok && b {
									w.storeState(k, false) // bypass schema validation: Sets keys are user-defined
								}
							}
						}
						w.SetStatus(WantStatusReaching)
						w.StoreLog("[%s] Direction '%s' want is no longer achieved — resetting for re-dispatch", DispatchThinkerName, direction)
					}
					break
				}
			}
		}

		// 4. Resolve IDs and check completion
		for direction, wantID := range dispatched {
			if wantID == doneMarker {
				continue
			}

			namespacedDir := fmt.Sprintf("%s:%s", w.Metadata.Name, direction)
			if strings.HasPrefix(wantID, "pending-") {
				for _, child := range cb.GetWants() {
					if child.Metadata.Labels["direction"] == namespacedDir && w.isOwnerOf(child) {
						wantID = child.Metadata.ID
						dispatched[direction] = wantID
						w.StoreLog("[%s] Resolved pending direction '%s' to want '%s'", DispatchThinkerName, direction, wantID)
						break
					}
				}
			}

			for _, child := range cb.GetWants() {
				if child.Metadata.ID == wantID {
					if child.Status == WantStatusAchieved {
						completedIDs[direction] = wantID
						dispatched[direction] = doneMarker
						w.StoreLog("[%s] Direction '%s' completed", DispatchThinkerName, direction)

						if req, ok := requestByDirection[direction]; ok {
							if len(req.Sets) > 0 {
								sets := make(map[string]any)
								if raw, ok := w.getState("sets"); ok {
									if m, ok := raw.(map[string]any); ok {
										for k, v := range m {
											sets[k] = v
										}
									}
								}
								for k, v := range req.Sets {
									sets[k] = v
									w.storeState(k, v) // bypass schema validation: Sets keys are user-defined
								}
								w.storeState("sets", sets)
							}

							if req.CostField != "" {
								var cost float64
								var ok bool
								if rawCost, found := child.GetCurrent("actual_cost"); found {
									cost = ToFloat64(rawCost, 0)
									ok = cost > 0
								} else {
									cost, ok = child.GetStateFloat64("cost", 0)
								}
								if ok {
									w.storeState(req.CostField, cost)
									w.StoreLog("[%s] Propagated cost %.2f to %s", DispatchThinkerName, cost, req.CostField)
								}
							}
						}
					} else if child.Status == WantStatusCancelled || child.Status == WantStatusFailed {
						delete(dispatched, direction)
					}
					break
				}
			}
		}

		// 5. Process desired requests
		changed := false
		for _, req := range desiredRequests {
			direction := req.Direction
			if v, already := dispatched[direction]; already && v != "" {
				continue
			}

			// Handle Replacement Orchestration
			var inheritedSeries string
			var inheritedVersion int
			if req.CancelsDirection != "" {
				if oldWantID, ok := completedIDs[req.CancelsDirection]; ok && oldWantID != "" {
					cancelKey := "_cancel_pending_" + direction
					cancelPendingID, _ := w.GetStateString(cancelKey, "")

					if cancelPendingID == "" {
						for _, target := range cb.GetWants() {
							if target.Metadata.ID == oldWantID {
								target.storeState("_cancel_requested", true)
								cb.RestartWant(oldWantID)
								w.storeState(cancelKey, oldWantID)
								w.StoreLog("[%s] Requested cancel for old direction '%s' (want: %s)", DispatchThinkerName, req.CancelsDirection, oldWantID)
								break
							}
						}
						continue
					}

					oldCancelled := false
					for _, target := range cb.GetWants() {
						if target.Metadata.ID == cancelPendingID {
							if target.Status == WantStatusCancelled {
								oldCancelled = true
								inheritedSeries = target.Metadata.Series
								inheritedVersion = target.Metadata.Version
							} else if target.Status == WantStatusModuleError || target.Status == WantStatusFailed {
								inheritedSeries = target.Metadata.Series
								inheritedVersion = target.Metadata.Version
								target.SetStatus(WantStatusCancelled)
								oldCancelled = true
								w.StoreLog("[%s] Cancel for '%s' ended in %s; forced cancelled, proceeding with replacement", DispatchThinkerName, req.CancelsDirection, target.Status)
							}
							break
						}
					}
					if !oldCancelled {
						continue
					}
					w.storeState(cancelKey, "")
				}
			}

			// Check using prerequisites
			if len(req.Using) > 0 {
				allReady := true
				for _, rawSelector := range req.Using {
					selector := make(map[string]string, len(rawSelector))
					for k, v := range rawSelector {
						selector[k] = fmt.Sprintf("%s:%s", w.Metadata.Name, v)
					}
					found := false
					for _, sibling := range cb.GetWants() {
						if sibling.Metadata.ID == w.Metadata.ID {
							continue
						}
						if !w.isOwnerOf(sibling) {
							continue
						}
						if matchesLabels(sibling.Metadata.Labels, selector) {
							found = true
							if sibling.Status != WantStatusAchieved {
								allReady = false
							}
						}
					}
					if !found {
						allReady = false
					}
				}
				if !allReady {
					continue
				}
			}

			// Dispatch New Want
			w.StoreLog("[%s] Realizing direction '%s' (type: %s)", DispatchThinkerName, direction, req.Type)
			namespacedDirection := fmt.Sprintf("%s:%s", w.Metadata.Name, direction)
			child := &Want{
				Metadata: Metadata{
					ID:   GenerateUUID(),
					Name: fmt.Sprintf("%s-%s", direction, w.Metadata.Name),
					Type: req.Type,
					Labels: map[string]string{
						"direction": namespacedDirection,
					},
					Series:  inheritedSeries,
					Version: inheritedVersion + 1,
				},
				Spec: WantSpec{
					Params: req.Params,
				},
			}

			if err := w.AddChildWant(child); err != nil {
				w.StoreLog("[%s] ERROR dispatching: %v", DispatchThinkerName, err)
				continue
			}

			dispatched[direction] = child.Metadata.ID
			changed = true
		}

		if changed || true {
			w.storeState("_dispatched_directions", dispatched)
			w.storeState("_completed_direction_ids", completedIDs)
		}

		return nil
	})
}

// ShouldRunAgent checks if an agent should run based on its input data.
// It calculates a hash of the input data and compares it with a previously stored hash.
// If the hash has changed, it returns true and the new hash.
// The caller should call want.SetInternal(hashKey, newHash) after successful execution.
func ShouldRunAgent(want *Want, hashKey string, inputs ...any) (bool, string) {
	if len(inputs) == 0 {
		return true, ""
	}

	// Serialize all inputs to JSON to compute a combined hash
	var combined []byte
	for _, input := range inputs {
		if input == nil {
			combined = append(combined, []byte("null")...)
			continue
		}
		data, err := json.Marshal(input)
		if err != nil {
			// If marshalling fails, we can't reliably detect changes, so we assume it changed
			return true, ""
		}
		combined = append(combined, data...)
	}

	hash := fmt.Sprintf("%x", md5.Sum(combined))
	prevHash := GetState[string](want, hashKey, "")

	return prevHash != hash, hash
}
