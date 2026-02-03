package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	mywant "mywant/engine/src"
)

// ============================================================================
// Agent Service HTTP Handlers
// Serve registered agents via HTTP for external execution (Webhook/RPC mode)
// ============================================================================

// handleAgentServiceExecute executes registered DoAgent via HTTP
// POST /api/v1/agent-service/execute
func (s *Server) handleAgentServiceExecute(w http.ResponseWriter, r *http.Request) {
	// 1. Validate authentication
	if !s.validateAgentAuth(r) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// 2. Parse request
	var req mywant.ExecuteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	// 3. Get agent from global registry
	if s.globalBuilder == nil || s.globalBuilder.GetAgentRegistry() == nil {
		http.Error(w, "Agent registry not initialized", http.StatusInternalServerError)
		return
	}

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

	// 5. Execute the agent's Exec() method (same as local execution!)
	start := time.Now()
	ctx := r.Context()
	_, err := agent.Exec(ctx, want)

	// 6. Get only the changed fields
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

	// 8. Send response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("[AGENT-SERVICE] Failed to encode response: %v", err)
		return
	}

	log.Printf("[AGENT-SERVICE] Executed DoAgent %s (status: %s, changed: %d fields, duration: %dms)",
		req.AgentName, response.Status, len(stateUpdates), response.ExecutionTimeMs)
}

// handleAgentServiceMonitorExecute executes registered MonitorAgent one cycle via HTTP
// POST /api/v1/agent-service/monitor/execute
func (s *Server) handleAgentServiceMonitorExecute(w http.ResponseWriter, r *http.Request) {
	// 1. Validate authentication
	if !s.validateAgentAuth(r) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// 2. Parse request
	var req mywant.MonitorRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	// 3. Get agent from global registry
	if s.globalBuilder == nil || s.globalBuilder.GetAgentRegistry() == nil {
		http.Error(w, "Agent registry not initialized", http.StatusInternalServerError)
		return
	}

	agent, exists := s.globalBuilder.GetAgentRegistry().GetAgent(req.AgentName)
	if !exists {
		http.Error(w, fmt.Sprintf("Agent not found: %s", req.AgentName), http.StatusNotFound)
		return
	}

	// 4. Create want with latest state (synced before each cycle)
	want := &mywant.Want{
		Metadata: mywant.Metadata{Name: req.WantID},
		State:    req.WantState, // Latest state from server
	}

	// 5. Set callback configuration for remote execution
	want.SetRemoteCallback(req.CallbackURL, req.AgentName)

	want.BeginProgressCycle()

	// 6. Execute MonitorAgent one cycle
	start := time.Now()
	_, err := agent.Exec(context.Background(), want)

	// 7. Get changes and send callback if any
	stateUpdates := want.GetPendingStateChanges()
	if len(stateUpdates) > 0 {
		if callbackErr := want.SendCallback(); callbackErr != nil {
			log.Printf("[AGENT-SERVICE] Callback failed for %s: %v", req.AgentName, callbackErr)
		}
	}

	want.EndProgressCycle()

	// 8. Build response
	response := map[string]interface{}{
		"status":              "completed",
		"state_updates_count": len(stateUpdates),
		"execution_time_ms":   time.Since(start).Milliseconds(),
	}

	if err != nil {
		response["status"] = "failed"
		response["error"] = err.Error()
	}

	// 9. Send response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("[AGENT-SERVICE] Failed to encode response: %v", err)
		return
	}

	log.Printf("[AGENT-SERVICE] MonitorAgent %s executed (cycle: %dms, changes: %d)",
		req.AgentName, response["execution_time_ms"], len(stateUpdates))
}
