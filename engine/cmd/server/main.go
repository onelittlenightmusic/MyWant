package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"gopkg.in/yaml.v3"
	types "mywant/engine/cmd/types"
	mywant "mywant/engine/src"
)

// ServerConfig holds server configuration
type ServerConfig struct {
	Port int    `json:"port"`
	Host string `json:"host"`
}

// ErrorHistoryEntry represents an API error with detailed context
type ErrorHistoryEntry struct {
	ID          string      `json:"id"`
	Timestamp   string      `json:"timestamp"`
	Message     string      `json:"message"`
	Status      int         `json:"status"`
	Code        string      `json:"code,omitempty"`
	Type        string      `json:"type,omitempty"`
	Details     string      `json:"details,omitempty"`
	Endpoint    string      `json:"endpoint"`
	Method      string      `json:"method"`
	RequestData interface{} `json:"request_data,omitempty"`
	UserAgent   string      `json:"user_agent,omitempty"`
	Resolved    bool        `json:"resolved"`
	Notes       string      `json:"notes,omitempty"`
}

// Server represents the MyWant server
type Server struct {
	config         ServerConfig
	wants          map[string]*WantExecution        // Store active want executions
	globalBuilder  *mywant.ChainBuilder             // Global builder with running reconcile loop for server mode
	agentRegistry  *mywant.AgentRegistry            // Agent and capability registry
	recipeRegistry *mywant.CustomTargetTypeRegistry // Recipe registry
	errorHistory   []ErrorHistoryEntry              // Store error history
	router         *mux.Router
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
		log.Printf("[SERVER] Warning: Failed to load capabilities: %v\n", err)
	}

	if err := agentRegistry.LoadAgents("agents/"); err != nil {
		log.Printf("[SERVER] Warning: Failed to load agents: %v\n", err)
	}

	// Create recipe registry
	recipeRegistry := mywant.NewCustomTargetTypeRegistry()

	// Load recipes from recipes/ directory as custom types
	if err := mywant.ScanAndRegisterCustomTypes("recipes", recipeRegistry); err != nil {
		log.Printf("[SERVER] Warning: Failed to load recipes as custom types: %v\n", err)
	}

	// Also load the recipe files themselves into the recipe registry
	if err := loadRecipeFilesIntoRegistry("recipes", recipeRegistry); err != nil {
		log.Printf("[SERVER] Warning: Failed to load recipe files: %v\n", err)
	}

	// Create global builder for server mode with empty config
	// Note: Registration order no longer matters - OwnerAware wrapping happens automatically at creation time
	globalBuilder := mywant.NewChainBuilderWithPaths("", "engine/memory/memory-0000-latest.yaml")
	globalBuilder.SetConfigInternal(mywant.Config{Wants: []*mywant.Want{}})
	globalBuilder.SetAgentRegistry(agentRegistry)

	// Create temporary server instance to call registerDynamicAgents
	tempServer := &Server{}

	// Register dynamic agent implementations on global registry
	// This provides the actual Action/Monitor functions for YAML-loaded agents
	tempServer.registerDynamicAgents(agentRegistry)

	return &Server{
		config:         config,
		wants:          make(map[string]*WantExecution),
		globalBuilder:  globalBuilder,
		agentRegistry:  agentRegistry,
		recipeRegistry: recipeRegistry,
		errorHistory:   make([]ErrorHistoryEntry, 0),
		router:         mux.NewRouter(),
	}
}

// corsMiddleware adds CORS headers to allow cross-origin requests
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		// Handle preflight OPTIONS request
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// handleOptions handles OPTIONS requests for CORS preflight
func (s *Server) handleOptions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	w.WriteHeader(http.StatusOK)
}

