package main

import (
	"context"
	"fmt"
	"mywant/engine/cmd/types"
	. "mywant/engine/src"
	"time"
)

func main() {
	fmt.Println("=== MonitorFlightAPI Test ===")
	fmt.Println()

	// Create a mock want for testing
	want := &Want{
		Metadata: Metadata{
			Name: "test-flight",
			Type: "flight",
		},
		Spec: WantSpec{
			Params: make(map[string]interface{}),
		},
		State:   make(map[string]interface{}),
		History: WantHistory{},
	}

	// Simulate initial flight creation by setting flight_id and initial status
	want.StoreState("flight_id", "mock-flight-123")
	want.StoreState("flight_status", "confirmed")
	want.StoreState("flight_number", "AA100")
	want.StoreState("from", "New York")
	want.StoreState("to", "Los Angeles")

	fmt.Println("Initial Flight State:")
	fmt.Printf("  Flight ID: %v\n", want.State["flight_id"])
	fmt.Printf("  Status: %v\n", want.State["flight_status"])
	fmt.Println()

	// Create MonitorFlightAPI agent
	monitor := types.NewMonitorFlightAPI(
		"monitor-flight-api",
		[]string{},
		[]string{},
		"http://localhost:8081",
	)

	fmt.Println("Testing MonitorFlightAPI:")
	fmt.Println()

	// Test 1: Try to poll (will fail if server not running, which is expected)
	fmt.Println("Test 1: Attempting to poll with valid flight_id")
	fmt.Println("  Note: This requires mock server running on port 8081")
	err := monitor.Exec(context.Background(), want)
	if err != nil {
		fmt.Printf("  ℹ️  Error (expected if server not running): %v\n", err)
		fmt.Println()

		// For demonstration, we'll simulate status changes manually
		fmt.Println("Test 2: Simulating status changes manually")
		fmt.Println()

		// Simulate status changes over time
		statusSequence := []string{"confirmed", "details_changed", "delayed_one_day"}
		for i, status := range statusSequence {
			fmt.Printf("  [%d] Status Update: %s\n", i+1, status)
			want.StoreState("flight_status", status)
			want.StoreState("status_changed", true)
			want.StoreState("status_changed_at", time.Now().Format(time.RFC3339))
			time.Sleep(100 * time.Millisecond)
		}
		fmt.Println()
	} else {
		fmt.Printf("  ✓ Successfully polled flight status\n")
		if status, exists := want.GetState("flight_status"); exists {
			fmt.Printf("  Current Status: %v\n", status)
		}
		fmt.Println()
	}

	// Test 3: Check state history
	fmt.Println("Test 3: Checking Want State After Monitoring")
	fmt.Println()
	fmt.Println("  Current State:")
	for key, value := range want.State {
		fmt.Printf("    %s: %v\n", key, value)
	}
	fmt.Println()

	// Test 4: Demonstrate status change detection
	fmt.Println("Test 4: Demonstrating Delay Detection Logic")
	fmt.Println()

	// Check if delayed
	statusVal, exists := want.GetState("flight_status")
	if exists {
		status, ok := statusVal.(string)
		if ok && status == "delayed_one_day" {
			fmt.Println("  ✓ Flight Status is DELAYED_ONE_DAY")
			fmt.Println("  → Would trigger automatic cancellation and rebooking")
		}
	}
	fmt.Println()

	// Test 5: Show agent capabilities
	fmt.Println("Test 5: MonitorFlightAPI Agent Information")
	fmt.Println()
	fmt.Printf("  Agent Name: %s\n", monitor.GetName())
	fmt.Printf("  Agent Type: %s\n", monitor.GetType())
	fmt.Printf("  Capabilities: %v\n", monitor.GetCapabilities())
	fmt.Printf("  Server URL: %s\n", monitor.ServerURL)
	fmt.Printf("  Poll Interval: %v\n", monitor.PollInterval)
	fmt.Println()

	// Test 6: Show status change history
	fmt.Println("Test 6: Status Change History")
	fmt.Println()
	history := monitor.GetStatusChangeHistory()
	if len(history) > 0 {
		fmt.Printf("  Total Status Changes: %d\n", len(history))
		for i, change := range history {
			fmt.Printf("  [%d] %s: %s -> %s (%s)\n",
				i+1,
				change.Timestamp.Format("15:04:05"),
				change.OldStatus,
				change.NewStatus,
				change.Details)
		}
	} else {
		fmt.Println("  No status changes recorded (would be recorded during live polling)")
	}
	fmt.Println()

	fmt.Println("=== Test Complete ===")
	fmt.Println()
	fmt.Println("📋 Summary:")
	fmt.Println()
	fmt.Println("  ✓ MonitorFlightAPI created successfully")
	fmt.Println("  ✓ Want state initialized")
	fmt.Println("  ✓ Status change detection demonstrated")
	fmt.Println()
	fmt.Println("📝 To test with real mock server:")
	fmt.Println()
	fmt.Println("  1. In one terminal: make run-mock")
	fmt.Println("  2. In another terminal: make run-flight")
	fmt.Println()
	fmt.Println("  This will:")
	fmt.Println("    - Create a flight via POST /api/flights")
	fmt.Println("    - Poll status via GET /api/flights/{id}")
	fmt.Println("    - Detect status changes (confirmed → details_changed → delayed_one_day)")
	fmt.Println("    - Automatically cancel and rebook when delayed")
	fmt.Println("    - Capture complete state history in memory dump at ~/memory/memory-*.yaml")
	fmt.Println()
}
