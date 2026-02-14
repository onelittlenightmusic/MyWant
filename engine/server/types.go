package server

import (
	"net/http"

	mywant "mywant/engine/core"
)

// Config holds server configuration
type Config struct {
	Port           int             `json:"port"`
	Host           string          `json:"host"`
	Debug          bool            `json:"debug"`
	HeaderPosition string          `json:"header_position"`
	ColorMode      string          `json:"color_mode"`
	WebFS          http.FileSystem `json:"-"` // Embedded web assets filesystem (injected by caller)
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
	WantID   string                       `json:"wantId"`
	Metadata mywant.GenericRecipeMetadata `json:"metadata"`
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
