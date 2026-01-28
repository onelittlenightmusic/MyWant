package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	types "mywant/engine/cmd/types"
	mywant "mywant/engine/src"

	"github.com/gorilla/mux"
	"gopkg.in/yaml.v3"
)

// ServerConfig holds server configuration
type ServerConfig struct {
	Port  int    `json:"port"`
	Host  string `json:"host"`
	Debug bool   `json:"debug"`
}
type ErrorHistoryEntry struct {
	ID          string `json:"id"`
	Timestamp   string `json:"timestamp"`
	Message     string `json:"message"`
	Status      int    `json:"status"`
	Code        string `json:"code,omitempty"`
	Type        string `json:"type,omitempty"`
	Details     string `json:"details,omitempty"`
	Endpoint    string `json:"endpoint"`
	Method      string `json:"method"`
	RequestData any    `json:"request_data,omitempty"`
	UserAgent   string `json:"user_agent,omitempty"`
	Resolved    bool   `json:"resolved"`
	Notes       string `json:"notes,omitempty"`
}

// Server represents the MyWant server
type Server struct {
	config               ServerConfig
	wants                map[string]*WantExecution        // Store active want executions
	globalBuilder        *mywant.ChainBuilder             // Global builder with running reconcile loop for server mode
	agentRegistry        *mywant.AgentRegistry            // Agent and capability registry
	recipeRegistry       *mywant.CustomTargetTypeRegistry // Recipe registry
	wantTypeLoader       *mywant.WantTypeLoader           // Want type definitions loader
	errorHistory         []ErrorHistoryEntry              // Store error history
	router               *mux.Router
	globalLabels         map[string]map[string]bool  // Globally registered labels (key -> value -> true)
	reactionQueueManager *types.ReactionQueueManager // Reaction queue manager for reminder wants
	interactionManager   *mywant.InteractionManager  // Interactive want creation manager
}

// WantExecution represents a running want execution
type WantExecution struct {
	ID      string               `json:"id"`
	Config  mywant.Config        `json:"config"` // Changed from pointer
	Status  string               `json:"status"` // "running", "completed", "failed"
	Results map[string]any       `json:"results,omitempty"`
	Builder *mywant.ChainBuilder `json:"-"` // Don't serialize builder
}

// WantResponseWithGroupedAgents wraps a Want with grouped agent history
type WantResponseWithGroupedAgents struct {
	Metadata mywant.Metadata    `json:"metadata"`
	Spec     mywant.WantSpec    `json:"spec"`
	Status   mywant.WantStatus  `json:"status"`
	History  mywant.WantHistory `json:"history"`
	State    map[string]any     `json:"state"`
}

// SaveRecipeFromWantRequest represents the request to create a recipe from an existing want
type SaveRecipeFromWantRequest struct {
	WantID   string                       `json:"wantId"`
	Metadata mywant.GenericRecipeMetadata `json:"metadata"`
}

// ========= Interactive Want Creation API Types =========

