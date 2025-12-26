package mywant

import (
	"testing"
)

func TestRecipeParameterSubstitution(t *testing.T) {
	loader := &RecipeLoader{}

	params := map[string]any{
		"count": 100,
		"rate":  10.5,
		"name":  "test-prefix",
	}

	spec := map[string]any{
		"count":       "count",
		"rate":        "rate",
		"custom_name": "name",
		"fixed":       "literal_value",
	}

	resolved := loader.resolveParams(spec, params)

	if resolved["count"] != 100 {
		t.Errorf("Expected count 100, got %v", resolved["count"])
	}
	if resolved["rate"] != 10.5 {
		t.Errorf("Expected rate 10.5, got %v", resolved["rate"])
	}
	if resolved["custom_name"] != "test-prefix" {
		t.Errorf("Expected custom_name 'test-prefix', got %v", resolved["custom_name"])
	}
	if resolved["fixed"] != "literal_value" {
		t.Error("Fixed values should remain unchanged")
	}
}

func TestDRYWantInstantiation(t *testing.T) {
	loader := &RecipeLoader{}

	dryWant := DRYWantSpec{
		Type: "sequence",
		Labels: map[string]string{
			"role": "generator",
		},
		Params: map[string]any{
			"count": "count",
			"rate":  "rate",
		},
	}

	params := map[string]any{
		"count": 1000,
		"rate":  5.0,
	}

	want, err := loader.instantiateDRYWant(dryWant, nil, params, "test", 1)
	if err != nil {
		t.Fatalf("Failed to instantiate DRY want: %v", err)
	}

	// Verify generated name
	expectedName := "test-sequence-1"
	if want.Metadata.Name != expectedName {
		t.Errorf("Expected name %s, got %s", expectedName, want.Metadata.Name)
	}

	// Verify type
	if want.Metadata.Type != "sequence" {
		t.Errorf("Expected type 'sequence', got %s", want.Metadata.Type)
	}

	// Verify labels
	if want.Metadata.Labels["role"] != "generator" {
		t.Error("Expected role label to be 'generator'")
	}

	// Verify resolved parameters
	if want.Spec.Params["count"] != 1000 {
		t.Error("Count parameter not properly resolved")
	}
	if want.Spec.Params["rate"] != 5.0 {
		t.Error("Rate parameter not properly resolved")
	}

	// Verify owner references
	if len(want.Metadata.OwnerReferences) == 0 {
		t.Error("Expected owner references to be set")
	}
	if want.Metadata.OwnerReferences[0].Name != "test" {
		t.Error("Owner reference name not set correctly")
	}
}

func TestRecipeWantCreation(t *testing.T) {
	wantRecipe := WantRecipe{
		Metadata: struct {
			Name   string            `yaml:"name"`
			Type   string            `yaml:"type"`
			Labels map[string]string `yaml:"labels"`
		}{
			Name:   "test-want",
			Type:   "queue",
			Labels: map[string]string{"category": "processor"},
		},
		Spec: struct {
			Params map[string]any      `yaml:"params"`
			Using  []map[string]string `yaml:"using,omitempty"`
		}{
			Params: map[string]any{
				"service_time": "service_time",
			},
		},
	}

	loader := &RecipeLoader{}
	params := map[string]any{
		"service_time": 0.1,
	}

	want, err := loader.instantiateWantFromTemplate(wantRecipe, params, "parent")
	if err != nil {
		t.Fatalf("Failed to instantiate want from template: %v", err)
	}

	if want.Metadata.Name != "test-want" {
		t.Error("Want name not preserved")
	}
	if want.Metadata.Type != "queue" {
		t.Error("Want type not preserved")
	}
	if want.Spec.Params["service_time"] != 0.1 {
		t.Error("Parameter not resolved correctly")
	}
}

func TestRecipeDefaults(t *testing.T) {
	loader := &RecipeLoader{}

	dryWant := DRYWantSpec{
		Type:   "queue",
		Params: map[string]any{},
	}

	defaults := &DRYRecipeDefaults{
		Metadata: struct {
			Labels map[string]string `yaml:"labels,omitempty"`
		}{
			Labels: map[string]string{
				"default_label": "default_value",
			},
		},
		Spec: struct {
			Params map[string]any `yaml:"params,omitempty"`
		}{
			Params: map[string]any{
				"default_param": 42,
			},
		},
	}

	params := map[string]any{}

	want, err := loader.instantiateDRYWant(dryWant, defaults, params, "test", 1)
	if err != nil {
		t.Fatalf("Failed to instantiate DRY want with defaults: %v", err)
	}

	// Verify default labels are applied
	if want.Metadata.Labels["default_label"] != "default_value" {
		t.Error("Default label not applied")
	}

	// Note: Default params are applied through mergeDRYDefaults function which is tested separately
}

func TestWantMatchingLogic(t *testing.T) {
	loader := &RecipeLoader{}

	want := &Want{
		Metadata: Metadata{
			Name: "test-queue-1",
			Type: "queue",
			Labels: map[string]string{
				"role": "processor",
			},
		},
	}

	// Test exact name match
	if !loader.matchesResultWant(want, "test-queue-1", "parent") {
		t.Error("Should match exact name")
	}

	// Test type match with prefix
	if !loader.matchesResultWant(want, "queue", "parent") {
		t.Error("Should match type")
	}

	// Test no match
	if loader.matchesResultWant(want, "different", "parent") {
		t.Error("Should not match different selector")
	}
}

func TestEmptyRecipeHandling(t *testing.T) {
	loader := &RecipeLoader{}

	// Test with empty wants list
	wants, err := loader.InstantiateRecipe("nonexistent", "test", map[string]any{})
	if err == nil {
		t.Error("Expected error for nonexistent recipe")
	}
	if wants != nil {
		t.Error("Expected nil wants for failed recipe load")
	}
}

func TestParameterValidation(t *testing.T) {
	loader := &RecipeLoader{}

	// Test nil parameters map
	resolved := loader.resolveParams(nil, map[string]any{})
	if resolved == nil {
		t.Error("Should return empty map for nil input")
	}

	// Test empty parameters
	resolved = loader.resolveParams(map[string]any{}, map[string]any{})
	if len(resolved) != 0 {
		t.Error("Should return empty map for empty input")
	}
}

func TestOwnerReferenceGeneration(t *testing.T) {
	loader := &RecipeLoader{}

	dryWant := DRYWantSpec{
		Type: "test",
	}

	want, err := loader.instantiateDRYWant(dryWant, nil, map[string]any{}, "parent", 1)
	if err != nil {
		t.Fatalf("Failed to instantiate want: %v", err)
	}

	if len(want.Metadata.OwnerReferences) == 0 {
		t.Error("Expected owner references to be generated")
	}

	ownerRef := want.Metadata.OwnerReferences[0]
	if ownerRef.Name != "parent" {
		t.Error("Owner reference name incorrect")
	}
	if ownerRef.APIVersion != "mywant/v1" {
		t.Error("Owner reference API version incorrect")
	}
	if ownerRef.Kind != "Want" {
		t.Error("Owner reference kind incorrect")
	}
	if !ownerRef.Controller {
		t.Error("Owner reference should be controller")
	}
}
