package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
		fmt.Printf("[SERVER] Warning: Failed to load capabilities: %v\n", err)
	}

	if err := agentRegistry.LoadAgents("agents/"); err != nil {
		fmt.Printf("[SERVER] Warning: Failed to load agents: %v\n", err)
	}

	// Create recipe registry
	recipeRegistry := mywant.NewCustomTargetTypeRegistry()

	// Load recipes from recipes/ directory as custom types
	if err := mywant.ScanAndRegisterCustomTypes("recipes", recipeRegistry); err != nil {
		fmt.Printf("[SERVER] Warning: Failed to load recipes as custom types: %v\n", err)
	}

	// Also load the recipe files themselves into the recipe registry
	if err := loadRecipeFilesIntoRegistry("recipes", recipeRegistry); err != nil {
		fmt.Printf("[SERVER] Warning: Failed to load recipe files: %v\n", err)
	}

	// Create global builder for server mode with empty config
	// This builder will have its reconcile loop started when server starts
	globalBuilder := mywant.NewChainBuilderWithPaths("", "engine/memory/memory-server.yaml")
	globalBuilder.SetConfigInternal(mywant.Config{Wants: []*mywant.Want{}})
	globalBuilder.SetAgentRegistry(agentRegistry)

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

	fmt.Printf("[SERVER] 📋 Loaded configuration with %d wants\n", len(config.Wants))
	for _, want := range config.Wants {
		fmt.Printf("[SERVER]   - %s (%s)\n", want.Metadata.Name, want.Metadata.Type)
		if len(want.Spec.Requires) > 0 {
			fmt.Printf("[SERVER]     Requires: %v\n", want.Spec.Requires)
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
		fmt.Printf("[SERVER] Warning: Failed to load capabilities: %v\n", err)
	}

	if err := agentRegistry.LoadAgents("agents/"); err != nil {
		fmt.Printf("[SERVER] Warning: Failed to load agents: %v\n", err)
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
	fmt.Println("[SERVER] 🚀 Executing configuration...")
	builder.Execute()

	fmt.Println("[SERVER] ✅ Configuration execution completed!")
	return config, builder, nil
}

// registerDynamicAgents registers the same dynamic agents as demo_travel_agent_full.go
func (s *Server) registerDynamicAgents(agentRegistry *mywant.AgentRegistry) {
	// Same agent registration as demo_travel_agent_full.go:52-98

	// AgentPremium for hotel
	agentPremium := types.NewAgentPremium(
		"agent_premium",
		[]string{"hotel_reservation"},
		[]string{"xxx"},
		"platinum",
	)
	agentPremium.Action = func(ctx context.Context, want *mywant.Want) error {
		fmt.Printf("[SERVER][AGENT_PREMIUM_ACTION] Hotel agent called, delegating to AgentPremium.Exec()\n")
		return agentPremium.Exec(ctx, want)
	}
	agentRegistry.RegisterAgent(agentPremium)
	fmt.Printf("[SERVER] 🔧 Dynamically registered AgentPremium: %s\n", agentPremium.GetName())

	// Restaurant Agent
	agentRestaurant := types.NewAgentPremium(
		"agent_restaurant_premium",
		[]string{"restaurant_reservation"},
		[]string{"xxx"},
		"premium",
	)
	agentRestaurant.Action = func(ctx context.Context, want *mywant.Want) error {
		fmt.Printf("[SERVER][AGENT_RESTAURANT_ACTION] Restaurant agent called, processing reservation\n")
		return agentRestaurant.Exec(ctx, want)
	}
	agentRegistry.RegisterAgent(agentRestaurant)
	fmt.Printf("[SERVER] 🔧 Dynamically registered Restaurant Agent: %s\n", agentRestaurant.GetName())

	// Buffet Agent
	agentBuffet := types.NewAgentPremium(
		"agent_buffet_premium",
		[]string{"buffet_reservation"},
		[]string{"xxx"},
		"premium",
	)
	agentBuffet.Action = func(ctx context.Context, want *mywant.Want) error {
		fmt.Printf("[SERVER][AGENT_BUFFET_ACTION] Buffet agent called, processing reservation\n")
		return agentBuffet.Exec(ctx, want)
	}
	agentRegistry.RegisterAgent(agentBuffet)
	fmt.Printf("[SERVER] 🔧 Dynamically registered Buffet Agent: %s\n", agentBuffet.GetName())
}

// createWant handles POST /api/v1/wants - creates a new want object
func (s *Server) createWant(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Parse want object directly
	var newWant *mywant.Want

	if r.Header.Get("Content-Type") == "application/yaml" || r.Header.Get("Content-Type") == "text/yaml" {
		// Handle YAML want object directly
		wantYAML := make([]byte, r.ContentLength)
		r.Body.Read(wantYAML)

		if err := yaml.Unmarshal(wantYAML, &newWant); err != nil {
			http.Error(w, fmt.Sprintf("Invalid YAML want: %v", err), http.StatusBadRequest)
			return
		}
	} else {
		// Handle JSON want object directly
		if err := json.NewDecoder(r.Body).Decode(&newWant); err != nil {
			http.Error(w, fmt.Sprintf("Invalid JSON want: %v", err), http.StatusBadRequest)
			return
		}
	}

	if newWant == nil {
		http.Error(w, "Want object is required", http.StatusBadRequest)
		return
	}

	// Create config with single want
	config := mywant.Config{Wants: []*mywant.Want{newWant}}

	// Validate want type before proceeding
	if err := s.validateWantTypes(config); err != nil {
		errorMsg := fmt.Sprintf("Invalid want type: %v", err)
		s.logError(r, http.StatusBadRequest, errorMsg, "validation", err.Error(), "")
		http.Error(w, errorMsg, http.StatusBadRequest)
		return
	}

	// Generate unique ID for this want execution
	wantID := generateWantID()

	// Assign ID to the want if not already set
	if newWant.Metadata.ID == "" {
		newWant.Metadata.ID = generateWantID()
	}

	// Create want execution with global builder (server mode)
	execution := &WantExecution{
		ID:      wantID,
		Config:  config,
		Status:  "created",
		Builder: s.globalBuilder, // Use shared global builder
	}

	// Store the execution
	s.wants[wantID] = execution

	// Add want to global builder - reconcile loop will pick it up automatically
	if err := s.globalBuilder.AddDynamicWants([]*mywant.Want{newWant}); err != nil {
		// Remove from wants map since it wasn't added to builder
		delete(s.wants, wantID)
		errorMsg := fmt.Sprintf("Failed to add want: %v", err)
		s.logError(r, http.StatusConflict, errorMsg, "duplicate_name", err.Error(), "")
		http.Error(w, errorMsg, http.StatusConflict)
		return
	}

	fmt.Printf("[SERVER] Added want %s to global builder, reconcile loop will process it\n", wantID)

	// Return created want
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(execution)
}

// listWants handles GET /api/v1/wants - lists all wants in memory dump format
func (s *Server) listWants(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Collect all wants from all executions in memory dump format
	// Use map to deduplicate wants by ID (same want may exist across multiple executions)
	wantsByID := make(map[string]*mywant.Want)

	fmt.Printf("[LIST_WANTS] Processing %d executions\n", len(s.wants))
	for execID, execution := range s.wants {
		// Get current want states from the builder (builder always exists)
		currentStates := execution.Builder.GetAllWantStates()
		fmt.Printf("[LIST_WANTS] Execution %s has %d wants\n", execID, len(currentStates))
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
			fmt.Printf("[LIST_WANTS] Adding want %s (ID: %s) from execution %s\n", want.Metadata.Name, want.Metadata.ID, execID)
			wantsByID[want.Metadata.ID] = wantCopy
		}
	}
	fmt.Printf("[LIST_WANTS] After deduplication: %d unique wants\n", len(wantsByID))

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

	// Search for the want across all executions
	for executionID, execution := range s.wants {

		var wantNameToDelete string
		var foundInBuilder bool

		// Search in builder states if available
		if execution.Builder != nil {
			currentStates := execution.Builder.GetAllWantStates()
			for wantName, want := range currentStates {
				if want.Metadata.ID == wantID {
					wantNameToDelete = wantName
					foundInBuilder = true
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
				break
			}
		}

		// If want was found, delete it
		if wantNameToDelete != "" {
			fmt.Printf("[API] Before deletion: %d wants in config\n", len(execution.Config.Wants))

			// Remove from config if it exists there
			if configIndex >= 0 {
				execution.Config.Wants = append(execution.Config.Wants[:configIndex], execution.Config.Wants[configIndex+1:]...)
				fmt.Printf("[API] Removed from config, now %d wants in config\n", len(execution.Config.Wants))
			} else {
				fmt.Printf("[API] Want not in config (likely a dynamically created child want)\n")
			}

			// If using global builder (server mode), delete from runtime
			if foundInBuilder && execution.Builder != nil {
				// Delete the want directly from runtime by ID
				if err := execution.Builder.DeleteWantByID(wantID); err != nil {
					fmt.Printf("[API] Warning: Failed to delete want from runtime: %v\n", err)
				}

				// Also update config if it was removed
				if configIndex >= 0 {
					execution.Builder.SetConfigInternal(execution.Config)
				}

				fmt.Printf("[API] Want %s (%s) removed from runtime\n", wantNameToDelete, wantID)
			}

			// If no wants left, remove the entire execution
			if len(execution.Config.Wants) == 0 {
				delete(s.wants, executionID)
			}

			w.WriteHeader(http.StatusNoContent)
			return
		}
	}

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
					"status": string(want.Status),
				}
				json.NewEncoder(w).Encode(status)
				return
			}
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
					"data": want.State,
				}
				json.NewEncoder(w).Encode(results)
				return
			}
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

	// Register want types on global builder before starting reconcile loop
	types.RegisterQNetWantTypes(s.globalBuilder)
	types.RegisterFibonacciWantTypes(s.globalBuilder)
	types.RegisterPrimeWantTypes(s.globalBuilder)
	types.RegisterTravelWantTypes(s.globalBuilder)
	types.RegisterApprovalWantTypes(s.globalBuilder)
	mywant.RegisterMonitorWantTypes(s.globalBuilder)

	// Start global builder's reconcile loop for server mode (runs indefinitely)
	fmt.Println("[SERVER] Starting global reconcile loop for server mode...")
	go s.globalBuilder.ExecuteWithMode(true)

	addr := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)

	fmt.Printf("🚀 MyWant server starting on %s\n", addr)
	fmt.Printf("📋 Available endpoints:\n")
	fmt.Printf("  GET  /health                        - Health check\n")
	fmt.Printf("  POST /api/v1/configs               - Create config (YAML config with recipe reference)\n")
	fmt.Printf("  POST /api/v1/wants                 - Create want (JSON/YAML want object)\n")
	fmt.Printf("  GET  /api/v1/wants                 - List wants\n")
	fmt.Printf("  GET  /api/v1/wants/{id}            - Get want\n")
	fmt.Printf("  PUT  /api/v1/wants/{id}            - Update want (JSON/YAML want object)\n")
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
		fmt.Printf("[SERVER] Recipe directory '%s' does not exist, skipping recipe loading\n", recipeDir)
		return nil
	}

	// Create a recipe loader
	loader := mywant.NewGenericRecipeLoader(recipeDir)

	// List all recipe files
	recipes, err := loader.ListRecipes()
	if err != nil {
		return fmt.Errorf("failed to list recipes: %v", err)
	}

	fmt.Printf("[SERVER] Loading %d recipe files into registry...\n", len(recipes))

	// Load each recipe file
	loadedCount := 0
	for _, relativePath := range recipes {
		fullPath := fmt.Sprintf("%s/%s", recipeDir, relativePath)

		// Read and parse the recipe file directly
		data, err := os.ReadFile(fullPath)
		if err != nil {
			fmt.Printf("[SERVER] Warning: Failed to read recipe %s: %v\n", relativePath, err)
			continue
		}

		var recipe mywant.GenericRecipe
		if err := yaml.Unmarshal(data, &recipe); err != nil {
			fmt.Printf("[SERVER] Warning: Failed to parse recipe %s: %v\n", relativePath, err)
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
			fmt.Printf("[SERVER] Warning: Failed to register recipe %s: %v\n", recipeID, err)
			continue
		}

		fmt.Printf("[SERVER] ✅ Loaded recipe: %s\n", recipeID)
		loadedCount++
	}

	fmt.Printf("[SERVER] Successfully loaded %d/%d recipe files\n", loadedCount, len(recipes))
	return nil
}

// loadRecipesFromDirectory loads all recipe files from a directory into the registry

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
