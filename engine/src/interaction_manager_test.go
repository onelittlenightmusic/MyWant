package mywant

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockGooseExecutor for testing
type MockGooseExecutor struct {
	ExecuteFunc func(ctx context.Context, operation string, params map[string]interface{}) (interface{}, error)
}

func (m *MockGooseExecutor) ExecuteViaGoose(ctx context.Context, operation string, params map[string]interface{}) (interface{}, error) {
	if m.ExecuteFunc != nil {
		return m.ExecuteFunc(ctx, operation, params)
	}
	// Default mock response
	return map[string]interface{}{
		"recommendations": []map[string]interface{}{
			{
				"id":          "rec-1",
				"title":       "Test Recommendation",
				"approach":    "custom",
				"description": "Test description",
				"config": map[string]interface{}{
					"wants": []interface{}{},
				},
				"metadata": map[string]interface{}{
					"want_count":      1,
					"want_types_used": []string{"hotel"},
					"complexity":      "low",
					"pros_cons": map[string]interface{}{
						"pros": []string{"Fast"},
						"cons": []string{"Limited"},
					},
				},
			},
		},
	}, nil
}

// TestSessionCache_CreateSession verifies session creation
func TestSessionCache_CreateSession(t *testing.T) {
	cache := NewSessionCache()

	session := cache.CreateSession()

	assert.NotEmpty(t, session.SessionID)
	assert.True(t, len(session.SessionID) > 10, "Session ID should be sufficiently long")
	assert.Contains(t, session.SessionID, "sess-", "Session ID should have sess- prefix")
	assert.NotZero(t, session.CreatedAt)
	assert.NotZero(t, session.ExpiresAt)

	// Verify 30 minute expiry
	duration := session.ExpiresAt.Sub(session.CreatedAt)
	assert.Equal(t, 30*time.Minute, duration, "Session should expire in 30 minutes")

	// Verify empty history
	assert.Empty(t, session.ConversationHistory)
	assert.Empty(t, session.Recommendations)
}

// TestSessionCache_GetSession verifies session retrieval
func TestSessionCache_GetSession(t *testing.T) {
	cache := NewSessionCache()

	// Create session
	created := cache.CreateSession()

	// Retrieve session
	retrieved, err := cache.GetSession(created.SessionID)
	require.NoError(t, err)
	assert.Equal(t, created.SessionID, retrieved.SessionID)
	assert.Equal(t, created.CreatedAt, retrieved.CreatedAt)
	assert.Equal(t, created.ExpiresAt, retrieved.ExpiresAt)
}

