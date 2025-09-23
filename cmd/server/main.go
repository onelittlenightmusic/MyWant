package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	mywant "mywant/src"
	types "mywant/cmd/types"
)

// ServerConfig holds server configuration
type ServerConfig struct {
	Port int    `json:"port"`
	Host string `json:"host"`
}

// ErrorHistoryEntry represents an API error with detailed context
type ErrorHistoryEntry struct {
	ID          string                 `json:"id"`
	Timestamp   string                 `json:"timestamp"`
	Message     string                 `json:"message"`
	Status      int                    `json:"status"`
	Code        string                 `json:"code,omitempty"`
	Type        string                 `json:"type,omitempty"`
	Details     string                 `json:"details,omitempty"`
	Endpoint    string                 `json:"endpoint"`
	Method      string                 `json:"method"`
	RequestData interface{}            `json:"request_data,omitempty"`
	UserAgent   string                 `json:"user_agent,omitempty"`
	Resolved    bool                   `json:"resolved"`
	Notes       string                 `json:"notes,omitempty"`
}

// Server represents the MyWant server
type Server struct {
	config        ServerConfig
	wants         map[string]*WantExecution // Store active want executions
	agentRegistry *mywant.AgentRegistry     // Agent and capability registry
	errorHistory  []ErrorHistoryEntry       // Store error history
	router        *mux.Router
}

// WantExecution represents a running want execution
type WantExecution struct {
	ID      string                 `json:"id"`
	Config  mywant.Config          `json:"config"` // Changed from pointer
	Status  string                 `json:"status"` // "running", "completed", "failed"
	Results map[string]interface{} `json:"results,omitempty"`
	Builder *mywant.ChainBuilder   `json:"-"` // Don't serialize builder
}

// NewServer creates a new MyWant server
func NewServer(config ServerConfig) *Server {
	// Create agent registry and load existing capabilities/agents
	agentRegistry := mywant.NewAgentRegistry()

	// Load capabilities and agents from directories if they exist
	if err := agentRegistry.LoadCapabilities("capabilities/"); err != nil {
		fmt.Printf("[SERVER] Warning: Failed to load capabilities: %v\n", err)
	}

	if err := agentRegistry.LoadAgents("agents/"); err != nil {
		fmt.Printf("[SERVER] Warning: Failed to load agents: %v\n", err)
	}

	return &Server{
		config:        config,
		wants:         make(map[string]*WantExecution),
		agentRegistry: agentRegistry,
		errorHistory:  make([]ErrorHistoryEntry, 0),
		router:        mux.NewRouter(),
	}
}

// setupRoutes configures the HTTP routes
func (s *Server) setupRoutes() {
	api := s.router.PathPrefix("/api/v1").Subrouter()

	// Wants CRUD endpoints
	wants := api.PathPrefix("/wants").Subrouter()
	wants.HandleFunc("", s.createWant).Methods("POST")
	wants.HandleFunc("", s.listWants).Methods("GET")
	wants.HandleFunc("/{id}", s.getWant).Methods("GET")
	wants.HandleFunc("/{id}", s.updateWant).Methods("PUT")
	wants.HandleFunc("/{id}", s.deleteWant).Methods("DELETE")
	wants.HandleFunc("/{id}/status", s.getWantStatus).Methods("GET")
	wants.HandleFunc("/{id}/results", s.getWantResults).Methods("GET")
	wants.HandleFunc("/{id}/suspend", s.suspendWant).Methods("POST")
	wants.HandleFunc("/{id}/resume", s.resumeWant).Methods("POST")

	// Agents CRUD endpoints
	agents := api.PathPrefix("/agents").Subrouter()
	agents.HandleFunc("", s.createAgent).Methods("POST")
	agents.HandleFunc("", s.listAgents).Methods("GET")
	agents.HandleFunc("/{name}", s.getAgent).Methods("GET")
	agents.HandleFunc("/{name}", s.deleteAgent).Methods("DELETE")

	// Capabilities CRUD endpoints
	capabilities := api.PathPrefix("/capabilities").Subrouter()
	capabilities.HandleFunc("", s.createCapability).Methods("POST")
	capabilities.HandleFunc("", s.listCapabilities).Methods("GET")
	capabilities.HandleFunc("/{name}", s.getCapability).Methods("GET")
	capabilities.HandleFunc("/{name}", s.deleteCapability).Methods("DELETE")
	capabilities.HandleFunc("/{name}/agents", s.findAgentsByCapability).Methods("GET")

	// Error history endpoints
	errors := api.PathPrefix("/errors").Subrouter()
	errors.HandleFunc("", s.listErrorHistory).Methods("GET")
	errors.HandleFunc("/{id}", s.getErrorHistoryEntry).Methods("GET")
	errors.HandleFunc("/{id}", s.updateErrorHistoryEntry).Methods("PUT")
	errors.HandleFunc("/{id}", s.deleteErrorHistoryEntry).Methods("DELETE")

	// Health check endpoint
	s.router.HandleFunc("/health", s.healthCheck).Methods("GET")

	// Serve static files (for future web UI)
	s.router.PathPrefix("/").Handler(http.FileServer(http.Dir("./web/"))).Methods("GET")
}

