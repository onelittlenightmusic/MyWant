package mywant

import (
	"os"
	"path/filepath"
	"testing"
)

func resetGlobalParams() {
	globalParamsMu.Lock()
	globalParameters = nil
	globalParamsPath = ""
	globalParamsMu.Unlock()
}

func TestLoadGlobalParameters_FileNotExist(t *testing.T) {
	resetGlobalParams()
	defer resetGlobalParams()

	err := LoadGlobalParameters("/nonexistent/path/parameters.yaml")
	if err != nil {
		t.Errorf("Expected no error for missing file, got: %v", err)
	}
	if globalParamsPath != "/nonexistent/path/parameters.yaml" {
		t.Errorf("Expected path to be stored, got: %q", globalParamsPath)
	}
}

func TestLoadGlobalParameters_ValidFile(t *testing.T) {
	resetGlobalParams()
	defer resetGlobalParams()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "parameters.yaml")
	content := "api_key: secret123\nmax_retry: 3\nflag: true\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	if err := LoadGlobalParameters(path); err != nil {
		t.Fatalf("LoadGlobalParameters failed: %v", err)
	}

	v, ok := GetGlobalParameter("api_key")
	if !ok || v != "secret123" {
		t.Errorf("Expected api_key='secret123', got %v (ok=%v)", v, ok)
	}

	v, ok = GetGlobalParameter("max_retry")
	if !ok {
		t.Error("Expected max_retry to exist")
	}
	_ = v // YAML unmarshals int; value type may vary

	v, ok = GetGlobalParameter("flag")
	if !ok || v != true {
		t.Errorf("Expected flag=true, got %v (ok=%v)", v, ok)
	}
}

func TestLoadGlobalParameters_InvalidYAML(t *testing.T) {
	resetGlobalParams()
	defer resetGlobalParams()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "parameters.yaml")
	if err := os.WriteFile(path, []byte(":\tbad: yaml: ["), 0644); err != nil {
		t.Fatal(err)
	}

	err := LoadGlobalParameters(path)
	if err == nil {
		t.Error("Expected error for invalid YAML")
	}
}

func TestGetGlobalParameter_NotFound(t *testing.T) {
	resetGlobalParams()
	defer resetGlobalParams()

	v, ok := GetGlobalParameter("nonexistent")
	if ok || v != nil {
		t.Errorf("Expected (nil, false) for missing key, got (%v, %v)", v, ok)
	}
}

func TestGetGlobalParameter_NilMap(t *testing.T) {
	resetGlobalParams()
	defer resetGlobalParams()

	// globalParameters is nil by default after reset
	v, ok := GetGlobalParameter("any_key")
	if ok || v != nil {
		t.Error("Expected false/nil when parameters map is nil")
	}
}

func TestGetAllGlobalParameters_Empty(t *testing.T) {
	resetGlobalParams()
	defer resetGlobalParams()

	all := GetAllGlobalParameters()
	if len(all) != 0 {
		t.Errorf("Expected empty map, got %d entries", len(all))
	}
}

func TestGetAllGlobalParameters_ReturnsCopy(t *testing.T) {
	resetGlobalParams()
	defer resetGlobalParams()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "parameters.yaml")
	os.WriteFile(path, []byte("key: value\n"), 0644)
	LoadGlobalParameters(path)

	all := GetAllGlobalParameters()
	all["injected"] = "should not affect original"

	v, ok := GetGlobalParameter("injected")
	if ok {
		t.Errorf("Mutation of returned map should not affect original, got %v", v)
	}
}

func TestSetAllGlobalParameters_UpdatesMemory(t *testing.T) {
	resetGlobalParams()
	defer resetGlobalParams()

	params := map[string]any{
		"host":    "localhost",
		"port":    8080,
		"enabled": true,
	}

	if err := SetAllGlobalParameters(params); err != nil {
		t.Fatalf("SetAllGlobalParameters failed: %v", err)
	}

	v, ok := GetGlobalParameter("host")
	if !ok || v != "localhost" {
		t.Errorf("Expected host='localhost', got %v (ok=%v)", v, ok)
	}

	v, ok = GetGlobalParameter("enabled")
	if !ok || v != true {
		t.Errorf("Expected enabled=true, got %v (ok=%v)", v, ok)
	}

	all := GetAllGlobalParameters()
	if len(all) != 3 {
		t.Errorf("Expected 3 params, got %d", len(all))
	}
}

func TestSetAllGlobalParameters_PersistsToDisk(t *testing.T) {
	resetGlobalParams()
	defer resetGlobalParams()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "parameters.yaml")

	// Set path by loading (even from non-existent file)
	LoadGlobalParameters(path)

	params := map[string]any{
		"db_host": "postgres",
		"db_port": 5432,
	}
	if err := SetAllGlobalParameters(params); err != nil {
		t.Fatalf("SetAllGlobalParameters failed: %v", err)
	}

	// File should exist now
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("Expected file to be created: %v", err)
	}

	// Reload and verify
	resetGlobalParams()
	if err := LoadGlobalParameters(path); err != nil {
		t.Fatalf("Reload failed: %v", err)
	}

	v, ok := GetGlobalParameter("db_host")
	if !ok || v != "postgres" {
		t.Errorf("Expected db_host='postgres' after reload, got %v (ok=%v)", v, ok)
	}
}

func TestSetAllGlobalParameters_ReplacesExisting(t *testing.T) {
	resetGlobalParams()
	defer resetGlobalParams()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "parameters.yaml")
	os.WriteFile(path, []byte("old_key: old_value\n"), 0644)
	LoadGlobalParameters(path)

	// old_key should exist
	if _, ok := GetGlobalParameter("old_key"); !ok {
		t.Fatal("Expected old_key before replace")
	}

	// Replace with entirely new params
	if err := SetAllGlobalParameters(map[string]any{"new_key": "new_value"}); err != nil {
		t.Fatal(err)
	}

	if _, ok := GetGlobalParameter("old_key"); ok {
		t.Error("old_key should be gone after SetAllGlobalParameters")
	}
	v, ok := GetGlobalParameter("new_key")
	if !ok || v != "new_value" {
		t.Errorf("Expected new_key='new_value', got %v (ok=%v)", v, ok)
	}
}

func TestSetAllGlobalParameters_NoPathSet(t *testing.T) {
	resetGlobalParams()
	defer resetGlobalParams()

	// No path loaded — should succeed without file I/O
	err := SetAllGlobalParameters(map[string]any{"k": "v"})
	if err != nil {
		t.Errorf("Expected no error when path is empty, got: %v", err)
	}

	v, ok := GetGlobalParameter("k")
	if !ok || v != "v" {
		t.Errorf("In-memory update should still work, got %v (ok=%v)", v, ok)
	}
}

func TestSetAllGlobalParameters_EmptyMap(t *testing.T) {
	resetGlobalParams()
	defer resetGlobalParams()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "parameters.yaml")
	os.WriteFile(path, []byte("key: value\n"), 0644)
	LoadGlobalParameters(path)

	if err := SetAllGlobalParameters(map[string]any{}); err != nil {
		t.Fatalf("SetAllGlobalParameters with empty map failed: %v", err)
	}

	all := GetAllGlobalParameters()
	if len(all) != 0 {
		t.Errorf("Expected 0 params after setting empty map, got %d", len(all))
	}
}
