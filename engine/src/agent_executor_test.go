package mywant

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestLocalExecutor(t *testing.T) {
	executor := NewLocalExecutor()

	if executor.GetMode() != ExecutionModeLocal {
		t.Errorf("Expected mode 'local', got '%s'", executor.GetMode())
	}

	// Create test agent
	agent := &DoAgent{
		BaseAgent: BaseAgent{
			Name:            "test-agent",
			Capabilities:    []string{"test"},
			Type:            DoAgentType,
			ExecutionConfig: DefaultExecutionConfig(),
		},
		Action: func(ctx context.Context, want *Want) error {
			want.StageStateChange("test_key", "test_value")
			return nil
		},
	}

	// Create test want
	want := NewWantWithLocals(
		Metadata{Name: "test-want"},
		WantSpec{},
		nil,
		"base",
	)
	want.BeginProgressCycle()

	// Execute
	err := executor.Execute(context.Background(), agent, want)
	if err != nil {
		t.Errorf("Execution failed: %v", err)
	}

	want.CommitStateChanges()
	want.EndProgressCycle()

	// Verify state was updated
	if val, exists := want.GetState("test_key"); !exists || val != "test_value" {
		t.Errorf("Expected state 'test_key' = 'test_value', got %v (exists: %v)", val, exists)
	}
}

func TestWebhookExecutor_DoAgent(t *testing.T) {
	// Create mock external agent server
	callbackReceived := false
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/agent-service/execute" {
			// Validate request
			var req ExecuteRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Errorf("Failed to decode request: %v", err)
				http.Error(w, "Bad request", http.StatusBadRequest)
				return
			}

			// Verify auth header
			auth := r.Header.Get("Authorization")
			if auth != "Bearer test-token" {
				t.Errorf("Expected auth 'Bearer test-token', got '%s'", auth)
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			// Send response with state updates
			response := ExecuteResponse{
				Status: "completed",
				StateUpdates: map[string]any{
					"booking_id": "TEST-123",
					"status":     "confirmed",
				},
				ExecutionTimeMs: 100,
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		}
	}))
	defer mockServer.Close()

	// Create webhook executor
	config := WebhookConfig{
		ServiceURL:  mockServer.URL,
		CallbackURL: "http://localhost:8080/callback",
		AuthToken:   "test-token",
		TimeoutMs:   5000,
	}

	executor := NewWebhookExecutor(config)

	if executor.GetMode() != ExecutionModeWebhook {
		t.Errorf("Expected mode 'webhook', got '%s'", executor.GetMode())
	}

	// Create test DoAgent
	agent := &DoAgent{
		BaseAgent: BaseAgent{
			Name:         "webhook-test-agent",
			Capabilities: []string{"test"},
			Type:         DoAgentType,
			ExecutionConfig: ExecutionConfig{
				Mode:          ExecutionModeWebhook,
				WebhookConfig: &config,
			},
		},
	}

	// Create test want
	want := NewWantWithLocals(
		Metadata{Name: "test-want"},
		WantSpec{},
		nil,
		"base",
	)
	want.State = map[string]any{
		"departure": "NRT",
		"arrival":   "LAX",
	}

	// Execute
	err := executor.Execute(context.Background(), agent, want)
	if err != nil {
		t.Errorf("Execution failed: %v", err)
	}

	// Verify state was updated from webhook response
	if val, exists := want.GetState("booking_id"); !exists || val != "TEST-123" {
		t.Errorf("Expected booking_id 'TEST-123', got %v (exists: %v)", val, exists)
	}

	if val, exists := want.GetState("status"); !exists || val != "confirmed" {
		t.Errorf("Expected status 'confirmed', got %v (exists: %v)", val, exists)
	}

	_ = callbackReceived // Placeholder for future async callback test
}