// InteractCreateResponse is returned when creating a new session
type InteractCreateResponse struct {
	SessionID string    `json:"session_id"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// InteractMessageRequest is the request to send a message to a session
type InteractMessageRequest struct {
	Message string                  `json:"message"`
	Context *mywant.InteractContext `json:"context,omitempty"`
}

// InteractMessageResponse is returned after sending a message
type InteractMessageResponse struct {
	Recommendations     []mywant.Recommendation      `json:"recommendations"`
	ConversationHistory []mywant.ConversationMessage `json:"conversation_history"`
	Timestamp           time.Time                    `json:"timestamp"`
}

// InteractDeployRequest is the request to deploy a recommendation
type InteractDeployRequest struct {
	RecommendationID string               `json:"recommendation_id"`
	Modifications    *ConfigModifications `json:"modifications,omitempty"`
}

// ConfigModifications allows modifying a recommendation before deployment
type ConfigModifications struct {
	ParameterOverrides map[string]interface{} `json:"parameterOverrides,omitempty"`
	DisableWants       []string               `json:"disableWants,omitempty"`
}

// InteractDeployResponse is returned after deploying a recommendation
type InteractDeployResponse struct {
	ExecutionID string    `json:"execution_id"`
	WantIDs     []string  `json:"want_ids"`
	Status      string    `json:"status"`
	Message     string    `json:"message"`
	Timestamp   time.Time `json:"timestamp"`
}

// ValidationResult represents the complete validation response
type ValidationResult struct {
	Valid       bool                `json:"valid"`
	FatalErrors []ValidationError   `json:"fatalErrors"`
	Warnings    []ValidationWarning `json:"warnings"`
	WantCount   int                 `json:"wantCount"`
	ValidatedAt string              `json:"validatedAt"`
}

// ValidationError represents a fatal validation error
type ValidationError struct {
	WantName  string `json:"wantName,omitempty"`
	ErrorType string `json:"errorType"`
	Field     string `json:"field,omitempty"`
	Message   string `json:"message"`
	Details   string `json:"details,omitempty"`
}

// ValidationWarning represents a non-fatal issue
type ValidationWarning struct {
	WantName     string   `json:"wantName"`
	WarningType  string   `json:"warningType"`
	Field        string   `json:"field,omitempty"`
	Message      string   `json:"message"`
	Suggestion   string   `json:"suggestion,omitempty"`
	RelatedWants []string `json:"relatedWants,omitempty"`
}

func buildWantResponse(want *mywant.Want, groupBy string) any {
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
	agentRegistry := mywant.NewAgentRegistry()

	// Load capabilities and agents from directories if they exist
	if err := agentRegistry.LoadCapabilities(mywant.CapabilitiesDir + "/"); err != nil {
		log.Printf("[SERVER] Warning: Failed to load capabilities: %v\n", err)
	}

	// Register mock server capability (ensure it exists before agent registration)
	agentRegistry.RegisterCapability(mywant.Capability{
		Name:  "mock_server_management",
		Gives: []string{"mock_server_management"},
	})

	if err := agentRegistry.LoadAgents(mywant.AgentsDir + "/"); err != nil {
		log.Printf("[SERVER] Warning: Failed to load agents: %v\n", err)
	}
	recipeRegistry := mywant.NewCustomTargetTypeRegistry()

	// Load recipes from recipes/ directory as custom types
	_ = mywant.ScanAndRegisterCustomTypes(mywant.RecipesDir, recipeRegistry)

	// Also load the recipe files themselves into the recipe registry
	_ = loadRecipeFilesIntoRegistry(mywant.RecipesDir, recipeRegistry)

	// Load want type definitions
	wantTypeLoader := mywant.NewWantTypeLoader(mywant.WantTypesDir)
	if err := wantTypeLoader.LoadAllWantTypes(); err != nil {
		log.Printf("[WARN] Failed to load want types: %v", err)
	}

	// Create interaction manager for interactive want creation
	gooseManager, err := types.GetGooseManager(context.Background())
	if err != nil {
		log.Printf("[WARN] Failed to initialize GooseManager for InteractionManager: %v", err)
		log.Printf("[WARN] Interactive want creation will not be available")
	}
	interactionManager := mywant.NewInteractionManager(wantTypeLoader, recipeRegistry, gooseManager)

	globalBuilder := mywant.NewChainBuilderWithPaths("", "engine/memory/memory-0000-latest.yaml")
	globalBuilder.SetConfigInternal(mywant.Config{Wants: []*mywant.Want{}})
	globalBuilder.SetServerMode(true)
	globalBuilder.SetAgentRegistry(agentRegistry)
	globalBuilder.SetCustomTargetRegistry(recipeRegistry) // Set custom types from recipes

	// Register the global ChainBuilder so wants can access it for the retrigger mechanism
	mywant.SetGlobalChainBuilder(globalBuilder)
	tempServer := &Server{}

	// Register dynamic agent implementations on global registry This provides the actual Action/Monitor functions for YAML-loaded agents
	tempServer.registerDynamicAgents(agentRegistry)
	types.RegisterExecutionAgents(agentRegistry)
	types.RegisterMCPAgent(agentRegistry)

	// Create reaction queue manager for reminders (multi-queue system)
	reactionQueueManager := types.NewReactionQueueManager()

	// Register reminder queue management agent (DoAgent for queue lifecycle)
	if err := types.RegisterReminderQueueAgent(agentRegistry); err != nil {
		log.Printf("[SERVER] Warning: Failed to register reminder queue agent: %v\n", err)
	}

	// Register mock server management agent (DoAgent for server lifecycle)
	if err := types.RegisterMockServerAgent(agentRegistry); err != nil {
		log.Printf("[SERVER] Warning: Failed to register mock server agent: %v\n", err)
	}

	// Register user reaction monitor agent (MonitorAgent using HTTP API)
	if err := types.RegisterUserReactionMonitorAgent(agentRegistry); err != nil {
		log.Printf("[SERVER] Warning: Failed to register user reaction monitor agent: %v\n", err)
	}

	// Register user reaction do agent
	if err := types.RegisterUserReactionDoAgent(agentRegistry); err != nil {
		log.Printf("[SERVER] Warning: Failed to register user reaction do agent: %v\n", err)
	}

	// Register knowledge management agent
	types.RegisterKnowledgeAgents(agentRegistry)

	// Initialize internal HTTP client for agents
	baseURL := fmt.Sprintf("http://%s:%d", config.Host, config.Port)
	globalBuilder.SetHTTPClient(mywant.NewHTTPClient(baseURL))

	return &Server{
		config:               config,
		wants:                make(map[string]*WantExecution),
		globalBuilder:        globalBuilder,
		agentRegistry:        agentRegistry,
		recipeRegistry:       recipeRegistry,
		wantTypeLoader:       wantTypeLoader,
		errorHistory:         make([]ErrorHistoryEntry, 0),
		router:               mux.NewRouter(),
		globalLabels:         make(map[string]map[string]bool),
		reactionQueueManager: reactionQueueManager,
		interactionManager:   interactionManager,
	}
}

// corsMiddleware adds CORS headers to allow cross-origin requests
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, PATCH")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With, Accept, Origin")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}
func (s *Server) handleOptions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, PATCH")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With, Accept, Origin")
	w.WriteHeader(http.StatusOK)
}
func (s *Server) setupRoutes() {
	s.router.Use(corsMiddleware)

	api := s.router.PathPrefix("/api/v1").Subrouter()

	// Wants CRUD endpoints
	wants := api.PathPrefix("/wants").Subrouter()
	wants.HandleFunc("", s.createWant).Methods("POST")
	wants.HandleFunc("", s.listWants).Methods("GET")
	wants.HandleFunc("", s.deleteWants).Methods("DELETE")
	wants.HandleFunc("", s.handleOptions).Methods("OPTIONS")
	wants.HandleFunc("/validate", s.validateWant).Methods("POST")
	wants.HandleFunc("/validate", s.handleOptions).Methods("OPTIONS")
	wants.HandleFunc("/export", s.exportWants).Methods("POST", "OPTIONS")
	wants.HandleFunc("/import", s.importWants).Methods("POST", "OPTIONS")
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
	wants.HandleFunc("/suspend", s.suspendWants).Methods("POST")
	wants.HandleFunc("/resume", s.resumeWants).Methods("POST")
	wants.HandleFunc("/stop", s.stopWants).Methods("POST")
	wants.HandleFunc("/start", s.startWants).Methods("POST")
	wants.HandleFunc("/{id}/labels", s.addLabelToWant).Methods("POST")
	wants.HandleFunc("/{id}/labels/{key}", s.removeLabelFromWant).Methods("DELETE")
	wants.HandleFunc("/{id}/labels", s.handleOptions).Methods("OPTIONS")
	wants.HandleFunc("/{id}/using", s.addUsingDependency).Methods("POST")
	wants.HandleFunc("/{id}/using/{key}", s.removeUsingDependency).Methods("DELETE")
	wants.HandleFunc("/{id}/using", s.handleOptions).Methods("OPTIONS")
	wants.HandleFunc("/{id}/using/{key}", s.handleOptions).Methods("OPTIONS")
	wants.HandleFunc("/{id}/labels/{key}", s.handleOptions).Methods("OPTIONS")

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
	s.router.HandleFunc("/api/v1/recipes", s.createRecipe).Methods("POST", "OPTIONS")
	s.router.HandleFunc("/api/v1/recipes/from-want", s.saveRecipeFromWant).Methods("POST", "OPTIONS")
	s.router.HandleFunc("/api/v1/recipes", s.listRecipes).Methods("GET", "OPTIONS")
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
	errors := api.PathPrefix("/errors").Subrouter()
	errors.HandleFunc("", s.listErrorHistory).Methods("GET")
	errors.HandleFunc("/{id}", s.getErrorHistoryEntry).Methods("GET")
	errors.HandleFunc("/{id}", s.updateErrorHistoryEntry).Methods("PUT")
	errors.HandleFunc("/{id}", s.deleteErrorHistoryEntry).Methods("DELETE")

	// API logs endpoints
	logs := api.PathPrefix("/logs").Subrouter()
	logs.HandleFunc("", s.getLogs).Methods("GET")
	logs.HandleFunc("", s.clearLogs).Methods("DELETE")
	logs.HandleFunc("", s.handleOptions).Methods("OPTIONS")

	// Interactive want creation endpoints
	interact := api.PathPrefix("/interact").Subrouter()
	// Session management
	interact.HandleFunc("", s.interactCreate).Methods("POST")
	interact.HandleFunc("", s.handleOptions).Methods("OPTIONS")
	// Session operations
	interact.HandleFunc("/{id}", s.interactMessage).Methods("POST")
	interact.HandleFunc("/{id}", s.interactDelete).Methods("DELETE")
	interact.HandleFunc("/{id}", s.handleOptions).Methods("OPTIONS")
	// Deployment
	interact.HandleFunc("/{id}/deploy", s.interactDeploy).Methods("POST")
	interact.HandleFunc("/{id}/deploy", s.handleOptions).Methods("OPTIONS")

	// Reactions endpoints for reminder wants - multi-queue system
	reactions := api.PathPrefix("/reactions").Subrouter()
	// Root path handlers - ORDER MATTERS: specific methods before OPTIONS
	reactions.HandleFunc("/", s.createReactionQueue).Methods("POST")
	reactions.HandleFunc("/", s.listReactionQueues).Methods("GET")
	reactions.HandleFunc("/", s.handleOptions).Methods("OPTIONS")
	// ID-based handlers
	reactions.HandleFunc("/{id}", s.getReactionQueue).Methods("GET")
	reactions.HandleFunc("/{id}", s.addReactionToQueue).Methods("PUT")
	reactions.HandleFunc("/{id}", s.deleteReactionQueue).Methods("DELETE")
	reactions.HandleFunc("/{id}", s.handleOptions).Methods("OPTIONS")

	// Health check endpoint
	s.router.HandleFunc("/health", s.healthCheck).Methods("GET")

	// Serve static files (for future web UI)
	s.router.PathPrefix("/").Handler(http.FileServer(http.Dir("./web/"))).Methods("GET")
}

// registerDynamicAgents registers implementations for special agents loaded from YAML
func (s *Server) registerDynamicAgents(agentRegistry *mywant.AgentRegistry) {
	// Override the generic implementations with specific ones for special agents
	setupFlightAPIAgents(agentRegistry)
}

func setupFlightAPIAgents(agentRegistry *mywant.AgentRegistry) {
	// Override the action implementation for the agent loaded from YAML
	if agent, exists := agentRegistry.GetAgent("agent_flight_api"); exists {
		if doAgent, ok := agent.(*mywant.DoAgent); ok {
			// Create a separate instance for the execution logic
			flightExec := types.NewAgentFlightAPI(
				"agent_flight_api",
				[]string{"flight_api_agency"},
				[]string{},
				"http://localhost:8081",
			)
			doAgent.Action = func(ctx context.Context, want *mywant.Want) error {
				_, err := flightExec.Exec(ctx, want)
				if err != nil {
					mywant.ErrorLog("[AGENT] agent_flight_api.Exec failed for want %s: %v", want.Metadata.Name, err)
				}
				return err
			}
		}
	}
}

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
			errorMsg := fmt.Sprintf("Invalid request: must be either a Want object or Config with wants array. Error: %v", configErr)
			s.globalBuilder.LogAPIOperation("POST", "/api/v1/wants", "", "error", http.StatusBadRequest, errorMsg, "")
			http.Error(w, errorMsg, http.StatusBadRequest)
			return
		}
		config = mywant.Config{Wants: []*mywant.Want{newWant}}
	}
	if err := s.validateWantTypes(config); err != nil {
		errorMsg := fmt.Sprintf("Invalid want type: %v", err)
		s.globalBuilder.LogAPIOperation("POST", "/api/v1/wants", "", "error", http.StatusBadRequest, errorMsg, "validation")
		s.logError(r, http.StatusBadRequest, errorMsg, "validation", err.Error(), "")
		http.Error(w, errorMsg, http.StatusBadRequest)
		return
	}
	if err := s.validateWantSpec(config); err != nil {
		errorMsg := fmt.Sprintf("Invalid want spec: %v", err)
		s.globalBuilder.LogAPIOperation("POST", "/api/v1/wants", "", "error", http.StatusBadRequest, errorMsg, "validation")
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
	execution := &WantExecution{
		ID:      executionID,
		Config:  config,
		Status:  "created",
		Builder: s.globalBuilder, // Use shared global builder
	}
	s.wants[executionID] = execution
	wantIDs, err := s.globalBuilder.AddWantsAsyncWithTracking(config.Wants)
	if err != nil {
		delete(s.wants, executionID)
		errorMsg := fmt.Sprintf("Failed to add wants: %v", err)
		s.globalBuilder.LogAPIOperation("POST", "/api/v1/wants", "", "error", http.StatusConflict, errorMsg, "duplicate_name")
		s.logError(r, http.StatusConflict, errorMsg, "duplicate_name", err.Error(), "")
		http.Error(w, errorMsg, http.StatusConflict)
		return
	}

	// Return immediately without blocking HTTP handler
	// The reconciliation loop adds wants asynchronously
	for _, want := range config.Wants {
		// API-level logging for want creation
		InfoLog("[API:CREATE] Want queued for addition: %s (%s, ID: %s)\n", want.Metadata.Name, want.Metadata.Type, want.Metadata.ID)
	}
	w.WriteHeader(http.StatusCreated)

	// Safety check for invalid want count
	wantCount := len(config.Wants)
	if wantCount < 0 || wantCount > 1000000 {
		errorMsg := fmt.Sprintf("Invalid want count after parsing: %d", wantCount)
		s.globalBuilder.LogAPIOperation("POST", "/api/v1/wants", "", "error", http.StatusInternalServerError, errorMsg, "parsing_error")
		s.logError(r, http.StatusInternalServerError, errorMsg, "parsing_error", "Invalid want count", "")
		http.Error(w, errorMsg, http.StatusInternalServerError)
		return
	}

	// Log successful creation
	wantNames := []string{}
	for _, want := range config.Wants {
		wantNames = append(wantNames, want.Metadata.Name)
	}
	s.globalBuilder.LogAPIOperation("POST", "/api/v1/wants", strings.Join(wantNames, ", "), "success", http.StatusCreated, "", fmt.Sprintf("Created %d want(s)", wantCount))

	response := map[string]any{
		"id":       executionID,
		"status":   execution.Status,
		"wants":    wantCount,
		"want_ids": wantIDs, // wantIDs already extracted from AddWantsAsyncWithTracking
		"message":  "Wants created and added to execution queue",
	}

	json.NewEncoder(w).Encode(response)
}

// validateWant handles POST /api/v1/wants/validate - validates want configuration without deployment
func (s *Server) validateWant(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Read request body
	var buf bytes.Buffer
	io.Copy(&buf, r.Body)
	data := buf.Bytes()

	result := ValidationResult{
		Valid:       true,
		FatalErrors: make([]ValidationError, 0),
		Warnings:    make([]ValidationWarning, 0),
		ValidatedAt: time.Now().Format(time.RFC3339),
	}

	// Step 1: Parse YAML/JSON (Fatal: syntax errors)
	var config mywant.Config
	var parseErr error

	if r.Header.Get("Content-Type") == "application/yaml" || r.Header.Get("Content-Type") == "text/yaml" {
		parseErr = yaml.Unmarshal(data, &config)
	} else {
		parseErr = json.Unmarshal(data, &config)
	}

	if parseErr != nil || len(config.Wants) == 0 {
		// Try parsing as single Want
		var newWant *mywant.Want

		if r.Header.Get("Content-Type") == "application/yaml" || r.Header.Get("Content-Type") == "text/yaml" {
			parseErr = yaml.Unmarshal(data, &newWant)
		} else {
			parseErr = json.Unmarshal(data, &newWant)
		}

		if parseErr != nil || newWant == nil {
			result.Valid = false
			result.FatalErrors = append(result.FatalErrors, ValidationError{
				ErrorType: "syntax",
				Message:   "Invalid YAML/JSON syntax",
				Details:   fmt.Sprintf("%v", parseErr),
			})
			statusCode := http.StatusBadRequest
			w.WriteHeader(statusCode)
			json.NewEncoder(w).Encode(result)
			s.globalBuilder.LogAPIOperation("POST", "/api/v1/wants/validate", "", "error", statusCode, "Syntax error", "validation")
			return
		}
		config = mywant.Config{Wants: []*mywant.Want{newWant}}
	}

	result.WantCount = len(config.Wants)

	// Step 2: Fatal error checks
	s.collectFatalErrors(&config, &result)

	// Step 3: Warning checks (only if no fatal errors)
	if result.Valid {
		s.collectWarnings(&config, &result)
	}

	// Return validation result
	statusCode := http.StatusOK
	if !result.Valid {
		statusCode = http.StatusBadRequest
	}

	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(result)

	// Log validation request
	s.globalBuilder.LogAPIOperation("POST", "/api/v1/wants/validate",
		fmt.Sprintf("%d wants", result.WantCount),
		"success", statusCode, "", "")
}

// listWants handles GET /api/v1/wants - lists all wants in memory dump format
func (s *Server) listWants(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Parse includeSystemWants query parameter (default: false)
	includeSystemWants := false
	if includeSystemWantsStr := r.URL.Query().Get("includeSystemWants"); includeSystemWantsStr != "" {
		includeSystemWants = strings.ToLower(includeSystemWantsStr) == "true"
	}

	// Parse type query parameter for filtering by want type
	wantTypeFilter := r.URL.Query().Get("type")

	// Parse label query parameters for filtering by labels (format: key=value)
	labelFilters := make(map[string]string)
	for _, label := range r.URL.Query()["label"] {
		parts := strings.SplitN(label, "=", 2)
		if len(parts) == 2 {
			labelFilters[parts[0]] = parts[1]
		}
	}

	// Collect all wants from all executions in memory dump format Use map to deduplicate wants by ID (same want may exist across multiple executions)
	wantsByID := make(map[string]*mywant.Want)

	for _, execution := range s.wants {
		currentStates := execution.Builder.GetAllWantStates()
		for _, want := range currentStates {
			wantCopy := &mywant.Want{
				Metadata:    want.Metadata,
				Spec:        want.Spec,
				Status:      want.GetStatus(),
				History:     want.History,
				State:       want.GetExplicitState(),
				HiddenState: want.GetHiddenState(),
			}
			// Calculate hash for change detection
			wantCopy.Hash = mywant.CalculateWantHash(wantCopy)
			wantsByID[want.Metadata.ID] = wantCopy
		}
	}

	// If no wants from executions, also check global builder (for wants loaded from memory file)
	if len(wantsByID) == 0 && s.globalBuilder != nil {
		currentStates := s.globalBuilder.GetAllWantStates()
		for _, want := range currentStates {
			wantCopy := &mywant.Want{
				Metadata:    want.Metadata,
				Spec:        want.Spec,
				Status:      want.GetStatus(),
				History:     want.History,
				State:       want.GetExplicitState(),
				HiddenState: want.GetHiddenState(),
			}
			// Calculate hash for change detection
			wantCopy.Hash = mywant.CalculateWantHash(wantCopy)
			wantsByID[want.Metadata.ID] = wantCopy
		}
	}
	allWants := make([]*mywant.Want, 0, len(wantsByID))
	for _, want := range wantsByID {
		// Filter out system wants if includeSystemWants is false
		if !includeSystemWants && want.Metadata.IsSystemWant {
			continue
		}
		// Filter by want type if specified
		if wantTypeFilter != "" && want.Metadata.Type != wantTypeFilter {
			continue
		}
		// Filter by labels if specified
		if len(labelFilters) > 0 {
			matchesAllLabels := true
			for key, value := range labelFilters {
				if want.Metadata.Labels == nil {
					matchesAllLabels = false
					break
				}
				labelValue, exists := want.Metadata.Labels[key]
				if !exists || labelValue != value {
					matchesAllLabels = false
					break
				}
			}
			if !matchesAllLabels {
				continue
			}
		}
		allWants = append(allWants, want)
	}
	response := map[string]any{
		"timestamp":    time.Now().Format(time.RFC3339),
		"execution_id": fmt.Sprintf("api-dump-%d", time.Now().Unix()),
		"wants":        allWants,
	}

	json.NewEncoder(w).Encode(response)
}

// exportWants handles POST /api/v1/wants/export - exports all wants as YAML
func (s *Server) exportWants(w http.ResponseWriter, r *http.Request) {
	// Parse includeSystemWants query parameter (default: false)
	includeSystemWants := false
	if includeSystemWantsStr := r.URL.Query().Get("includeSystemWants"); includeSystemWantsStr != "" {
		includeSystemWants = strings.ToLower(includeSystemWantsStr) == "true"
	}

	// Collect all wants from all executions (same logic as listWants)
	wantsByID := make(map[string]*mywant.Want)

	for _, execution := range s.wants {
		currentStates := execution.Builder.GetAllWantStates()
		for _, want := range currentStates {
			wantCopy := &mywant.Want{
				Metadata:    want.Metadata,
				Spec:        want.Spec,
				Status:      want.GetStatus(),
				History:     want.History,
				State:       want.GetExplicitState(),
				HiddenState: want.GetHiddenState(),
			}
			// Calculate hash for change detection
			wantCopy.Hash = mywant.CalculateWantHash(wantCopy)
			wantsByID[want.Metadata.ID] = wantCopy
		}
	}

	// If no wants from executions, also check global builder
	if len(wantsByID) == 0 && s.globalBuilder != nil {
		currentStates := s.globalBuilder.GetAllWantStates()
		for _, want := range currentStates {
			wantCopy := &mywant.Want{
				Metadata:    want.Metadata,
				Spec:        want.Spec,
				Status:      want.GetStatus(),
				History:     want.History,
				State:       want.GetExplicitState(),
				HiddenState: want.GetHiddenState(),
			}
			wantsByID[want.Metadata.ID] = wantCopy
		}
	}

	// Build list of wants to export (filter out system wants if needed)
	allWants := make([]*mywant.Want, 0, len(wantsByID))
	for _, want := range wantsByID {
		if !includeSystemWants && want.Metadata.IsSystemWant {
			continue
		}
		allWants = append(allWants, want)
	}

	// Create config with all wants
	config := mywant.Config{
		Wants: allWants,
	}

	// Marshal to YAML
	yamlData, err := yaml.Marshal(&config)
	if err != nil {
		errorMsg := fmt.Sprintf("Failed to marshal wants to YAML: %v", err)
		s.globalBuilder.LogAPIOperation("EXPORT", "/api/v1/wants/export", "", "error", http.StatusInternalServerError, errorMsg, "marshaling_error")
		http.Error(w, errorMsg, http.StatusInternalServerError)
		return
	}

	// Set response headers for file download
	w.Header().Set("Content-Type", "application/yaml")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"wants-export-%d.yaml\"", time.Now().Unix()))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(yamlData)))

	w.WriteHeader(http.StatusOK)
	w.Write(yamlData)

	// Collect want names for logging
	wantNames := []string{}
	for _, want := range allWants {
		wantNames = append(wantNames, want.Metadata.Name)
	}

	// Log successful export
	s.globalBuilder.LogAPIOperation(
		"EXPORT",
		"/api/v1/wants/export",
		strings.Join(wantNames, ", "),
		"success",
		http.StatusOK,
		"",
		fmt.Sprintf("Exported %d wants to YAML (%d bytes)", len(allWants), len(yamlData)),
	)

	InfoLog("[API:EXPORT] Exported %d wants to YAML\n", len(allWants))
}

// importWants handles POST /api/v1/wants/import - imports wants from YAML
func (s *Server) importWants(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Read request body
	var buf bytes.Buffer
	io.Copy(&buf, r.Body)
	data := buf.Bytes()

	// Parse YAML into Config
	var config mywant.Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		errorMsg := fmt.Sprintf("Failed to parse YAML: %v", err)
		s.globalBuilder.LogAPIOperation("IMPORT", "/api/v1/wants/import", "", "error", http.StatusBadRequest, errorMsg, "parsing_error")
		http.Error(w, errorMsg, http.StatusBadRequest)
		return
	}

	if len(config.Wants) == 0 {
		errorMsg := "No wants found in YAML"
		s.globalBuilder.LogAPIOperation("IMPORT", "/api/v1/wants/import", "", "error", http.StatusBadRequest, errorMsg, "empty_wants")
		http.Error(w, errorMsg, http.StatusBadRequest)
		return
	}

	// Validate want types
	if err := s.validateWantTypes(config); err != nil {
		errorMsg := fmt.Sprintf("Invalid want type: %v", err)
		s.globalBuilder.LogAPIOperation("IMPORT", "/api/v1/wants/import", "", "error", http.StatusBadRequest, errorMsg, "validation")
		s.logError(r, http.StatusBadRequest, errorMsg, "validation", err.Error(), "")
		http.Error(w, errorMsg, http.StatusBadRequest)
		return
	}

	// Validate want spec
	if err := s.validateWantSpec(config); err != nil {
		errorMsg := fmt.Sprintf("Invalid want spec: %v", err)
		s.globalBuilder.LogAPIOperation("IMPORT", "/api/v1/wants/import", "", "error", http.StatusBadRequest, errorMsg, "validation")
		s.logError(r, http.StatusBadRequest, errorMsg, "validation", err.Error(), "")
		http.Error(w, errorMsg, http.StatusBadRequest)
		return
	}

	// Verify all wants have IDs (from export)
	for _, want := range config.Wants {
		if want.Metadata.ID == "" {
			errorMsg := "Imported wants must have IDs (from export)"
			s.globalBuilder.LogAPIOperation("IMPORT", "/api/v1/wants/import", "", "error", http.StatusBadRequest, errorMsg, "missing_ids")
			http.Error(w, errorMsg, http.StatusBadRequest)
			return
		}
	}

	// Generate unique ID for this execution
	executionID := generateWantID()
	execution := &WantExecution{
		ID:      executionID,
		Config:  config,
		Status:  "created",
		Builder: s.globalBuilder,
	}
	s.wants[executionID] = execution

	// Add wants to global builder
	wantIDs, err := s.globalBuilder.AddWantsAsyncWithTracking(config.Wants)
	if err != nil {
		delete(s.wants, executionID)
		errorMsg := fmt.Sprintf("Failed to add wants: %v", err)
		s.globalBuilder.LogAPIOperation("IMPORT", "/api/v1/wants/import", "", "error", http.StatusConflict, errorMsg, "duplicate_ids")
		s.logError(r, http.StatusConflict, errorMsg, "duplicate_id", err.Error(), "")
		http.Error(w, errorMsg, http.StatusConflict)
		return
	}

	// Restore state asynchronously (non-blocking)
	// This allows wants to be added to the system while we return response immediately
	go func() {
		// Wait a short time for wants to be added (with very short timeout)
		maxAttempts := 20
		for attempt := 0; attempt < maxAttempts; attempt++ {
			if s.globalBuilder.AreWantsAdded(wantIDs) {
				break
			}
			time.Sleep(5 * time.Millisecond)
		}

		// Restore state for each want
		for _, want := range config.Wants {
			if importedWant, _, found := s.globalBuilder.FindWantByID(want.Metadata.ID); found {
				// Restore State
				if want.State != nil && len(want.State) > 0 {
					for key, value := range want.State {
						importedWant.StoreState(key, value)
					}
					InfoLog("[API:IMPORT] Restored state for want %s: %d fields\n", want.Metadata.ID, len(want.State))
				}

				// Note: HiddenState is already part of State map, so it's restored above
			}
		}
	}()

	// Collect want names for logging
	wantNames := []string{}
	for _, want := range config.Wants {
		InfoLog("[API:IMPORT] Want imported: %s (%s, ID: %s)\n", want.Metadata.Name, want.Metadata.Type, want.Metadata.ID)
		wantNames = append(wantNames, want.Metadata.Name)
	}

	// Log successful import with summary
	s.globalBuilder.LogAPIOperation(
		"IMPORT",
		"/api/v1/wants/import",
		strings.Join(wantNames, ", "),
		"success",
		http.StatusCreated,
		"",
		fmt.Sprintf("Imported %d wants with state restoration", len(config.Wants)),
	)

	w.WriteHeader(http.StatusCreated)
	response := map[string]any{
		"id":      executionID,
		"status":  execution.Status,
		"wants":   len(config.Wants),
		"message": fmt.Sprintf("Successfully imported %d wants", len(config.Wants)),
	}
	json.NewEncoder(w).Encode(response)
}

func (s *Server) getWant(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	wantID := vars["id"]
	groupBy := r.URL.Query().Get("groupBy")                                    // "name" or "type"
	includeConnectivity := r.URL.Query().Get("connectivityMetadata") == "true" // Whether to include ConnectivityMetadata

	// Search for the want by metadata.id across all executions using universal search
	for _, execution := range s.wants {
		if execution.Builder != nil {
			if want, _, found := execution.Builder.FindWantByID(wantID); found {
				// Create response based on whether connectivity metadata is requested
				if includeConnectivity {
					// Include ConnectivityMetadata
					wantCopy := &mywant.Want{
						Metadata:             want.Metadata,
						Spec:                 want.Spec,
						Status:               want.GetStatus(),
						History:              want.History,
						State:                want.GetExplicitState(),
						HiddenState:          want.GetHiddenState(),
						ConnectivityMetadata: want.ConnectivityMetadata,
					}
					// If groupBy is specified, return response with grouped agent history
					if groupBy != "" {
						response := buildWantResponse(wantCopy, groupBy)
						json.NewEncoder(w).Encode(response)
						return
					}
					json.NewEncoder(w).Encode(wantCopy)
					return
				}

				// Default response without ConnectivityMetadata
				wantCopy := &mywant.Want{
					Metadata:    want.Metadata,
					Spec:        want.Spec,
					Status:      want.GetStatus(),
					History:     want.History,
					State:       want.GetExplicitState(),
					HiddenState: want.GetHiddenState(),
				}

				// If groupBy is specified, return response with grouped agent history
				if groupBy != "" {
					response := buildWantResponse(wantCopy, groupBy)
					json.NewEncoder(w).Encode(response)
					return
				}
				json.NewEncoder(w).Encode(wantCopy)
				return
			}
		}
	}

	// If not found in executions, also search in global builder (for wants loaded from memory file)
	if s.globalBuilder != nil {
		if want, _, found := s.globalBuilder.FindWantByID(wantID); found {
			// Create response based on whether connectivity metadata is requested
			if includeConnectivity {
				// Include ConnectivityMetadata
				wantCopy := &mywant.Want{
					Metadata:             want.Metadata,
					Spec:                 want.Spec,
					Status:               want.GetStatus(),
					History:              want.History,
					State:                want.GetExplicitState(),
					HiddenState:          want.GetHiddenState(),
					ConnectivityMetadata: want.ConnectivityMetadata,
				}
				// If groupBy is specified, return response with grouped agent history
				if groupBy != "" {
					response := buildWantResponse(wantCopy, groupBy)
					json.NewEncoder(w).Encode(response)
					return
				}
				json.NewEncoder(w).Encode(wantCopy)
				return
			}

			// Default response without ConnectivityMetadata
			wantCopy := &mywant.Want{
				Metadata:    want.Metadata,
				Spec:        want.Spec,
				Status:      want.GetStatus(),
				History:     want.History,
				State:       want.GetExplicitState(),
				HiddenState: want.GetHiddenState(),
			}

			// If groupBy is specified, return response with grouped agent history
			if groupBy != "" {
				response := buildWantResponse(wantCopy, groupBy)
				json.NewEncoder(w).Encode(response)
				return
			}
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
				log.Printf("[API:UPDATE] Found want in execution %s\n", i)
				targetExecution = execution
				foundWant = want
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
		// If not found in executions, check global builder
		if s.globalBuilder != nil {
			if want, _, found := s.globalBuilder.FindWantByID(wantID); found {
				foundWant = want
				log.Printf("[API:UPDATE] Found want in global builder: %s\n", wantID)
				// For global wants, we don't have a targetExecution in s.wants
			}
		}
	}

	if foundWant == nil {
		errorMsg := "Want not found"
		s.globalBuilder.LogAPIOperation("PUT", "/api/v1/wants/{id}", wantID, "error", http.StatusNotFound, errorMsg, "want_not_found")
		http.Error(w, errorMsg, http.StatusNotFound)
		return
	}

	// Read request body robustly
	bodyData, err := io.ReadAll(r.Body)
	if err != nil {
		errorMsg := fmt.Sprintf("Failed to read request body: %v", err)
		http.Error(w, errorMsg, http.StatusBadRequest)
		return
	}

	var updatedWant *mywant.Want
	contentType := r.Header.Get("Content-Type")
	if strings.Contains(contentType, "yaml") {
		if err := yaml.Unmarshal(bodyData, &updatedWant); err != nil {
			errorMsg := fmt.Sprintf("Invalid YAML want: %v", err)
			s.globalBuilder.LogAPIOperation("PUT", "/api/v1/wants/{id}", wantID, "error", http.StatusBadRequest, errorMsg, "parsing_error")
			http.Error(w, errorMsg, http.StatusBadRequest)
			return
		}
	} else {
		if err := json.Unmarshal(bodyData, &updatedWant); err != nil {
			errorMsg := fmt.Sprintf("Invalid JSON want: %v", err)
			s.globalBuilder.LogAPIOperation("PUT", "/api/v1/wants/{id}", wantID, "error", http.StatusBadRequest, errorMsg, "parsing_error")
			http.Error(w, errorMsg, http.StatusBadRequest)
			return
		}
	}

	if updatedWant == nil {
		errorMsg := "Want object is required"
		s.globalBuilder.LogAPIOperation("PUT", "/api/v1/wants/{id}", wantID, "error", http.StatusBadRequest, errorMsg, "validation")
		http.Error(w, errorMsg, http.StatusBadRequest)
		return
	}

	// Relaxed status check: allow updates even if reaching/running
	// The ChainBuilder.UpdateWant method handles dynamic updates safely

	tempConfig := mywant.Config{Wants: []*mywant.Want{updatedWant}}
	if err := s.validateWantTypes(tempConfig); err != nil {
		errorMsg := fmt.Sprintf("Invalid want type: %v", err)
		s.globalBuilder.LogAPIOperation("PUT", "/api/v1/wants/{id}", wantID, "error", http.StatusBadRequest, errorMsg, "validation")
		s.logError(r, http.StatusBadRequest, errorMsg, "validation", err.Error(), "")
		http.Error(w, errorMsg, http.StatusBadRequest)
		return
	}

	if err := s.validateWantSpec(tempConfig); err != nil {
		errorMsg := fmt.Sprintf("Invalid want spec: %v", err)
		s.globalBuilder.LogAPIOperation("PUT", "/api/v1/wants/{id}", wantID, "error", http.StatusBadRequest, errorMsg, "validation")
		s.logError(r, http.StatusBadRequest, errorMsg, "validation", err.Error(), "")
		http.Error(w, errorMsg, http.StatusBadRequest)
		return
	}

	// Preserve the original want ID
	updatedWant.Metadata.ID = foundWant.Metadata.ID

	// Update config in target execution if it exists
	if targetExecution != nil {
		if targetWantIndex >= 0 && targetWantIndex < len(targetExecution.Config.Wants) {
			targetExecution.Config.Wants[targetWantIndex] = updatedWant
		} else {
			targetExecution.Config.Wants = append(targetExecution.Config.Wants, updatedWant)
		}

		// Use execution's own builder
		if targetExecution.Builder != nil {
			targetExecution.Builder.UpdateWant(updatedWant)
		}
		targetExecution.Status = "updated"
	} else if s.globalBuilder != nil {
		// Fallback to global builder for wants not associated with a specific execution
		s.globalBuilder.UpdateWant(updatedWant)
	}

	s.globalBuilder.LogAPIOperation("PUT", "/api/v1/wants/{id}", wantID, "success", http.StatusOK, "", fmt.Sprintf("Updated want: %s", updatedWant.Metadata.Name))
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
					s.globalBuilder.LogAPIOperation("DELETE", "/api/v1/wants/{id}", wantID, "error", http.StatusInternalServerError, err.Error(), "deletion_failed")
				} else {
					// Return immediately without blocking HTTP handler
					// The reconciliation loop processes deletion asynchronously
					s.globalBuilder.LogAPIOperation("DELETE", "/api/v1/wants/{id}", wantID, "success", http.StatusNoContent, "", fmt.Sprintf("Deletion queued for want: %s", wantNameToDelete))
					InfoLog("Deletion queued asynchronously for want %s (type: %s)\n", wantID, wantTypeToDelete)
				}

				// NOTE: Do NOT call SetConfigInternal with partial execution config! API-created wants from different executions should not affect the global config. The global config is only for wants loaded from YAML files or recipe-based creation. Calling SetConfigInternal here with an incomplete config causes other wants to be deleted incorrectly.
				// This was the root cause of the bug where deleting buffet caused coordinator to be deleted.
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
				s.globalBuilder.LogAPIOperation("DELETE", "/api/v1/wants/{id}", wantID, "error", http.StatusInternalServerError, errorMsg, "deletion_failed")
				s.logError(r, http.StatusInternalServerError, errorMsg, "deletion", err.Error(), wantID)
				http.Error(w, errorMsg, http.StatusInternalServerError)
				return
			}

			// Return immediately without blocking HTTP handler
			// The reconciliation loop processes deletion asynchronously
			InfoLog("Deletion queued asynchronously for want %s from global builder\n", wantID)
			s.globalBuilder.LogAPIOperation("DELETE", "/api/v1/wants/{id}", wantID, "success", http.StatusNoContent, "", "Deletion queued from global builder")
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}

	ErrorLog("Want %s not found in any execution or global builder\n", wantID)

	errorMsg := fmt.Sprintf("Want not found: %s", wantID)
	s.globalBuilder.LogAPIOperation("DELETE", "/api/v1/wants/{id}", wantID, "error", http.StatusNotFound, errorMsg, "want_not_found")
	s.logError(r, http.StatusNotFound, errorMsg, "deletion", "want not found", wantID)
	http.Error(w, "Want not found", http.StatusNotFound)
}

// deleteWants handles DELETE /api/v1/wants - deletes multiple wants by their IDs
func (s *Server) deleteWants(w http.ResponseWriter, r *http.Request) {
	// Try to get IDs from query parameter first (convenient for some clients)
	idsParam := r.URL.Query().Get("ids")
	if idsParam != "" {
		w.Header().Set("Content-Type", "application/json")
		wantIDs := strings.Split(idsParam, ",")
		if err := s.globalBuilder.QueueWantDelete(wantIDs); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]any{
			"message": "Batch deletion queued",
			"ids":     wantIDs,
		})
		return
	}

	// Otherwise use the helper which handles JSON body
	s.handleBatchOperation(w, r, "delete")
}

// suspendWants handles POST /api/v1/wants/suspend - suspends multiple wants
func (s *Server) suspendWants(w http.ResponseWriter, r *http.Request) {
	s.handleBatchOperation(w, r, "suspend")
}

// resumeWants handles POST /api/v1/wants/resume - resumes multiple wants
func (s *Server) resumeWants(w http.ResponseWriter, r *http.Request) {
	s.handleBatchOperation(w, r, "resume")
}

// stopWants handles POST /api/v1/wants/stop - stops multiple wants
func (s *Server) stopWants(w http.ResponseWriter, r *http.Request) {
	s.handleBatchOperation(w, r, "stop")
}

// startWants handles POST /api/v1/wants/start - starts multiple wants
func (s *Server) startWants(w http.ResponseWriter, r *http.Request) {
	s.handleBatchOperation(w, r, "start")
}

// handleBatchOperation helper for batch operations (suspend, resume, stop, start)
func (s *Server) handleBatchOperation(w http.ResponseWriter, r *http.Request, operation string) {
	w.Header().Set("Content-Type", "application/json")

	var body struct {
		IDs []string `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errorMsg := "Invalid request format"
		http.Error(w, errorMsg, http.StatusBadRequest)
		return
	}

	if len(body.IDs) == 0 {
		errorMsg := "No want IDs provided"
		http.Error(w, errorMsg, http.StatusBadRequest)
		return
	}

	var err error
	switch operation {
	case "delete":
		err = s.globalBuilder.QueueWantDelete(body.IDs)
	case "suspend":
		err = s.globalBuilder.QueueWantSuspend(body.IDs)
	case "resume":
		err = s.globalBuilder.QueueWantResume(body.IDs)
	case "stop":
		err = s.globalBuilder.QueueWantStop(body.IDs)
	case "start":
		err = s.globalBuilder.QueueWantStart(body.IDs)
	}

	if err != nil {
		errorMsg := fmt.Sprintf("Failed to queue batch %s: %v", operation, err)
		s.globalBuilder.LogAPIOperation("POST", "/api/v1/wants/"+operation, strings.Join(body.IDs, ","), "error", http.StatusInternalServerError, errorMsg, "queue_full")
		http.Error(w, errorMsg, http.StatusInternalServerError)
		return
	}

	s.globalBuilder.LogAPIOperation("POST", "/api/v1/wants/"+operation, strings.Join(body.IDs, ","), "success", http.StatusAccepted, "", fmt.Sprintf("Batch %s queued for %d wants", operation, len(body.IDs)))
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]any{
		"message":   fmt.Sprintf("Batch %s operation queued", operation),
		"ids":       body.IDs,
		"count":     len(body.IDs),
		"timestamp": time.Now().Format(time.RFC3339),
	})
}
func (s *Server) getWantStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	wantID := vars["id"]

	// Search for the want by metadata.id across all executions using universal search
	for _, execution := range s.wants {
		if execution.Builder != nil {
			if want, _, found := execution.Builder.FindWantByID(wantID); found {
				// Found the want, return its status
				status := map[string]any{
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
			status := map[string]any{
				"id":     want.Metadata.ID,
				"status": string(want.GetStatus()),
			}
			json.NewEncoder(w).Encode(status)
			return
		}
	}

	http.Error(w, "Want not found", http.StatusNotFound)
}
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
					want.State = make(map[string]any)
				}
				results := map[string]any{
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
			results := map[string]any{
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
	health := map[string]any{
		"status":  "healthy",
		"wants":   len(s.wants),
		"version": "1.0.0",
		"server":  "mywant",
	}
	json.NewEncoder(w).Encode(health)
}

// ======= AGENT CRUD HANDLERS =======
func (s *Server) createAgent(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var agentData struct {
		Name         string   `json:"name"`
		Type         string   `json:"type"`
		Capabilities []string `json:"capabilities"`
		Uses         []string `json:"uses"`
	}

	if err := json.NewDecoder(r.Body).Decode(&agentData); err != nil {
		errorMsg := fmt.Sprintf("Invalid JSON: %v", err)
		s.globalBuilder.LogAPIOperation("POST", "/api/v1/agents", "", "error", http.StatusBadRequest, errorMsg, "invalid_json")
		http.Error(w, errorMsg, http.StatusBadRequest)
		return
	}
	var agent mywant.Agent
	baseAgent := mywant.BaseAgent{
		Name:         agentData.Name,
		Capabilities: agentData.Capabilities,
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
		errorMsg := "Invalid agent type. Must be 'do' or 'monitor'"
		s.globalBuilder.LogAPIOperation("POST", "/api/v1/agents", agentData.Name, "error", http.StatusBadRequest, errorMsg, "invalid_type")
		http.Error(w, errorMsg, http.StatusBadRequest)
		return
	}

	// Register the agent
	s.agentRegistry.RegisterAgent(agent)

	s.globalBuilder.LogAPIOperation("POST", "/api/v1/agents", agentData.Name, "success", http.StatusCreated, "", fmt.Sprintf("Created agent type=%s", agentData.Type))
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]any{
		"name":         agent.GetName(),
		"type":         agent.GetType(),
		"capabilities": agent.GetCapabilities(),
	})
}

