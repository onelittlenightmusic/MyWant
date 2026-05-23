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
)

// ============================================================================
// Generic HTTP Client
// ============================================================================

// HTTPClient wraps http.Client with convenience methods for internal API calls
type HTTPClient struct {
	client  *http.Client
	baseURL string
}

func NewHTTPClient(baseURL string) *HTTPClient {
	return &HTTPClient{
		client:  &http.Client{Timeout: 10 * time.Second},
		baseURL: baseURL,
	}
}

func (c *HTTPClient) POST(path string, body interface{}) (*http.Response, error) {
	url := c.baseURL + path
	var reqBody io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewBuffer(jsonData)
	}
	req, err := http.NewRequest("POST", url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create POST request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("POST request failed: %w", err)
	}
	return resp, nil
}

func (c *HTTPClient) GET(path string) (*http.Response, error) {
	url := c.baseURL + path
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create GET request: %w", err)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET request failed: %w", err)
	}
	return resp, nil
}

func (c *HTTPClient) PUT(path string, body interface{}) (*http.Response, error) {
	url := c.baseURL + path
	var reqBody io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewBuffer(jsonData)
	}
	req, err := http.NewRequest("PUT", url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create PUT request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("PUT request failed: %w", err)
	}
	return resp, nil
}

func (c *HTTPClient) DELETE(path string) (*http.Response, error) {
	url := c.baseURL + path
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create DELETE request: %w", err)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("DELETE request failed: %w", err)
	}
	return resp, nil
}

func (c *HTTPClient) DecodeJSON(resp *http.Response, v interface{}) error {
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP error %d: %s", resp.StatusCode, string(body))
	}
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		return fmt.Errorf("failed to decode JSON response: %w", err)
	}
	return nil
}

// ============================================================================
// Webhook Request / Response Types
// ============================================================================

// ExecuteRequest is sent to an external DoAgent service.
type ExecuteRequest struct {
	WantID      string         `json:"want_id"`
	AgentName   string         `json:"agent_name"`
	Operation   string         `json:"operation"`
	WantState   map[string]any `json:"want_state"`
	Params      map[string]any `json:"params,omitempty"`
	CallbackURL string         `json:"callback_url,omitempty"`
}

// ExecuteResponse is received from an external DoAgent service.
type ExecuteResponse struct {
	Status          string         `json:"status"` // completed, failed, running
	StateUpdates    map[string]any `json:"state_updates,omitempty"`
	Error           string         `json:"error,omitempty"`
	ExecutionTimeMs int64          `json:"execution_time_ms,omitempty"`
}

// MonitorRequest is sent to an external MonitorAgent service.
type MonitorRequest struct {
	WantID      string         `json:"want_id"`
	AgentName   string         `json:"agent_name"`
	CallbackURL string         `json:"callback_url"`
	WantState   map[string]any `json:"want_state"`
}

// MonitorResponse is received from an external MonitorAgent service.
type MonitorResponse struct {
	MonitorID string `json:"monitor_id"`
	Status    string `json:"status"` // started, failed
	Error     string `json:"error,omitempty"`
}

// WebhookCallback is sent from an external agent back to the MyWant callback endpoint.
type WebhookCallback struct {
	AgentName    string         `json:"agent_name"`
	WantID       string         `json:"want_id"`
	Status       string         `json:"status"` // completed, failed, state_changed
	StateUpdates map[string]any `json:"state_updates,omitempty"`
	Error        string         `json:"error,omitempty"`
}

// StateUpdateRequest is sent by external agents to push state updates.
type StateUpdateRequest struct {
	AgentName    string         `json:"agent_name"`
	StateUpdates map[string]any `json:"state_updates"`
}

// WantStateResponse is returned when external agents query want state.
type WantStateResponse struct {
	WantID    string         `json:"want_id"`
	State     map[string]any `json:"state"`
	Status    string         `json:"status"`
	Timestamp time.Time      `json:"timestamp"`
}

// ============================================================================
// WebhookExecutor — HTTP-based external agent execution
// ============================================================================

// WebhookExecutor executes agents via HTTP webhooks.
type WebhookExecutor struct {
	config     WebhookConfig
	httpClient *http.Client
}

func NewWebhookExecutor(config WebhookConfig) *WebhookExecutor {
	if config.TimeoutMs <= 0 {
		config.TimeoutMs = 30000
	}
	return &WebhookExecutor{
		config:     config,
		httpClient: &http.Client{Timeout: time.Duration(config.TimeoutMs) * time.Millisecond},
	}
}

