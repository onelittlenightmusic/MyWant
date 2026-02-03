package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	types "mywant/engine/cmd/types"
	mywant "mywant/engine/src"

	"github.com/gorilla/mux"
)

// Config holds the Agent Service worker configuration
type Config struct {
	Port  int
	Host  string
	Debug bool
}

// Worker represents the Agent Service worker (stateless)
type Worker struct {
	config        Config
	agentRegistry *mywant.AgentRegistry
	router        *mux.Router
}

// New creates a new Agent Service worker
func New(config Config) *Worker {
	// Initialize agent registry
	agentRegistry := mywant.NewAgentRegistry()

	// Load capabilities from files
	if err := agentRegistry.LoadCapabilities(mywant.CapabilitiesDir + "/"); err != nil {
		log.Printf("[WORKER] Warning: Failed to load capabilities: %v\n", err)
	}

	// Register built-in capabilities
	agentRegistry.RegisterCapability(mywant.Capability{
		Name:  "mock_server_management",
		Gives: []string{"mock_server_management"},
	})

	// Load agents from YAML files
	if err := agentRegistry.LoadAgents(mywant.AgentsDir + "/"); err != nil {
		log.Printf("[WORKER] Warning: Failed to load agents: %v\n", err)
	}

	worker := &Worker{
		config:        config,
		agentRegistry: agentRegistry,
		router:        mux.NewRouter(),
	}

	// Register dynamic agent implementations (code-based agents)
	worker.registerDynamicAgents()

	// Register Agent Service routes
	worker.registerRoutes()

	return worker
}

// registerDynamicAgents registers code-based agent implementations
func (w *Worker) registerDynamicAgents() {
	// Register execution agents (command execution)
	types.RegisterExecutionAgents(w.agentRegistry)

	// Register MCP agents
	types.RegisterMCPAgent(w.agentRegistry)
	types.RegisterDynamicMCPAgents(w.agentRegistry)

	// Register reminder queue agent
	if err := types.RegisterReminderQueueAgent(w.agentRegistry); err != nil {
		log.Printf("[WORKER] Warning: Failed to register reminder queue agent: %v\n", err)
	}

	// Register mock server agent
	if err := types.RegisterMockServerAgent(w.agentRegistry); err != nil {
		log.Printf("[WORKER] Warning: Failed to register mock server agent: %v\n", err)
	}

	log.Printf("[WORKER] Registered %d agents", len(w.agentRegistry.GetAllAgents()))
}

// registerRoutes registers HTTP routes for Agent Service
func (w *Worker) registerRoutes() {
	// Agent Service endpoints (execute registered agents)
	w.router.HandleFunc("/api/v1/agent-service/execute", w.handleAgentServiceExecute).Methods("POST")
	w.router.HandleFunc("/api/v1/agent-service/monitor/execute", w.handleAgentServiceMonitorExecute).Methods("POST")

	// Health check
	w.router.HandleFunc("/health", w.handleHealth).Methods("GET")

	// Agent registry info
	w.router.HandleFunc("/api/v1/agents", w.handleListAgents).Methods("GET")
	w.router.HandleFunc("/api/v1/capabilities", w.handleListCapabilities).Methods("GET")

	log.Println("[WORKER] Registered Agent Service routes:")
	log.Println("  POST /api/v1/agent-service/execute")
	log.Println("  POST /api/v1/agent-service/monitor/execute")
	log.Println("  GET  /health")
	log.Println("  GET  /api/v1/agents")
	log.Println("  GET  /api/v1/capabilities")
}

// Start starts the Agent Service worker
func (w *Worker) Start() error {
	addr := fmt.Sprintf("%s:%d", w.config.Host, w.config.Port)
	log.Printf("[WORKER] Agent Service listening on %s", addr)
	log.Printf("[WORKER] Available agents: %d", len(w.agentRegistry.GetAllAgents()))

	return http.ListenAndServe(addr, w.router)
}

// ============================================================================
// Agent Service Handlers (from engine/cmd/server/agent_service_handlers.go)
// ============================================================================

