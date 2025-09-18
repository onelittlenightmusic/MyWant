package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	mywant "mywant/src"
)

// ServerConfig holds server configuration
type ServerConfig struct {
	Port int    `json:"port"`
	Host string `json:"host"`
}

// Server represents the MyWant server
type Server struct {
	config ServerConfig
	wants  map[string]*WantExecution // Store active want executions
	router *mux.Router
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
	return &Server{
		config: config,
		wants:  make(map[string]*WantExecution),
		router: mux.NewRouter(),
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

	// Generate unique ID for this want execution
	wantID := generateWantID()

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

// listWants handles GET /api/v1/wants - lists all wants
func (s *Server) listWants(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	wants := make([]*WantExecution, 0, len(s.wants))
	for _, want := range s.wants {
		wants = append(wants, want)
	}

	json.NewEncoder(w).Encode(wants)
}

// getWant handles GET /api/v1/wants/{id} - gets current runtime state of wants
func (s *Server) getWant(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	wantID := vars["id"]

	execution, exists := s.wants[wantID]
	if !exists {
		http.Error(w, "Want not found", http.StatusNotFound)
		return
	}

	// Return current runtime state if builder is available (execution started)
	if execution.Builder != nil {
		currentStates := execution.Builder.GetAllWantStates()
		response := map[string]interface{}{
			"id":               execution.ID,
			"execution_status": execution.Status,
			"wants":            currentStates,
			"results":          execution.Results,
		}
		json.NewEncoder(w).Encode(response)
	} else {
		// If no builder yet (not executed), return the original execution info
		json.NewEncoder(w).Encode(execution)
	}
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

	// Create chain builder
	builder := mywant.NewChainBuilder(want.Config)
	want.Builder = builder

	// Register QNet types from demos package
	RegisterQNetWantTypes(builder)

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

// Start starts the HTTP server
func (s *Server) Start() error {
	s.setupRoutes()
	addr := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)

	fmt.Printf("🚀 MyWant server starting on %s\n", addr)
	fmt.Printf("📋 Available endpoints:\n")
	fmt.Printf("  GET  /health                     - Health check\n")
	fmt.Printf("  POST /api/v1/wants              - Create want (YAML config)\n")
	fmt.Printf("  GET  /api/v1/wants              - List wants\n")
	fmt.Printf("  GET  /api/v1/wants/{id}         - Get want\n")
	fmt.Printf("  PUT  /api/v1/wants/{id}         - Update want\n")
	fmt.Printf("  DELETE /api/v1/wants/{id}       - Delete want\n")
	fmt.Printf("  GET  /api/v1/wants/{id}/status  - Get execution status\n")
	fmt.Printf("  GET  /api/v1/wants/{id}/results - Get execution results\n")
	fmt.Printf("\n")

	return http.ListenAndServe(addr, s.router)
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