// setupRoutes configures the HTTP routes
func (s *Server) setupRoutes() {
	// Add CORS middleware to the main router
	s.router.Use(corsMiddleware)

	api := s.router.PathPrefix("/api/v1").Subrouter()

	// Wants CRUD endpoints
	wants := api.PathPrefix("/wants").Subrouter()
	wants.HandleFunc("", s.createWant).Methods("POST")
	wants.HandleFunc("", s.listWants).Methods("GET")
	wants.HandleFunc("", s.handleOptions).Methods("OPTIONS")
	wants.HandleFunc("/{id}", s.getWant).Methods("GET")
	wants.HandleFunc("/{id}", s.updateWant).Methods("PUT")
	wants.HandleFunc("/{id}", s.deleteWant).Methods("DELETE")
	wants.HandleFunc("/{id}", s.handleOptions).Methods("OPTIONS")
	wants.HandleFunc("/{id}/status", s.getWantStatus).Methods("GET")
	wants.HandleFunc("/{id}/results", s.getWantResults).Methods("GET")
	wants.HandleFunc("/{id}/suspend", s.suspendWant).Methods("POST")
	wants.HandleFunc("/{id}/resume", s.resumeWant).Methods("POST")
	wants.HandleFunc("/{id}/stop", s.stopWant).Methods("POST")
	wants.HandleFunc("/{id}/start", s.startWant).Methods("POST")

	// Config CRUD endpoints - for loading recipe-based configurations
	configs := api.PathPrefix("/configs").Subrouter()
	configs.HandleFunc("", s.createConfig).Methods("POST")
	configs.HandleFunc("", s.handleOptions).Methods("OPTIONS")

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

	// Recipe CRUD endpoints
	recipes := api.PathPrefix("/recipes").Subrouter()
	recipes.HandleFunc("", s.createRecipe).Methods("POST")
	recipes.HandleFunc("", s.listRecipes).Methods("GET")
	recipes.HandleFunc("/{id}", s.getRecipe).Methods("GET")
	recipes.HandleFunc("/{id}", s.updateRecipe).Methods("PUT")
	recipes.HandleFunc("/{id}", s.deleteRecipe).Methods("DELETE")

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

// createConfig handles POST /api/v1/configs - creates a configuration from recipe-based config files
// Uses the same logic as offline demo programs for DRY principle
func (s *Server) createConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Read raw config data from request body and save to temporary file
	configData := make([]byte, r.ContentLength)
	r.Body.Read(configData)

	// Generate unique ID for this configuration execution
	configID := generateWantID()

	// Create temporary config file
	tempConfigPath := fmt.Sprintf("/tmp/config-%s.yaml", configID)
	if err := os.WriteFile(tempConfigPath, configData, 0644); err != nil {
		errorMsg := fmt.Sprintf("Failed to create temporary config file: %v", err)
		s.logError(r, http.StatusInternalServerError, errorMsg, "file_creation", err.Error(), "")
		http.Error(w, errorMsg, http.StatusInternalServerError)
		return
	}
	defer os.Remove(tempConfigPath) // Clean up temp file

	// Execute using the same logic as demo_travel_agent_full.go for DRY principle
	config, builder, err := s.executeConfigLikeDemo(tempConfigPath, configID)
	if err != nil {
		errorMsg := fmt.Sprintf("Failed to execute config: %v", err)
		s.logError(r, http.StatusBadRequest, errorMsg, "execution", err.Error(), tempConfigPath)
		http.Error(w, errorMsg, http.StatusBadRequest)
		return
	}

	// Create execution tracking
	execution := &WantExecution{
		ID:      configID,
		Config:  config,
		Status:  "completed",
		Builder: builder,
	}

	// Store the execution
	s.wants[configID] = execution

	// Return created configuration
	w.WriteHeader(http.StatusCreated)
	response := map[string]interface{}{
		"id":      configID,
		"status":  "completed",
		"wants":   len(config.Wants),
		"message": "Configuration executed using demo program logic (DRY)",
	}
	json.NewEncoder(w).Encode(response)
}

// executeConfigLikeDemo executes config using the same logic as demo programs
// This is the DRY implementation that reuses offline mode logic
func (s *Server) executeConfigLikeDemo(configPath string, configID string) (mywant.Config, *mywant.ChainBuilder, error) {
	// Step 1: Load configuration using automatic recipe loading (same as demo_travel_agent_full.go:23)
	config, err := mywant.LoadConfigFromYAML(configPath)
	if err != nil {
		return mywant.Config{}, nil, fmt.Errorf("error loading %s: %v", configPath, err)
	}

	log.Printf("[SERVER] 📋 Loaded configuration with %d wants\n", len(config.Wants))
	for _, want := range config.Wants {
		log.Printf("[SERVER]   - %s (%s)\n", want.Metadata.Name, want.Metadata.Type)
		if len(want.Spec.Requires) > 0 {
			log.Printf("[SERVER]     Requires: %v\n", want.Spec.Requires)
		}
	}

	// Step 2: Create chain builder (same as demo_travel_agent_full.go:38)
	memoryPath := fmt.Sprintf("engine/memory/memory-%s.yaml", configID)
	builder := mywant.NewChainBuilderWithPaths("", memoryPath)
	builder.SetConfigInternal(config)

	// Step 3: Create and configure agent registry (same as demo_travel_agent_full.go:40-50)
	agentRegistry := mywant.NewAgentRegistry()

	// Load capabilities and agents
	if err := agentRegistry.LoadCapabilities("capabilities/"); err != nil {
		log.Printf("[SERVER] Warning: Failed to load capabilities: %v\n", err)
	}

	if err := agentRegistry.LoadAgents("agents/"); err != nil {
		log.Printf("[SERVER] Warning: Failed to load agents: %v\n", err)
	}

	// Register dynamic agents (same as demo_travel_agent_full.go:52-98)
	s.registerDynamicAgents(agentRegistry)

	// Set agent registry on the builder (same as demo_travel_agent_full.go:100)
	builder.SetAgentRegistry(agentRegistry)

	// Step 4: Register want types (same as demo_travel_agent_full.go:103)
	types.RegisterTravelWantTypes(builder)
	types.RegisterQNetWantTypes(builder)
	types.RegisterFibonacciWantTypes(builder)
	types.RegisterPrimeWantTypes(builder)
	types.RegisterApprovalWantTypes(builder)
	mywant.RegisterMonitorWantTypes(builder)

	// Step 5: Execute (same as demo_travel_agent_full.go:106)
	log.Println("[SERVER] 🚀 Executing configuration...")
	builder.Execute()

	log.Println("[SERVER] ✅ Configuration execution completed!")
	return config, builder, nil
}

