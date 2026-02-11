# Agent Execution Mode Implementation Plan

## Overview

This document outlines the implementation plan for the unified Agent execution system that supports multiple execution modes (Local, Webhook) with the same Agent implementation.

**Goal**: Enable the same Agent implementation to run in different execution modes by only changing ExecutionConfig, without modifying Want or Agent code.

## Design Principles

1. **Execution mode transparency**: Want and Agent implementations are not affected by execution mode changes
2. **Single implementation**: The same Agent Action function works in all execution modes
3. **Configuration-based switching**: Only ExecutionConfig needs to be changed
4. **Efficient state synchronization**: Sync state once per execution cycle for MonitorAgents

## Implementation Phases

### Phase 1: Foundation (Core Features) - PRIORITY: CRITICAL

#### 1.1 Want State Synchronization (2-3 hours)

**File**: `engine/core/want.go`

**Tasks**:

1. Add `GetPendingStateChanges()` method
   ```go
   func (n *Want) GetPendingStateChanges() map[string]any {
       n.stateMutex.RLock()
       defer n.stateMutex.RUnlock()

       if n.pendingStateChanges == nil {
           return make(map[string]any)
       }

       // Return a copy to avoid concurrent access issues
       changes := make(map[string]any, len(n.pendingStateChanges))
       for k, v := range n.pendingStateChanges {
           changes[k] = v
       }
       return changes
   }
   ```

2. Add `SetRemoteCallback()` method
   ```go
   func (n *Want) SetRemoteCallback(callbackURL, agentName string) {
       n.callbackURL = callbackURL
       n.agentName = agentName
       n.remoteMode = true
   }
   ```

3. Add `SendCallback()` method
   ```go
   func (n *Want) SendCallback() {
       if n.callbackURL == "" {
           return
       }

       changes := n.GetPendingStateChanges()
       if len(changes) == 0 {
           return
       }

       callback := WebhookCallback{
           AgentName:    n.agentName,
           WantID:       n.Metadata.Name,
           Status:       "state_changed",
           StateUpdates: changes,
       }

       go func() {
           body, _ := json.Marshal(callback)
           http.Post(n.callbackURL, "application/json", bytes.NewReader(body))
       }()
   }
   ```

4. Add fields to Want struct:
   - `remoteMode bool`
   - `callbackURL string`
   - `agentName string`

**Tests**: `engine/core/want_test.go`
- Test GetPendingStateChanges() with various scenarios
- Test concurrent access safety
- Test SetRemoteCallback() and SendCallback()

---

#### 1.2 Agent Service HTTP Implementation (4-5 hours)

**File**: `engine/cmd/server/agent_service_handlers.go` (new)

**Tasks**:

1. Implement `handleAgentServiceExecute` - DoAgent execution
   ```go
   func (s *Server) handleAgentServiceExecute(w http.ResponseWriter, r *http.Request) {
       // 1. Validate auth
       if !s.validateAgentAuth(r) {
           http.Error(w, "Unauthorized", http.StatusUnauthorized)
           return
       }

       // 2. Parse request
       var req mywant.ExecuteRequest
       if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
           http.Error(w, "Invalid request", http.StatusBadRequest)
           return
       }

       // 3. Get agent from registry
       agent, exists := s.globalBuilder.GetAgentRegistry().GetAgent(req.AgentName)
       if !exists {
           http.Error(w, fmt.Sprintf("Agent not found: %s", req.AgentName), http.StatusNotFound)
           return
       }

       // 4. Create temporary want with provided state
       want := &mywant.Want{
           Metadata: mywant.Metadata{Name: req.WantID},
           State:    req.WantState,
       }
       want.BeginProgressCycle()

       // 5. Execute the agent
       start := time.Now()
       ctx := r.Context()
       _, err := agent.Exec(ctx, want)

       // 6. Get only changed fields
       stateUpdates := want.GetPendingStateChanges()

       want.EndProgressCycle()

       // 7. Build response
       response := mywant.ExecuteResponse{
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

       log.Printf("[AGENT-SERVICE] Executed DoAgent %s (status: %s, changed: %d fields, duration: %dms)",
           req.AgentName, response.Status, len(stateUpdates), response.ExecutionTimeMs)
   }
   ```

