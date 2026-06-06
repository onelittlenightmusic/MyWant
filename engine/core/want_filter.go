package mywant

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"slices"
)

// want_filter.go — want filtering, hash, and dot-notation field lookup utilities

// WantFilters contains filter criteria for want list queries
type WantFilters struct {
	Type         string            // Filter by want type
	Labels       map[string]string // Filter by labels (key=value pairs, AND logic)
	UsingFilters map[string]string // Filter by using selectors (key=value pairs, AND logic)
}

// MatchesFilters checks if a want matches all specified filters
// Returns true if the want passes all filters (AND logic)
func (w *Want) MatchesFilters(filters WantFilters) bool {
	if filters.Type != "" && w.Metadata.Type != filters.Type {
		return false
	}

	if len(filters.Labels) > 0 {
		for key, value := range filters.Labels {
			if w.Metadata.Labels == nil {
				return false
			}
			labelValue, exists := w.Metadata.Labels[key]
			if !exists || labelValue != value {
				return false
			}
		}
	}

	if len(filters.UsingFilters) > 0 {
		for key, value := range filters.UsingFilters {
			if len(w.Spec.Using) == 0 {
				return false
			}
			found := false
			for _, usingEntry := range w.Spec.Using {
				if usingValue, exists := usingEntry.Labels[key]; exists && usingValue == value {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
	}

	return true
}

// FilterWants filters a list of wants based on the provided filters
func FilterWants(wants []*Want, filters WantFilters) []*Want {
	filtered := make([]*Want, 0, len(wants))
	for _, want := range wants {
		if want.MatchesFilters(filters) {
			filtered = append(filtered, want)
		}
	}
	return filtered
}

// CalculateWantHash computes a hash of want's metadata, spec, all state fields, and status
// This hash is used for change detection to avoid unnecessary frontend re-renders
func CalculateWantHash(w *Want) string {
	hashData := struct {
		Metadata Metadata       `json:"metadata"`
		Spec     WantSpec       `json:"spec"`
		Status   WantStatus     `json:"status"`
		State    map[string]any `json:"state"`
	}{
		Metadata: w.Metadata,
		Spec:     w.Spec,
		Status:   w.Status,
		State:    w.GetAllState(),
	}

	jsonData, err := json.Marshal(hashData)
	if err != nil {
		return ""
	}

	hash := sha256.Sum256(jsonData)
	return hex.EncodeToString(hash[:])
}

// resolveNestedStateField resolves a dot-notation field path from Want state.
// For example, "slack_latest_message.text" first fetches the "slack_latest_message"
// state key (expected to be a map) and then navigates to the "text" sub-field.
// A plain key with no dots falls back to a normal getState call.
func resolveNestedStateField(n *Want, field string) (any, bool) {
	parts := splitFirst(field, '.')
	top, ok := n.getState(parts[0])
	if !ok || top == nil {
		return nil, false
	}
	if len(parts) == 1 {
		return top, true
	}
	cur := top
	for _, part := range parts[1:] {
		switch m := cur.(type) {
		case map[string]any:
			v, exists := m[part]
			if !exists {
				return nil, false
			}
			cur = v
		case map[any]any:
			v, exists := m[part]
			if !exists {
				return nil, false
			}
			cur = v
		default:
			return nil, false
		}
	}
	return cur, true
}

// splitFirst splits s on the first occurrence of sep and returns all parts.
func splitFirst(s string, sep byte) []string {
	for i := 0; i < len(s); i++ {
		if s[i] == sep {
			rest := s[i+1:]
			return append([]string{s[:i]}, splitFirst(rest, sep)...)
		}
	}
	return []string{s}
}

func Contains(slice []string, item string) bool {
	return slices.Contains(slice, item)
}

// syncLocalsAfterInitialize persists the locals to internal state immediately after
// Initialize() is called. Without this, the next SyncLocalsState(toStruct=true) would
// read stale internal state (e.g., old _phase = "completed") and overwrite the
// freshly initialized locals.Phase back to the old value, causing the want to
// skip re-execution on restart.
func (n *Want) syncLocalsAfterInitialize() {
	if n.progressable == nil {
		return
	}
	progressableVal := reflect.ValueOf(n.progressable)
	if progressableVal.Kind() != reflect.Pointer {
		return
	}
	method := progressableVal.MethodByName("GetLocals")
	if !method.IsValid() || method.Type().NumIn() != 0 || method.Type().NumOut() != 1 {
		return
	}
	results := method.Call(nil)
	locals := results[0].Interface()
	SyncLocalsState(n, locals, false)
}
