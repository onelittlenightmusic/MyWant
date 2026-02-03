package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/mux"
	mywant "mywant/engine/src"
)

// ============================================================================
// Authentication
// ============================================================================

// validateAgentAuth validates Bearer token authentication for external agents
func (s *Server) validateAgentAuth(r *http.Request) bool {
	expectedToken := os.Getenv("WEBHOOK_AUTH_TOKEN")

	// If no token configured, allow all requests (development mode)
	if expectedToken == "" {
		log.Printf("[AUTH] Warning: WEBHOOK_AUTH_TOKEN not set, allowing all agent requests")
		return true
	}

	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return false
	}

	// Check for Bearer token
	const prefix = "Bearer "
	if len(authHeader) < len(prefix) {
		return false
	}

	token := authHeader[len(prefix):]
	return token == expectedToken
}

// ============================================================================
// Want State API (for external agents to query/update state)
// ============================================================================

// handleGetWantState returns the current state of a want
// GET /api/v1/wants/{id}/state
func (s *Server) handleGetWantState(w http.ResponseWriter, r *http.Request) {
	if !s.validateAgentAuth(r) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	wantID := vars["id"]

	want := s.findWant(wantID)
	if want == nil {
		http.Error(w, fmt.Sprintf("Want not found: %s", wantID), http.StatusNotFound)
		return
	}

	response := mywant.WantStateResponse{
		WantID:    wantID,
		State:     want.State,
		Status:    string(want.Status),
		Timestamp: time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)

	log.Printf("[AGENT API] Get state for want %s (keys: %d)", wantID, len(want.State))
}

// handleUpdateWantState updates the state of a want
// POST /api/v1/wants/{id}/state
func (s *Server) handleUpdateWantState(w http.ResponseWriter, r *http.Request) {
	if !s.validateAgentAuth(r) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	wantID := vars["id"]

	var req mywant.StateUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	want := s.findWant(wantID)
	if want == nil {
		http.Error(w, fmt.Sprintf("Want not found: %s", wantID), http.StatusNotFound)
		return
	}

	// Apply state updates
	want.BeginProgressCycle()
	updatedKeys := make([]string, 0, len(req.StateUpdates))
	for key, value := range req.StateUpdates {
		want.StoreState(key, value)
		updatedKeys = append(updatedKeys, key)
	}
	want.EndProgressCycle()

	// Record agent activity in history
	if req.AgentName != "" {
		want.SetAgentActivity(req.AgentName, fmt.Sprintf("Updated %d state fields", len(req.StateUpdates)))
	}

	response := map[string]interface{}{
		"status":       "ok",
		"updated_keys": updatedKeys,
		"count":        len(updatedKeys),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)

	log.Printf("[AGENT API] Updated state for want %s by agent %s (%d keys)", wantID, req.AgentName, len(updatedKeys))
}

// ============================================================================
// Webhook Callback API (for external agents to report results)
// ============================================================================

