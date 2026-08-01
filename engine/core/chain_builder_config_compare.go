package mywant

import (
	"reflect"

	ws "github.com/onelittlenightmusic/want-spec"
)

// wantsEqual compares two wants for equality.
// Only spec fields (params, using, when) and type trigger a restart.
// Label changes are metadata-only and must NOT cause a want restart.
func (cb *ChainBuilder) wantsEqual(a, b *Want) bool {
	// Compare metadata
	if a.Metadata.Type != b.Metadata.Type {
		return false
	}

	// Labels are metadata-only (e.g. canvas position, UI hints) — do NOT compare them.

	if !reflect.DeepEqual(a.Metadata.OwnerReferences, b.Metadata.OwnerReferences) {
		return false
	}

	// Compare spec. Params are compared by their EFFECTIVE values: a param
	// declared as {fromGlobalParam: key} keeps that declaration in Spec.Params
	// for good reason (it must survive persistence as a reference), so two
	// snapshots of a want wired to another want's value look identical even
	// after that value moves. Resolving here is what lets reconcile see the
	// change and restart the want, the same as any other config change.
	if !reflect.DeepEqual(resolveParamRefs(a.Spec.Params), resolveParamRefs(b.Spec.Params)) {
		return false
	}

	if !reflect.DeepEqual(a.Spec.Using, b.Spec.Using) {
		return false
	}

	if !reflect.DeepEqual(a.Spec.When, b.Spec.When) {
		return false
	}

	if !reflect.DeepEqual(a.Spec.Exposes, b.Spec.Exposes) {
		return false
	}

	if !reflect.DeepEqual(a.Spec.Imports, b.Spec.Imports) {
		return false
	}

	return true
}

// hasParamRef reports whether any configured want has a parameter declared as
// {fromGlobalParam: key}.
func (cb *ChainBuilder) hasParamRef(key string) bool {
	cb.wantsMu.RLock()
	defer cb.wantsMu.RUnlock()
	for _, w := range cb.config {
		for _, v := range w.Spec.Params {
			if paramRefKey(v) == key {
				return true
			}
		}
	}
	return false
}

// paramRefKey returns the global parameter a value references, or "" if the
// value is a plain one. Persistence can bring the map back keyed by `any`.
func paramRefKey(v any) string {
	switch m := v.(type) {
	case map[string]any:
		k, _ := m["fromGlobalParam"].(string)
		return k
	case map[string]string:
		return m["fromGlobalParam"]
	case map[any]any:
		for mk, mv := range m {
			if ks, ok := mk.(string); ok && ks == "fromGlobalParam" {
				k, _ := mv.(string)
				return k
			}
		}
	}
	return ""
}

// resolveParamRefs returns params with every {fromGlobalParam: key} replaced by
// the value that key currently holds. An unresolved reference keeps its
// declaration, so a want waiting on a parameter that does not exist yet stays
// equal to itself instead of restarting on every reconcile.
func resolveParamRefs(params map[string]any) map[string]any {
	if params == nil {
		return nil
	}
	out := make(map[string]any, len(params))
	for k, v := range params {
		if key := paramRefKey(v); key != "" {
			if resolved, ok := GetGlobalParameter(key); ok {
				out[k] = resolved
				continue
			}
		}
		out[k] = v
	}
	return out
}

// mapsEqual compares two string maps for equality
func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}

	for k, v := range a {
		if b[k] != v {
			return false
		}
	}

	return true
}

// deepCopyWants creates a deep copy of a []*Want slice to prevent reference aliasing.
// This is critical for change detection to work correctly.
func (cb *ChainBuilder) deepCopyWants(src []*Want) []*Want {
	copiedWants := make([]*Want, 0, len(src))
	for _, want := range src {
		// Deep copy the want
		copiedWant := &Want{
			Metadata: Metadata{
				ID:              want.Metadata.ID,
				Name:            want.Metadata.Name,
				Type:            want.Metadata.Type,
				Labels:          want.GetLabels(),
				OwnerReferences: copyOwnerReferences(want.Metadata.OwnerReferences),
				OrderKey:        want.Metadata.OrderKey,
			},
			Spec: WantSpec{
				Params:              copyInterfaceMap(want.Spec.Params),
				Using:               copyUsing(want.Spec.Using),
				StateSubscriptions:  copyStateSubscriptions(want.Spec.StateSubscriptions),
				NotificationFilters: copyNotificationFilters(want.Spec.NotificationFilters),
				Requires:            copyStringSlice(want.Spec.Requires),
				When:                copyWhen(want.Spec.When),
				Exposes:             copyExposes(want.Spec.Exposes),
				Imports:             copyStringMap(want.Spec.Imports),
			},
		}
		copiedWants = append(copiedWants, copiedWant)
	}

	return copiedWants
}

// Helper functions for deep copying
func copyWhen(src []WhenSpec) []WhenSpec {
	if src == nil {
		return nil
	}
	dst := make([]WhenSpec, len(src))
	copy(dst, src)
	return dst
}

func copyStringMap(src map[string]string) map[string]string {
	if src == nil {
		return nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func copyInterfaceMap(src map[string]any) map[string]any {
	if src == nil {
		return nil
	}
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func copyUsing(src []ws.UsingEntry) []ws.UsingEntry {
	if src == nil {
		return nil
	}
	dst := make([]ws.UsingEntry, 0, len(src))
	for _, entry := range src {
		copied := ws.UsingEntry{Labels: copyStringMap(entry.Labels)}
		if entry.When != nil {
			c := *entry.When
			copied.When = &c
		}
		dst = append(dst, copied)
	}
	return dst
}

func copyStringSlice(src []string) []string {
	if src == nil {
		return nil
	}
	dst := make([]string, len(src))
	copy(dst, src)
	return dst
}

func copyOwnerReferences(src []OwnerReference) []OwnerReference {
	if src == nil {
		return nil
	}
	dst := make([]OwnerReference, len(src))
	copy(dst, src)
	return dst
}

func copyStateSubscriptions(src []StateSubscription) []StateSubscription {
	if src == nil {
		return nil
	}
	dst := make([]StateSubscription, 0, len(src))
	for _, sub := range src {
		copiedSub := StateSubscription{
			WantName:   sub.WantName,
			StateKeys:  copyStringSlice(sub.StateKeys),
			Conditions: copyStringSlice(sub.Conditions),
			BufferSize: sub.BufferSize,
		}
		dst = append(dst, copiedSub)
	}
	return dst
}

func copyNotificationFilters(src []NotificationFilter) []NotificationFilter {
	if src == nil {
		return nil
	}
	dst := make([]NotificationFilter, 0, len(src))
	for _, filter := range src {
		copiedFilter := NotificationFilter{
			SourcePattern: filter.SourcePattern,
			StateKeys:     copyStringSlice(filter.StateKeys),
			ValuePattern:  filter.ValuePattern,
		}
		dst = append(dst, copiedFilter)
	}
	return dst
}

func copyExposes(src []ExposeEntry) []ExposeEntry {
	if src == nil {
		return nil
	}
	dst := make([]ExposeEntry, len(src))
	copy(dst, src)
	return dst
}
