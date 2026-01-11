package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	mywant "mywant/engine/src"
	"mywant/pkg/server"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestServerForE2E creates a test server with all components
func setupTestServerForE2E() *server.Server {
	config := server.Config{Port: 0, Host: "localhost", Debug: true}

	// Create dummy directories if they don't exist
	os.MkdirAll("../want_types", 0755)
	os.MkdirAll("../recipes", 0755)
	os.MkdirAll("../agents", 0755)
	os.MkdirAll("../capabilities", 0755)

	s := server.New(config)
	s.setupRoutes()

	return s
}

// mockGooseForE2E provides a simple mock for E2E tests
type mockGooseForE2E struct{}

func (m *mockGooseForE2E) ExecuteViaGoose(ctx context.Context, operation string, params map[string]interface{}) (interface{}, error) {
	// Return a simple hotel booking recommendation
	return map[string]interface{}{
		"recommendations": []map[string]interface{}{
			{
				"id":          "rec-1",
				"title":       "Simple Hotel Booking",
				"approach":    "custom",
				"description": "Direct hotel booking",
				"config": map[string]interface{}{
					"wants": []interface{}{
						map[string]interface{}{
							"metadata": map[string]interface{}{
								"name": "tokyo-hotel",
								"type": "hotel",
								"labels": map[string]interface{}{
									"location": "tokyo",
								},
							},
							"spec": map[string]interface{}{
								"params": map[string]interface{}{
									"location":       "Tokyo",
									"check_in_date":  "2026-02-01",
									"check_out_date": "2026-02-03",
								},
							},
						},
					},
				},
				"metadata": map[string]interface{}{
					"want_count":      1,
					"want_types_used": []string{"hotel"},
					"complexity":      "low",
					"pros_cons": map[string]interface{}{
						"pros": []string{"Simple", "Fast"},
						"cons": []string{"No integration"},
					},
				},
			},
			{
				"id":          "rec-2",
				"title":       "Travel Package",
				"approach":    "recipe",
				"description": "Complete travel solution",
				"config": map[string]interface{}{
					"wants": []interface{}{
						map[string]interface{}{
							"metadata": map[string]interface{}{
								"name": "hotel",
								"type": "hotel",
							},
							"spec": map[string]interface{}{
								"params": map[string]interface{}{
									"location": "Tokyo",
								},
							},
						},
						map[string]interface{}{
							"metadata": map[string]interface{}{
								"name": "restaurant",
								"type": "restaurant",
							},
							"spec": map[string]interface{}{
								"params": map[string]interface{}{
									"location": "Tokyo",
								},
							},
						},
					},
				},
				"metadata": map[string]interface{}{
					"want_count":      2,
					"want_types_used": []string{"hotel", "restaurant"},
					"complexity":      "medium",
					"pros_cons": map[string]interface{}{
						"pros": []string{"Complete", "Integrated"},
						"cons": []string{"More complex"},
					},
				},
			},
		},
	}, nil
}