// registerDynamicAgents registers implementations for special agents loaded from YAML
func (s *Server) registerDynamicAgents(agentRegistry *mywant.AgentRegistry) {
	log.Printf("[SERVER] Setting up dynamic agent implementations...\n")

	// Override the generic implementations with specific ones for special agents
	setupFlightAPIAgents(agentRegistry)
	setupMonitorFlightAgents(agentRegistry)

	log.Printf("[SERVER] Dynamic agent implementations registered\n")
}

// setupFlightAPIAgents sets up the Flight API agent implementations
func setupFlightAPIAgents(agentRegistry *mywant.AgentRegistry) {
	// Get the agent_flight_api from registry if it exists
	if agent, exists := agentRegistry.GetAgent("agent_flight_api"); exists {
		if doAgent, ok := agent.(*mywant.DoAgent); ok {
			// Set up the Flight API agent with the actual implementation
			flightAgent := types.NewAgentFlightAPI(
				"agent_flight_api",
				[]string{"flight_api_agency"},
				[]string{},
				"http://localhost:8081",
			)
			doAgent.Action = flightAgent.Exec
			log.Printf("[SERVER] ✅ Set up agent_flight_api with real implementation\n")
		}
	}
}

// setupMonitorFlightAgents sets up the Monitor Flight agent implementations
func setupMonitorFlightAgents(agentRegistry *mywant.AgentRegistry) {
	// Get the monitor_flight_api from registry if it exists
	if agent, exists := agentRegistry.GetAgent("monitor_flight_api"); exists {
		if monitorAgent, ok := agent.(*mywant.MonitorAgent); ok {
			// Set up the Monitor Flight agent with the actual implementation
			flightMonitor := types.NewMonitorFlightAPI(
				"monitor_flight_api",
				[]string{"flight_api_agency"},
				[]string{},
				"http://localhost:8081",
			)
			monitorAgent.Monitor = flightMonitor.Exec
			log.Printf("[SERVER] ✅ Set up monitor_flight_api with real implementation\n")
		}
	}
}

// createWant handles POST /api/v1/wants - creates a new want object
// Supports two formats:
// 1. Single Want object (JSON/YAML)
// 2. Config object with wants array (for recipe-based configs)
func (s *Server) createWant(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Read request body using a buffer to handle both known and unknown content lengths
	var buf bytes.Buffer
	io.Copy(&buf, r.Body)
	data := buf.Bytes()

	// First try to parse as a Config (recipe-based with multiple wants)
	var config mywant.Config
	var configErr error

	if r.Header.Get("Content-Type") == "application/yaml" || r.Header.Get("Content-Type") == "text/yaml" {
		configErr = yaml.Unmarshal(data, &config)
	} else {
		configErr = json.Unmarshal(data, &config)
	}

	// If config parsing failed or has no wants, try parsing as single Want
	if configErr != nil || len(config.Wants) == 0 {
		var newWant *mywant.Want

		if r.Header.Get("Content-Type") == "application/yaml" || r.Header.Get("Content-Type") == "text/yaml" {
			configErr = yaml.Unmarshal(data, &newWant)
		} else {
			configErr = json.Unmarshal(data, &newWant)
		}

		if configErr != nil || newWant == nil {
			http.Error(w, fmt.Sprintf("Invalid request: must be either a Want object or Config with wants array. Error: %v", configErr), http.StatusBadRequest)
			return
		}

		// Create config with single want
		config = mywant.Config{Wants: []*mywant.Want{newWant}}
	}

	// Validate want type before proceeding
	if err := s.validateWantTypes(config); err != nil {
		errorMsg := fmt.Sprintf("Invalid want type: %v", err)
		s.logError(r, http.StatusBadRequest, errorMsg, "validation", err.Error(), "")
		http.Error(w, errorMsg, http.StatusBadRequest)
		return
	}

	// Assign IDs to all wants if not already set
	for _, want := range config.Wants {
		if want.Metadata.ID == "" {
			want.Metadata.ID = generateWantID()
		}
	}

	// Generate unique ID for this execution (group of wants)
	executionID := generateWantID()

	// Create want execution with global builder (server mode)
	execution := &WantExecution{
		ID:      executionID,
		Config:  config,
		Status:  "created",
		Builder: s.globalBuilder, // Use shared global builder
	}

	// Store the execution
	s.wants[executionID] = execution

	// Add all wants to global builder - reconcile loop will pick them up automatically
	if err := s.globalBuilder.AddDynamicWants(config.Wants); err != nil {
		// Remove from wants map since they weren't added to builder
		delete(s.wants, executionID)
		errorMsg := fmt.Sprintf("Failed to add wants: %v", err)
		s.logError(r, http.StatusConflict, errorMsg, "duplicate_name", err.Error(), "")
		http.Error(w, errorMsg, http.StatusConflict)
		return
	}

	// Trigger reconciliation to immediately process the newly added wants
	if err := s.globalBuilder.TriggerReconcile(); err != nil {
		log.Printf("[SERVER] Failed to trigger reconciliation: %v\n", err)
	} else {
		log.Printf("[SERVER] Sent reconciliation trigger\n")
	}

	log.Printf("[SERVER] Added %d wants to global builder (execution %s), reconcile loop will process them\n", len(config.Wants), executionID)
	for _, want := range config.Wants {
		log.Printf("[SERVER]   - %s (%s, ID: %s)\n", want.Metadata.Name, want.Metadata.Type, want.Metadata.ID)
	}

	// Return created execution with first want ID as reference
	w.WriteHeader(http.StatusCreated)
	response := map[string]interface{}{
		"id":       executionID,
		"status":   execution.Status,
		"wants":    len(config.Wants),
		"want_ids": make([]string, len(config.Wants)),
		"message":  "Wants created and added to execution queue",
	}
	// Build list of want IDs
	wantIDs := make([]string, len(config.Wants))
	for i, want := range config.Wants {
		wantIDs[i] = want.Metadata.ID
	}
	response["want_ids"] = wantIDs

	json.NewEncoder(w).Encode(response)
}

