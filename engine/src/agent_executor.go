package mywant

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	pb "mywant/engine/src/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// AgentExecutor defines the interface for executing agents
type AgentExecutor interface {
	Execute(ctx context.Context, agent Agent, want *Want) error
	GetMode() ExecutionMode
}

// ============================================================================
// Local Executor (existing in-process execution)
// ============================================================================

// LocalExecutor executes agents in-process as goroutines
type LocalExecutor struct{}

func NewLocalExecutor() *LocalExecutor {
	return &LocalExecutor{}
}

func (e *LocalExecutor) GetMode() ExecutionMode {
	return ExecutionModeLocal
}

func (e *LocalExecutor) Execute(ctx context.Context, agent Agent, want *Want) error {
	// Execute agent directly (existing logic from want_agent.go:163)
	shouldStop, err := agent.Exec(ctx, want)
	if err != nil {
		return fmt.Errorf("agent execution failed: %w", err)
	}

	_ = shouldStop // Currently unused at framework level
	return nil
}

// ============================================================================
// Webhook Executor (HTTP-based external execution)
// ============================================================================

// WebhookExecutor executes agents via HTTP webhooks
type WebhookExecutor struct {
	config     WebhookConfig
	httpClient *http.Client
}

func NewWebhookExecutor(config WebhookConfig) *WebhookExecutor {
	if config.TimeoutMs <= 0 {
		config.TimeoutMs = 30000
	}

	return &WebhookExecutor{
		config: config,
		httpClient: &http.Client{
			Timeout: time.Duration(config.TimeoutMs) * time.Millisecond,
		},
	}
}

func (e *WebhookExecutor) GetMode() ExecutionMode {
	return ExecutionModeWebhook
}

func (e *WebhookExecutor) Execute(ctx context.Context, agent Agent, want *Want) error {
	if agent.GetType() == DoAgentType {
		return e.executeSyncWebhook(ctx, agent, want)
	}

	// MonitorAgent execution
	if e.config.MonitorMode == "one-shot" {
		// Execute once and return
		return e.executeMonitorWithSync(ctx, agent, want)
	}

	// Periodic execution (default)
	return e.startMonitorLoop(ctx, agent, want)
}

// executeSyncWebhook executes DoAgent via webhook and waits for response
func (e *WebhookExecutor) executeSyncWebhook(ctx context.Context, agent Agent, want *Want) error {
	request := ExecuteRequest{
		WantID:      want.Metadata.Name,
		AgentName:   agent.GetName(),
		Operation:   "execute",
		WantState:   want.State,
		CallbackURL: e.config.CallbackURL,
	}

	body, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	url := e.config.ServiceURL + "/api/v1/agent-service/execute"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if e.config.AuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+e.config.AuthToken)
	}

	log.Printf("[WEBHOOK] Executing DoAgent %s at %s", agent.GetName(), url)

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("webhook request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("webhook returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// For sync execution, we expect immediate response with state updates
	var response ExecuteResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		log.Printf("[WEBHOOK] Warning: failed to decode response: %v", err)
		return nil // Don't fail if response parsing fails
	}

	// Apply state updates if provided
	if len(response.StateUpdates) > 0 {
		want.BeginProgressCycle()
		for key, value := range response.StateUpdates {
			want.StoreState(key, value)
		}
		want.EndProgressCycle()
		log.Printf("[WEBHOOK] Applied %d state updates from DoAgent %s", len(response.StateUpdates), agent.GetName())
	}

	if response.Status == "failed" && response.Error != "" {
		return fmt.Errorf("agent execution failed: %s", response.Error)
	}

	return nil
}

