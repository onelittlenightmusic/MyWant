package mywant

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGlobalState_StoreAndGet(t *testing.T) {
	cb := &ChainBuilder{}

	cb.StoreGlobalState("key1", "value1")
	cb.StoreGlobalState("key2", 42)

	val, ok := cb.GetGlobalStateValue("key1")
	if !ok || val != "value1" {
		t.Errorf("Expected 'value1', got %v (ok=%v)", val, ok)
	}

	val, ok = cb.GetGlobalStateValue("key2")
	if !ok || val != 42 {
		t.Errorf("Expected 42, got %v (ok=%v)", val, ok)
	}

	_, ok = cb.GetGlobalStateValue("nonexistent")
	if ok {
		t.Error("Expected false for nonexistent key")
	}
}

func TestGlobalState_MergeState(t *testing.T) {
	cb := &ChainBuilder{}

	cb.StoreGlobalState("nested", map[string]any{"a": 1, "b": 2})
	cb.MergeGlobalState(map[string]any{
		"nested":   map[string]any{"b": 99, "c": 3},
		"toplevel": "hello",
	})

	val, ok := cb.GetGlobalStateValue("nested")
	if !ok {
		t.Fatal("Expected nested key to exist")
	}
	nested, ok := val.(map[string]any)
	if !ok {
		t.Fatal("Expected nested to be map[string]any")
	}
	if nested["a"] != 1 {
		t.Errorf("Expected a=1, got %v", nested["a"])
	}
	if nested["b"] != 99 {
		t.Errorf("Expected b=99, got %v", nested["b"])
	}
	if nested["c"] != 3 {
		t.Errorf("Expected c=3, got %v", nested["c"])
	}

	val, ok = cb.GetGlobalStateValue("toplevel")
	if !ok || val != "hello" {
		t.Errorf("Expected 'hello', got %v", val)
	}
}

func TestGlobalState_GetAll(t *testing.T) {
	cb := &ChainBuilder{}

	cb.StoreGlobalState("x", 1)
	cb.StoreGlobalState("y", 2)

	all := cb.GetGlobalStateAll()
	if len(all) != 2 {
		t.Errorf("Expected 2 keys, got %d", len(all))
	}
	if all["x"] != 1 || all["y"] != 2 {
		t.Errorf("Unexpected values: %v", all)
	}
}

func TestGlobalState_Persistence(t *testing.T) {
	// Create temp dir for test
	tmpDir := t.TempDir()
	memPath := filepath.Join(tmpDir, "memory.yaml")
	globalPath := filepath.Join(tmpDir, "global_state.yaml")

	cb := NewChainBuilderWithPaths("", memPath)

	// Verify globalStatePath was set correctly
	if cb.globalStatePath != globalPath {
		t.Errorf("Expected globalStatePath=%s, got %s", globalPath, cb.globalStatePath)
	}

	// Store some state
	cb.StoreGlobalState("persist_key", "persist_value")
	cb.MergeGlobalState(map[string]any{"count": 42})

	// Verify file was created
	if _, err := os.Stat(globalPath); err != nil {
		t.Fatalf("Expected global_state.yaml to exist: %v", err)
	}

	// Simulate restart: new ChainBuilder loads from same file
	cb2 := NewChainBuilderWithPaths("", memPath)

	val, ok := cb2.GetGlobalStateValue("persist_key")
	if !ok || val != "persist_value" {
		t.Errorf("Expected 'persist_value' after restart, got %v (ok=%v)", val, ok)
	}

	val, ok = cb2.GetGlobalStateValue("count")
	if !ok {
		t.Error("Expected 'count' to exist after restart")
	}
	// YAML unmarshals integers as int by default
	_ = val
}

func TestGlobalState_PackageLevelFunctions(t *testing.T) {
	// Set up a global chain builder
	cb := &ChainBuilder{}
	SetGlobalChainBuilder(cb)
	defer SetGlobalChainBuilder(nil)

	StoreGlobalState("pkg_key", "pkg_value")

	val, ok := GetGlobalState("pkg_key")
	if !ok || val != "pkg_value" {
		t.Errorf("Expected 'pkg_value', got %v (ok=%v)", val, ok)
	}

	MergeGlobalState(map[string]any{"pkg_key2": "v2"})
	all := GetAllGlobalState()
	if all["pkg_key2"] != "v2" {
		t.Errorf("Expected pkg_key2='v2' in all state")
	}
}

func TestGlobalState_PackageLevelFunctions_NilBuilder(t *testing.T) {
	// Ensure nil globalChainBuilder doesn't panic
	prev := GetGlobalChainBuilder()
	SetGlobalChainBuilder(nil)
	defer SetGlobalChainBuilder(prev)

	StoreGlobalState("key", "value") // should not panic
	MergeGlobalState(map[string]any{"key": "value"}) // should not panic
	val, ok := GetGlobalState("key")
	if ok || val != nil {
		t.Error("Expected nil/false when no global builder")
	}
	all := GetAllGlobalState()
	if all != nil {
		t.Error("Expected nil when no global builder")
	}
}

func TestGlobalState_WantMethods(t *testing.T) {
	cb := &ChainBuilder{}
	SetGlobalChainBuilder(cb)
	defer SetGlobalChainBuilder(nil)

	want := &Want{
		Metadata: Metadata{Name: "test-want", Type: "test"},
		State:    make(map[string]any),
	}

	want.StoreGlobalState("wkey", "wvalue")
	val, ok := want.GetGlobalState("wkey")
	if !ok || val != "wvalue" {
		t.Errorf("Expected 'wvalue', got %v (ok=%v)", val, ok)
	}

	want.MergeGlobalState(map[string]any{"merged": true})
	val, ok = want.GetGlobalState("merged")
	if !ok || val != true {
		t.Errorf("Expected true, got %v (ok=%v)", val, ok)
	}
}

func TestGlobalState_ParentStateFallback(t *testing.T) {
	cb := &ChainBuilder{}
	SetGlobalChainBuilder(cb)
	defer SetGlobalChainBuilder(nil)

	// Want with no parent
	orphan := &Want{
		Metadata: Metadata{Name: "orphan", Type: "test"},
		State:    make(map[string]any),
	}

	// StoreParentState should fall back to globalState
	orphan.StoreParentState("fallback_key", "fallback_value")

	val, ok := orphan.GetParentState("fallback_key")
	if !ok || val != "fallback_value" {
		t.Errorf("Expected 'fallback_value' via GetParentState fallback, got %v (ok=%v)", val, ok)
	}

	// MergeParentState should also fall back
	orphan.MergeParentState(map[string]any{"merged_key": "merged_value"})
	val, ok = orphan.GetParentState("merged_key")
	if !ok || val != "merged_value" {
		t.Errorf("Expected 'merged_value' via MergeParentState fallback, got %v (ok=%v)", val, ok)
	}
}