func (e *WebhookExecutor) GetMode() ExecutionMode { return ExecutionModeWebhook }

func (e *WebhookExecutor) Execute(ctx context.Context, agent Agent, want *Want) error {
	if agent.GetType() == DoAgentType {
		return e.executeSyncWebhook(ctx, agent, want)
	}
	if e.config.MonitorMode == "one-shot" {
		return e.executeMonitorWithSync(ctx, agent, want)
	}
	return e.startMonitorLoop(ctx, agent, want)
}

func (e *WebhookExecutor) executeSyncWebhook(ctx context.Context, agent Agent, want *Want) error {
	request := ExecuteRequest{
		WantID:      want.Metadata.Name,
		AgentName:   agent.GetName(),
		Operation:   "execute",
		WantState:   want.GetAllState(),
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
	var response ExecuteResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		log.Printf("[WEBHOOK] Warning: failed to decode response: %v", err)
		return nil
	}
	if len(response.StateUpdates) > 0 {
		want.BeginProgressCycle()
		for key, value := range response.StateUpdates {
			want.storeState(key, value)
		}
		want.EndProgressCycle()
		log.Printf("[WEBHOOK] Applied %d state updates from DoAgent %s", len(response.StateUpdates), agent.GetName())
	}
	if response.Status == "failed" && response.Error != "" {
		return fmt.Errorf("agent execution failed: %s", response.Error)
	}
	return nil
}

func (e *WebhookExecutor) executeAsyncWebhook(ctx context.Context, agent Agent, want *Want) error {
	request := MonitorRequest{
		WantID:      want.Metadata.Name,
		AgentName:   agent.GetName(),
		CallbackURL: e.config.CallbackURL,
		WantState:   want.GetAllState(),
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

func (e *WebhookExecutor) executeMonitorWithSync(ctx context.Context, agent Agent, want *Want) error {
	latestState, err := e.fetchLatestWantState(want.Metadata.Name)
	if err != nil {
		log.Printf("[WEBHOOK] Failed to fetch latest state: %v, using current state", err)
		latestState = want.GetAllState()
	}
	request := MonitorRequest{
		WantID:      want.Metadata.Name,
		AgentName:   agent.GetName(),
		WantState:   latestState,
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
	log.Printf("[WEBHOOK] Executing MonitorAgent %s at %s (state fields: %d)", agent.GetName(), url, len(latestState))
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
	return nil
}

func (e *WebhookExecutor) fetchLatestWantState(wantID string) (map[string]any, error) {
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

func (e *WebhookExecutor) startMonitorLoop(ctx context.Context, agent Agent, want *Want) error {
	interval := time.Duration(e.config.MonitorIntervalMs) * time.Millisecond
	if interval <= 0 {
		interval = 30 * time.Second
	}
	agentName := agent.GetName()
	log.Printf("[WEBHOOK] Starting MonitorAgent %s loop (interval: %v)", agentName, interval)
	monitorCtx, cancel := context.WithCancel(ctx)
	want.RegisterRunningAgent(agentName, cancel)
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		defer want.UnregisterRunningAgent(agentName)
		if err := e.executeMonitorWithSync(monitorCtx, agent, want); err != nil {
			log.Printf("[WEBHOOK] MonitorAgent %s initial execution failed: %v", agentName, err)
		}
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
// Want — remote callback support (called by external agent services)
// ============================================================================

// SetRemoteCallback configures the want for remote execution mode with callback support.
func (n *Want) SetRemoteCallback(callbackURL, agentName string) {
	n.callbackURL = callbackURL
	n.agentName = agentName
	n.remoteMode = true
}

// SendCallback sends accumulated state changes to the callback URL asynchronously.
func (n *Want) SendCallback() error {
	if n.callbackURL == "" {
		return fmt.Errorf("callback URL not set")
	}
	changes := n.GetPendingStateChanges()
	if len(changes) == 0 {
		return nil
	}
	callback := WebhookCallback{
		AgentName:    n.agentName,
		WantID:       n.Metadata.Name,
		Status:       "state_changed",
		StateUpdates: changes,
	}
	go func() {
		body, err := json.Marshal(callback)
		if err != nil {
			log.Printf("[CALLBACK] Failed to marshal callback: %v", err)
			return
		}
		resp, err := http.Post(n.callbackURL, "application/json", bytes.NewReader(body))
		if err != nil {
			log.Printf("[CALLBACK] Failed to send callback to %s: %v", n.callbackURL, err)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
			log.Printf("[CALLBACK] Callback returned status %d", resp.StatusCode)
		}
	}()
	return nil
}
