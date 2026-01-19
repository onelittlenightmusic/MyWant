package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	mywant "mywant/engine/src"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockGooseExecutor for testing
type mockGooseExecutor struct {
	recommendations []mywant.Recommendation
	executeFunc     func(ctx context.Context, operation string, params map[string]interface{}) (interface{}, error)
}

func (m *mockGooseExecutor) ExecuteViaGoose(ctx context.Context, operation string, params map[string]interface{}) (interface{}, error) {
	if m.executeFunc != nil {
		return m.executeFunc(ctx, operation, params)
	}

	// Default mock response - properly serialize wants
	recs := make([]map[string]interface{}, len(m.recommendations))
	for i, rec := range m.recommendations {
		// Serialize wants from the recommendation
		wants := make([]interface{}, len(rec.Config.Wants))
		for j, want := range rec.Config.Wants {
			wants[j] = map[string]interface{}{
				"metadata": map[string]interface{}{
					"name": want.Metadata.Name,
					"type": want.Metadata.Type,
					"id":   want.Metadata.ID,
				},
				"spec": map[string]interface{}{
					"params": want.Spec.Params,
				},
			}
		}

		recs[i] = map[string]interface{}{
			"id":          rec.ID,
			"title":       rec.Title,
			"approach":    rec.Approach,
			"description": rec.Description,
			"config": map[string]interface{}{
				"wants": wants,
			},
			"metadata": map[string]interface{}{
				"want_count":      len(rec.Config.Wants),
				"want_types_used": []string{},
				"complexity":      "low",
				"pros_cons": map[string]interface{}{
					"pros": []string{},
					"cons": []string{},
				},
			},
		}
	}

	return map[string]interface{}{
		"recommendations": recs,
	}, nil
}

// TestInteractCreate verifies session creation endpoint
func TestInteractCreate(t *testing.T) {
	s := setupTestServer()

	req, _ := http.NewRequest("POST", "/api/v1/interact", nil)
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var resp InteractCreateResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.NotEmpty(t, resp.SessionID)
	assert.Contains(t, resp.SessionID, "sess-")
	assert.NotZero(t, resp.CreatedAt)
	assert.NotZero(t, resp.ExpiresAt)

	// Verify 30 minute expiry
	duration := resp.ExpiresAt.Sub(resp.CreatedAt)
	assert.Equal(t, 30*60, int(duration.Seconds()))
}

// TestInteractMessage_Success verifies successful message sending
func TestInteractMessage_Success(t *testing.T) {
	s := setupTestServer()

	// Override interactionManager with mock
	mockGoose := &mockGooseExecutor{
		recommendations: []mywant.Recommendation{
			{
				ID:          "rec-1",
				Title:       "Test Hotel Booking",
				Approach:    "custom",
				Description: "Test description",
				Config:      mywant.Config{Wants: []*mywant.Want{}},
			},
		},
	}
	s.interactionManager = mywant.NewInteractionManager(s.wantTypeLoader, s.recipeRegistry, mockGoose)

	// Create session first
	session := s.interactionManager.CreateSession()

	// Send message
	payload := InteractMessageRequest{
		Message: "I want to book a hotel",
	}
	body, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", "/api/v1/interact/"+session.SessionID, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp InteractMessageResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Len(t, resp.Recommendations, 1)
	assert.Equal(t, "rec-1", resp.Recommendations[0].ID)
	assert.Equal(t, "Test Hotel Booking", resp.Recommendations[0].Title)
	assert.Len(t, resp.ConversationHistory, 2) // user + assistant
	assert.Equal(t, "user", resp.ConversationHistory[0].Role)
	assert.Equal(t, "I want to book a hotel", resp.ConversationHistory[0].Content)
}

// TestInteractMessage_EmptyMessage verifies empty message validation
func TestInteractMessage_EmptyMessage(t *testing.T) {
	s := setupTestServer()
	session := s.interactionManager.CreateSession()

	payload := InteractMessageRequest{
		Message: "",
	}
	body, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", "/api/v1/interact/"+session.SessionID, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "required")
}

// TestInteractMessage_SessionNotFound verifies error handling for missing session
func TestInteractMessage_SessionNotFound(t *testing.T) {
	s := setupTestServer()

	payload := InteractMessageRequest{
		Message: "Test",
	}
	body, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", "/api/v1/interact/invalid-session-id", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "not found")
}

