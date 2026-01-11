package client

import "time"

// WantExecution represents a want execution returned by the API
type WantExecution struct {
	ID      string         `json:"id"`
	Config  Config         `json:"config"`
	Status  string         `json:"status"`
	Results map[string]any `json:"results,omitempty"`
}

// Config represents the want configuration
type Config struct {
	Wants []*Want `json:"wants"`
}

// Want represents a single want definition
type Want struct {
	Metadata Metadata       `json:"metadata"`
	Spec     WantSpec       `json:"spec"`
	Status   string         `json:"status,omitempty"`
	State    map[string]any `json:"state,omitempty"`
}

// Metadata represents want metadata
type Metadata struct {
	ID     string            `json:"id,omitempty"`
	Name   string            `json:"name"`
	Type   string            `json:"type"`
	Labels map[string]string `json:"labels,omitempty"`
}

// WantSpec represents want specification
type WantSpec struct {
	Params map[string]any      `json:"params"`
	Using  []map[string]string `json:"using,omitempty"`
}

// CreateWantResponse represents the response from creating a want
type CreateWantResponse struct {
	ID      string   `json:"id"`
	Status  string   `json:"status"`
	Wants   int      `json:"wants"`
	WantIDs []string `json:"want_ids"`
	Message string   `json:"message"`
}

type ImportWantsResponse struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Wants   int    `json:"wants"`
	Message string `json:"message"`
}

// ValidationResult represents validation response
type ValidationResult struct {
	Valid       bool                `json:"valid"`
	FatalErrors []ValidationError   `json:"fatalErrors"`
	Warnings    []ValidationWarning `json:"warnings"`
	WantCount   int                 `json:"wantCount"`
	ValidatedAt string              `json:"validatedAt"`
}

type ValidationError struct {
	WantName  string `json:"wantName,omitempty"`
	ErrorType string `json:"errorType"`
	Field     string `json:"field,omitempty"`
	Message   string `json:"message"`
	Details   string `json:"details,omitempty"`
}

type ValidationWarning struct {
	WantName    string `json:"wantName"`
	WarningType string `json:"warningType"`
	Field       string `json:"field,omitempty"`
	Message     string `json:"message"`
	Suggestion  string `json:"suggestion,omitempty"`
}

// APIDumpResponse represents the response from list wants (dump format)
type APIDumpResponse struct {
	Timestamp   time.Time `json:"timestamp"`
	ExecutionID string    `json:"execution_id"`
	Wants       []*Want   `json:"wants"`
}

// GenericRecipe represents a recipe structure
type GenericRecipe struct {
	Recipe RecipeContent `json:"recipe"`
}

type RecipeContent struct {
	Metadata   RecipeMetadata `json:"metadata"`
	Parameters map[string]any `json:"parameters,omitempty"`
	Wants      []any          `json:"wants"` // Simplified for now
	Example    *Config        `json:"example,omitempty"`
}

type RecipeMetadata struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Version     string `json:"version,omitempty"`
	CustomType  string `json:"custom_type,omitempty"`
}

type SaveRecipeFromWantRequest struct {
	WantID   string         `json:"wantId"`
	Metadata RecipeMetadata `json:"metadata"`
}

type SaveRecipeResponse struct {
	ID      string `json:"id"`
	Message string `json:"message"`
	File    string `json:"file"`
	Wants   int    `json:"wants"`
}

type Agent struct {
	Name         string   `json:"name"`
	Type         string   `json:"type"`
	Capabilities []string `json:"capabilities"`
}

type Capability struct {
	Name string `json:"name"`
}

type WantType struct {
	Name     string `json:"name"`
	Title    string `json:"title"`
	Category string `json:"category"`
	Version  string `json:"version"`
	Pattern  string `json:"pattern"`
}

type WantTypeListResponse struct {
	Count     int        `json:"count"`
	WantTypes []WantType `json:"wantTypes"`
}

type APILogEntry struct {
	Timestamp  string `json:"timestamp"`
	Method     string `json:"method"`
	Endpoint   string `json:"endpoint"`
	Resource   string `json:"resource"`
	Status     string `json:"status"`
	StatusCode int    `json:"statusCode"`
	Details    string `json:"details"`
}

type APILogsResponse struct {
	Count     int           `json:"count"`
	Logs      []APILogEntry `json:"logs"`
	Timestamp string        `json:"timestamp"`
}

// InteractCreateResponse represents the response from creating a new session
type InteractCreateResponse struct {
	SessionID string    `json:"session_id"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// InteractMessageRequest represents a message sent to an interaction session
type InteractMessageRequest struct {
	Message string           `json:"message"`
	Context *InteractContext `json:"context,omitempty"`
}

// InteractContext provides optional context for message processing
type InteractContext struct {
	PreferRecipes bool     `json:"preferRecipes"`
	Categories    []string `json:"categories,omitempty"`
}

// InteractMessageResponse represents the response with recommendations
type InteractMessageResponse struct {
	Recommendations     []Recommendation      `json:"recommendations"`
	ConversationHistory []ConversationMessage `json:"conversation_history"`
	Timestamp           time.Time             `json:"timestamp"`
}

// ConversationMessage represents a single message in the conversation
type ConversationMessage struct {
	Role      string    `json:"role"` // "user" | "assistant"
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

// Recommendation represents a recommended want configuration
type Recommendation struct {
	ID          string             `json:"id"` // rec-1, rec-2, rec-3
	Title       string             `json:"title"`
	Approach    string             `json:"approach"` // "recipe" | "custom" | "hybrid"
	Description string             `json:"description"`
	Config      Config             `json:"config"`
	Metadata    RecommendationMeta `json:"metadata"`
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

// InteractDeployRequest represents a request to deploy a recommendation
type InteractDeployRequest struct {
	RecommendationID string               `json:"recommendation_id"`
	Modifications    *ConfigModifications `json:"modifications,omitempty"`
}

// ConfigModifications allows modifying a recommendation before deployment
type ConfigModifications struct {
	ParameterOverrides map[string]any `json:"parameterOverrides,omitempty"`
	DisableWants       []string       `json:"disableWants,omitempty"`
}

// InteractDeployResponse represents the response from deploying a recommendation
type InteractDeployResponse struct {
	ExecutionID string    `json:"execution_id"`
	WantIDs     []string  `json:"want_ids"`
	Status      string    `json:"status"`
	Message     string    `json:"message"`
	Timestamp   time.Time `json:"timestamp"`
}
