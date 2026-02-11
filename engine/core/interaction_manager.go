package mywant

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"sync"
	"time"
)

// GooseExecutor is an interface for executing Goose operations
type GooseExecutor interface {
	ExecuteViaGoose(ctx context.Context, operation string, params map[string]interface{}) (interface{}, error)
}

// InteractionManager orchestrates interactive want creation sessions
type InteractionManager struct {
	sessionCache   *SessionCache
	wantTypeLoader *WantTypeLoader
	recipeRegistry *CustomTargetTypeRegistry
	gooseExecutor  GooseExecutor
}

// InteractContext provides optional context for message processing
type InteractContext struct {
	PreferRecipes bool     `json:"preferRecipes,omitempty"`
	Categories    []string `json:"categories,omitempty"`
	Provider      string   `json:"provider,omitempty"`
}

// InteractionSession represents a conversational session for want creation
type InteractionSession struct {
	SessionID           string                `json:"session_id"`
	ConversationHistory []ConversationMessage `json:"conversation_history"`
	Recommendations     []Recommendation      `json:"recommendations"`
	CreatedAt           time.Time             `json:"created_at"`
	ExpiresAt           time.Time             `json:"expires_at"`
	LastAccessedAt      time.Time             `json:"last_accessed_at"`
}

// ConversationMessage represents a single message in the conversation
type ConversationMessage struct {
	Role      string    `json:"role"`      // "user" | "assistant"
	Content   string    `json:"content"`   // Message text
	Timestamp time.Time `json:"timestamp"` // When message was sent
}

// Recommendation represents a recommended want configuration
type Recommendation struct {
	ID          string             `json:"id"`          // rec-1, rec-2, rec-3
	Title       string             `json:"title"`       // User-friendly title
	Approach    string             `json:"approach"`    // "recipe" | "custom" | "hybrid"
	Description string             `json:"description"` // Why this fits the user's need
	Config      Config             `json:"config"`      // Complete YAML structure
	Metadata    RecommendationMeta `json:"metadata"`    // Additional info
}

// RecommendationMeta contains metadata about a recommendation
type RecommendationMeta struct {
	WantCount     int      `json:"want_count"`
	RecipesUsed   []string `json:"recipes_used,omitempty"`
	WantTypesUsed []string `json:"want_types_used"`
	Complexity    string   `json:"complexity"` // "low" | "medium" | "high"
	ProsCons      ProsCons `json:"pros_cons"`
}

// ProsCons lists pros and cons of a recommendation
type ProsCons struct {
	Pros []string `json:"pros"`
	Cons []string `json:"cons"`
}

// SessionCache manages interaction sessions with thread-safe access
type SessionCache struct {
	mu       sync.RWMutex
	sessions map[string]*InteractionSession
	cleanup  *time.Ticker
	done     chan bool
}

// NewInteractionManager creates a new interaction manager
func NewInteractionManager(loader *WantTypeLoader, registry *CustomTargetTypeRegistry, gooseExecutor GooseExecutor) *InteractionManager {
	im := &InteractionManager{
		sessionCache:   NewSessionCache(),
		wantTypeLoader: loader,
		recipeRegistry: registry,
		gooseExecutor:  gooseExecutor,
	}

	// Start cleanup goroutine
	go im.sessionCache.startCleanup(5 * time.Minute)

	return im
}

// NewSessionCache creates a new session cache
func NewSessionCache() *SessionCache {
	return &SessionCache{
		sessions: make(map[string]*InteractionSession),
		done:     make(chan bool),
	}
}

// CreateSession creates a new interaction session
func (sc *SessionCache) CreateSession() *InteractionSession {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	sessionID := generateSessionID()
	now := time.Now()

	session := &InteractionSession{
		SessionID:           sessionID,
		ConversationHistory: []ConversationMessage{},
		Recommendations:     []Recommendation{},
		CreatedAt:           now,
		ExpiresAt:           now.Add(30 * time.Minute),
		LastAccessedAt:      now,
	}

	sc.sessions[sessionID] = session
	return session
}

