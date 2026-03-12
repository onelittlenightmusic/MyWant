package mywant

// CrossWantStateEntry represents a single state field found across wants.
type CrossWantStateEntry struct {
	WantID   string     `json:"want_id,omitempty"`
	WantName string     `json:"want_name,omitempty"`
	Key      string     `json:"key"`
	Value    any        `json:"value"`
	Label    StateLabel `json:"label"`
	Source   string     `json:"source"` // "want" or "global"
}

// --- ChainBuilder methods ---

// GetWantStateValue returns a single state key from the want identified by wantID.
func (cb *ChainBuilder) GetWantStateValue(wantID, key string) (any, bool) {
	want, _, found := cb.FindWantByID(wantID)
	if !found {
		return nil, false
	}
	return want.State.Load(key)
}

// GetWantStateAll returns a full snapshot of the state for the want identified by wantID.
func (cb *ChainBuilder) GetWantStateAll(wantID string) (map[string]any, bool) {
	want, _, found := cb.FindWantByID(wantID)
	if !found {
		return nil, false
	}
	return want.GetAllState(), true
}

// StoreWantState stores a key-value pair in the state of the want identified by wantID.
// Returns false if the want is not found.
func (cb *ChainBuilder) StoreWantState(wantID, key string, value any) bool {
	want, _, found := cb.FindWantByID(wantID)
	if !found {
		return false
	}
	want.StoreState(key, value)
	return true
}

// DeleteWantState removes a key from the state of the want identified by wantID.
// Returns false if the want is not found.
func (cb *ChainBuilder) DeleteWantState(wantID, key string) bool {
	want, _, found := cb.FindWantByID(wantID)
	if !found {
		return false
	}
	want.DeleteState(key)
	return true
}

// SearchWantStateByField returns all state entries (across all wants + global state)
// that contain the given field name. Pass includeGlobal=true to include global state.
// Pass ancestorID="" to search all wants; set it to a want ID to limit to descendants only.
func (cb *ChainBuilder) SearchWantStateByField(field, ancestorID string, includeGlobal bool) []CrossWantStateEntry {
	allWants := cb.GetWants()

	var descendants map[string]bool
	if ancestorID != "" {
		descendants = cb.collectDescendantIDs(ancestorID, allWants)
	}

	var results []CrossWantStateEntry
	for _, want := range allWants {
		if descendants != nil && !descendants[want.Metadata.ID] {
			continue
		}
		val, ok := want.State.Load(field)
		if !ok {
			continue
		}
		label := LabelNone
		if l, has := want.StateLabels[field]; has {
			label = l
		}
		results = append(results, CrossWantStateEntry{
			WantID:   want.Metadata.ID,
			WantName: want.Metadata.Name,
			Key:      field,
			Value:    val,
			Label:    label,
			Source:   "want",
		})
	}

	if includeGlobal {
		if val, ok := cb.globalState.Load(field); ok {
			results = append(results, CrossWantStateEntry{
				Key:    field,
				Value:  val,
				Label:  LabelNone,
				Source: "global",
			})
		}
	}
	return results
}

// collectDescendantIDs performs a BFS over OwnerReferences to collect all descendant
// Want IDs of the given ancestorID.
func (cb *ChainBuilder) collectDescendantIDs(ancestorID string, allWants []*Want) map[string]bool {
	childrenOf := make(map[string][]string)
	for _, w := range allWants {
		for _, ref := range w.Metadata.OwnerReferences {
			if ref.Controller {
				childrenOf[ref.ID] = append(childrenOf[ref.ID], w.Metadata.ID)
			}
		}
	}
	descendants := make(map[string]bool)
	queue := childrenOf[ancestorID]
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		descendants[id] = true
		queue = append(queue, childrenOf[id]...)
	}
	return descendants
}

// --- Package-level functions (callable from any Want or code) ---

// GetWantState returns a single state key from the want identified by wantID.
func GetWantState(wantID, key string) (any, bool) {
	cb := GetGlobalChainBuilder()
	if cb == nil {
		return nil, false
	}
	return cb.GetWantStateValue(wantID, key)
}

// GetAllWantState returns a full state snapshot for the want identified by wantID.
func GetAllWantState(wantID string) (map[string]any, bool) {
	cb := GetGlobalChainBuilder()
	if cb == nil {
		return nil, false
	}
	return cb.GetWantStateAll(wantID)
}

// StoreWantState stores a key-value pair in the state of the want identified by wantID.
func StoreWantState(wantID, key string, value any) bool {
	cb := GetGlobalChainBuilder()
	if cb == nil {
		return false
	}
	return cb.StoreWantState(wantID, key, value)
}

// DeleteWantState removes a key from the state of the want identified by wantID.
func DeleteWantState(wantID, key string) bool {
	cb := GetGlobalChainBuilder()
	if cb == nil {
		return false
	}
	return cb.DeleteWantState(wantID, key)
}

// SearchStateByField returns all state entries matching the given field name across all wants.
// Set includeGlobal=true to also check global state.
// Set ancestorID="" to search all wants, or a want ID to scope to descendants only.
func SearchStateByField(field, ancestorID string, includeGlobal bool) []CrossWantStateEntry {
	cb := GetGlobalChainBuilder()
	if cb == nil {
		return nil
	}
	return cb.SearchWantStateByField(field, ancestorID, includeGlobal)
}

// --- Want convenience methods (callable as want.GetWantState(...)) ---

// GetStateOf returns a single state key from the want identified by wantID.
func (n *Want) GetStateOf(wantID, key string) (any, bool) {
	return GetWantState(wantID, key)
}

// GetAllStateOf returns a full state snapshot for the want identified by wantID.
func (n *Want) GetAllStateOf(wantID string) (map[string]any, bool) {
	return GetAllWantState(wantID)
}

// StoreStateOf stores a key-value pair in the state of the want identified by wantID.
func (n *Want) StoreStateOf(wantID, key string, value any) bool {
	return StoreWantState(wantID, key, value)
}

// DeleteStateOf removes a key from the state of the want identified by wantID.
func (n *Want) DeleteStateOf(wantID, key string) bool {
	return DeleteWantState(wantID, key)
}

// SearchStateByField returns all state entries matching field across all wants (+ optionally global).
func (n *Want) SearchStateByField(field, ancestorID string, includeGlobal bool) []CrossWantStateEntry {
	return SearchStateByField(field, ancestorID, includeGlobal)
}