// createWant handles POST /api/v1/wants - creates a new want configuration
func (s *Server) createWant(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Parse YAML config from request body
	var configYAML []byte
	if r.Header.Get("Content-Type") == "application/yaml" || r.Header.Get("Content-Type") == "text/yaml" {
		// Read raw YAML
		configYAML = make([]byte, r.ContentLength)
		r.Body.Read(configYAML)
	} else {
		// Expect JSON with yaml field
		var request struct {
			YAML string `json:"yaml"`
			Name string `json:"name,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
			return
		}
		configYAML = []byte(request.YAML)
	}

	// Parse YAML config
	config, err := mywant.LoadConfigFromYAMLBytes(configYAML)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid YAML config: %v", err), http.StatusBadRequest)
		return
	}

	// Validate want types before proceeding
	if err := s.validateWantTypes(config); err != nil {
		errorMsg := fmt.Sprintf("Invalid want types: %v", err)
		s.logError(r, http.StatusBadRequest, errorMsg, "validation", err.Error(), string(configYAML))
		http.Error(w, errorMsg, http.StatusBadRequest)
		return
	}

	// Generate unique ID for this want execution
	wantID := generateWantID()

	// Assign individual IDs to each want if not already set
	baseID := time.Now().UnixNano()
	for i := range config.Wants {
		if config.Wants[i].Metadata.ID == "" {
			config.Wants[i].Metadata.ID = fmt.Sprintf("want-%d", baseID+int64(i))
		}
	}

	// Create want execution
	execution := &WantExecution{
		ID:     wantID,
		Config: config,
		Status: "created",
	}

	// Store the execution
	s.wants[wantID] = execution

	// Auto-execute for demo (as requested - run demo qnet)
	go s.executeWantAsync(wantID)

	// Return created want
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(execution)
}

// listWants handles GET /api/v1/wants - lists all wants in memory dump format
func (s *Server) listWants(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Collect all wants from all executions in memory dump format
	allWants := make([]*mywant.Want, 0)

	for _, execution := range s.wants {
		if execution.Builder != nil {
			// Get current want states from the builder (map[string]*Want)
			currentStates := execution.Builder.GetAllWantStates()
			for _, want := range currentStates {
				// Ensure state is populated by calling GetAllState()
				if want.State == nil {
					want.State = make(map[string]interface{})
				}
				// Get current runtime state and update the want's state field
				currentState := want.GetAllState()
				for k, v := range currentState {
					want.State[k] = v
				}
				allWants = append(allWants, want)
			}
		} else {
			// If no builder yet, use the original config wants
			allWants = append(allWants, execution.Config.Wants...)
		}
	}

	// Create memory dump format response
	response := map[string]interface{}{
		"timestamp":    time.Now().Format(time.RFC3339),
		"execution_id": fmt.Sprintf("api-dump-%d", time.Now().Unix()),
		"wants":        allWants,
	}

	json.NewEncoder(w).Encode(response)
}

// getWant handles GET /api/v1/wants/{id} - gets wants for a specific execution
func (s *Server) getWant(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	wantID := vars["id"]

	execution, exists := s.wants[wantID]
	if !exists {
		http.Error(w, "Want not found", http.StatusNotFound)
		return
	}

	// Collect wants from this specific execution
	allWants := make([]*mywant.Want, 0)

	if execution.Builder != nil {
		// Get current want states from the builder (map[string]*Want)
		currentStates := execution.Builder.GetAllWantStates()
		for _, want := range currentStates {
			// Ensure state is populated by calling GetAllState()
			if want.State == nil {
				want.State = make(map[string]interface{})
			}
			// Get current runtime state and update the want's state field
			currentState := want.GetAllState()
			for k, v := range currentState {
				want.State[k] = v
			}
			allWants = append(allWants, want)
		}
	} else {
		// If no builder yet, use the original config wants
		allWants = append(allWants, execution.Config.Wants...)
	}

	// Return just the wants array (single want objects)
	json.NewEncoder(w).Encode(allWants)
}

// updateWant handles PUT /api/v1/wants/{id} - updates a want configuration
func (s *Server) updateWant(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	wantID := vars["id"]

	want, exists := s.wants[wantID]
	if !exists {
		http.Error(w, "Want not found", http.StatusNotFound)
		return
	}

	// Only allow updates if not currently running
	if want.Status == "running" {
		http.Error(w, "Cannot update running want", http.StatusConflict)
		return
	}

	// Parse updated config
	var configYAML []byte
	if r.Header.Get("Content-Type") == "application/yaml" || r.Header.Get("Content-Type") == "text/yaml" {
		configYAML = make([]byte, r.ContentLength)
		r.Body.Read(configYAML)
	} else {
		var request struct {
			YAML string `json:"yaml"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
			return
		}
		configYAML = []byte(request.YAML)
	}

	config, err := mywant.LoadConfigFromYAMLBytes(configYAML)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid YAML config: %v", err), http.StatusBadRequest)
		return
	}

	// Validate want types before proceeding
	if err := s.validateWantTypes(config); err != nil {
		errorMsg := fmt.Sprintf("Invalid want types: %v", err)
		s.logError(r, http.StatusBadRequest, errorMsg, "validation", err.Error(), string(configYAML))
		http.Error(w, errorMsg, http.StatusBadRequest)
		return
	}

	// Update config
	want.Config = config
	want.Status = "updated"
	want.Results = nil // Clear previous results

	json.NewEncoder(w).Encode(want)
}