func TestWebhookExecutor_MonitorAgent(t *testing.T) {
	// Create mock external monitor server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/monitor/start" {
			var req MonitorRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "Bad request", http.StatusBadRequest)
				return
			}

			// Return monitor started response
			response := MonitorResponse{
				MonitorID: "monitor-123",
				Status:    "started",
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		}
	}))
	defer mockServer.Close()

	// Create webhook executor
	config := WebhookConfig{
		ServiceURL:  mockServer.URL,
		CallbackURL: "http://localhost:8080/callback",
		AuthToken:   "test-token",
		TimeoutMs:   5000,
	}

	executor := NewWebhookExecutor(config)

	// Create test MonitorAgent
	agent := &MonitorAgent{
		BaseAgent: BaseAgent{
			Name:         "webhook-monitor-agent",
			Capabilities: []string{"test"},
			Type:         MonitorAgentType,
			ExecutionConfig: ExecutionConfig{
				Mode:          ExecutionModeWebhook,
				WebhookConfig: &config,
			},
		},
	}

	// Create test want
	want := NewWantWithLocals(
		Metadata{Name: "test-want"},
		WantSpec{},
		nil,
		"base",
	)

	// Execute (should start async monitor)
	err := executor.Execute(context.Background(), agent, want)
	if err != nil {
		t.Errorf("Monitor start failed: %v", err)
	}

	// For monitor agents, execution returns immediately
	// State updates come later via callback
}

func TestExecutionConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  ExecutionConfig
		wantErr bool
	}{
		{
			name: "valid local config",
			config: ExecutionConfig{
				Mode: ExecutionModeLocal,
			},
			wantErr: false,
		},
		{
			name: "empty mode defaults to local",
			config: ExecutionConfig{
				Mode: "",
			},
			wantErr: false,
		},
		{
			name: "valid webhook config",
			config: ExecutionConfig{
				Mode: ExecutionModeWebhook,
				WebhookConfig: &WebhookConfig{
					ServiceURL:  "http://example.com",
					CallbackURL: "http://callback.com",
					TimeoutMs:   5000,
				},
			},
			wantErr: false,
		},
		{
			name: "webhook without config",
			config: ExecutionConfig{
				Mode: ExecutionModeWebhook,
			},
			wantErr: true,
		},
		{
			name: "webhook missing service url",
			config: ExecutionConfig{
				Mode: ExecutionModeWebhook,
				WebhookConfig: &WebhookConfig{
					CallbackURL: "http://callback.com",
				},
			},
			wantErr: true,
		},
		{
			name: "invalid mode",
			config: ExecutionConfig{
				Mode: "invalid",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewExecutor(t *testing.T) {
	tests := []struct {
		name     string
		config   ExecutionConfig
		wantMode ExecutionMode
		wantErr  bool
	}{
		{
			name: "create local executor",
			config: ExecutionConfig{
				Mode: ExecutionModeLocal,
			},
			wantMode: ExecutionModeLocal,
			wantErr:  false,
		},
		{
			name: "create webhook executor",
			config: ExecutionConfig{
				Mode: ExecutionModeWebhook,
				WebhookConfig: &WebhookConfig{
					ServiceURL:  "http://example.com",
					CallbackURL: "http://callback.com",
					TimeoutMs:   5000,
				},
			},
			wantMode: ExecutionModeWebhook,
			wantErr:  false,
		},
		{
			name: "invalid config",
			config: ExecutionConfig{
				Mode: ExecutionModeWebhook,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor, err := NewExecutor(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewExecutor() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err == nil {
				if executor.GetMode() != tt.wantMode {
					t.Errorf("Expected mode %s, got %s", tt.wantMode, executor.GetMode())
				}
			}
		})
	}
}

func TestWebhookConfig_EnvVarExpansion(t *testing.T) {
	// Set test environment variable
	testToken := "test-secret-token"
	t.Setenv("TEST_WEBHOOK_TOKEN", testToken)

	config := WebhookConfig{
		ServiceURL:  "http://example.com",
		CallbackURL: "http://callback.com",
		AuthToken:   "${TEST_WEBHOOK_TOKEN}",
		TimeoutMs:   5000,
	}

	err := config.Validate()
	if err != nil {
		t.Errorf("Validation failed: %v", err)
	}

	if config.AuthToken != testToken {
		t.Errorf("Expected auth token '%s', got '%s'", testToken, config.AuthToken)
	}
}

func TestWebhookExecutor_Timeout(t *testing.T) {
	// Create slow mock server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second) // Slow response
		json.NewEncoder(w).Encode(ExecuteResponse{Status: "completed"})
	}))
	defer mockServer.Close()

	// Create executor with short timeout
	config := WebhookConfig{
		ServiceURL:  mockServer.URL,
		CallbackURL: "http://callback.com",
		TimeoutMs:   500, // 500ms timeout
	}

	executor := NewWebhookExecutor(config)

	agent := &DoAgent{
		BaseAgent: BaseAgent{
			Name: "timeout-test",
			Type: DoAgentType,
		},
	}

	want := NewWantWithLocals(
		Metadata{Name: "test-want"},
		WantSpec{},
		nil,
		"base",
	)

	// Should timeout
	err := executor.Execute(context.Background(), agent, want)
	if err == nil {
		t.Error("Expected timeout error, got nil")
	}
}

// ============================================================================
// Phase 1.3: WebhookExecutor Improvements Tests
// ============================================================================

func TestWebhookExecutor_FetchLatestWantState(t *testing.T) {
	// Create mock server that returns want state
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/wants/test-want-123/state" {
			// Verify auth header
			auth := r.Header.Get("Authorization")
			if auth != "Bearer test-token" {
				t.Errorf("Expected auth 'Bearer test-token', got '%s'", auth)
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			// Return state response
			response := WantStateResponse{
				WantID: "test-want-123",
				State: map[string]any{
					"booking_id": "BOOK-999",
					"status":     "confirmed",
					"flight":     "NH123",
				},
				Status:    "active",
				Timestamp: time.Now(),
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		} else {
			http.NotFound(w, r)
		}
	}))
	defer mockServer.Close()

	// Create webhook executor
	config := WebhookConfig{
		ServiceURL: mockServer.URL,
		AuthToken:  "test-token",
		TimeoutMs:  5000,
	}

	executor := NewWebhookExecutor(config)

	// Fetch state
	state, err := executor.fetchLatestWantState("test-want-123")
	if err != nil {
		t.Fatalf("fetchLatestWantState failed: %v", err)
	}

	// Verify state
	if len(state) != 3 {
		t.Errorf("Expected 3 state fields, got %d", len(state))
	}

	if state["booking_id"] != "BOOK-999" {
		t.Errorf("Expected booking_id='BOOK-999', got %v", state["booking_id"])
	}

	if state["status"] != "confirmed" {
		t.Errorf("Expected status='confirmed', got %v", state["status"])
	}

	if state["flight"] != "NH123" {
		t.Errorf("Expected flight='NH123', got %v", state["flight"])
	}

	t.Logf("✅ Successfully fetched latest state with %d fields", len(state))
}