// TestInteractMessage_InvalidJSON verifies JSON parsing error handling
func TestInteractMessage_InvalidJSON(t *testing.T) {
	s := setupTestServer()
	session := s.interactionManager.CreateSession()

	req, _ := http.NewRequest("POST", "/api/v1/interact/"+session.SessionID, bytes.NewBufferString("{invalid json}"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestInteractDelete verifies session deletion
func TestInteractDelete(t *testing.T) {
	s := setupTestServer()
	session := s.interactionManager.CreateSession()

	req, _ := http.NewRequest("DELETE", "/api/v1/interact/"+session.SessionID, nil)
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)

	// Verify session is deleted
	_, err := s.interactionManager.GetSession(session.SessionID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// TestInteractDeploy_Success verifies successful recommendation deployment
func TestInteractDeploy_Success(t *testing.T) {
	s := setupTestServer()

	// Override with mock that returns a specific recommendation
	mockGoose := &mockGooseExecutor{
		recommendations: []mywant.Recommendation{
			{
				ID:          "rec-1",
				Title:       "Test",
				Approach:    "custom",
				Description: "Test",
				Config: mywant.Config{
					Wants: []*mywant.Want{
						{
							Metadata: mywant.Metadata{
								Name: "test-queue",
								Type: "qnet queue",
							},
							Spec: mywant.WantSpec{
								Params: map[string]interface{}{
									"service_time": 0.1,
								},
							},
						},
					},
				},
			},
		},
	}
	s.interactionManager = mywant.NewInteractionManager(s.wantTypeLoader, s.recipeRegistry, mockGoose)

	// Create session and generate recommendations via message
	session := s.interactionManager.CreateSession()
	_, msgErr := s.interactionManager.SendMessage(context.Background(), session.SessionID, "Book a hotel", nil)
	require.NoError(t, msgErr)

	// Deploy
	payload := InteractDeployRequest{
		RecommendationID: "rec-1",
	}
	body, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", "/api/v1/interact/"+session.SessionID+"/deploy", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp InteractDeployResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.NotEmpty(t, resp.ExecutionID)
	assert.Len(t, resp.WantIDs, 1)
	assert.Equal(t, "deployed", resp.Status)
	assert.Contains(t, resp.Message, "Successfully deployed")
}

// TestInteractDeploy_WithModifications verifies deployment with parameter modifications
func TestInteractDeploy_WithModifications(t *testing.T) {
	s := setupTestServer()

	// Override with mock that returns multiple wants
	mockGoose := &mockGooseExecutor{
		recommendations: []mywant.Recommendation{
			{
				ID:       "rec-1",
				Title:    "Test",
				Approach: "custom",
				Config: mywant.Config{
					Wants: []*mywant.Want{
						{
							Metadata: mywant.Metadata{Name: "queue1", Type: "qnet queue"},
							Spec:     mywant.WantSpec{Params: map[string]interface{}{"service_time": 0.1}},
						},
						{
							Metadata: mywant.Metadata{Name: "queue2", Type: "qnet queue"},
							Spec:     mywant.WantSpec{Params: map[string]interface{}{"service_time": 0.2}},
						},
					},
				},
			},
		},
	}
	s.interactionManager = mywant.NewInteractionManager(s.wantTypeLoader, s.recipeRegistry, mockGoose)

	// Create session and generate recommendations
	session := s.interactionManager.CreateSession()
	_, msgErr := s.interactionManager.SendMessage(context.Background(), session.SessionID, "Book hotels", nil)
	require.NoError(t, msgErr)

	// Deploy with modifications
	payload := InteractDeployRequest{
		RecommendationID: "rec-1",
		Modifications: &ConfigModifications{
			ParameterOverrides: map[string]interface{}{
				"service_time": 0.15,
			},
			DisableWants: []string{"queue2"},
		},
	}
	body, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", "/api/v1/interact/"+session.SessionID+"/deploy", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp InteractDeployResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	// Only 1 want should be deployed (hotel2 was disabled)
	assert.Len(t, resp.WantIDs, 1)
}

// TestInteractDeploy_RecommendationNotFound verifies error handling
func TestInteractDeploy_RecommendationNotFound(t *testing.T) {
	s := setupTestServer()

	session := s.interactionManager.CreateSession()

	payload := InteractDeployRequest{
		RecommendationID: "rec-999",
	}
	body, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", "/api/v1/interact/"+session.SessionID+"/deploy", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "not found")
}

// TestInteractDeploy_EmptyRecommendationID verifies validation
func TestInteractDeploy_EmptyRecommendationID(t *testing.T) {
	s := setupTestServer()

	session := s.interactionManager.CreateSession()

	payload := InteractDeployRequest{
		RecommendationID: "",
	}
	body, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", "/api/v1/interact/"+session.SessionID+"/deploy", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "required")
}

// TestInteractConversationalFlow verifies conversation history preservation
func TestInteractConversationalFlow(t *testing.T) {
	s := setupTestServer()

	// Setup mock
	mockGoose := &mockGooseExecutor{
		recommendations: []mywant.Recommendation{
			{ID: "rec-1", Title: "Test", Approach: "custom", Config: mywant.Config{Wants: []*mywant.Want{}}},
		},
	}
	s.interactionManager = mywant.NewInteractionManager(s.wantTypeLoader, s.recipeRegistry, mockGoose)

	// Create session
	session := s.interactionManager.CreateSession()

	// First message
	payload1 := InteractMessageRequest{Message: "I want to travel to Tokyo"}
	body1, _ := json.Marshal(payload1)
	req1, _ := http.NewRequest("POST", "/api/v1/interact/"+session.SessionID, bytes.NewBuffer(body1))
	req1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()
	s.router.ServeHTTP(w1, req1)
	assert.Equal(t, http.StatusOK, w1.Code)

	var resp1 InteractMessageResponse
	json.Unmarshal(w1.Body.Bytes(), &resp1)
	assert.Len(t, resp1.ConversationHistory, 2) // user + assistant

	// Second message
	payload2 := InteractMessageRequest{Message: "Add a hotel"}
	body2, _ := json.Marshal(payload2)
	req2, _ := http.NewRequest("POST", "/api/v1/interact/"+session.SessionID, bytes.NewBuffer(body2))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	s.router.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusOK, w2.Code)

	var resp2 InteractMessageResponse
	json.Unmarshal(w2.Body.Bytes(), &resp2)
	assert.Len(t, resp2.ConversationHistory, 4) // 2 user + 2 assistant

	// Verify order
	assert.Equal(t, "user", resp2.ConversationHistory[0].Role)
	assert.Equal(t, "I want to travel to Tokyo", resp2.ConversationHistory[0].Content)
	assert.Equal(t, "assistant", resp2.ConversationHistory[1].Role)
	assert.Equal(t, "user", resp2.ConversationHistory[2].Role)
	assert.Equal(t, "Add a hotel", resp2.ConversationHistory[2].Content)
	assert.Equal(t, "assistant", resp2.ConversationHistory[3].Role)
}

// TestInteractMultipleSessions verifies session isolation
func TestInteractMultipleSessions(t *testing.T) {
	s := setupTestServer()

	// Create two sessions
	req1, _ := http.NewRequest("POST", "/api/v1/interact", nil)
	w1 := httptest.NewRecorder()
	s.router.ServeHTTP(w1, req1)
	var session1 InteractCreateResponse
	json.Unmarshal(w1.Body.Bytes(), &session1)

	req2, _ := http.NewRequest("POST", "/api/v1/interact", nil)
	w2 := httptest.NewRecorder()
	s.router.ServeHTTP(w2, req2)
	var session2 InteractCreateResponse
	json.Unmarshal(w2.Body.Bytes(), &session2)

	// Verify they're different
	assert.NotEqual(t, session1.SessionID, session2.SessionID)

	// Both should be retrievable
	s1, err1 := s.interactionManager.GetSession(session1.SessionID)
	assert.NoError(t, err1)
	assert.Equal(t, session1.SessionID, s1.SessionID)

	s2, err2 := s.interactionManager.GetSession(session2.SessionID)
	assert.NoError(t, err2)
	assert.Equal(t, session2.SessionID, s2.SessionID)
}
