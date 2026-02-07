package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/mux"
)

// Request/Response types matching MyWant's executor types
type ExecuteRequest struct {
	WantID      string         `json:"want_id"`
	AgentName   string         `json:"agent_name"`
	Operation   string         `json:"operation"`
	WantState   map[string]any `json:"want_state"`
	Params      map[string]any `json:"params,omitempty"`
	CallbackURL string         `json:"callback_url,omitempty"`
}

type ExecuteResponse struct {
	Status          string         `json:"status"`
	StateUpdates    map[string]any `json:"state_updates,omitempty"`
	Error           string         `json:"error,omitempty"`
	ExecutionTimeMs int64          `json:"execution_time_ms,omitempty"`
}

type MonitorRequest struct {
	WantID      string         `json:"want_id"`
	AgentName   string         `json:"agent_name"`
	CallbackURL string         `json:"callback_url"`
	WantState   map[string]any `json:"want_state"`
}

type MonitorResponse struct {
	MonitorID string `json:"monitor_id"`
	Status    string `json:"status"`
	Error     string `json:"error,omitempty"`
}

type WebhookCallback struct {
	AgentName    string         `json:"agent_name"`
	WantID       string         `json:"want_id"`
	Status       string         `json:"status"`
	StateUpdates map[string]any `json:"state_updates,omitempty"`
	Error        string         `json:"error,omitempty"`
}

// MyWant client for state queries
type MyWantClient struct {
	baseURL   string
	authToken string
	client    *http.Client
}

func NewMyWantClient(baseURL, authToken string) *MyWantClient {
	return &MyWantClient{
		baseURL:   baseURL,
		authToken: authToken,
		client:    &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *MyWantClient) SendCallback(callbackURL string, callback WebhookCallback) error {
	bodyBytes, err := json.Marshal(callback)
	if err != nil {
		return fmt.Errorf("failed to marshal callback: %w", err)
	}

	req, err := http.NewRequest("POST", callbackURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.authToken)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("callback request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("callback returned status %d", resp.StatusCode)
	}

	return nil
}

// External Agent Server
type ExternalAgentServer struct {
	mywantClient *MyWantClient
}

func NewExternalAgentServer(mywantURL, authToken string) *ExternalAgentServer {
	return &ExternalAgentServer{
		mywantClient: NewMyWantClient(mywantURL, authToken),
	}
}

// handleFlightExecute simulates a flight booking DoAgent
func (s *ExternalAgentServer) handleFlightExecute(w http.ResponseWriter, r *http.Request) {
	var req ExecuteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	log.Printf("[FLIGHT AGENT] Executing flight booking for want %s", req.WantID)
	log.Printf("[FLIGHT AGENT] Current state: %+v", req.WantState)

	// Simulate flight booking process
	time.Sleep(2 * time.Second)

	// Generate booking result
	bookingID := fmt.Sprintf("FLT-%d", time.Now().Unix())
	stateUpdates := map[string]any{
		"flight_booking_id": bookingID,
		"booking_status":    "confirmed",
		"booking_time":      time.Now().Format(time.RFC3339),
		"departure":         req.WantState["departure"],
		"arrival":           req.WantState["arrival"],
	}

	// If callback URL provided, send async callback
	if req.CallbackURL != "" {
		go func() {
			callback := WebhookCallback{
				AgentName:    req.AgentName,
				WantID:       req.WantID,
				Status:       "completed",
				StateUpdates: stateUpdates,
			}

			if err := s.mywantClient.SendCallback(req.CallbackURL, callback); err != nil {
				log.Printf("[FLIGHT AGENT] Failed to send callback: %v", err)
			} else {
				log.Printf("[FLIGHT AGENT] Callback sent successfully for booking %s", bookingID)
			}
		}()
	}

	// Return immediate response
	response := ExecuteResponse{
		Status:          "completed",
		StateUpdates:    stateUpdates,
		ExecutionTimeMs: 2000,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)

	log.Printf("[FLIGHT AGENT] Flight booked: %s", bookingID)
}

// handleReactionMonitor simulates a user reaction MonitorAgent
func (s *ExternalAgentServer) handleReactionMonitor(w http.ResponseWriter, r *http.Request) {
	var req MonitorRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	log.Printf("[REACTION MONITOR] Starting monitor for want %s", req.WantID)

	monitorID := fmt.Sprintf("monitor-%d", time.Now().UnixNano())

	// Start background monitoring
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		checkCount := 0
		for {
			<-ticker.C
			checkCount++

			log.Printf("[REACTION MONITOR] Check #%d for want %s", checkCount, req.WantID)

			// Simulate detecting user reaction after 3 checks
			if checkCount >= 3 {
				log.Printf("[REACTION MONITOR] User reaction detected for want %s", req.WantID)

				callback := WebhookCallback{
					AgentName: req.AgentName,
					WantID:    req.WantID,
					Status:    "state_changed",
					StateUpdates: map[string]any{
						"user_reaction": map[string]any{
							"approved":  true,
							"timestamp": time.Now().Format(time.RFC3339),
							"comment":   "Looks good!",
						},
					},
				}

				if err := s.mywantClient.SendCallback(req.CallbackURL, callback); err != nil {
					log.Printf("[REACTION MONITOR] Failed to send callback: %v", err)
				} else {
					log.Printf("[REACTION MONITOR] User reaction callback sent for want %s", req.WantID)
				}

				return // Stop monitoring
			}
		}
	}()

	response := MonitorResponse{
		MonitorID: monitorID,
		Status:    "started",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)

	log.Printf("[REACTION MONITOR] Monitor started: %s", monitorID)
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "9000"
	}

	mywantURL := os.Getenv("MYWANT_URL")
	if mywantURL == "" {
		mywantURL = "http://localhost:8080"
	}

	authToken := os.Getenv("WEBHOOK_AUTH_TOKEN")
	if authToken == "" {
		log.Println("WARNING: WEBHOOK_AUTH_TOKEN not set")
	}

	server := NewExternalAgentServer(mywantURL, authToken)

	router := mux.NewRouter()

	// DoAgent endpoints
	router.HandleFunc("/flight/execute", server.handleFlightExecute).Methods("POST")

	// MonitorAgent endpoints
	router.HandleFunc("/reaction/monitor/start", server.handleReactionMonitor).Methods("POST")

	// Health check
	router.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
	}).Methods("GET")

	log.Printf("External Agent Server starting on port %s", port)
	log.Printf("MyWant URL: %s", mywantURL)
	log.Printf("Available endpoints:")
	log.Printf("  POST /flight/execute - Flight booking DoAgent")
	log.Printf("  POST /reaction/monitor/start - User reaction MonitorAgent")
	log.Printf("  GET  /health - Health check")

	if err := http.ListenAndServe(":"+port, router); err != nil {
		log.Fatal(err)
	}
}
