package mywant

import (
	"os"
	"path/filepath"
	"testing"
)

// helpers

func makeWantWithParams(params map[string]any) *Want {
	w := &Want{
		Metadata: Metadata{Name: "test", Type: "test"},
	}
	if params != nil {
		w.Spec.SetParamsFromMap(params)
	}
	return w
}

func makeTypeDef(params []ParameterDef) *WantTypeDefinition {
	return &WantTypeDefinition{
		Metadata: WantTypeMetadata{
			Name: "test", Title: "Test", Description: "test",
			Version: "1.0", Category: "test", Pattern: "independent",
		},
		Parameters: params,
	}
}

// --- ParameterDef.Default tests ---

func TestSetWantTypeDefinition_DefaultApplied(t *testing.T) {
	resetGlobalParams()
	defer resetGlobalParams()

	w := makeWantWithParams(nil)
	typeDef := makeTypeDef([]ParameterDef{
		{Name: "timeout", Type: "int", Description: "d", Required: false, Default: 30},
	})
	w.SetWantTypeDefinition(typeDef)

	v, ok := w.Spec.GetParam("timeout")
	if !ok {
		t.Fatal("Expected default to be applied")
	}
	if v != 30 {
		t.Errorf("Expected 30, got %v", v)
	}
}

func TestSetWantTypeDefinition_SpecParamsOverridesDefault(t *testing.T) {
	resetGlobalParams()
	defer resetGlobalParams()

	w := makeWantWithParams(map[string]any{"timeout": 99})
	typeDef := makeTypeDef([]ParameterDef{
		{Name: "timeout", Type: "int", Description: "d", Required: false, Default: 30},
	})
	w.SetWantTypeDefinition(typeDef)

	v, _ := w.Spec.GetParam("timeout")
	if v != 99 {
		t.Errorf("spec.params should take precedence over default, got %v", v)
	}
}

func TestSetWantTypeDefinition_NoDefaultNoGlobal(t *testing.T) {
	resetGlobalParams()
	defer resetGlobalParams()

	w := makeWantWithParams(nil)
	typeDef := makeTypeDef([]ParameterDef{
		{Name: "host", Type: "string", Description: "d", Required: false},
	})
	w.SetWantTypeDefinition(typeDef)

	if w.Spec.HasParam("host") {
		t.Error("No default and no global parameter — key should not be set")
	}
}

// --- DefaultGlobalParameter tests ---

func TestSetWantTypeDefinition_DefaultGlobalParameter_Used(t *testing.T) {
	resetGlobalParams()
	defer resetGlobalParams()

	// Load a global parameter
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "parameters.yaml")
	os.WriteFile(path, []byte("api_host: global-host\n"), 0644)
	LoadGlobalParameters(path)

	w := makeWantWithParams(nil)
	typeDef := makeTypeDef([]ParameterDef{
		{Name: "host", Type: "string", Description: "d", Required: false,
			DefaultGlobalParameter: "api_host"},
	})
	w.SetWantTypeDefinition(typeDef)

	v, ok := w.Spec.GetParam("host")
	if !ok {
		t.Fatal("Expected global parameter fallback to be applied")
	}
	if v != "global-host" {
		t.Errorf("Expected 'global-host', got %v", v)
	}
}

func TestSetWantTypeDefinition_DefaultGlobalParameter_MissingKey(t *testing.T) {
	resetGlobalParams()
	defer resetGlobalParams()

	// Global parameters loaded but key is absent
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "parameters.yaml")
	os.WriteFile(path, []byte("other_key: value\n"), 0644)
	LoadGlobalParameters(path)

	w := makeWantWithParams(nil)
	typeDef := makeTypeDef([]ParameterDef{
		{Name: "host", Type: "string", Description: "d", Required: false,
			DefaultGlobalParameter: "api_host"},
	})
	w.SetWantTypeDefinition(typeDef)

	if w.Spec.HasParam("host") {
		t.Error("Global parameter key is absent — should not set param")
	}
}

