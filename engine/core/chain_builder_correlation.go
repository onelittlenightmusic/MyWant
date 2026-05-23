package mywant

import (
	"fmt"
	"log"
	"sort"
	"strings"
)

// buildStateAccessIndex reconstructs the system-wide mapping of state fields to their accessors.
// This "State Access Dictionary" is a structural representation of data dependencies,
// moving beyond transient label-based correlation.
// Called during reconcileWants() before correlationPhase().
func (cb *ChainBuilder) buildStateAccessIndex() {
	// 1. Clear existing indices
	cb.stateAccessIndex = make(map[string][]string)
	cb.fieldConsumerIndex = make(map[string]map[string]struct{})
	cb.fieldProviderIndex = make(map[string]map[string]struct{})
	cb.fieldAccessDetails = make(map[string]map[string]map[string]struct{})

	// Helper to register an accessor to a specific field and maintain bidirectional peer indices.
	register := func(providerID, fieldName, accessorID string) {
		key := fmt.Sprintf("%s.%s", providerID, fieldName)
		cb.stateAccessIndex[key] = append(cb.stateAccessIndex[key], accessorID)
		if providerID == accessorID {
			return // self-registration does not create a peer relationship
		}
		// fieldConsumerIndex: providerID → set of consumerIDs
		if cb.fieldConsumerIndex[providerID] == nil {
			cb.fieldConsumerIndex[providerID] = make(map[string]struct{})
		}
		cb.fieldConsumerIndex[providerID][accessorID] = struct{}{}
		// fieldProviderIndex: consumerID → set of providerIDs
		if cb.fieldProviderIndex[accessorID] == nil {
			cb.fieldProviderIndex[accessorID] = make(map[string]struct{})
		}
		cb.fieldProviderIndex[accessorID][providerID] = struct{}{}
		// fieldAccessDetails: providerID → consumerID → set of fieldNames
		if cb.fieldAccessDetails[providerID] == nil {
			cb.fieldAccessDetails[providerID] = make(map[string]map[string]struct{})
		}
		if cb.fieldAccessDetails[providerID][accessorID] == nil {
			cb.fieldAccessDetails[providerID][accessorID] = make(map[string]struct{})
		}
		cb.fieldAccessDetails[providerID][accessorID][fieldName] = struct{}{}
	}

	for _, rw := range cb.wants {
		want := rw.want
		wantID := want.Metadata.ID
		if wantID == "" {
			continue
		}

		// A. Process Explicit StateSubscriptions (Reader side)
		for _, sub := range want.Spec.StateSubscriptions {
			// Find provider want ID by name via the name→ID index
			providerID, nameKnown := cb.wantNameToID[sub.WantName]
			if !nameKnown {
				continue
			}
			if _, ok := cb.wants[providerID]; !ok {
				continue
			}
			if providerID == "" {
				continue
			}

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
				if def, ok := cb.wantTypeDefinitions[want.Metadata.Type]; ok {
					// Check regular agent requirements (now includes think-agent capabilities)
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
		if def, ok := cb.wantTypeDefinitions[want.Metadata.Type]; ok {
			for _, state := range def.State {
				register(wantID, state.Name, wantID)
			}
		}
	}

	// D. Import-param references: if want B has an import-style param (e.g. choice_import_field)
	// whose string value matches an expose.As key declared by want A, treat B as a consumer of A's
	// exposed state.
	//
	// NOTE: we intentionally do NOT match against raw type-definition state field names here.
	// A type definition declaring a field does not mean the field is accessible — it must be
	// explicitly exposed via Spec.Exposes.  Matching unexposed fields caused phantom correlations
	// (stateAccess dots) for wants that referenced a field name that happened to exist in another
	// want's type definition, even though no actual data flow existed.
	exposeKeyToProvider := make(map[string]string) // exposeAs key → providerWantID
	for _, rw := range cb.wants {
		w := rw.want
		wID := w.Metadata.ID
		if wID == "" {
			continue
		}
		for _, exp := range w.Spec.Exposes {
			if exp.As != "" {
				exposeKeyToProvider[exp.As] = wID
			}
		}
	}
	for _, rw := range cb.wants {
		w := rw.want
		wID := w.Metadata.ID
		if wID == "" || w.Spec.Params == nil {
			continue
		}
		for paramName, paramVal := range w.Spec.Params {
			strVal, ok := paramVal.(string)
			if !ok || strVal == "" {
				continue
			}
			if !isCorrelationImportParam(paramName) {
				continue
			}
			// Match against expose global keys only
			if providerID, found := exposeKeyToProvider[strVal]; found && providerID != wID {
				register(providerID, "expose/"+strVal, wID)
			}
		}
	}

	// Section E: Imports-based correlations.
	// When a want declares `imports: { globalKey: localKey }`, find the want that exposes
	// `as: globalKey` and register a direct stateAccess correlation between them.
	// This replaces ad-hoc param-name inference (e.g. choice_import_field) with an explicit link.
	for _, rw := range cb.wants {
		w := rw.want
		wID := w.Metadata.ID
		if wID == "" || len(w.Spec.Imports) == 0 {
			continue
		}
		for globalKey := range w.Spec.Imports {
			if providerID, found := exposeKeyToProvider[globalKey]; found && providerID != wID {
				register(providerID, "expose/"+globalKey, wID)
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

	if len(cb.stateAccessIndex) > 0 {
		log.Printf("[ACCESS-INDEX] Built state access index with %d fields\n", len(cb.stateAccessIndex))
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
			for _, userID := range cb.labelToUsers[key] {
				// userID is the want ID (labelToUsers stores IDs after migration).
				if _, ok := cb.wants[userID]; ok {
					add(userID, k+"="+v)
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

		// ── 3. State Access Dependencies (bidirectional peer index) ──────────────
		// Direct consumers: wants that read at least one field dirty provides.
		// Emit one label per field so the frontend can display exactly which fields flow.
		for consumerID := range cb.fieldConsumerIndex[dirtyID] {
			for fieldName := range cb.fieldAccessDetails[dirtyID][consumerID] {
				add(consumerID, "stateAccess/consumer:"+fieldName)
			}
		}
		// Direct providers: wants whose fields dirty reads.
		// Also add co-consumers (siblings): other wants reading the same provider's fields.
		for providerID := range cb.fieldProviderIndex[dirtyID] {
			for fieldName := range cb.fieldAccessDetails[providerID][dirtyID] {
				add(providerID, "stateAccess/provider:"+fieldName)
			}
			for siblingID := range cb.fieldConsumerIndex[providerID] {
				if siblingID != dirtyID {
					add(siblingID, "stateAccess/sibling")
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
// stateAccess labels contribute a fixed +2 regardless of how many field-specific
// labels exist (stateAccess/consumer:fieldA, stateAccess/consumer:fieldB …) to
// prevent rate inflation when many fields are shared.
func correlationRate(labels []string) int {
	rate := 0
	hasStateAccess := false
	for _, l := range labels {
		switch {
		case strings.HasPrefix(l, "stateAccess/"):
			hasStateAccess = true
		default:
			rate += 1
		}
	}
	if hasStateAccess {
		rate += 2
	}
	return rate
}

// isCorrelationImportParam returns true if the param name looks like a field-import selector.
// Mirrors the server-side isImportParam heuristic without a cross-package dependency.
func isCorrelationImportParam(name string) bool {
	lower := strings.ToLower(name)
	return strings.Contains(lower, "_import_field") ||
		strings.Contains(lower, "_source_field") ||
		strings.HasSuffix(lower, "_field") ||
		strings.Contains(lower, "_from_field")
}