// listAgents handles GET /api/v1/agents - lists all agents
func (s *Server) listAgents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	agents := s.agentRegistry.GetAllAgents()
	agentResponses := make([]map[string]any, len(agents))
	for i, agent := range agents {
		agentResponses[i] = map[string]any{
			"name":         agent.GetName(),
			"type":         agent.GetType(),
			"capabilities": agent.GetCapabilities(),
		}
	}

	response := map[string]any{
		"agents": agentResponses,
	}

	json.NewEncoder(w).Encode(response)
}
func (s *Server) getAgent(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	agentName := vars["name"]

	agent, exists := s.agentRegistry.GetAgent(agentName)
	if !exists {
		http.Error(w, "Agent not found", http.StatusNotFound)
		return
	}

	response := map[string]any{
		"name":         agent.GetName(),
		"type":         agent.GetType(),
		"capabilities": agent.GetCapabilities(),
	}

	json.NewEncoder(w).Encode(response)
}

// deleteAgent handles DELETE /api/v1/agents/{name} - deletes an agent
func (s *Server) deleteAgent(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	agentName := vars["name"]
	if !s.agentRegistry.UnregisterAgent(agentName) {
		errorMsg := "Agent not found"
		s.globalBuilder.LogAPIOperation("DELETE", "/api/v1/agents/{name}", agentName, "error", http.StatusNotFound, errorMsg, "agent_not_found")
		http.Error(w, errorMsg, http.StatusNotFound)
		return
	}

	s.globalBuilder.LogAPIOperation("DELETE", "/api/v1/agents/{name}", agentName, "success", http.StatusNoContent, "", "Agent deleted")
	w.WriteHeader(http.StatusNoContent)
}

