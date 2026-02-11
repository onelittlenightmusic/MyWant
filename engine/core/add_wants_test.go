package mywant

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// TestAddWantsAsyncDuplicateNameGuard tests that duplicate want names are rejected
func TestAddWantsAsyncDuplicateNameGuard(t *testing.T) {
	// Create a minimal ChainBuilder for testing
	cb := NewChainBuilder(Config{
		Wants: []*Want{},
	})

	// Start the reconcile loop in the background
	go cb.reconcileLoop()
	defer cb.Stop()

	// Give the reconcile loop time to start
	time.Sleep(100 * time.Millisecond)

	// Test 1: Add a want successfully
	want1 := &Want{
		Metadata: Metadata{
			Name: "test-want-1",
			ID:   "id-1",
			Type: "noop",
		},
		Spec: WantSpec{
			Params: map[string]any{},
		},
	}

	ids, err := cb.AddWantsAsyncWithTracking([]*Want{want1})
	if err != nil {
		t.Fatalf("Failed to add first want: %v", err)
	}
	if len(ids) != 1 || ids[0] != "id-1" {
		t.Fatalf("Expected ID 'id-1', got: %v", ids)
	}
	t.Log("✅ Successfully added first want")

	// Wait for reconciliation to complete
	time.Sleep(200 * time.Millisecond)

	// Test 2: Try to add a want with duplicate name (should fail)
	want2 := &Want{
		Metadata: Metadata{
			Name: "test-want-1", // Same name as want1
			ID:   "id-2",
			Type: "noop",
		},
		Spec: WantSpec{
			Params: map[string]any{},
		},
	}

	_, err = cb.AddWantsAsyncWithTracking([]*Want{want2})
	if err == nil {
		t.Fatal("Expected error when adding want with duplicate name, but got nil")
	}

	expectedErrSubstring := "already exists"
	if !strings.Contains(err.Error(), expectedErrSubstring) {
		t.Errorf("Expected error message to contain '%s', got: %v", expectedErrSubstring, err)
	}
	t.Logf("✅ Correctly rejected duplicate want name with error: %v", err)

	// Test 3: Add a want with unique name (should succeed)
	want3 := &Want{
		Metadata: Metadata{
			Name: "test-want-3",
			ID:   "id-3",
			Type: "noop",
		},
		Spec: WantSpec{
			Params: map[string]any{},
		},
	}

	ids, err = cb.AddWantsAsyncWithTracking([]*Want{want3})
	if err != nil {
		t.Fatalf("Failed to add want with unique name: %v", err)
	}
	if len(ids) != 1 || ids[0] != "id-3" {
		t.Fatalf("Expected ID 'id-3', got: %v", ids)
	}
	t.Log("✅ Successfully added want with unique name")

	// Wait for reconciliation
	time.Sleep(200 * time.Millisecond)

	// Test 4: Try to add multiple wants where one has duplicate name
	want4 := &Want{
		Metadata: Metadata{
			Name: "test-want-4",
			ID:   "id-4",
			Type: "noop",
		},
		Spec: WantSpec{
			Params: map[string]any{},
		},
	}
	want5 := &Want{
		Metadata: Metadata{
			Name: "test-want-1", // Duplicate
			ID:   "id-5",
			Type: "noop",
		},
		Spec: WantSpec{
			Params: map[string]any{},
		},
	}

	_, err = cb.AddWantsAsyncWithTracking([]*Want{want4, want5})
	if err == nil {
		t.Fatal("Expected error when adding batch with duplicate name, but got nil")
	}
	t.Logf("✅ Correctly rejected batch with duplicate name: %v", err)

	t.Log("✅ All duplicate name guard tests passed")
}

// TestAddWantsAsyncChannelFull tests behavior when the add channel is full
func TestAddWantsAsyncChannelFull(t *testing.T) {
	cb := NewChainBuilder(Config{
		Wants: []*Want{},
	})

	// Start reconcile loop, but immediately fill the channel before it can process
	go cb.reconcileLoop()
	defer cb.Stop()

	// Send many requests rapidly to fill the channel buffer (size 10)
	// Use goroutines to send concurrently so they try to queue up
	results := make(chan error, 20)
	for i := 0; i < 20; i++ {
		go func(idx int) {
			want := &Want{
				Metadata: Metadata{
					Name: fmt.Sprintf("test-want-%d", idx),
					ID:   fmt.Sprintf("id-%d", idx),
					Type: "noop",
				},
				Spec: WantSpec{
					Params: map[string]any{},
				},
			}
			results <- cb.AddWantsAsync([]*Want{want})
		}(i)
	}

	// Collect results
	successCount := 0
	channelFullCount := 0
	for i := 0; i < 20; i++ {
		err := <-results
		if err == nil {
			successCount++
		} else if strings.Contains(err.Error(), "channel full") {
			channelFullCount++
		} else {
			t.Logf("Unexpected error: %v", err)
		}
	}

	t.Logf("Results: %d succeeded, %d got 'channel full' error", successCount, channelFullCount)

	// We expect most requests to succeed since the reconcileLoop is running,
	// but some might get "channel full" if they all arrive at once
	if successCount == 0 {
		t.Error("Expected at least some requests to succeed")
	}

	t.Logf("✅ Channel test passed: %d requests succeeded, %d got channel full", successCount, channelFullCount)
}
