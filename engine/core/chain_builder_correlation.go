package mywant

import (
	"sort"
	"strings"
)

// correlationPhase computes inter-Want Correlation for the Wants recorded in
// dirtyWantIDs.  It runs after connectPhase() so that labelToUsers and
// OwnerReferences are already up-to-date.
//
// Called inside reconcileWants(), which already holds reconcileMutex, so
// cb.wants and cb.labelToUsers are safely accessible without additional locks.
func (cb *ChainBuilder) correlationPhase() {
	if len(cb.dirtyWantIDs) == 0 {
		return
	}
	// Always clear dirty set on exit, even if we return early.
	defer func() { cb.dirtyWantIDs = make(map[string]struct{}) }()

	// cb.wants is keyed by name; dirtyWantIDs stores names accordingly.
	for dirtyName := range cb.dirtyWantIDs {
		rw, ok := cb.wants[dirtyName]
		if !ok {
			continue
		}
		dirty := rw.want

		// peerName → set of correlation labels (de-duplicated via map)
		peerLabels := make(map[string]map[string]struct{})
		add := func(peerName, label string) {
			if _, ok := peerLabels[peerName]; !ok {
				peerLabels[peerName] = make(map[string]struct{})
			}
			peerLabels[peerName][label] = struct{}{}
		}

		// ── 1. Wants that reference dirty via a using selector ────────────────
		// labelToUsers["role:coordinator"] = ["want-A", "want-B"]
		// This is an O(L) lookup using the pre-built inverted index from connectPhase.
		for k, v := range dirty.Metadata.Labels {
			key := cb.selectorToKey(map[string]string{k: v})
			for _, userName := range cb.labelToUsers[key] {
				if userName != dirty.Metadata.Name {
					add(userName, k+"="+v)
				}
			}
		}

		// ── 2. Wants that dirty references via its own using selectors ────────
		// labelToUsers only covers the "user side"; for the "provider side" we
		// scan cb.wants once (O(n)) using the existing matchesSelector helper.
		if spec := dirty.GetSpec(); spec != nil {
			for _, sel := range spec.Using {
				for peerName, peerRW := range cb.wants {
					if peerName == dirty.Metadata.Name {
						continue
					}
					if cb.matchesSelector(peerRW.want.Metadata.Labels, sel) {
						for k, v := range sel {
							add(peerName, "using.select/"+k+"="+v)
						}
					}
				}
			}
		}

		// ── 3. State Subscriptions (Field-level data flow) ────────────────────
		// Unified 'stateAccess' relationship indicating field-level coupling.
		// Format: stateAccess/<provider_id>.<field>
		for peerName, peerRW := range cb.wants {
			if peerName == dirty.Metadata.Name {
				continue
			}

			// A. Check if dirty subscribes to peer (peer is provider)
			for _, sub := range dirty.Spec.StateSubscriptions {
				if sub.WantName == peerName {
					keys := sub.StateKeys
					if len(keys) == 0 {
						keys = []string{"*"}
					}
					peerID := peerRW.want.Metadata.ID
					for _, k := range keys {
						add(peerName, "stateAccess/"+peerID+"."+k)
					}
				}
			}

			// B. Check if peer subscribes to dirty (dirty is provider)
			for _, sub := range peerRW.want.Spec.StateSubscriptions {
				if sub.WantName == dirty.Metadata.Name {
					keys := sub.StateKeys
					if len(keys) == 0 {
						keys = []string{"*"}
					}
					dirtyID := dirty.Metadata.ID
					for _, k := range keys {
						add(peerName, "stateAccess/"+dirtyID+"."+k)
					}
				}
			}
		}

		// ── 4. Parent State Access (ThinkAgent capabilities) ──────────────────
		// If a want accesses its parent's state, they share a stateAccess label.
		// Also, siblings that access the same parent field are correlated.
		if cb.agentRegistry != nil {
			// Get fields accessed by dirty
			dirtyParentFields := make(map[string][]string) // parentID -> []fieldNames
			for _, ref := range dirty.Metadata.OwnerReferences {
				if ref.Controller && ref.Kind == "Want" {
					if def := dirty.WantTypeDefinition; def != nil {
						for _, capName := range def.Requires {
							if cap, ok := cb.agentRegistry.GetCapability(capName); ok {
								for _, field := range cap.ParentStateAccess {
									dirtyParentFields[ref.ID] = append(dirtyParentFields[ref.ID], field.Name)
								}
							}
						}
					}
				}
			}

			// Compare with all other wants
			for peerName, peerRW := range cb.wants {
				if peerName == dirty.Metadata.Name {
					continue
				}

				peer := peerRW.want
				peerID := peer.Metadata.ID

				// A. If peer is a parent of dirty
				for _, fields := range dirtyParentFields[peerID] {
					add(peerName, "stateAccess/"+peerID+"."+fields)
				}

				// B. If dirty is a parent of peer
				if def := peer.WantTypeDefinition; def != nil {
					for _, ref := range peer.Metadata.OwnerReferences {
						if ref.Controller && ref.Kind == "Want" && ref.ID == dirty.Metadata.ID {
							for _, capName := range def.Requires {
								if cap, ok := cb.agentRegistry.GetCapability(capName); ok {
									for _, field := range cap.ParentStateAccess {
										add(peerName, "stateAccess/"+dirty.Metadata.ID+"."+field.Name)
									}
								}
							}
						}
					}
				}

				// C. If dirty and peer share the same parent and same field access
				for parentID, dirtyFields := range dirtyParentFields {
					for _, ref := range peer.Metadata.OwnerReferences {
						if ref.Controller && ref.Kind == "Want" && ref.ID == parentID {
							// Shared parent! Check if peer accesses same fields.
							if def := peer.WantTypeDefinition; def != nil {
								for _, capName := range def.Requires {
									if cap, ok := cb.agentRegistry.GetCapability(capName); ok {
										for _, field := range cap.ParentStateAccess {
											// Check if dirty also accesses this field in this parent
											for _, df := range dirtyFields {
												if df == field.Name {
													add(peerName, "stateAccess/"+parentID+"."+field.Name)
												}
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}

		// ── Build CorrelationEntry slice ──────────────────────────────────────
		entries := make([]CorrelationEntry, 0, len(peerLabels))
		for peerName, labelSet := range peerLabels {
			peerRW, ok := cb.wants[peerName]
			if !ok {
				continue
			}
			labels := make([]string, 0, len(labelSet))
			for l := range labelSet {
				labels = append(labels, l)
			}
			sort.Strings(labels)
			entries = append(entries, CorrelationEntry{
				WantID: peerRW.want.Metadata.ID,
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
//
//   - "stateAccess/…"   : +2  (explicit field dependency)
//   - "using.select/…"  : +1  (explicit data-flow dependency)
//   - everything else   : +1  (shared metadata label)
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
