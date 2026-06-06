package mywant

import (
	"fmt"
	"strings"
)

// want_parent.go — parent/child want relationship, role, dispatch, and suggestion helpers

func (n *Want) getParentWant() *Want {
	if len(n.Metadata.OwnerReferences) == 0 {
		return nil
	}
	var parentID string
	for _, ref := range n.Metadata.OwnerReferences {
		if ref.Controller && ref.Kind == "Want" {
			parentID = ref.ID
			break
		}
	}
	if parentID == "" {
		return nil
	}

	// DO NOT CACHE Parent Want pointer.
	// Caching can lead to using stale Want objects if the parent is recreated
	// during reconciliation or if it was initially found in Config but later promoted to Runtime.
	// Always look up the latest runtime instance from the global builder.

	cb := GetGlobalChainBuilder()
	if cb == nil {
		return nil
	}
	parent, _, found := cb.FindWantByID(parentID)
	if !found {
		return nil
	}
	return parent
}

func (n *Want) GetParentWant() *Want { return n.getParentWant() }

// isOwnerOf returns true if this want is a controller owner of the target want.
func (n *Want) isOwnerOf(target *Want) bool {
	for _, ref := range target.Metadata.OwnerReferences {
		if ref.Controller && ref.ID == n.Metadata.ID {
			return true
		}
	}
	return false
}

// AddChildWant adds a new child want to the system, automatically setting
// this want as the owner (parent).
//
// HIERARCHY RULE: Sub-wants are forbidden from calling cb.AddWant directly.
// They must instead use AddChildWant on their parent, or request the parent
// to dispatch via a ThinkAgent (DispatchThinkerAgent).
func (n *Want) AddChildWant(child *Want) error {
	cb := GetGlobalChainBuilder()
	if cb == nil {
		return fmt.Errorf("ChainBuilder unavailable")
	}

	// Ensure child has a valid ID
	if child.Metadata.ID == "" {
		child.Metadata.ID = GenerateUUID()
	}

	// Set owner reference to this want
	ownerRef := OwnerReference{
		APIVersion: "mywant/v1",
		Kind:       "Want",
		Name:       n.Metadata.Name,
		ID:         n.Metadata.ID,
		Controller: true,
	}
	child.Metadata.OwnerReferences = append(child.Metadata.OwnerReferences, ownerRef)

	// Add to system asynchronously
	return cb.AddWantsAsync([]*Want{child})
}

// HasParent returns true if this want has a parent coordinator.
func (n *Want) HasParent() bool {
	return n.getParentWant() != nil
}

func (n *Want) GetParentState(path string) (any, bool) {
	parent := n.getParentWant()
	if parent == nil {
		return resolveGlobalPath(path)
	}
	// もし path にドットが含まれていたら階層探索、そうでなければ親のステート取得
	if strings.Contains(path, ".") {
		return resolveGlobalPath(path)
	}
	return parent.getState(path)
}

func resolveGlobalPath(path string) (any, bool) {
	parts := strings.Split(path, ".")
	if len(parts) == 0 {
		return nil, false
	}

	// ルート（wants など）を取得
	val, ok := GetGlobalState(parts[0])
	if !ok {
		return nil, false
	}

	// 階層を辿る
	current := val
	for i := 1; i < len(parts); i++ {
		if m, ok := current.(map[string]any); ok {
			current, ok = m[parts[i]]
			if !ok {
				return nil, false
			}
		} else if m, ok := current.(map[any]any); ok {
			// YAML unmarshalでmap[any]anyになる場合への対応
			current, ok = m[parts[i]]
			if !ok {
				return nil, false
			}
		} else {
			return nil, false
		}
	}
	return current, true
}

func (n *Want) StoreParentState(key string, value any) {
	parent := n.getParentWant()
	if parent == nil {
		StoreGlobalState(key, value) // fallback to globalState for top-level wants
		return
	}

	role := n.GetRole()
	label := parent.StateLabels[key]
	engine := &GovernanceEngine{}

	if !engine.CanWriteParentState(role, label) {
		n.StoreLog("[GOVERNANCE] Write Warning: child %q (role:%s) attempted to write parent %q's key %q (label:%v) — policy not satisfied, proceeding anyway\n",
			n.Metadata.Name, role, parent.Metadata.Name, key, label)
		n.governanceViolationCount++
	}

	parent.storeState(key, value)
}

func (n *Want) MergeParentState(updates map[string]any) {
	parent := n.getParentWant()
	if parent == nil {
		MergeGlobalState(updates) // fallback to globalState for top-level wants
		return
	}

	role := n.GetRole()
	engine := &GovernanceEngine{}

	for k := range updates {
		label := parent.StateLabels[k]
		if !engine.CanWriteParentState(role, label) {
			n.StoreLog("[GOVERNANCE] Merge Warning: child %q (role:%s) attempted to write %q's key %q (label:%v) — policy not satisfied, proceeding anyway\n",
				n.Metadata.Name, role, parent.Metadata.Name, k, label)
			n.governanceViolationCount++
		}
	}

	if len(updates) > 0 {
		parent.MergeState(updates)
	}
}

// GetRole returns the ChildRole assigned to this want via its labels.
func (n *Want) GetRole() ChildRole {
	if n.Metadata.Labels != nil {
		if role, ok := n.Metadata.Labels["child-role"]; ok {
			return ChildRole(role)
		}
	}
	return RoleUnknown
}

// ProposeDispatch writes a fully-resolved list of DispatchRequests to the parent
// Target's "desired_dispatch" state. The parent's DispatchExecutor reconciles this
// list against existing children and calls AddChildWant idempotently.
// Overwrites (not appends) so that OPA replanning that removes directions is respected.
func (n *Want) ProposeDispatch(requests []DispatchRequest) {
	if requests == nil {
		requests = []DispatchRequest{}
	}
	n.StoreParentState("desired_dispatch", requests)
}

// SuggestParent suggests a set of directions to the parent want (or global state).
// This is typically used by planning wants (like Itinerary) to request the
// parent's DispatchThinker to realize new child wants.
// It appends to existing directions instead of overwriting to support incremental planning.
func (n *Want) SuggestParent(directions []string) {
	parent := n.getParentWant()

	var existing []string
	var raw any
	var exists bool

	if parent != nil {
		raw, exists = parent.getState("directions")
	} else {
		raw, exists = GetGlobalState("directions")
	}

	if exists {
		if slice, ok := raw.([]string); ok {
			existing = slice
		} else if slice, ok := raw.([]any); ok {
			for _, item := range slice {
				if s, ok := item.(string); ok {
					existing = append(existing, s)
				}
			}
		}
	}

	changed := false
	for _, d := range directions {
		if !Contains(existing, d) {
			existing = append(existing, d)
			changed = true
		}
	}

	if changed || (!exists && len(directions) > 0) {
		if parent != nil {
			DebugLog("[SUGGEST] Adding %d directions to parent %s (total: %d)",
				len(directions), parent.Metadata.Name, len(existing))
			n.MergeParentState(map[string]any{"directions": existing})
		} else {
			DebugLog("[SUGGEST] Adding %d directions to global state (total: %d)",
				len(directions), len(existing))
			MergeGlobalState(map[string]any{"directions": existing})
		}
	}
}
