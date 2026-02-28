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

		// ── 3. Sibling Wants (same OwnerReference → may share parent State) ──
		if len(dirty.Metadata.OwnerReferences) > 0 {
			ownerSet := make(map[string]struct{}, len(dirty.Metadata.OwnerReferences))
			for _, ref := range dirty.Metadata.OwnerReferences {
				ownerSet[ref.ID] = struct{}{}
			}
			for peerName, peerRW := range cb.wants {
				if peerName == dirty.Metadata.Name {
					continue
				}
				for _, ref := range peerRW.want.Metadata.OwnerReferences {
					if _, shared := ownerSet[ref.ID]; shared {
						add(peerName, "state.sibling/parent="+ref.ID)
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
//   - "state.sibling/…" : +2  (shared parent State — potential side-effects)
//   - "using.select/…"  : +1  (explicit data-flow dependency)
//   - everything else   : +1  (shared metadata label)
func correlationRate(labels []string) int {
	rate := 0
	for _, l := range labels {
		switch {
		case strings.HasPrefix(l, "state.sibling/"):
			rate += 2
		default:
			rate += 1
		}
	}
	return rate
}