2. Implement `handleAgentServiceMonitorExecute` - MonitorAgent single cycle execution
   ```go
   func (s *Server) handleAgentServiceMonitorExecute(w http.ResponseWriter, r *http.Request) {
       // 1. Validate auth
       if !s.validateAgentAuth(r) {
           http.Error(w, "Unauthorized", http.StatusUnauthorized)
           return
       }

       // 2. Parse request
       var req mywant.MonitorRequest
       if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
           http.Error(w, "Invalid request", http.StatusBadRequest)
           return
       }

       // 3. Get agent from registry
       agent, exists := s.globalBuilder.GetAgentRegistry().GetAgent(req.AgentName)
       if !exists {
           http.Error(w, fmt.Sprintf("Agent not found: %s", req.AgentName), http.StatusNotFound)
           return
       }

       // 4. Create want with latest state
       want := &mywant.Want{
           Metadata: mywant.Metadata{Name: req.WantID},
           State:    req.WantState,  // Latest state from server
       }

       // 5. Set callback configuration
       want.SetRemoteCallback(req.CallbackURL, req.AgentName)

       want.BeginProgressCycle()

       // 6. Execute MonitorAgent one cycle
       start := time.Now()
       _, err := agent.Exec(r.Context(), want)

       // 7. Get changes and send callback if any
       stateUpdates := want.GetPendingStateChanges()
       if len(stateUpdates) > 0 {
           want.SendCallback()  // Async callback
       }

       want.EndProgressCycle()

       // 8. Build response
       response := map[string]interface{}{
           "status":             "completed",
           "state_updates_count": len(stateUpdates),
           "execution_time_ms":   time.Since(start).Milliseconds(),
       }

       if err != nil {
           response["status"] = "failed"
           response["error"] = err.Error()
       }

       w.Header().Set("Content-Type", "application/json")
       json.NewEncoder(w).Encode(response)

       log.Printf("[AGENT-SERVICE] MonitorAgent %s executed (cycle: %dms, changes: %d)",
           req.AgentName, response["execution_time_ms"], len(stateUpdates))
   }
   ```

**File**: `engine/cmd/server/agent_api_handlers.go` (existing)

**Tasks**:

3. Add new routes to `registerAgentAPIRoutes()`
   ```go
   // Agent Service (serve registered agents via HTTP)
   s.router.HandleFunc("/api/v1/agent-service/execute",
       s.handleAgentServiceExecute).Methods("POST")
   s.router.HandleFunc("/api/v1/agent-service/monitor/execute",
       s.handleAgentServiceMonitorExecute).Methods("POST")

   log.Println("[ROUTES] Registered agent service routes:")
   log.Println("  POST /api/v1/agent-service/execute")
   log.Println("  POST /api/v1/agent-service/monitor/execute")
   ```

**Tests**: `engine/cmd/server/agent_service_handlers_test.go` (new)
- Test DoAgent execution endpoint
- Test MonitorAgent execution endpoint
- Test with mock agents
- Test error handling

---

#### 1.3 WebhookExecutor Improvements (3-4 hours)

**File**: `engine/core/agent_executor.go` (existing)

**Tasks**:

1. Update `executeSyncWebhook()` to use GetPendingStateChanges pattern
   - Already mostly correct, but ensure it uses response.StateUpdates properly

2. Add `executeMonitorWithSync()` method for MonitorAgent
   ```go
   func (e *WebhookExecutor) executeMonitorWithSync(ctx context.Context, agent Agent, want *Want) error {
       // 1. Fetch latest state from server
       latestState, err := e.fetchLatestWantState(want.Metadata.Name)
       if err != nil {
           log.Printf("[WEBHOOK] Failed to fetch latest state: %v", err)
           latestState = want.State  // Fallback to current state
       }

       // 2. Create request with latest state
       request := MonitorRequest{
           WantID:      want.Metadata.Name,
           AgentName:   agent.GetName(),
           WantState:   latestState,  // Latest state
           CallbackURL: e.config.CallbackURL,
       }

       body, _ := json.Marshal(request)
       url := e.config.ServiceURL + "/api/v1/agent-service/monitor/execute"

       req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
       req.Header.Set("Content-Type", "application/json")
       if e.config.AuthToken != "" {
           req.Header.Set("Authorization", "Bearer "+e.config.AuthToken)
       }

       resp, err := e.httpClient.Do(req)
       if err != nil {
           return fmt.Errorf("webhook request failed: %w", err)
       }
       defer resp.Body.Close()

       if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
           bodyBytes, _ := io.ReadAll(resp.Body)
           return fmt.Errorf("webhook returned status %d: %s", resp.StatusCode, string(bodyBytes))
       }

       log.Printf("[WEBHOOK] MonitorAgent %s executed with latest state (%d fields)",
           agent.GetName(), len(latestState))

       return nil
   }
   ```