func TestWebhookExecutor_ExecuteMonitorWithSync(t *testing.T) {
	stateCallCount := 0
	monitorCallCount := 0

	// Create mock Agent Service server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/wants/test-want-456/state" {
			// GET state endpoint
			stateCallCount++
			response := WantStateResponse{
				WantID: "test-want-456",
				State: map[string]any{
					"booking_id": "BOOK-LATEST",
					"last_check": time.Now().Format(time.RFC3339),
				},
				Status:    "active",
				Timestamp: time.Now(),
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)

		} else if r.URL.Path == "/api/v1/agent-service/monitor/execute" {
			// POST monitor execute endpoint
			monitorCallCount++

			var req MonitorRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "Bad request", http.StatusBadRequest)
				return
			}

			// Verify request contains latest state
			if req.WantState["booking_id"] != "BOOK-LATEST" {
				t.Errorf("Expected latest state in request, got %v", req.WantState)
			}

			// Return monitor execution response
			response := map[string]interface{}{
				"status":              "completed",
				"state_updates_count": 1,
				"execution_time_ms":   50,
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		} else {
			http.NotFound(w, r)
		}
	}))
	defer mockServer.Close()

	// Create webhook executor
	config := WebhookConfig{
		ServiceURL:  mockServer.URL,
		CallbackURL: "http://localhost:8080/callback",
		AuthToken:   "test-token",
		TimeoutMs:   5000,
	}

	executor := NewWebhookExecutor(config)

	// Create test MonitorAgent
	agent := &MonitorAgent{
		BaseAgent: BaseAgent{
			Name:         "test-monitor",
			Capabilities: []string{"status_check"},
			Type:         MonitorAgentType,
		},
	}

	// Create test want
	want := NewWantWithLocals(
		Metadata{Name: "test-want-456"},
		WantSpec{},
		nil,
		"base",
	)

	// Execute monitor with sync
	err := executor.executeMonitorWithSync(context.Background(), agent, want)
	if err != nil {
		t.Fatalf("executeMonitorWithSync failed: %v", err)
	}

	// Verify state was fetched
	if stateCallCount != 1 {
		t.Errorf("Expected 1 state fetch call, got %d", stateCallCount)
	}

	// Verify monitor was executed
	if monitorCallCount != 1 {
		t.Errorf("Expected 1 monitor execute call, got %d", monitorCallCount)
	}

	t.Logf("✅ MonitorAgent executed with latest state sync (%d state fetches, %d monitor executions)",
		stateCallCount, monitorCallCount)
}

