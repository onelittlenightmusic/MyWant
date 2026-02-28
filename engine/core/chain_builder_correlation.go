package mywant

import (
	"fmt"
	"sort"
	"strings"
)

// buildStateAccessIndex reconstructs the system-wide mapping of state fields to their accessors.
// This "State Access Dictionary" is a structural representation of data dependencies,
// moving beyond transient label-based correlation.
// Called during reconcileWants() before correlationPhase().
func (cb *ChainBuilder) buildStateAccessIndex() {
	// 1. Clear existing index
	cb.stateAccessIndex = make(map[string][]string)

	// Helper to register an accessor to a specific field
	register := func(providerID, fieldName, accessorID string) {
		key := fmt.Sprintf("%s.%s", providerID, fieldName)
		cb.stateAccessIndex[key] = append(cb.stateAccessIndex[key], accessorID)
	}

	for _, rw := range cb.wants {
		want := rw.want
		wantID := want.Metadata.ID

		// A. Process Explicit StateSubscriptions (Reader side)
		for _, sub := range want.Spec.StateSubscriptions {
			// Find provider want ID by name
			providerRW, ok := cb.wants[sub.WantName]
			if !ok {
				continue
			}
			providerID := providerRW.want.Metadata.ID

			keys := sub.StateKeys
			if len(keys) == 0 {
				keys = []string{"*"}
			}
			for _, k := range keys {
				register(providerID, k, wantID)
			}
		}

		// B. Process ParentStateAccess (ThinkAgent capabilities)
		if cb.agentRegistry != nil && len(want.Metadata.OwnerReferences) > 0 {
			// Find parent ID
			var parentID string
			for _, ref := range want.Metadata.OwnerReferences {
				if ref.Controller && ref.Kind == "Want" {
					parentID = ref.ID
					break
				}
			}

			if parentID != "" {
				// Check capabilities of this want (from its type definition)
				if def := want.WantTypeDefinition; def != nil {
					for _, capName := range def.Requires {
						if cap, ok := cb.agentRegistry.GetCapability(capName); ok {
							for _, field := range cap.ParentStateAccess {
								register(parentID, field.Name, wantID)
							}
						}
					}
				}
			}
		}

		// C. Self-registration for provider (A provider is an accessor of its own state)
		// This ensures the field exists in the dictionary even if there are no readers yet.
		// Also helps correlate siblings who write to the same field.
		if def := want.WantTypeDefinition; def != nil {
			for _, state := range def.State {
				register(wantID, state.Name, wantID)
			}
		}
	}

	// Post-processing: remove duplicates if any (e.g. multiple capabilities accessing same field)
	for key, accessors := range cb.stateAccessIndex {
		unique := make(map[string]struct{})
		for _, a := range accessors {
			unique[a] = struct{}{}
		}
		if len(unique) != len(accessors) {
			newAccessors := make([]string, 0, len(unique))
			for a := range unique {
				newAccessors = append(newAccessors, a)
			}
			sort.Strings(newAccessors)
			cb.stateAccessIndex[key] = newAccessors
		}
	}
}

// correlationPhase computes inter-Want Correlation for ALL Wants.
// It utilizes the stateAccessIndex dictionary for efficient dependency discovery.
// We recompute for all wants because correlation is a reciprocal relationship;
// adding or updating one Want can affect the correlation metadata of its peers.
func (cb *ChainBuilder) correlationPhase() {
	// Always clear dirty set on exit.
	defer func() { cb.dirtyWantIDs = make(map[string]struct{}) }()

	// Process all runtime wants to ensure reciprocal consistency.
	for _, rw := range cb.wants {
		dirty := rw.want
		dirtyID := dirty.Metadata.ID

		// peerID → set of correlation labels (de-duplicated via map)
		peerLabels := make(map[string]map[string]struct{})
		add := func(peerID, label string) {
			if peerID == dirtyID || peerID == "" {
				return
			}
			if _, ok := peerLabels[peerID]; !ok {
				peerLabels[peerID] = make(map[string]struct{})
			}
			peerLabels[peerID][label] = struct{}{}
		}

		// ── 1. Metadata Label Selectors (Inverted Index) ──────────────────────
		// labelToUsers index covers Wants referencing dirty via selector.
		for k, v := range dirty.Metadata.Labels {
			key := cb.selectorToKey(map[string]string{k: v})
			for _, userName := range cb.labelToUsers[key] {
				// userName is a name, we need its ID for consistency.
				if userRW, ok := cb.wants[userName]; ok {
					add(userRW.want.Metadata.ID, k+"="+v)
				}
			}
		}

		// ── 2. Wants that dirty references via its own using selectors ────────
		if spec := dirty.GetSpec(); spec != nil {
			for _, sel := range spec.Using {
				for _, peerRW := range cb.wants {
					if peerRW.want.Metadata.ID == dirtyID {
						continue
					}
					if cb.matchesSelector(peerRW.want.Metadata.Labels, sel) {
						for k, v := range sel {
							add(peerRW.want.Metadata.ID, "using.select/"+k+"="+v)
						}
					}
				}
			}
		}

		// ── 3. State Access Dependencies (using the structural Dictionary) ────
		// Find all fields accessed by dirty, then find other wants accessing the SAME fields.
		for fieldPath, accessors := range cb.stateAccessIndex {
			// fieldPath is "providerID.fieldName"
			isDirtyAccessor := false
			for _, a := range accessors {
				if a == dirtyID {
					isDirtyAccessor = true
					break
				}
			}

			if isDirtyAccessor {
				// Dirty accesses this field. All other accessors are correlated peers.
				parts := strings.Split(fieldPath, ".")
				providerID := parts[0]
				for _, peerID := range accessors {
					if peerID != dirtyID {
						add(peerID, "stateAccess/"+fieldPath)
					}
				}
				// Also correlate dirty with the provider of the field (if not self)
				if providerID != dirtyID {
					add(providerID, "stateAccess/"+fieldPath)
				}
			}
		}

		// ── Build CorrelationEntry slice ──────────────────────────────────────
		entries := make([]CorrelationEntry, 0, len(peerLabels))
		for peerID, labelSet := range peerLabels {
			labels := make([]string, 0, len(labelSet))
			for l := range labelSet {
				labels = append(labels, l)
			}
			sort.Strings(labels)
			entries = append(entries, CorrelationEntry{
				WantID: peerID,
				Labels: labels,
				Rate:   correlationRate(labels),
			})
		}
		// Sort by descending Rate for stable, readable output.
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].Rate > entries[j].Rate
		})
		dirty.Metadata.Correlation = entries
	}
}

// correlationRate returns the weighted coupling strength for a set of
// correlation labels.
func correlationRate(labels []string) int {
	rate := 0
	for _, l := range labels {
		switch {
		case strings.HasPrefix(l, "stateAccess/"):
			rate += 2
		default:
			rate += 1
		}
	}
	return rate
}
