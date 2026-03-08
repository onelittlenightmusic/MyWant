package mywant

import (
	"testing"
	"time"
)

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
	want.storeState("test_key", "test_value")
	err = want.SendCallback()
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Give async callback time to execute
	time.Sleep(100 * time.Millisecond)
}

// TestFinalResultFieldAutoOverride tests that EndProgressCycle automatically sets final_result from FinalResultField
func TestFinalResultFieldAutoOverride(t *testing.T) {
	// Test 1: FinalResultField copies named state to final_result
	want := NewWantWithLocals(
		Metadata{Name: "test-want"},
		WantSpec{FinalResultField: "reservation_name"},
		nil,
		"base",
	)

	want.BeginProgressCycle()
	want.storeState("reservation_name", "Le Bernardin")
	want.EndProgressCycle()

	val, exists := want.getState("final_result")
	if !exists {
		t.Fatal("Expected final_result to exist after EndProgressCycle")
	}
	if val != "Le Bernardin" {
		t.Errorf("Expected final_result='Le Bernardin', got %v", val)
	}

	// Test 2: Empty FinalResultField does nothing
	want2 := NewWantWithLocals(
		Metadata{Name: "test-want-2"},
		WantSpec{},
		nil,
		"base",
	)

	want2.BeginProgressCycle()
	want2.storeState("some_field", "some_value")
	want2.EndProgressCycle()

	_, exists = want2.getState("final_result")
	if exists {
		t.Error("Expected final_result NOT to exist when FinalResultField is empty")
	}

	// Test 3: Zero-value field is skipped (empty string)
	want3 := NewWantWithLocals(
		Metadata{Name: "test-want-3"},
		WantSpec{FinalResultField: "result"},
		nil,
		"base",
	)

	want3.BeginProgressCycle()
	want3.storeState("result", "")
	want3.EndProgressCycle()

	val, exists = want3.getState("final_result")
	if exists && val != nil && val != "" {
		t.Errorf("Expected final_result to be empty/nonexistent for zero-value, got %v", val)
	}

	// Test 4: Non-string values work (int)
	want4 := NewWantWithLocals(
		Metadata{Name: "test-want-4"},
		WantSpec{FinalResultField: "count"},
		nil,
		"base",
	)

	want4.BeginProgressCycle()
	want4.storeState("count", 42)
	want4.EndProgressCycle()

	val, exists = want4.getState("final_result")
	if !exists {
		t.Fatal("Expected final_result to exist for int value")
	}
	if val != 42 {
		t.Errorf("Expected final_result=42, got %v", val)
	}

	// Test 5: Value from previous cycle (already in State, not in pendingStateChanges)
	want5 := NewWantWithLocals(
		Metadata{Name: "test-want-5"},
		WantSpec{FinalResultField: "url"},
		nil,
		"base",
	)

	// First cycle: set the url
	want5.BeginProgressCycle()
	want5.storeState("url", "https://example.ngrok.io")
	want5.EndProgressCycle()

	// Second cycle: don't change url, but EndProgressCycle should still copy it
	want5.BeginProgressCycle()
	// Don't store anything new
	want5.EndProgressCycle()

	val, exists = want5.getState("final_result")
	if !exists {
		t.Fatal("Expected final_result to persist from previous cycle")
	}
	if val != "https://example.ngrok.io" {
		t.Errorf("Expected final_result='https://example.ngrok.io', got %v", val)
	}
}

// TestFinalResultFieldNestedDotNotation tests dot-notation support for FinalResultField
func TestFinalResultFieldNestedDotNotation(t *testing.T) {
	// Test 1: single-level nesting — "slack_latest_message.text"
	want := NewWantWithLocals(
		Metadata{Name: "test-nested-1"},
		WantSpec{FinalResultField: "slack_latest_message.text"},
		nil,
		"base",
	)

	want.BeginProgressCycle()
	want.storeState("slack_latest_message", map[string]any{
		"sender": "U01ABC",
		"text":   "hello world",
	})
	want.EndProgressCycle()

	val, exists := want.getState("final_result")
	if !exists {
		t.Fatal("Expected final_result to exist for nested dot-notation field")
	}
	if val != "hello world" {
		t.Errorf("Expected final_result='hello world', got %v", val)
	}

	// Test 2: two-level nesting — "outer.inner.value"
	want2 := NewWantWithLocals(
		Metadata{Name: "test-nested-2"},
		WantSpec{FinalResultField: "outer.inner.value"},
		nil,
		"base",
	)

	want2.BeginProgressCycle()
	want2.storeState("outer", map[string]any{
		"inner": map[string]any{
			"value": "deep",
		},
	})
	want2.EndProgressCycle()

	val, exists = want2.getState("final_result")
	if !exists {
		t.Fatal("Expected final_result for two-level nesting")
	}
	if val != "deep" {
		t.Errorf("Expected final_result='deep', got %v", val)
	}

	// Test 3: nested field is missing — final_result should not be set
	want3 := NewWantWithLocals(
		Metadata{Name: "test-nested-3"},
		WantSpec{FinalResultField: "msg.nonexistent"},
		nil,
		"base",
	)

	want3.BeginProgressCycle()
	want3.storeState("msg", map[string]any{"text": "hi"})
	want3.EndProgressCycle()

	_, exists = want3.getState("final_result")
	if exists {
		t.Error("Expected final_result NOT to be set when nested key is missing")
	}
}