// listWants handles GET /api/v1/wants - lists all wants in memory dump format
func (s *Server) listWants(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Collect all wants from all executions in memory dump format
	// Use map to deduplicate wants by ID (same want may exist across multiple executions)
	wantsByID := make(map[string]*mywant.Want)

	log.Printf("[LIST_WANTS] Processing %d executions\n", len(s.wants))
	for execID, execution := range s.wants {
		// Get current want states from the builder (builder always exists)
		currentStates := execution.Builder.GetAllWantStates()
		log.Printf("[LIST_WANTS] Execution %s has %d wants\n", execID, len(currentStates))
		for _, want := range currentStates {
			// Create a snapshot copy of the want to avoid concurrent map access
			wantCopy := &mywant.Want{
				Metadata: want.Metadata,
				Spec:     want.Spec,
				Status:   want.GetStatus(),
				History:  want.History,
				State:    make(map[string]interface{}),
			}

			// Get current runtime state and copy to the snapshot
			currentState := want.GetAllState()
			for k, v := range currentState {
				wantCopy.State[k] = v
			}

			// Store by ID to deduplicate (keep latest version)
			log.Printf("[LIST_WANTS] Adding want %s (ID: %s) from execution %s\n", want.Metadata.Name, want.Metadata.ID, execID)
			wantsByID[want.Metadata.ID] = wantCopy
		}
	}

	// If no wants from executions, also check global builder (for wants loaded from memory file)
	if len(wantsByID) == 0 && s.globalBuilder != nil {
		log.Printf("[LIST_WANTS] No wants in executions, checking global builder...\n")
		currentStates := s.globalBuilder.GetAllWantStates()
		log.Printf("[LIST_WANTS] Global builder has %d wants\n", len(currentStates))
		for _, want := range currentStates {
			// Create a snapshot copy of the want to avoid concurrent map access
			wantCopy := &mywant.Want{
				Metadata: want.Metadata,
				Spec:     want.Spec,
				Status:   want.GetStatus(),
				History:  want.History,
				State:    make(map[string]interface{}),
			}

			// Get current runtime state and copy to the snapshot
			currentState := want.GetAllState()
			for k, v := range currentState {
				wantCopy.State[k] = v
			}

			// Store by ID to deduplicate (keep latest version)
			log.Printf("[LIST_WANTS] Adding want %s (ID: %s) from global builder\n", want.Metadata.Name, want.Metadata.ID)
			wantsByID[want.Metadata.ID] = wantCopy
		}
	}

	log.Printf("[LIST_WANTS] After deduplication: %d unique wants\n", len(wantsByID))

	// Convert map to slice
	allWants := make([]*mywant.Want, 0, len(wantsByID))
	for _, want := range wantsByID {
		allWants = append(allWants, want)
	}

	// Create memory dump format response
	response := map[string]interface{}{
		"timestamp":    time.Now().Format(time.RFC3339),
		"execution_id": fmt.Sprintf("api-dump-%d", time.Now().Unix()),
		"wants":        allWants,
	}

	json.NewEncoder(w).Encode(response)
}

// getWant handles GET /api/v1/wants/{id} - gets a specific individual want by its ID
func (s *Server) getWant(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	wantID := vars["id"]

	// Search for the want by metadata.id across all executions using universal search
	for _, execution := range s.wants {
		if execution.Builder != nil {
			if want, _, found := execution.Builder.FindWantByID(wantID); found {
				// Create a snapshot copy of the want to avoid concurrent map access
				wantCopy := &mywant.Want{
					Metadata: want.Metadata,
					Spec:     want.Spec,
					Status:   want.GetStatus(),
					History:  want.History,
					State:    make(map[string]interface{}),
				}

				// Get current runtime state and copy to the snapshot
				currentState := want.GetAllState()
				for k, v := range currentState {
					wantCopy.State[k] = v
				}

				// Return the snapshot copy
				json.NewEncoder(w).Encode(wantCopy)
				return
			}
		}
	}

	// If not found in executions, also search in global builder (for wants loaded from memory file)
	if s.globalBuilder != nil {
		if want, _, found := s.globalBuilder.FindWantByID(wantID); found {
			// Create a snapshot copy of the want to avoid concurrent map access
			wantCopy := &mywant.Want{
				Metadata: want.Metadata,
				Spec:     want.Spec,
				Status:   want.GetStatus(),
				History:  want.History,
				State:    make(map[string]interface{}),
			}

			// Get current runtime state and copy to the snapshot
			currentState := want.GetAllState()
			for k, v := range currentState {
				wantCopy.State[k] = v
			}

			// Return the snapshot copy
			json.NewEncoder(w).Encode(wantCopy)
			return
		}
	}

	http.Error(w, "Want not found", http.StatusNotFound)
}

