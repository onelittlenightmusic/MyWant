package mywant

// want_using.go — import-based gate evaluation
//
// Step 3.9 of the main progress loop: before running Progress(), check that all
// imported fields have resolved to a non-nil value. A nil import means the
// upstream want has not yet produced its result, so the want stays idle and
// polls on the next cycle. This replaces the old using:when: packet-based
// mechanism which was not idempotent across restarts.

// hasUnresolvedImports returns true if any imported field currently resolves to
// nil. A nil value means the upstream provider has not yet produced it; Progress()
// must not run until all imports are non-nil.
func (n *Want) hasUnresolvedImports() bool {
	for _, localKey := range n.Spec.Imports {
		val, ok := n.getState(localKey)
		if !ok || val == nil {
			return true
		}
	}
	return false
}

// HasUsingWhenConditions returns true if at least one using: entry has a When condition.
func (n *Want) HasUsingWhenConditions() bool {
	return n.hasUsingWhenConditions()
}

// CheckUsingWhenConditions is the exported form for use by agent implementations.
func (n *Want) CheckUsingWhenConditions() bool {
	return n.checkUsingWhenConditions()
}

// hasUsingWhenConditions returns true if at least one using: entry has a When condition.
// Only such wants are subject to the step-3.9 data gate; plain using: entries are unaffected.
func (n *Want) hasUsingWhenConditions() bool {
	for _, entry := range n.Spec.Using {
		if entry.When != nil {
			return true
		}
	}
	return false
}

// checkUsingWhenConditions evaluates the when: conditions declared on using entries.
// First checks cached packet data; if no packet, reads live from the provider want
// found via gate label matching. This makes the check idempotent across restarts.
func (n *Want) checkUsingWhenConditions() bool {
	n.cacheMutex.Lock()
	cached := n.cachedPacket
	n.cacheMutex.Unlock()

	var packetData map[string]any
	if cached != nil {
		packetData, _ = cached.Packet.(map[string]any)
	}

	cb := GetGlobalChainBuilder()

	for _, entry := range n.Spec.Using {
		if entry.When == nil {
			continue
		}
		field := resolveConditionField(entry.When.Field)
		var actual any

		if packetData != nil {
			actual = packetData[field]
		} else if cb != nil {
			// No cached packet — read live from the provider want.
			// Find provider by matching gate labels (skip routing-only labels).
			routingKeys := map[string]bool{"owner-name": true, "owner": true}
			allWants := cb.GetAllWantStates()
			for _, pw := range allWants {
				if pw.Metadata.ID == n.Metadata.ID {
					continue
				}
				match := true
				for k, v := range entry.Labels {
					if routingKeys[k] {
						continue
					}
					if pw.Metadata.Labels[k] != v {
						match = false
						break
					}
				}
				if match {
					actual, _ = pw.getState(field)
					break
				}
			}
		}

		if !evaluateCondition(actual, entry.When.Operator, entry.When.Value) {
			return false
		}
	}
	return true
}
