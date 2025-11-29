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
	Port  int    `json:"port"`
	Host  string `json:"host"`
	Debug bool   `json:"debug"`
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
	config            ServerConfig
	wants             map[string]*WantExecution        // Store active want executions
	globalBuilder     *mywant.ChainBuilder             // Global builder with running reconcile loop for server mode
	agentRegistry     *mywant.AgentRegistry            // Agent and capability registry
	recipeRegistry    *mywant.CustomTargetTypeRegistry // Recipe registry
	wantTypeLoader    *mywant.WantTypeLoader           // Want type definitions loader
	errorHistory      []ErrorHistoryEntry              // Store error history
	router            *mux.Router
	globalLabels      map[string]map[string]bool       // Globally registered labels (key -> value -> true)
}

// WantExecution represents a running want execution
type WantExecution struct {
	ID      string                 `json:"id"`
	Config  mywant.Config          `json:"config"` // Changed from pointer
	Status  string                 `json:"status"` // "running", "completed", "failed"
	Results map[string]interface{} `json:"results,omitempty"`
	Builder *mywant.ChainBuilder   `json:"-"` // Don't serialize builder
}

// WantResponseWithGroupedAgents wraps a Want with grouped agent history
type WantResponseWithGroupedAgents struct {
	Metadata mywant.Metadata        `json:"metadata"`
	Spec     mywant.WantSpec        `json:"spec"`
	Status   mywant.WantStatus      `json:"status"`
	History  mywant.WantHistory     `json:"history"`
	State    map[string]interface{} `json:"state"`
}

// LLMRequest represents a request to the LLM inference API
type LLMRequest struct {
	Message string `json:"message"`
	Model   string `json:"model,omitempty"`
}

// LLMResponse represents a response from the LLM inference API
type LLMResponse struct {
	Response  string `json:"response"`
	Model     string `json:"model"`
	Timestamp string `json:"timestamp"`
}

// OllamaRequest represents the request format for Ollama API
type OllamaRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

// OllamaResponse represents the response format from Ollama API
type OllamaResponse struct {
	Model              string `json:"model"`
	CreatedAt          string `json:"created_at"`
	Response           string `json:"response"`
	Done               bool   `json:"done"`
	DoneReason         string `json:"done_reason,omitempty"`
	Context            []int  `json:"context,omitempty"`
	TotalDuration      int64  `json:"total_duration,omitempty"`
	LoadDuration       int64  `json:"load_duration,omitempty"`
	PromptEvalCount    int    `json:"prompt_eval_count,omitempty"`
	PromptEvalDuration int64  `json:"prompt_eval_duration,omitempty"`
	EvalCount          int    `json:"eval_count,omitempty"`
	EvalDuration       int64  `json:"eval_duration,omitempty"`
}

