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
	Type           string         `json:"type"`
	Params         map[string]any `json:"params"`
	Sets           map[string]any `json:"sets"`
	CostField      string         `json:"cost_field"`
	CancelsDirection string       `json:"cancels_direction,omitempty"`
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

		// 4. Resolve IDs and check completion
		for direction, wantID := range dispatched {
			if wantID == doneMarker { continue }

			// Resolve pending IDs if any
			if strings.HasPrefix(wantID, "pending-") {
				for _, child := range cb.GetWants() {
					if child.Metadata.Labels["direction"] == direction && w.isOwnerOf(child) {
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
								// Apply updates
								for k, v := range cfg.Sets {
									sets[k] = v
									// Also write directly to parent state for convenience
									w.storeState(k, v) 
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

			// Dispatch New Want
			w.StoreLog("[%s] Realizing direction '%s' (type: %s)", DispatchThinkerName, direction, cfg.Type)
			
			child := &Want{
				Metadata: Metadata{
					ID:      GenerateUUID(),
					Name:    fmt.Sprintf("%s-%s", direction, w.Metadata.Name),
					Type:    cfg.Type,
					Labels: map[string]string{
						"direction": direction,
					},
					Series:  inheritedSeries,
					Version: inheritedVersion + 1,
				},
				Spec: WantSpec{
					Params: cfg.Params,
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