3. Add `fetchLatestWantState()` helper method
   ```go
   func (e *WebhookExecutor) fetchLatestWantState(wantID string) (map[string]any, error) {
       // GET /api/v1/wants/{id}/state
       url := fmt.Sprintf("%s/api/v1/wants/%s/state", e.config.ServiceURL, wantID)

       req, _ := http.NewRequest("GET", url, nil)
       if e.config.AuthToken != "" {
           req.Header.Set("Authorization", "Bearer "+e.config.AuthToken)
       }

       resp, err := e.httpClient.Do(req)
       if err != nil {
           return nil, err
       }
       defer resp.Body.Close()

       var stateResp WantStateResponse
       if err := json.NewDecoder(resp.Body).Decode(&stateResp); err != nil {
           return nil, err
       }

       return stateResp.State, nil
   }
   ```

4. Update `Execute()` method to dispatch to appropriate handler
   ```go
   func (e *WebhookExecutor) Execute(ctx context.Context, agent Agent, want *Want) error {
       if agent.GetType() == DoAgentType {
           return e.executeSyncWebhook(ctx, agent, want)
       }
       return e.executeMonitorWithSync(ctx, agent, want)
   }
   ```

**Tests**: `engine/core/agent_executor_test.go` (existing)
- Test executeMonitorWithSync with mock server
- Test fetchLatestWantState
- Test state synchronization

---

### Phase 2: MonitorAgent Periodic Execution (3-4 hours)

**File**: `engine/core/agent_executor.go` (existing)

**Tasks**:

1. Add MonitorAgent periodic execution loop
   ```go
   func (e *WebhookExecutor) startMonitorLoop(ctx context.Context, agent Agent, want *Want, interval time.Duration) error {
       ticker := time.NewTicker(interval)
       defer ticker.Stop()

       for {
           select {
           case <-ticker.C:
               if err := e.executeMonitorWithSync(ctx, agent, want); err != nil {
                   log.Printf("[WEBHOOK] MonitorAgent %s cycle failed: %v", agent.GetName(), err)
               }
           case <-ctx.Done():
               log.Printf("[WEBHOOK] MonitorAgent %s loop stopped", agent.GetName())
               return ctx.Err()
           }
       }
   }
   ```

2. Update MonitorAgent execution to use periodic loop
   - Consider making this optional (one-shot vs continuous)
   - Add configuration for interval (default: 30 seconds)

3. Implement lifecycle management
   - Store running monitors in Want
   - Stop monitors when Want is stopped/deleted
   - Handle graceful shutdown

**Tests**: `engine/core/agent_executor_test.go`
- Test periodic execution
- Test lifecycle management
- Test graceful shutdown

---

### Phase 3: Integration Testing (4-5 hours)

**File**: `engine/core/agent_execution_e2e_test.go` (new)

**Tasks**:

1. E2E test: Same Agent in multiple execution modes
   ```go
   func TestAgentExecutionModes(t *testing.T) {
       // Common Agent implementation
       testAction := func(ctx context.Context, want *Want) error {
           departure, _ := want.GetState("departure")
           want.StoreState("booking_id", fmt.Sprintf("BOOK-%s", departure))
           want.StoreState("status", "confirmed")
           return nil
       }

       // Test Local execution
       t.Run("Local", func(t *testing.T) {
           agent := &DoAgent{
               BaseAgent: BaseAgent{
                   Name: "test_local",
                   ExecutionConfig: ExecutionConfig{Mode: ExecutionModeLocal},
               },
               Action: testAction,
           }

           want := &Want{State: map[string]any{"departure": "NRT"}}
           want.BeginProgressCycle()
           agent.Exec(context.Background(), want)
           want.EndProgressCycle()

           bookingID, _ := want.GetState("booking_id")
           assert.Equal(t, "BOOK-NRT", bookingID)
       })

       // Test Webhook execution
       t.Run("Webhook", func(t *testing.T) {
           // Start Agent Service
           registry := NewAgentRegistry()
           agent := &DoAgent{
               BaseAgent: BaseAgent{Name: "test_webhook"},
               Action: testAction,
           }
           registry.RegisterAgent(agent)

           server := httptest.NewServer(createAgentServiceHandler(registry))
           defer server.Close()

           // Execute via Webhook
           executor := NewWebhookExecutor(WebhookConfig{
               ServiceURL: server.URL,
           })

           want := &Want{State: map[string]any{"departure": "NRT"}}
           executor.Execute(context.Background(), agent, want)

           bookingID, _ := want.GetState("booking_id")
           assert.Equal(t, "BOOK-NRT", bookingID)
       })
   }
   ```

2. E2E test: MonitorAgent with state synchronization
   - Test that latest state is fetched before each cycle
   - Test callback is sent when state changes
   - Test periodic execution

3. E2E test: Error handling
   - Test agent execution failure
   - Test network failure
   - Test timeout handling

**Tests**: `engine/core/agent_execution_e2e_test.go`
- Multiple execution modes with same implementation
- MonitorAgent state sync and callbacks
- Error scenarios

---

### Phase 4: Documentation & Examples (Optional)