// deleteWant handles DELETE /api/v1/wants/{id} - deletes a want
func (s *Server) deleteWant(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	wantID := vars["id"]

	want, exists := s.wants[wantID]
	if !exists {
		http.Error(w, "Want not found", http.StatusNotFound)
		return
	}

	// Only allow deletion if not currently running
	if want.Status == "running" {
		http.Error(w, "Cannot delete running want", http.StatusConflict)
		return
	}

	delete(s.wants, wantID)
	w.WriteHeader(http.StatusNoContent)
}

// getWantStatus handles GET /api/v1/wants/{id}/status - gets want execution status
func (s *Server) getWantStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	wantID := vars["id"]

	want, exists := s.wants[wantID]
	if !exists {
		http.Error(w, "Want not found", http.StatusNotFound)
		return
	}

	status := map[string]interface{}{
		"id":     want.ID,
		"status": want.Status,
	}

	// Add suspension state if builder exists
	if want.Builder != nil {
		status["suspended"] = want.Builder.IsSuspended()
	} else {
		status["suspended"] = false
	}

	json.NewEncoder(w).Encode(status)
}

// getWantResults handles GET /api/v1/wants/{id}/results - gets want execution results
func (s *Server) getWantResults(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	wantID := vars["id"]

	want, exists := s.wants[wantID]
	if !exists {
		http.Error(w, "Want not found", http.StatusNotFound)
		return
	}

	if want.Results == nil {
		want.Results = make(map[string]interface{})
	}

	json.NewEncoder(w).Encode(want.Results)
}