// handleAgentServiceExecute executes a DoAgent
func (w *Worker) handleAgentServiceExecute(rw http.ResponseWriter, r *http.Request) {
	// Validate authentication
	if !w.validateAuth(r) {
		http.Error(rw, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req mywant.ExecuteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(rw, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	// Get agent from registry
	agent, exists := w.agentRegistry.GetAgent(req.AgentName)
	if !exists {
		http.Error(rw, fmt.Sprintf("Agent not found: %s", req.AgentName), http.StatusNotFound)
		return
	}

	// Create temporary want with provided state (stateless)
	want := &mywant.Want{
		Metadata: mywant.Metadata{Name: req.WantID},
		State:    req.WantState,
	}
	want.BeginProgressCycle()

	// Execute the agent
	start := time.Now()
	ctx := r.Context()
	_, err := agent.Exec(ctx, want)

	// Get only changed fields
	stateUpdates := want.GetPendingStateChanges()
	want.EndProgressCycle()

	// Build response
	response := mywant.ExecuteResponse{
		Status:          "completed",
		StateUpdates:    stateUpdates,
		ExecutionTimeMs: time.Since(start).Milliseconds(),
	}

	if err != nil {
		response.Status = "failed"
		response.Error = err.Error()
	}

	rw.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(rw).Encode(response); err != nil {
		log.Printf("[WORKER] Failed to encode response: %v", err)
		return
	}

	log.Printf("[WORKER] Executed DoAgent %s (status: %s, changed: %d fields, duration: %dms)",
		req.AgentName, response.Status, len(stateUpdates), response.ExecutionTimeMs)
}

// handleAgentServiceMonitorExecute executes a MonitorAgent one cycle
func (w *Worker) handleAgentServiceMonitorExecute(rw http.ResponseWriter, r *http.Request) {
	// Validate authentication
	if !w.validateAuth(r) {
		http.Error(rw, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req mywant.MonitorRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(rw, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	// Get agent from registry
	agent, exists := w.agentRegistry.GetAgent(req.AgentName)
	if !exists {
		http.Error(rw, fmt.Sprintf("Agent not found: %s", req.AgentName), http.StatusNotFound)
		return
	}

	// Create want with latest state (provided by main server)
	want := &mywant.Want{
		Metadata: mywant.Metadata{Name: req.WantID},
		State:    req.WantState, // Latest state from main server
	}

	// Set callback configuration
	want.SetRemoteCallback(req.CallbackURL, req.AgentName)
	want.BeginProgressCycle()

	// Execute MonitorAgent one cycle
	start := time.Now()
	_, err := agent.Exec(context.Background(), want)

	// Get changes and send callback if any
	stateUpdates := want.GetPendingStateChanges()
	if len(stateUpdates) > 0 {
		if callbackErr := want.SendCallback(); callbackErr != nil {
			log.Printf("[WORKER] Callback failed for %s: %v", req.AgentName, callbackErr)
		}
	}

	want.EndProgressCycle()

	// Build response
	response := map[string]interface{}{
		"status":              "completed",
		"state_updates_count": len(stateUpdates),
		"execution_time_ms":   time.Since(start).Milliseconds(),
	}

	if err != nil {
		response["status"] = "failed"
		response["error"] = err.Error()
	}

	rw.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(rw).Encode(response); err != nil {
		log.Printf("[WORKER] Failed to encode response: %v", err)
		return
	}

	log.Printf("[WORKER] MonitorAgent %s executed (cycle: %dms, changes: %d)",
		req.AgentName, response["execution_time_ms"], len(stateUpdates))
}

// ============================================================================
// Utility Handlers
// ============================================================================

// handleHealth returns health status
func (w *Worker) handleHealth(rw http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"status":         "healthy",
		"mode":           "worker",
		"agents_count":   len(w.agentRegistry.GetAllAgents()),
		"capabilities":   len(w.agentRegistry.GetAllCapabilities()),
	}

	rw.Header().Set("Content-Type", "application/json")
	json.NewEncoder(rw).Encode(response)
}

// handleListAgents returns list of registered agents
func (w *Worker) handleListAgents(rw http.ResponseWriter, r *http.Request) {
	agents := w.agentRegistry.GetAllAgents()

	// Extract agent info
	agentInfo := make([]map[string]interface{}, 0, len(agents))
	for _, agent := range agents {
		agentInfo = append(agentInfo, map[string]interface{}{
			"name":         agent.GetName(),
			"type":         agent.GetType(),
			"capabilities": agent.GetCapabilities(),
		})
	}

	rw.Header().Set("Content-Type", "application/json")
	json.NewEncoder(rw).Encode(map[string]interface{}{
		"agents": agentInfo,
		"count":  len(agents),
	})
}

// handleListCapabilities returns list of registered capabilities
func (w *Worker) handleListCapabilities(rw http.ResponseWriter, r *http.Request) {
	capabilities := w.agentRegistry.GetAllCapabilities()

	// Extract capability info
	capabilityInfo := make([]map[string]interface{}, 0, len(capabilities))
	for _, cap := range capabilities {
		capabilityInfo = append(capabilityInfo, map[string]interface{}{
			"name":  cap.Name,
			"gives": cap.Gives,
		})
	}

	rw.Header().Set("Content-Type", "application/json")
	json.NewEncoder(rw).Encode(map[string]interface{}{
		"capabilities": capabilityInfo,
		"count":        len(capabilities),
	})
}

// ============================================================================
// Authentication
// ============================================================================

// validateAuth validates Bearer token authentication
func (w *Worker) validateAuth(r *http.Request) bool {
	expectedToken := os.Getenv("WEBHOOK_AUTH_TOKEN")

	// If no token configured, allow all requests (development mode)
	if expectedToken == "" {
		log.Printf("[WORKER] Warning: WEBHOOK_AUTH_TOKEN not set, allowing all requests")
		return true
	}

	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return false
	}

	const prefix = "Bearer "
	if len(authHeader) < len(prefix) {
		return false
	}

	token := authHeader[len(prefix):]
	return token == expectedToken
}