// TestInteractE2E_CompleteFlow verifies the complete workflow
func TestInteractE2E_CompleteFlow(t *testing.T) {
	s := setupTestServerForE2E()

	// Override with mock
	mockGoose := &mockGooseForE2E{}
	s.interactionManager = mywant.NewInteractionManager(s.wantTypeLoader, s.recipeRegistry, mockGoose)

	// Step 1: Create session
	req, _ := http.NewRequest("POST", "/api/v1/interact", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	var createResp server.InteractCreateResponse
	json.Unmarshal(w.Body.Bytes(), &createResp)
	sessionID := createResp.SessionID
	assert.NotEmpty(t, sessionID)
	t.Logf("Created session: %s", sessionID)

	// Step 2: Send message and get recommendations
	msgPayload := server.InteractMessageRequest{
		Message: "I want to book a hotel in Tokyo",
	}
	body, _ := json.Marshal(msgPayload)

	req, _ = http.NewRequest("POST", "/api/v1/interact/"+sessionID, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	s.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var msgResp server.InteractMessageResponse
	json.Unmarshal(w.Body.Bytes(), &msgResp)
	require.NotEmpty(t, msgResp.Recommendations)
	assert.Len(t, msgResp.Recommendations, 2)
	recID := msgResp.Recommendations[0].ID
	t.Logf("Received %d recommendations, using: %s", len(msgResp.Recommendations), recID)

	// Step 3: Deploy recommendation
	deployPayload := server.InteractDeployRequest{
		RecommendationID: recID,
	}
	body, _ = json.Marshal(deployPayload)

	req, _ = http.NewRequest("POST", "/api/v1/interact/"+sessionID+"/deploy", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	s.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var deployResp server.InteractDeployResponse
	json.Unmarshal(w.Body.Bytes(), &deployResp)
	assert.NotEmpty(t, deployResp.ExecutionID)
	assert.NotEmpty(t, deployResp.WantIDs)
	assert.Equal(t, "deployed", deployResp.Status)
	t.Logf("Deployed execution: %s with %d wants", deployResp.ExecutionID, len(deployResp.WantIDs))

	// Step 4: Verify wants were created
	req, _ = http.NewRequest("GET", "/api/v1/wants", nil)
	w = httptest.NewRecorder()
	s.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var wantsResp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &wantsResp)
	wants := wantsResp["wants"].([]interface{})
	assert.NotEmpty(t, wants)
	t.Logf("Verified %d wants created", len(wants))

	// Step 5: Delete session
	req, _ = http.NewRequest("DELETE", "/api/v1/interact/"+sessionID, nil)
	w = httptest.NewRecorder()
	s.router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNoContent, w.Code)
	t.Logf("Session deleted successfully")

	// Step 6: Verify session is gone
	_, err := s.interactionManager.GetSession(sessionID)
	assert.Error(t, err)
}

// TestInteractE2E_ConversationalFlow verifies conversation continuity
func TestInteractE2E_ConversationalFlow(t *testing.T) {
	s := setupTestServerForE2E()

	// Override with mock
	mockGoose := &mockGooseForE2E{}
	s.interactionManager = mywant.NewInteractionManager(s.wantTypeLoader, s.recipeRegistry, mockGoose)

	// Create session
	session := createSessionE2E(t, s)
	t.Logf("Created session: %s", session)

	// First message
	resp1 := sendMessageE2E(t, s, session, "I want to travel to Tokyo")
	assert.Len(t, resp1.ConversationHistory, 2) // user + assistant
	t.Logf("First message: %d history items", len(resp1.ConversationHistory))

	// Second message (conversational continuation)
	resp2 := sendMessageE2E(t, s, session, "Add a hotel booking to that")
	assert.Len(t, resp2.ConversationHistory, 4) // 2 user + 2 assistant
	t.Logf("Second message: %d history items", len(resp2.ConversationHistory))

	// Verify conversation history order
	assert.Equal(t, "user", resp2.ConversationHistory[0].Role)
	assert.Equal(t, "I want to travel to Tokyo", resp2.ConversationHistory[0].Content)
	assert.Equal(t, "assistant", resp2.ConversationHistory[1].Role)
	assert.Equal(t, "user", resp2.ConversationHistory[2].Role)
	assert.Equal(t, "Add a hotel booking to that", resp2.ConversationHistory[2].Content)
	assert.Equal(t, "assistant", resp2.ConversationHistory[3].Role)

	// Third message
	resp3 := sendMessageE2E(t, s, session, "Make it luxury")
	assert.Len(t, resp3.ConversationHistory, 6) // 3 user + 3 assistant
	t.Logf("Third message: %d history items", len(resp3.ConversationHistory))
}

// TestInteractE2E_DeploymentWithModifications verifies parameter override
func TestInteractE2E_DeploymentWithModifications(t *testing.T) {
	s := setupTestServerForE2E()

	mockGoose := &mockGooseForE2E{}
	s.interactionManager = mywant.NewInteractionManager(s.wantTypeLoader, s.recipeRegistry, mockGoose)

	// Create session and get recommendations
	session := createSessionE2E(t, s)
	resp := sendMessageE2E(t, s, session, "Book hotel and restaurant")
	require.Len(t, resp.Recommendations, 2)

	// Deploy second recommendation (travel package) with modifications
	deployPayload := server.InteractDeployRequest{
		RecommendationID: "rec-2",
		Modifications: &server.ConfigModifications{
			ParameterOverrides: map[string]interface{}{
				"check_in_date": "2026-03-15",
			},
			DisableWants: []string{"restaurant"},
		},
	}
	body, _ := json.Marshal(deployPayload)

	req, _ := http.NewRequest("POST", "/api/v1/interact/"+session+"/deploy", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var deployResp server.InteractDeployResponse
	json.Unmarshal(w.Body.Bytes(), &deployResp)

	// Should have 1 want (hotel only, restaurant disabled)
	assert.Len(t, deployResp.WantIDs, 1)
	t.Logf("Deployed %d wants (restaurant disabled)", len(deployResp.WantIDs))
}

// TestInteractE2E_MultipleSessionsIsolation verifies session isolation
func TestInteractE2E_MultipleSessionsIsolation(t *testing.T) {
	s := setupTestServerForE2E()

	mockGoose := &mockGooseForE2E{}
	s.interactionManager = mywant.NewInteractionManager(s.wantTypeLoader, s.recipeRegistry, mockGoose)

	// Create session A
	sessionA := createSessionE2E(t, s)
	respA := sendMessageE2E(t, s, sessionA, "Book hotel in Tokyo")
	t.Logf("Session A: %d recommendations", len(respA.Recommendations))

	// Create session B
	sessionB := createSessionE2E(t, s)
	respB := sendMessageE2E(t, s, sessionB, "Book hotel in Osaka")
	t.Logf("Session B: %d recommendations", len(respB.Recommendations))

	// Verify sessions are independent
	assert.NotEqual(t, sessionA, sessionB)

	// Session A should have its own history
	sessionAData, err := s.interactionManager.GetSession(sessionA)
	require.NoError(t, err)
	assert.Len(t, sessionAData.ConversationHistory, 2)
	assert.Contains(t, sessionAData.ConversationHistory[0].Content, "Tokyo")

	// Session B should have its own history
	sessionBData, err := s.interactionManager.GetSession(sessionB)
	require.NoError(t, err)
	assert.Len(t, sessionBData.ConversationHistory, 2)
	assert.Contains(t, sessionBData.ConversationHistory[0].Content, "Osaka")
}

// TestInteractE2E_ErrorRecovery verifies error handling doesn't break the system
func TestInteractE2E_ErrorRecovery(t *testing.T) {
	s := setupTestServerForE2E()

	mockGoose := &mockGooseForE2E{}
	s.interactionManager = mywant.NewInteractionManager(s.wantTypeLoader, s.recipeRegistry, mockGoose)

	session := createSessionE2E(t, s)

	// Test 1: Empty message
	payload1 := server.InteractMessageRequest{Message: ""}
	body1, _ := json.Marshal(payload1)
	req1, _ := http.NewRequest("POST", "/api/v1/interact/"+session, bytes.NewBuffer(body1))
	req1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()
	s.router.ServeHTTP(w1, req1)
	assert.Equal(t, http.StatusBadRequest, w1.Code)
	t.Logf("Empty message properly rejected")

	// Test 2: Valid message after error (system should still work)
	resp := sendMessageE2E(t, s, session, "Book hotel")
	assert.NotEmpty(t, resp.Recommendations)
	t.Logf("System recovered, recommendations: %d", len(resp.Recommendations))

	// Test 3: Invalid recommendation deployment
	deployPayload := server.InteractDeployRequest{RecommendationID: "rec-999"}
	body, _ := json.Marshal(deployPayload)
	req, _ := http.NewRequest("POST", "/api/v1/interact/"+session+"/deploy", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
	t.Logf("Invalid recommendation properly rejected")

	// Test 4: Valid deployment after error
	getRecommendations := sendMessageE2E(t, s, session, "Book hotel again")
	if len(getRecommendations.Recommendations) > 0 {
		validDeployPayload := server.InteractDeployRequest{
			RecommendationID: getRecommendations.Recommendations[0].ID,
		}
		validBody, _ := json.Marshal(validDeployPayload)
		validReq, _ := http.NewRequest("POST", "/api/v1/interact/"+session+"/deploy", bytes.NewBuffer(validBody))
		validReq.Header.Set("Content-Type", "application/json")
		validW := httptest.NewRecorder()
		s.router.ServeHTTP(validW, validReq)
		assert.Equal(t, http.StatusOK, validW.Code)
		t.Logf("Valid deployment succeeded after errors")
	}
}

// TestInteractE2E_SessionLifecycle verifies session creation and cleanup
func TestInteractE2E_SessionLifecycle(t *testing.T) {
	s := setupTestServerForE2E()

	// Create session
	req, _ := http.NewRequest("POST", "/api/v1/interact", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	var createResp server.InteractCreateResponse
	json.Unmarshal(w.Body.Bytes(), &createResp)
	sessionID := createResp.SessionID

	// Verify expiry time is ~30 minutes
	duration := createResp.ExpiresAt.Sub(createResp.CreatedAt)
	assert.InDelta(t, 30*60, duration.Seconds(), 5) // Allow 5 second tolerance
	t.Logf("Session expires in %.0f seconds", duration.Seconds())

	// Session should be accessible
	session, err := s.interactionManager.GetSession(sessionID)
	require.NoError(t, err)
	assert.Equal(t, sessionID, session.SessionID)

	// Delete session
	delReq, _ := http.NewRequest("DELETE", "/api/v1/interact/"+sessionID, nil)
	delW := httptest.NewRecorder()
	s.router.ServeHTTP(delW, delReq)
	assert.Equal(t, http.StatusNoContent, delW.Code)

	// Session should be gone
	_, err = s.interactionManager.GetSession(sessionID)
	assert.Error(t, err)
	t.Logf("Session properly cleaned up")
}

// Helper functions

func createSessionE2E(t *testing.T, s *server.Server) string {
	req, _ := http.NewRequest("POST", "/api/v1/interact", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	var resp server.InteractCreateResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	return resp.SessionID
}

func sendMessageE2E(t *testing.T, s *server.Server, sessionID string, message string) server.InteractMessageResponse {
	payload := server.InteractMessageRequest{Message: message}
	body, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", "/api/v1/interact/"+sessionID, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp server.InteractMessageResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	return resp
}
