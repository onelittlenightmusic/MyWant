package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	mywant "mywant/engine/src"
)

// TestAgentServiceExecute tests the DoAgent execution endpoint
func TestAgentServiceExecute(t *testing.T) {
	// 1. Setup: Create registry and register a test agent
	registry := mywant.NewAgentRegistry()

	testAgent := &mywant.DoAgent{
		BaseAgent: *mywant.NewBaseAgent("test_agent", []string{"test"}, mywant.DoAgentType),
		Action: func(ctx context.Context, want *mywant.Want) error {
			// Simple action: read departure and create booking
			departure, _ := want.GetState("departure")
			want.StoreState("booking_id", "BOOK-"+departure.(string))
			want.StoreState("status", "confirmed")
			return nil
		},
	}
	registry.RegisterAgent(testAgent)

	// 2. Create server with global builder
	builder := mywant.NewChainBuilderWithPaths("", "")
	builder.SetAgentRegistry(registry)

	server := &Server{
		globalBuilder: builder,
	}

	// 3. Create test request
	requestBody := mywant.ExecuteRequest{
		WantID:    "test-want-123",
		AgentName: "test_agent",
		WantState: map[string]any{
			"departure": "NRT",
			"arrival":   "LAX",
		},
	}

	bodyBytes, _ := json.Marshal(requestBody)
	req := httptest.NewRequest("POST", "/api/v1/agent-service/execute", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")

	// 4. Execute request
	recorder := httptest.NewRecorder()
	server.handleAgentServiceExecute(recorder, req)

	// 5. Verify response
	if recorder.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", recorder.Code)
		t.Logf("Response body: %s", recorder.Body.String())
	}

	var response mywant.ExecuteResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// 6. Verify response data
	if response.Status != "completed" {
		t.Errorf("Expected status 'completed', got '%s'", response.Status)
	}

	if len(response.StateUpdates) != 2 {
		t.Errorf("Expected 2 state updates, got %d", len(response.StateUpdates))
	}

	if response.StateUpdates["booking_id"] != "BOOK-NRT" {
		t.Errorf("Expected booking_id='BOOK-NRT', got %v", response.StateUpdates["booking_id"])
	}

	if response.StateUpdates["status"] != "confirmed" {
		t.Errorf("Expected status='confirmed', got %v", response.StateUpdates["status"])
	}

	t.Logf("✅ Agent executed successfully in %dms", response.ExecutionTimeMs)
	t.Logf("✅ State updates: %v", response.StateUpdates)
}

// TestAgentServiceMonitorExecute tests the MonitorAgent execution endpoint
func TestAgentServiceMonitorExecute(t *testing.T) {
	// 1. Setup: Create registry and register a test monitor agent
	registry := mywant.NewAgentRegistry()

	monitorAgent := &mywant.MonitorAgent{
		BaseAgent: *mywant.NewBaseAgent("status_monitor", []string{"monitor"}, mywant.MonitorAgentType),
		Monitor: func(ctx context.Context, want *mywant.Want) error {
			// Simple monitor: check booking_id and update status
			bookingID, exists := want.GetState("booking_id")
			if exists {
				want.StoreState("last_checked", bookingID)
				want.StoreState("monitor_status", "active")
			}
			return nil
		},
	}
	registry.RegisterAgent(monitorAgent)

	// 2. Create server
	builder := mywant.NewChainBuilderWithPaths("", "")
	builder.SetAgentRegistry(registry)

	server := &Server{
		globalBuilder: builder,
	}

	// 3. Create test request
	requestBody := mywant.MonitorRequest{
		WantID:      "test-want-456",
		AgentName:   "status_monitor",
		CallbackURL: "http://localhost:8080/callback", // Won't actually be called in test
		WantState: map[string]any{
			"booking_id": "BOOK-123",
		},
	}

	bodyBytes, _ := json.Marshal(requestBody)
	req := httptest.NewRequest("POST", "/api/v1/agent-service/monitor/execute", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")

	// 4. Execute request
	recorder := httptest.NewRecorder()
	server.handleAgentServiceMonitorExecute(recorder, req)

	// 5. Verify response
	if recorder.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", recorder.Code)
		t.Logf("Response body: %s", recorder.Body.String())
	}

	var response map[string]interface{}
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// 6. Verify response data
	if response["status"] != "completed" {
		t.Errorf("Expected status 'completed', got '%v'", response["status"])
	}

	stateUpdatesCount := int(response["state_updates_count"].(float64))
	if stateUpdatesCount != 2 {
		t.Errorf("Expected 2 state updates, got %d", stateUpdatesCount)
	}

	t.Logf("✅ MonitorAgent executed successfully in %.0fms", response["execution_time_ms"])
	t.Logf("✅ State updates count: %d", stateUpdatesCount)
}

// TestAgentServiceNotFound tests error handling when agent doesn't exist
func TestAgentServiceNotFound(t *testing.T) {
	// Empty registry
	registry := mywant.NewAgentRegistry()
	builder := mywant.NewChainBuilderWithPaths("", "")
	builder.SetAgentRegistry(registry)

	server := &Server{
		globalBuilder: builder,
	}

	requestBody := mywant.ExecuteRequest{
		WantID:    "test-want",
		AgentName: "non_existent_agent",
		WantState: map[string]any{},
	}

	bodyBytes, _ := json.Marshal(requestBody)
	req := httptest.NewRequest("POST", "/api/v1/agent-service/execute", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")

	recorder := httptest.NewRecorder()
	server.handleAgentServiceExecute(recorder, req)

	if recorder.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", recorder.Code)
	}

	t.Logf("✅ Correctly returns 404 for non-existent agent")
}