// TestSessionCache_GetSession_NotFound verifies error handling for missing sessions
func TestSessionCache_GetSession_NotFound(t *testing.T) {
	cache := NewSessionCache()

	_, err := cache.GetSession("nonexistent-session-id")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// TestSessionCache_GetSession_Expired verifies expired session handling
func TestSessionCache_GetSession_Expired(t *testing.T) {
	cache := NewSessionCache()

	// Create session
	session := cache.CreateSession()

	// Manually expire it
	cache.mu.Lock()
	cache.sessions[session.SessionID].ExpiresAt = time.Now().Add(-1 * time.Minute)
	cache.mu.Unlock()

	_, err := cache.GetSession(session.SessionID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expired")
}

// TestSessionCache_UpdateSession verifies session updates
func TestSessionCache_UpdateSession(t *testing.T) {
	cache := NewSessionCache()

	// Create session
	session := cache.CreateSession()

	// Update with conversation and recommendations
	history := []ConversationMessage{
		{Role: "user", Content: "Hello", Timestamp: time.Now()},
	}
	recommendations := []Recommendation{
		{ID: "rec-1", Title: "Test", Approach: "custom"},
	}

	err := cache.UpdateSession(session.SessionID, history, recommendations)
	require.NoError(t, err)

	// Verify update
	updated, err := cache.GetSession(session.SessionID)
	require.NoError(t, err)
	assert.Len(t, updated.ConversationHistory, 1)
	assert.Len(t, updated.Recommendations, 1)
	assert.Equal(t, "Hello", updated.ConversationHistory[0].Content)
}

// TestSessionCache_DeleteSession verifies session deletion
func TestSessionCache_DeleteSession(t *testing.T) {
	cache := NewSessionCache()

	// Create session
	session := cache.CreateSession()

	// Verify it exists
	_, err := cache.GetSession(session.SessionID)
	require.NoError(t, err)

	// Delete it
	cache.DeleteSession(session.SessionID)

	// Verify it's gone
	_, err = cache.GetSession(session.SessionID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// TestSessionCache_GetRecommendation verifies recommendation retrieval
func TestSessionCache_GetRecommendation(t *testing.T) {
	cache := NewSessionCache()

	// Create session
	session := cache.CreateSession()

	// Add recommendations
	recommendations := []Recommendation{
		{ID: "rec-1", Title: "First", Approach: "custom"},
		{ID: "rec-2", Title: "Second", Approach: "recipe"},
	}
	cache.UpdateSession(session.SessionID, []ConversationMessage{}, recommendations)

	// Retrieve specific recommendation
	rec, err := cache.GetRecommendation(session.SessionID, "rec-2")
	require.NoError(t, err)
	assert.Equal(t, "rec-2", rec.ID)
	assert.Equal(t, "Second", rec.Title)
	assert.Equal(t, "recipe", rec.Approach)
}

// TestSessionCache_GetRecommendation_NotFound verifies error handling
func TestSessionCache_GetRecommendation_NotFound(t *testing.T) {
	cache := NewSessionCache()

	session := cache.CreateSession()
	recommendations := []Recommendation{
		{ID: "rec-1", Title: "First", Approach: "custom"},
	}
	cache.UpdateSession(session.SessionID, []ConversationMessage{}, recommendations)

	_, err := cache.GetRecommendation(session.SessionID, "rec-999")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// TestSessionCache_Cleanup verifies expired session cleanup
func TestSessionCache_Cleanup(t *testing.T) {
	cache := NewSessionCache()

	// Create two sessions
	session1 := cache.CreateSession()
	session2 := cache.CreateSession()

	// Expire first session
	cache.mu.Lock()
	cache.sessions[session1.SessionID].ExpiresAt = time.Now().Add(-1 * time.Minute)
	cache.mu.Unlock()

	// Run cleanup
	cache.cleanupExpired()

	// First session should be deleted
	_, err := cache.GetSession(session1.SessionID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	// Second session should still exist
	_, err = cache.GetSession(session2.SessionID)
	assert.NoError(t, err)
}

// TestInteractionManager_CreateSession verifies interaction manager session creation
func TestInteractionManager_CreateSession(t *testing.T) {
	loader := NewWantTypeLoader("../../want_types")
	registry := NewCustomTargetTypeRegistry()
	mockGoose := &MockGooseExecutor{}

	manager := NewInteractionManager(loader, registry, mockGoose)

	session := manager.CreateSession()

	assert.NotEmpty(t, session.SessionID)
	assert.Empty(t, session.ConversationHistory)
	assert.Empty(t, session.Recommendations)
	assert.NotZero(t, session.CreatedAt)
	assert.NotZero(t, session.ExpiresAt)
}

// TestInteractionManager_SendMessage verifies message sending and recommendation generation
func TestInteractionManager_SendMessage(t *testing.T) {
	loader := NewWantTypeLoader("../../want_types")
	registry := NewCustomTargetTypeRegistry()

	mockGoose := &MockGooseExecutor{
		ExecuteFunc: func(ctx context.Context, operation string, params map[string]interface{}) (interface{}, error) {
			// Verify operation
			assert.Equal(t, "interact_recommend", operation)

			// Verify params
			assert.NotNil(t, params["message"])
			assert.NotNil(t, params["conversation_history"])

			return map[string]interface{}{
				"recommendations": []map[string]interface{}{
					{
						"id":          "rec-1",
						"title":       "Mock Hotel Booking",
						"approach":    "custom",
						"description": "Mock description",
						"config": map[string]interface{}{
							"wants": []interface{}{},
						},
						"metadata": map[string]interface{}{
							"want_count":      1,
							"want_types_used": []string{"hotel"},
							"complexity":      "low",
							"pros_cons": map[string]interface{}{
								"pros": []string{"Fast"},
								"cons": []string{"Limited"},
							},
						},
					},
				},
			}, nil
		},
	}

	manager := NewInteractionManager(loader, registry, mockGoose)

	// Create session
	session := manager.CreateSession()

	// Send message
	updatedSession, err := manager.SendMessage(context.Background(), session.SessionID, "Book a hotel")
	require.NoError(t, err)

	// Verify conversation history
	assert.Len(t, updatedSession.ConversationHistory, 2) // user + assistant
	assert.Equal(t, "user", updatedSession.ConversationHistory[0].Role)
	assert.Equal(t, "Book a hotel", updatedSession.ConversationHistory[0].Content)
	assert.Equal(t, "assistant", updatedSession.ConversationHistory[1].Role)

	// Verify recommendations
	assert.Len(t, updatedSession.Recommendations, 1)
	assert.Equal(t, "rec-1", updatedSession.Recommendations[0].ID)
	assert.Equal(t, "Mock Hotel Booking", updatedSession.Recommendations[0].Title)
}

// TestInteractionManager_SendMessage_Conversational verifies conversation history preservation
func TestInteractionManager_SendMessage_Conversational(t *testing.T) {
	loader := NewWantTypeLoader("../../want_types")
	registry := NewCustomTargetTypeRegistry()
	mockGoose := &MockGooseExecutor{}

	manager := NewInteractionManager(loader, registry, mockGoose)

	// Create session
	session := manager.CreateSession()

	// First message
	session1, err := manager.SendMessage(context.Background(), session.SessionID, "I want to travel")
	require.NoError(t, err)
	assert.Len(t, session1.ConversationHistory, 2) // user + assistant

	// Second message
	session2, err := manager.SendMessage(context.Background(), session.SessionID, "Add a hotel")
	require.NoError(t, err)
	assert.Len(t, session2.ConversationHistory, 4) // 2 user + 2 assistant

	// Verify order
	assert.Equal(t, "user", session2.ConversationHistory[0].Role)
	assert.Equal(t, "I want to travel", session2.ConversationHistory[0].Content)
	assert.Equal(t, "assistant", session2.ConversationHistory[1].Role)
	assert.Equal(t, "user", session2.ConversationHistory[2].Role)
	assert.Equal(t, "Add a hotel", session2.ConversationHistory[2].Content)
	assert.Equal(t, "assistant", session2.ConversationHistory[3].Role)
}

// TestInteractionManager_GetSession verifies session retrieval
func TestInteractionManager_GetSession(t *testing.T) {
	loader := NewWantTypeLoader("../../want_types")
	registry := NewCustomTargetTypeRegistry()
	mockGoose := &MockGooseExecutor{}

	manager := NewInteractionManager(loader, registry, mockGoose)

	// Create session
	created := manager.CreateSession()

	// Retrieve session
	retrieved, err := manager.GetSession(created.SessionID)
	require.NoError(t, err)
	assert.Equal(t, created.SessionID, retrieved.SessionID)
}

// TestInteractionManager_DeleteSession verifies session deletion
func TestInteractionManager_DeleteSession(t *testing.T) {
	loader := NewWantTypeLoader("../../want_types")
	registry := NewCustomTargetTypeRegistry()
	mockGoose := &MockGooseExecutor{}

	manager := NewInteractionManager(loader, registry, mockGoose)

	// Create session
	session := manager.CreateSession()

	// Delete session
	manager.DeleteSession(session.SessionID)

	// Verify deletion
	_, err := manager.GetSession(session.SessionID)
	assert.Error(t, err)
}

// TestInteractionManager_GetRecommendation verifies recommendation retrieval
func TestInteractionManager_GetRecommendation(t *testing.T) {
	loader := NewWantTypeLoader("../../want_types")
	registry := NewCustomTargetTypeRegistry()
	mockGoose := &MockGooseExecutor{}

	manager := NewInteractionManager(loader, registry, mockGoose)
	session := manager.CreateSession()

	// Manually add a recommendation to the session
	rec := Recommendation{
		ID:       "rec-test",
		Title:    "Test Rec",
		Approach: "custom",
		Config:   Config{Wants: []*Want{}},
	}
	manager.sessionCache.UpdateSession(session.SessionID, []ConversationMessage{}, []Recommendation{rec})

	// Retrieve recommendation
	retrieved, err := manager.GetRecommendation(session.SessionID, "rec-test")
	require.NoError(t, err)
	assert.Equal(t, "rec-test", retrieved.ID)
	assert.Equal(t, "Test Rec", retrieved.Title)
}

// TestGenerateSessionID verifies session ID generation
func TestGenerateSessionID(t *testing.T) {
	id1 := generateSessionID()
	id2 := generateSessionID()

	// Should start with "sess-"
	assert.Contains(t, id1, "sess-")
	assert.Contains(t, id2, "sess-")

	// Should be unique
	assert.NotEqual(t, id1, id2)

	// Should be long enough (sess- + 32 hex chars)
	assert.True(t, len(id1) > 35)
}
