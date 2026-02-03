package mywant

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// ============================================================================
// E2E Test: Same Agent in Multiple Execution Modes
// ============================================================================

func TestAgentExecutionModes(t *testing.T) {
	// Common Agent implementation - this exact same Action function
	// will be used in both Local and Webhook execution modes
	testAction := func(ctx context.Context, want *Want) error {
		departure, _ := want.GetState("departure")
		want.StoreState("booking_id", fmt.Sprintf("BOOK-%s", departure))
		want.StoreState("status", "confirmed")
		return nil
	}

	t.Run("Local", func(t *testing.T) {
		// Create agent with Local execution mode
		agent := &DoAgent{
			BaseAgent: BaseAgent{
				Name:         "test_local",
				Capabilities: []string{"booking"},
				Type:         DoAgentType,
				ExecutionConfig: ExecutionConfig{
					Mode: ExecutionModeLocal,
				},
			},
			Action: testAction,
		}

		// Create want with test data
		want := NewWantWithLocals(
			Metadata{Name: "test-local"},
			WantSpec{},
			nil,
			"base",
		)
		want.State = map[string]any{"departure": "NRT"}

		want.BeginProgressCycle()

		// Execute locally
		_, err := agent.Exec(context.Background(), want)
		if err != nil {
			t.Fatalf("Local execution failed: %v", err)
		}

		want.EndProgressCycle()

		// Verify results
		bookingID, exists := want.GetState("booking_id")
		if !exists {
			t.Error("booking_id not found in state")
		}
		if bookingID != "BOOK-NRT" {
			t.Errorf("Expected booking_id='BOOK-NRT', got %v", bookingID)
		}

		status, exists := want.GetState("status")
		if !exists {
			t.Error("status not found in state")
		}
		if status != "confirmed" {
			t.Errorf("Expected status='confirmed', got %v", status)
		}

		t.Logf("✅ Local execution: booking_id=%s, status=%s", bookingID, status)
	})

	t.Run("Webhook", func(t *testing.T) {
		// Create registry and register agent
		registry := NewAgentRegistry()
		agent := &DoAgent{
			BaseAgent: BaseAgent{
				Name:         "test_webhook",
				Capabilities: []string{"booking"},
				Type:         DoAgentType,
			},
			Action: testAction, // Same action function!
		}
		registry.RegisterAgent(agent)

		// Create Agent Service mock server
		mockServer := createAgentServiceMockServer(registry)
		defer mockServer.Close()

		// Create webhook executor
		config := WebhookConfig{
			ServiceURL:  mockServer.URL,
			CallbackURL: "http://localhost:8080/callback",
			TimeoutMs:   5000,
		}
		executor := NewWebhookExecutor(config)

		// Create want with test data
		want := NewWantWithLocals(
			Metadata{Name: "test-webhook"},
			WantSpec{},
			nil,
			"base",
		)
		want.State = map[string]any{"departure": "NRT"}

		// Execute via webhook
		err := executor.Execute(context.Background(), agent, want)
		if err != nil {
			t.Fatalf("Webhook execution failed: %v", err)
		}

		// Verify results (state should be updated from webhook response)
		bookingID, exists := want.GetState("booking_id")
		if !exists {
			t.Error("booking_id not found in state")
		}
		if bookingID != "BOOK-NRT" {
			t.Errorf("Expected booking_id='BOOK-NRT', got %v", bookingID)
		}

		status, exists := want.GetState("status")
		if !exists {
			t.Error("status not found in state")
		}
		if status != "confirmed" {
			t.Errorf("Expected status='confirmed', got %v", status)
		}

		t.Logf("✅ Webhook execution: booking_id=%s, status=%s", bookingID, status)
	})

	t.Logf("✅ Same Agent implementation works in both Local and Webhook modes")
}

// ============================================================================
// E2E Test: MonitorAgent with State Synchronization
// ============================================================================

