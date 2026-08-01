package mywant

import (
	"strings"
	"testing"
)

func TestGenerateFirstOrderKey(t *testing.T) {
	key := GenerateFirstOrderKey()
	if key != "a0" {
		t.Errorf("Expected 'a0', got '%s'", key)
	}
}

func TestGenerateOrderKeyAfter(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", "a0"},
		{"a0", "a1"},
		{"a1", "a2"},
		{"az", "b0"},
		{"a0M", "a0N"},
	}

	for _, tt := range tests {
		result := GenerateOrderKeyAfter(tt.input)
		if result != tt.expected {
			t.Errorf("GenerateOrderKeyAfter('%s') = '%s', expected '%s'", tt.input, result, tt.expected)
		}
	}
}

func TestGenerateOrderKeyBefore(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"a1", "a0"},
		{"a2", "a1"},
		{"b0", "az"},
		{"a0N", "a0M"},
	}

	for _, tt := range tests {
		result := GenerateOrderKeyBefore(tt.input)
		if result != tt.expected {
			t.Errorf("GenerateOrderKeyBefore('%s') = '%s', expected '%s'", tt.input, result, tt.expected)
		}
	}
}

func TestGenerateOrderKeyBetween(t *testing.T) {
	tests := []struct {
		keyA     string
		keyB     string
		validate func(result, keyA, keyB string) bool
	}{
		{"", "", func(r, a, b string) bool { return r == "a0" }},
		{"a0", "", func(r, a, b string) bool { return r > a }},
		{"", "a1", func(r, a, b string) bool { return r < b }},
		{"a0", "a2", func(r, a, b string) bool { return r > a && r < b }},
		{"a0", "a1", func(r, a, b string) bool { return r > a && r < b }},
	}

	for _, tt := range tests {
		result := GenerateOrderKeyBetween(tt.keyA, tt.keyB)
		if !tt.validate(result, tt.keyA, tt.keyB) {
			t.Errorf("GenerateOrderKeyBetween('%s', '%s') = '%s' failed validation", tt.keyA, tt.keyB, result)
		}

		// Verify lexicographic ordering
		if tt.keyA != "" && strings.Compare(result, tt.keyA) <= 0 {
			t.Errorf("Result '%s' should be greater than keyA '%s'", result, tt.keyA)
		}
		if tt.keyB != "" && strings.Compare(result, tt.keyB) >= 0 {
			t.Errorf("Result '%s' should be less than keyB '%s'", result, tt.keyB)
		}
	}
}

func TestGenerateSequentialOrderKeys(t *testing.T) {
	count := 5
	keys := GenerateSequentialOrderKeys(count, "")

	if len(keys) != count {
		t.Errorf("Expected %d keys, got %d", count, len(keys))
	}

	// Verify all keys are in ascending order
	for i := 1; i < len(keys); i++ {
		if strings.Compare(keys[i-1], keys[i]) >= 0 {
			t.Errorf("Keys not in ascending order: '%s' >= '%s'", keys[i-1], keys[i])
		}
	}

	// Verify starting from a specific key
	keys2 := GenerateSequentialOrderKeys(3, "a5")
	if keys2[0] != "a5" {
		t.Errorf("Expected first key to be 'a5', got '%s'", keys2[0])
	}
}

func TestAssignOrderKeys(t *testing.T) {
	// Create wants without order keys
	wants := []*Want{
		{Metadata: Metadata{Name: "want1"}},
		{Metadata: Metadata{Name: "want2"}},
		{Metadata: Metadata{Name: "want3"}},
	}

	assigned := AssignOrderKeys(wants)
	if assigned != 3 {
		t.Errorf("Expected 3 wants to be assigned keys, got %d", assigned)
	}

	// Verify all wants have keys
	for i, want := range wants {
		if want.Metadata.OrderKey == "" {
			t.Errorf("Want %d has no order key", i)
		}
	}

	// Verify keys are in ascending order
	for i := 1; i < len(wants); i++ {
		if strings.Compare(wants[i-1].Metadata.OrderKey, wants[i].Metadata.OrderKey) >= 0 {
			t.Errorf("Order keys not in ascending order at index %d", i)
		}
	}

	// Test with some wants already having keys
	wants2 := []*Want{
		{Metadata: Metadata{Name: "want1", OrderKey: "a0"}},
		{Metadata: Metadata{Name: "want2"}},
		{Metadata: Metadata{Name: "want3", OrderKey: "a2"}},
		{Metadata: Metadata{Name: "want4"}},
	}

	assigned2 := AssignOrderKeys(wants2)
	if assigned2 != 2 {
		t.Errorf("Expected 2 wants to be assigned keys, got %d", assigned2)
	}

	// Verify the new keys are after the last key
	if wants2[1].Metadata.OrderKey == "" || wants2[3].Metadata.OrderKey == "" {
		t.Error("New wants should have been assigned order keys")
	}
}

func TestSortWantsByOrderKey(t *testing.T) {
	wants := []*Want{
		{Metadata: Metadata{Name: "want3", OrderKey: "a2"}},
		{Metadata: Metadata{Name: "want1", OrderKey: "a0"}},
		{Metadata: Metadata{Name: "want2", OrderKey: "a1"}},
	}

	SortWantsByOrderKey(wants)

	expectedNames := []string{"want1", "want2", "want3"}
	for i, want := range wants {
		if want.Metadata.Name != expectedNames[i] {
			t.Errorf("At index %d, expected '%s', got '%s'", i, expectedNames[i], want.Metadata.Name)
		}
	}
}