// handleWebhookCallback receives callbacks from external agents
// POST /api/v1/agents/webhook/callback
func (s *Server) handleWebhookCallback(w http.ResponseWriter, r *http.Request) {
	if !s.validateAgentAuth(r) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var callback mywant.WebhookCallback
	if err := json.NewDecoder(r.Body).Decode(&callback); err != nil {
		http.Error(w, fmt.Sprintf("Invalid callback payload: %v", err), http.StatusBadRequest)
		return
	}

	want := s.findWant(callback.WantID)
	if want == nil {
		http.Error(w, fmt.Sprintf("Want not found: %s", callback.WantID), http.StatusNotFound)
		return
	}

	log.Printf("[WEBHOOK CALLBACK] Received from agent %s for want %s (status: %s)",
		callback.AgentName, callback.WantID, callback.Status)

	// Apply state updates if provided
	if len(callback.StateUpdates) > 0 {
		want.BeginProgressCycle()
		for key, value := range callback.StateUpdates {
			want.StoreState(key, value)
		}
		want.EndProgressCycle()
		log.Printf("[WEBHOOK CALLBACK] Applied %d state updates", len(callback.StateUpdates))
	}

	// Update agent execution record
	s.updateAgentExecutionStatus(want, callback.AgentName, callback.Status, callback.Error)

	// Set agent activity
	if callback.Status == "completed" {
		want.SetAgentActivity(callback.AgentName, "Execution completed via webhook")
	} else if callback.Status == "state_changed" {
		want.SetAgentActivity(callback.AgentName, "State changed via webhook monitor")
	} else if callback.Status == "failed" {
		want.SetAgentActivity(callback.AgentName, fmt.Sprintf("Failed: %s", callback.Error))
	}

	response := map[string]string{
		"status":  "accepted",
		"want_id": callback.WantID,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// updateAgentExecutionStatus updates the agent execution record in history
func (s *Server) updateAgentExecutionStatus(want *mywant.Want, agentName, status, errorMsg string) {
	if want.History.AgentHistory == nil {
		return
	}

	// Find the most recent execution record for this agent
	for i := len(want.History.AgentHistory) - 1; i >= 0; i-- {
		exec := &want.History.AgentHistory[i]
		if exec.AgentName == agentName && exec.Status == "running" {
			// Update status
			switch status {
			case "completed":
				exec.Status = "achieved"
			case "failed":
				exec.Status = "failed"
				exec.Error = errorMsg
			case "state_changed":
				// Monitor agent detected state change, keep running
				exec.Activity = "State change detected"
			}

			// Set end time for completed/failed
			if status == "completed" || status == "failed" {
				now := time.Now()
				exec.EndTime = &now
			}

			log.Printf("[AGENT HISTORY] Updated agent %s status to %s", agentName, exec.Status)
			break
		}
	}
}

// ============================================================================
// Agent Execution API (manual agent triggering)
// ============================================================================

// handleAgentExecute manually triggers agent execution
// POST /api/v1/wants/{id}/agents/{agentName}/execute
func (s *Server) handleAgentExecute(w http.ResponseWriter, r *http.Request) {
	if !s.validateAgentAuth(r) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	wantID := vars["id"]
	agentName := vars["agentName"]

	var req struct {
		Operation string         `json:"operation"`
		Params    map[string]any `json:"params"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	want := s.findWant(wantID)
	if want == nil {
		http.Error(w, fmt.Sprintf("Want not found: %s", wantID), http.StatusNotFound)
		return
	}

	// Get agent from registry
	agent, exists := want.GetAgentRegistry().GetAgent(agentName)
	if !exists {
		http.Error(w, fmt.Sprintf("Agent not found: %s", agentName), http.StatusNotFound)
		return
	}

	// Execute agent asynchronously using ExecuteAgents
	// We need to temporarily set the requirements to match the agent
	originalRequires := want.Spec.Requires
	want.Spec.Requires = agent.GetCapabilities()

	go func() {
		if err := want.ExecuteAgents(); err != nil {
			log.Printf("[AGENT API] Failed to execute agent %s: %v", agentName, err)
		}
		// Restore original requirements
		want.Spec.Requires = originalRequires
	}()

	executionID := fmt.Sprintf("exec-%d", time.Now().UnixNano())
	response := map[string]string{
		"execution_id": executionID,
		"status":       "started",
		"agent_name":   agentName,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)

	log.Printf("[AGENT API] Triggered execution of agent %s for want %s", agentName, wantID)
}

// ============================================================================
// Helper Methods
// ============================================================================

// findWant finds a want by ID from all executions or global builder
func (s *Server) findWant(wantID string) *mywant.Want {
	// Check all executions first
	for _, execution := range s.wants {
		wants := execution.Builder.GetAllWantStates()
		for _, want := range wants {
			if want.Metadata.Name == wantID || want.Metadata.ID == wantID {
				return want
			}
		}
	}

	// Check global builder
	if s.globalBuilder != nil {
		wants := s.globalBuilder.GetAllWantStates()
		for _, want := range wants {
			if want.Metadata.Name == wantID || want.Metadata.ID == wantID {
				return want
			}
		}
	}

	return nil
}

// ============================================================================
// Route Registration
// ============================================================================

// registerAgentAPIRoutes registers all agent-related API routes
func (s *Server) registerAgentAPIRoutes() {
	// Want state query/update (for external agents)
	s.router.HandleFunc("/api/v1/wants/{id}/state", s.handleGetWantState).Methods("GET")
	s.router.HandleFunc("/api/v1/wants/{id}/state", s.handleUpdateWantState).Methods("POST")

	// Webhook callback (for external agents to report results)
	s.router.HandleFunc("/api/v1/agents/webhook/callback", s.handleWebhookCallback).Methods("POST")

	// Manual agent execution
	s.router.HandleFunc("/api/v1/wants/{id}/agents/{agentName}/execute", s.handleAgentExecute).Methods("POST")

	// Agent Service (serve registered agents via HTTP)
	s.router.HandleFunc("/api/v1/agent-service/execute", s.handleAgentServiceExecute).Methods("POST")
	s.router.HandleFunc("/api/v1/agent-service/monitor/execute", s.handleAgentServiceMonitorExecute).Methods("POST")

	log.Println("[ROUTES] Registered agent API routes:")
	log.Println("  GET  /api/v1/wants/{id}/state")
	log.Println("  POST /api/v1/wants/{id}/state")
	log.Println("  POST /api/v1/agents/webhook/callback")
	log.Println("  POST /api/v1/wants/{id}/agents/{agentName}/execute")
	log.Println("[ROUTES] Registered agent service routes:")
	log.Println("  POST /api/v1/agent-service/execute")
	log.Println("  POST /api/v1/agent-service/monitor/execute")
}