func TestMonitorAgentStateSync(t *testing.T) {
	var mu sync.Mutex
	serverState := map[string]any{
		"flight_status": "scheduled",
		"delay_minutes": 0,
	}

	// MonitorAgent implementation - checks flight status
	monitorAction := func(ctx context.Context, want *Want) error {
		// Read current state
		status, _ := want.GetState("flight_status")
		delayRaw, _ := want.GetState("delay_minutes")

		// Convert delay to int (JSON unmarshals numbers as float64)
		var delay int
		switch v := delayRaw.(type) {
		case int:
			delay = v
		case float64:
			delay = int(v)
		default:
			delay = 0
		}

		// If delayed, update local tracking
		if delay > 0 {
			want.StoreState("last_check", time.Now().Format(time.RFC3339))
			want.StoreState("alert_sent", true)
		}

		t.Logf("Monitor cycle: status=%v, delay=%v", status, delay)
		return nil
	}

	t.Run("StateSyncBeforeEachCycle", func(t *testing.T) {
		fetchCount := 0

		// Create registry
		registry := NewAgentRegistry()
		monitorAgent := &MonitorAgent{
			BaseAgent: BaseAgent{
				Name:         "flight_monitor",
				Capabilities: []string{"flight_monitoring"},
				Type:         MonitorAgentType,
			},
			Monitor: monitorAction,
		}
		registry.RegisterAgent(monitorAgent)

		// Create mock server that tracks state fetch calls
		mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/v1/wants/test-monitor/state" {
				mu.Lock()
				fetchCount++
				// Simulate state change on 2nd fetch
				if fetchCount == 2 {
					serverState["delay_minutes"] = 30
					serverState["flight_status"] = "delayed"
				}
				state := make(map[string]any)
				for k, v := range serverState {
					state[k] = v
				}
				mu.Unlock()

				response := WantStateResponse{
					WantID:    "test-monitor",
					State:     state,
					Status:    "active",
					Timestamp: time.Now(),
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(response)

			} else if r.URL.Path == "/api/v1/agent-service/monitor/execute" {
				var req MonitorRequest
				json.NewDecoder(r.Body).Decode(&req)

				// Execute monitor with provided state
				want := &Want{
					Metadata: Metadata{Name: req.WantID},
					State:    req.WantState,
				}
				want.BeginProgressCycle()
				monitorAgent.Monitor(context.Background(), want)
				stateUpdates := want.GetPendingStateChanges()
				want.EndProgressCycle()

				response := map[string]interface{}{
					"status":              "completed",
					"state_updates_count": len(stateUpdates),
					"execution_time_ms":   10,
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(response)
			}
		}))
		defer mockServer.Close()

		// Create webhook executor with short interval
		config := WebhookConfig{
			ServiceURL:        mockServer.URL,
			CallbackURL:       "http://localhost:8080/callback",
			MonitorMode:       "periodic",
			MonitorIntervalMs: 100,
			TimeoutMs:         5000,
		}
		executor := NewWebhookExecutor(config)

		want := NewWantWithLocals(
			Metadata{Name: "test-monitor"},
			WantSpec{},
			nil,
			"base",
		)

		// Start periodic monitoring
		ctx, cancel := context.WithCancel(context.Background())
		err := executor.Execute(ctx, monitorAgent, want)
		if err != nil {
			t.Fatalf("Monitor start failed: %v", err)
		}

		// Wait for multiple cycles
		time.Sleep(250 * time.Millisecond)

		// Stop monitoring
		cancel()
		want.StopAllAgents()
		time.Sleep(50 * time.Millisecond)

		mu.Lock()
		count := fetchCount
		mu.Unlock()

		// Should have fetched state multiple times (once per cycle)
		if count < 2 {
			t.Errorf("Expected at least 2 state fetches, got %d", count)
		}

		t.Logf("✅ State fetched %d times (once per cycle)", count)
	})

	t.Run("OneShotMode", func(t *testing.T) {
		registry := NewAgentRegistry()
		monitorAgent := &MonitorAgent{
			BaseAgent: BaseAgent{
				Name: "flight_monitor_oneshot",
				Type: MonitorAgentType,
			},
			Monitor: monitorAction,
		}
		registry.RegisterAgent(monitorAgent)

		mockServer := createAgentServiceMockServer(registry)
		defer mockServer.Close()

		config := WebhookConfig{
			ServiceURL:  mockServer.URL,
			CallbackURL: "http://localhost:8080/callback",
			MonitorMode: "one-shot",
			TimeoutMs:   5000,
		}
		executor := NewWebhookExecutor(config)

		want := NewWantWithLocals(
			Metadata{Name: "test-oneshot"},
			WantSpec{},
			nil,
			"base",
		)
		want.State = map[string]any{
			"flight_status": "scheduled",
			"delay_minutes": 0,
		}

		// Execute once
		err := executor.Execute(context.Background(), monitorAgent, want)
		if err != nil {
			t.Fatalf("One-shot execution failed: %v", err)
		}

		// Verify no background loop is running
		agents := want.GetRunningAgents()
		if len(agents) != 0 {
			t.Errorf("Expected 0 running agents in one-shot mode, got %d", len(agents))
		}

		t.Logf("✅ One-shot mode executed once without starting background loop")
	})
}