// GetSession retrieves a session by ID
func (sc *SessionCache) GetSession(sessionID string) (*InteractionSession, error) {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	session, exists := sc.sessions[sessionID]
	if !exists {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	if time.Now().After(session.ExpiresAt) {
		return nil, fmt.Errorf("session expired: %s", sessionID)
	}

	// Update last accessed time (note: this modifies the session in-place)
	session.LastAccessedAt = time.Now()
	return session, nil
}

// UpdateSession updates a session's conversation history and recommendations
func (sc *SessionCache) UpdateSession(sessionID string, history []ConversationMessage, recommendations []Recommendation) error {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	session, exists := sc.sessions[sessionID]
	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	if time.Now().After(session.ExpiresAt) {
		return fmt.Errorf("session expired: %s", sessionID)
	}

	session.ConversationHistory = history
	session.Recommendations = recommendations
	session.LastAccessedAt = time.Now()

	return nil
}

// GetRecommendation retrieves a specific recommendation from a session
func (sc *SessionCache) GetRecommendation(sessionID, recID string) (*Recommendation, error) {
	session, err := sc.GetSession(sessionID)
	if err != nil {
		return nil, err
	}

	for _, rec := range session.Recommendations {
		if rec.ID == recID {
			return &rec, nil
		}
	}

	return nil, fmt.Errorf("recommendation not found: %s", recID)
}

// DeleteSession removes a session from the cache
func (sc *SessionCache) DeleteSession(sessionID string) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	delete(sc.sessions, sessionID)
}

// startCleanup periodically removes expired sessions
func (sc *SessionCache) startCleanup(interval time.Duration) {
	sc.cleanup = time.NewTicker(interval)
	defer sc.cleanup.Stop()

	for {
		select {
		case <-sc.cleanup.C:
			sc.cleanupExpired()
		case <-sc.done:
			return
		}
	}
}

// cleanupExpired removes all expired sessions
func (sc *SessionCache) cleanupExpired() {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	now := time.Now()
	for id, session := range sc.sessions {
		if now.After(session.ExpiresAt) {
			delete(sc.sessions, id)
		}
	}
}

// Stop stops the cleanup goroutine
func (sc *SessionCache) Stop() {
	close(sc.done)
}

// CreateSession creates a new interaction session
func (im *InteractionManager) CreateSession() *InteractionSession {
	return im.sessionCache.CreateSession()
}

// SendMessage processes a user message and returns recommendations
func (im *InteractionManager) SendMessage(ctx context.Context, sessionID string, message string, interactCtx *InteractContext) (*InteractionSession, error) {
	// Get existing session
	session, err := im.sessionCache.GetSession(sessionID)
	if err != nil {
		return nil, err
	}

	// Add user message to conversation history
	userMessage := ConversationMessage{
		Role:      "user",
		Content:   message,
		Timestamp: time.Now(),
	}
	session.ConversationHistory = append(session.ConversationHistory, userMessage)

	// Generate recommendations via Goose
	recommendations, err := im.generateRecommendations(ctx, session.ConversationHistory, interactCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to generate recommendations: %w", err)
	}

	// Add assistant response to conversation
	assistantMessage := ConversationMessage{
		Role:      "assistant",
		Content:   fmt.Sprintf("Generated %d recommendations", len(recommendations)),
		Timestamp: time.Now(),
	}
	session.ConversationHistory = append(session.ConversationHistory, assistantMessage)

	// Update session with new history and recommendations
	err = im.sessionCache.UpdateSession(
		sessionID,
		session.ConversationHistory,
		recommendations,
	)
	if err != nil {
		return nil, err
	}

	// Return updated session
	return im.sessionCache.GetSession(sessionID)
}