#### 4.1 Documentation Update (2-3 hours)

**Files**:
- `docs/AgentExecutionModes.md` (existing - update)
- `docs/agent-examples.md` (existing - update)
- `docs/WantDeveloperGuide.md` (existing - update)

**Tasks**:
- Document GetPendingStateChanges() usage
- Document Agent Service endpoints
- Add MonitorAgent state sync examples
- Add troubleshooting guide
- Update architecture diagrams

#### 4.2 Practical Examples (3-4 hours)

**File**: `engine/demos/demo_agent_modes/main.go` (new)

**Tasks**:
- Implement Flight Booking Agent in 3 modes
- Implement Flight Status Monitor (Webhook)
- Create demo showing execution mode switching
- Add README with usage instructions

---

## Task List

### Phase 1.1: Want State Synchronization
- [ ] #1: Implement Want.GetPendingStateChanges() method
- [ ] #2: Implement Want.SetRemoteCallback() method
- [ ] #3: Implement Want.SendCallback() method
- [ ] #4: Add remoteMode, callbackURL, agentName fields to Want
- [ ] #5: Write tests for state synchronization functions

### Phase 1.2: Agent Service HTTP
- [ ] #6: Create agent_service_handlers.go file
- [ ] #7: Implement handleAgentServiceExecute
- [ ] #8: Implement handleAgentServiceMonitorExecute
- [ ] #9: Register new routes in registerAgentAPIRoutes
- [ ] #10: Write tests for agent service handlers

### Phase 1.3: WebhookExecutor Improvements
- [ ] #11: Add executeMonitorWithSync method
- [ ] #12: Add fetchLatestWantState helper
- [ ] #13: Update Execute method dispatch logic
- [ ] #14: Write tests for WebhookExecutor improvements

### Phase 2: MonitorAgent Periodic Execution
- [ ] #15: Implement startMonitorLoop method
- [ ] #16: Add lifecycle management
- [ ] #17: Write tests for periodic execution

### Phase 3: Integration Testing
- [ ] #18: Write E2E test for multiple execution modes
- [ ] #19: Write E2E test for MonitorAgent state sync
- [ ] #20: Write E2E test for error handling

### Phase 4: Documentation & Examples (Optional)
- [ ] #21: Update AgentExecutionModes.md
- [ ] #22: Update agent-examples.md
- [ ] #23: Create demo_agent_modes example

---

## Implementation Schedule

| Phase | Task | Time | Priority |
|-------|------|------|----------|
| 1.1 | Want State Sync | 2-3h | 🔴 CRITICAL |
| 1.2 | Agent Service HTTP | 4-5h | 🔴 CRITICAL |
| 1.3 | Executor Improvements | 3-4h | 🔴 CRITICAL |
| 2 | Monitor Periodic Exec | 3-4h | 🟡 RECOMMENDED |
| 3 | Integration Tests | 4-5h | 🔴 CRITICAL |
| 4 | Docs & Examples | 5-7h | 🟢 OPTIONAL |

**Minimum Implementation (MVP)**: Phase 1 + Basic tests = ~12 hours
**Recommended Implementation**: Phase 1-3 = ~20 hours
**Complete Implementation**: All phases = ~27 hours

---

## Pre-Implementation Checklist

- [ ] All existing tests pass
- [ ] Review go.mod dependencies
- [ ] Create Git branch: `feature/agent-execution-modes`
- [ ] Backup current codebase
- [ ] Read existing code: want.go, agent_executor.go, want_agent.go

---

## Success Criteria

### Phase 1 Complete
- [ ] GetPendingStateChanges() returns only changed fields
- [ ] Agent Service endpoints respond correctly
- [ ] WebhookExecutor fetches latest state before MonitorAgent execution
- [ ] Basic tests pass

### Phase 2 Complete
- [ ] MonitorAgent executes periodically
- [ ] State is synchronized each cycle
- [ ] Callbacks are sent on state changes
- [ ] Lifecycle is managed properly

### Phase 3 Complete
- [ ] Same Agent implementation works in Local and Webhook modes
- [ ] E2E tests pass
- [ ] Error handling works correctly

---

## Notes

- **No Connect/gRPC**: Focus on Webhook (HTTP/JSON) implementation only
- **State Sync Strategy**: Fetch latest state once per execution cycle (efficient)
- **Callback Pattern**: Asynchronous callbacks for state changes
- **Testing Priority**: Integration tests are critical for reliability

---

## Next Steps

1. Start with Phase 1.1 (Want State Synchronization)
2. Implement and test each task sequentially
3. Move to Phase 1.2 after Phase 1.1 is complete and tested
4. Continue through phases in order

---

**Document Version**: 1.0
**Last Updated**: 2026-02-02
**Status**: Ready to implement