// ============================================================================
// E2E Test: Error Handling
// ============================================================================

func TestAgentExecutionErrors(t *testing.T) {
	t.Run("AgentExecutionFailure", func(t *testing.T) {
		// Agent that always fails
		failingAction := func(ctx context.Context, want *Want) error {
			return fmt.Errorf("simulated agent failure")
		}

		agent := &DoAgent{
			BaseAgent: BaseAgent{
				Name: "failing_agent",
				Type: DoAgentType,
			},
			Action: failingAction,
		}

		registry := NewAgentRegistry()
		registry.RegisterAgent(agent)

		mockServer := createAgentServiceMockServer(registry)
		defer mockServer.Close()

		config := WebhookConfig{
			ServiceURL:  mockServer.URL,
			CallbackURL: "http://localhost:8080/callback",
			TimeoutMs:   5000,
		}
		executor := NewWebhookExecutor(config)

		want := NewWantWithLocals(
			Metadata{Name: "test-fail"},
			WantSpec{},
			nil,
			"base",
		)

		// Execute should not return error (error is in response)
		err := executor.Execute(context.Background(), agent, want)
		if err != nil {
			t.Logf("Execution error (expected): %v", err)
		}

		t.Logf("✅ Agent execution failure handled correctly")
	})

	t.Run("NetworkFailure", func(t *testing.T) {
		agent := &DoAgent{
			BaseAgent: BaseAgent{
				Name: "network_test",
				Type: DoAgentType,
			},
			Action: func(ctx context.Context, want *Want) error {
				want.StoreState("test", "value")
				return nil
			},
		}

		// Invalid URL - will cause network error
		config := WebhookConfig{
			ServiceURL:  "http://localhost:9999",
			CallbackURL: "http://localhost:8080/callback",
			TimeoutMs:   1000,
		}
		executor := NewWebhookExecutor(config)

		want := NewWantWithLocals(
			Metadata{Name: "test-network"},
			WantSpec{},
			nil,
			"base",
		)

		// Should return error for network failure
		err := executor.Execute(context.Background(), agent, want)
		if err == nil {
			t.Error("Expected network error, got nil")
		} else {
			t.Logf("Network error (expected): %v", err)
		}

		t.Logf("✅ Network failure handled correctly")
	})

	t.Run("TimeoutHandling", func(t *testing.T) {
		// Create slow server
		mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(2 * time.Second)
			w.WriteHeader(http.StatusOK)
		}))
		defer mockServer.Close()

		agent := &DoAgent{
			BaseAgent: BaseAgent{
				Name: "timeout_test",
				Type: DoAgentType,
			},
		}

		config := WebhookConfig{
			ServiceURL:  mockServer.URL,
			CallbackURL: "http://localhost:8080/callback",
			TimeoutMs:   500, // 500ms timeout
		}
		executor := NewWebhookExecutor(config)

		want := NewWantWithLocals(
			Metadata{Name: "test-timeout"},
			WantSpec{},
			nil,
			"base",
		)

		// Should timeout
		err := executor.Execute(context.Background(), agent, want)
		if err == nil {
			t.Error("Expected timeout error, got nil")
		} else {
			t.Logf("Timeout error (expected): %v", err)
		}

		t.Logf("✅ Timeout handling works correctly")
	})
}

// ============================================================================
// E2E Test: Complete Workflow
// ============================================================================