// updateWant handles PUT /api/v1/wants/{id} - updates a want object
func (s *Server) updateWant(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	wantID := vars["id"]

	// Search for the want by metadata.id across all executions using universal search
	var targetExecution *WantExecution
	var targetWantIndex int = -1
	var executionID string
	var foundWant *mywant.Want

	for execID, execution := range s.wants {
		if execution.Builder != nil {
			if want, _, found := execution.Builder.FindWantByID(wantID); found {
				targetExecution = execution
				foundWant = want
				executionID = execID
				// Find the index in the original config for updating
				for j, configWant := range execution.Config.Wants {
					if configWant.Metadata.ID == wantID {
						targetWantIndex = j
						break
					}
				}
				break
			}
		}
	}

	if targetExecution == nil || foundWant == nil {
		http.Error(w, "Want not found", http.StatusNotFound)
		return
	}

	// Only allow updates if the execution is not currently running
	if targetExecution.Status == "running" {
		http.Error(w, "Cannot update running want", http.StatusConflict)
		return
	}

	// Parse updated want object directly
	var updatedWant *mywant.Want

	if r.Header.Get("Content-Type") == "application/yaml" || r.Header.Get("Content-Type") == "text/yaml" {
		// Handle YAML want object directly
		wantYAML := make([]byte, r.ContentLength)
		r.Body.Read(wantYAML)

		if err := yaml.Unmarshal(wantYAML, &updatedWant); err != nil {
			http.Error(w, fmt.Sprintf("Invalid YAML want: %v", err), http.StatusBadRequest)
			return
		}
	} else {
		// Handle JSON want object directly
		if err := json.NewDecoder(r.Body).Decode(&updatedWant); err != nil {
			http.Error(w, fmt.Sprintf("Invalid JSON want: %v", err), http.StatusBadRequest)
			return
		}
	}

	if updatedWant == nil {
		http.Error(w, "Want object is required", http.StatusBadRequest)
		return
	}

	// Validate want type before proceeding
	tempConfig := mywant.Config{Wants: []*mywant.Want{updatedWant}}
	if err := s.validateWantTypes(tempConfig); err != nil {
		errorMsg := fmt.Sprintf("Invalid want type: %v", err)
		s.logError(r, http.StatusBadRequest, errorMsg, "validation", err.Error(), "")
		http.Error(w, errorMsg, http.StatusBadRequest)
		return
	}

	// Preserve the original want ID
	updatedWant.Metadata.ID = foundWant.Metadata.ID

	// Update config
	if targetWantIndex >= 0 && targetWantIndex < len(targetExecution.Config.Wants) {
		targetExecution.Config.Wants[targetWantIndex] = updatedWant
	} else {
		targetExecution.Config.Wants = append(targetExecution.Config.Wants, updatedWant)
	}

	// Use ChainBuilder's UpdateWant API - let reconcile loop handle the rest
	if targetExecution.Builder != nil {
		targetExecution.Builder.UpdateWant(updatedWant)
	}

	targetExecution.Status = "updated"

	// Return the updated want, not the entire execution
	response := &WantExecution{
		ID:      executionID,
		Config:  mywant.Config{Wants: []*mywant.Want{updatedWant}},
		Status:  "updated",
		Results: nil,
	}

	json.NewEncoder(w).Encode(response)
}