// ======= CAPABILITY CRUD HANDLERS =======
func (s *Server) createCapability(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var capability mywant.Capability
	if err := json.NewDecoder(r.Body).Decode(&capability); err != nil {
		errorMsg := fmt.Sprintf("Invalid JSON: %v", err)
		s.globalBuilder.LogAPIOperation("POST", "/api/v1/capabilities", "", "error", http.StatusBadRequest, errorMsg, "invalid_json")
		http.Error(w, errorMsg, http.StatusBadRequest)
		return
	}

	// Register the capability
	s.agentRegistry.RegisterCapability(capability)

	s.globalBuilder.LogAPIOperation("POST", "/api/v1/capabilities", capability.Name, "success", http.StatusCreated, "", fmt.Sprintf("Created capability"))
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(capability)
}

// listCapabilities handles GET /api/v1/capabilities - lists all capabilities
func (s *Server) listCapabilities(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	capabilities := s.agentRegistry.GetAllCapabilities()

	response := map[string]any{
		"capabilities": capabilities,
	}

	json.NewEncoder(w).Encode(response)
}
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
	if !s.agentRegistry.UnregisterCapability(capabilityName) {
		errorMsg := "Capability not found"
		s.globalBuilder.LogAPIOperation("DELETE", "/api/v1/capabilities/{name}", capabilityName, "error", http.StatusNotFound, errorMsg, "capability_not_found")
		http.Error(w, errorMsg, http.StatusNotFound)
		return
	}

	s.globalBuilder.LogAPIOperation("DELETE", "/api/v1/capabilities/{name}", capabilityName, "success", http.StatusNoContent, "", "Capability deleted")
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) findAgentsByCapability(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	capabilityName := vars["name"]

	agents := s.agentRegistry.FindAgentsByGives(capabilityName)
	if agents == nil {
		agents = make([]mywant.Agent, 0)
	}
	agentResponses := make([]map[string]any, len(agents))
	for i, agent := range agents {
		agentResponses[i] = map[string]any{
			"name":         agent.GetName(),
			"type":         agent.GetType(),
			"capabilities": agent.GetCapabilities(),
		}
	}

	response := map[string]any{
		"capability": capabilityName,
		"agents":     agentResponses,
	}

	json.NewEncoder(w).Encode(response)
}

