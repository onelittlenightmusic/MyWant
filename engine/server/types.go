package server

import (
	"net/http"

	mywant "mywant/engine/core"
)

// Config holds server configuration
type Config struct {
	Port           int             `json:"port" yaml:"port"`
	Host           string          `json:"host" yaml:"host"`
	Debug          bool            `json:"debug" yaml:"debug"`
	HeaderPosition string          `json:"header_position" yaml:"header_position"`
	ColorMode      string          `json:"color_mode" yaml:"color_mode"`
	ConfigPath     string          `json:"config_path" yaml:"config_path"`
	MemoryPath     string          `json:"memory_path" yaml:"memory_path"`
	WantTypesDir   string          `json:"want_types_dir" yaml:"want_types_dir"`
	WebFS          http.FileSystem `json:"-" yaml:"-"`
	OTELEndpoint   string          `json:"otel_endpoint" yaml:"otel_endpoint"`
	GoalThinker    GoalThinkerConfig `json:"goal_thinker" yaml:"goal_thinker"`
}

type GoalThinkerConfig struct {
	UseStub bool `json:"use_stub" yaml:"use_stub"`
}

type ErrorHistoryEntry struct {
	ID          string `json:"id"`
	Timestamp   string `json:"timestamp"`
	Message     string `json:"message"`
	Status      int    `json:"status"`
	Code        string `json:"code,omitempty"`
	Type        string `json:"type,omitempty"`
	Details     string `json:"details,omitempty"`
	Endpoint    string `json:"endpoint"`
	Method      string `json:"method"`
	RequestData any    `json:"request_data,omitempty"`
	UserAgent   string `json:"user_agent,omitempty"`
	Resolved    bool   `json:"resolved"`
	Notes       string `json:"notes,omitempty"`
}

// WantExecution represents a running want execution
type WantExecution struct {
	ID      string               `json:"id"`
	Config  mywant.Config        `json:"config"`
	Status  string               `json:"status"`
	Results map[string]any       `json:"results,omitempty"`
	Builder *mywant.ChainBuilder `json:"-"` // Don't serialize builder
}

// LLMRequest represents a request to the LLM inference API
type LLMRequest struct {
	Message string `json:"message"`
	Model   string `json:"model,omitempty"`
}

// LLMResponse represents a response from the LLM inference API
type LLMResponse struct {
	Response  string `json:"response"`
	Model     string `json:"model"`
	Timestamp string `json:"timestamp"`
}

// OllamaRequest represents the request format for Ollama API
type OllamaRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

// OllamaResponse represents the response format from Ollama API
type OllamaResponse struct {
	Model              string `json:"model"`
	CreatedAt          string `json:"created_at"`
	Response           string `json:"response"`
	Done               bool   `json:"done"`
	DoneReason         string `json:"done_reason,omitempty"`
	Context            []int  `json:"context,omitempty"`
	TotalDuration      int64  `json:"total_duration,omitempty"`
	LoadDuration       int64  `json:"load_duration,omitempty"`
	PromptEvalCount    int    `json:"prompt_eval_count,omitempty"`
	PromptEvalDuration int64  `json:"prompt_eval_duration,omitempty"`
	EvalCount          int    `json:"eval_count,omitempty"`
	EvalDuration       int64  `json:"eval_duration,omitempty"`
}

// SaveRecipeFromWantRequest represents the request to create a recipe from an existing want
type SaveRecipeFromWantRequest struct {
	WantID     string                       `json:"wantId"`
	Metadata   mywant.GenericRecipeMetadata `json:"metadata"`
	State      []mywant.StateDef            `json:"state,omitempty"`
	Parameters map[string]any               `json:"parameters,omitempty"`
}

// WantRecipeAnalysis represents the analysis result for recipe creation
type WantRecipeAnalysis struct {
	WantID            string                       `json:"wantId"`
	ChildCount        int                          `json:"childCount"`
	RecommendedState  []mywant.StateDef            `json:"recommendedState"`
	SuggestedMetadata mywant.GenericRecipeMetadata `json:"suggestedMetadata"`
}

// ValidationResult represents the complete validation response
type ValidationResult struct {
	Valid       bool                `json:"valid"`
	FatalErrors []ValidationError   `json:"fatalErrors"`
	Warnings    []ValidationWarning `json:"warnings"`
	WantCount   int                 `json:"wantCount"`
	ValidatedAt string              `json:"validatedAt"`
}

// ValidationError represents a fatal validation error
type ValidationError struct {
	WantName  string `json:"wantName,omitempty"`
	ErrorType string `json:"errorType"`
	Field     string `json:"field,omitempty"`
	Message   string `json:"message"`
	Details   string `json:"details,omitempty"`
}

// ValidationWarning represents a non-fatal issue
type ValidationWarning struct {
	WantName     string   `json:"wantName"`
	WarningType  string   `json:"warningType"`
	Field        string   `json:"field,omitempty"`
	Message      string   `json:"message"`
	Suggestion   string   `json:"suggestion,omitempty"`
	RelatedWants []string `json:"relatedWants,omitempty"`
}

// ReactionRequest represents a user reaction to a reminder
type ReactionRequest struct {
	Approved bool   `json:"approved"`
	Comment  string `json:"comment,omitempty"`
}

// WantStateSnapshot is a state snapshot for a single Want used in cross-want state API responses.
type WantStateSnapshot struct {
	WantID   string            `json:"want_id"`
	WantName string            `json:"want_name"`
	State    hierarchicalState `json:"state"`
}

// StatesListResponse is the response for GET /api/v1/states.
type StatesListResponse struct {
	Wants       []WantStateSnapshot `json:"wants"`
	GlobalState map[string]any      `json:"global_state,omitempty"`
	Total       int                 `json:"total"`
}

// StateSearchResult is a single field match in the cross-want state search.
type StateSearchResult struct {
	WantID   string `json:"want_id,omitempty"`
	WantName string `json:"want_name,omitempty"`
	Field    string `json:"field"`
	Value    any    `json:"value"`
	Label    string `json:"label"`  // "current", "goal", "plan", "none"
	Source   string `json:"source"` // "want" or "global"
}

// StateSearchResponse is the response for GET /api/v1/states/search.
type StateSearchResponse struct {
	Field   string              `json:"field"`
	Results []StateSearchResult `json:"results"`
	Total   int                 `json:"total"`
}