// deleteWant handles DELETE /api/v1/wants/{id} - deletes an individual want by its ID
func (s *Server) deleteWant(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	wantID := vars["id"]

	log.Printf("[API_DELETE] Starting deletion for want ID: %s\n", wantID)

	// Search for the want across all executions
	for executionID, execution := range s.wants {
		log.Printf("[API_DELETE] Checking execution %s\n", executionID)

		var wantNameToDelete string
		var foundInBuilder bool

		// Search in builder states if available
		if execution.Builder != nil {
			currentStates := execution.Builder.GetAllWantStates()
			log.Printf("[API_DELETE] Builder has %d wants in runtime\n", len(currentStates))
			for wantName, want := range currentStates {
				if want.Metadata.ID == wantID {
					wantNameToDelete = wantName
					foundInBuilder = true
					log.Printf("[API_DELETE] Found want in builder: %s\n", wantName)
					break
				}
			}
		}

		// Search in config wants
		var configIndex = -1
		for i, want := range execution.Config.Wants {
			if want.Metadata.ID == wantID {
				if wantNameToDelete == "" {
					wantNameToDelete = want.Metadata.Name
				}
				configIndex = i
				log.Printf("[API_DELETE] Found want in config at index %d\n", configIndex)
				break
			}
		}

		// If want was found, delete it
		if wantNameToDelete != "" {
			log.Printf("[API] Before deletion: %d wants in config\n", len(execution.Config.Wants))
			log.Printf("[API_DELETE] foundInBuilder=%v, configIndex=%d\n", foundInBuilder, configIndex)

			// Remove from config if it exists there
			if configIndex >= 0 {
				execution.Config.Wants = append(execution.Config.Wants[:configIndex], execution.Config.Wants[configIndex+1:]...)
				log.Printf("[API] Removed from config, now %d wants in config\n", len(execution.Config.Wants))
			} else {
				log.Printf("[API] Want not in config (likely a dynamically created child want)\n")
			}

			// If using global builder (server mode), delete from runtime
			if foundInBuilder && execution.Builder != nil {
				log.Printf("[API_DELETE] Calling DeleteWantByID(%s)\n", wantID)
				// Delete the want directly from runtime by ID
				if err := execution.Builder.DeleteWantByID(wantID); err != nil {
					log.Printf("[API] Warning: Failed to delete want from runtime: %v\n", err)
				} else {
					log.Printf("[API_DELETE] DeleteWantByID succeeded\n")
				}

				// Also update config if it was removed
				if configIndex >= 0 {
					execution.Builder.SetConfigInternal(execution.Config)
				}

				log.Printf("[API] Want %s (%s) removed from runtime\n", wantNameToDelete, wantID)
			} else {
				log.Printf("[API_DELETE] Skipping DeleteWantByID (foundInBuilder=%v)\n", foundInBuilder)
			}

			// If no wants left, remove the entire execution
			if len(execution.Config.Wants) == 0 {
				delete(s.wants, executionID)
			}

			w.WriteHeader(http.StatusNoContent)
			return
		}
	}

	// If not found in executions, also search in global builder (for wants loaded from memory file)
	if s.globalBuilder != nil {
		log.Printf("[API_DELETE] Searching in global builder...\n")
		if want, _, found := s.globalBuilder.FindWantByID(wantID); found {
			log.Printf("[API_DELETE] Found want in global builder: %s\n", want.Metadata.Name)

			// Delete the want from the global builder
			if err := s.globalBuilder.DeleteWantByID(wantID); err != nil {
				log.Printf("[API] Warning: Failed to delete want from global builder: %v\n", err)
				errorMsg := fmt.Sprintf("Failed to delete want: %v", err)
				s.logError(r, http.StatusInternalServerError, errorMsg, "deletion", err.Error(), wantID)
				http.Error(w, errorMsg, http.StatusInternalServerError)
				return
			}

			log.Printf("[API_DELETE] Successfully deleted want from global builder\n")
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}

	log.Printf("[API_DELETE] Want %s not found in any execution or global builder\n", wantID)

	errorMsg := fmt.Sprintf("Want not found: %s", wantID)
	s.logError(r, http.StatusNotFound, errorMsg, "deletion", "want not found", wantID)
	http.Error(w, "Want not found", http.StatusNotFound)
}

// getWantStatus handles GET /api/v1/wants/{id}/status - gets want execution status
func (s *Server) getWantStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	wantID := vars["id"]

	// Search for the want by metadata.id across all executions using universal search
	for _, execution := range s.wants {
		if execution.Builder != nil {
			if want, _, found := execution.Builder.FindWantByID(wantID); found {
				// Found the want, return its status
				status := map[string]interface{}{
					"id":     want.Metadata.ID,
					"status": string(want.GetStatus()),
				}
				json.NewEncoder(w).Encode(status)
				return
			}
		}
	}

	// If not found in executions, also search in global builder (for wants loaded from memory file)
	if s.globalBuilder != nil {
		if want, _, found := s.globalBuilder.FindWantByID(wantID); found {
			// Found the want, return its status
			status := map[string]interface{}{
				"id":     want.Metadata.ID,
				"status": string(want.GetStatus()),
			}
			json.NewEncoder(w).Encode(status)
			return
		}
	}

	http.Error(w, "Want not found", http.StatusNotFound)
}

// getWantResults handles GET /api/v1/wants/{id}/results - gets want execution results
func (s *Server) getWantResults(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	wantID := vars["id"]

	// Search for the want by metadata.id across all executions using universal search
	for _, execution := range s.wants {
		if execution.Builder != nil {
			if want, _, found := execution.Builder.FindWantByID(wantID); found {
				// Found the want, return its results (stored in State)
				if want.State == nil {
					want.State = make(map[string]interface{})
				}
				results := map[string]interface{}{
					"data": want.GetAllState(),
				}
				json.NewEncoder(w).Encode(results)
				return
			}
		}
	}

	// If not found in executions, also search in global builder (for wants loaded from memory file)
	if s.globalBuilder != nil {
		if want, _, found := s.globalBuilder.FindWantByID(wantID); found {
			// Found the want, return its results (stored in State)
			results := map[string]interface{}{
				"data": want.GetAllState(),
			}
			json.NewEncoder(w).Encode(results)
			return
		}
	}

	http.Error(w, "Want not found", http.StatusNotFound)
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

	// Get all agents from registry
	agents := s.agentRegistry.GetAllAgents()

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
		"agents": agentResponses,
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

	// Check if agent exists and delete it
	if !s.agentRegistry.UnregisterAgent(agentName) {
		http.Error(w, "Agent not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
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

	// Get all capabilities from registry
	capabilities := s.agentRegistry.GetAllCapabilities()

	response := map[string]interface{}{
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

	// Check if capability exists and delete it
	if !s.agentRegistry.UnregisterCapability(capabilityName) {
		http.Error(w, "Capability not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
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

// stopWant handles POST /api/v1/wants/{id}/stop - stops want execution
func (s *Server) stopWant(w http.ResponseWriter, r *http.Request) {
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
			"error": "Want execution has no active builder to stop",
		})
		return
	}

	// Stop the execution
	if err := execution.Builder.Stop(); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error": fmt.Sprintf("Failed to stop want execution: %v", err),
		})
		return
	}

	// Update execution status
	execution.Status = "stopped"

	// Return success response
	response := map[string]interface{}{
		"message":   "Want execution stopped successfully",
		"wantId":    wantID,
		"status":    "stopped",
		"timestamp": time.Now().Format(time.RFC3339),
	}
	json.NewEncoder(w).Encode(response)
}