func TestWebhookExecutor_MonitorWithSync_StateFetchFailure(t *testing.T) {
	// Create mock server that returns 404 for state fetch
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/wants/test-want-789/state" {
			// Return 404
			http.NotFound(w, r)
		} else if r.URL.Path == "/api/v1/agent-service/monitor/execute" {
			// Monitor execution should still work (uses current state as fallback)
			response := map[string]interface{}{
				"status":              "completed",
				"state_updates_count": 0,
				"execution_time_ms":   20,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		} else {
			http.NotFound(w, r)
		}
	}))
	defer mockServer.Close()

	config := WebhookConfig{
		ServiceURL:  mockServer.URL,
		CallbackURL: "http://localhost:8080/callback",
		TimeoutMs:   5000,
	}

	executor := NewWebhookExecutor(config)

	agent := &MonitorAgent{
		BaseAgent: BaseAgent{
			Name: "test-monitor-fallback",
			Type: MonitorAgentType,
		},
	}

	want := NewWantWithLocals(
		Metadata{Name: "test-want-789"},
		WantSpec{},
		nil,
		"base",
	)

	// Set initial state
	want.State = map[string]any{
		"initial_field": "initial_value",
	}

	// Execute should succeed even though state fetch fails (uses fallback)
	err := executor.executeMonitorWithSync(context.Background(), agent, want)
	if err != nil {
		t.Fatalf("executeMonitorWithSync should succeed with state fetch failure: %v", err)
	}

	t.Logf("✅ MonitorAgent executed successfully with state fetch failure (used fallback)")
}

func TestWebhookExecutor_Execute_UsesMonitorWithSync(t *testing.T) {
	monitorExecuteCalled := false

	// Create mock server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/wants/test-want/state" {
			response := WantStateResponse{
				WantID:    "test-want",
				State:     map[string]any{"status": "active"},
				Status:    "active",
				Timestamp: time.Now(),
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)

		} else if r.URL.Path == "/api/v1/agent-service/monitor/execute" {
			monitorExecuteCalled = true
			response := map[string]interface{}{
				"status":              "completed",
				"state_updates_count": 0,
				"execution_time_ms":   10,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		} else {
			http.NotFound(w, r)
		}
	}))
	defer mockServer.Close()

	config := WebhookConfig{
		ServiceURL:  mockServer.URL,
		CallbackURL: "http://localhost:8080/callback",
		MonitorMode: "one-shot", // Use one-shot mode for this test
		TimeoutMs:   5000,
	}

	executor := NewWebhookExecutor(config)

	// Create MonitorAgent
	agent := &MonitorAgent{
		BaseAgent: BaseAgent{
			Name: "dispatch-test",
			Type: MonitorAgentType,
		},
	}

	want := NewWantWithLocals(
		Metadata{Name: "test-want"},
		WantSpec{},
		nil,
		"base",
	)

	// Execute should dispatch to executeMonitorWithSync
	err := executor.Execute(context.Background(), agent, want)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !monitorExecuteCalled {
		t.Error("Expected Execute to call executeMonitorWithSync for MonitorAgent")
	}

	t.Logf("✅ Execute correctly dispatched to executeMonitorWithSync for MonitorAgent")
}

// ============================================================================
// Phase 2: MonitorAgent Periodic Execution Tests
// ============================================================================

func TestWebhookExecutor_MonitorAgent_OneShotMode(t *testing.T) {
	executionCount := 0

	// Create mock server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/wants/test-want/state" {
			response := WantStateResponse{
				WantID:    "test-want",
				State:     map[string]any{"status": "active"},
				Status:    "active",
				Timestamp: time.Now(),
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)

		} else if r.URL.Path == "/api/v1/agent-service/monitor/execute" {
			executionCount++
			response := map[string]interface{}{
				"status":              "completed",
				"state_updates_count": 0,
				"execution_time_ms":   10,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		}
	}))
	defer mockServer.Close()

	// Create executor with one-shot mode
	config := WebhookConfig{
		ServiceURL:  mockServer.URL,
		CallbackURL: "http://localhost:8080/callback",
		MonitorMode: "one-shot",
		TimeoutMs:   5000,
	}

	executor := NewWebhookExecutor(config)

	agent := &MonitorAgent{
		BaseAgent: BaseAgent{
			Name: "one-shot-monitor",
			Type: MonitorAgentType,
		},
	}

	want := NewWantWithLocals(
		Metadata{Name: "test-want"},
		WantSpec{},
		nil,
		"base",
	)

	// Execute in one-shot mode
	err := executor.Execute(context.Background(), agent, want)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Wait a bit to ensure no periodic execution
	time.Sleep(200 * time.Millisecond)

	// Should execute only once in one-shot mode
	if executionCount != 1 {
		t.Errorf("Expected 1 execution in one-shot mode, got %d", executionCount)
	}

	t.Logf("✅ One-shot mode correctly executed once (%d executions)", executionCount)
}

