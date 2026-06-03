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

	// Compare spec
	if !reflect.DeepEqual(a.Spec.Params, b.Spec.Params) {
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