// startWant handles POST /api/v1/wants/{id}/start - starts/restarts want execution
func (s *Server) startWant(w http.ResponseWriter, r *http.Request) {
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
			"error": "Want execution has no active builder to start",
		})
		return
	}

	// Start the execution
	if err := execution.Builder.Start(); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error": fmt.Sprintf("Failed to start want execution: %v", err),
		})
		return
	}

	// Update execution status
	execution.Status = "running"

	// Return success response
	response := map[string]interface{}{
		"message":   "Want execution started successfully",
		"wantId":    wantID,
		"status":    "running",
		"timestamp": time.Now().Format(time.RFC3339),
	}
	json.NewEncoder(w).Encode(response)
}

// Start starts the HTTP server
func (s *Server) Start() error {
	s.setupRoutes()

	// Register all want types on global builder before starting reconcile loop
	// Note: Registration order no longer matters - OwnerAware wrapping happens automatically at creation time
	types.RegisterQNetWantTypes(s.globalBuilder)
	types.RegisterFibonacciWantTypes(s.globalBuilder)
	types.RegisterPrimeWantTypes(s.globalBuilder)
	types.RegisterTravelWantTypes(s.globalBuilder)
	types.RegisterApprovalWantTypes(s.globalBuilder)
	mywant.RegisterMonitorWantTypes(s.globalBuilder)
	mywant.RegisterOwnerWantTypes(s.globalBuilder)

	// Start global builder's reconcile loop for server mode (runs indefinitely)
	log.Println("[SERVER] Starting global reconcile loop for server mode...")
	go s.globalBuilder.ExecuteWithMode(true)

	addr := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)

	log.Printf("🚀 MyWant server starting on %s\n", addr)
	log.Printf("📋 Available endpoints:\n")
	log.Printf("  GET  /health                        - Health check\n")
	log.Printf("  POST /api/v1/configs               - Create config (YAML config with recipe reference)\n")
	log.Printf("  POST /api/v1/wants                 - Create want (JSON/YAML want object)\n")
	log.Printf("  GET  /api/v1/wants                 - List wants\n")
	log.Printf("  GET  /api/v1/wants/{id}            - Get want\n")
	log.Printf("  PUT  /api/v1/wants/{id}            - Update want (JSON/YAML want object)\n")
	log.Printf("  DELETE /api/v1/wants/{id}          - Delete want\n")
	log.Printf("  GET  /api/v1/wants/{id}/status     - Get execution status\n")
	log.Printf("  GET  /api/v1/wants/{id}/results    - Get execution results\n")
	log.Printf("  POST /api/v1/wants/{id}/suspend    - Suspend want execution\n")
	log.Printf("  POST /api/v1/wants/{id}/resume     - Resume want execution\n")
	log.Printf("  POST /api/v1/agents                - Create agent\n")
	log.Printf("  GET  /api/v1/agents                - List agents\n")
	log.Printf("  GET  /api/v1/agents/{name}         - Get agent\n")
	log.Printf("  DELETE /api/v1/agents/{name}       - Delete agent\n")
	log.Printf("  POST /api/v1/capabilities          - Create capability\n")
	log.Printf("  GET  /api/v1/capabilities          - List capabilities\n")
	log.Printf("  GET  /api/v1/capabilities/{name}   - Get capability\n")
	log.Printf("  DELETE /api/v1/capabilities/{name} - Delete capability\n")
	log.Printf("  GET  /api/v1/capabilities/{name}/agents - Find agents by capability\n")
	log.Printf("  GET  /api/v1/errors              - List error history\n")
	log.Printf("  GET  /api/v1/errors/{id}         - Get error details\n")
	log.Printf("  PUT  /api/v1/errors/{id}         - Update error (mark resolved, add notes)\n")
	log.Printf("  DELETE /api/v1/errors/{id}       - Delete error entry\n")
	log.Printf("\n")

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
	types.RegisterApprovalWantTypes(builder)
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

// generateWantID generates a unique ID for want execution using UUID
func generateWantID() string {
	// Generate UUID v4 (random)
	uuid := make([]byte, 16)
	rand.Read(uuid)

	// Set version (4) and variant bits
	uuid[6] = (uuid[6] & 0x0f) | 0x40 // Version 4
	uuid[8] = (uuid[8] & 0x3f) | 0x80 // Variant

	// Format as UUID string: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
	return fmt.Sprintf("want-%x-%x-%x-%x-%x",
		uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16])
}

// Recipe API handlers