// suspendWant handles POST /api/v1/wants/{id}/suspend - suspends want execution
func (s *Server) suspendWant(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	wantID := vars["id"]

	// Verify want exists before queuing operation
	found := false
	// Check in executions
	for _, execution := range s.wants {
		if execution.Builder != nil {
			if _, _, ok := execution.Builder.FindWantByID(wantID); ok {
				found = true
				break
			}
		}
	}
	// Check in global builder
	if !found && s.globalBuilder != nil {
		if _, _, ok := s.globalBuilder.FindWantByID(wantID); ok {
			found = true
		}
	}

	if !found {
		errorMsg := fmt.Sprintf("Want with ID %s not found", wantID)
		s.globalBuilder.LogAPIOperation("POST", "/api/v1/wants/{id}/suspend", wantID, "error", http.StatusNotFound, errorMsg, "want_not_found")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": errorMsg})
		return
	}

	// Queue suspension operation
	if err := s.globalBuilder.QueueWantSuspend([]string{wantID}); err != nil {
		errorMsg := fmt.Sprintf("Failed to queue suspension: %v", err)
		s.globalBuilder.LogAPIOperation("POST", "/api/v1/wants/{id}/suspend", wantID, "error", http.StatusConflict, errorMsg, "queue_full")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]string{"error": errorMsg})
		return
	}

	s.globalBuilder.LogAPIOperation("POST", "/api/v1/wants/{id}/suspend", wantID, "success", http.StatusAccepted, "", "Suspension queued")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]any{
		"message":   "Suspension operation queued",
		"wantId":    wantID,
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

// resumeWant handles POST /api/v1/wants/{id}/resume - resumes want execution
func (s *Server) resumeWant(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	wantID := vars["id"]

	// Verify want exists before queuing operation
	found := false
	// Check in executions
	for _, execution := range s.wants {
		if execution.Builder != nil {
			if _, _, ok := execution.Builder.FindWantByID(wantID); ok {
				found = true
				break
			}
		}
	}
	// Check in global builder
	if !found && s.globalBuilder != nil {
		if _, _, ok := s.globalBuilder.FindWantByID(wantID); ok {
			found = true
		}
	}

	if !found {
		errorMsg := fmt.Sprintf("Want with ID %s not found", wantID)
		s.globalBuilder.LogAPIOperation("POST", "/api/v1/wants/{id}/resume", wantID, "error", http.StatusNotFound, errorMsg, "want_not_found")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": errorMsg})
		return
	}

	// Queue resume operation
	if err := s.globalBuilder.QueueWantResume([]string{wantID}); err != nil {
		errorMsg := fmt.Sprintf("Failed to queue resume: %v", err)
		s.globalBuilder.LogAPIOperation("POST", "/api/v1/wants/{id}/resume", wantID, "error", http.StatusConflict, errorMsg, "queue_full")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]string{"error": errorMsg})
		return
	}

	s.globalBuilder.LogAPIOperation("POST", "/api/v1/wants/{id}/resume", wantID, "success", http.StatusAccepted, "", "Resume queued")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]any{
		"message":   "Resume operation queued",
		"wantId":    wantID,
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

// stopWant handles POST /api/v1/wants/{id}/stop - stops want execution
func (s *Server) stopWant(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	wantID := vars["id"]

	// Verify want exists before queuing operation
	found := false
	// Check in executions
	for _, execution := range s.wants {
		if execution.Builder != nil {
			if _, _, ok := execution.Builder.FindWantByID(wantID); ok {
				found = true
				break
			}
		}
	}
	// Check in global builder
	if !found && s.globalBuilder != nil {
		if _, _, ok := s.globalBuilder.FindWantByID(wantID); ok {
			found = true
		}
	}

	if !found {
		errorMsg := fmt.Sprintf("Want with ID %s not found", wantID)
		s.globalBuilder.LogAPIOperation("POST", "/api/v1/wants/{id}/stop", wantID, "error", http.StatusNotFound, errorMsg, "want_not_found")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": errorMsg})
		return
	}

	// Queue stop operation
	if err := s.globalBuilder.QueueWantStop([]string{wantID}); err != nil {
		errorMsg := fmt.Sprintf("Failed to queue stop: %v", err)
		s.globalBuilder.LogAPIOperation("POST", "/api/v1/wants/{id}/stop", wantID, "error", http.StatusConflict, errorMsg, "queue_full")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]string{"error": errorMsg})
		return
	}

	s.globalBuilder.LogAPIOperation("POST", "/api/v1/wants/{id}/stop", wantID, "success", http.StatusAccepted, "", "Stop queued")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]any{
		"message":   "Stop operation queued",
		"wantId":    wantID,
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

// startWant handles POST /api/v1/wants/{id}/start - starts/restarts want execution
func (s *Server) startWant(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	wantID := vars["id"]

	// Verify want exists before queuing operation
	found := false
	// Check in executions
	for _, execution := range s.wants {
		if execution.Builder != nil {
			if _, _, ok := execution.Builder.FindWantByID(wantID); ok {
				found = true
				break
			}
		}
	}
	// Check in global builder
	if !found && s.globalBuilder != nil {
		if _, _, ok := s.globalBuilder.FindWantByID(wantID); ok {
			found = true
		}
	}

	if !found {
		errorMsg := fmt.Sprintf("Want with ID %s not found", wantID)
		s.globalBuilder.LogAPIOperation("POST", "/api/v1/wants/{id}/start", wantID, "error", http.StatusNotFound, errorMsg, "want_not_found")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": errorMsg})
		return
	}

	// Queue start operation
	if err := s.globalBuilder.QueueWantStart([]string{wantID}); err != nil {
		errorMsg := fmt.Sprintf("Failed to queue start: %v", err)
		s.globalBuilder.LogAPIOperation("POST", "/api/v1/wants/{id}/start", wantID, "error", http.StatusConflict, errorMsg, "queue_full")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]string{"error": errorMsg})
		return
	}

	s.globalBuilder.LogAPIOperation("POST", "/api/v1/wants/{id}/start", wantID, "success", http.StatusAccepted, "", "Start queued")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]any{
		"message":   "Start operation queued",
		"wantId":    wantID,
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

func (s *Server) addLabelToWant(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	wantID := vars["id"]
	var labelReq struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}

	if err := json.NewDecoder(r.Body).Decode(&labelReq); err != nil {
		errorMsg := fmt.Sprintf("Invalid JSON request: %v", err)
		s.globalBuilder.LogAPIOperation("POST", "/api/v1/wants/{id}/labels", wantID, "error", http.StatusBadRequest, errorMsg, "invalid_json")
		http.Error(w, errorMsg, http.StatusBadRequest)
		return
	}
	if labelReq.Key == "" || labelReq.Value == "" {
		errorMsg := "Label key and value are required"
		s.globalBuilder.LogAPIOperation("POST", "/api/v1/wants/{id}/labels", wantID, "error", http.StatusBadRequest, errorMsg, "missing_fields")
		http.Error(w, errorMsg, http.StatusBadRequest)
		return
	}

	// Verify want exists before queuing operation
	found := false
	// Check in executions
	for _, execution := range s.wants {
		if execution.Builder != nil {
			if _, _, ok := execution.Builder.FindWantByID(wantID); ok {
				found = true
				break
			}
		}
	}
	// Check in global builder
	if !found && s.globalBuilder != nil {
		if _, _, ok := s.globalBuilder.FindWantByID(wantID); ok {
			found = true
		}
	}

	if !found {
		errorMsg := fmt.Sprintf("Want with ID %s not found", wantID)
		s.globalBuilder.LogAPIOperation("POST", "/api/v1/wants/{id}/labels", wantID, "error", http.StatusNotFound, errorMsg, "want_not_found")
		http.Error(w, errorMsg, http.StatusNotFound)
		return
	}

	// Queue add label operation
	if err := s.globalBuilder.QueueWantAddLabel(wantID, labelReq.Key, labelReq.Value); err != nil {
		errorMsg := fmt.Sprintf("Failed to queue add label: %v", err)
		s.globalBuilder.LogAPIOperation("POST", "/api/v1/wants/{id}/labels", wantID, "error", http.StatusConflict, errorMsg, "queue_full")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]string{"error": errorMsg})
		return
	}

	s.globalBuilder.LogAPIOperation("POST", "/api/v1/wants/{id}/labels", wantID, "success", http.StatusAccepted, "", fmt.Sprintf("Add label queued %s=%s", labelReq.Key, labelReq.Value))
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]any{
		"message":   "Add label operation queued",
		"wantId":    wantID,
		"key":       labelReq.Key,
		"value":     labelReq.Value,
		"timestamp": time.Now().Format(time.RFC3339),
	})
}
func (s *Server) removeLabelFromWant(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	wantID := vars["id"]
	keyToRemove := vars["key"]

	// Verify want exists before queuing operation
	found := false
	// Check in executions
	for _, execution := range s.wants {
		if execution.Builder != nil {
			if _, _, ok := execution.Builder.FindWantByID(wantID); ok {
				found = true
				break
			}
		}
	}
	// Check in global builder
	if !found && s.globalBuilder != nil {
		if _, _, ok := s.globalBuilder.FindWantByID(wantID); ok {
			found = true
		}
	}

	if !found {
		errorMsg := fmt.Sprintf("Want with ID %s not found", wantID)
		s.globalBuilder.LogAPIOperation("DELETE", "/api/v1/wants/{id}/labels/{key}", wantID, "error", http.StatusNotFound, errorMsg, "want_not_found")
		http.Error(w, errorMsg, http.StatusNotFound)
		return
	}

	// Queue remove label operation
	if err := s.globalBuilder.QueueWantRemoveLabel(wantID, keyToRemove); err != nil {
		errorMsg := fmt.Sprintf("Failed to queue remove label: %v", err)
		s.globalBuilder.LogAPIOperation("DELETE", "/api/v1/wants/{id}/labels/{key}", wantID, "error", http.StatusConflict, errorMsg, "queue_full")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]string{"error": errorMsg})
		return
	}

	s.globalBuilder.LogAPIOperation("DELETE", "/api/v1/wants/{id}/labels/{key}", wantID, "success", http.StatusAccepted, "", fmt.Sprintf("Remove label queued %s", keyToRemove))
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]any{
		"message":   "Remove label operation queued",
		"wantId":    wantID,
		"key":       keyToRemove,
		"timestamp": time.Now().Format(time.RFC3339),
	})
}
func (s *Server) addUsingDependency(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	wantID := vars["id"]
	var usingReq struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}

	if err := json.NewDecoder(r.Body).Decode(&usingReq); err != nil {
		errorMsg := fmt.Sprintf("Invalid JSON request: %v", err)
		s.globalBuilder.LogAPIOperation("POST", "/api/v1/wants/{id}/using", wantID, "error", http.StatusBadRequest, errorMsg, "invalid_json")
		http.Error(w, errorMsg, http.StatusBadRequest)
		return
	}
	if usingReq.Key == "" || usingReq.Value == "" {
		errorMsg := "Using dependency key and value are required"
		s.globalBuilder.LogAPIOperation("POST", "/api/v1/wants/{id}/using", wantID, "error", http.StatusBadRequest, errorMsg, "missing_fields")
		http.Error(w, errorMsg, http.StatusBadRequest)
		return
	}

	// Search for the want by metadata.id across all executions
	var targetExecution *WantExecution
	var targetWant *mywant.Want
	var targetWantIndex int = -1
	var found bool

	for _, execution := range s.wants {
		if execution.Builder != nil {
			if want, _, foundInExecution := execution.Builder.FindWantByID(wantID); foundInExecution {
				targetWant = want
				targetExecution = execution
				found = true
				for i, cfgWant := range execution.Config.Wants {
					if cfgWant.Metadata.ID == wantID {
						targetWantIndex = i
						break
					}
				}
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
		errorMsg := fmt.Sprintf("Want with ID %s not found", wantID)
		s.globalBuilder.LogAPIOperation("POST", "/api/v1/wants/{id}/using", wantID, "error", http.StatusNotFound, errorMsg, "want_not_found")
		http.Error(w, errorMsg, http.StatusNotFound)
		return
	}
	if targetWant.Spec.Using == nil {
		targetWant.Spec.Using = make([]map[string]string, 0)
	}
	newDependency := map[string]string{usingReq.Key: usingReq.Value}
	targetWant.Spec.Using = append(targetWant.Spec.Using, newDependency)

	// Also update the config if found
	if targetExecution != nil && targetWantIndex >= 0 && targetWantIndex < len(targetExecution.Config.Wants) {
		targetExecution.Config.Wants[targetWantIndex].Spec.Using = targetWant.Spec.Using
	}

	// Update the metadata timestamp
	targetWant.Metadata.UpdatedAt = time.Now().Unix()
	s.globalBuilder.LogAPIOperation("POST", "/api/v1/wants/{id}/using", wantID, "success", http.StatusOK, "", fmt.Sprintf("Added dependency %s=%s", usingReq.Key, usingReq.Value))
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]any{
		"message":   "Using dependency added successfully",
		"wantId":    wantID,
		"key":       usingReq.Key,
		"value":     usingReq.Value,
		"timestamp": time.Now().Format(time.RFC3339),
	})
}
func (s *Server) removeUsingDependency(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	wantID := vars["id"]
	keyToRemove := vars["key"]

	// Search for the want by metadata.id across all executions
	var targetExecution *WantExecution
	var targetWant *mywant.Want
	var targetWantIndex int = -1
	var found bool

	for _, execution := range s.wants {
		if execution.Builder != nil {
			if want, _, foundInExecution := execution.Builder.FindWantByID(wantID); foundInExecution {
				targetWant = want
				targetExecution = execution
				found = true
				for i, cfgWant := range execution.Config.Wants {
					if cfgWant.Metadata.ID == wantID {
						targetWantIndex = i
						break
					}
				}
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
		errorMsg := fmt.Sprintf("Want with ID %s not found", wantID)
		s.globalBuilder.LogAPIOperation("DELETE", "/api/v1/wants/{id}/using/{key}", wantID, "error", http.StatusNotFound, errorMsg, "want_not_found")
		http.Error(w, errorMsg, http.StatusNotFound)
		return
	}
	if targetWant.Spec.Using != nil {
		newUsing := make([]map[string]string, 0)
		for _, dep := range targetWant.Spec.Using {
			if _, hasKey := dep[keyToRemove]; !hasKey {
				newUsing = append(newUsing, dep)
			}
		}
		targetWant.Spec.Using = newUsing
	}

	// Also update the config if found
	if targetExecution != nil && targetWantIndex >= 0 && targetWantIndex < len(targetExecution.Config.Wants) {
		targetExecution.Config.Wants[targetWantIndex].Spec.Using = targetWant.Spec.Using
	}

	// Update the metadata timestamp
	targetWant.Metadata.UpdatedAt = time.Now().Unix()
	s.globalBuilder.LogAPIOperation("DELETE", "/api/v1/wants/{id}/using/{key}", wantID, "success", http.StatusOK, "", fmt.Sprintf("Removed dependency %s", keyToRemove))
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]any{
		"message":   "Using dependency removed successfully",
		"wantId":    wantID,
		"key":       keyToRemove,
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

// Start starts the HTTP server
func (s *Server) Start() error {
	s.setupRoutes()

	// Start pprof profiling server only in debug mode
	if s.config.Debug {
		go func() {
			pprofAddr := "localhost:6060"
			log.Printf("📊 pprof profiling server starting on http://%s/debug/pprof/\n", pprofAddr)
			if err := http.ListenAndServe(pprofAddr, nil); err != nil {
				log.Printf("⚠️  pprof server error: %v\n", err)
			}
		}()
	}

	// Register all want types on global builder before starting reconcile loop Note: Registration order no longer matters - OwnerAware wrapping happens automatically at creation time
	types.RegisterQNetWantTypes(s.globalBuilder)
	types.RegisterFibonacciWantTypes(s.globalBuilder)
	types.RegisterTravelWantTypesWithAgents(s.globalBuilder, s.agentRegistry)
	types.RegisterPrimeWantTypes(s.globalBuilder)
	types.RegisterApprovalWantTypes(s.globalBuilder)
	types.RegisterExecutionResultWantType(s.globalBuilder)
	types.RegisterReminderWantType(s.globalBuilder)
	types.RegisterSilencerWantType(s.globalBuilder)
	types.RegisterGmailWantType(s.globalBuilder)
	types.RegisterKnowledgeWantType(s.globalBuilder)
	types.RegisterFlightMockServerWantType(s.globalBuilder)
	types.RegisterDraftWantType(s.globalBuilder)
	mywant.RegisterMonitorWantTypes(s.globalBuilder)
	mywant.RegisterOwnerWantTypes(s.globalBuilder)
	mywant.RegisterSchedulerWantTypes(s.globalBuilder)

	// Transfer loaded want type definitions to global builder for state initialization
	// This ensures that SetWantTypeDefinition can be called during want creation
	if s.wantTypeLoader != nil {
		allDefs := s.wantTypeLoader.GetAll()
		for _, def := range allDefs {
			// Store definition in builder's wantTypeDefinitions map
			// This populates the definitions without re-registering factories
			s.globalBuilder.StoreWantTypeDefinition(def)
		}
	}

	// Start global builder's reconcile loop for server mode (runs indefinitely)
	go s.globalBuilder.ExecuteWithMode(true)

	addr := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)

	log.Printf("🚀 MyWant server starting on %s\n", addr)
	log.Printf("📋 Available endpoints:\n")
	log.Printf("  GET  /health                        - Health check\n")
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
	log.Printf("  DELETE /api/v1/wants/{id}/labels/{key} - Remove label from want\n")
	log.Printf("  POST /api/v1/wants/{id}/using      - Add using dependency to want\n")
	log.Printf("  DELETE /api/v1/wants/{id}/using/{key} - Remove using dependency from want\n")
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
	log.Printf("  GET  /api/v1/logs                - List API operation logs (POST/PUT/DELETE only)\n")
	log.Printf("  DELETE /api/v1/logs              - Clear all API operation logs\n")
	log.Printf("\n")

	return http.ListenAndServe(addr, s.router)
}
func (s *Server) validateWantTypes(config mywant.Config) error {
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
		testWant := &mywant.Want{
			Metadata: mywant.Metadata{
				Name: want.Metadata.Name,
				Type: wantType,
			},
			Spec: mywant.WantSpec{
				Params: make(map[string]any),
			},
		}

		// Try to create the want function to check if type is valid This uses the globalBuilder which has all registries synchronized
		_, err := s.globalBuilder.TestCreateWantFunction(testWant)
		if err != nil {
			return fmt.Errorf("invalid want type '%s' in want '%s': %v", wantType, want.Metadata.Name, err)
		}
	}

	return nil
}
func (s *Server) validateWantSpec(config mywant.Config) error {
	for _, want := range config.Wants {
		for i, selector := range want.Spec.Using {
			for key := range selector {
				if key == "" {
					return fmt.Errorf("want '%s': using[%d] has empty selector key, all selector keys must be non-empty", want.Metadata.Name, i)
				}
			}
		}
		for key := range want.Metadata.Labels {
			if key == "" {
				return fmt.Errorf("want '%s': labels has empty key, all label keys must be non-empty", want.Metadata.Name)
			}
		}
	}
	return nil
}

// collectFatalErrors performs all fatal validation checks
func (s *Server) collectFatalErrors(config *mywant.Config, result *ValidationResult) {
	for _, want := range config.Wants {
		wantName := want.Metadata.Name
		if wantName == "" {
			wantName = want.Metadata.Type
		}

		// Check 1: Want type exists (if not using recipe)
		if want.Spec.Recipe == "" {
			if err := s.validateWantType(want); err != nil {
				result.Valid = false
				result.FatalErrors = append(result.FatalErrors, ValidationError{
					WantName:  wantName,
					ErrorType: "want_type",
					Field:     "metadata.type",
					Message:   fmt.Sprintf("Want type '%s' does not exist", want.Metadata.Type),
					Details:   err.Error(),
				})
			}
		}

		// Check 2: Recipe file exists (if using recipe)
		if want.Spec.Recipe != "" {
			if err := s.validateRecipeExists(want.Spec.Recipe); err != nil {
				result.Valid = false
				result.FatalErrors = append(result.FatalErrors, ValidationError{
					WantName:  wantName,
					ErrorType: "recipe",
					Field:     "spec.recipe",
					Message:   fmt.Sprintf("Recipe file '%s' does not exist", want.Spec.Recipe),
					Details:   err.Error(),
				})
			}
		}

		// Check 3: Empty selector/label keys
		if err := s.validateSelectors(want); err != nil {
			result.Valid = false
			result.FatalErrors = append(result.FatalErrors, ValidationError{
				WantName:  wantName,
				ErrorType: "selector",
				Field:     "spec.using or metadata.labels",
				Message:   "Empty keys in selectors or labels",
				Details:   err.Error(),
			})
		}

		// Check 4: Required parameters missing (skip for recipe wants)
		if want.Spec.Recipe == "" {
			if errs := s.validateRequiredParameters(want); len(errs) > 0 {
				result.Valid = false
				for _, err := range errs {
					result.FatalErrors = append(result.FatalErrors, err)
				}
			}
		}
	}
}

// validateWantType checks if want type exists
func (s *Server) validateWantType(want *mywant.Want) error {
	wantType := want.Metadata.Type

	if wantType == "" {
		return fmt.Errorf("want type is empty")
	}

	testWant := &mywant.Want{
		Metadata: mywant.Metadata{
			Name: want.Metadata.Name,
			Type: wantType,
		},
		Spec: mywant.WantSpec{
			Params: make(map[string]any),
		},
	}

	_, err := s.globalBuilder.TestCreateWantFunction(testWant)
	return err
}

// validateRecipeExists checks if recipe file exists
func (s *Server) validateRecipeExists(recipePath string) error {
	// Try with recipes/ prefix if not absolute
	fullPath := recipePath
	if !strings.HasPrefix(recipePath, "/") {
		fullPath = fmt.Sprintf("%s/%s", mywant.RecipesDir, recipePath)
	}

	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		return fmt.Errorf("recipe file not found: %s", fullPath)
	}

	return nil
}