// executeWantAsync executes a want configuration asynchronously
func (s *Server) executeWantAsync(wantID string) {
	want := s.wants[wantID]
	if want == nil {
		return
	}

	want.Status = "running"

	// Create chain builder (automatically registers owner want types)
	builder := mywant.NewChainBuilder(want.Config)
	want.Builder = builder

	// Set agent registry for agent-enabled wants
	builder.SetAgentRegistry(s.agentRegistry)

	// Register want types
	types.RegisterQNetWantTypes(builder)              // QNet types (qnet numbers, qnet queue, qnet sink, etc.)
	types.RegisterFibonacciWantTypes(builder)         // Fibonacci types (fibonacci_numbers, fibonacci_sequence)
	types.RegisterPrimeWantTypes(builder)             // Prime types (prime_numbers, prime_sieve)
	types.RegisterTravelWantTypes(builder)            // Travel types (restaurant, hotel, buffet, travel_coordinator)
	mywant.RegisterMonitorWantTypes(builder)          // Monitor types (monitor, alert)

	// Execute the chain
	fmt.Printf("[SERVER] Executing want %s with %d wants\n", wantID, len(want.Config.Wants))

	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("[SERVER] Want %s execution failed: %v\n", wantID, r)
			want.Status = "failed"
			want.Results = map[string]interface{}{
				"error": fmt.Sprintf("Execution failed: %v", r),
			}
		}
	}()

	// Execute with error handling
	err := func() error {
		defer func() {
			if r := recover(); r != nil {
				want.Status = "failed"
				want.Results = map[string]interface{}{
					"error": fmt.Sprintf("Panic during execution: %v", r),
				}
			}
		}()

		builder.Execute()
		return nil
	}()

	if err != nil {
		want.Status = "failed"
		want.Results = map[string]interface{}{
			"error": err.Error(),
		}
		return
	}

	// Collect results
	want.Status = "completed"
	want.Results = make(map[string]interface{})

	// Get final states
	states := builder.GetAllWantStates()
	want.Results["final_states"] = states
	want.Results["want_count"] = len(states)

	// Add execution summary
	completedCount := 0
	for _, state := range states {
		if state.Status == "completed" {
			completedCount++
		}
	}

	want.Results["summary"] = map[string]interface{}{
		"total_wants":     len(states),
		"completed_wants": completedCount,
		"status":          want.Status,
	}

	fmt.Printf("[SERVER] Want %s execution completed successfully\n", wantID)
}

// healthCheck handles GET /health - server health check
func (s *Server) healthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	health := map[string]interface{}{
		"status":  "healthy",
		"wants":   len(s.wants),
		"version": "1.0.0",
		"server":  "mywant",
	}
	json.NewEncoder(w).Encode(health)
}

// ======= AGENT CRUD HANDLERS =======

// createAgent handles POST /api/v1/agents - creates a new agent
func (s *Server) createAgent(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var agentData struct {
		Name         string   `json:"name"`
		Type         string   `json:"type"`
		Capabilities []string `json:"capabilities"`
		Uses         []string `json:"uses"`
	}

	if err := json.NewDecoder(r.Body).Decode(&agentData); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	// Create agent based on type
	var agent mywant.Agent
	baseAgent := mywant.BaseAgent{
		Name:         agentData.Name,
		Capabilities: agentData.Capabilities,
		Uses:         agentData.Uses,
		Type:         mywant.AgentType(agentData.Type),
	}

	switch agentData.Type {
	case "do":
		agent = &mywant.DoAgent{
			BaseAgent: baseAgent,
			Action:    nil, // Default action will be set by registry
		}
	case "monitor":
		agent = &mywant.MonitorAgent{
			BaseAgent: baseAgent,
			Monitor:   nil, // Default monitor will be set by registry
		}
	default:
		http.Error(w, "Invalid agent type. Must be 'do' or 'monitor'", http.StatusBadRequest)
		return
	}

	// Register the agent
	s.agentRegistry.RegisterAgent(agent)

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"name":         agent.GetName(),
		"type":         agent.GetType(),
		"capabilities": agent.GetCapabilities(),
		"uses":         agent.GetUses(),
	})
}

// listAgents handles GET /api/v1/agents - lists all agents
func (s *Server) listAgents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Since AgentRegistry doesn't have a GetAllAgents method, we'll access via reflection
	// In a production system, you'd add this method to AgentRegistry
	agents := make([]map[string]interface{}, 0)

	// We need to add a method to AgentRegistry to list all agents
	// For now, we'll return a message indicating this
	response := map[string]interface{}{
		"message": "Agent listing requires GetAllAgents method to be added to AgentRegistry",
		"agents":  agents,
	}

	json.NewEncoder(w).Encode(response)
}

// getAgent handles GET /api/v1/agents/{name} - gets a specific agent
func (s *Server) getAgent(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	agentName := vars["name"]

	agent, exists := s.agentRegistry.GetAgent(agentName)
	if !exists {
		http.Error(w, "Agent not found", http.StatusNotFound)
		return
	}

	response := map[string]interface{}{
		"name":         agent.GetName(),
		"type":         agent.GetType(),
		"capabilities": agent.GetCapabilities(),
		"uses":         agent.GetUses(),
	}

	json.NewEncoder(w).Encode(response)
}

