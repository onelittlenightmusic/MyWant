package mywant

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
		if r.URL.Path == "/execute" {
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