// validateSelectors checks for empty keys
func (s *Server) validateSelectors(want *mywant.Want) error {
	// Check using selectors
	for i, selector := range want.Spec.Using {
		for key := range selector {
			if key == "" {
				return fmt.Errorf("using[%d] has empty selector key", i)
			}
		}
	}

	// Check labels
	for key := range want.Metadata.Labels {
		if key == "" {
			return fmt.Errorf("labels has empty key")
		}
	}

	return nil
}

// validateRequiredParameters checks if all required parameters are provided
func (s *Server) validateRequiredParameters(want *mywant.Want) []ValidationError {
	errors := make([]ValidationError, 0)

	// Get want type definition
	if s.wantTypeLoader == nil {
		return errors
	}

	typeDef := s.wantTypeLoader.GetDefinition(want.Metadata.Type)
	if typeDef == nil {
		return errors
	}

	// Check each required parameter
	for _, paramDef := range typeDef.Parameters {
		if paramDef.Required {
			if _, exists := want.Spec.Params[paramDef.Name]; !exists {
				errors = append(errors, ValidationError{
					WantName:  want.Metadata.Name,
					ErrorType: "parameter",
					Field:     fmt.Sprintf("spec.params.%s", paramDef.Name),
					Message:   fmt.Sprintf("Required parameter '%s' is missing", paramDef.Name),
					Details:   paramDef.Description,
				})
			}
		}
	}

	return errors
}

// collectWarnings performs all non-fatal validation checks
func (s *Server) collectWarnings(config *mywant.Config, result *ValidationResult) {
	for _, want := range config.Wants {
		wantName := want.Metadata.Name
		if wantName == "" {
			wantName = want.Metadata.Type
		}

		// Warning 1: Using dependencies not satisfied
		if warnings := s.checkDependencySatisfaction(want); len(warnings) > 0 {
			result.Warnings = append(result.Warnings, warnings...)
		}

		// Warning 2: Connectivity requirements not met
		if warnings := s.checkConnectivityRequirements(want); len(warnings) > 0 {
			result.Warnings = append(result.Warnings, warnings...)
		}

		// Warning 3: Parameter type mismatches (skip for recipe wants)
		if want.Spec.Recipe == "" {
			if warnings := s.checkParameterTypes(want); len(warnings) > 0 {
				result.Warnings = append(result.Warnings, warnings...)
			}
		}
	}
}

// checkDependencySatisfaction checks if using dependencies are satisfied
func (s *Server) checkDependencySatisfaction(want *mywant.Want) []ValidationWarning {
	warnings := make([]ValidationWarning, 0)

	if len(want.Spec.Using) == 0 {
		return warnings
	}

	// Get all currently deployed wants
	deployedWants := s.globalBuilder.GetAllWantStates()

	for i, selector := range want.Spec.Using {
		matched := false

		for _, deployed := range deployedWants {
			if s.matchesSelector(deployed.Metadata.Labels, selector) {
				matched = true
				break
			}
		}

		if !matched {
			selectorStr := fmt.Sprintf("%v", selector)
			warnings = append(warnings, ValidationWarning{
				WantName:    want.Metadata.Name,
				WarningType: "dependency",
				Field:       fmt.Sprintf("spec.using[%d]", i),
				Message:     fmt.Sprintf("No deployed wants match selector: %s", selectorStr),
				Suggestion:  "Deploy wants with matching labels before deploying this want",
			})
		}
	}

	return warnings
}

// matchesSelector checks if labels match selector
func (s *Server) matchesSelector(labels map[string]string, selector map[string]string) bool {
	for key, value := range selector {
		if labels[key] != value {
			return false
		}
	}
	return true
}

// checkConnectivityRequirements checks if connectivity requirements are met
func (s *Server) checkConnectivityRequirements(want *mywant.Want) []ValidationWarning {
	warnings := make([]ValidationWarning, 0)

	// Get want type definition for connectivity requirements
	if s.wantTypeLoader == nil {
		return warnings
	}

	typeDef := s.wantTypeLoader.GetDefinition(want.Metadata.Type)
	if typeDef == nil || typeDef.Require == nil {
		return warnings
	}

	// Convert RequireSpec to ConnectivityMetadata
	connMeta := typeDef.Require.ToConnectivityMetadata(want.Metadata.Type)

	// Simulate path generation to check connectivity
	deployedWants := s.globalBuilder.GetAllWantStates()

	// Count potential inputs (wants that this want uses via using selectors)
	inputCount := 0
	for _, selector := range want.Spec.Using {
		for _, deployed := range deployedWants {
			if s.matchesSelector(deployed.Metadata.Labels, selector) {
				inputCount++
				break
			}
		}
	}

	// Count potential outputs (wants that use this want's labels)
	outputCount := 0
	for _, deployed := range deployedWants {
		for _, deployedSelector := range deployed.Spec.Using {
			if s.matchesSelector(want.Metadata.Labels, deployedSelector) {
				outputCount++
				break
			}
		}
	}

	// Check connectivity requirements
	if connMeta.RequiredInputs > 0 && inputCount < connMeta.RequiredInputs {
		warnings = append(warnings, ValidationWarning{
			WantName:    want.Metadata.Name,
			WarningType: "connectivity",
			Field:       "spec.using",
			Message:     fmt.Sprintf("Requires %d input connection(s), but may only have %d", connMeta.RequiredInputs, inputCount),
			Suggestion:  "Ensure upstream wants with matching labels are deployed",
		})
	}

	if connMeta.RequiredOutputs > 0 && outputCount < connMeta.RequiredOutputs {
		warnings = append(warnings, ValidationWarning{
			WantName:    want.Metadata.Name,
			WarningType: "connectivity",
			Field:       "metadata.labels",
			Message:     fmt.Sprintf("Requires %d output connection(s), but may only have %d", connMeta.RequiredOutputs, outputCount),
			Suggestion:  "Ensure downstream wants that depend on this want are deployed",
		})
	}

	return warnings
}

// checkParameterTypes checks for parameter type mismatches
func (s *Server) checkParameterTypes(want *mywant.Want) []ValidationWarning {
	warnings := make([]ValidationWarning, 0)

	if s.wantTypeLoader == nil {
		return warnings
	}

	typeDef := s.wantTypeLoader.GetDefinition(want.Metadata.Type)
	if typeDef == nil {
		return warnings
	}

	// Build parameter definition map
	paramDefs := make(map[string]*mywant.ParameterDef)
	for i := range typeDef.Parameters {
		paramDefs[typeDef.Parameters[i].Name] = &typeDef.Parameters[i]
	}

	// Check each provided parameter
	for paramName, paramValue := range want.Spec.Params {
		paramDef, exists := paramDefs[paramName]
		if !exists {
			continue
		}

		// Simple type checking
		expectedType := paramDef.Type
		actualType := s.getGoType(paramValue)

		if !s.isTypeCompatible(expectedType, actualType) {
			warnings = append(warnings, ValidationWarning{
				WantName:    want.Metadata.Name,
				WarningType: "parameter_type",
				Field:       fmt.Sprintf("spec.params.%s", paramName),
				Message:     fmt.Sprintf("Parameter type mismatch: expected %s, got %s", expectedType, actualType),
				Suggestion:  fmt.Sprintf("Convert value to %s type", expectedType),
			})
		}
	}

	return warnings
}

// getGoType returns the Go type name of a value
func (s *Server) getGoType(value any) string {
	if value == nil {
		return "nil"
	}

	switch value.(type) {
	case bool:
		return "bool"
	case float64:
		return "float64"
	case string:
		return "string"
	case []any:
		return "[]any"
	case map[string]any:
		return "map[string]any"
	default:
		return fmt.Sprintf("%T", value)
	}
}

// isTypeCompatible checks if types are compatible
func (s *Server) isTypeCompatible(expected, actual string) bool {
	if expected == actual {
		return true
	}

	// Allow numeric conversions
	numericTypes := map[string]bool{"int": true, "int64": true, "float64": true}
	if numericTypes[expected] && numericTypes[actual] {
		return true
	}

	// JSON unmarshals numbers as float64
	if expected == "int" && actual == "float64" {
		return true
	}

	return false
}

// ======= ERROR HISTORY HANDLERS =======

