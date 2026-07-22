package mywant

import "testing"

// TestMigrateAuraDefaults covers the one-way rewrite of pre-target aura marks —
// keyed by want instance UUID, carrying a flat section/key pair — into
// target-keyed form. It runs against a bare characterManager rather than the
// singleton so it never touches the developer's real ~/.mywant/characters.yaml.
func newTestCharacterManager(t *testing.T, chars ...Character) *characterManager {
	t.Helper()
	return &characterManager{
		path:  t.TempDir() + "/characters.yaml",
		store: characterStoreFile{Characters: chars},
	}
}

func TestMigrateAuraDefaultsRewritesLegacyMarks(t *testing.T) {
	m := newTestCharacterManager(t, Character{
		ID:   "char-1",
		Name: "Tester",
		AuraDefaults: map[string]AuraMark{
			"want-uuid-1": {Value: "true", LegacySection: "current", LegacyKey: "going"},
		},
	})

	m.MigrateAuraDefaults(func(wantID string) (string, bool) {
		if wantID == "want-uuid-1" {
			return "going", true
		}
		return "", false
	})

	marks := m.store.Characters[0].AuraDefaults
	if len(marks) != 1 {
		t.Fatalf("expected 1 mark after migration, got %d: %+v", len(marks), marks)
	}
	want := AuraTarget{Kind: AuraTargetKindWantType, Name: "going", Path: "current/going"}
	mark, ok := marks[want.Key()]
	if !ok {
		t.Fatalf("mark not re-keyed to %q, got keys %v", want.Key(), keysOf(marks))
	}
	if mark.Target != want {
		t.Errorf("target = %+v, want %+v", mark.Target, want)
	}
	if mark.Value != "true" {
		t.Errorf("value = %q, want %q", mark.Value, "true")
	}
	if mark.Mode != AuraModeSet {
		t.Errorf("mode = %q, want %q (legacy marks were all applied)", mark.Mode, AuraModeSet)
	}
	if mark.By != "char-1" {
		t.Errorf("by = %q, want the character that held the mark", mark.By)
	}
	if mark.LegacySection != "" || mark.LegacyKey != "" {
		t.Errorf("legacy fields not cleared: %+v", mark)
	}
}

// A mark whose want is gone cannot be re-addressed to a type, so it is dropped
// rather than kept under a key that would never match anything.
func TestMigrateAuraDefaultsDropsUnresolvableMarks(t *testing.T) {
	m := newTestCharacterManager(t, Character{
		ID: "char-1",
		AuraDefaults: map[string]AuraMark{
			"deleted-want": {Value: "x", LegacySection: "current", LegacyKey: "selected"},
		},
	})

	m.MigrateAuraDefaults(func(string) (string, bool) { return "", false })

	if got := len(m.store.Characters[0].AuraDefaults); got != 0 {
		t.Fatalf("expected unresolvable mark to be dropped, %d left", got)
	}
}

// Migration must be safe to run on every startup: already-target-keyed marks
// are left exactly as they are, and the resolver is never consulted for them.
func TestMigrateAuraDefaultsIsIdempotent(t *testing.T) {
	target := AuraTarget{Kind: AuraTargetKindWantType, Name: "choice", Path: "current/selected"}
	m := newTestCharacterManager(t, Character{
		ID: "char-1",
		AuraDefaults: map[string]AuraMark{
			target.Key(): {Target: target, Value: "premium", Mode: AuraModeSet, By: "char-1"},
		},
	})

	m.MigrateAuraDefaults(func(string) (string, bool) {
		t.Fatal("resolver called for an already-migrated mark")
		return "", false
	})

	mark, ok := m.store.Characters[0].AuraDefaults[target.Key()]
	if !ok || mark.Value != "premium" || mark.Target != target {
		t.Fatalf("migrated mark was altered: %+v", m.store.Characters[0].AuraDefaults)
	}
}

// SectionKey splits only on the first "/" so a path may itself contain slashes
// once a future target kind needs nested addressing.
func TestAuraTargetSectionKey(t *testing.T) {
	cases := []struct{ path, section, key string }{
		{"current/going", "current", "going"},
		{"parameter/arrive_by", "parameter", "arrive_by"},
		{"current/legs/2/carrier", "current", "legs/2/carrier"},
		{"current", "current", ""},
	}
	for _, c := range cases {
		section, key := AuraTarget{Path: c.path}.SectionKey()
		if section != c.section || key != c.key {
			t.Errorf("SectionKey(%q) = (%q, %q), want (%q, %q)", c.path, section, key, c.section, c.key)
		}
	}
}

// A definition mark carries a structured object as its value and is addressed
// by a catalog target (empty path = the whole object). It is stored and read by
// name, and never treated as a binding.
func TestAuraDefinitionMarkStoresAndResolves(t *testing.T) {
	m := newTestCharacterManager(t, Character{ID: "char-1", Name: "Tester"})

	office := map[string]any{"lat": 35.68, "lng": 139.76, "radius": 120.0}
	_, ok := m.SetAuraDefault("char-1", AuraMark{
		Target: AuraTarget{Kind: "place", Name: "会社"},
		Value:  office,
	})
	if !ok {
		t.Fatal("SetAuraDefault returned not-ok for a valid definition mark")
	}

	got, ok := m.ResolveAuraDefinition("place", "会社", "")
	if !ok {
		t.Fatal("ResolveAuraDefinition did not find the place")
	}
	obj, isMap := got.(map[string]any)
	if !isMap || obj["radius"] != 120.0 {
		t.Fatalf("definition value not preserved: %#v", got)
	}

	// A definition target is not a binding, so it must never leak into the
	// per-want-type apply lookup.
	stored := m.store.Characters[0].AuraDefaults[AuraTarget{Kind: "place", Name: "会社"}.Key()]
	if stored.Target.IsBinding() {
		t.Error("place definition mark wrongly classified as a binding")
	}
	if stored.Mode != "" {
		t.Errorf("definition mark should carry no mode, got %q", stored.Mode)
	}

	// AuraDefinitions lists it under its entry name.
	places := m.AuraDefinitions("place")
	if _, ok := places["会社"]; !ok || len(places) != 1 {
		t.Fatalf("AuraDefinitions(place) = %v, want just 会社", keysOf(places))
	}
}

// The widening of Value to `any` must not change binding behaviour: a scalar
// value round-trips, and clearing still works via an empty value.
func TestAuraBindingValueRoundTripsAndClears(t *testing.T) {
	m := newTestCharacterManager(t, Character{ID: "char-1"})
	target := AuraTarget{Kind: AuraTargetKindWantType, Name: "going", Path: "current/going"}

	m.SetAuraDefault("char-1", AuraMark{Target: target, Value: "true"})
	mark := m.store.Characters[0].AuraDefaults[target.Key()]
	if mark.Value != "true" {
		t.Fatalf("binding value = %#v, want \"true\"", mark.Value)
	}
	if mark.Mode != AuraModeSet {
		t.Errorf("binding defaulted to mode %q, want %q", mark.Mode, AuraModeSet)
	}

	m.SetAuraDefault("char-1", AuraMark{Target: target, Value: ""})
	if _, ok := m.store.Characters[0].AuraDefaults[target.Key()]; ok {
		t.Error("empty value did not clear the binding mark")
	}
}

func keysOf(m map[string]AuraMark) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