func TestValidateOrderKey(t *testing.T) {
	validKeys := []string{"", "a0", "a1", "b0", "xyz123"}
	for _, key := range validKeys {
		if err := ValidateOrderKey(key); err != nil {
			t.Errorf("Expected key '%s' to be valid, got error: %v", key, err)
		}
	}

	invalidKeys := []string{"!", "a!", "a b", "a-1"}
	for _, key := range invalidKeys {
		if err := ValidateOrderKey(key); err == nil {
			t.Errorf("Expected key '%s' to be invalid, but no error was returned", key)
		}
	}
}

// GenerateOrderKeyBetween must always land strictly inside the interval it is
// given. The prefix cases below are the ones that used to escape it: when keyB
// extended keyA with a minimum character ("zz" / "zz0"), the old split returned
// "zzV", which sorts AFTER keyB — the want ended up on the wrong side of the
// neighbour it was dropped next to.
func TestGenerateOrderKeyBetweenStaysInInterval(t *testing.T) {
	pairs := [][2]string{
		{"a0", "a1"},   // adjacent characters
		{"a0", "a0V"},  // keyB extends keyA
		{"a0V", "a1"},  // deeper key against a shallow one
		{"a0V", "a0W"}, // adjacent at depth
		{"zz", "zz0V"}, // keyB extends keyA past a minimum character
		{"a0", "a00V"}, // several minimum characters before room appears
		{"az", "b0"},   // carry boundary
		{"a9", "aA"},   // digit/uppercase boundary
		{"aZ", "aa"},   // uppercase/lowercase boundary
	}

	for _, p := range pairs {
		keyA, keyB := p[0], p[1]
		got := GenerateOrderKeyBetween(keyA, keyB)
		if !(got > keyA && got < keyB) {
			t.Errorf("GenerateOrderKeyBetween(%q, %q) = %q, want strictly between", keyA, keyB, got)
		}
	}
}

// Repeatedly inserting into the same gap must keep producing keys inside the
// ever-shrinking interval — this is the operation a user performs by dragging a
// card into the same slot over and over.
func TestGenerateOrderKeyBetweenRepeatedInsertion(t *testing.T) {
	lo, hi := "a0", "a1"
	for i := 0; i < 50; i++ {
		mid := GenerateOrderKeyBetween(lo, hi)
		if !(mid > lo && mid < hi) {
			t.Fatalf("iteration %d: GenerateOrderKeyBetween(%q, %q) = %q, want strictly between", i, lo, hi, mid)
		}
		// Alternate which side we keep, so both the "extend" and "adjacent"
		// branches get exercised as the interval narrows.
		if i%2 == 0 {
			lo = mid
		} else {
			hi = mid
		}
	}
}

func TestAssignOrderKeysUsesMaximumExistingKey(t *testing.T) {
	// Deliberately unsorted: the highest key is not the last one in the slice.
	wants := []*Want{
		{Metadata: Metadata{Name: "high", OrderKey: "a9"}},
		{Metadata: Metadata{Name: "new"}},
		{Metadata: Metadata{Name: "low", OrderKey: "a0"}},
	}

	if assigned := AssignOrderKeys(wants); assigned != 1 {
		t.Fatalf("expected 1 want to be assigned a key, got %d", assigned)
	}

	got := wants[1].Metadata.OrderKey
	if got <= "a9" {
		t.Errorf("new want got key %q, expected it to sort after the maximum existing key %q", got, "a9")
	}
}

func TestBackfillMissingOrderKeys(t *testing.T) {
	wants := []*Want{
		{Metadata: Metadata{ID: "1", Name: "has-key", OrderKey: "a0"}},
		{Metadata: Metadata{ID: "2", Name: "no-key"}},
		{Metadata: Metadata{ID: "3", Name: "also-no-key"}},
	}

	healed := BackfillMissingOrderKeys(wants)
	if len(healed) != 2 {
		t.Fatalf("expected 2 healed wants, got %d", len(healed))
	}
	for _, w := range wants {
		if w.Metadata.OrderKey == "" {
			t.Errorf("want %q still has no order key", w.Metadata.Name)
		}
	}
	// Nothing left to do the second time around.
	if healed := BackfillMissingOrderKeys(wants); healed != nil {
		t.Errorf("expected no wants to need healing on a second pass, got %d", len(healed))
	}
}

// A keyless want must land in a defined place rather than wherever the input
// permutation happens to put it — the client sorts the same list independently,
// and the two orders have to agree.
func TestSortWantsByOrderKeyIsTotal(t *testing.T) {
	build := func() []*Want {
		return []*Want{
			{Metadata: Metadata{ID: "c", OrderKey: "a2"}},
			{Metadata: Metadata{ID: "keyless"}},
			{Metadata: Metadata{ID: "a", OrderKey: "a0"}},
			{Metadata: Metadata{ID: "b", OrderKey: "a1"}},
		}
	}

	first := build()
	SortWantsByOrderKey(first)

	// Same set, different starting permutation → same result.
	second := build()
	second[0], second[3] = second[3], second[0]
	SortWantsByOrderKey(second)

	for i := range first {
		if first[i].Metadata.ID != second[i].Metadata.ID {
			t.Fatalf("sort is permutation-dependent: got %q vs %q at index %d",
				first[i].Metadata.ID, second[i].Metadata.ID, i)
		}
	}

	wantOrder := []string{"a", "b", "c", "keyless"}
	for i, id := range wantOrder {
		if first[i].Metadata.ID != id {
			t.Errorf("at index %d expected %q, got %q", i, id, first[i].Metadata.ID)
		}
	}
}

// Benchmark for key generation
func BenchmarkGenerateOrderKeyBetween(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = GenerateOrderKeyBetween("a0", "a1")
	}
}

func BenchmarkGenerateSequentialOrderKeys(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = GenerateSequentialOrderKeys(100, "")
	}
}