// logError adds an error to the error history
func (s *Server) logError(r *http.Request, status int, message, errorType, details string, requestData any) {
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

// listErrorHistory handles GET /api/v1/errors - lists all error history entries
func (s *Server) listErrorHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Sort errors by timestamp (newest first)
	sortedErrors := make([]ErrorHistoryEntry, len(s.errorHistory))
	copy(sortedErrors, s.errorHistory)
	sort.Slice(sortedErrors, func(i, j int) bool {
		return sortedErrors[i].Timestamp > sortedErrors[j].Timestamp
	})

	response := map[string]any{
		"errors": sortedErrors,
		"total":  len(sortedErrors),
	}

	json.NewEncoder(w).Encode(response)
}
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
		errorMsg := fmt.Sprintf("Invalid JSON: %v", err)
		s.globalBuilder.LogAPIOperation("PUT", "/api/v1/errors/{id}", errorID, "error", http.StatusBadRequest, errorMsg, "invalid_json")
		http.Error(w, errorMsg, http.StatusBadRequest)
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
			s.globalBuilder.LogAPIOperation("PUT", "/api/v1/errors/{id}", errorID, "success", http.StatusOK, "", "Error entry updated")
			json.NewEncoder(w).Encode(s.errorHistory[i])
			return
		}
	}

	errorMsg := "Error entry not found"
	s.globalBuilder.LogAPIOperation("PUT", "/api/v1/errors/{id}", errorID, "error", http.StatusNotFound, errorMsg, "error_not_found")
	http.Error(w, errorMsg, http.StatusNotFound)
}

// deleteErrorHistoryEntry handles DELETE /api/v1/errors/{id} - deletes an error entry
func (s *Server) deleteErrorHistoryEntry(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	errorID := vars["id"]

	for i, entry := range s.errorHistory {
		if entry.ID == errorID {
			s.errorHistory = append(s.errorHistory[:i], s.errorHistory[i+1:]...)
			s.globalBuilder.LogAPIOperation("DELETE", "/api/v1/errors/{id}", errorID, "success", http.StatusNoContent, "", "Error entry deleted")
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}

	errorMsg := "Error entry not found"
	s.globalBuilder.LogAPIOperation("DELETE", "/api/v1/errors/{id}", errorID, "error", http.StatusNotFound, errorMsg, "error_not_found")
	http.Error(w, errorMsg, http.StatusNotFound)
}

// getLogs returns all API operation logs
func (s *Server) getLogs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Get logs from global builder
	var logs []mywant.APILogEntry
	if s.globalBuilder != nil {
		logs = s.globalBuilder.GetAPILogs()
	}

	response := map[string]any{
		"timestamp": time.Now().Format(time.RFC3339),
		"count":     len(logs),
		"logs":      logs,
	}

	json.NewEncoder(w).Encode(response)
}

// clearLogs clears all API operation logs
func (s *Server) clearLogs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if s.globalBuilder != nil {
		s.globalBuilder.ClearAPILogs()
	}

	response := map[string]any{
		"timestamp": time.Now().Format(time.RFC3339),
		"message":   "All API logs cleared",
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
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
	uuid[6] = (uuid[6] & 0x0f) | 0x40 // Version 4
	uuid[8] = (uuid[8] & 0x3f) | 0x80 // Variant
	return fmt.Sprintf("want-%x-%x-%x-%x-%x",
		uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16])
}

// Recipe API handlers
func (s *Server) createRecipe(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var recipe mywant.GenericRecipe
	if err := json.NewDecoder(r.Body).Decode(&recipe); err != nil {
		s.logError(r, http.StatusBadRequest, "Invalid recipe format", "recipe_creation", err.Error(), "")
		s.globalBuilder.LogAPIOperation("POST", "/api/v1/recipes", "", "error", http.StatusBadRequest, "Invalid recipe format", "parsing_error")
		http.Error(w, "Invalid recipe format", http.StatusBadRequest)
		return
	}

	// Use recipe name as ID if no custom ID provided
	recipeID := recipe.Recipe.Metadata.Name
	if recipeID == "" {
		s.logError(r, http.StatusBadRequest, "Recipe name is required", "recipe_creation", "missing name", "")
		s.globalBuilder.LogAPIOperation("POST", "/api/v1/recipes", "", "error", http.StatusBadRequest, "Recipe name is required", "missing_name")
		http.Error(w, "Recipe name is required", http.StatusBadRequest)
		return
	}

	if err := s.recipeRegistry.CreateRecipe(recipeID, &recipe); err != nil {
		s.logError(r, http.StatusConflict, err.Error(), "recipe_creation", "duplicate recipe", recipeID)
		s.globalBuilder.LogAPIOperation("POST", "/api/v1/recipes", recipeID, "error", http.StatusConflict, err.Error(), "duplicate_recipe")
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	s.globalBuilder.LogAPIOperation("POST", "/api/v1/recipes", recipeID, "success", http.StatusCreated, "", "Recipe created successfully")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{
		"id":      recipeID,
		"message": "Recipe created successfully",
	})
}

// saveRecipeFromWant handles POST /api/v1/recipes/from-want
func (s *Server) saveRecipeFromWant(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var req SaveRecipeFromWantRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.globalBuilder.LogAPIOperation("POST", "/api/v1/recipes/from-want", "", "error", http.StatusBadRequest, "Invalid request format", "parsing_error")
		http.Error(w, "Invalid request format", http.StatusBadRequest)
		return
	}

	if req.WantID == "" {
		s.globalBuilder.LogAPIOperation("POST", "/api/v1/recipes/from-want", "", "error", http.StatusBadRequest, "wantId is required", "missing_id")
		http.Error(w, "wantId is required", http.StatusBadRequest)
		return
	}

	// Find the parent want
	var parentWant *mywant.Want
	var builder *mywant.ChainBuilder

	// Search across all executions
	for _, execution := range s.wants {
		if execution.Builder != nil {
			if wnt, _, found := execution.Builder.FindWantByID(req.WantID); found {
				parentWant = wnt
				builder = execution.Builder
				break
			}
		}
	}

	// Also check global builder
	if parentWant == nil && s.globalBuilder != nil {
		if wnt, _, found := s.globalBuilder.FindWantByID(req.WantID); found {
			parentWant = wnt
			builder = s.globalBuilder
		}
	}

	if parentWant == nil {
		s.globalBuilder.LogAPIOperation("POST", "/api/v1/recipes/from-want", req.WantID, "error", http.StatusNotFound, "Want not found", "not_found")
		http.Error(w, "Want not found", http.StatusNotFound)
		return
	}

	// Find all child wants
	allWants := builder.GetAllWantStates()
	var childWants []mywant.RecipeWant

	for _, wnt := range allWants {
		isChild := false
		for _, ownerRef := range wnt.Metadata.OwnerReferences {
			if ownerRef.ID == req.WantID || (ownerRef.Name == parentWant.Metadata.Name && ownerRef.Kind == "Want") {
				isChild = true
				break
			}
		}

		if isChild {
			// Convert to RecipeWant and strip instance-specific IDs and references
			metadata := wnt.Metadata
			metadata.ID = ""               // Strip unique ID
			metadata.OwnerReferences = nil // Strip specific owner references (Target will recreate them)

			childWants = append(childWants, mywant.RecipeWant{
				Metadata: metadata,
				Spec:     wnt.Spec,
			})
		}
	}

	// Construct recipe
	recipe := mywant.GenericRecipe{
		Recipe: mywant.RecipeContent{
			Metadata: req.Metadata,
			Wants:    childWants,
		},
	}

	if recipe.Recipe.Metadata.Name == "" {
		recipe.Recipe.Metadata.Name = parentWant.Metadata.Name + "-recipe"
	}

	// Save to registry
	recipeID := recipe.Recipe.Metadata.Name
	if err := s.recipeRegistry.CreateRecipe(recipeID, &recipe); err != nil {
		// If it exists, update it instead
		_ = s.recipeRegistry.UpdateRecipe(recipeID, &recipe)
	}

	// Save to file in recipes/ directory
	filename := fmt.Sprintf("%s/%s.yaml", mywant.RecipesDir, recipeID)
	// Sanitize filename: replace spaces with hyphens
	filename = strings.ReplaceAll(filename, " ", "-")

	yamlData, err := yaml.Marshal(recipe)
	if err != nil {
		s.globalBuilder.LogAPIOperation("POST", "/api/v1/recipes/from-want", recipeID, "error", http.StatusInternalServerError, err.Error(), "marshaling_error")
		http.Error(w, fmt.Sprintf("Failed to marshal recipe: %v", err), http.StatusInternalServerError)
		return
	}

	if err := os.WriteFile(filename, yamlData, 0644); err != nil {
		log.Printf("[SERVER] Warning: Failed to save recipe file %s: %v\n", filename, err)
	}

	s.globalBuilder.LogAPIOperation("POST", "/api/v1/recipes/from-want", recipeID, "success", http.StatusCreated, "", fmt.Sprintf("Recipe created from want with %d children", len(childWants)))

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]any{
		"id":      recipeID,
		"message": "Recipe created successfully from want",
		"file":    filename,
		"wants":   len(childWants),
	})
}

// listRecipes handles GET /api/v1/recipes - lists all recipes
func (s *Server) listRecipes(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	recipes := s.recipeRegistry.ListRecipes()
	json.NewEncoder(w).Encode(recipes)
}
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
		s.globalBuilder.LogAPIOperation("PUT", "/api/v1/recipes/{id}", recipeID, "error", http.StatusBadRequest, "Invalid recipe format", "parsing_error")
		http.Error(w, "Invalid recipe format", http.StatusBadRequest)
		return
	}

	if err := s.recipeRegistry.UpdateRecipe(recipeID, &recipe); err != nil {
		s.logError(r, http.StatusNotFound, err.Error(), "recipe_update", "recipe not found", recipeID)
		s.globalBuilder.LogAPIOperation("PUT", "/api/v1/recipes/{id}", recipeID, "error", http.StatusNotFound, err.Error(), "recipe_not_found")
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	s.globalBuilder.LogAPIOperation("PUT", "/api/v1/recipes/{id}", recipeID, "success", http.StatusOK, "", "Recipe updated successfully")
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
		s.globalBuilder.LogAPIOperation("DELETE", "/api/v1/recipes/{id}", recipeID, "error", http.StatusNotFound, err.Error(), "recipe_not_found")
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	s.globalBuilder.LogAPIOperation("DELETE", "/api/v1/recipes/{id}", recipeID, "success", http.StatusNoContent, "", "Recipe deleted")
	w.WriteHeader(http.StatusNoContent)
}

// loadRecipeFilesIntoRegistry loads recipe YAML files into the recipe registry for the API
func loadRecipeFilesIntoRegistry(recipeDir string, registry *mywant.CustomTargetTypeRegistry) error {
	if _, err := os.Stat(recipeDir); os.IsNotExist(err) {
		log.Printf("[SERVER] Recipe directory '%s' does not exist, skipping recipe loading\n", recipeDir)
		return nil
	}
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

	json.NewEncoder(w).Encode(map[string]any{
		"wantTypes": items,
		"count":     len(items),
	})
}
func (s *Server) getLabels(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Collect all unique label keys and their values from all wants Also track which wants use each label
	labelKeys := make(map[string]bool)
	labelValues := make(map[string]map[string]bool)             // key -> (value -> true)
	labelToWants := make(map[string]map[string]map[string]bool) // key -> value -> (wantID -> true)

	// Start with globally registered labels (added via POST /api/v1/labels) These labels don't have owners - they're just registered labels
	if s.globalLabels != nil {
		for key, valueMap := range s.globalLabels {
			labelKeys[key] = true
			if labelValues[key] == nil {
				labelValues[key] = make(map[string]bool)
				labelToWants[key] = make(map[string]map[string]bool)
			}
			for value := range valueMap {
				labelValues[key][value] = true
				if labelToWants[key][value] == nil {
					labelToWants[key][value] = make(map[string]bool)
				}
			}
		}
	}

	// Collect from wants in executions
	for _, execution := range s.wants {
		if execution.Builder != nil {
			currentStates := execution.Builder.GetAllWantStates()
			for _, want := range currentStates {
				// Skip internal wants and label-holder wants
				if want.Metadata.Name != "" && (strings.HasPrefix(want.Metadata.Name, "__") || strings.HasPrefix(want.Metadata.Name, "_label-")) {
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
			// Skip internal wants and label-holder wants
			if want.Metadata.Name != "" && (strings.HasPrefix(want.Metadata.Name, "__") || strings.HasPrefix(want.Metadata.Name, "_label-")) {
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
	for _, execution := range s.wants {
		if execution.Builder != nil {
			currentStates := execution.Builder.GetAllWantStates()
			for _, want := range currentStates {
				// Skip internal wants
				if want.Metadata.Name != "" && strings.HasPrefix(want.Metadata.Name, "__") {
					continue
				}
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
	if s.globalBuilder != nil {
		currentStates := s.globalBuilder.GetAllWantStates()
		for _, want := range currentStates {
			// Skip internal wants
			if want.Metadata.Name != "" && strings.HasPrefix(want.Metadata.Name, "__") {
				continue
			}
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
	keys := make([]string, 0, len(labelKeys))
	for key := range labelKeys {
		keys = append(keys, key)
	}
	sort.Strings(keys)
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
	json.NewEncoder(w).Encode(map[string]any{
		"labelKeys":   keys,
		"labelValues": values,
		"count":       len(keys),
	})
}
func (s *Server) addLabel(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var labelReq struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}

	if err := json.NewDecoder(r.Body).Decode(&labelReq); err != nil {
		errorMsg := fmt.Sprintf("Invalid request body: %v", err)
		s.globalBuilder.LogAPIOperation("POST", "/api/v1/labels", "", "error", http.StatusBadRequest, errorMsg, "invalid_json")
		http.Error(w, errorMsg, http.StatusBadRequest)
		return
	}
	labelReq.Key = strings.TrimSpace(labelReq.Key)
	labelReq.Value = strings.TrimSpace(labelReq.Value)

	if labelReq.Key == "" || labelReq.Value == "" {
		errorMsg := "Label key and value must not be empty"
		s.globalBuilder.LogAPIOperation("POST", "/api/v1/labels", "", "error", http.StatusBadRequest, errorMsg, "missing_fields")
		http.Error(w, errorMsg, http.StatusBadRequest)
		return
	}
	if s.globalLabels == nil {
		s.globalLabels = make(map[string]map[string]bool) // key -> value -> true
	}
	if s.globalLabels[labelReq.Key] == nil {
		s.globalLabels[labelReq.Key] = make(map[string]bool)
	}
	s.globalLabels[labelReq.Key][labelReq.Value] = true
	s.globalBuilder.LogAPIOperation("POST", "/api/v1/labels", fmt.Sprintf("%s=%s", labelReq.Key, labelReq.Value), "success", http.StatusCreated, "", "Label registered successfully")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]any{
		"key":     labelReq.Key,
		"value":   labelReq.Value,
		"message": "Label registered successfully",
	})
}
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

	json.NewEncoder(w).Encode(map[string]any{
		"name":     name,
		"examples": def.Examples,
	})
}

// ReactionRequest represents a user reaction to a reminder
type ReactionRequest struct {
	Approved bool   `json:"approved"`
	Comment  string `json:"comment,omitempty"`
}

// createReactionQueue handles POST /api/v1/reactions/ - Create new reaction queue
func (s *Server) createReactionQueue(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Create new queue
	queueID, err := s.reactionQueueManager.CreateQueue()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create queue: %v", err), http.StatusInternalServerError)
		s.globalBuilder.LogAPIOperation("POST", r.URL.Path, "", "error", http.StatusInternalServerError,
			fmt.Sprintf("Failed to create queue: %v", err), "queue_creation_failed")
		return
	}

	// Return success response
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]any{
		"queue_id":   queueID,
		"created_at": time.Now().Format(time.RFC3339),
	})

	s.globalBuilder.LogAPIOperation("POST", r.URL.Path, "", "success", http.StatusCreated,
		fmt.Sprintf("Reaction queue created: %s", queueID), "queue_created")
}

// listReactionQueues handles GET /api/v1/reactions/ - List all reaction queues
func (s *Server) listReactionQueues(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	queues := s.reactionQueueManager.ListQueues()

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]any{
		"count":  len(queues),
		"queues": queues,
	})

	s.globalBuilder.LogAPIOperation("GET", r.URL.Path, "", "success", http.StatusOK,
		fmt.Sprintf("Listed %d reaction queues", len(queues)), "queues_listed")
}

// getReactionQueue handles GET /api/v1/reactions/{id} - Read from queue (non-destructive)
func (s *Server) getReactionQueue(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Extract queue ID from URL
	vars := mux.Vars(r)
	queueID := vars["id"]

	InfoLog("[API:REACTION] GET request received for queue ID: %s\n", queueID)

	if queueID == "" {
		InfoLog("[API:REACTION] ERROR: Missing queue ID\n")
		http.Error(w, "Missing queue ID", http.StatusBadRequest)
		return
	}

	// Get queue
	queue, err := s.reactionQueueManager.GetQueue(queueID)
	if err != nil {
		InfoLog("[API:REACTION] ERROR: Queue not found: %s\n", queueID)
		http.Error(w, err.Error(), http.StatusNotFound)
		s.globalBuilder.LogAPIOperation("GET", r.URL.Path, "", "error", http.StatusNotFound,
			fmt.Sprintf("Queue not found: %s", queueID), "queue_not_found")
		return
	}

	// Get all reactions from queue (non-destructive)
	reactions := queue.GetReactions()
	InfoLog("[API:REACTION] Returning %d reactions for queue %s\n", len(reactions), queueID)

	// Return success response
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]any{
		"queue_id":   queueID,
		"reactions":  reactions,
		"created_at": queue.CreatedAt.Format(time.RFC3339),
	})

	s.globalBuilder.LogAPIOperation("GET", r.URL.Path, "", "success", http.StatusOK,
		fmt.Sprintf("Retrieved queue %s with %d reactions", queueID, len(reactions)), "queue_retrieved")
}