// buildWantResponse creates a response with grouped agent history nested in history
func buildWantResponse(want *mywant.Want, groupBy string) interface{} {
	response := &WantResponseWithGroupedAgents{
		Metadata: want.Metadata,
		Spec:     want.Spec,
		Status:   want.Status,
		History:  want.History,
		State:    want.State,
	}

	// Populate grouped agent history in the history field
	if groupBy == "name" {
		response.History.GroupedAgentHistory = want.GetAgentHistoryGroupedByName()
	} else if groupBy == "type" {
		response.History.GroupedAgentHistory = want.GetAgentHistoryGroupedByType()
	}

	return response
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

	// Load want type definitions
	wantTypeLoader := mywant.NewWantTypeLoader("want_types")
	if err := wantTypeLoader.LoadAllWantTypes(); err != nil {
		log.Printf("[SERVER] Warning: Failed to load want type definitions: %v\n", err)
	} else {
		stats := wantTypeLoader.GetStats()
		log.Printf("[SERVER] Loaded want type definitions: %v\n", stats)
	}

	// Create global builder for server mode with empty config
	// Note: Registration order no longer matters - OwnerAware wrapping happens automatically at creation time
	globalBuilder := mywant.NewChainBuilderWithPaths("", "engine/memory/memory-0000-latest.yaml")
	globalBuilder.SetConfigInternal(mywant.Config{Wants: []*mywant.Want{}})
	globalBuilder.SetAgentRegistry(agentRegistry)
	globalBuilder.SetCustomTargetRegistry(recipeRegistry) // Set custom types from recipes

	// Create temporary server instance to call registerDynamicAgents
	tempServer := &Server{}

	// Register dynamic agent implementations on global registry
	// This provides the actual Action/Monitor functions for YAML-loaded agents
	tempServer.registerDynamicAgents(agentRegistry)

	return &Server{
		config:            config,
		wants:             make(map[string]*WantExecution),
		globalBuilder:     globalBuilder,
		agentRegistry:     agentRegistry,
		recipeRegistry:    recipeRegistry,
		wantTypeLoader:    wantTypeLoader,
		errorHistory:      make([]ErrorHistoryEntry, 0),
		router:            mux.NewRouter(),
		globalLabels:      make(map[string]map[string]bool),
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
	wants.HandleFunc("/{id}/labels", s.addLabelToWant).Methods("POST")
	wants.HandleFunc("/{id}/labels", s.handleOptions).Methods("OPTIONS")

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

	// Want Type endpoints - for discovery and introspection
	wantTypes := api.PathPrefix("/want-types").Subrouter()
	wantTypes.HandleFunc("", s.listWantTypes).Methods("GET")
	wantTypes.HandleFunc("", s.handleOptions).Methods("OPTIONS")
	wantTypes.HandleFunc("/{name}", s.getWantType).Methods("GET")
	wantTypes.HandleFunc("/{name}", s.handleOptions).Methods("OPTIONS")
	wantTypes.HandleFunc("/{name}/examples", s.getWantTypeExamples).Methods("GET")
	wantTypes.HandleFunc("/{name}/examples", s.handleOptions).Methods("OPTIONS")

	// Labels endpoints - for autocomplete in want creation form
	labels := api.PathPrefix("/labels").Subrouter()
	labels.HandleFunc("", s.getLabels).Methods("GET")
	labels.HandleFunc("", s.addLabel).Methods("POST")
	labels.HandleFunc("", s.handleOptions).Methods("OPTIONS")

	// Error history endpoints
	errors := api.PathPrefix("/errors").Subrouter()
	errors.HandleFunc("", s.listErrorHistory).Methods("GET")
	errors.HandleFunc("/{id}", s.getErrorHistoryEntry).Methods("GET")
	errors.HandleFunc("/{id}", s.updateErrorHistoryEntry).Methods("PUT")
	errors.HandleFunc("/{id}", s.deleteErrorHistoryEntry).Methods("DELETE")

	// LLM inference endpoints
	llm := api.PathPrefix("/llm").Subrouter()
	llm.HandleFunc("/query", s.queryLLM).Methods("POST")
	llm.HandleFunc("/query", s.handleOptions).Methods("OPTIONS")

	// Health check endpoint
	s.router.HandleFunc("/health", s.healthCheck).Methods("GET")

	// Serve static files (for future web UI)
	s.router.PathPrefix("/").Handler(http.FileServer(http.Dir("./web/"))).Methods("GET")
}

// queryLLM handles POST /api/v1/llm/query - sends a query to the Ollama LLM
func (s *Server) queryLLM(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Parse request
	var req LLMRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorMsg := "Invalid request format"
		s.logError(r, http.StatusBadRequest, errorMsg, "parse_error", err.Error(), "")
		http.Error(w, errorMsg, http.StatusBadRequest)
		return
	}

	// Use default model if not specified
	model := req.Model
	if model == "" {
		model = "gpt-oss:20b"
	}

	// Call Ollama LLM
	response, err := s.callOllamaLLM(model, req.Message)
	if err != nil {
		errorMsg := fmt.Sprintf("Failed to query LLM: %v", err)
		s.logError(r, http.StatusInternalServerError, errorMsg, "llm_error", err.Error(), "")
		http.Error(w, errorMsg, http.StatusInternalServerError)
		return
	}

	// Return success
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// callOllamaLLM calls the Ollama LLM API
func (s *Server) callOllamaLLM(model string, prompt string) (*LLMResponse, error) {
	// Get Ollama base URL from environment variable or use default
	ollamaURL := os.Getenv("GPT_BASE_URL")
	if ollamaURL == "" {
		ollamaURL = "localhost:11434"
	}

	// Ensure URL has proper protocol
	if !strings.HasPrefix(ollamaURL, "http://") && !strings.HasPrefix(ollamaURL, "https://") {
		ollamaURL = "http://" + ollamaURL
	}

	// Create Ollama request
	ollamaReq := OllamaRequest{
		Model:  model,
		Prompt: prompt,
		Stream: false,
	}

	// Marshal request
	reqBody, err := json.Marshal(ollamaReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Make HTTP request to Ollama
	url := fmt.Sprintf("%s/api/generate", ollamaURL)
	resp, err := http.Post(url, "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Ollama at %s: %w", url, err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Ollama returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var ollamaResp OllamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return nil, fmt.Errorf("failed to decode Ollama response: %w", err)
	}

	// Return formatted response
	return &LLMResponse{
		Response:  ollamaResp.Response,
		Model:     ollamaResp.Model,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}, nil
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

	for _, want := range config.Wants {
		if len(want.Spec.Requires) > 0 {
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
	builder.Execute()

	return config, builder, nil
}

// registerDynamicAgents registers implementations for special agents loaded from YAML
func (s *Server) registerDynamicAgents(agentRegistry *mywant.AgentRegistry) {

	// Override the generic implementations with specific ones for special agents
	setupFlightAPIAgents(agentRegistry)
	setupMonitorFlightAgents(agentRegistry)

}

// setupFlightAPIAgents sets up the Flight API agent implementations
func setupFlightAPIAgents(agentRegistry *mywant.AgentRegistry) {
	// Get the agent_flight_api from registry if it exists
	if agent, exists := agentRegistry.GetAgent("agent_flight_api"); exists {
		if doAgent, ok := agent.(*mywant.DoAgent); ok {
			// Set up the Flight API agent with the actual implementation
			// Agent has flight_api_agency capability which gives: create_flight and cancel_flight
			flightAgent := types.NewAgentFlightAPI(
				"agent_flight_api",
				[]string{"flight_api_agency"},
				[]string{},
				"http://localhost:8081",
			)
			doAgent.Action = flightAgent.Exec
		}
	}
}

// setupMonitorFlightAgents sets up the Monitor Flight agent implementations
func setupMonitorFlightAgents(agentRegistry *mywant.AgentRegistry) {
	// Get the monitor_flight_api from registry if it exists
	if agent, exists := agentRegistry.GetAgent("monitor_flight_api"); exists {
		if monitorAgent, ok := agent.(*mywant.MonitorAgent); ok {
			// Set up the Monitor Flight agent with the actual implementation
			// Note: Monitor agents don't provide capabilities, they observe/monitor state
			flightMonitor := types.NewMonitorFlightAPI(
				"monitor_flight_api",
				[]string{},
				[]string{},
				"http://localhost:8081",
			)
			monitorAgent.Monitor = flightMonitor.Exec
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

	// Add all wants to global builder asynchronously with tracking
	wantIDs, err := s.globalBuilder.AddWantsAsyncWithTracking(config.Wants)
	if err != nil {
		// Remove from wants map since they weren't added to builder
		delete(s.wants, executionID)
		errorMsg := fmt.Sprintf("Failed to add wants: %v", err)
		s.logError(r, http.StatusConflict, errorMsg, "duplicate_name", err.Error(), "")
		http.Error(w, errorMsg, http.StatusConflict)
		return
	}

	// Wait for wants to be added to reconcile loop (poll with timeout)
	maxAttempts := 100
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if s.globalBuilder.AreWantsAdded(wantIDs) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	for _, want := range config.Wants {
		// API-level logging for want creation
		InfoLog("[API:CREATE] Want created: %s (%s, ID: %s)\n", want.Metadata.Name, want.Metadata.Type, want.Metadata.ID)
	}

	// Return created execution with first want ID as reference
	w.WriteHeader(http.StatusCreated)

	// Safety check for invalid want count
	wantCount := len(config.Wants)
	if wantCount < 0 || wantCount > 1000000 {
		errorMsg := fmt.Sprintf("Invalid want count after parsing: %d", wantCount)
		s.logError(r, http.StatusInternalServerError, errorMsg, "parsing_error", "Invalid want count", "")
		http.Error(w, errorMsg, http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"id":       executionID,
		"status":   execution.Status,
		"wants":    wantCount,
		"want_ids": wantIDs, // wantIDs already extracted from AddWantsAsyncWithTracking
		"message":  "Wants created and added to execution queue",
	}

	json.NewEncoder(w).Encode(response)
}

// listWants handles GET /api/v1/wants - lists all wants in memory dump format
func (s *Server) listWants(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Collect all wants from all executions in memory dump format
	// Use map to deduplicate wants by ID (same want may exist across multiple executions)
	wantsByID := make(map[string]*mywant.Want)

	for _, execution := range s.wants {
		// Get current want states from the builder (builder always exists)
		currentStates := execution.Builder.GetAllWantStates()
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
			wantsByID[want.Metadata.ID] = wantCopy
		}
	}

	// If no wants from executions, also check global builder (for wants loaded from memory file)
	if len(wantsByID) == 0 && s.globalBuilder != nil {
		currentStates := s.globalBuilder.GetAllWantStates()
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
			wantsByID[want.Metadata.ID] = wantCopy
		}
	}


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

	// Get query parameters for grouping/filtering agent history
	groupBy := r.URL.Query().Get("groupBy") // "name" or "type"

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

				// If groupBy is specified, return response with grouped agent history
				if groupBy != "" {
					response := buildWantResponse(wantCopy, groupBy)
					json.NewEncoder(w).Encode(response)
					return
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

			// If groupBy is specified, return response with grouped agent history
			if groupBy != "" {
				response := buildWantResponse(wantCopy, groupBy)
				json.NewEncoder(w).Encode(response)
				return
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

	log.Printf("[API:UPDATE] Looking for want ID: %s across %d executions\n", wantID, len(s.wants))

	// Search for the want by metadata.id across all executions using universal search
	var targetExecution *WantExecution
	var targetWantIndex int = -1
	var foundWant *mywant.Want

	for i, execution := range s.wants {
		if execution.Builder != nil {
			if want, _, found := execution.Builder.FindWantByID(wantID); found {
				log.Printf("[API:UPDATE] Found want in execution %d\n", i)
				targetExecution = execution
				foundWant = want
				// Find the index in the original config for updating
				for j, configWant := range execution.Config.Wants {
					if configWant.Metadata.ID == wantID {
						targetWantIndex = j
						break
					}
				}
				break
			}
		} else {
			log.Printf("[API:UPDATE] Execution %d has nil builder\n", i)
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

	// Use ChainBuilder's UpdateWant API - automatically triggers reconciliation
	// UpdateWant handles reconciliation internally via reconcileTrigger channel
	if targetExecution.Builder != nil {
		targetExecution.Builder.UpdateWant(updatedWant)
	}

	targetExecution.Status = "updated"

	// Return the updated want directly (matching createWant response format)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updatedWant)
}

// deleteWant handles DELETE /api/v1/wants/{id} - deletes an individual want by its ID
func (s *Server) deleteWant(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	wantID := vars["id"]

	InfoLog("Starting deletion for want ID: %s\n", wantID)

	// Search for the want across all executions
	for executionID, execution := range s.wants {

		var wantNameToDelete string
		var wantTypeToDelete string
		var foundInBuilder bool

		// Search in builder states if available
		if execution.Builder != nil {
			currentStates := execution.Builder.GetAllWantStates()
			for wantName, want := range currentStates {
				if want.Metadata.ID == wantID {
					wantNameToDelete = wantName
					wantTypeToDelete = want.Metadata.Type
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
				if wantTypeToDelete == "" {
					wantTypeToDelete = want.Metadata.Type
				}
				configIndex = i
				break
			}
		}

		// If want was found, delete it
		if wantNameToDelete != "" {

			// Remove from config if it exists there
			if configIndex >= 0 {
				execution.Config.Wants = append(execution.Config.Wants[:configIndex], execution.Config.Wants[configIndex+1:]...)
			} else {
			}

			// If using global builder (server mode), delete from runtime asynchronously
			if foundInBuilder && execution.Builder != nil {

				// Delete the want asynchronously
				_, err := execution.Builder.DeleteWantsAsyncWithTracking([]string{wantID})
				if err != nil {
					ErrorLog("Failed to send deletion request: %v\n", err)
				} else {
					// Wait for want to be deleted (poll with timeout)
					maxAttempts := 100
					for attempt := 0; attempt < maxAttempts; attempt++ {
						if execution.Builder.AreWantsDeleted([]string{wantID}) {
							break
						}
						time.Sleep(10 * time.Millisecond)
					}
				}

				// Also update config if it was removed
				if configIndex >= 0 {
					execution.Builder.SetConfigInternal(execution.Config)
				}
			} else {
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
		if _, _, found := s.globalBuilder.FindWantByID(wantID); found {

			// Delete the want from the global builder asynchronously
			_, err := s.globalBuilder.DeleteWantsAsyncWithTracking([]string{wantID})
			if err != nil {
				ErrorLog("Failed to send deletion request: %v\n", err)
				errorMsg := fmt.Sprintf("Failed to delete want: %v", err)
				s.logError(r, http.StatusInternalServerError, errorMsg, "deletion", err.Error(), wantID)
				http.Error(w, errorMsg, http.StatusInternalServerError)
				return
			}

			// Wait for want to be deleted (poll with timeout)
			maxAttempts := 100
			for attempt := 0; attempt < maxAttempts; attempt++ {
				if s.globalBuilder.AreWantsDeleted([]string{wantID}) {
					InfoLog("Want %s deletion confirmed from global builder\n", wantID)
					break
				}
				time.Sleep(10 * time.Millisecond)
			}

			InfoLog("Successfully deleted want from global builder\n")
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}

	ErrorLog("Want %s not found in any execution or global builder\n", wantID)

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

	// Search for the want by metadata.id across all executions
	for _, execution := range s.wants {
		if execution.Builder != nil {
			if _, _, found := execution.Builder.FindWantByID(wantID); found {
				// Suspend the specific want execution
				if err := execution.Builder.SuspendWant(wantID); err != nil {
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
				return
			}
		}
	}

	// If not found in executions, also search in global builder (for wants loaded from memory file)
	if s.globalBuilder != nil {
		if _, _, found := s.globalBuilder.FindWantByID(wantID); found {
			// Suspend the specific want execution
			if err := s.globalBuilder.SuspendWant(wantID); err != nil {
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
			return
		}
	}

	// Want not found
	w.WriteHeader(http.StatusNotFound)
	json.NewEncoder(w).Encode(map[string]string{
		"error": fmt.Sprintf("Want with ID %s not found", wantID),
	})
}

// resumeWant handles POST /api/v1/wants/{id}/resume - resumes want execution
func (s *Server) resumeWant(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Extract want ID from URL path
	vars := mux.Vars(r)
	wantID := vars["id"]

	// Search for the want by metadata.id across all executions
	for _, execution := range s.wants {
		if execution.Builder != nil {
			if _, _, found := execution.Builder.FindWantByID(wantID); found {
				// Resume the specific want execution
				if err := execution.Builder.ResumeWant(wantID); err != nil {
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
				return
			}
		}
	}

	// If not found in executions, also search in global builder (for wants loaded from memory file)
	if s.globalBuilder != nil {
		if _, _, found := s.globalBuilder.FindWantByID(wantID); found {
			// Resume the specific want execution
			if err := s.globalBuilder.ResumeWant(wantID); err != nil {
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
			return
		}
	}

	// Want not found
	w.WriteHeader(http.StatusNotFound)
	json.NewEncoder(w).Encode(map[string]string{
		"error": fmt.Sprintf("Want with ID %s not found", wantID),
	})
}

// stopWant handles POST /api/v1/wants/{id}/stop - stops want execution
func (s *Server) stopWant(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Extract want ID from URL path
	vars := mux.Vars(r)
	wantID := vars["id"]

	// Search for the want by metadata.id across all executions
	for _, execution := range s.wants {
		if execution.Builder != nil {
			if _, _, found := execution.Builder.FindWantByID(wantID); found {
				// Stop the specific want execution
				if err := execution.Builder.StopWant(wantID); err != nil {
					w.WriteHeader(http.StatusInternalServerError)
					json.NewEncoder(w).Encode(map[string]string{
						"error": fmt.Sprintf("Failed to stop want execution: %v", err),
					})
					return
				}

				// Return success response
				response := map[string]interface{}{
					"message":   "Want execution stopped successfully",
					"wantId":    wantID,
					"status":    "stopped",
					"timestamp": time.Now().Format(time.RFC3339),
				}
				json.NewEncoder(w).Encode(response)
				return
			}
		}
	}

	// If not found in executions, also search in global builder (for wants loaded from memory file)
	if s.globalBuilder != nil {
		if _, _, found := s.globalBuilder.FindWantByID(wantID); found {
			// Stop the specific want execution
			if err := s.globalBuilder.StopWant(wantID); err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{
					"error": fmt.Sprintf("Failed to stop want execution: %v", err),
				})
				return
			}

			// Return success response
			response := map[string]interface{}{
				"message":   "Want execution stopped successfully",
				"wantId":    wantID,
				"status":    "stopped",
				"timestamp": time.Now().Format(time.RFC3339),
			}
			json.NewEncoder(w).Encode(response)
			return
		}
	}

	// Want not found
	w.WriteHeader(http.StatusNotFound)
	json.NewEncoder(w).Encode(map[string]string{
		"error": fmt.Sprintf("Want with ID %s not found", wantID),
	})
}

// startWant handles POST /api/v1/wants/{id}/start - starts/restarts want execution
func (s *Server) startWant(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Extract want ID from URL path
	vars := mux.Vars(r)
	wantID := vars["id"]

	// Search for the want by metadata.id across all executions
	for _, execution := range s.wants {
		if execution.Builder != nil {
			if _, _, found := execution.Builder.FindWantByID(wantID); found {
				// Restart the specific want execution
				if err := execution.Builder.RestartWant(wantID); err != nil {
					w.WriteHeader(http.StatusInternalServerError)
					json.NewEncoder(w).Encode(map[string]string{
						"error": fmt.Sprintf("Failed to restart want execution: %v", err),
					})
					return
				}

				// Return success response
				response := map[string]interface{}{
					"message":   "Want execution restarted successfully",
					"wantId":    wantID,
					"status":    "running",
					"timestamp": time.Now().Format(time.RFC3339),
				}
				json.NewEncoder(w).Encode(response)
				return
			}
		}
	}

	// If not found in executions, also search in global builder (for wants loaded from memory file)
	if s.globalBuilder != nil {
		if _, _, found := s.globalBuilder.FindWantByID(wantID); found {
			// Restart the specific want execution
			if err := s.globalBuilder.RestartWant(wantID); err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{
					"error": fmt.Sprintf("Failed to restart want execution: %v", err),
				})
				return
			}

			// Return success response
			response := map[string]interface{}{
				"message":   "Want execution restarted successfully",
				"wantId":    wantID,
				"status":    "running",
				"timestamp": time.Now().Format(time.RFC3339),
			}
			json.NewEncoder(w).Encode(response)
			return
		}
	}

	// Want not found
	w.WriteHeader(http.StatusNotFound)
	json.NewEncoder(w).Encode(map[string]string{
		"error": fmt.Sprintf("Want with ID %s not found", wantID),
	})
}

// addLabelToWant handles POST /api/v1/wants/{id}/labels - adds a label to a want
func (s *Server) addLabelToWant(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	wantID := vars["id"]

	// Parse label request body
	var labelReq struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}

	if err := json.NewDecoder(r.Body).Decode(&labelReq); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON request: %v", err), http.StatusBadRequest)
		return
	}

	// Validate label key and value
	if labelReq.Key == "" || labelReq.Value == "" {
		http.Error(w, "Label key and value are required", http.StatusBadRequest)
		return
	}

	// Search for the want by metadata.id across all executions
	var targetWant *mywant.Want
	var found bool

	for _, execution := range s.wants {
		if execution.Builder != nil {
			if want, _, foundInExecution := execution.Builder.FindWantByID(wantID); foundInExecution {
				targetWant = want
				found = true
				break
			}
		}
	}

	// If not found in executions, search in global builder
	if !found && s.globalBuilder != nil {
		if want, _, foundInGlobal := s.globalBuilder.FindWantByID(wantID); foundInGlobal {
			targetWant = want
			found = true
		}
	}

	if !found || targetWant == nil {
		http.Error(w, fmt.Sprintf("Want with ID %s not found", wantID), http.StatusNotFound)
		return
	}

	// Add the label to the want's metadata
	if targetWant.Metadata.Labels == nil {
		targetWant.Metadata.Labels = make(map[string]string)
	}
	targetWant.Metadata.Labels[labelReq.Key] = labelReq.Value

	// Return success response
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":   "Label added successfully",
		"wantId":    wantID,
		"key":       labelReq.Key,
		"value":     labelReq.Value,
		"timestamp": time.Now().Format(time.RFC3339),
	})
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
	log.Printf("  POST /api/v1/wants/{id}/labels     - Add label to want\n")
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
// Uses the global builder which has all registries already configured
// Allows either want.Type or want.Spec.Recipe to be specified
func (s *Server) validateWantTypes(config mywant.Config) error {
	// Check each want type by trying to create a minimal want
	// Using globalBuilder ensures we validate against the exact same registries
	// that will be used during actual execution
	for _, want := range config.Wants {
		wantType := want.Metadata.Type
		hasRecipe := want.Spec.Recipe != ""

		// Want must have either a type or a recipe
		if wantType == "" && !hasRecipe {
			return fmt.Errorf("want '%s' must have either a type or a recipe specified", want.Metadata.Name)
		}

		// If recipe is specified without a type, skip type validation
		if hasRecipe && wantType == "" {
			continue
		}

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
		// This uses the globalBuilder which has all registries synchronized
		_, err := s.globalBuilder.TestCreateWantFunction(testWant)
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

// listWantTypes handles GET /api/v1/want-types
func (s *Server) listWantTypes(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if s.wantTypeLoader == nil {
		http.Error(w, "Want type loader not initialized", http.StatusServiceUnavailable)
		return
	}

	// Get query parameters for filtering
	category := r.URL.Query().Get("category")
	pattern := r.URL.Query().Get("pattern")

	var defs []*mywant.WantTypeDefinition

	if category != "" {
		defs = s.wantTypeLoader.ListByCategory(category)
	} else if pattern != "" {
		defs = s.wantTypeLoader.ListByPattern(pattern)
	} else {
		defs = s.wantTypeLoader.GetAll()
	}

	// Build response with minimal info for listing
	type WantTypeListItem struct {
		Name     string `json:"name"`
		Title    string `json:"title"`
		Category string `json:"category"`
		Pattern  string `json:"pattern"`
		Version  string `json:"version"`
	}

	items := make([]WantTypeListItem, len(defs))
	for i, def := range defs {
		items[i] = WantTypeListItem{
			Name:     def.Metadata.Name,
			Title:    def.Metadata.Title,
			Category: def.Metadata.Category,
			Pattern:  def.Metadata.Pattern,
			Version:  def.Metadata.Version,
		}
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"wantTypes": items,
		"count":     len(items),
	})
}

// getLabels handles GET /api/v1/labels - returns all label keys and values used across wants
func (s *Server) getLabels(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Collect all unique label keys and their values from all wants
	// Also track which wants use each label
	labelKeys := make(map[string]bool)
	labelValues := make(map[string]map[string]bool)                    // key -> (value -> true)
	labelToWants := make(map[string]map[string]map[string]bool)        // key -> value -> (wantID -> true)

	// Start with globally registered labels (added via POST /api/v1/labels)
	if s.globalLabels != nil {
		for key, valueMap := range s.globalLabels {
			labelKeys[key] = true
			if labelValues[key] == nil {
				labelValues[key] = make(map[string]bool)
			}
			for value := range valueMap {
				labelValues[key][value] = true
			}
		}
	}

	// Collect from wants in executions
	for _, execution := range s.wants {
		if execution.Builder != nil {
			currentStates := execution.Builder.GetAllWantStates()
			for _, want := range currentStates {
				// Skip internal wants
				if want.Metadata.Name != "" && strings.HasPrefix(want.Metadata.Name, "__") {
					continue
				}
				for key, value := range want.Metadata.Labels {
					labelKeys[key] = true
					if labelValues[key] == nil {
						labelValues[key] = make(map[string]bool)
						labelToWants[key] = make(map[string]map[string]bool)
					}
					labelValues[key][value] = true

					// Track which wants use this label
					if labelToWants[key][value] == nil {
						labelToWants[key][value] = make(map[string]bool)
					}
					wantID := want.Metadata.ID
					labelToWants[key][value][wantID] = true
				}
			}
		}
	}

	// Also check global builder
	if s.globalBuilder != nil {
		currentStates := s.globalBuilder.GetAllWantStates()
		for _, want := range currentStates {
			// Skip internal wants
			if want.Metadata.Name != "" && strings.HasPrefix(want.Metadata.Name, "__") {
				continue
			}
			for key, value := range want.Metadata.Labels {
				labelKeys[key] = true
				if labelValues[key] == nil {
					labelValues[key] = make(map[string]bool)
					labelToWants[key] = make(map[string]map[string]bool)
				}
				labelValues[key][value] = true

				// Track which wants use this label
				if labelToWants[key][value] == nil {
					labelToWants[key][value] = make(map[string]bool)
				}
				wantID := want.Metadata.ID
				labelToWants[key][value][wantID] = true
			}
		}
	}

	// Collect wants that use labels via 'using' selectors (users)
	labelToUsers := make(map[string]map[string]map[string]bool) // key -> value -> (wantID -> true)

	// Check executions for wants with 'using' selectors
	for _, execution := range s.wants {
		if execution.Builder != nil {
			currentStates := execution.Builder.GetAllWantStates()
			for _, want := range currentStates {
				// Skip internal wants
				if want.Metadata.Name != "" && strings.HasPrefix(want.Metadata.Name, "__") {
					continue
				}
				// Check if this want uses any labels via 'using' selectors
				for _, usingSelector := range want.Spec.Using {
					for key, value := range usingSelector {
						if labelToUsers[key] == nil {
							labelToUsers[key] = make(map[string]map[string]bool)
						}
						if labelToUsers[key][value] == nil {
							labelToUsers[key][value] = make(map[string]bool)
						}
						wantID := want.Metadata.ID
						labelToUsers[key][value][wantID] = true
					}
				}
			}
		}
	}

	// Check global builder for wants with 'using' selectors
	if s.globalBuilder != nil {
		currentStates := s.globalBuilder.GetAllWantStates()
		for _, want := range currentStates {
			// Skip internal wants
			if want.Metadata.Name != "" && strings.HasPrefix(want.Metadata.Name, "__") {
				continue
			}
			// Check if this want uses any labels via 'using' selectors
			for _, usingSelector := range want.Spec.Using {
				for key, value := range usingSelector {
					if labelToUsers[key] == nil {
						labelToUsers[key] = make(map[string]map[string]bool)
					}
					if labelToUsers[key][value] == nil {
						labelToUsers[key][value] = make(map[string]bool)
					}
					wantID := want.Metadata.ID
					labelToUsers[key][value][wantID] = true
				}
			}
		}
	}

	// Convert keys to sorted slice
	keys := make([]string, 0, len(labelKeys))
	for key := range labelKeys {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	// Convert values and owners/users to response format
	type LabelValueInfo struct {
		Value  string   `json:"value"`
		Owners []string `json:"owners"` // Wants that have this label
		Users  []string `json:"users"`  // Wants that use this label via 'using'
	}

	values := make(map[string][]LabelValueInfo)
	for key, valueMap := range labelValues {
		valueSlice := make([]string, 0, len(valueMap))
		for value := range valueMap {
			valueSlice = append(valueSlice, value)
		}
		sort.Strings(valueSlice)

		// Ensure labelToWants[key] is initialized
		if labelToWants[key] == nil {
			labelToWants[key] = make(map[string]map[string]bool)
		}
		// Ensure labelToUsers[key] is initialized
		if labelToUsers[key] == nil {
			labelToUsers[key] = make(map[string]map[string]bool)
		}

		// Convert to response format with owners and users
		valueInfos := make([]LabelValueInfo, 0, len(valueSlice))
		for _, value := range valueSlice {
			// Ensure the value maps exist
			if labelToWants[key][value] == nil {
				labelToWants[key][value] = make(map[string]bool)
			}
			if labelToUsers[key][value] == nil {
				labelToUsers[key][value] = make(map[string]bool)
			}

			ownerIDs := make([]string, 0, len(labelToWants[key][value]))
			for wantID := range labelToWants[key][value] {
				ownerIDs = append(ownerIDs, wantID)
			}
			sort.Strings(ownerIDs)

			userIDs := make([]string, 0, len(labelToUsers[key][value]))
			for wantID := range labelToUsers[key][value] {
				userIDs = append(userIDs, wantID)
			}
			sort.Strings(userIDs)

			valueInfos = append(valueInfos, LabelValueInfo{
				Value:  value,
				Owners: ownerIDs,
				Users:  userIDs,
			})
		}
		values[key] = valueInfos
	}

	// Return response
	json.NewEncoder(w).Encode(map[string]interface{}{
		"labelKeys":   keys,
		"labelValues": values,
		"count":       len(keys),
	})
}

// addLabel handles POST /api/v1/labels - adds a label to the global label registry
// This allows registering labels even if they don't exist on any want yet
// Labels are stored by adding a minimal want to the global builder's state for persistence
func (s *Server) addLabel(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Parse request body
	var labelReq struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}

	if err := json.NewDecoder(r.Body).Decode(&labelReq); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	// Validate input
	labelReq.Key = strings.TrimSpace(labelReq.Key)
	labelReq.Value = strings.TrimSpace(labelReq.Value)

	if labelReq.Key == "" || labelReq.Value == "" {
		http.Error(w, "Label key and value must not be empty", http.StatusBadRequest)
		return
	}

	// Add the label to the global label registry (a separate tracking structure)
	// This ensures labels persist even if we can't create a want for them
	if s.globalLabels == nil {
		s.globalLabels = make(map[string]map[string]bool) // key -> value -> true
	}
	if s.globalLabels[labelReq.Key] == nil {
		s.globalLabels[labelReq.Key] = make(map[string]bool)
	}
	s.globalLabels[labelReq.Key][labelReq.Value] = true

	// Also try to create a want with the label for persistence
	// Use "monitor" type which is a valid, simple type that won't execute
	// Use a name without "__" prefix so it won't be filtered out in getLabels
	labelWant := &mywant.Want{
		Metadata: mywant.Metadata{
			Name: fmt.Sprintf("_label-%s-%d", labelReq.Key, time.Now().UnixNano()),
			Type: "monitor", // Use a valid type that can be registered
			ID:   generateWantID(),
			Labels: map[string]string{
				labelReq.Key: labelReq.Value,
			},
		},
		Spec: mywant.WantSpec{},
		Status: "idle",
	}

	// Add to global builder if it exists (optional - for persistence)
	// But the label is already registered above in globalLabels
	if s.globalBuilder != nil {
		_, _ = s.globalBuilder.AddWantsAsyncWithTracking([]*mywant.Want{labelWant})
	}

	// Return success response
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"key":     labelReq.Key,
		"value":   labelReq.Value,
		"message": "Label registered successfully",
	})
}

// getWantType handles GET /api/v1/want-types/{name}
func (s *Server) getWantType(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if s.wantTypeLoader == nil {
		http.Error(w, "Want type loader not initialized", http.StatusServiceUnavailable)
		return
	}

	// Extract name from URL
	parts := strings.Split(r.URL.Path, "/")
	var name string
	for i, part := range parts {
		if part == "want-types" && i+1 < len(parts) {
			name = parts[i+1]
			break
		}
	}

	if name == "" || name == "examples" {
		http.Error(w, "Invalid want type name", http.StatusBadRequest)
		return
	}

	def := s.wantTypeLoader.GetDefinition(name)
	if def == nil {
		http.Error(w, fmt.Sprintf("Want type not found: %s", name), http.StatusNotFound)
		return
	}

	json.NewEncoder(w).Encode(def)
}

// getWantTypeExamples handles GET /api/v1/want-types/{name}/examples
func (s *Server) getWantTypeExamples(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if s.wantTypeLoader == nil {
		http.Error(w, "Want type loader not initialized", http.StatusServiceUnavailable)
		return
	}

	// Extract name from URL
	parts := strings.Split(r.URL.Path, "/")
	var name string
	for i, part := range parts {
		if part == "want-types" && i+1 < len(parts) {
			name = parts[i+1]
			break
		}
	}

	if name == "" {
		http.Error(w, "Invalid want type name", http.StatusBadRequest)
		return
	}

	def := s.wantTypeLoader.GetDefinition(name)
	if def == nil {
		http.Error(w, fmt.Sprintf("Want type not found: %s", name), http.StatusNotFound)
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"name":     name,
		"examples": def.Examples,
	})
}

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
	log.SetFlags(log.Ldate | log.Ltime)

	// Parse command line arguments: [port] [host] [debug]
	// Examples:
	//   ./server           - port 8080, localhost, no debug
	//   ./server 8080      - port 8080, localhost, no debug
	//   ./server 8080 0.0.0.0 - port 8080, 0.0.0.0, no debug
	//   ./server 8080 0.0.0.0 debug - port 8080, 0.0.0.0, debug enabled
	port := 8080
	host := "localhost"
	debugEnabled := false

	if len(os.Args) > 1 {
		if p, err := strconv.Atoi(os.Args[1]); err == nil {
			port = p
		}
	}

	if len(os.Args) > 2 {
		host = os.Args[2]
	}

	if len(os.Args) > 3 {
		debugArg := strings.ToLower(os.Args[3])
		if debugArg == "debug" || debugArg == "true" || debugArg == "1" {
			debugEnabled = true
		}
	}

	// Set global debug flags (both server and engine)
	GlobalDebugEnabled = debugEnabled
	mywant.DebugLoggingEnabled = debugEnabled

	// Create server config
	config := ServerConfig{
		Port:  port,
		Host:  host,
		Debug: debugEnabled,
	}

	// Log startup info
	if debugEnabled {
		InfoLog("🐛 Debug mode ENABLED - verbose logging active")
	} else {
		InfoLog("ℹ️  Debug mode disabled - reduced logging (use 'debug' argument to enable)")
	}

	// Create and start server
	server := NewServer(config)
	if err := server.Start(); err != nil {
		log.Fatal("Server failed to start:", err)
	}
}