func TestSetWantTypeDefinition_SpecParamsOverridesDefaultGlobalParameter(t *testing.T) {
	resetGlobalParams()
	defer resetGlobalParams()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "parameters.yaml")
	os.WriteFile(path, []byte("api_host: global-host\n"), 0644)
	LoadGlobalParameters(path)

	w := makeWantWithParams(map[string]any{"host": "explicit-host"})
	typeDef := makeTypeDef([]ParameterDef{
		{Name: "host", Type: "string", Description: "d", Required: false,
			DefaultGlobalParameter: "api_host"},
	})
	w.SetWantTypeDefinition(typeDef)

	v, _ := w.Spec.GetParam("host")
	if v != "explicit-host" {
		t.Errorf("spec.params should override defaultGlobalParameter, got %v", v)
	}
}

func TestSetWantTypeDefinition_DefaultOverridesDefaultGlobalParameter(t *testing.T) {
	resetGlobalParams()
	defer resetGlobalParams()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "parameters.yaml")
	os.WriteFile(path, []byte("api_host: global-host\n"), 0644)
	LoadGlobalParameters(path)

	w := makeWantWithParams(nil)
	typeDef := makeTypeDef([]ParameterDef{
		{Name: "host", Type: "string", Description: "d", Required: false,
			Default: "yaml-default-host", DefaultGlobalParameter: "api_host"},
	})
	w.SetWantTypeDefinition(typeDef)

	v, _ := w.Spec.GetParam("host")
	if v != "yaml-default-host" {
		t.Errorf("YAML default should take precedence over defaultGlobalParameter, got %v", v)
	}
}

func TestSetWantTypeDefinition_PriorityOrder_AllThreeSources(t *testing.T) {
	resetGlobalParams()
	defer resetGlobalParams()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "parameters.yaml")
	os.WriteFile(path, []byte("region: us-east\n"), 0644)
	LoadGlobalParameters(path)

	// param_a: spec.params wins
	// param_b: yaml default wins (no spec value)
	// param_c: global param wins (no spec value, no yaml default)
	// param_d: nothing set (no spec, no default, no global key)
	w := makeWantWithParams(map[string]any{"param_a": "from-spec"})
	typeDef := makeTypeDef([]ParameterDef{
		{Name: "param_a", Type: "string", Description: "d", Required: false,
			Default: "from-yaml", DefaultGlobalParameter: "region"},
		{Name: "param_b", Type: "string", Description: "d", Required: false,
			Default: "from-yaml", DefaultGlobalParameter: "region"},
		{Name: "param_c", Type: "string", Description: "d", Required: false,
			DefaultGlobalParameter: "region"},
		{Name: "param_d", Type: "string", Description: "d", Required: false},
	})
	w.SetWantTypeDefinition(typeDef)

	if got, _ := w.Spec.GetParam("param_a"); got != "from-spec" {
		t.Errorf("param_a: want 'from-spec', got %v", got)
	}
	if got, _ := w.Spec.GetParam("param_b"); got != "from-yaml" {
		t.Errorf("param_b: want 'from-yaml', got %v", got)
	}
	if got, _ := w.Spec.GetParam("param_c"); got != "us-east" {
		t.Errorf("param_c: want 'us-east', got %v", got)
	}
	if w.Spec.HasParam("param_d") {
		t.Errorf("param_d: should not be set when no source available")
	}
}

func TestSetWantTypeDefinition_DefaultGlobalParameter_NilGlobalParams(t *testing.T) {
	resetGlobalParams()
	defer resetGlobalParams()

	// No global parameters loaded at all
	w := makeWantWithParams(nil)
	typeDef := makeTypeDef([]ParameterDef{
		{Name: "host", Type: "string", Description: "d", Required: false,
			DefaultGlobalParameter: "api_host"},
	})
	// Should not panic
	w.SetWantTypeDefinition(typeDef)

	if w.Spec.HasParam("host") {
		t.Error("Should not set param when global params are empty")
	}
}

func TestSetWantTypeDefinition_SpecParamsNilInitialized(t *testing.T) {
	resetGlobalParams()
	defer resetGlobalParams()

	// Want has nil Spec.Params
	w := makeWantWithParams(nil)
	typeDef := makeTypeDef([]ParameterDef{
		{Name: "count", Type: "int", Description: "d", Required: false, Default: 5},
	})
	w.SetWantTypeDefinition(typeDef)

	if !w.Spec.HasParam("count") {
		t.Fatal("Spec.Params should be initialized with count")
	}
	if v, _ := w.Spec.GetParam("count"); v != 5 {
		t.Errorf("Expected 5, got %v", v)
	}
}