// deleteAgent handles DELETE /api/v1/agents/{name} - deletes an agent
func (s *Server) deleteAgent(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	agentName := vars["name"]

	// Check if agent exists
	_, exists := s.agentRegistry.GetAgent(agentName)
	if !exists {
		http.Error(w, "Agent not found", http.StatusNotFound)
		return
	}

	// AgentRegistry doesn't have a Delete method, so we'll return a message
	// In production, you'd add UnregisterAgent method to AgentRegistry
	response := map[string]interface{}{
		"message": "Agent deletion requires UnregisterAgent method to be added to AgentRegistry",
		"agent":   agentName,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// ======= CAPABILITY CRUD HANDLERS =======

// createCapability handles POST /api/v1/capabilities - creates a new capability
func (s *Server) createCapability(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var capability mywant.Capability
	if err := json.NewDecoder(r.Body).Decode(&capability); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	// Register the capability
	s.agentRegistry.RegisterCapability(capability)

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(capability)
}

// listCapabilities handles GET /api/v1/capabilities - lists all capabilities
func (s *Server) listCapabilities(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Since AgentRegistry doesn't have a GetAllCapabilities method
	// we'll return a message indicating this needs to be implemented
	capabilities := make([]mywant.Capability, 0)

	response := map[string]interface{}{
		"message":      "Capability listing requires GetAllCapabilities method to be added to AgentRegistry",
		"capabilities": capabilities,
	}

	json.NewEncoder(w).Encode(response)
}

// getCapability handles GET /api/v1/capabilities/{name} - gets a specific capability
func (s *Server) getCapability(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	capabilityName := vars["name"]

	capability, exists := s.agentRegistry.GetCapability(capabilityName)
	if !exists {
		http.Error(w, "Capability not found", http.StatusNotFound)
		return
	}

	json.NewEncoder(w).Encode(capability)
}

// deleteCapability handles DELETE /api/v1/capabilities/{name} - deletes a capability
func (s *Server) deleteCapability(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	capabilityName := vars["name"]

	// Check if capability exists
	_, exists := s.agentRegistry.GetCapability(capabilityName)
	if !exists {
		http.Error(w, "Capability not found", http.StatusNotFound)
		return
	}

	// AgentRegistry doesn't have a Delete method for capabilities
	response := map[string]interface{}{
		"message":    "Capability deletion requires UnregisterCapability method to be added to AgentRegistry",
		"capability": capabilityName,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// findAgentsByCapability handles GET /api/v1/capabilities/{name}/agents - finds agents by capability
func (s *Server) findAgentsByCapability(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	capabilityName := vars["name"]

	agents := s.agentRegistry.FindAgentsByGives(capabilityName)
	if agents == nil {
		agents = make([]mywant.Agent, 0)
	}

	// Convert agents to response format
	agentResponses := make([]map[string]interface{}, len(agents))
	for i, agent := range agents {
		agentResponses[i] = map[string]interface{}{
			"name":         agent.GetName(),
			"type":         agent.GetType(),
			"capabilities": agent.GetCapabilities(),
			"uses":         agent.GetUses(),
		}
	}

	response := map[string]interface{}{
		"capability": capabilityName,
		"agents":     agentResponses,
	}

	json.NewEncoder(w).Encode(response)
}

// suspendWant handles POST /api/v1/wants/{id}/suspend - suspends want execution
func (s *Server) suspendWant(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Extract want ID from URL path
	vars := mux.Vars(r)
	wantID := vars["id"]

	// Find the want execution
	execution, exists := s.wants[wantID]
	if !exists {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{
			"error": fmt.Sprintf("Want execution with ID %s not found", wantID),
		})
		return
	}

	// Check if builder exists
	if execution.Builder == nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "Want execution has no active builder to suspend",
		})
		return
	}

	// Suspend the execution
	if err := execution.Builder.Suspend(); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error": fmt.Sprintf("Failed to suspend want execution: %v", err),
		})
		return
	}

	// Return success response
	response := map[string]interface{}{
		"message":   "Want execution suspended successfully",
		"wantId":    wantID,
		"suspended": true,
		"timestamp": time.Now().Format(time.RFC3339),
	}
	json.NewEncoder(w).Encode(response)
}