// executeAsyncWebhook executes MonitorAgent via webhook (fire-and-forget)
func (e *WebhookExecutor) executeAsyncWebhook(ctx context.Context, agent Agent, want *Want) error {
	request := MonitorRequest{
		WantID:      want.Metadata.Name,
		AgentName:   agent.GetName(),
		CallbackURL: e.config.CallbackURL,
		WantState:   want.State,
	}

	body, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	url := e.config.ServiceURL + "/monitor/start"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if e.config.AuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+e.config.AuthToken)
	}

	log.Printf("[WEBHOOK] Starting MonitorAgent %s at %s", agent.GetName(), url)

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("webhook request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("webhook returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var response MonitorResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		log.Printf("[WEBHOOK] Warning: failed to decode response: %v", err)
	} else {
		log.Printf("[WEBHOOK] MonitorAgent %s started with monitor_id: %s", agent.GetName(), response.MonitorID)
	}

	return nil
}

// executeMonitorWithSync executes MonitorAgent via Agent Service (one cycle with state sync)
func (e *WebhookExecutor) executeMonitorWithSync(ctx context.Context, agent Agent, want *Want) error {
	// 1. Fetch latest state from server
	latestState, err := e.fetchLatestWantState(want.Metadata.Name)
	if err != nil {
		log.Printf("[WEBHOOK] Failed to fetch latest state: %v, using current state", err)
		latestState = want.State // Fallback to current state
	}

	// 2. Create request with latest state
	request := MonitorRequest{
		WantID:      want.Metadata.Name,
		AgentName:   agent.GetName(),
		WantState:   latestState, // Latest state synced from server
		CallbackURL: e.config.CallbackURL,
	}

	body, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	url := e.config.ServiceURL + "/api/v1/agent-service/monitor/execute"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if e.config.AuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+e.config.AuthToken)
	}

	log.Printf("[WEBHOOK] Executing MonitorAgent %s at %s (state fields: %d)",
		agent.GetName(), url, len(latestState))

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("webhook request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("webhook returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var response map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		log.Printf("[WEBHOOK] Warning: failed to decode response: %v", err)
		return nil
	}

	log.Printf("[WEBHOOK] MonitorAgent %s executed (status: %v, changes: %v)",
		agent.GetName(), response["status"], response["state_updates_count"])

	// Note: State updates are sent via callback for MonitorAgents
	// They are not applied directly here like DoAgents
	return nil
}

// fetchLatestWantState fetches the latest state from the server
func (e *WebhookExecutor) fetchLatestWantState(wantID string) (map[string]any, error) {
	// GET /api/v1/wants/{id}/state
	url := fmt.Sprintf("%s/api/v1/wants/%s/state", e.config.ServiceURL, wantID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if e.config.AuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+e.config.AuthToken)
	}

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var stateResp WantStateResponse
	if err := json.NewDecoder(resp.Body).Decode(&stateResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return stateResp.State, nil
}

// startMonitorLoop starts a periodic execution loop for MonitorAgent
func (e *WebhookExecutor) startMonitorLoop(ctx context.Context, agent Agent, want *Want) error {
	interval := time.Duration(e.config.MonitorIntervalMs) * time.Millisecond
	if interval <= 0 {
		interval = 30 * time.Second // Default to 30s if not configured
	}
	agentName := agent.GetName()

	log.Printf("[WEBHOOK] Starting MonitorAgent %s loop (interval: %v)", agentName, interval)

	// Create cancellable context for this monitor
	monitorCtx, cancel := context.WithCancel(ctx)

	// Register cancel function with Want for lifecycle management
	want.RegisterRunningAgent(agentName, cancel)

	// Start monitoring loop in background
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		defer want.UnregisterRunningAgent(agentName)

		// Execute immediately on start
		if err := e.executeMonitorWithSync(monitorCtx, agent, want); err != nil {
			log.Printf("[WEBHOOK] MonitorAgent %s initial execution failed: %v", agentName, err)
		}

		// Periodic execution
		for {
			select {
			case <-ticker.C:
				if err := e.executeMonitorWithSync(monitorCtx, agent, want); err != nil {
					log.Printf("[WEBHOOK] MonitorAgent %s cycle failed: %v", agentName, err)
				}
			case <-monitorCtx.Done():
				log.Printf("[WEBHOOK] MonitorAgent %s loop stopped: %v", agentName, monitorCtx.Err())
				return
			}
		}
	}()

	return nil
}

