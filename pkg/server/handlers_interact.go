package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	mywant "mywant/engine/core"

	"github.com/gorilla/mux"
)

// ========= Interactive Want Creation Types =========

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