// resumeWant handles POST /api/v1/wants/{id}/resume - resumes want execution
func (s *Server) resumeWant(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Extract want ID from URL path
	vars := mux.Vars(r)
	wantID := vars["id"]

	// Find the want execution
	execution, exists := s.wants[wantID]
	if !exists {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{
			"error": fmt.Sprintf("Want execution with ID %s not found", wantID),
		})
		return
	}

	// Check if builder exists
	if execution.Builder == nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "Want execution has no active builder to resume",
		})
		return
	}

	// Resume the execution
	if err := execution.Builder.Resume(); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error": fmt.Sprintf("Failed to resume want execution: %v", err),
		})
		return
	}

	// Return success response
	response := map[string]interface{}{
		"message":   "Want execution resumed successfully",
		"wantId":    wantID,
		"suspended": false,
		"timestamp": time.Now().Format(time.RFC3339),
	}
	json.NewEncoder(w).Encode(response)
}

// Start starts the HTTP server
func (s *Server) Start() error {
	s.setupRoutes()
	addr := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)

	fmt.Printf("🚀 MyWant server starting on %s\n", addr)
	fmt.Printf("📋 Available endpoints:\n")
	fmt.Printf("  GET  /health                        - Health check\n")
	fmt.Printf("  POST /api/v1/wants                 - Create want (YAML config)\n")
	fmt.Printf("  GET  /api/v1/wants                 - List wants\n")
	fmt.Printf("  GET  /api/v1/wants/{id}            - Get want\n")
	fmt.Printf("  PUT  /api/v1/wants/{id}            - Update want\n")
	fmt.Printf("  DELETE /api/v1/wants/{id}          - Delete want\n")
	fmt.Printf("  GET  /api/v1/wants/{id}/status     - Get execution status\n")
	fmt.Printf("  GET  /api/v1/wants/{id}/results    - Get execution results\n")
	fmt.Printf("  POST /api/v1/wants/{id}/suspend    - Suspend want execution\n")
	fmt.Printf("  POST /api/v1/wants/{id}/resume     - Resume want execution\n")
	fmt.Printf("  POST /api/v1/agents                - Create agent\n")
	fmt.Printf("  GET  /api/v1/agents                - List agents\n")
	fmt.Printf("  GET  /api/v1/agents/{name}         - Get agent\n")
	fmt.Printf("  DELETE /api/v1/agents/{name}       - Delete agent\n")
	fmt.Printf("  POST /api/v1/capabilities          - Create capability\n")
	fmt.Printf("  GET  /api/v1/capabilities          - List capabilities\n")
	fmt.Printf("  GET  /api/v1/capabilities/{name}   - Get capability\n")
	fmt.Printf("  DELETE /api/v1/capabilities/{name} - Delete capability\n")
	fmt.Printf("  GET  /api/v1/capabilities/{name}/agents - Find agents by capability\n")
	fmt.Printf("  GET  /api/v1/errors              - List error history\n")
	fmt.Printf("  GET  /api/v1/errors/{id}         - Get error details\n")
	fmt.Printf("  PUT  /api/v1/errors/{id}         - Update error (mark resolved, add notes)\n")
	fmt.Printf("  DELETE /api/v1/errors/{id}       - Delete error entry\n")
	fmt.Printf("\n")

	return http.ListenAndServe(addr, s.router)
}


// validateWantTypes validates that all want types are known before execution
func (s *Server) validateWantTypes(config mywant.Config) error {
	// Create a temporary builder to check available types
	builder := mywant.NewChainBuilder(mywant.Config{})
	builder.SetAgentRegistry(s.agentRegistry)

	// Register all want types (same as in executeWantAsync)
	types.RegisterQNetWantTypes(builder)
	types.RegisterFibonacciWantTypes(builder)
	types.RegisterPrimeWantTypes(builder)
	types.RegisterTravelWantTypes(builder)
	mywant.RegisterMonitorWantTypes(builder)

	// Check each want type by trying to create a minimal want
	for _, want := range config.Wants {
		wantType := want.Metadata.Type

		// Create a minimal test want to validate the type
		testWant := &mywant.Want{
			Metadata: mywant.Metadata{
				Name: want.Metadata.Name,
				Type: wantType,
			},
			Spec: mywant.WantSpec{
				Params: make(map[string]interface{}),
			},
		}

		// Try to create the want function to check if type is valid
		_, err := builder.TestCreateWantFunction(testWant)
		if err != nil {
			return fmt.Errorf("invalid want type '%s' in want '%s': %v", wantType, want.Metadata.Name, err)
		}
	}

	return nil
}

// ======= ERROR HISTORY HANDLERS =======