// createRecipe handles POST /api/v1/recipes - creates a new recipe
func (s *Server) createRecipe(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var recipe mywant.GenericRecipe
	if err := json.NewDecoder(r.Body).Decode(&recipe); err != nil {
		s.logError(r, http.StatusBadRequest, "Invalid recipe format", "recipe_creation", err.Error(), "")
		http.Error(w, "Invalid recipe format", http.StatusBadRequest)
		return
	}

	// Use recipe name as ID if no custom ID provided
	recipeID := recipe.Recipe.Metadata.Name
	if recipeID == "" {
		s.logError(r, http.StatusBadRequest, "Recipe name is required", "recipe_creation", "missing name", "")
		http.Error(w, "Recipe name is required", http.StatusBadRequest)
		return
	}

	if err := s.recipeRegistry.CreateRecipe(recipeID, &recipe); err != nil {
		s.logError(r, http.StatusConflict, err.Error(), "recipe_creation", "duplicate recipe", recipeID)
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{
		"id":      recipeID,
		"message": "Recipe created successfully",
	})
}

// listRecipes handles GET /api/v1/recipes - lists all recipes
func (s *Server) listRecipes(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	recipes := s.recipeRegistry.ListRecipes()
	json.NewEncoder(w).Encode(recipes)
}

// getRecipe handles GET /api/v1/recipes/{id} - gets a specific recipe
func (s *Server) getRecipe(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	recipeID := vars["id"]

	recipe, exists := s.recipeRegistry.GetRecipe(recipeID)
	if !exists {
		s.logError(r, http.StatusNotFound, "Recipe not found", "recipe_retrieval", "recipe not found", recipeID)
		http.Error(w, "Recipe not found", http.StatusNotFound)
		return
	}

	json.NewEncoder(w).Encode(recipe)
}

// updateRecipe handles PUT /api/v1/recipes/{id} - updates an existing recipe
func (s *Server) updateRecipe(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	recipeID := vars["id"]

	var recipe mywant.GenericRecipe
	if err := json.NewDecoder(r.Body).Decode(&recipe); err != nil {
		s.logError(r, http.StatusBadRequest, "Invalid recipe format", "recipe_update", err.Error(), recipeID)
		http.Error(w, "Invalid recipe format", http.StatusBadRequest)
		return
	}

	if err := s.recipeRegistry.UpdateRecipe(recipeID, &recipe); err != nil {
		s.logError(r, http.StatusNotFound, err.Error(), "recipe_update", "recipe not found", recipeID)
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	json.NewEncoder(w).Encode(map[string]string{
		"id":      recipeID,
		"message": "Recipe updated successfully",
	})
}

// deleteRecipe handles DELETE /api/v1/recipes/{id} - deletes a recipe
func (s *Server) deleteRecipe(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	recipeID := vars["id"]

	if err := s.recipeRegistry.DeleteRecipe(recipeID); err != nil {
		s.logError(r, http.StatusNotFound, err.Error(), "recipe_deletion", "recipe not found", recipeID)
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// loadRecipeFilesIntoRegistry loads recipe YAML files into the recipe registry for the API
func loadRecipeFilesIntoRegistry(recipeDir string, registry *mywant.CustomTargetTypeRegistry) error {
	// Check if recipes directory exists
	if _, err := os.Stat(recipeDir); os.IsNotExist(err) {
		log.Printf("[SERVER] Recipe directory '%s' does not exist, skipping recipe loading\n", recipeDir)
		return nil
	}

	// Create a recipe loader
	loader := mywant.NewGenericRecipeLoader(recipeDir)

	// List all recipe files
	recipes, err := loader.ListRecipes()
	if err != nil {
		return fmt.Errorf("failed to list recipes: %v", err)
	}

	log.Printf("[SERVER] Loading %d recipe files into registry...\n", len(recipes))

	// Load each recipe file
	loadedCount := 0
	for _, relativePath := range recipes {
		fullPath := fmt.Sprintf("%s/%s", recipeDir, relativePath)

		// Read and parse the recipe file directly
		data, err := os.ReadFile(fullPath)
		if err != nil {
			log.Printf("[SERVER] Warning: Failed to read recipe %s: %v\n", relativePath, err)
			continue
		}

		var recipe mywant.GenericRecipe
		if err := yaml.Unmarshal(data, &recipe); err != nil {
			log.Printf("[SERVER] Warning: Failed to parse recipe %s: %v\n", relativePath, err)
			continue
		}

		// Use recipe name as ID, fall back to filename without extension
		recipeID := recipe.Recipe.Metadata.Name
		if recipeID == "" {
			recipeID = strings.TrimSuffix(relativePath, ".yaml")
			recipeID = strings.TrimSuffix(recipeID, ".yml")
		}

		// Create the recipe in the registry
		if err := registry.CreateRecipe(recipeID, &recipe); err != nil {
			log.Printf("[SERVER] Warning: Failed to register recipe %s: %v\n", recipeID, err)
			continue
		}

		log.Printf("[SERVER] ✅ Loaded recipe: %s\n", recipeID)
		loadedCount++
	}

	log.Printf("[SERVER] Successfully loaded %d/%d recipe files\n", loadedCount, len(recipes))
	return nil
}

// loadRecipesFromDirectory loads all recipe files from a directory into the registry

func main() {
	// Create logs directory if it doesn't exist
	if err := os.MkdirAll("./logs", 0755); err != nil {
		log.Fatalf("Failed to create logs directory: %v", err)
	}

	// Configure logging to a file
	logFile, err := os.OpenFile("./logs/mywant-backend.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}
	defer logFile.Close()
	log.SetOutput(logFile)
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

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
