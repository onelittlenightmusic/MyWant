package server

import (
	"net/http"

	want_spec "github.com/onelittlenightmusic/want-spec"
	mywant "mywant/engine/core"
)

// Config holds server configuration
type Config struct {
	Port           int    `json:"port" yaml:"port"`
	Host           string `json:"host" yaml:"host"`
	Debug          bool   `json:"debug" yaml:"debug"`
	HeaderPosition string `json:"header_position" yaml:"header_position"`
	ColorMode      string `json:"color_mode" yaml:"color_mode"`
	CardHeight     string `json:"card_height" yaml:"card_height"`
	// CardOpacity is how strongly a card's category background layer (gradient
	// or image) is painted, 0.0–1.0. Pointer so an explicit 0 (fully flat
	// cards) is distinguishable from "not set", which falls back to 0.7.
	CardOpacity    *float64 `json:"card_opacity,omitempty" yaml:"card_opacity,omitempty"`
	SoundEnabled   *bool  `json:"sound_enabled" yaml:"sound_enabled"`
	IconFont       string `json:"icon_font,omitempty" yaml:"icon_font,omitempty"`
	CanvasBgURL    string `json:"canvas_bg_url,omitempty" yaml:"canvas_bg_url,omitempty"`
	// TunnelURL is the public URL captured from a managed_launch want (e.g.
	// cloudflared/ngrok) whose result_field is "tunnel_url" — see SetTunnelURL.
	TunnelURL            string `json:"tunnel_url,omitempty" yaml:"tunnel_url,omitempty"`
	CanvasBgColor        string `json:"canvas_bg_color,omitempty" yaml:"canvas_bg_color,omitempty"`
	CanvasDPad           *bool  `json:"canvas_dpad,omitempty" yaml:"canvas_dpad,omitempty"`
	CanvasWeatherEffect  string `json:"canvas_weather_effect,omitempty" yaml:"canvas_weather_effect,omitempty"`
	CanvasDesign         string `json:"canvas_design,omitempty" yaml:"canvas_design,omitempty"`
	// InteractionMode is "edit" (default, canvas tiles draggable) or "game"
	// (tile positions locked — see the game-mode barrier in updateWant and
	// the label-mutation endpoints in handlers_modifications.go).
	InteractionMode string `json:"interaction_mode,omitempty" yaml:"interaction_mode,omitempty"`
	// CurrentWorld is the name of the currently-open world snapshot
	// (~/.mywant/worlds/<name>.yaml) — see handlers_worlds.go. Empty means no
	// world has been opened yet (ad-hoc wants, not yet snapshotted).
	CurrentWorld         string `json:"current_world,omitempty" yaml:"current_world,omitempty"`
	ActiveLocationDevice string `json:"active_location_device,omitempty" yaml:"active_location_device,omitempty"`
	LocationWantId       string `json:"location_want_id,omitempty" yaml:"location_want_id,omitempty"`
	// WebInspectorLANHost is the mywant server's LAN-reachable address (host
	// only, no scheme/port) — needed because a phone on the same Wi-Fi can't
	// use "localhost" (that resolves to the phone itself). User-confirmed/
	// editable; DetectedLANIP below is only an unconfirmed suggestion.
	WebInspectorLANHost string `json:"web_inspector_lan_host,omitempty" yaml:"web_inspector_lan_host,omitempty"`
	// WebInspectorCACertPath is the filesystem path to Caddy's internal CA
	// root cert (see mywant-gui/docs/WebInspectorIPhone.md) — served at
	// GET /api/v1/web-wants/ca-cert so it can be downloaded straight from an
	// iPhone's own Safari instead of AirDropped from the Mac.
	WebInspectorCACertPath string `json:"web_inspector_ca_cert_path,omitempty" yaml:"web_inspector_ca_cert_path,omitempty"`
	// WebInspectorExternalHost is a public hostname (no scheme/port) that
	// reaches this machine from outside the LAN — e.g. a Cloudflare Tunnel
	// hostname. Unlike WebInspectorLANHost, TLS is terminated at the tunnel
	// provider's edge with a publicly-trusted cert, so WebInspectorCACertPath
	// doesn't apply here.
	WebInspectorExternalHost string `json:"web_inspector_external_host,omitempty" yaml:"web_inspector_external_host,omitempty"`
	// HTTPSPath is a certificate-confirmed https:// origin (e.g.
	// "https://localhost:8443") auto-persisted by a local reverse-proxy want
	// (e.g. Caddy) once it confirms the process is running — see SetHTTPSPath.
	HTTPSPath string `json:"https_path,omitempty" yaml:"https_path,omitempty"`
	// DetectedLANIP is computed fresh on every GET /api/v1/config (see
	// detectLANIP in handlers_others.go) — never persisted (yaml:"-") and
	// never accepted from PUT (updateConfig doesn't read it back).
	DetectedLANIP string            `json:"detected_lan_ip,omitempty" yaml:"-"`
	ConfigPath    string            `json:"config_path" yaml:"config_path"`
	MemoryPath    string            `json:"memory_path" yaml:"memory_path"`
	WantTypesDir  string            `json:"want_types_dir" yaml:"want_types_dir"`
	WebFS         http.FileSystem   `json:"-" yaml:"-"`
	OTELEndpoint  string            `json:"otel_endpoint" yaml:"otel_endpoint"`
	GoalThinker   GoalThinkerConfig `json:"goal_thinker" yaml:"goal_thinker"`
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
	ID      string            `json:"id"`
	Wants   []*want_spec.Want `json:"wants"` // DTO snapshot of wants in this execution
	Status  string            `json:"status"`
	Results map[string]any    `json:"results,omitempty"`
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

// ClusterMember represents a single want in a cluster, with its canvas position.
type ClusterMember struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	X    int    `json:"x"`
	Y    int    `json:"y"`
}

// ClusterResponse is the response for GET /api/v1/wants/{id}/cluster.
// Members includes the root want itself.
type ClusterResponse struct {
	RootID  string          `json:"rootId"`
	Members []ClusterMember `json:"members"`
}
