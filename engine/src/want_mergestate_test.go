package mywant

import (
	"sync"
	"testing"
	"time"
)

// TestMergeStateConcurrentMapMerge tests that MergeState correctly merges map[string]any values
// when multiple goroutines concurrently update the same key
func TestMergeStateConcurrentMapMerge(t *testing.T) {
	want := &Want{
		Metadata: Metadata{Name: "test-coordinator"},
		State:    make(map[string]any),
	}
	want.Init()

	// Simulate concurrent packet arrivals like Evidence and Description
	var wg sync.WaitGroup
	wg.Add(2)

	// Goroutine 1: Evidence packet (channel 0)
	go func() {
		defer wg.Done()
		evidenceData := map[string]any{
			"ApprovalID": "test-001",
			"Evidence":   "Evidence document",
		}
		want.MergeState(map[string]any{
			"data_by_channel": map[string]any{
				"0": evidenceData,
			},
		})
	}()

	// Goroutine 2: Description packet (channel 1)
	go func() {
		defer wg.Done()
		// Small delay to ensure Evidence goes first (but not guaranteed)
		time.Sleep(1 * time.Millisecond)
		descriptionData := map[string]any{
			"ApprovalID":  "test-001",
			"Description": "Approval description",
		}
		want.MergeState(map[string]any{
			"data_by_channel": map[string]any{
				"1": descriptionData,
			},
		})
	}()

	wg.Wait()

	// Verify both entries are in pendingStateChanges
	dataByChannel, exists := want.pendingStateChanges["data_by_channel"]
	if !exists {
		t.Fatal("data_by_channel not found in pendingStateChanges")
	}

	dataMap, ok := dataByChannel.(map[string]any)
	if !ok {
		t.Fatalf("data_by_channel is not map[string]any, got %T", dataByChannel)
	}

	// CRITICAL: Both entries must exist
	if _, exists := dataMap["0"]; !exists {
		t.Error("Channel 0 (Evidence) data missing from data_by_channel")
	}
	if _, exists := dataMap["1"]; !exists {
		t.Error("Channel 1 (Description) data missing from data_by_channel")
	}

	if len(dataMap) != 2 {
		t.Errorf("Expected 2 entries in data_by_channel, got %d: %v", len(dataMap), dataMap)
	}
}

// TestMergeStateReadFromPendingDuringConcurrentUpdates tests that GetState reads from pendingStateChanges
// during concurrent MergeState operations (simulating ApprovalDataHandler pattern)
func TestMergeStateReadFromPendingDuringConcurrentUpdates(t *testing.T) {
	want := &Want{
		Metadata: Metadata{Name: "test-coordinator"},
		State:    make(map[string]any),
	}
	want.Init()

	var wg sync.WaitGroup
	wg.Add(2)

	// Goroutine 1: Evidence packet (simulates ApprovalDataHandler logic)
	go func() {
		defer wg.Done()
		// Read current data_by_channel (should be empty initially)
		currentData, _ := want.GetState("data_by_channel")
		dataByChannel := make(map[string]any)
		if currentMap, ok := currentData.(map[string]any); ok {
			for k, v := range currentMap {
				dataByChannel[k] = v
			}
		}

		// Add new entry
		dataByChannel["0"] = map[string]any{"Evidence": "Evidence data"}

		// Merge
		want.MergeState(map[string]any{
			"data_by_channel": dataByChannel,
		})
	}()

	// Goroutine 2: Description packet (simulates ApprovalDataHandler logic)
	go func() {
		defer wg.Done()
		time.Sleep(2 * time.Millisecond) // Slight delay to increase chance of reading pending

		// Read current data_by_channel (MUST read from pendingStateChanges if Evidence wrote there)
		currentData, _ := want.GetState("data_by_channel")
		dataByChannel := make(map[string]any)
		if currentMap, ok := currentData.(map[string]any); ok {
			for k, v := range currentMap {
				dataByChannel[k] = v
			}
		}

		// Add new entry
		dataByChannel["1"] = map[string]any{"Description": "Description data"}

		// Merge
		want.MergeState(map[string]any{
			"data_by_channel": dataByChannel,
		})
	}()

	wg.Wait()

	// Verify both entries exist
	dataByChannel, exists := want.pendingStateChanges["data_by_channel"]
	if !exists {
		t.Fatal("data_by_channel not found in pendingStateChanges")
	}

	dataMap, ok := dataByChannel.(map[string]any)
	if !ok {
		t.Fatalf("data_by_channel is not map[string]any, got %T", dataByChannel)
	}

	if _, exists := dataMap["0"]; !exists {
		t.Error("Channel 0 (Evidence) data missing - GetState did not read from pending")
	}
	if _, exists := dataMap["1"]; !exists {
		t.Error("Channel 1 (Description) data missing")
	}

	if len(dataMap) != 2 {
		t.Errorf("Expected 2 entries in data_by_channel, got %d: %v", len(dataMap), dataMap)
		t.Logf("Full dataMap: %+v", dataMap)
	}
}

// TestMergeStateOverwriteNonMapValues tests that non-map values are correctly overwritten
func TestMergeStateOverwriteNonMapValues(t *testing.T) {
	want := &Want{
		Metadata: Metadata{Name: "test-want"},
		State:    make(map[string]any),
	}
	want.Init()

	// First update
	want.MergeState(map[string]any{
		"counter": 1,
	})

	// Second update (should overwrite)
	want.MergeState(map[string]any{
		"counter": 2,
	})

	counter, exists := want.pendingStateChanges["counter"]
	if !exists {
		t.Fatal("counter not found")
	}

	if counter != 2 {
		t.Errorf("Expected counter=2, got %v", counter)
	}
}

// TestMergeStateFromPersistedState tests that MergeState reads from persisted State
// when the key is not in pendingStateChanges
func TestMergeStateFromPersistedState(t *testing.T) {
	want := &Want{
		Metadata: Metadata{Name: "test-want"},
	}
	want.Init()

	// Set State AFTER Init (since Init overwrites State)
	want.State["data_by_channel"] = map[string]any{
		"0": map[string]any{"existing": "data"},
	}

	// Verify initial State
	t.Logf("Initial State: %+v", want.State)
	initialData, exists := want.State["data_by_channel"]
	t.Logf("Initial data_by_channel exists: %v, value: %+v, type: %T", exists, initialData, initialData)

	// MergeState should read from State and merge
	want.MergeState(map[string]any{
		"data_by_channel": map[string]any{
			"1": map[string]any{"new": "data"},
		},
	})

	// Log what happened
	t.Logf("After MergeState, pendingStateChanges: %+v", want.pendingStateChanges)

	dataByChannel, exists := want.pendingStateChanges["data_by_channel"]
	if !exists {
		t.Fatal("data_by_channel not found in pendingStateChanges")
	}

	dataMap, ok := dataByChannel.(map[string]any)
	if !ok {
		t.Fatalf("data_by_channel is not map[string]any, got %T", dataByChannel)
	}

	if _, exists := dataMap["0"]; !exists {
		t.Error("Channel 0 (from State) missing after merge")
	}
	if _, exists := dataMap["1"]; !exists {
		t.Error("Channel 1 (new) missing after merge")
	}

	if len(dataMap) != 2 {
		t.Errorf("Expected 2 entries, got %d: %v", len(dataMap), dataMap)
	}
}