// addReactionToQueue handles PUT /api/v1/reactions/{id} - Add reaction to queue
func (s *Server) addReactionToQueue(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Extract queue ID from URL
	vars := mux.Vars(r)
	queueID := vars["id"]

	InfoLog("[API:REACTION] PUT request received for queue ID: %s\n", queueID)

	if queueID == "" {
		InfoLog("[API:REACTION] ERROR: Missing queue ID\n")
		http.Error(w, "Missing queue ID", http.StatusBadRequest)
		return
	}

	// Parse request body
	var req ReactionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		InfoLog("[API:REACTION] ERROR: Failed to decode request: %v\n", err)
		http.Error(w, "Invalid request format", http.StatusBadRequest)
		return
	}

	InfoLog("[API:REACTION] Parsed request - Approved: %v, Comment: %s\n", req.Approved, req.Comment)

	// Add reaction to queue
	reactionID, err := s.reactionQueueManager.AddReactionToQueue(queueID, req.Approved, req.Comment)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		s.globalBuilder.LogAPIOperation("PUT", r.URL.Path, "", "error", http.StatusNotFound,
			fmt.Sprintf("Failed to add reaction: %v", err), "reaction_add_failed")
		return
	}

	// Return success response
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]any{
		"queue_id":    queueID,
		"reaction_id": reactionID,
		"timestamp":   time.Now().Format(time.RFC3339),
	})

	s.globalBuilder.LogAPIOperation("PUT", r.URL.Path, "", "success", http.StatusOK,
		fmt.Sprintf("Reaction added to queue %s: %s", queueID, reactionID), "reaction_added")
}

// deleteReactionQueue handles DELETE /api/v1/reactions/{id} - Delete queue
func (s *Server) deleteReactionQueue(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Extract queue ID from URL
	vars := mux.Vars(r)
	queueID := vars["id"]

	if queueID == "" {
		http.Error(w, "Missing queue ID", http.StatusBadRequest)
		return
	}

	// Delete queue
	err := s.reactionQueueManager.DeleteQueue(queueID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		s.globalBuilder.LogAPIOperation("DELETE", r.URL.Path, "", "error", http.StatusNotFound,
			fmt.Sprintf("Failed to delete queue: %v", err), "queue_deletion_failed")
		return
	}

	// Return success response
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]any{
		"queue_id": queueID,
		"deleted":  true,
	})

	s.globalBuilder.LogAPIOperation("DELETE", r.URL.Path, "", "success", http.StatusOK,
		fmt.Sprintf("Reaction queue deleted: %s", queueID), "queue_deleted")
}

// ========= Interactive Want Creation Handlers =========

// interactCreate creates a new interactive session
func (s *Server) interactCreate(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Create new session
	session := s.interactionManager.CreateSession()

	// Build response
	response := InteractCreateResponse{
		SessionID: session.SessionID,
		CreatedAt: session.CreatedAt,
		ExpiresAt: session.ExpiresAt,
	}

	s.globalBuilder.LogAPIOperation("POST", "/api/v1/interact", session.SessionID, "success", http.StatusCreated,
		fmt.Sprintf("Session created: %s", session.SessionID), "session_created")

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

// interactMessage sends a message to an interactive session and returns recommendations
func (s *Server) interactMessage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Extract session ID from URL
	vars := mux.Vars(r)
	sessionID := vars["id"]

	if sessionID == "" {
		http.Error(w, "Missing session ID", http.StatusBadRequest)
		s.globalBuilder.LogAPIOperation("POST", r.URL.Path, "", "error", http.StatusBadRequest,
			"Missing session ID", "missing_session_id")
		return
	}

	// Parse request body
	var req InteractMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request format", http.StatusBadRequest)
		s.globalBuilder.LogAPIOperation("POST", r.URL.Path, sessionID, "error", http.StatusBadRequest,
			fmt.Sprintf("Invalid request: %v", err), "invalid_request")
		return
	}

	if req.Message == "" {
		http.Error(w, "Message is required", http.StatusBadRequest)
		s.globalBuilder.LogAPIOperation("POST", r.URL.Path, sessionID, "error", http.StatusBadRequest,
			"Empty message", "empty_message")
		return
	}

	// Send message and get recommendations
	session, err := s.interactionManager.SendMessage(r.Context(), sessionID, req.Message, req.Context)
	if err != nil {
		statusCode := http.StatusInternalServerError
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "expired") {
			statusCode = http.StatusNotFound
		}
		http.Error(w, err.Error(), statusCode)
		s.globalBuilder.LogAPIOperation("POST", r.URL.Path, sessionID, "error", statusCode,
			fmt.Sprintf("Failed to process message: %v", err), "message_processing_failed")
		return
	}

	// Build response
	response := InteractMessageResponse{
		Recommendations:     session.Recommendations,
		ConversationHistory: session.ConversationHistory,
		Timestamp:           time.Now(),
	}

	s.globalBuilder.LogAPIOperation("POST", r.URL.Path, sessionID, "success", http.StatusOK,
		fmt.Sprintf("Generated %d recommendations", len(session.Recommendations)), "recommendations_generated")

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// interactDelete deletes an interactive session
func (s *Server) interactDelete(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Extract session ID from URL
	vars := mux.Vars(r)
	sessionID := vars["id"]

	if sessionID == "" {
		http.Error(w, "Missing session ID", http.StatusBadRequest)
		s.globalBuilder.LogAPIOperation("DELETE", r.URL.Path, "", "error", http.StatusBadRequest,
			"Missing session ID", "missing_session_id")
		return
	}

	// Delete session
	s.interactionManager.DeleteSession(sessionID)

	s.globalBuilder.LogAPIOperation("DELETE", r.URL.Path, sessionID, "success", http.StatusNoContent,
		fmt.Sprintf("Session deleted: %s", sessionID), "session_deleted")

	w.WriteHeader(http.StatusNoContent)
}

// interactDeploy deploys a recommendation from an interactive session
func (s *Server) interactDeploy(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Extract session ID from URL
	vars := mux.Vars(r)
	sessionID := vars["id"]

	if sessionID == "" {
		http.Error(w, "Missing session ID", http.StatusBadRequest)
		s.globalBuilder.LogAPIOperation("POST", r.URL.Path, "", "error", http.StatusBadRequest,
			"Missing session ID", "missing_session_id")
		return
	}

	// Parse request body
	var req InteractDeployRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request format", http.StatusBadRequest)
		s.globalBuilder.LogAPIOperation("POST", r.URL.Path, sessionID, "error", http.StatusBadRequest,
			fmt.Sprintf("Invalid request: %v", err), "invalid_request")
		return
	}

	if req.RecommendationID == "" {
		http.Error(w, "Recommendation ID is required", http.StatusBadRequest)
		s.globalBuilder.LogAPIOperation("POST", r.URL.Path, sessionID, "error", http.StatusBadRequest,
			"Missing recommendation ID", "missing_recommendation_id")
		return
	}

	// Get recommendation from session
	recommendation, err := s.interactionManager.GetRecommendation(sessionID, req.RecommendationID)
	if err != nil {
		statusCode := http.StatusNotFound
		http.Error(w, err.Error(), statusCode)
		s.globalBuilder.LogAPIOperation("POST", r.URL.Path, sessionID, "error", statusCode,
			fmt.Sprintf("Failed to get recommendation: %v", err), "recommendation_not_found")
		return
	}

	// Apply modifications if provided
	config := recommendation.Config
	if req.Modifications != nil {
		// Apply parameter overrides
		if req.Modifications.ParameterOverrides != nil {
			for _, want := range config.Wants {
				for key, value := range req.Modifications.ParameterOverrides {
					if want.Spec.Params == nil {
						want.Spec.Params = make(map[string]interface{})
					}
					want.Spec.Params[key] = value
				}
			}
		}

		// Filter out disabled wants
		if len(req.Modifications.DisableWants) > 0 {
			disabledSet := make(map[string]bool)
			for _, name := range req.Modifications.DisableWants {
				disabledSet[name] = true
			}

			var filteredWants []*mywant.Want
			for _, want := range config.Wants {
				if !disabledSet[want.Metadata.Name] {
					filteredWants = append(filteredWants, want)
				}
			}
			config.Wants = filteredWants
		}
	}

	// Assign IDs to wants
	for _, want := range config.Wants {
		if want.Metadata.ID == "" {
			want.Metadata.ID = generateWantID()
		}
	}

	// Deploy wants using existing logic
	executionID := generateWantID()
	execution := &WantExecution{
		ID:      executionID,
		Config:  config,
		Status:  "created",
		Builder: s.globalBuilder,
	}
	s.wants[executionID] = execution

	wantIDs, err := s.globalBuilder.AddWantsAsyncWithTracking(config.Wants)
	if err != nil {
		delete(s.wants, executionID)
		errorMsg := fmt.Sprintf("Failed to deploy wants: %v", err)
		http.Error(w, errorMsg, http.StatusInternalServerError)
		s.globalBuilder.LogAPIOperation("POST", r.URL.Path, sessionID, "error", http.StatusInternalServerError,
			errorMsg, "deployment_failed")
		return
	}

	// Build response
	response := InteractDeployResponse{
		ExecutionID: executionID,
		WantIDs:     wantIDs,
		Status:      "deployed",
		Message:     fmt.Sprintf("Successfully deployed %d wants from recommendation %s", len(wantIDs), req.RecommendationID),
		Timestamp:   time.Now(),
	}

	s.globalBuilder.LogAPIOperation("POST", r.URL.Path, sessionID, "success", http.StatusOK,
		fmt.Sprintf("Deployed recommendation %s: %d wants", req.RecommendationID, len(wantIDs)), "recommendation_deployed")

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

func main() {
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
	// ./server 8080 0.0.0.0 - port 8080, 0.0.0.0, no debug ./server 8080 0.0.0.0 debug - port 8080, 0.0.0.0, debug enabled
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
	GlobalDebugEnabled = debugEnabled
	mywant.DebugLoggingEnabled = debugEnabled
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
	server := NewServer(config)

	// Initialize HTTP client for internal API calls (agents use this to communicate with reaction queue API)
	baseURL := fmt.Sprintf("http://%s:%d", host, port)
	httpClient := mywant.NewHTTPClient(baseURL)
	server.globalBuilder.SetHTTPClient(httpClient)
	InfoLog(fmt.Sprintf("📡 HTTP client initialized with base URL: %s", baseURL))

	if err := server.Start(); err != nil {
		log.Fatal("Server failed to start:", err)
	}
}