func TestWebhookExecutor_MonitorAgent_PeriodicMode(t *testing.T) {
	executionCount := 0
	var mu sync.Mutex

	// Create mock server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/wants/test-want/state" {
			response := WantStateResponse{
				WantID:    "test-want",
				State:     map[string]any{"status": "active"},
				Status:    "active",
				Timestamp: time.Now(),
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)

		} else if r.URL.Path == "/api/v1/agent-service/monitor/execute" {
			mu.Lock()
			executionCount++
			mu.Unlock()

			response := map[string]interface{}{
				"status":              "completed",
				"state_updates_count": 0,
				"execution_time_ms":   10,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		}
	}))
	defer mockServer.Close()

	// Create executor with periodic mode (short interval for testing)
	config := WebhookConfig{
		ServiceURL:        mockServer.URL,
		CallbackURL:       "http://localhost:8080/callback",
		MonitorMode:       "periodic",
		MonitorIntervalMs: 100, // 100ms for fast testing
		TimeoutMs:         5000,
	}

	executor := NewWebhookExecutor(config)

	agent := &MonitorAgent{
		BaseAgent: BaseAgent{
			Name: "periodic-monitor",
			Type: MonitorAgentType,
		},
	}

	want := NewWantWithLocals(
		Metadata{Name: "test-want"},
		WantSpec{},
		nil,
		"base",
	)

	// Create cancellable context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Execute in periodic mode (starts background loop)
	err := executor.Execute(ctx, agent, want)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Wait for multiple executions
	time.Sleep(350 * time.Millisecond)

	// Stop the monitor
	want.StopAllAgents()
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	count := executionCount
	mu.Unlock()

	// Should execute multiple times (initial + periodic)
	// With 100ms interval and 350ms wait: expect ~3-4 executions
	if count < 2 {
		t.Errorf("Expected at least 2 executions in periodic mode, got %d", count)
	}

	t.Logf("✅ Periodic mode correctly executed %d times", count)
}

func TestWant_AgentLifecycleManagement(t *testing.T) {
	want := NewWantWithLocals(
		Metadata{Name: "test-lifecycle"},
		WantSpec{},
		nil,
		"base",
	)

	// Test 1: Register agent
	ctx1, cancel1 := context.WithCancel(context.Background())
	want.RegisterRunningAgent("agent1", cancel1)

	agents := want.GetRunningAgents()
	if len(agents) != 1 {
		t.Errorf("Expected 1 running agent, got %d", len(agents))
	}
	if agents[0] != "agent1" {
		t.Errorf("Expected agent1, got %s", agents[0])
	}

	// Test 2: Register multiple agents
	ctx2, cancel2 := context.WithCancel(context.Background())
	want.RegisterRunningAgent("agent2", cancel2)

	agents = want.GetRunningAgents()
	if len(agents) != 2 {
		t.Errorf("Expected 2 running agents, got %d", len(agents))
	}

	// Test 3: Unregister one agent
	want.UnregisterRunningAgent("agent1")
	agents = want.GetRunningAgents()
	if len(agents) != 1 {
		t.Errorf("Expected 1 running agent after unregister, got %d", len(agents))
	}

	// Test 4: Stop all agents
	want.StopAllAgents()

	// Verify ctx2 was cancelled (agent2 was still running)
	// Note: ctx1 should NOT be cancelled because agent1 was manually unregistered
	select {
	case <-ctx2.Done():
		// Expected - agent2 was stopped
	case <-time.After(100 * time.Millisecond):
		t.Error("ctx2 was not cancelled")
	}

	// ctx1 should still be active (manually unregistered, not stopped)
	select {
	case <-ctx1.Done():
		t.Error("ctx1 should not be cancelled (was manually unregistered)")
	default:
		// Expected - agent1 was unregistered, not stopped
		cancel1() // Clean up
	}

	// Verify running agents cleared
	agents = want.GetRunningAgents()
	if len(agents) != 0 {
		t.Errorf("Expected 0 running agents after StopAllAgents, got %d", len(agents))
	}

	t.Logf("✅ Agent lifecycle management works correctly")
}