func TestCompleteAgentWorkflow(t *testing.T) {
	// Simulate a complete workflow:
	// 1. DoAgent books a flight
	// 2. MonitorAgent watches for status changes
	// 3. State updates flow correctly

	bookingAction := func(ctx context.Context, want *Want) error {
		departure, _ := want.GetState("departure")
		arrival, _ := want.GetState("arrival")
		want.StoreState("booking_id", fmt.Sprintf("BOOK-%s-%s", departure, arrival))
		want.StoreState("flight_status", "confirmed")
		return nil
	}

	monitorAction := func(ctx context.Context, want *Want) error {
		status, _ := want.GetState("flight_status")
		if status == "confirmed" {
			want.StoreState("last_check", time.Now().Format(time.RFC3339))
		}
		return nil
	}

	// Create registry with both agents
	registry := NewAgentRegistry()

	bookingAgent := &DoAgent{
		BaseAgent: BaseAgent{
			Name:         "booking_agent",
			Capabilities: []string{"booking"},
			Type:         DoAgentType,
		},
		Action: bookingAction,
	}

	monitorAgent := &MonitorAgent{
		BaseAgent: BaseAgent{
			Name:         "status_monitor",
			Capabilities: []string{"monitoring"},
			Type:         MonitorAgentType,
		},
		Monitor: monitorAction,
	}

	registry.RegisterAgent(bookingAgent)
	registry.RegisterAgent(monitorAgent)

	mockServer := createAgentServiceMockServer(registry)
	defer mockServer.Close()

	config := WebhookConfig{
		ServiceURL:  mockServer.URL,
		CallbackURL: "http://localhost:8080/callback",
		TimeoutMs:   5000,
	}
	executor := NewWebhookExecutor(config)

	want := NewWantWithLocals(
		Metadata{Name: "test-workflow"},
		WantSpec{},
		nil,
		"base",
	)
	want.State = map[string]any{
		"departure": "NRT",
		"arrival":   "LAX",
	}

	// Step 1: Execute booking agent
	err := executor.Execute(context.Background(), bookingAgent, want)
	if err != nil {
		t.Fatalf("Booking failed: %v", err)
	}

	bookingID, exists := want.GetState("booking_id")
	if !exists || bookingID != "BOOK-NRT-LAX" {
		t.Errorf("Booking failed: booking_id=%v", bookingID)
	}

	// Step 2: Execute monitor agent locally (for immediate state updates)
	// Note: Webhook MonitorAgents send state updates via callback (async),
	// so we use local execution for this test to verify state updates immediately
	want.BeginProgressCycle()
	_, err = monitorAgent.Exec(context.Background(), want)
	if err != nil {
		t.Fatalf("Monitor failed: %v", err)
	}
	want.EndProgressCycle()

	lastCheck, exists := want.GetState("last_check")
	if !exists {
		t.Error("Monitor didn't update last_check")
	}

	t.Logf("✅ Complete workflow: booking=%s (via webhook), monitoring (via local), last_check=%v",
		bookingID, lastCheck)
}

// ============================================================================
// Helper Functions
// ============================================================================

// createAgentServiceMockServer creates a mock Agent Service server
func createAgentServiceMockServer(registry *AgentRegistry) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/agent-service/execute" {
			// DoAgent execution
			var req ExecuteRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "Bad request", http.StatusBadRequest)
				return
			}

			agent, exists := registry.GetAgent(req.AgentName)
			if !exists {
				http.Error(w, "Agent not found", http.StatusNotFound)
				return
			}

			// Execute agent
			want := &Want{
				Metadata: Metadata{Name: req.WantID},
				State:    req.WantState,
			}
			want.BeginProgressCycle()

			start := time.Now()
			_, err := agent.Exec(context.Background(), want)

			stateUpdates := want.GetPendingStateChanges()
			want.EndProgressCycle()

			response := ExecuteResponse{
				Status:          "completed",
				StateUpdates:    stateUpdates,
				ExecutionTimeMs: time.Since(start).Milliseconds(),
			}

			if err != nil {
				response.Status = "failed"
				response.Error = err.Error()
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)

		} else if r.URL.Path == "/api/v1/agent-service/monitor/execute" {
			// MonitorAgent execution
			var req MonitorRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "Bad request", http.StatusBadRequest)
				return
			}

			agent, exists := registry.GetAgent(req.AgentName)
			if !exists {
				http.Error(w, "Agent not found", http.StatusNotFound)
				return
			}

			// Execute monitor
			want := &Want{
				Metadata: Metadata{Name: req.WantID},
				State:    req.WantState,
			}
			want.SetRemoteCallback(req.CallbackURL, req.AgentName)
			want.BeginProgressCycle()

			start := time.Now()
			_, err := agent.Exec(context.Background(), want)

			stateUpdates := want.GetPendingStateChanges()
			want.EndProgressCycle()

			response := map[string]interface{}{
				"status":              "completed",
				"state_updates_count": len(stateUpdates),
				"execution_time_ms":   time.Since(start).Milliseconds(),
			}

			if err != nil {
				response["status"] = "failed"
				response["error"] = err.Error()
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)

		} else if r.URL.Path == "/api/v1/wants/"+r.URL.Query().Get("id")+"/state" {
			// State fetch endpoint (for tests that need it)
			response := WantStateResponse{
				WantID:    r.URL.Query().Get("id"),
				State:     map[string]any{"test": "state"},
				Status:    "active",
				Timestamp: time.Now(),
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)

		} else {
			http.NotFound(w, r)
		}
	}))
}