// ============================================================================
// RPC Executor (gRPC/JSON-RPC based external execution)
// ============================================================================

// RPCExecutor executes agents via RPC protocols
type RPCExecutor struct {
	config     RPCConfig
	grpcClient pb.AgentServiceClient
	grpcConn   *grpc.ClientConn
}

func NewRPCExecutor(config RPCConfig) (*RPCExecutor, error) {
	executor := &RPCExecutor{
		config: config,
	}

	// Initialize gRPC client if using gRPC protocol
	if config.Protocol == "grpc" {
		var opts []grpc.DialOption
		if config.UseTLS {
			// TODO: Add TLS credentials
			opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
		} else {
			opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
		}

		conn, err := grpc.NewClient(config.Endpoint, opts...)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to gRPC server: %w", err)
		}

		executor.grpcConn = conn
		executor.grpcClient = pb.NewAgentServiceClient(conn)
		log.Printf("[RPC] Connected to gRPC server at %s", config.Endpoint)
	}

	return executor, nil
}

func (e *RPCExecutor) GetMode() ExecutionMode {
	return ExecutionModeRPC
}

func (e *RPCExecutor) Execute(ctx context.Context, agent Agent, want *Want) error {
	if e.config.Protocol == "grpc" {
		return e.executeGRPC(ctx, agent, want)
	}
	return fmt.Errorf("unsupported RPC protocol: %s", e.config.Protocol)
}

// executeGRPC executes agent via gRPC
func (e *RPCExecutor) executeGRPC(ctx context.Context, agent Agent, want *Want) error {
	if agent.GetType() == DoAgentType {
		return e.executeGRPCDoAgent(ctx, agent, want)
	}
	return e.executeGRPCMonitorAgent(ctx, agent, want)
}

// executeGRPCDoAgent executes DoAgent via gRPC (synchronous)
func (e *RPCExecutor) executeGRPCDoAgent(ctx context.Context, agent Agent, want *Want) error {
	// Convert want state to map[string]string for proto
	stateMap := make(map[string]string)
	for k, v := range want.State {
		stateMap[k] = fmt.Sprintf("%v", v)
	}

	req := &pb.ExecuteRequest{
		WantId:    want.Metadata.Name,
		AgentName: agent.GetName(),
		Operation: "execute",
		WantState: stateMap,
	}

	log.Printf("[gRPC] Executing DoAgent %s at %s", agent.GetName(), e.config.Endpoint)

	resp, err := e.grpcClient.Execute(ctx, req)
	if err != nil {
		return fmt.Errorf("gRPC Execute failed: %w", err)
	}

	// Apply state updates
	if len(resp.StateUpdates) > 0 {
		want.BeginProgressCycle()
		for key, value := range resp.StateUpdates {
			want.StoreState(key, value)
		}
		want.EndProgressCycle()
		log.Printf("[gRPC] Applied %d state updates from DoAgent %s", len(resp.StateUpdates), agent.GetName())
	}

	if resp.Status == "failed" && resp.Error != "" {
		return fmt.Errorf("agent execution failed: %s", resp.Error)
	}

	log.Printf("[gRPC] DoAgent %s completed in %dms", agent.GetName(), resp.ExecutionTimeMs)
	return nil
}

