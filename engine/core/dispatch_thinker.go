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

// matchesLabels returns true if all selector key=value pairs are present in labels.
func matchesLabels(labels, selector map[string]string) bool {
	for k, v := range selector {
		if labels[k] != v {
			return false
		}
	}
	return true
}

// NewDispatchThinker creates a ThinkingAgent that monitors the want's "directions" state
// and dispatches/manages child wants based on the "direction_map".
func NewDispatchThinker(id string) *ThinkingAgent {
	return NewThinkingAgent(id, 1*time.Second, DispatchThinkerName, func(ctx context.Context, w *Want) error {
		// 1. Get desired directions from state (set by Itinerary or other planners)
		var desiredDirections []string
		if raw, ok := w.getState("directions"); ok {
			if slice, ok := raw.([]string); ok {
				desiredDirections = slice
			} else if slice, ok := raw.([]any); ok {
				for _, item := range slice {
					if s, ok := item.(string); ok {
						desiredDirections = append(desiredDirections, s)
					}
				}
			}
		}

		// 1.5. Sync provider_state_map unconditionally (before early return).
		// Maps child want names to parent state flags so Rego can see provider completion.
		// Must run before the desiredDirections check to bootstrap the planning cycle:
		// providers achieve → flags set → Rego sees flags → outputs directions → dispatch.
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
			// Collect all provider state keys and publish as provider_keys so Rego
			// can compute all_providers_done generically without hardcoding flag names.
			keys := make([]string, 0, len(providerStateMap))
			for _, stateKey := range providerStateMap {
				keys = append(keys, stateKey)
			}
			w.SetCurrent("provider_keys", keys)

			for childName, stateKey := range providerStateMap {
				if cur, ok := w.GetCurrent(stateKey); ok {
					if b, ok := cur.(bool); ok && b {
						continue // already true
					}
				}
				for _, child := range cb.GetWants() {
					if child.Metadata.Name == childName && w.isOwnerOf(child) {
						if child.Status == WantStatusAchieved {
							w.SetCurrent(stateKey, true)
							w.StoreLog("[%s] Provider '%s' achieved → %s=true", DispatchThinkerName, childName, stateKey)
						}
						break
					}
				}
			}
		}

		if len(desiredDirections) == 0 {
			return nil
		}

		// 2. Get direction_map from params
		directionMap := make(map[string]DirectionConfig)
		if rawMap, ok := w.Spec.Params["direction_map"]; ok {
			// Handle both map[string]any and JSON string
			switch v := rawMap.(type) {
			case string:
				json.Unmarshal([]byte(v), &directionMap)
			case map[string]any:
				bytes, _ := json.Marshal(v)
				json.Unmarshal(bytes, &directionMap)
			}
		}

		if len(directionMap) == 0 {
			return nil
		}

		// 3. Load tracking state
		dispatched := make(map[string]string)
		if raw, ok := w.getState("_dispatched_directions"); ok {
			if m, ok := raw.(map[string]string); ok {
				dispatched = m
			} else if m, ok := raw.(map[string]any); ok {
				for k, v := range m {
					if s, ok := v.(string); ok { dispatched[k] = s }
				}
			}
		}
		
		completedIDs := make(map[string]string)
		if raw, ok := w.getState("_completed_direction_ids"); ok {
			if m, ok := raw.(map[string]string); ok {
				completedIDs = m
			} else if m, ok := raw.(map[string]any); ok {
				for k, v := range m {
					if s, ok := v.(string); ok { completedIDs[k] = s }
				}
			}
		}

		const doneMarker = "DONE"
		cb := GetGlobalChainBuilder()

		// 3.5. Reconcile: recover tracking state from existing child wants (e.g. after server restart).
		// For each direction not yet tracked, scan child wants to find any already dispatched/achieved.
		// Note: dynamic children get namespaced direction labels (e.g. "otp-pipeline:build_graph").
		for _, direction := range desiredDirections {
			if v, already := dispatched[direction]; already && v != "" {
				continue // Already tracked
			}
			namespacedDir := fmt.Sprintf("%s:%s", w.Metadata.Name, direction)
			for _, child := range cb.GetWants() {
				if child.Metadata.Labels["direction"] == namespacedDir && w.isOwnerOf(child) {
					if child.Status == WantStatusAchieved {
						// Already done - apply sets and mark complete
						if cfg, ok := directionMap[direction]; ok && len(cfg.Sets) > 0 {
							for k, v := range cfg.Sets {
								w.SetCurrent(k, v)
							}
						}
						completedIDs[direction] = child.Metadata.ID
						dispatched[direction] = doneMarker
						w.StoreLog("[%s] Reconciled completed direction '%s' (want: %s)", DispatchThinkerName, direction, child.Metadata.ID)
					} else {
						// In-progress - resume tracking
						dispatched[direction] = child.Metadata.ID
						w.StoreLog("[%s] Reconciled in-progress direction '%s' (want: %s)", DispatchThinkerName, direction, child.Metadata.ID)
					}
					break
				}
			}
		}
		// 3.6. If a DONE direction's child want was deleted from the system,
		// reset the DONE marker and its sets flags so the Rego planner can re-suggest it.
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
				if cfg, ok := directionMap[direction]; ok {
					for k, v := range cfg.Sets {
						if b, ok := v.(bool); ok && b {
							w.SetCurrent(k, false)
						}
					}
				}
				w.SetStatus(WantStatusReaching)
				w.StoreLog("[%s] Direction '%s' want was deleted — resetting for re-dispatch", DispatchThinkerName, direction)
			} else {
				// Want still exists but verify it is still actually achieved (e.g. file deleted).
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
						if cfg, ok := directionMap[direction]; ok {
							for k, v := range cfg.Sets {
								if b, ok := v.(bool); ok && b {
									w.SetCurrent(k, false)
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
			if wantID == doneMarker { continue }

			// Resolve pending IDs if any
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

			// Check completion
			for _, child := range cb.GetWants() {
				if child.Metadata.ID == wantID {
					if child.Status == WantStatusAchieved {
						completedIDs[direction] = wantID
						dispatched[direction] = doneMarker
						w.StoreLog("[%s] Direction '%s' completed", DispatchThinkerName, direction)

						// APPLY FEEDBACK: Sync results back to parent state
						if cfg, ok := directionMap[direction]; ok {
							// 1. Sync sets (flags like hotel_reserved)
							if len(cfg.Sets) > 0 {
								// Get current sets from parent
								sets := make(map[string]any)
								if raw, ok := w.getState("sets"); ok {
									if m, ok := raw.(map[string]any); ok {
										for k, v := range m { sets[k] = v }
									}
								}
								// Apply updates via labeled state methods (GCP rule: no direct storeState)
								for k, v := range cfg.Sets {
									sets[k] = v
									w.SetCurrent(k, v)
								}
								w.storeState("sets", sets)
							}

							// 2. Sync cost
							if cfg.CostField != "" {
								// Support both legacy "cost" and GCP-style "actual_cost"
								var cost float64
								var ok bool
								if rawCost, found := child.GetCurrent("actual_cost"); found {
									cost = ToFloat64(rawCost, 0)
									ok = cost > 0
								} else {
									cost, ok = child.GetStateFloat64("cost", 0)
								}

								if ok {
									// Write cost to the designated field in parent state.
									// NOTE: We don't add to the "costs" map here to avoid
									// double-counting with the ConditionThinker.
									w.storeState(cfg.CostField, cost)
									w.StoreLog("[%s] Propagated cost %.2f to %s", DispatchThinkerName, cost, cfg.CostField)
								}
							}
						}
					} else if child.Status == WantStatusCancelled || child.Status == WantStatusFailed {
						// If failed or cancelled without being requested, allow re-dispatch
						delete(dispatched, direction)
					}
					break
				}
			}
		}

		// 5. Process desired directions
		changed := false
		for _, direction := range desiredDirections {
			if v, already := dispatched[direction]; already && v != "" {
				continue // In-flight or Done
			}

			cfg, exists := directionMap[direction]
			if !exists {
				w.StoreLog("[%s] WARNING: No mapping for direction '%s'", DispatchThinkerName, direction)
				continue
			}

			// Handle Replacement Orchestration
			var inheritedSeries string
			var inheritedVersion int
			if cfg.CancelsDirection != "" {
				if oldWantID, ok := completedIDs[cfg.CancelsDirection]; ok && oldWantID != "" {
					cancelKey := "_cancel_pending_" + direction
					cancelPendingID, _ := w.GetStateString(cancelKey, "")

					if cancelPendingID == "" {
						// Send cancel signal
						for _, target := range cb.GetWants() {
							if target.Metadata.ID == oldWantID {
								target.storeState("_cancel_requested", true)
								cb.RestartWant(oldWantID)
								w.storeState(cancelKey, oldWantID)
								w.StoreLog("[%s] Requested cancel for old direction '%s' (want: %s)", DispatchThinkerName, cfg.CancelsDirection, oldWantID)
								break
							}
						}
						continue 
					}

					// Wait for cancel to complete
					oldCancelled := false
					for _, target := range cb.GetWants() {
						if target.Metadata.ID == cancelPendingID {
							if target.Status == WantStatusCancelled {
								oldCancelled = true
								inheritedSeries = target.Metadata.Series
								inheritedVersion = target.Metadata.Version
							} else if target.Status == WantStatusModuleError || target.Status == WantStatusFailed {
								// Cancel ended in error; force to cancelled and proceed with replacement
								inheritedSeries = target.Metadata.Series
								inheritedVersion = target.Metadata.Version
								target.SetStatus(WantStatusCancelled)
								oldCancelled = true
								w.StoreLog("[%s] Cancel for '%s' ended in %s; forced cancelled, proceeding with replacement", DispatchThinkerName, cfg.CancelsDirection, target.Status)
							}
							break
						}
					}
					if !oldCancelled {
						continue // Still waiting
					}
					w.storeState(cancelKey, "") // Clear marker
				}
			}

			// Check using prerequisites: all sibling wants matching the selectors must be achieved.
			// If no sibling matches a selector, treat as not ready (provider not yet dispatched).
			// Recipe labels are namespaced as "<targetName>:<value>", so we namespace selector values too.
			if len(cfg.Using) > 0 {
				allReady := true
				for _, rawSelector := range cfg.Using {
					// Namespace selector values to match how recipe_loader_generic namespaces labels
					selector := make(map[string]string, len(rawSelector))
					for k, v := range rawSelector {
						selector[k] = fmt.Sprintf("%s:%s", w.Metadata.Name, v)
					}
					found := false
					for _, sibling := range cb.GetWants() {
						if sibling.Metadata.ID == w.Metadata.ID { continue }
						if !w.isOwnerOf(sibling) { continue }
						if matchesLabels(sibling.Metadata.Labels, selector) {
							found = true
							if sibling.Status != WantStatusAchieved {
								allReady = false
							}
						}
					}
					if !found {
						allReady = false // Provider for this selector not yet in system
					}
				}
				if !allReady {
					continue // Prerequisites not met yet
				}
			}

			// Dispatch New Want
			w.StoreLog("[%s] Realizing direction '%s' (type: %s)", DispatchThinkerName, direction, cfg.Type)
			namespacedDirection := fmt.Sprintf("%s:%s", w.Metadata.Name, direction)
			child := &Want{
				Metadata: Metadata{
					ID:      GenerateUUID(),
					Name:    fmt.Sprintf("%s-%s", direction, w.Metadata.Name),
					Type:    cfg.Type,
					Labels: map[string]string{
						"direction": namespacedDirection,
					},
					Series:  inheritedSeries,
					Version: inheritedVersion + 1,
				},
				Spec: WantSpec{
					Params: cfg.Params,
					// NOTE: Do NOT set Using here. dispatch_thinker already verifies
					// prerequisites before dispatching. Passing Using to the child
					// would cause it to wait for PubSub packets that were already
					// sent by providers that achieved before this child was created.
				},
			}

			if err := w.AddChildWant(child); err != nil {
				w.StoreLog("[%s] ERROR dispatching: %v", DispatchThinkerName, err)
				continue
			}

			dispatched[direction] = child.Metadata.ID
			changed = true
		}

		if changed || true { // Always save to ensure sync
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
