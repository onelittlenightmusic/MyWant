package mywant

import (
	"testing"
	"time"
)

// TestGetPendingStateChanges tests the GetPendingStateChanges method
func TestGetPendingStateChanges(t *testing.T) {
	want := NewWantWithLocals(
		Metadata{Name: "test-want"},
		WantSpec{},
		nil,
		"base",
	)

	// Test 1: Begin fresh progress cycle
	want.BeginProgressCycle()
	want.EndProgressCycle() // Clear any initial state

	// Test 2: New cycle with our changes
	want.BeginProgressCycle()
	want.StoreState("key1", "value1")
	want.StoreState("key2", 42)
	want.StoreState("key3", true)

	changes := want.GetPendingStateChanges()
	if len(changes) != 3 {
		t.Errorf("Expected 3 pending changes, got %d", len(changes))
	}

	if changes["key1"] != "value1" {
		t.Errorf("Expected key1='value1', got %v", changes["key1"])
	}
	if changes["key2"] != 42 {
		t.Errorf("Expected key2=42, got %v", changes["key2"])
	}
	if changes["key3"] != true {
		t.Errorf("Expected key3=true, got %v", changes["key3"])
	}

	// Test 3: After EndProgressCycle, pending changes should be cleared
	want.EndProgressCycle()
	changes = want.GetPendingStateChanges()
	if len(changes) != 0 {
		t.Errorf("Expected pending changes to be cleared after EndProgressCycle, got %d", len(changes))
	}

	// Test 4: Verify the changes are in State
	val, exists := want.GetState("key1")
	if !exists || val != "value1" {
		t.Errorf("Expected key1 to be in State after EndProgressCycle")
	}
}

// TestGetPendingStateChangesConcurrency tests concurrent access
func TestGetPendingStateChangesConcurrency(t *testing.T) {
	want := NewWantWithLocals(
		Metadata{Name: "test-want"},
		WantSpec{},
		nil,
		"base",
	)

	want.BeginProgressCycle()

	// Concurrent writes
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(index int) {
			want.StoreState(string(rune('a'+index)), index)
			done <- true
		}(i)
	}

	// Wait for all writes
	for i := 0; i < 10; i++ {
		<-done
	}

	// Concurrent reads
	for i := 0; i < 10; i++ {
		go func() {
			changes := want.GetPendingStateChanges()
			if len(changes) != 10 {
				t.Errorf("Expected 10 changes, got %d", len(changes))
			}
			done <- true
		}()
	}

	// Wait for all reads
	for i := 0; i < 10; i++ {
		<-done
	}
}

// TestSetRemoteCallback tests the SetRemoteCallback method
func TestSetRemoteCallback(t *testing.T) {
	want := NewWantWithLocals(
		Metadata{Name: "test-want"},
		WantSpec{},
		nil,
		"base",
	)

	// Test initial state
	if want.remoteMode {
		t.Error("Expected remoteMode to be false initially")
	}

	// Set remote callback
	callbackURL := "http://localhost:8080/callback"
	agentName := "test-agent"
	want.SetRemoteCallback(callbackURL, agentName)

	// Verify
	if !want.remoteMode {
		t.Error("Expected remoteMode to be true after SetRemoteCallback")
	}
	if want.callbackURL != callbackURL {
		t.Errorf("Expected callbackURL=%s, got %s", callbackURL, want.callbackURL)
	}
	if want.agentName != agentName {
		t.Errorf("Expected agentName=%s, got %s", agentName, want.agentName)
	}
}

// TestSendCallback tests the SendCallback method
func TestSendCallback(t *testing.T) {
	want := NewWantWithLocals(
		Metadata{Name: "test-want"},
		WantSpec{},
		nil,
		"base",
	)

	// Test 1: No callback URL set
	err := want.SendCallback()
	if err == nil {
		t.Error("Expected error when callback URL not set")
	}

	// Test 2: Callback URL set but no changes
	want.SetRemoteCallback("http://localhost:8080/callback", "test-agent")
	want.BeginProgressCycle()
	err = want.SendCallback()
	if err != nil {
		t.Errorf("Expected no error with empty changes, got %v", err)
	}

	// Test 3: With changes (callback is async, so we just verify no panic)
	want.StoreState("test_key", "test_value")
	err = want.SendCallback()
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Give async callback time to execute
	time.Sleep(100 * time.Millisecond)
}

// TestGetPendingStateChangesIsolation tests that returned map is a copy
func TestGetPendingStateChangesIsolation(t *testing.T) {
	want := NewWantWithLocals(
		Metadata{Name: "test-want"},
		WantSpec{},
		nil,
		"base",
	)

	want.BeginProgressCycle()
	want.StoreState("key1", "value1")

	// Get changes
	changes1 := want.GetPendingStateChanges()

	// Modify returned map
	changes1["key2"] = "value2"
	changes1["key1"] = "modified"

	// Get changes again
	changes2 := want.GetPendingStateChanges()

	// Verify original is not affected
	if len(changes2) != 1 {
		t.Errorf("Expected 1 change in original, got %d", len(changes2))
	}
	if changes2["key1"] != "value1" {
		t.Errorf("Expected original key1='value1', got %v", changes2["key1"])
	}
	if _, exists := changes2["key2"]; exists {
		t.Error("Expected key2 not to exist in original")
	}
}
