package types

import (
	"fmt"
	"sync"
	"testing"
	"time"

	. "mywant/engine/src"
)

// TestApprovalDataHandlerConcurrentPackets tests that ApprovalDataHandler correctly
// merges Evidence and Description packets when they arrive concurrently
func TestApprovalDataHandlerConcurrentPackets(t *testing.T) {
	// Create coordinator want
	want := &CoordinatorWant{
		Want: Want{
			Metadata: Metadata{Name: "test-coordinator", Type: "coordinator"},
			State:    make(map[string]any),
		},
		DataHandler:       &ApprovalDataHandler{Level: 2},
		CoordinatorType:   "coordinator",
		channelsHeard:     make(map[int]bool),
	}
	want.Init()

	// Simulate concurrent packet arrivals like in Level 2 approval
	var wg sync.WaitGroup
	wg.Add(2)

	// Goroutine 1: Evidence packet (channel 0)
	go func() {
		defer wg.Done()
		evidenceData := &ApprovalData{
			ApprovalID:  "test-level2-001",
			Evidence:    "Evidence document",
			Description: "",
			Timestamp:   time.Now(),
		}
		want.DataHandler.ProcessData(want, 0, evidenceData)
	}()

	// Goroutine 2: Description packet (channel 1)
	go func() {
		defer wg.Done()
		time.Sleep(1 * time.Millisecond) // Small delay to increase race condition likelihood
		descriptionData := &ApprovalData{
			ApprovalID:  "test-level2-001",
			Evidence:    nil,
			Description: "Level 2 Approval: test-level2-001",
			Timestamp:   time.Now(),
		}
		want.DataHandler.ProcessData(want, 1, descriptionData)
	}()

	wg.Wait()

	// Verify both entries exist in data_by_channel
	dataByChannel, exists := want.GetState("data_by_channel")
	if !exists {
		t.Fatal("data_by_channel not found in state")
	}

	dataMap, ok := dataByChannel.(map[string]any)
	if !ok {
		t.Fatalf("data_by_channel is not map[string]any, got %T", dataByChannel)
	}

	// CRITICAL: Both entries must exist
	if _, exists := dataMap["0"]; !exists {
		t.Error("Channel 0 (Evidence) data missing from data_by_channel")
		t.Logf("data_by_channel content: %+v", dataMap)
	}
	if _, exists := dataMap["1"]; !exists {
		t.Error("Channel 1 (Description) data missing from data_by_channel")
		t.Logf("data_by_channel content: %+v", dataMap)
	}

	if len(dataMap) != 2 {
		t.Errorf("Expected 2 entries in data_by_channel, got %d: %v", len(dataMap), dataMap)
		for k, v := range dataMap {
			t.Logf("  Channel %s: %+v", k, v)
		}
	}

	// Verify total_packets_received
	totalPackets, exists := want.GetState("total_packets_received")
	if !exists {
		t.Error("total_packets_received not found")
	} else if totalPackets != 2 {
		t.Errorf("Expected total_packets_received=2, got %v", totalPackets)
	}
}

// TestApprovalDataHandlerSequentialPackets tests sequential packet processing
func TestApprovalDataHandlerSequentialPackets(t *testing.T) {
	want := &CoordinatorWant{
		Want: Want{
			Metadata: Metadata{Name: "test-coordinator", Type: "coordinator"},
			State:    make(map[string]any),
		},
		DataHandler:       &ApprovalDataHandler{Level: 2},
		CoordinatorType:   "coordinator",
		channelsHeard:     make(map[int]bool),
	}
	want.Init()

	// Process Evidence first
	evidenceData := &ApprovalData{
		ApprovalID:  "test-seq-001",
		Evidence:    "Evidence document",
		Description: "",
		Timestamp:   time.Now(),
	}
	want.DataHandler.ProcessData(want, 0, evidenceData)

	// Process Description second
	descriptionData := &ApprovalData{
		ApprovalID:  "test-seq-001",
		Evidence:    nil,
		Description: "Description text",
		Timestamp:   time.Now(),
	}
	want.DataHandler.ProcessData(want, 1, descriptionData)

	// Verify both entries exist
	dataByChannel, exists := want.GetState("data_by_channel")
	if !exists {
		t.Fatal("data_by_channel not found")
	}

	dataMap, ok := dataByChannel.(map[string]any)
	if !ok {
		t.Fatalf("data_by_channel is not map[string]any, got %T", dataByChannel)
	}

	if len(dataMap) != 2 {
		t.Errorf("Expected 2 entries, got %d: %v", len(dataMap), dataMap)
	}

	// Verify content
	if channel0, exists := dataMap["0"]; !exists {
		t.Error("Channel 0 missing")
	} else {
		evidence, ok := channel0.(*ApprovalData)
		if !ok {
			t.Errorf("Channel 0 is not *ApprovalData, got %T", channel0)
		} else if evidence.Evidence == nil {
			t.Error("Evidence field is nil")
		}
	}

	if channel1, exists := dataMap["1"]; !exists {
		t.Error("Channel 1 missing")
	} else {
		description, ok := channel1.(*ApprovalData)
		if !ok {
			t.Errorf("Channel 1 is not *ApprovalData, got %T", channel1)
		} else if description.Description == "" {
			t.Error("Description field is empty")
		}
	}
}

// TestApprovalDataHandlerRepeatedConcurrentPackets runs the concurrent test multiple times
// to catch intermittent race conditions
func TestApprovalDataHandlerRepeatedConcurrentPackets(t *testing.T) {
	const iterations = 10

	for i := 0; i < iterations; i++ {
		t.Run(fmt.Sprintf("Iteration_%d", i), func(t *testing.T) {
			want := &CoordinatorWant{
				Want: Want{
					Metadata: Metadata{Name: fmt.Sprintf("test-coordinator-%d", i), Type: "coordinator"},
					State:    make(map[string]any),
				},
				DataHandler:       &ApprovalDataHandler{Level: 2},
				CoordinatorType:   "coordinator",
				channelsHeard:     make(map[int]bool),
			}
			want.Init()

			var wg sync.WaitGroup
			wg.Add(2)

			// Evidence packet
			go func() {
				defer wg.Done()
				evidenceData := &ApprovalData{
					ApprovalID:  fmt.Sprintf("test-%d", i),
					Evidence:    fmt.Sprintf("Evidence-%d", i),
					Timestamp:   time.Now(),
				}
				want.DataHandler.ProcessData(want, 0, evidenceData)
			}()

			// Description packet
			go func() {
				defer wg.Done()
				time.Sleep(time.Duration(i%3) * time.Millisecond) // Vary timing
				descriptionData := &ApprovalData{
					ApprovalID:  fmt.Sprintf("test-%d", i),
					Description: fmt.Sprintf("Description-%d", i),
					Timestamp:   time.Now(),
				}
				want.DataHandler.ProcessData(want, 1, descriptionData)
			}()

			wg.Wait()

			// Verify
			dataByChannel, exists := want.GetState("data_by_channel")
			if !exists {
				t.Fatalf("Iteration %d: data_by_channel not found", i)
			}

			dataMap, ok := dataByChannel.(map[string]any)
			if !ok {
				t.Fatalf("Iteration %d: data_by_channel is not map[string]any, got %T", i, dataByChannel)
			}

			if len(dataMap) != 2 {
				t.Errorf("Iteration %d: Expected 2 entries, got %d", i, len(dataMap))
				t.Logf("data_by_channel: %+v", dataMap)
			}
		})
	}
}
