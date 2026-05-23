package mywant

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

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
	Wants       []*Want            `json:"wants"`       // Complete list of wants
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