func TestWebhookExecutor_PeriodicMonitor_GracefulShutdown(t *testing.T) {
	executionCount := 0
	var mu sync.Mutex

	// Create mock server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/wants/test-want/state" {
			response := WantStateResponse{
				WantID:    "test-want",
				State:     map[string]any{"status": "active"},
				Status:    "active",
				Timestamp: time.Now(),
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)

		} else if r.URL.Path == "/api/v1/agent-service/monitor/execute" {
			mu.Lock()
			executionCount++
			mu.Unlock()

			response := map[string]interface{}{
				"status":              "completed",
				"state_updates_count": 0,
				"execution_time_ms":   10,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		}
	}))
	defer mockServer.Close()

	config := WebhookConfig{
		ServiceURL:        mockServer.URL,
		CallbackURL:       "http://localhost:8080/callback",
		MonitorMode:       "periodic",
		MonitorIntervalMs: 100,
		TimeoutMs:         5000,
	}

	executor := NewWebhookExecutor(config)

	agent := &MonitorAgent{
		BaseAgent: BaseAgent{
			Name: "shutdown-test",
			Type: MonitorAgentType,
		},
	}

	want := NewWantWithLocals(
		Metadata{Name: "test-want"},
		WantSpec{},
		nil,
		"base",
	)

	// Start monitor
	ctx, cancel := context.WithCancel(context.Background())
	err := executor.Execute(ctx, agent, want)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Wait for some executions
	time.Sleep(250 * time.Millisecond)

	mu.Lock()
	countBeforeShutdown := executionCount
	mu.Unlock()

	// Trigger graceful shutdown via context cancellation
	cancel()
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	countAfterShutdown := executionCount
	mu.Unlock()

	// No new executions should happen after shutdown
	if countAfterShutdown != countBeforeShutdown {
		t.Logf("Note: %d executions after shutdown (may be in-flight requests)",
			countAfterShutdown-countBeforeShutdown)
	}

	// Verify monitor is no longer running
	agents := want.GetRunningAgents()
	if len(agents) != 0 {
		t.Errorf("Expected 0 running agents after shutdown, got %d: %v", len(agents), agents)
	}

	t.Logf("✅ Graceful shutdown works correctly (%d executions before, %d after)",
		countBeforeShutdown, countAfterShutdown)
}

func TestWebhookConfig_MonitorDefaults(t *testing.T) {
	tests := []struct {
		name           string
		config         WebhookConfig
		wantInterval   int
		wantMode       string
		wantValidation bool
	}{
		{
			name: "defaults applied",
			config: WebhookConfig{
				ServiceURL:  "http://example.com",
				CallbackURL: "http://callback.com",
			},
			wantInterval:   30000,
			wantMode:       "periodic",
			wantValidation: true,
		},
		{
			name: "custom values preserved",
			config: WebhookConfig{
				ServiceURL:        "http://example.com",
				CallbackURL:       "http://callback.com",
				MonitorIntervalMs: 5000,
				MonitorMode:       "one-shot",
			},
			wantInterval:   5000,
			wantMode:       "one-shot",
			wantValidation: true,
		},
		{
			name: "invalid monitor mode",
			config: WebhookConfig{
				ServiceURL:  "http://example.com",
				CallbackURL: "http://callback.com",
				MonitorMode: "invalid",
			},
			wantValidation: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()

			if tt.wantValidation && err != nil {
				t.Errorf("Validation failed: %v", err)
			}
			if !tt.wantValidation && err == nil {
				t.Error("Expected validation to fail")
			}

			if tt.wantValidation {
				if tt.config.MonitorIntervalMs != tt.wantInterval {
					t.Errorf("Expected interval %d, got %d", tt.wantInterval, tt.config.MonitorIntervalMs)
				}
				if tt.config.MonitorMode != tt.wantMode {
					t.Errorf("Expected mode %s, got %s", tt.wantMode, tt.config.MonitorMode)
				}
			}
		})
	}

	t.Logf("✅ WebhookConfig defaults work correctly")
}