// executeGRPCMonitorAgent executes MonitorAgent via gRPC (asynchronous)
func (e *RPCExecutor) executeGRPCMonitorAgent(ctx context.Context, agent Agent, want *Want) error {
	// Convert want state to map[string]string for proto
	stateMap := make(map[string]string)
	for k, v := range want.State {
		stateMap[k] = fmt.Sprintf("%v", v)
	}

	req := &pb.MonitorRequest{
		WantId:      want.Metadata.Name,
		AgentName:   agent.GetName(),
		CallbackUrl: "", // TODO: Get callback URL from config
		WantState:   stateMap,
	}

	log.Printf("[gRPC] Starting MonitorAgent %s at %s", agent.GetName(), e.config.Endpoint)

	resp, err := e.grpcClient.StartMonitor(ctx, req)
	if err != nil {
		return fmt.Errorf("gRPC StartMonitor failed: %w", err)
	}

	if resp.Status == "failed" && resp.Error != "" {
		return fmt.Errorf("monitor start failed: %s", resp.Error)
	}

	log.Printf("[gRPC] MonitorAgent %s started with monitor_id: %s", agent.GetName(), resp.MonitorId)
	return nil
}

// Close closes the gRPC connection
func (e *RPCExecutor) Close() error {
	if e.grpcConn != nil {
		return e.grpcConn.Close()
	}
	return nil
}

// ============================================================================
// Executor Factory
// ============================================================================

// NewExecutor creates the appropriate executor based on configuration
func NewExecutor(config ExecutionConfig) (AgentExecutor, error) {
	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid execution config: %w", err)
	}

	switch config.GetExecutionMode() {
	case ExecutionModeLocal:
		return NewLocalExecutor(), nil
	case ExecutionModeWebhook:
		return NewWebhookExecutor(*config.WebhookConfig), nil
	case ExecutionModeRPC:
		executor, err := NewRPCExecutor(*config.RPCConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create RPC executor: %w", err)
		}
		return executor, nil
	default:
		return nil, fmt.Errorf("unknown execution mode: %s", config.Mode)
	}
}

// ============================================================================
// Request/Response Types for External Communication
// ============================================================================

// ExecuteRequest is sent to external DoAgent service
type ExecuteRequest struct {
	WantID      string         `json:"want_id"`
	AgentName   string         `json:"agent_name"`
	Operation   string         `json:"operation"`
	WantState   map[string]any `json:"want_state"`
	Params      map[string]any `json:"params,omitempty"`
	CallbackURL string         `json:"callback_url,omitempty"`
}

// ExecuteResponse is received from external DoAgent service
type ExecuteResponse struct {
	Status          string         `json:"status"` // completed, failed, running
	StateUpdates    map[string]any `json:"state_updates,omitempty"`
	Error           string         `json:"error,omitempty"`
	ExecutionTimeMs int64          `json:"execution_time_ms,omitempty"`
}

// MonitorRequest is sent to external MonitorAgent service
type MonitorRequest struct {
	WantID      string         `json:"want_id"`
	AgentName   string         `json:"agent_name"`
	CallbackURL string         `json:"callback_url"`
	WantState   map[string]any `json:"want_state"`
}

// MonitorResponse is received from external MonitorAgent service
type MonitorResponse struct {
	MonitorID string `json:"monitor_id"`
	Status    string `json:"status"` // started, failed
	Error     string `json:"error,omitempty"`
}

// WebhookCallback is sent from external agent to MyWant callback endpoint
type WebhookCallback struct {
	AgentName    string         `json:"agent_name"`
	WantID       string         `json:"want_id"`
	Status       string         `json:"status"` // completed, failed, state_changed
	StateUpdates map[string]any `json:"state_updates,omitempty"`
	Error        string         `json:"error,omitempty"`
}

// StateUpdateRequest is sent by external agents to update want state
type StateUpdateRequest struct {
	AgentName    string         `json:"agent_name"`
	StateUpdates map[string]any `json:"state_updates"`
}

// WantStateResponse is returned when external agents query want state
type WantStateResponse struct {
	WantID    string         `json:"want_id"`
	State     map[string]any `json:"state"`
	Status    string         `json:"status"`
	Timestamp time.Time      `json:"timestamp"`
}