// logError adds an error to the error history
func (s *Server) logError(r *http.Request, status int, message, errorType, details string, requestData interface{}) {
	errorID := generateErrorID()

	entry := ErrorHistoryEntry{
		ID:          errorID,
		Timestamp:   time.Now().Format(time.RFC3339),
		Message:     message,
		Status:      status,
		Type:        errorType,
		Details:     details,
		Endpoint:    r.URL.Path,
		Method:      r.Method,
		RequestData: requestData,
		UserAgent:   r.Header.Get("User-Agent"),
		Resolved:    false,
	}

	s.errorHistory = append(s.errorHistory, entry)

	// Keep only the last 1000 errors to prevent memory issues
	if len(s.errorHistory) > 1000 {
		s.errorHistory = s.errorHistory[len(s.errorHistory)-1000:]
	}
}

// httpErrorWithLogging handles HTTP errors and logs them to error history
func (s *Server) httpErrorWithLogging(w http.ResponseWriter, r *http.Request, status int, message, errorType string, requestData interface{}) {
	// Log the error to history
	s.logError(r, status, message, errorType, "", requestData)

	// Send HTTP error response
	http.Error(w, message, status)
}

// listErrorHistory handles GET /api/v1/errors - lists all error history entries
func (s *Server) listErrorHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Sort errors by timestamp (newest first)
	sortedErrors := make([]ErrorHistoryEntry, len(s.errorHistory))
	copy(sortedErrors, s.errorHistory)
	sort.Slice(sortedErrors, func(i, j int) bool {
		return sortedErrors[i].Timestamp > sortedErrors[j].Timestamp
	})

	response := map[string]interface{}{
		"errors": sortedErrors,
		"total":  len(sortedErrors),
	}

	json.NewEncoder(w).Encode(response)
}

// getErrorHistoryEntry handles GET /api/v1/errors/{id} - gets a specific error entry
func (s *Server) getErrorHistoryEntry(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	errorID := vars["id"]

	for _, entry := range s.errorHistory {
		if entry.ID == errorID {
			json.NewEncoder(w).Encode(entry)
			return
		}
	}

	http.Error(w, "Error entry not found", http.StatusNotFound)
}

// updateErrorHistoryEntry handles PUT /api/v1/errors/{id} - updates an error entry (mark as resolved, add notes)
func (s *Server) updateErrorHistoryEntry(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	errorID := vars["id"]

	var updateRequest struct {
		Resolved bool   `json:"resolved,omitempty"`
		Notes    string `json:"notes,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&updateRequest); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	for i, entry := range s.errorHistory {
		if entry.ID == errorID {
			if updateRequest.Resolved {
				s.errorHistory[i].Resolved = true
			}
			if updateRequest.Notes != "" {
				s.errorHistory[i].Notes = updateRequest.Notes
			}
			json.NewEncoder(w).Encode(s.errorHistory[i])
			return
		}
	}

	http.Error(w, "Error entry not found", http.StatusNotFound)
}

// deleteErrorHistoryEntry handles DELETE /api/v1/errors/{id} - deletes an error entry
func (s *Server) deleteErrorHistoryEntry(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	errorID := vars["id"]

	for i, entry := range s.errorHistory {
		if entry.ID == errorID {
			// Remove the entry from the slice
			s.errorHistory = append(s.errorHistory[:i], s.errorHistory[i+1:]...)
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}

	http.Error(w, "Error entry not found", http.StatusNotFound)
}

// generateErrorID generates a unique ID for error entries
func generateErrorID() string {
	bytes := make([]byte, 6)
	rand.Read(bytes)
	return fmt.Sprintf("error-%s-%d", hex.EncodeToString(bytes), time.Now().Unix()%10000)
}

// generateWantID generates a unique ID for want execution
func generateWantID() string {
	bytes := make([]byte, 6)
	rand.Read(bytes)
	return fmt.Sprintf("want-%s-%d", hex.EncodeToString(bytes), time.Now().Unix()%10000)
}

func main() {
	// Parse command line arguments
	port := 8080
	host := "localhost"

	if len(os.Args) > 1 {
		if p, err := strconv.Atoi(os.Args[1]); err == nil {
			port = p
		}
	}

	if len(os.Args) > 2 {
		host = os.Args[2]
	}

	// Create server config
	config := ServerConfig{
		Port: port,
		Host: host,
	}

	// Create and start server
	server := NewServer(config)
	if err := server.Start(); err != nil {
		log.Fatal("Server failed to start:", err)
	}
}
