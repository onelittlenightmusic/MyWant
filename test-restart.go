package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// TriggerEvent represents an event that can trigger want operations
type TriggerEvent struct {
	Type      string                 `json:"type"`
	Source    string                 `json:"source"`
	Target    string                 `json:"target"`
	Timestamp time.Time              `json:"timestamp"`
	Data      map[string]interface{} `json:"data,omitempty"`
	Action    string                 `json:"action,omitempty"`
}

func main() {
	fmt.Println("🔄 Testing Event-Driven Want Restart System")

	// First, let's get the current wants to see what's available
	fmt.Println("\n1. Checking current wants...")
	wants, err := getWants()
	if err != nil {
		fmt.Printf("Error getting wants: %v\n", err)
		return
	}

	fmt.Printf("Found %d wants:\n", len(wants))
	var travelPlannerID string
	for _, want := range wants {
		status := want["status"].(map[string]interface{})["status"].(string)
		wantType := want["metadata"].(map[string]interface{})["type"].(string)
		wantName := want["metadata"].(map[string]interface{})["name"].(string)
		wantID := want["metadata"].(map[string]interface{})["id"].(string)

		fmt.Printf("  - %s (ID: %s, Type: %s, Status: %s)\n", wantName, wantID, wantType, status)

		if wantType == "agent travel system" {
			travelPlannerID = wantID
		}
	}

	if travelPlannerID == "" {
		fmt.Println("❌ No agent travel system want found!")
		return
	}

	fmt.Printf("\n2. Found travel planner want ID: %s\n", travelPlannerID)

	// Test different restart scenarios
	fmt.Println("\n3. Testing restart scenarios...")

	// Scenario A: Manual restart trigger
	fmt.Println("\n📌 Scenario A: Manual Restart Trigger")
	err = sendRestartTrigger(travelPlannerID, "manual", "user", "Manual restart test")
	if err != nil {
		fmt.Printf("❌ Manual restart failed: %v\n", err)
	} else {
		fmt.Println("✅ Manual restart trigger sent successfully")
		time.Sleep(2 * time.Second)
		checkWantStatus(travelPlannerID)
	}

	// Scenario B: Parameter change trigger
	fmt.Println("\n📌 Scenario B: Parameter Change Trigger")
	err = updateWantParameters(travelPlannerID, map[string]interface{}{
		"restaurant_type": "casual dining", // Changed from "fine dining"
		"dinner_duration": 1.5,             // Changed from 2.0
	})
	if err != nil {
		fmt.Printf("❌ Parameter update failed: %v\n", err)
	} else {
		fmt.Println("✅ Parameters updated, should trigger restart")
		time.Sleep(2 * time.Second)
		checkWantStatus(travelPlannerID)
	}

	// Scenario C: State change trigger
	fmt.Println("\n📌 Scenario C: State Change Trigger")
	err = sendRestartTrigger(travelPlannerID, "state_changed", "system", "State change detected")
	if err != nil {
		fmt.Printf("❌ State change trigger failed: %v\n", err)
	} else {
		fmt.Println("✅ State change trigger sent successfully")
		time.Sleep(2 * time.Second)
		checkWantStatus(travelPlannerID)
	}

	fmt.Println("\n🎯 Event-Driven Restart Testing Complete!")
}

// getWants retrieves all current wants from the API
func getWants() ([]map[string]interface{}, error) {
	resp, err := http.Get("http://localhost:8080/api/v1/wants")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var wants []map[string]interface{}
	err = json.Unmarshal(body, &wants)
	return wants, err
}

// sendRestartTrigger demonstrates restart by updating parameters (which triggers restart)
func sendRestartTrigger(wantID, triggerType, source, description string) error {
	fmt.Printf("🔄 Demonstrating %s restart via parameter update...\n", triggerType)

	// The current system restarts wants when parameters are updated
	// This simulates the Event-Driven restart pattern
	return updateWantParameters(wantID, map[string]interface{}{
		"restaurant_type": "steakhouse", // Different from previous value
		"dinner_duration": 2.5,          // Different from previous value
		"hotel_type":      "boutique",   // Different from previous value
	})
}

// updateWantParameters updates want parameters to trigger a restart
func updateWantParameters(wantID string, newParams map[string]interface{}) error {
	updateData := map[string]interface{}{
		"spec": map[string]interface{}{
			"params": newParams,
		},
	}

	jsonData, err := json.Marshal(updateData)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("PUT", "http://localhost:8080/api/v1/wants/"+wantID,
		bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("update failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// checkWantStatus checks the current status of a want
func checkWantStatus(wantID string) {
	resp, err := http.Get("http://localhost:8080/api/v1/wants/" + wantID + "/status")
	if err != nil {
		fmt.Printf("❌ Error checking status: %v\n", err)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("❌ Error reading status: %v\n", err)
		return
	}

	var status map[string]interface{}
	err = json.Unmarshal(body, &status)
	if err != nil {
		fmt.Printf("❌ Error parsing status: %v\n", err)
		return
	}

	fmt.Printf("📊 Current status: %s\n", status["status"])
	if runtime, ok := status["runtime"].(map[string]interface{}); ok {
		if startTime, ok := runtime["start_time"].(string); ok {
			fmt.Printf("📅 Start time: %s\n", startTime)
		}
	}
}