// generateRecommendations calls GooseManager to generate recommendations
func (im *InteractionManager) generateRecommendations(ctx context.Context, history []ConversationMessage, interactCtx *InteractContext) ([]Recommendation, error) {
	// Get the latest user message
	var latestMessage string
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role == "user" {
			latestMessage = history[i].Content
			break
		}
	}

	// Serialize conversation history for Goose
	historyJSON, err := json.Marshal(history)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal conversation history: %w", err)
	}

	params := map[string]interface{}{
		"message":              latestMessage,
		"conversation_history": string(historyJSON),
	}

	// Add context info if provided
	if interactCtx != nil {
		if interactCtx.Provider != "" {
			params["provider"] = interactCtx.Provider
		}
		if len(interactCtx.Categories) > 0 {
			params["categories"] = interactCtx.Categories
		}
		params["preferRecipes"] = interactCtx.PreferRecipes
	}

	// Execute via Goose with MCP-based prompt
	result, err := im.gooseExecutor.ExecuteViaGoose(ctx, "interact_recommend", params)
	if err != nil {
		return nil, fmt.Errorf("Goose execution failed: %w", err)
	}

	// Parse recommendations from result
	recommendations, err := parseRecommendationsFromGoose(result)
	if err != nil {
		return nil, fmt.Errorf("failed to parse recommendations: %w", err)
	}

	return recommendations, nil
}

// parseRecommendationsFromGoose extracts recommendations from Goose response
func parseRecommendationsFromGoose(result interface{}) ([]Recommendation, error) {
	// Try to parse as map first
	resultMap, ok := result.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected result type: %T", result)
	}

	// Look for "recommendations" key
	recsInterface, ok := resultMap["recommendations"]
	if !ok {
		// If "text" exists, it might be an error message from Goose
		if text, ok := resultMap["text"].(string); ok {
			return nil, fmt.Errorf("no recommendations found in result. AI response: %s", text)
		}
		return nil, fmt.Errorf("no recommendations found in result (keys found: %v)", reflect.ValueOf(resultMap).MapKeys())
	}

	// Fix "using" field format before unmarshaling
	fixedRecsInterface := fixUsingFieldFormat(recsInterface)

	// Convert to JSON and back to parse properly
	recsJSON, err := json.Marshal(fixedRecsInterface)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal recommendations: %w", err)
	}

	var recommendations []Recommendation
	if err := json.Unmarshal(recsJSON, &recommendations); err != nil {
		return nil, fmt.Errorf("failed to unmarshal recommendations: %w", err)
	}

	return recommendations, nil
}

// fixUsingFieldFormat recursively fixes "using" field format in recommendations
// Converts object {"key": "value"} to array [{"key": "value"}]
func fixUsingFieldFormat(data interface{}) interface{} {
	switch v := data.(type) {
	case map[string]interface{}:
		// Recursively fix nested maps
		fixed := make(map[string]interface{})
		for key, val := range v {
			if key == "using" {
				// Fix the "using" field
				fixed[key] = normalizeUsingField(val)
			} else {
				fixed[key] = fixUsingFieldFormat(val)
			}
		}
		return fixed
	case []interface{}:
		// Recursively fix array elements
		fixed := make([]interface{}, len(v))
		for i, item := range v {
			fixed[i] = fixUsingFieldFormat(item)
		}
		return fixed
	default:
		return data
	}
}

// normalizeUsingField ensures "using" is an array of maps
func normalizeUsingField(val interface{}) interface{} {
	switch v := val.(type) {
	case map[string]interface{}:
		// Single object -> wrap in array
		return []interface{}{v}
	case []interface{}:
		// Already an array, return as is
		return v
	case nil:
		// Null -> return empty array
		return []interface{}{}
	default:
		// Unknown type, return empty array
		return []interface{}{}
	}
}

// GetSession retrieves an existing session
func (im *InteractionManager) GetSession(sessionID string) (*InteractionSession, error) {
	return im.sessionCache.GetSession(sessionID)
}

// DeleteSession removes a session
func (im *InteractionManager) DeleteSession(sessionID string) {
	im.sessionCache.DeleteSession(sessionID)
}

// GetRecommendation retrieves a specific recommendation
func (im *InteractionManager) GetRecommendation(sessionID, recID string) (*Recommendation, error) {
	return im.sessionCache.GetRecommendation(sessionID, recID)
}

// generateSessionID creates a unique session ID
func generateSessionID() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return "sess-" + hex.EncodeToString(bytes)
}
