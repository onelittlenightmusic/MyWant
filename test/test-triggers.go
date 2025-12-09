package main

import (
	"fmt"
	"mywant/engine/src"
	"time"
)

func main() {
	fmt.Println("🔥 Trigger System Test")
	fmt.Println("=====================")

	// Load a simple config
	config, _, err := src.LoadRecipeWithConfig("config/config-travel-recipe.yaml")
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		return
	}

	// Create builder with trigger system
	builder := src.NewChainBuilder(*config)
	src.RegisterTravelWantTypes(builder)

	// Start execution in background
	go func() {
		builder.Execute()
	}()

	// Wait for system to start
	time.Sleep(2 * time.Second)

	// Test manual triggers
	fmt.Println("\n🎯 Testing Manual Triggers:")

	// Test reconciliation trigger
	fmt.Println("📋 Triggering manual reconciliation...")
	err = builder.IgniteManual("system", map[string]interface{}{
		"action": "reconcile",
	})
	if err != nil {
		fmt.Printf("Error triggering reconcile: %v\n", err)
	}

	// Test agent trigger
	fmt.Println("🤖 Triggering agent alert...")
	err = builder.IgniteAgent("test-agent", "travel planner", map[string]interface{}{
		"alert_type": "test_alert",
		"message":    "Test agent trigger",
	})
	if err != nil {
		fmt.Printf("Error triggering agent: %v\n", err)
	}

	// Test scheduled trigger
	fmt.Println("⏰ Triggering scheduled action...")
	err = builder.IgniteScheduled("travel planner", 1*time.Second, map[string]interface{}{
		"action": "start",
	})
	if err != nil {
		fmt.Printf("Error triggering scheduled: %v\n", err)
	}

	// Test conditional trigger
	fmt.Println("🔍 Testing conditional trigger...")
	conditions := []src.TriggerCondition{
		{
			Field:    "status",
			Operator: "eq",
			Value:    "running",
		},
	}
	err = builder.IgniteConditional("travel planner", conditions, map[string]interface{}{
		"trigger_action": "restart",
		"state_changes": map[string]interface{}{
			"test_condition": "triggered",
		},
	})
	if err != nil {
		fmt.Printf("Error triggering conditional: %v\n", err)
	}

	// Wait for triggers to process
	time.Sleep(3 * time.Second)

	fmt.Println("\n✅ Trigger system test completed!")
	fmt.Println("Check the logs above to see trigger processing in action.")
}
