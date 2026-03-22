package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	mywant "mywant/engine/core"

	"github.com/gorilla/mux"
)

// WhimThinkerMessageRequest is sent when the user sends a message to the whim thinker
type WhimThinkerMessageRequest struct {
	Message string `json:"message"`
}

// WhimThinkerMessageResponse is returned after processing a message
type WhimThinkerMessageResponse struct {
	Recommendations     []mywant.Recommendation      `json:"recommendations"`
	ConversationHistory []mywant.ConversationMessage `json:"conversation_history"`
	Timestamp           time.Time                    `json:"timestamp"`
}

// WhimThinkerDeployRequest selects a recommendation to deploy as sibling wants
type WhimThinkerDeployRequest struct {
	RecommendationID string `json:"recommendation_id"`
}

// WhimThinkerDeployResponse is returned after deployment
type WhimThinkerDeployResponse struct {
	WantIDs   []string  `json:"want_ids"`
	Status    string    `json:"status"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

// whimThinkerMessage handles POST /api/v1/whim-thinker/{id}/message
// It sends a user message to the interact session associated with the whim thinker,
// stores the resulting recommendations in the thinker want's state, and returns them.
func (s *Server) whimThinkerMessage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	thinkerID := mux.Vars(r)["id"]
	thinkerWant := s.findWantByIDInAll(thinkerID)
	if thinkerWant == nil {
		http.Error(w, fmt.Sprintf("whim_thinker want %s not found", thinkerID), http.StatusNotFound)
		return
	}

	var req WhimThinkerMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Message == "" {
		http.Error(w, "message is required", http.StatusBadRequest)
		return
	}

	// Mark as thinking
	thinkerWant.StoreState("isThinking", true)
	thinkerWant.StoreState("error", "")

	// Get or create interact session
	sessionID, hasSession := thinkerWant.GetStateString("sessionId", "")
	if !hasSession || sessionID == "" {
		session := s.interactionManager.CreateSession()
		sessionID = session.SessionID
		thinkerWant.StoreState("sessionId", sessionID)
	}

	// Get parent whim's "want" param to provide context to the AI
	parentContext := s.getWhimParentWantParam(thinkerWant)

	// Build full message with whim context if available
	fullMessage := req.Message
	if parentContext != "" && fullMessage != parentContext {
		fullMessage = fmt.Sprintf("[やりたいこと: %s]\n\n%s", parentContext, req.Message)
	}

	// Send message and get recommendations
	session, err := s.interactionManager.SendMessage(r.Context(), sessionID, fullMessage, nil)
	if err != nil {
		thinkerWant.StoreState("isThinking", false)
		thinkerWant.StoreState("error", err.Error())
		http.Error(w, fmt.Sprintf("failed to get recommendations: %v", err), http.StatusInternalServerError)
		return
	}

	// Update thinker state
	thinkerWant.StoreState("isThinking", false)
	thinkerWant.StoreState("recommendations", session.Recommendations)
	thinkerWant.StoreState("conversationHistory", session.ConversationHistory)

	resp := WhimThinkerMessageResponse{
		Recommendations:     session.Recommendations,
		ConversationHistory: session.ConversationHistory,
		Timestamp:           time.Now(),
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// whimThinkerDeploy handles POST /api/v1/whim-thinker/{id}/deploy
// It deploys a selected recommendation as child wants of the parent whim coordinator.
func (s *Server) whimThinkerDeploy(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	thinkerID := mux.Vars(r)["id"]
	thinkerWant := s.findWantByIDInAll(thinkerID)
	if thinkerWant == nil {
		http.Error(w, fmt.Sprintf("whim_thinker want %s not found", thinkerID), http.StatusNotFound)
		return
	}

	var req WhimThinkerDeployRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.RecommendationID == "" {
		http.Error(w, "recommendation_id is required", http.StatusBadRequest)
		return
	}

	sessionID, hasSession := thinkerWant.GetStateString("sessionId", "")
	if !hasSession || sessionID == "" {
		http.Error(w, "no interact session found; send a message first", http.StatusBadRequest)
		return
	}

	recommendation, err := s.interactionManager.GetRecommendation(sessionID, req.RecommendationID)
	if err != nil {
		http.Error(w, fmt.Sprintf("recommendation not found: %v", err), http.StatusNotFound)
		return
	}

	// Assign IDs and set the parent whim as owner of the new wants
	parentID := s.getWhimParentID(thinkerWant)
	allWants := s.globalBuilder.GetWants()

	newWants := recommendation.Config.Wants
	for _, want := range newWants {
		if want.Metadata.ID == "" {
			want.Metadata.ID = generateWantID()
		}
		if parentID != "" {
			// Find parent name
			parentName := parentID
			for _, w := range allWants {
				if w.Metadata.ID == parentID {
					parentName = w.Metadata.Name
					break
				}
			}
			want.Metadata.OwnerReferences = []mywant.OwnerReference{
				{
					APIVersion:         "mywant/v1",
					Kind:               "Want",
					Name:               parentName,
					ID:                 parentID,
					Controller:         true,
					BlockOwnerDeletion: true,
				},
			}
		}
	}

	wantIDs, err := s.globalBuilder.AddWantsAsyncWithTracking(newWants)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to deploy wants: %v", err), http.StatusInternalServerError)
		return
	}

	resp := WhimThinkerDeployResponse{
		WantIDs:   wantIDs,
		Status:    "deployed",
		Message:   fmt.Sprintf("Deployed %d want(s) from recommendation %s", len(wantIDs), req.RecommendationID),
		Timestamp: time.Now(),
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// getWhimParentID returns the ID of the parent whim coordinator of a whim_thinker want.
func (s *Server) getWhimParentID(thinkerWant *mywant.Want) string {
	for _, ref := range thinkerWant.Metadata.OwnerReferences {
		if ref.Controller {
			return ref.ID
		}
	}
	return ""
}

// getWhimParentWantParam retrieves the "want" param from the parent whim coordinator.
func (s *Server) getWhimParentWantParam(thinkerWant *mywant.Want) string {
	parentID := s.getWhimParentID(thinkerWant)
	if parentID == "" {
		return ""
	}
	parentWant := s.findWantByIDInAll(parentID)
	if parentWant == nil {
		return ""
	}
	// Try current state first (the "want" field has label: current), then spec params
	if v, ok := parentWant.GetCurrent("want"); ok {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	if parentWant.Spec.Params != nil {
		if v, ok := parentWant.Spec.Params["want"]; ok {
			if s, ok := v.(string); ok {
				return s
			}
		}
	}
	return ""
}
