package mywant

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"mywant/engine/core/pubsub"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/attribute"
	otellog "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/trace"
	"gopkg.in/yaml.v3"
)

// TransportPacket wraps data sent over channels to include termination signals
type TransportPacket struct {
	Payload any
	Done    bool
}

// CachedPacket holds a packet and its original channel index for the caching mechanism.
type CachedPacket struct {
	Packet        any
	OriginalIndex int
}

// System-reserved state field names - automatically initialized and managed by the framework
// These fields don't need to be explicitly defined in want type YAML specifications
const (
	StateFieldActionByAgent    = "action_by_agent"      // Agent type that performed last action (MonitorAgent, DoAgent)
	StateFieldAchievingPercent = "achieving_percentage" // Progress percentage of want execution (0-100)
	StateFieldCompleted        = "completed"            // Whether the want has reached achieved status
)

// SystemReservedStateFields returns the list of state fields automatically managed by the framework
func SystemReservedStateFields() []string {
	return []string{
		StateFieldActionByAgent,
		StateFieldAchievingPercent,
		StateFieldCompleted,
		"final_result",
	}
}

// BackgroundAgent is an interface for long-running background operations
// Implementations should handle their own goroutine lifecycle
type BackgroundAgent interface {
	// ID returns a unique identifier for this background agent
	ID() string

	// Start starts the background agent operation
	// Called once when agent is added to a want
	Start(ctx context.Context, w *Want) error

	// Stop gracefully stops the background agent
	// Called when want is completed or agent is explicitly deleted
	Stop() error
}

// OwnerReference represents a reference to an owner object
type OwnerReference struct {
	APIVersion         string `json:"apiVersion" yaml:"apiVersion"`
	Kind               string `json:"kind" yaml:"kind"`
	Name               string `json:"name" yaml:"name"`
	ID                 string `json:"id" yaml:"id"`
	Controller         bool   `json:"controller,omitempty" yaml:"controller,omitempty"`
	BlockOwnerDeletion bool   `json:"blockOwnerDeletion,omitempty" yaml:"blockOwnerDeletion,omitempty"`
}

// CorrelationEntry represents the correlation relationship between two Wants.
// Labels uses a unified label space:
//   - "key=value"               : shared metadata label
//   - "using.select/key=value"  : one Want references the other via a using selector
//   - "state.sibling/parent=id" : both Wants share the same parent Want (sibling relationship)
//
// Rate is the weighted sum of Labels (higher = stronger coupling).
type CorrelationEntry struct {
	WantID string   `json:"wantID" yaml:"wantID"`
	Labels []string `json:"labels" yaml:"labels"`
	Rate   int      `json:"rate"   yaml:"rate"`
}

// Metadata contains want identification and classification info
type Metadata struct {
	ID              string             `json:"id,omitempty" yaml:"id,omitempty"`
	Name            string             `json:"name" yaml:"name"`
	Type            string             `json:"type" yaml:"type"`
	Labels          map[string]string  `json:"labels" yaml:"labels"`
	OwnerReferences []OwnerReference   `json:"ownerReferences,omitempty" yaml:"ownerReferences,omitempty"`
	UpdatedAt       int64              `json:"updatedAt" yaml:"-"`                                   // Server-managed timestamp, ignored on load
	IsSystemWant    bool               `json:"isSystemWant,omitempty" yaml:"isSystemWant,omitempty"` // true for system-managed wants
	OrderKey        string             `json:"orderKey,omitempty" yaml:"orderKey,omitempty"`         // Fractional index for custom ordering (supports drag-and-drop reordering)
	Correlation     []CorrelationEntry `json:"correlation,omitempty" yaml:"correlation,omitempty"`   // Inter-Want correlation (computed by correlationPhase, read-only at runtime)
	Series          string             `json:"series,omitempty" yaml:"series,omitempty"`             // Stable lineage ID shared across cancel+rebook cycles; auto-assigned on creation
	Version         int                `json:"version,omitempty" yaml:"version,omitempty"`           // 1-based counter; incremented each time a want replaces a cancelled predecessor in the same series
}

// newSeriesID generates a new random UUID-format series identifier.
// Used to stamp the Metadata.Series field on every freshly created want.
func newSeriesID() string {
	b := make([]byte, 16)
	rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant bits
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// WantSpec contains the desired state configuration for a want
type WantSpec struct {
	Params              map[string]any       `json:"params" yaml:"params"`
	Exposes             []ExposeEntry        `json:"exposes,omitempty" yaml:"exposes,omitempty"`
	Using               []map[string]string  `json:"using,omitempty" yaml:"using,omitempty"`
	Recipe              string               `json:"recipe,omitempty" yaml:"recipe,omitempty"`
	StateSubscriptions  []StateSubscription  `json:"stateSubscriptions,omitempty" yaml:"stateSubscriptions,omitempty"`
	NotificationFilters []NotificationFilter `json:"notificationFilters,omitempty" yaml:"notificationFilters,omitempty"`
	Requires            []string             `json:"requires,omitempty" yaml:"requires,omitempty"`
	When                []WhenSpec           `json:"when,omitempty" yaml:"when,omitempty"`
	FinalResultField    string               `json:"finalResultField,omitempty" yaml:"finalResultField,omitempty"`
	// ResetOnRestart controls whether state is reset to initialValues on each scheduled restart.
	// Defaults to true (nil treated as true). Set explicitly to false to preserve state across restarts.
	ResetOnRestart *bool `json:"resetOnRestart,omitempty" yaml:"resetOnRestart,omitempty"`
}

// GetParam returns the value for the given key from Params map
func (s *WantSpec) GetParam(key string) (any, bool) {
	v, ok := s.Params[key]
	return v, ok
}

// SetParam sets a param value, initializing the map if nil
func (s *WantSpec) SetParam(key string, val any) {
	if s.Params == nil {
		s.Params = make(map[string]any)
	}
	s.Params[key] = val
}

// HasParam returns true if the key exists in Params
func (s *WantSpec) HasParam(key string) bool {
	_, ok := s.Params[key]
	return ok
}

// ParamsAsMap returns the Params map directly
func (s *WantSpec) ParamsAsMap() map[string]any {
	return s.Params
}

// SetParamsFromMap replaces Params with the given map
func (s *WantSpec) SetParamsFromMap(m map[string]any) {
	s.Params = m
}

// UnmarshalJSON implements json.Unmarshaler to support both old map format and new array format.
// Params can be a JSON object (legacy) or array of {key,value} entries.
func (s *WantSpec) UnmarshalJSON(data []byte) error {
	// Use a struct without Params to avoid duplicate key issue
	type WantSpecNoParams struct {
		Exposes             []ExposeEntry        `json:"exposes,omitempty"`
		Using               []map[string]string  `json:"using,omitempty"`
		Recipe              string               `json:"recipe,omitempty"`
		StateSubscriptions  []StateSubscription  `json:"stateSubscriptions,omitempty"`
		NotificationFilters []NotificationFilter `json:"notificationFilters,omitempty"`
		Requires            []string             `json:"requires,omitempty"`
		When                []WhenSpec           `json:"when,omitempty"`
		FinalResultField    string               `json:"finalResultField,omitempty"`
	}
	var raw struct {
		WantSpecNoParams
		Params json.RawMessage `json:"params"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	s.Exposes = raw.Exposes
	s.Using = raw.Using
	s.Recipe = raw.Recipe
	s.StateSubscriptions = raw.StateSubscriptions
	s.NotificationFilters = raw.NotificationFilters
	s.Requires = raw.Requires
	s.When = raw.When
	s.FinalResultField = raw.FinalResultField

	if raw.Params == nil {
		return nil
	}

	// Detect array vs object
	trimmed := bytes.TrimSpace(raw.Params)
	if len(trimmed) > 0 && trimmed[0] == '[' {
		// Array format: [{key: k, value: v}]
		var entries []ParamEntry
		if err := json.Unmarshal(raw.Params, &entries); err != nil {
			return err
		}
		s.Params = make(map[string]any, len(entries))
		for _, e := range entries {
			s.Params[e.Key] = e.Value
		}
	} else {
		// Map format
		var m map[string]any
		if err := json.Unmarshal(raw.Params, &m); err != nil {
			return err
		}
		s.Params = m
	}
	return nil
}

// UnmarshalYAML implements yaml.Unmarshaler to support both old map format and new array format.
// Params can be:
//   - map format (legacy):  params: {key: val}
//   - array format (new):   params: [{key: k, value: v}]
//
// Exposes is a separate section:
//
//	exposes:
//	  - param: local_key
//	    as: upper_scope_name
//	  - currentState: local_state_key
//	    as: upper_scope_name
func (s *WantSpec) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("WantSpec must be a YAML mapping")
	}

	// Split content into params node vs everything else
	type restSpec struct {
		Exposes             []ExposeEntry        `yaml:"exposes,omitempty"`
		Using               []map[string]string  `yaml:"using,omitempty"`
		Recipe              string               `yaml:"recipe,omitempty"`
		StateSubscriptions  []StateSubscription  `yaml:"stateSubscriptions,omitempty"`
		NotificationFilters []NotificationFilter `yaml:"notificationFilters,omitempty"`
		Requires            []string             `yaml:"requires,omitempty"`
		When                []WhenSpec           `yaml:"when,omitempty"`
		FinalResultField    string               `yaml:"finalResultField,omitempty"`
	}

	var paramsNode *yaml.Node
	var otherContent []*yaml.Node
	for i := 0; i+1 < len(value.Content); i += 2 {
		if value.Content[i].Value == "params" {
			paramsNode = value.Content[i+1]
		} else {
			otherContent = append(otherContent, value.Content[i], value.Content[i+1])
		}
	}

	otherNode := &yaml.Node{Kind: yaml.MappingNode, Content: otherContent}
	var rest restSpec
	if err := otherNode.Decode(&rest); err != nil {
		return err
	}
	s.Exposes = rest.Exposes
	s.Using = rest.Using
	s.Recipe = rest.Recipe
	s.StateSubscriptions = rest.StateSubscriptions
	s.NotificationFilters = rest.NotificationFilters
	s.Requires = rest.Requires
	s.When = rest.When
	s.FinalResultField = rest.FinalResultField

	if paramsNode == nil {
		return nil
	}
	if paramsNode.Kind == yaml.SequenceNode {
		// Array format: [{key: k, value: v}]
		var entries []ParamEntry
		if err := paramsNode.Decode(&entries); err != nil {
			return err
		}
		s.Params = make(map[string]any, len(entries))
		for _, e := range entries {
			s.Params[e.Key] = e.Value
		}
	} else {
		// Map format (legacy): {key: val}
		var m map[string]any
		if err := paramsNode.Decode(&m); err != nil {
			return err
		}
		s.Params = m
	}
	return nil
}

// WantHistory contains both parameter and state history
type WantHistory struct {
	ParameterHistory []StateHistoryEntry `json:"parameterHistory" yaml:"parameterHistory"`
	StateHistory     []StateHistoryEntry `json:"stateHistory" yaml:"stateHistory"`
	AgentHistory     []AgentExecution    `json:"agentHistory,omitempty" yaml:"agentHistory,omitempty"`
	LogHistory       []LogHistoryEntry   `json:"logHistory,omitempty" yaml:"logHistory,omitempty"`
}

// LogHistoryEntry represents a collection of log messages from a single Exec cycle
type LogHistoryEntry struct {
	Timestamp time.Time `json:"timestamp" yaml:"timestamp"`
	Logs      string    `json:"logs" yaml:"logs"` // Multiple log lines concatenated
}

// WantStatus represents the current state of a want
type WantStatus string

const (
	WantStatusIdle                WantStatus = "idle"
	WantStatusInitializing        WantStatus = "initializing"
	WantStatusReaching            WantStatus = "reaching"
	WantStatusReachingWithWarning WantStatus = "reaching_with_warning" // Running but governance/label access violations were logged
	WantStatusSuspended           WantStatus = "suspended"
	WantStatusAchieved            WantStatus = "achieved"
	WantStatusAchievedWithWarning WantStatus = "achieved_with_warning" // Achieved but governance/label access violations were logged
	WantStatusCancelled           WantStatus = "cancelled"             // Superseded by a rebook; no longer the active booking
	WantStatusFailed              WantStatus = "failed"
	WantStatusTerminated          WantStatus = "terminated"
	WantStatusDeleting            WantStatus = "deleting"
	WantStatusConfigError         WantStatus = "config_error"        // Invalid input values or spec configuration
	WantStatusModuleError         WantStatus = "module_error"        // Want type implementation error (GetState failure, cast failure, etc.)
	WantStatusPrepareAgent        WantStatus = "prepare_agent"       // Preparing agent runtime
	WantStatusWaitingUserAction   WantStatus = "waiting_user_action" // Waiting for user action (e.g., reaction approval)
)

// IsAchievedStatus returns true if the status represents a completed/achieved state (with or without warnings).
func IsAchievedStatus(s WantStatus) bool {
	return s == WantStatusAchieved || s == WantStatusAchievedWithWarning
}

// IsReachingStatus returns true if the status represents an actively running state (with or without warnings).
func IsReachingStatus(s WantStatus) bool {
	return s == WantStatusReaching || s == WantStatusReachingWithWarning
}

// ControlTrigger represents a control command sent to a Want
type ControlTrigger string

const (
	// Suspend temporarily pauses want execution
	ControlTriggerSuspend ControlTrigger = "suspend"
	// Resume resumes suspended want execution
	ControlTriggerResume ControlTrigger = "resume"
	// Stop terminates want execution
	ControlTriggerStop ControlTrigger = "stop"
	// Restart restarts want execution from initial state
	ControlTriggerRestart ControlTrigger = "restart"
)

// ControlCommand represents a control command with metadata
type ControlCommand struct {
	Trigger   ControlTrigger `json:"trigger"`
	WantID    string         `json:"wantId"`
	Timestamp time.Time      `json:"timestamp"`
	Reason    string         `json:"reason,omitempty"`
}

// TriggerCommand is a union type that wraps either a reconciliation trigger or a control command This allows reconcileTrigger channel to handle both types of signals in a unified way
type TriggerCommand struct {
	Type             string          // "reconcile" or "control"
	ControlCommand   *ControlCommand // Non-nil for control triggers
	ReconcileTrigger bool            // Non-zero for reconciliation triggers
}

// WantStats is deprecated - use State field instead Keeping type alias for backward compatibility during transition
type WantStats = map[string]any

func getCurrentTimestamp() int64 {
	return time.Now().Unix()
}

// WantLocals defines the interface for type-specific local state in wants
// WantLocals is the base interface for type-specific want local state
// Implementations should initialize their fields in the constructor after NewWant() is called
type WantLocals interface {
}

// WantFactory defines the interface for creating want functions
// Returns Progressable which is implemented by concrete Want types (e.g., FlightWant, RestaurantWant)
type WantFactory func(metadata Metadata, spec WantSpec) Progressable

// LocalsFactory defines a factory function for creating WantLocals instances
type LocalsFactory func() WantLocals

// NewWantWithLocals is a variant of NewWant that accepts a WantLocals instance directly
// instead of a factory function. Useful when you have a simple initialization that doesn't
// require re-instantiation for each want.
func NewWantWithLocals(
	metadata Metadata,
	spec WantSpec,
	locals WantLocals,
	wantType string,
) *Want {
	want := &Want{
		Metadata: metadata,
		Spec:     spec,
	}
	want.Init()

	// Assign locals directly
	if locals != nil {
		want.Locals = locals
	}

	// Set type-specific fields
	want.WantType = wantType
	// ConnectivityMetadata is now loaded from YAML via ChainBuilder

	return want
}

// Want represents a processing unit in the chain
type Want struct {
	Metadata        Metadata              `json:"metadata" yaml:"metadata"`
	Spec            WantSpec              `json:"spec" yaml:"spec"`
	Status          WantStatus            `json:"status,omitempty" yaml:"status,omitempty"`
	State           sync.Map              `json:"-" yaml:"-"`
	stateTimestamps sync.Map              `json:"-" yaml:"-"` // key → time.Time, updated on every StoreState call
	StateLabels     map[string]StateLabel `json:"state_labels,omitempty" yaml:"state_labels,omitempty"`
	HiddenState     map[string]any        `json:"hidden_state,omitempty" yaml:"hidden_state,omitempty"`
	History         WantHistory           `json:"history" yaml:"-"`
	Hash            string                `json:"hash,omitempty" yaml:"hash,omitempty"` // Hash for change detection (metadata, spec, all state fields, status)

	// History ring buffers - private, lock-free concurrent stores (populated at runtime, not serialized)
	// History tracking (ring buffers)
	history *HistoryManager `json:"-" yaml:"-"`

	// Lifecycle control
	PreservePendingState bool `json:"-" yaml:"-"` // Deprecated: no longer used

	// Agent execution information
	CurrentAgent  string   `json:"current_agent,omitempty" yaml:"current_agent,omitempty"`
	RunningAgents []string `json:"running_agents,omitempty" yaml:"running_agents,omitempty"`

	// Internal fields for execution tracking
	execCycleCount           int
	inExecCycle              bool
	governanceViolationCount int // incremented on each state access policy violation (label mismatch or role/label governance denial)

	// Agent system
	agentRegistry   *AgentRegistry                `json:"-" yaml:"-"`
	runningAgents   map[string]context.CancelFunc `json:"-" yaml:"-"`
	agentStateMutex sync.RWMutex                  `json:"-" yaml:"-"` // Mutex for runningAgents
	// agentRunGuard tracks per-agent execution state for both DoAgents and MonitorAgents.
	// value "running": agent is currently executing (transient — cleared when done).
	// value "done":    agent completed successfully and must not run again (permanent until Init).
	agentRunGuard sync.Map `json:"-" yaml:"-"`

	// Background agents for long-running operations
	backgroundAgents map[string]BackgroundAgent `json:"-" yaml:"-"`
	backgroundMutex  sync.RWMutex               `json:"-" yaml:"-"`

	// Unified subscription event system
	subscriptionSystem *UnifiedSubscriptionSystem `json:"-" yaml:"-"`

	// Hook called after MergeState completes (used by Target to signal stateNotify)
	onMergeState func() `json:"-" yaml:"-"`

	// Granular locking for non-comparable state types (e.g. nested maps)
	keyMutexes sync.Map `json:"-" yaml:"-"`

	// Remote execution mode (for external agent execution via webhook/grpc)
	remoteMode  bool   `json:"-" yaml:"-"` // True when executing in external agent service
	callbackURL string `json:"-" yaml:"-"` // Callback URL for state updates
	agentName   string `json:"-" yaml:"-"` // Current executing agent name

	// Stop channel for graceful shutdown of want's goroutines
	stopChannel chan struct{} `json:"-" yaml:"-"`

	// Control channel for suspend/resume/stop/restart operations
	controlChannel chan *ControlCommand `json:"-" yaml:"-"`

	// Control state tracking
	suspended atomic.Bool `json:"-" yaml:"-"` // Current suspension state

	// Fields for eliminating duplicate methods in want types
	WantType             string               `json:"-" yaml:"-"`
	paths                Paths                `json:"-" yaml:"-"`
	ConnectivityMetadata ConnectivityMetadata `json:"connectivity_metadata,omitempty" yaml:"-"`

	// Want type definition and state field management
	WantTypeDefinition  *WantTypeDefinition `json:"-" yaml:"-"`
	ProvidedStateFields []string            `json:"-" yaml:"-"` // State field names defined in want type

	// Type-specific local state managed by WantLocals interface
	Locals WantLocals `json:"-" yaml:"-"`

	// Progressable function - concrete want implementation (e.g., RestaurantWant, QueueWant)
	progressable Progressable `json:"-" yaml:"-"`

	// Goroutine execution tracking - Want owns this state for proper encapsulation
	// ChainBuilder sets this via SetGoroutineActive() to inform Want when goroutine starts/stops
	goroutineActive atomic.Bool `json:"-" yaml:"-"`

	// Cached parent want pointer (resolved from OwnerReferences)
	cachedParentWant   *Want  `json:"-" yaml:"-"`
	cachedParentWantID string `json:"-" yaml:"-"`

	// Packet cache for non-consuming checks
	cachedPacket *CachedPacket `json:"-" yaml:"-"`
	cacheMutex   sync.Mutex    `json:"-" yaml:"-"`

	// Metadata protection
	metadataMutex sync.RWMutex `json:"-" yaml:"-"`

	// Retry mechanism for failed phases
	PhaseRetryCount map[string]int `json:"phase_retry_count,omitempty" yaml:"phase_retry_count,omitempty"`
	LastPhaseError  string         `json:"last_phase_error,omitempty" yaml:"last_phase_error,omitempty"`

	// OpenTelemetry: lifecycle span for this want (started on first non-idle status, ended at terminal)
	otelSpan    trace.Span      `json:"-" yaml:"-"`
	otelSpanCtx context.Context `json:"-" yaml:"-"`
}

// MarshalYAML handles custom YAML marshalling for the Want struct.
// Includes State so it is persisted to state.yaml and survives server restarts.
func (n *Want) MarshalYAML() (interface{}, error) {
	type Alias Want
	stateMap := make(map[string]any)
	n.State.Range(func(key, value any) bool {
		stateMap[key.(string)] = value
		return true
	})
	tsMap := make(map[string]time.Time)
	n.stateTimestamps.Range(func(key, value any) bool {
		tsMap[key.(string)] = value.(time.Time)
		return true
	})
	return &struct {
		State           map[string]any       `yaml:"state,omitempty"`
		StateTimestamps map[string]time.Time `yaml:"state_timestamps,omitempty"`
		*Alias          `yaml:",inline"`
	}{
		State:           stateMap,
		StateTimestamps: tsMap,
		Alias:           (*Alias)(n),
	}, nil
}

// UnmarshalYAML handles custom YAML unmarshalling for the Want struct.
// Mirrors UnmarshalJSON: restores State from state.yaml on startup.
func (n *Want) UnmarshalYAML(value *yaml.Node) error {
	type Alias Want
	aux := &struct {
		State           map[string]any       `yaml:"state"`
		StateTimestamps map[string]time.Time `yaml:"state_timestamps"`
		*Alias          `yaml:",inline"`
	}{
		Alias: (*Alias)(n),
	}
	if err := value.Decode(aux); err != nil {
		return err
	}
	for key, val := range aux.State {
		n.State.Store(key, val)
	}
	for key, ts := range aux.StateTimestamps {
		n.stateTimestamps.Store(key, ts)
	}
	return nil
}

// SetStatus sets the status of this want.
//
// STATE OWNERSHIP RULE: A want must only call SetStatus on itself.
// It is illegal for an Agent — or any code running on behalf of Want A — to call
// SetStatus on a different Want B.  Each want is the sole owner of its own status
// field.  Violating this rule causes data races and breaks the execution model
// because every want runs in its own goroutine.
//
// To trigger a status change in another want, use an indirect signal:
//   - Write a flag to the target want's state (e.g. StoreState(_cancel_requested, true))
//     and call cb.RestartWant(targetID) so the target's own goroutine picks it up.
//   - Never reach into cb.GetWants() and call w.SetStatus() on a foreign want.
func (n *Want) SetStatus(status WantStatus) {
	oldStatus := n.Status
	n.Status = status

	// Emit StatusChange event (Group B - synchronous control)
	if oldStatus != status {
		event := &StatusChangeEvent{
			BaseEvent: BaseEvent{
				EventType:  EventTypeStatusChange,
				SourceName: n.Metadata.Name,
				TargetName: "", // Broadcast to all subscribers
				Timestamp:  time.Now(),
				Priority:   5,
			},
			OldStatus: oldStatus,
			NewStatus: status,
		}
		n.GetSubscriptionSystem().Emit(context.Background(), event)

		// Automatically notify ChainBuilder when want reaches achieved status (with or without warnings).
		if IsAchievedStatus(status) {
			// CRITICAL: When want achieves, always set achieving_percentage to 100%
			// Use StoreState (not MergeState) to ensure it's a confirmed value that won't be overwritten
			// MergeState adds to pendingStateChanges which can be overwritten by later StoreState calls
			n.storeState("achieving_percentage", 100.0)

			n.NotifyCompletion()
			// Automatically emit OwnerCompletionEvent to parent target if this want has an owner
			// This is part of the standard progression cycle completion pattern
			n.emitOwnerCompletionEventIfOwned()
		}

		// OpenTelemetry: manage want lifecycle span
		n.otelOnStatusChange(oldStatus, status)
	}
}

// otelOnStatusChange manages the OTEL lifecycle span for this want.
//   - First active status → start a new span
//   - Every status change → add a span event
//   - Terminal status → end the span
func (n *Want) otelOnStatusChange(oldStatus, newStatus WantStatus) {
	if !IsOTELEnabled() {
		return
	}

	terminalStatuses := map[WantStatus]bool{
		WantStatusAchieved:            true,
		WantStatusAchievedWithWarning: true,
		WantStatusFailed:              true,
		WantStatusCancelled:           true,
		WantStatusTerminated:          true,
	}

	// Start span when want first becomes active
	if n.otelSpan == nil && oldStatus == WantStatusIdle {
		tracer := otelTracer()
		if tracer != nil {
			ctx, span := tracer.Start(context.Background(), "want/"+n.Metadata.Name,
				trace.WithAttributes(
					attribute.String("want.name", n.Metadata.Name),
					attribute.String("want.type", n.Metadata.Type),
					attribute.String("want.id", n.Metadata.ID),
				),
			)
			n.otelSpan = span
			n.otelSpanCtx = ctx
		}
	}

	if n.otelSpan == nil {
		return
	}

	// Record status transition as a span event
	n.otelSpan.AddEvent("status_change", trace.WithAttributes(
		attribute.String("want.status.old", string(oldStatus)),
		attribute.String("want.status.new", string(newStatus)),
	))

	// End span at terminal status
	if terminalStatuses[newStatus] {
		n.otelSpan.End()
		n.otelSpan = nil
		n.otelSpanCtx = nil
	}
}

// RestartWant restarts the want's execution by setting its status to Idle
// This triggers the reconcile loop to re-run the want
// Used for scheduled restarts and other re-execution scenarios
func (n *Want) RestartWant() {
	n.StoreLog("[RESTART] Want '%s' restarting execution\n", n.Metadata.Name)
	n.SetStatus(WantStatusIdle)
}

// prepareForRestart resets state fields to initialValues before Initialize() is called.
// Called from the progression loop at both goroutine start and ControlTriggerRestart.
// Order is guaranteed: reset state → Initialize() → onInitialize.
// Type-specific per-run resets (e.g. agentRunGuard for ScriptableWant) are handled
// inside the progressable's own Initialize().
func (n *Want) prepareForRestart() {
	// Always reset agentRunGuard so DoAgents re-execute on restart.
	n.agentRunGuard = sync.Map{}

	// Reset state fields to initialValues when resetOnRestart is enabled (nil = true).
	if n.Spec.ResetOnRestart == nil || *n.Spec.ResetOnRestart {
		cb := GetGlobalChainBuilder()
		if cb != nil {
			typeDef := cb.GetWantTypeDefinition(n.Metadata.Type)
			if typeDef != nil {
				resetState := make(map[string]any)
				for _, sd := range typeDef.State {
					if sd.InitialValue != nil {
						resetState[sd.Name] = sd.InitialValue
					}
				}
				if len(resetState) > 0 {
					n.storeStateMulti(resetState)
					InfoLog("[RESTART] Reset %d state field(s) to initialValues for '%s'\n", len(resetState), n.Metadata.Name)
				}
			}
		}
	}
}

// NotifyCompletion notifies the ChainBuilder that this want has achieved completion Called automatically by SetStatus() when want reaches WantStatusAchieved This enables wants (especially receivers like Coordinators) to self-notify completion Replaces the previous pattern where senders would call UpdateCompletedFlag
func (n *Want) NotifyCompletion() {
	cb := GetGlobalChainBuilder()
	if cb == nil {
		return // No global builder available
	}

	// Notify ChainBuilder that this want is now completed using want ID (not name)
	cb.MarkWantCompleted(n.Metadata.ID, n.Status)

	// Note: Retrigger is triggered by Provide(), not here Only wants that send packets should trigger dependent want re-execution
}

// ReconcileStateFromConfig copies state from a config source using proper state batching
// This method ensures state is restored with proper reconciliation tracking
func (n *Want) ReconcileStateFromConfig(sourceState map[string]any) {
	if sourceState == nil {
		return
	}

	// Use StoreStateMulti to ensure proper batching and reconciliation
	n.storeStateMulti(sourceState)
}
func (n *Want) SetStateAtomic(stateData map[string]any) {
	if stateData == nil {
		return
	}

	// Use StoreStateMulti to ensure proper batching and mutex protection
	n.storeStateMulti(stateData)
}

// UpdateParameter updates a parameter and propagates the change to children
func (n *Want) UpdateParameter(paramName string, paramValue any) {
	// Update the parameter in spec (protected by metadataMutex to avoid concurrent map writes)
	n.metadataMutex.Lock()
	if n.Spec.Params == nil {
		n.Spec.Params = make(map[string]any)
	}
	n.Spec.Params[paramName] = paramValue
	n.metadataMutex.Unlock()

	n.getHistoryManager().AddParameterEntry(paramName, paramValue)

	notification := StateNotification{
		SourceWantName:   n.Metadata.Name,
		StateKey:         paramName,
		StateValue:       paramValue,
		Timestamp:        time.Now(),
		NotificationType: NotificationParameter,
	}
	sendParameterNotifications(notification)
}

// PropagateParameter updates a parameter on the parent want if one exists,
// otherwise sets it as a global parameter. Skips the update if the value is unchanged.
func (n *Want) PropagateParameter(paramName string, paramValue any) {
	parent := n.GetParentWant()
	if parent != nil {
		existing, _ := parent.Spec.GetParam(paramName)
		if existing != nil && reflect.DeepEqual(existing, paramValue) {
			return
		}
		parent.UpdateParameter(paramName, paramValue)
		return
	}
	existing, found := GetGlobalParameter(paramName)
	if found && reflect.DeepEqual(existing, paramValue) {
		return
	}
	SetGlobalParameter(paramName, paramValue)
}

// BeginProgressCycle starts a new execution cycle
func (n *Want) BeginProgressCycle() {
	n.inExecCycle = true
	n.execCycleCount++
}

// EndProgressCycle completes the execution cycle and commits all batched state and parameter changes
func (n *Want) EndProgressCycle() {
	if !n.inExecCycle {
		return
	}

	// Check for state access policy violations accumulated during this cycle (best-effort: log warning, do not fail).
	// Covers both label violations (undeclared keys) and governance violations (role/label permission denied).
	if n.governanceViolationCount > 0 {
		violations := n.governanceViolationCount
		n.governanceViolationCount = 0
		n.setGovernanceWarning("PolicyViolation", fmt.Sprintf("%d state access policy violation(s) in cycle %d — check [GOVERNANCE]/[WARN] logs", violations, n.execCycleCount))
	}
	n.governanceViolationCount = 0

	// CRITICAL: If achieved (with or without warning), ALWAYS enforce achieving_percentage = 100
	if IsAchievedStatus(n.Status) {
		n.storeState("achieving_percentage", 100.0)
	}

	// fetchFrom expansion must run BEFORE FinalResultField so that derived fields are
	// up-to-date when FinalResultField reads them in the same cycle.
	// (e.g. mrs_raw_output written by agent → fetchFrom populates smartgolf_all_available_times
	//  → FinalResultField reads smartgolf_all_available_times for final_result)
	if n.WantTypeDefinition != nil {
		for _, sd := range n.WantTypeDefinition.State {
			if sd.FetchFrom == "" || sd.OnFetchData == "" {
				continue
			}
			// Read fetchFrom source from the flat state map (not label-gated) so that
			// transiently written fields like mrs_raw_output (written via StoreState by
			// plugin agents) are accessible even when not declared in the type definition.
			source, _ := n.getState(sd.FetchFrom)
			if source == nil {
				continue
			}
			sourceMap, ok := source.(map[string]any)
			if !ok {
				continue
			}
			if val := extractJSONPath(sourceMap, sd.OnFetchData); val != nil {
				n.storeState(sd.Name, val)
			}
		}
	}

	// Auto-override final_result from FinalResultField if configured (handles dot-notation and zero-value skipping).
	// Propagation to parent is handled via the currentStateExposeHandler subscribed in RegisterWant.
	if field := n.Spec.FinalResultField; field != "" {
		val, ok := resolveNestedStateField(n, field)
		if ok && val != nil {
			skip := false
			switch v := val.(type) {
			case string:
				skip = v == ""
			case int:
				skip = v == 0
			case float64:
				skip = v == 0
			}
			if !skip {
				n.StoreState("final_result", val) // StoreState emits StateChangeEvent → handler propagates to parent
			}
		}
	}

	n.inExecCycle = false
}

// SetProgressable sets the concrete progressable implementation for this want
func (n *Want) SetProgressable(progressable Progressable) {
	n.progressable = progressable
}

// GetProgressable returns the concrete progressable implementation for this want
func (n *Want) GetProgressable() Progressable {
	return n.progressable
}

// SetGoroutineActive informs Want whether its execution goroutine is running
// Called by ChainBuilder when goroutine starts (true) or stops (false)
// This allows Want to own the goroutine state for proper encapsulation
func (n *Want) SetGoroutineActive(active bool) {
	n.goroutineActive.Store(active)
}

// ShouldRetrigger determines if retrigger should happen when a packet arrives
// Returns true if goroutine is NOT running AND there are pending packets
// This encapsulates the retrigger decision logic within Want itself
func (n *Want) ShouldRetrigger() bool {
	isGoroutineActive := n.goroutineActive.Load()

	status := n.GetStatus()

	// Retrigger if:
	// 1. Goroutine is NOT running (Idle, Suspended) AND has pending packets (check UnusedExists)
	// 2. OR want is already in terminal state (Achieved) but new packets arrived (check UnusedExists)
	// 3. OR want is currently reaching/running (isGoroutineActive == true) but has new packets (check UnusedExists)
	if (!isGoroutineActive && (status == WantStatusIdle || status == WantStatusSuspended)) || status == WantStatusAchieved || (isGoroutineActive && (status == WantStatusReaching || status == WantStatusWaitingUserAction)) {
		// Check for pending packets (non-blocking)
		// Use 0 timeout since packet should already be in channel if we're called after Provide()
		hasUnused := n.UnusedExists(0)
		return hasUnused
	}
	// Goroutine is active and not terminal, and no unused packets
	return false
}

// checkPreconditions verifies that path preconditions are satisfied
// Returns true if all required providers/users are connected, false otherwise
func (n *Want) checkPreconditions(paths Paths) bool {
	// Check minimum required inputs (providers)
	if n.ConnectivityMetadata.RequiredInputs > 0 {
		if len(paths.In) < n.ConnectivityMetadata.RequiredInputs {
			return false
		}
	}

	// Check minimum required outputs (users)
	if n.ConnectivityMetadata.RequiredOutputs > 0 {
		if len(paths.Out) < n.ConnectivityMetadata.RequiredOutputs {
			return false
		}
	}

	// Check maximum inputs if limited
	if n.ConnectivityMetadata.MaxInputs > 0 {
		if len(paths.In) > n.ConnectivityMetadata.MaxInputs {
			return false
		}
	}

	// Check maximum outputs if limited
	if n.ConnectivityMetadata.MaxOutputs > 0 {
		if len(paths.Out) > n.ConnectivityMetadata.MaxOutputs {
			return false
		}
	}

	return true
}

// StartProgressionLoop starts the want execution loop in a goroutine
//
// Parameters (minimal interface):
//   - getPathsFunc: Function that returns current paths (called each iteration)
//   - waitGroup: Goroutine lifecycle coordination only
//
// This method encapsulates the execution lifecycle:
// - Stop channel monitoring
// - Control signal handling (suspend/resume/stop/restart)
// - Path synchronization (from preconditions)
// - Execution cycle management (BeginProgressCycle → Exec → EndProgressCycle)
// - Status transitions
// Note: Uses self.progressable which is set via SetProgressable()
// Note: getPathsFunc is called each iteration to get latest paths
// StartProgressionLoop starts the want execution loop in a goroutine
//
// Parameters:
//   - getPathsFunc: Returns current paths (preconditions: providers/users)
//   - onComplete: Callback invoked when goroutine exits (for synchronization)
func (n *Want) StartProgressionLoop(
	getPathsFunc func() Paths,
	onComplete func(),
) {
	if n.stopChannel == nil {
		n.stopChannel = make(chan struct{})
	}

	go func() {
		defer onComplete()
		defer func() {
			switch n.GetStatus() {
			case WantStatusReaching:
				n.SetStatus(WantStatusAchieved)
			case WantStatusReachingWithWarning:
				n.SetStatus(WantStatusAchievedWithWarning)
			}
		}()

		// Reset per-run state then initialize.
		// Order: reset state/agentRunGuard → Initialize() → onInitialize.
		n.prepareForRestart()
		if n.progressable != nil {
			n.progressable.Initialize()
			n.syncLocalsAfterInitialize()
		}

		// Phase 0: Start persistent background agents (Monitor/Poll/Think) before the loop.
		// DoAgents are excluded here — they run from Progress() via ExecuteAgents().
		if err := n.StartBackgroundAgents(); err != nil {
			n.StoreLog("ERROR: Failed to start background agents during loop startup: %v", err)
		}

		for {
			// 1. Check stop channel
			select {
			case <-n.stopChannel:
				n.SetStatus(WantStatusTerminated)
				if err := n.StopAllBackgroundAgents(); err != nil {
					n.StoreLog("ERROR: Failed to stop background agents on stop: %v", err)
				}
				return
			default:
				// Continue execution
			}

			// 2. Check control signals
			if cmd, received := n.CheckControlSignal(); received {
				switch cmd.Trigger {
				case ControlTriggerSuspend:
					n.SetSuspended(true)
					n.SetStatus(WantStatusSuspended)
				case ControlTriggerResume:
					n.SetSuspended(false)
					n.SetStatus(WantStatusReaching)
				case ControlTriggerStop:
					n.SetStatus(WantStatusTerminated)
					return
				case ControlTriggerRestart:
					n.SetSuspended(false)
					// Stop background agents before re-initializing
					if err := n.StopAllBackgroundAgents(); err != nil {
						n.StoreLog("ERROR: Failed to stop background agents on restart: %v", err)
					}
					n.prepareForRestart()
					if n.progressable != nil {
						n.progressable.Initialize()
						n.syncLocalsAfterInitialize()
					}
					// Re-start background agents that were stopped for restart
					if err := n.StartBackgroundAgents(); err != nil {
						n.StoreLog("ERROR: Failed to re-start background agents on restart: %v", err)
					}
				}
			}

			// 3. Skip execution if suspended
			if n.IsSuspended() {
				time.Sleep(GlobalExecutionInterval)
				continue
			}

			// 3.1a. Check if want has failed (before precondition check)
			if failable, ok := n.progressable.(Failable); ok {
				if failable.IsFailed() {
					n.SetStatus(WantStatusFailed)
					if err := n.StopAllBackgroundAgents(); err != nil {
						n.StoreLog("ERROR: Failed to stop background agents on failed: %v", err)
					}
					return
				}
			}

			// 3.1. Check if want is achieved (before precondition check)
			if n.progressable != nil && n.progressable.IsAchieved() {
				n.SetStatus(WantStatusAchieved)
				// CRITICAL: Even if already achieved, we must run one cycle to ensure
				// FinalResultField is processed, state is aggregated, and child-to-parent
				// propagation (MergeParentState) in Progress() is executed.
				n.BeginProgressCycle()
				n.progressable.Progress()
				n.EndProgressCycle()

				// Dynamic dispatch coordinators (direction_map) keep monitoring after
				// achievement so they can detect deleted child wants and re-dispatch.
				if _, hasDirMap := n.GetParameter("direction_map"); hasDirMap {
					time.Sleep(GlobalExecutionInterval)
					continue
				}

				// Flush ThinkingAgents before stopping (ensures cost propagation etc.)
				n.FlushThinkingAgents(context.Background())
				// Stop all background agents when want is achieved
				if err := n.StopAllBackgroundAgents(); err != nil {
					n.StoreLog("ERROR: Failed to stop background agents: %v", err)
				}
				return
			}

			// 3.2. Check if want is in error or terminal state
			status := n.GetStatus()
			if status == WantStatusFailed || status == WantStatusTerminated || status == WantStatusModuleError || status == WantStatusCancelled {
				// Terminal error states - stop background agents and exit goroutine
				if err := n.StopAllBackgroundAgents(); err != nil {
					n.StoreLog("ERROR: Failed to stop background agents on terminal state: %v", err)
				}
				return
			}
			if status == WantStatusConfigError {
				// ConfigError is recoverable - wait for config update or control signal
				// Check for control signals that might clear the error
				if cmd, received := n.CheckControlSignal(); received {
					if cmd.Trigger == ControlTriggerRestart {
						n.ClearConfigError()
						if n.progressable != nil {
							n.progressable.Initialize()
						}
						continue
					}
				}
				// Stay in config error state, wait for external intervention
				time.Sleep(GlobalExecutionInterval)
				continue
			}
			// 3.5. Get current paths (called each iteration to track topology changes)
			currentPaths := getPathsFunc()

			// 3.8. Check preconditions: verify required providers/users are connected
			if !n.checkPreconditions(currentPaths) {
				// Preconditions not satisfied - mark as suspended and return to loop start
				n.SetSuspended(true)
				n.SetStatus(WantStatusSuspended)
				time.Sleep(GlobalExecutionInterval)
				continue // Go back to step 1: Check stop channel
			}

			// 4. Synchronize paths before execution (preconditions: providers + users)
			n.SetPaths(currentPaths.In, currentPaths.Out)

			// 5. Begin execution cycle (batching mode)
			n.BeginProgressCycle()

			// 6. Check stop channel before execution
			select {
			case <-n.stopChannel:
				n.EndProgressCycle()
				n.SetStatus(WantStatusTerminated)
				if err := n.StopAllBackgroundAgents(); err != nil {
					n.StoreLog("ERROR: Failed to stop background agents on stop: %v", err)
				}
				return
			default:
				// Continue with execution
			}

			// 7. Execute want logic
			if n.progressable == nil {
				// No progressable set, mark as failed
				n.SetStatus(WantStatusFailed)
				return
			}

			// Execute Progress() with panic/recover to handle immediate termination
			// SetModuleErrorAndExit() and SetConfigErrorAndExit() use panic to exit
			// from deep call stacks without requiring error propagation
			exitLoop := false
			func() {
				defer func() {
					if r := recover(); r != nil {
						switch r.(type) {
						case ModuleErrorPanic:
							// Status already set by SetModuleErrorAndExit()
							// Stop background agents and exit goroutine
							if err := n.StopAllBackgroundAgents(); err != nil {
								n.StoreLog("ERROR: Failed to stop background agents: %v", err)
							}
							exitLoop = true
						case ConfigErrorPanic:
							// Status already set by SetConfigErrorAndExit()
							// Will be handled in next loop iteration (wait for config update)
						default:
							// Re-panic for unexpected panics (actual bugs, runtime errors)
							panic(r)
						}
					}
				}()
				// Declarative mapping: State -> Locals
				var locals any
				if progressableVal := reflect.ValueOf(n.progressable); progressableVal.Kind() == reflect.Ptr {
					method := progressableVal.MethodByName("GetLocals")
					if method.IsValid() && method.Type().NumIn() == 0 && method.Type().NumOut() == 1 {
						results := method.Call(nil)
						locals = results[0].Interface()
						SyncLocalsState(n, locals, true)
					}
				}

				n.progressable.Progress()

				// Declarative mapping: Locals -> State
				if locals != nil {
					SyncLocalsState(n, locals, false)
				}
			}()

			// 8. End execution cycle (commit batched changes)
			n.EndProgressCycle()

			if exitLoop {
				return
			}

			// 8.5. Check if want entered error state during Progress()
			currentStatus := n.GetStatus()
			if currentStatus == WantStatusModuleError {
				// Module error - unrecoverable, exit goroutine
				if err := n.StopAllBackgroundAgents(); err != nil {
					n.StoreLog("ERROR: Failed to stop background agents: %v", err)
				}
				return
			}
			if currentStatus == WantStatusCancelled {
				// Self-cancelled by Progress() — stop agents and exit goroutine
				if err := n.StopAllBackgroundAgents(); err != nil {
					n.StoreLog("ERROR: Failed to stop background agents on cancel: %v", err)
				}
				return
			}
			if currentStatus == WantStatusConfigError {
				// Config error - wait for config update (will be handled in next iteration)
				continue
			}

			// 8.6a. Check if want has failed AFTER execution cycle
			if failable, ok := n.progressable.(Failable); ok && failable.IsFailed() {
				n.SetStatus(WantStatusFailed)
				n.FlushThinkingAgents(context.Background())
				if err := n.StopAllBackgroundAgents(); err != nil {
					n.StoreLog("ERROR: Failed to stop background agents on failed: %v", err)
				}
				return
			}

			// 8.6. Check if want is achieved AFTER execution cycle (catch state changes from Progress)
			if n.progressable != nil && n.progressable.IsAchieved() {
				n.SetStatus(WantStatusAchieved)
				// Flush ThinkingAgents before stopping (ensures cost propagation etc.)
				n.FlushThinkingAgents(context.Background())
				// Stop all background agents when want is achieved
				if err := n.StopAllBackgroundAgents(); err != nil {
					n.StoreLog("ERROR: Failed to stop background agents: %v", err)
				}
				return
			}

			// 9. Check for pending agent state changes and dump them
			if n.HasPendingAgentStateChanges() {
				n.DumpStateForAgent("DoAgent")
			}

			// 10. Throttle execution to avoid busy-waiting
			time.Sleep(GlobalExecutionInterval)
		}
	}()
}

// valuesEqual compares two any values for equality Handles different types properly including strings, numbers, booleans, etc.
func (n *Want) valuesEqual(val1, val2 any) bool {
	if val1 == nil && val2 == nil {
		return true
	}
	if val1 == nil || val2 == nil {
		return false
	}

	// Try direct comparison first (works for strings, numbers, booleans)
	return fmt.Sprintf("%v", val1) == fmt.Sprintf("%v", val2)
}

// stateSnapshotsEqual compares two state snapshots (maps) for deep equality Returns true if both maps have identical keys and values
func (n *Want) stateSnapshotsEqual(snapshot1, snapshot2 map[string]any) bool {
	if len(snapshot1) != len(snapshot2) {
		return false
	}
	for key, val1 := range snapshot1 {
		val2, exists := snapshot2[key]
		if !exists {
			return false
		}

		// Use valuesEqual for comparison (handles different types)
		if !n.valuesEqual(val1, val2) {
			return false
		}
	}

	return true
}

// isSignificantStateFields checks if only minor/metadata fields have changed Returns true if ONLY status-like fields have changed (fields ending with "_status") Returns false if significant functional fields have changed
func (n *Want) isOnlyStatusChange(oldState, newState map[string]any) bool {
	changedKeys := make(map[string]bool)
	for key, oldVal := range oldState {
		newVal, exists := newState[key]
		if !exists || !n.valuesEqual(oldVal, newVal) {
			changedKeys[key] = true
		}
	}
	for key := range newState {
		if _, exists := oldState[key]; !exists {
			changedKeys[key] = true
		}
	}

	// If no changes, return true (only status, trivially)
	if len(changedKeys) == 0 {
		return true
	}
	for changedKey := range changedKeys {
		// Status-like fields: end with "_status" or are known metadata fields
		isStatusField := len(changedKey) >= 7 && changedKey[len(changedKey)-7:] == "_status"
		isMetadataField := changedKey == "updated_at" || changedKey == "last_poll_time" ||
			changedKey == "status_changed_at" || changedKey == "status_changed" ||
			changedKey == "status_change_history_count"

		if !isStatusField && !isMetadataField {
			// A significant (non-status) field changed
			return false
		}
	}

	// All changed fields are status-like
	return true
}
func (n *Want) GetStatus() WantStatus {
	return n.Status
}
func (n *Want) InitializeControlChannel() {
	if n.controlChannel == nil {
		n.controlChannel = make(chan *ControlCommand, 10) // Buffered for non-blocking receives
	}
}
func (n *Want) SendControlCommand(cmd *ControlCommand) error {
	if n.controlChannel == nil {
		return fmt.Errorf("control channel not initialized for want %s", n.Metadata.Name)
	}
	select {
	case n.controlChannel <- cmd:
		return nil
	default:
		return fmt.Errorf("control channel full for want %s", n.Metadata.Name)
	}
}
func (n *Want) CheckControlSignal() (*ControlCommand, bool) {
	if n.controlChannel == nil {
		return nil, false
	}
	select {
	case cmd := <-n.controlChannel:
		return cmd, true
	default:
		return nil, false
	}
}

// IsSuspended returns whether the want is currently suspended
func (n *Want) IsSuspended() bool {
	return n.suspended.Load()
}
func (n *Want) SetSuspended(suspended bool) {
	n.suspended.Store(suspended)
}

// StoreState writes a key-value pair directly into this want's State and stages it
// for history recording in the next EndProgressCycle() call.
//
// STATE OWNERSHIP RULE: Only call StoreState on the want that owns the state.
//   - In Progress() / Initialize(): call on receiver (b.StoreState / o.StoreState).
//   - In a DoAgent / MonitorAgent Exec: call on the want passed into the agent.
//   - NEVER call want_A.storeState(...) from code that runs on behalf of want_B.
//     Cross-want state writes bypass locking assumptions and can corrupt state history.
func (n *Want) StoreState(key string, value any) {
	previousValue, _ := n.State.Load(key)
	if n.valuesEqual(previousValue, value) {
		return
	}

	now := time.Now()
	n.State.Store(key, value)
	n.stateTimestamps.Store(key, now)
	n.getHistoryManager().AddStateEntry(key, value)

	notification := StateNotification{
		SourceWantName: n.Metadata.Name,
		StateKey:       key,
		StateValue:     value,
		PreviousValue:  previousValue,
		Timestamp:      now,
	}
	sendStateNotifications(notification)
}

// GetStateUpdatedAt returns the time when the given state key was last updated.
// Returns zero time and false if the key has never been set.
func (n *Want) GetStateUpdatedAt(key string) (time.Time, bool) {
	if v, ok := n.stateTimestamps.Load(key); ok {
		return v.(time.Time), true
	}
	return time.Time{}, false
}

// GetStateTimestamps returns a snapshot of all per-key update timestamps.
func (n *Want) GetStateTimestamps() map[string]time.Time {
	m := make(map[string]time.Time)
	n.stateTimestamps.Range(func(key, value any) bool {
		m[key.(string)] = value.(time.Time)
		return true
	})
	return m
}

// DeleteState removes a key from the want's state map.
func (n *Want) DeleteState(key string) {
	n.State.Delete(key)
}

// MergeState safely merges new state updates into pending state changes for batch processing
// CRITICAL: Designed for async operations (e.g., concurrent Coordinator packet processing)
// Strategy: Each async call adds to pendingStateChanges without blocking
// GetState() reads from pendingStateChanges first (sees latest pending updates)
// EndProgressCycle() dumps all pending changes to disk together
// This ensures all concurrent async calls are recorded without synchronization delays
// Example: Evidence and Description handlers both call MergeState() concurrently
//
//	Evidence:    MergeState({"0": evidence}) → pendingStateChanges[0]=evidence
//	Description: MergeState({"1": description}) → pendingStateChanges[1]=description
//	GetState("0") → reads from pending → returns evidence (immediately)
//	GetState("1") → reads from pending → returns description (immediately)
//	EndProgressCycle() → dumps all pending together → both recorded
func (n *Want) MergeState(updates map[string]any) {
	extractMap := func(v any) (map[string]any, bool) {
		if m, ok := v.(map[string]any); ok {
			return m, true
		}
		if m, ok := v.(map[string]interface{}); ok {
			return m, true
		}
		return nil, false
	}

	for key, value := range updates {
		valueMap, isValueMap := extractMap(value)

		if !isValueMap {
			n.storeState(key, value)
			continue
		}

		// Use per-key mutex for atomic read-modify-write of maps
		muRaw, _ := n.keyMutexes.LoadOrStore(key, &sync.Mutex{})
		mu := muRaw.(*sync.Mutex)
		mu.Lock()

		var merged map[string]any
		if oldVal, ok := n.State.Load(key); ok {
			if oldMap, isOldMap := extractMap(oldVal); isOldMap {
				merged = make(map[string]any, len(oldMap)+len(valueMap))
				for k, v := range oldMap {
					merged[k] = v
				}
				for k, v := range valueMap {
					merged[k] = v
				}
			} else {
				merged = valueMap
			}
		} else {
			merged = valueMap
		}

		n.State.Store(key, merged)
		n.getHistoryManager().AddStateEntry(key, merged)
		mu.Unlock()
	}

	if n.onMergeState != nil {
		n.onMergeState()
	}
}

func (n *Want) getParentWant() *Want {
	if len(n.Metadata.OwnerReferences) == 0 {
		return nil
	}
	var parentID string
	for _, ref := range n.Metadata.OwnerReferences {
		if ref.Controller && ref.Kind == "Want" {
			parentID = ref.ID
			break
		}
	}
	if parentID == "" {
		return nil
	}

	// DO NOT CACHE Parent Want pointer.
	// Caching can lead to using stale Want objects if the parent is recreated
	// during reconciliation or if it was initially found in Config but later promoted to Runtime.
	// Always look up the latest runtime instance from the global builder.

	cb := GetGlobalChainBuilder()
	if cb == nil {
		return nil
	}
	parent, _, found := cb.FindWantByID(parentID)
	if !found {
		return nil
	}
	return parent
}

func (n *Want) GetParentWant() *Want { return n.getParentWant() }

// isOwnerOf returns true if this want is a controller owner of the target want.
func (n *Want) isOwnerOf(target *Want) bool {
	for _, ref := range target.Metadata.OwnerReferences {
		if ref.Controller && ref.ID == n.Metadata.ID {
			return true
		}
	}
	return false
}

// AddChildWant adds a new child want to the system, automatically setting
// this want as the owner (parent).
//
// HIERARCHY RULE: Sub-wants are forbidden from calling cb.AddWant directly.
// They must instead use AddChildWant on their parent, or request the parent
// to dispatch via a ThinkAgent (DispatchThinkerAgent).
func (n *Want) AddChildWant(child *Want) error {
	cb := GetGlobalChainBuilder()
	if cb == nil {
		return fmt.Errorf("ChainBuilder unavailable")
	}

	// Ensure child has a valid ID
	if child.Metadata.ID == "" {
		child.Metadata.ID = GenerateUUID()
	}

	// Set owner reference to this want
	ownerRef := OwnerReference{
		APIVersion: "mywant/v1",
		Kind:       "Want",
		Name:       n.Metadata.Name,
		ID:         n.Metadata.ID,
		Controller: true,
	}
	child.Metadata.OwnerReferences = append(child.Metadata.OwnerReferences, ownerRef)

	// Add to system asynchronously
	return cb.AddWantsAsync([]*Want{child})
}

// HasParent returns true if this want has a parent coordinator.
func (n *Want) HasParent() bool {

	return n.getParentWant() != nil
}

func (n *Want) GetParentState(path string) (any, bool) {
	parent := n.getParentWant()
	if parent == nil {
		return resolveGlobalPath(path)
	}
	// もし path にドットが含まれていたら階層探索、そうでなければ親のステート取得
	if strings.Contains(path, ".") {
		return resolveGlobalPath(path)
	}
	return parent.getState(path)
}

func resolveGlobalPath(path string) (any, bool) {
	parts := strings.Split(path, ".")
	if len(parts) == 0 {
		return nil, false
	}

	// ルート（wants など）を取得
	val, ok := GetGlobalState(parts[0])
	if !ok {
		return nil, false
	}

	// 階層を辿る
	current := val
	for i := 1; i < len(parts); i++ {
		if m, ok := current.(map[string]any); ok {
			current, ok = m[parts[i]]
			if !ok {
				return nil, false
			}
		} else if m, ok := current.(map[any]any); ok {
			// YAML unmarshalでmap[any]anyになる場合への対応
			current, ok = m[parts[i]]
			if !ok {
				return nil, false
			}
		} else {
			return nil, false
		}
	}
	return current, true
}

func (n *Want) StoreParentState(key string, value any) {
	parent := n.getParentWant()
	if parent == nil {
		StoreGlobalState(key, value) // fallback to globalState for top-level wants
		return
	}

	role := n.GetRole()
	label := parent.StateLabels[key]
	engine := &GovernanceEngine{}

	if !engine.CanWriteParentState(role, label) {
		n.StoreLog("[GOVERNANCE] Write Warning: child %q (role:%s) attempted to write parent %q's key %q (label:%v) — policy not satisfied, proceeding anyway\n",
			n.Metadata.Name, role, parent.Metadata.Name, key, label)
		n.governanceViolationCount++
	}

	parent.storeState(key, value)
}

func (n *Want) MergeParentState(updates map[string]any) {
	parent := n.getParentWant()
	if parent == nil {
		MergeGlobalState(updates) // fallback to globalState for top-level wants
		return
	}

	role := n.GetRole()
	engine := &GovernanceEngine{}

	for k := range updates {
		label := parent.StateLabels[k]
		if !engine.CanWriteParentState(role, label) {
			n.StoreLog("[GOVERNANCE] Merge Warning: child %q (role:%s) attempted to write %q's key %q (label:%v) — policy not satisfied, proceeding anyway\n",
				n.Metadata.Name, role, parent.Metadata.Name, k, label)
			n.governanceViolationCount++
		}
	}

	if len(updates) > 0 {
		parent.MergeState(updates)
	}
}

// GetRole returns the ChildRole assigned to this want via its labels.
func (n *Want) GetRole() ChildRole {
	if n.Metadata.Labels != nil {
		if role, ok := n.Metadata.Labels["child-role"]; ok {
			return ChildRole(role)
		}
	}
	return RoleUnknown
}

// ProposeDispatch writes a fully-resolved list of DispatchRequests to the parent
// Target's "desired_dispatch" state. The parent's DispatchExecutor reconciles this
// list against existing children and calls AddChildWant idempotently.
// Overwrites (not appends) so that OPA replanning that removes directions is respected.
func (n *Want) ProposeDispatch(requests []DispatchRequest) {
	if requests == nil {
		requests = []DispatchRequest{}
	}
	n.StoreParentState("desired_dispatch", requests)
}

// SuggestParent suggests a set of directions to the parent want (or global state).
// This is typically used by planning wants (like Itinerary) to request the
// parent's DispatchThinker to realize new child wants.
// It appends to existing directions instead of overwriting to support incremental planning.
func (n *Want) SuggestParent(directions []string) {
	parent := n.getParentWant()

	var existing []string
	var raw any
	var exists bool

	if parent != nil {
		raw, exists = parent.getState("directions")
	} else {
		raw, exists = GetGlobalState("directions")
	}

	if exists {
		if slice, ok := raw.([]string); ok {
			existing = slice
		} else if slice, ok := raw.([]any); ok {
			for _, item := range slice {
				if s, ok := item.(string); ok {
					existing = append(existing, s)
				}
			}
		}
	}

	changed := false
	for _, d := range directions {
		if !Contains(existing, d) {
			existing = append(existing, d)
			changed = true
		}
	}

	if changed || (!exists && len(directions) > 0) {
		if parent != nil {
			DebugLog("[SUGGEST] Adding %d directions to parent %s (total: %d)",
				len(directions), parent.Metadata.Name, len(existing))
			n.MergeParentState(map[string]any{"directions": existing})
		} else {
			DebugLog("[SUGGEST] Adding %d directions to global state (total: %d)",
				len(directions), len(existing))
			MergeGlobalState(map[string]any{"directions": existing})
		}
	}
}

// StoreStateMulti is the batch variant of StoreState: it writes all key-value pairs
// in updates into this want's State in a single mutex-protected pass and stages
// them all for history recording.
//
// STATE OWNERSHIP RULE: Same as StoreState — only call on the receiver want.
// From an Agent Exec function use StoreStateMultiForAgent() instead.
func (n *Want) storeStateMulti(updates map[string]any) {
	// Collect all notifications
	var notifications []StateNotification
	for key, value := range updates {
		previousValue, _ := n.State.Load(key)
		if n.valuesEqual(previousValue, value) {
			continue
		}

		now := time.Now()
		n.State.Store(key, value)
		n.stateTimestamps.Store(key, now)
		n.getHistoryManager().AddStateEntry(key, value)

		notification := StateNotification{
			SourceWantName: n.Metadata.Name,
			StateKey:       key,
			StateValue:     value,
			PreviousValue:  previousValue,
			Timestamp:      now,
		}
		notifications = append(notifications, notification)
	}

	for _, notification := range notifications {
		sendStateNotifications(notification)
	}
}

// // want.StoreLog("Calculation complete: result = 42")
// func (n *Want) StoreLog(message string) {
// 	// Only buffer logs if we're in an Exec cycle
// 	if !n.inExecCycle {
// 		return
// 	}
// 	n.pendingLogs = append(n.pendingLogs, message)
// }

// DirectLog writes a log entry immediately to both the server log and the log
// history ring, bypassing pendingLogs. Safe to call from background goroutines
// (ThinkAgent, MonitorAgent, PollAgent) that run outside the want's Progress cycle.
func (n *Want) DirectLog(message string, args ...any) {
	formatted := fmt.Sprintf(message, args...)
	InfoLog("[%s] %s", n.Metadata.Name, formatted)
	n.getHistoryManager().AddLogEntry(formatted)
	n.otelEmitWantLog(otellog.SeverityInfo, formatted)
}

func (n *Want) StoreLog(message string, args ...any) {
	formatted := fmt.Sprintf(message, args...)
	n.getHistoryManager().AddLogEntry(formatted)
	n.otelEmitWantLog(otellog.SeverityDebug, formatted)
}

// otelEmitWantLog emits a log record with want identity attributes.
func (n *Want) otelEmitWantLog(severity otellog.Severity, body string) {
	ctx := n.otelSpanCtx
	if ctx == nil {
		ctx = context.Background()
	}
	otelEmitLog(ctx, severity, body,
		otellog.String("want.name", n.Metadata.Name),
		otellog.String("want.type", n.Metadata.Type),
		otellog.String("want.status", string(n.Status)),
	)
}

func (n *Want) getState(key string) (any, bool) {
	return n.State.Load(key)
}

func (n *Want) storeState(key string, value any) {
	n.StoreState(key, value)
}

// StoreStateMulti writes multiple key-value pairs into the want's state.
// This is a package-level helper for system-internal use (e.g. restoring state).
// Deprecated: For logic within Progress(), use SetCurrent, SetGoal, etc.
func StoreStateMulti(wp WantPointer, updates map[string]any) {
	wp.GetWant().storeStateMulti(updates)
}

// GetPendingStateChanges is obsolete; state changes are committed immediately via StoreState.
func (n *Want) GetPendingStateChanges() map[string]any {
	return make(map[string]any)
}

// SetRemoteCallback configures Want for remote execution mode with callback support
func (n *Want) SetRemoteCallback(callbackURL, agentName string) {
	n.callbackURL = callbackURL
	n.agentName = agentName
	n.remoteMode = true
}

// SendCallback sends pending state changes to the callback URL (for remote agent execution)
func (n *Want) SendCallback() error {
	if n.callbackURL == "" {
		return fmt.Errorf("callback URL not set")
	}

	changes := n.GetPendingStateChanges()
	if len(changes) == 0 {
		return nil // No changes to send
	}

	callback := WebhookCallback{
		AgentName:    n.agentName,
		WantID:       n.Metadata.Name,
		Status:       "state_changed",
		StateUpdates: changes,
	}

	// Send callback asynchronously
	go func() {
		body, err := json.Marshal(callback)
		if err != nil {
			log.Printf("[CALLBACK] Failed to marshal callback: %v", err)
			return
		}

		resp, err := http.Post(n.callbackURL, "application/json", bytes.NewReader(body))
		if err != nil {
			log.Printf("[CALLBACK] Failed to send callback to %s: %v", n.callbackURL, err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
			log.Printf("[CALLBACK] Callback returned status %d", resp.StatusCode)
		}
	}()

	return nil
}

// ============================================================================
// Agent Lifecycle Management
// ============================================================================

// RegisterRunningAgent registers a running agent with its cancel function
func (n *Want) RegisterRunningAgent(agentName string, cancel context.CancelFunc) {
	n.agentStateMutex.Lock()
	defer n.agentStateMutex.Unlock()

	if n.runningAgents == nil {
		n.runningAgents = make(map[string]context.CancelFunc)
	}
	n.runningAgents[agentName] = cancel

	log.Printf("[LIFECYCLE] Registered running agent: %s for want %s", agentName, n.Metadata.Name)
}

// UnregisterRunningAgent removes a running agent from the registry
func (n *Want) UnregisterRunningAgent(agentName string) {
	n.agentStateMutex.Lock()
	defer n.agentStateMutex.Unlock()

	if n.runningAgents != nil {
		delete(n.runningAgents, agentName)
		log.Printf("[LIFECYCLE] Unregistered running agent: %s for want %s", agentName, n.Metadata.Name)
	}
}

// GetRunningAgents returns the names of all currently running agents
func (n *Want) GetRunningAgents() []string {
	n.agentStateMutex.RLock()
	defer n.agentStateMutex.RUnlock()

	if n.runningAgents == nil {
		return []string{}
	}

	agents := make([]string, 0, len(n.runningAgents))
	for agentName := range n.runningAgents {
		agents = append(agents, agentName)
	}
	return agents
}

// if ok { // provided is a valid bool }
func (n *Want) GetStateBool(key string, defaultValue bool) (bool, bool) {
	value, exists := n.getState(key)
	if !exists {
		return defaultValue, false
	}
	if boolVal, ok := value.(bool); ok {
		return boolVal, true
	}
	return defaultValue, false
}

// count, ok := want.GetStateInt("total_processed", 0) if ok { // count is a valid int }
func (n *Want) GetStateInt(key string, defaultValue int) (int, bool) {
	value, exists := n.getState(key)
	if !exists {
		return defaultValue, false
	}
	if intVal, ok := value.(int); ok {
		return intVal, true
	} else if floatVal, ok := value.(float64); ok {
		return int(floatVal), true
	}
	return defaultValue, false
}

func (n *Want) GetStateFloat64(key string, defaultValue float64) (float64, bool) {
	value, exists := n.getState(key)
	if !exists {
		return defaultValue, false
	}
	if floatVal, ok := value.(float64); ok {
		return floatVal, true
	} else if intVal, ok := value.(int); ok {
		return float64(intVal), true
	}
	return defaultValue, false
}

// if ok { // name is a valid string }
func (n *Want) GetStateString(key string, defaultValue string) (string, bool) {
	value, exists := n.getState(key)
	if !exists {
		return defaultValue, false
	}
	if strVal, ok := value.(string); ok {
		return strVal, true
	}
	return defaultValue, false
}

func (n *Want) GetStateTime(key string, defaultValue time.Time) (time.Time, bool) {
	value, exists := n.getState(key)
	if !exists {
		return defaultValue, false
	}
	if strVal, ok := value.(string); ok {
		if t, err := time.Parse(time.RFC3339, strVal); err == nil {
			return t, true
		}
	}
	return defaultValue, false
}

func GetStateAs[T any](n *Want, key string) (T, bool) {
	value, exists := n.getState(key)
	if !exists {
		var zero T
		return zero, false
	}
	typedVal, ok := value.(T)
	return typedVal, ok
}

// GetStateMulti populates the provided Dict with values from the state.
// For each key in the data map, it retrieves the current state value,
// converts it to the type of the value currently in the map (acting as a default),
// and updates either the map entry or the value it points to (if it's a pointer).
func (n *Want) GetStateMulti(data Dict) {
	for key, templateValue := range data {
		val, exists := n.getState(key)
		if !exists {
			continue
		}

		// Use reflection to handle both pointers and direct values
		rv := reflect.ValueOf(templateValue)

		// 1. If it's a pointer, we populate the value it points to
		if rv.Kind() == reflect.Ptr && !rv.IsNil() {
			elem := rv.Elem()
			switch elem.Interface().(type) {
			case string:
				s, _ := n.GetStateString(key, elem.String())
				elem.SetString(s)
			case int:
				i, _ := n.GetStateInt(key, int(elem.Int()))
				elem.SetInt(int64(i))
			case float64:
				f, _ := n.GetStateFloat64(key, elem.Float())
				elem.SetFloat(f)
			case bool:
				b, _ := n.GetStateBool(key, elem.Bool())
				elem.SetBool(b)
			case time.Time:
				current := elem.Interface().(time.Time)
				t, _ := n.GetStateTime(key, current)
				elem.Set(reflect.ValueOf(t))
			default:
				// Try direct assignment for other types
				v := reflect.ValueOf(val)
				if v.Type().AssignableTo(elem.Type()) {
					elem.Set(v)
				}
			}
			continue
		}

		// 2. Otherwise, update the map entry with a converted value
		switch v := templateValue.(type) {
		case string:
			data[key], _ = n.GetStateString(key, v)
		case int:
			data[key], _ = n.GetStateInt(key, v)
		case float64:
			data[key], _ = n.GetStateFloat64(key, v)
		case bool:
			data[key], _ = n.GetStateBool(key, v)
		case time.Time:
			data[key], _ = n.GetStateTime(key, v)
		default:
			data[key] = val
		}
	}
}

// CalculateAchievingPercentage computes the progress percentage toward completion. This is a virtual method that want type implementations should override to provide type-specific completion percentage calculation. Default implementation returns 0 unless the want has reached completion status,
// in which case it returns 100. Want types should override this to provide meaningful progress indicators: - ApprovalWant: Calculate based on evidence/description fields (0%, 50%, 100%) - QueueWant: Calculate based on processedCount / total capacity
// - Numbers generator: Calculate based on currentCount / target count - Coordinator: Calculate based on channels heard / total required channels - Travel wants (Restaurant, Hotel, Buffet): Return 100 if attempted, else 0
func (n *Want) CalculateAchievingPercentage() int {
	// Default implementation: check if completed Want types should override this method for specific logic
	switch n.Status {
	case "completed", "achieved":
		return 100
	case "idle", "reaching", "suspended":
		return 0
	default:
		return 0
	}
}

// SetStateLabels populates the static StateLabels map from a WantTypeDefinition.

// SetStateLabels populates the static StateLabels map from a WantTypeDefinition.
func (n *Want) SetStateLabels(def *WantTypeDefinition) {
	if def == nil {
		return
	}
	n.StateLabels = make(map[string]StateLabel)
	for _, s := range def.State {
		var label StateLabel
		switch s.Label {
		case "goal":
			label = LabelGoal
		case "current":
			label = LabelCurrent
		case "plan":
			label = LabelPlan
		case "internal":
			label = LabelInternal
		default:
			label = LabelNone
		}
		n.StateLabels[s.Name] = label
	}
}

// ============================================================================
// Agent State Labeling Helpers
// ============================================================================

func (n *Want) SetGoal(key string, value any) {
	if label, ok := n.StateLabels[key]; !ok || label != LabelGoal {
		WarnLog("[WARN] SetGoal(%q) on want %q (type=%s): key not declared with label 'goal' in state definition", key, n.Metadata.Name, n.Metadata.Type)
		n.governanceViolationCount++
	}
	n.storeState(key, value)
}

func (n *Want) GetGoal(key string) (any, bool) {
	if label, ok := n.StateLabels[key]; ok && label == LabelGoal {
		return n.getState(key)
	}
	return nil, false
}

func (n *Want) SetCurrent(key string, value any) {
	if label, ok := n.StateLabels[key]; !ok || label != LabelCurrent {
		WarnLog("[WARN] SetCurrent(%q) on want %q (type=%s): key not declared with label 'current' in state definition", key, n.Metadata.Name, n.Metadata.Type)
		n.governanceViolationCount++
	}
	n.storeState(key, value)
}

func (n *Want) GetCurrent(key string) (any, bool) {
	if label, ok := n.StateLabels[key]; ok && label == LabelCurrent {
		return n.getState(key)
	}
	return nil, false
}

// GetAllCurrent returns all state entries whose label is LabelCurrent.
func (n *Want) GetAllCurrent() map[string]any {
	return n.getAllByLabel(LabelCurrent)
}

// GetAllGoal returns all state entries whose label is LabelGoal.
func (n *Want) GetAllGoal() map[string]any {
	return n.getAllByLabel(LabelGoal)
}

// GetAllPlan returns all state entries whose label is LabelPlan.
func (n *Want) GetAllPlan() map[string]any {
	return n.getAllByLabel(LabelPlan)
}

// GetParentAllCurrent returns all current-labeled state entries from the parent want,
// or nil if there is no parent.
func (n *Want) GetParentAllCurrent() map[string]any {
	parent := n.getParentWant()
	if parent == nil {
		return nil
	}
	return parent.GetAllCurrent()
}

// GetParentAllGoal returns all goal-labeled state entries from the parent want,
// or nil if there is no parent.
func (n *Want) GetParentAllGoal() map[string]any {
	parent := n.getParentWant()
	if parent == nil {
		return nil
	}
	return parent.GetAllGoal()
}

// getAllByLabel collects all state key-value pairs registered under the given label.
func (n *Want) getAllByLabel(label StateLabel) map[string]any {
	result := make(map[string]any)
	for key, l := range n.StateLabels {
		if l != label {
			continue
		}
		if val, ok := n.getState(key); ok {
			result[key] = val
		}
	}
	return result
}

func (n *Want) SetPlan(key string, value any) {
	if label, ok := n.StateLabels[key]; !ok || label != LabelPlan {
		WarnLog("[WARN] SetPlan(%q) on want %q (type=%s): key not declared with label 'plan' in state definition", key, n.Metadata.Name, n.Metadata.Type)
		n.governanceViolationCount++
	}
	n.storeState(key, value)
}

func (n *Want) GetPlan(key string) (any, bool) {
	if label, ok := n.StateLabels[key]; ok && label == LabelPlan {
		return n.getState(key)
	}
	return nil, false
}

func (n *Want) ClearPlan(key string) {
	if label, ok := n.StateLabels[key]; ok && label == LabelPlan {
		n.storeState(key, nil)
	}
}

func (n *Want) SetInternal(key string, value any) {
	if label, ok := n.StateLabels[key]; !ok || label != LabelInternal {
		WarnLog("[WARN] SetInternal(%q) on want %q (type=%s): key not declared with label 'internal' in state definition", key, n.Metadata.Name, n.Metadata.Type)
		n.governanceViolationCount++
	}
	n.storeState(key, value)
}

func (n *Want) GetInternal(key string) (any, bool) {
	if label, ok := n.StateLabels[key]; ok && label == LabelInternal {
		return n.getState(key)
	}
	return nil, false
}

// WantPointer is an interface for types that can provide a pointer to their underlying Want.
// This allows our generic helpers to work with custom types that embed Want.
type WantPointer interface {
	GetWant() *Want
}

// GetWant implements the WantPointer interface for the base Want type.
func (n *Want) GetWant() *Want {
	return n
}

// --- Package-level Generic State Access Helpers ---

// GetState retrieves a value from the want's state with automatic type conversion.
func GetState[T any](wp WantPointer, key string, defaultVal T) T {
	n := wp.GetWant()
	raw, ok := n.getState(key)
	if !ok || raw == nil {
		return defaultVal
	}
	return convertToType(raw, defaultVal)
}

// GetGoal retrieves a goal-labeled value with automatic type conversion.
func GetGoal[T any](wp WantPointer, key string, defaultVal T) T {
	n := wp.GetWant()
	raw, ok := n.GetGoal(key)
	if !ok || raw == nil {
		return defaultVal
	}
	return convertToType(raw, defaultVal)
}

// GetCurrent retrieves a current-labeled value with automatic type conversion.
func GetCurrent[T any](wp WantPointer, key string, defaultVal T) T {
	n := wp.GetWant()
	raw, ok := n.GetCurrent(key)
	if !ok || raw == nil {
		return defaultVal
	}
	return convertToType(raw, defaultVal)
}

// GetPlan retrieves a plan-labeled value with automatic type conversion.
func GetPlan[T any](wp WantPointer, key string, defaultVal T) T {
	n := wp.GetWant()
	raw, ok := n.GetPlan(key)
	if !ok || raw == nil {
		return defaultVal
	}
	return convertToType(raw, defaultVal)
}

// GetInternal retrieves an internal-labeled value with automatic type conversion.
func GetInternal[T any](wp WantPointer, key string, defaultVal T) T {
	n := wp.GetWant()
	raw, ok := n.GetInternal(key)
	if !ok || raw == nil {
		return defaultVal
	}
	return convertToType(raw, defaultVal)
}

// GetParentState retrieves a value from the parent want's state with automatic type conversion.
func GetParentState[T any](wp WantPointer, key string, defaultVal T) T {
	n := wp.GetWant()
	raw, ok := n.GetParentState(key)
	if !ok || raw == nil {
		return defaultVal
	}
	return convertToType(raw, defaultVal)
}

// convertToType is an internal helper that bridges generics to our conversion utilities.
func convertToType[T any](val any, defaultVal T) T {
	// Attempt direct type assertion first for performance and complex types
	if tVal, ok := val.(T); ok {
		return tVal
	}

	// Fallback to our flexible conversion utilities for common primitive types
	var result any
	switch d := any(defaultVal).(type) {
	case string:
		result = ToString(val, d)
	case int:
		result = ToInt(val, d)
	case bool:
		result = ToBool(val, d)
	case float64:
		result = ToFloat64(val, d)
	case []string:
		result = ToStringSlice(val, d)
	case []int:
		result = ToIntSlice(val, d)
	case []float64:
		result = ToFloat64Slice(val, d)
	default:
		// If we don't have a conversion helper, we've already tried direct assertion
		return defaultVal
	}
	return result.(T)
}

func (n *Want) GetParameter(paramName string) (any, bool) {
	n.metadataMutex.RLock()
	defer n.metadataMutex.RUnlock()
	if n.Spec.Params == nil {
		return nil, false
	}
	value, exists := n.Spec.Params[paramName]
	return value, exists
}
func (n *Want) InitializeSubscriptionSystem() {
	if n.subscriptionSystem == nil {
		n.subscriptionSystem = NewUnifiedSubscriptionSystem()
	}
}
func (n *Want) GetSubscriptionSystem() *UnifiedSubscriptionSystem {
	return GetGlobalSubscriptionSystem()
}

func (n *Want) GetAllState() map[string]any {
	state := make(map[string]any)
	n.State.Range(func(key, value any) bool {
		state[key.(string)] = value
		return true
	})
	return state
}
func (n *Want) GetStopChannel() chan struct{} {
	if n.stopChannel == nil {
		n.stopChannel = make(chan struct{})
	}
	return n.stopChannel
}

// OnProcessEnd handles state storage when the want process ends
func (n *Want) OnProcessEnd(finalState map[string]any) {
	n.SetStatus(WantStatusAchieved)
	for key, value := range finalState {
		n.storeState(key, value)
	}
	n.storeState("completion_time", fmt.Sprintf("%d", getCurrentTimestamp()))

	// Commit any pending state changes into a single batched history entry
	n.CommitStateChanges()

	// Stop all running agents (synchronous DoAgents)
	n.StopAllAgents()
	// Stop all background agents (ThinkAgent, MonitorAgent, PollAgent)
	if err := n.StopAllBackgroundAgents(); err != nil {
		n.StoreLog("ERROR: Failed to stop background agents on process end: %v", err)
	}

	// Emit ProcessEnd event (Group B - synchronous control)
	event := &ProcessEndEvent{
		BaseEvent: BaseEvent{
			EventType:  EventTypeProcessEnd,
			SourceName: n.Metadata.Name,
			TargetName: "", // Broadcast to all subscribers
			Timestamp:  time.Now(),
			Priority:   5,
		},
		FinalState: n.GetAllState(),
		Success:    true,
	}
	n.GetSubscriptionSystem().Emit(context.Background(), event)
}

// OnProcessFail handles state storage when the want process fails
func (n *Want) OnProcessFail(errorState map[string]any, err error) {
	n.SetStatus(WantStatusFailed)
	for key, value := range errorState {
		n.storeState(key, value)
	}
	n.storeState("error", err.Error())
	n.storeState("failure_time", fmt.Sprintf("%d", getCurrentTimestamp()))

	// Commit any pending state changes into a single batched history entry
	n.CommitStateChanges()

	// Stop all running agents (synchronous DoAgents)
	n.StopAllAgents()
	// Stop all background agents (ThinkAgent, MonitorAgent, PollAgent)
	if err := n.StopAllBackgroundAgents(); err != nil {
		n.StoreLog("ERROR: Failed to stop background agents on process fail: %v", err)
	}

	// Emit ProcessEnd event with failure (Group B - synchronous control)
	event := &ProcessEndEvent{
		BaseEvent: BaseEvent{
			EventType:  EventTypeProcessEnd,
			SourceName: n.Metadata.Name,
			TargetName: "", // Broadcast to all subscribers
			Timestamp:  time.Now(),
			Priority:   5,
		},
		FinalState: n.GetAllState(),
		Success:    false,
	}
	n.GetSubscriptionSystem().Emit(context.Background(), event)
}

// serializeLabels converts label map to deterministic topic name for PubSub routing.
// Ensures consistent topic names across publisher and subscribers.
// Example: {role: "processor", stage: "final"} → "role=processor,stage=final"
func serializeLabels(labels map[string]string) string {
	if labels == nil || len(labels) == 0 {
		return ""
	}

	// Sort keys for deterministic output
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Build comma-separated key=value pairs
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = fmt.Sprintf("%s=%s", k, labels[k])
	}
	return strings.Join(parts, ",")
}

// provideRaw sends a raw data packet to PubSub topic for subscribers.
// Internal use only — callers should use Provide(*DataObject) instead.
func (n *Want) provideRaw(payload any) {
	cb := GetGlobalChainBuilder()

	n.metadataMutex.RLock()
	topic := n.Metadata.Name // Fallback to name if no labels
	hasLabels := len(n.Metadata.Labels) > 0
	wantName := n.Metadata.Name

	if hasLabels {
		topic = serializeLabels(n.Metadata.Labels)
	}
	n.metadataMutex.RUnlock()

	if cb != nil && cb.pubsub != nil && hasLabels {
		msg := &pubsub.Message{
			Payload:   payload,
			Timestamp: time.Now(),
			Done:      false,
		}
		if err := cb.pubsub.Publish(topic, msg); err != nil {
			ErrorLog("[PubSub] Failed to publish packet from '%s' to topic '%s': %v",
				wantName, topic, err)
		}

		// Also log PubSub routing
		InfoLog("[PROVIDE] Want '%s' published packet to PubSub topic '%s'",
			wantName, topic)
	}
}

// ProvideDone sends a termination signal to PubSub topic
func (n *Want) ProvideDone() {
	cb := GetGlobalChainBuilder()

	n.metadataMutex.RLock()
	topic := n.Metadata.Name
	hasLabels := len(n.Metadata.Labels) > 0
	wantName := n.Metadata.Name

	if hasLabels {
		topic = serializeLabels(n.Metadata.Labels)
	}
	n.metadataMutex.RUnlock()

	if cb != nil && cb.pubsub != nil && hasLabels {
		msg := &pubsub.Message{
			Payload:   nil,
			Timestamp: time.Now(),
			Done:      true,
		}
		if err := cb.pubsub.Publish(topic, msg); err != nil {
			ErrorLog("[PubSub] Failed to publish Done signal from '%s' to topic '%s': %v",
				wantName, topic, err)
		}
		InfoLog("[PROVIDE_DONE] Want '%s' published Done signal to PubSub topic '%s'",
			wantName, topic)
	}
}
func (n *Want) GetType() string {
	return n.WantType
}
func (n *Want) GetConnectivityMetadata() ConnectivityMetadata {
	return n.ConnectivityMetadata
}
func (n *Want) GetInCount() int {
	return n.paths.GetInCount()
}
func (n *Want) GetOutCount() int {
	return n.paths.GetOutCount()
}
func (n *Want) GetPaths() *Paths {
	return &n.paths
}

// UnusedExists checks if there are unused packets in the cache or any input channel.
// It uses a single, blocking `reflect.Select` call to wait for a packet, which is
// then cached internally with its original channel index. This avoids polling loops.
// timeoutMs: wait time in milliseconds. If 0, performs a non-blocking check.
// Returns true if a packet is in the cache or received from a channel.
func (n *Want) UnusedExists(timeoutMs int) bool {
	n.cacheMutex.Lock()
	// 1. Check if a packet is already cached.
	if n.cachedPacket != nil {
		n.cacheMutex.Unlock()
		return true
	}
	n.cacheMutex.Unlock()

	paths := n.GetPaths()
	if paths == nil || len(paths.In) == 0 {
		return false
	}

	// 2. Create select cases and map to original channel indices.
	cases := make([]reflect.SelectCase, 0, len(paths.In)+1)
	channelIndexMap := make([]int, 0, len(paths.In)+1)

	for i, pathInfo := range paths.In {
		if pathInfo.Channel != nil {
			cases = append(cases, reflect.SelectCase{
				Dir:  reflect.SelectRecv,
				Chan: reflect.ValueOf(pathInfo.Channel),
			})
			channelIndexMap = append(channelIndexMap, i) // Map case index to original path index
		}
	}

	// If no valid channels, we can't select.
	if len(cases) == 0 {
		return false
	}

	// 3. Add a timeout or default case.
	if timeoutMs > 0 {
		timeoutChan := time.After(time.Duration(timeoutMs) * time.Millisecond)
		timeoutCase := reflect.SelectCase{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(timeoutChan),
		}
		cases = append(cases, timeoutCase)
	} else {
		defaultCase := reflect.SelectCase{Dir: reflect.SelectDefault}
		cases = append(cases, defaultCase)
	}

	// 4. Perform the select.
	chosen, _, ok := reflect.Select(cases)

	// 5. Process the result.
	isTimeout := (timeoutMs > 0 && chosen == len(cases)-1)
	isDefault := (timeoutMs <= 0 && chosen == len(cases)-1)

	if isTimeout || isDefault {
		if isTimeout {
			n.StoreLog("[UnusedExists] TIMEOUT after %dms (no packets found).\n", timeoutMs)
		}
		return false
	}

	if !ok {
		n.StoreLog("[UnusedExists] A channel was closed (index: %d).\n", chosen)
		return false
	}

	// 6. A packet was received. Get its original index and cache it.
	// UnusedExists should not cache; it only checks if there is something to read.
	// The actual consumption is done by Use().
	return true
}

// Init initializes the Want base type with metadata and spec, plus type-specific fields This is a helper method used by all want constructors to reduce boilerplate Usage in want types: func NewMyWant(metadata Metadata, spec WantSpec) *MyWant {
// w := &MyWant{Want: Want{}} w.Init(metadata, spec)  // Common initialization w.WantType = "my_type"  // Type-specific fields w.ConnectivityMetadata = ConnectivityMetadata{...}
func (n *Want) Init() {
	n.SetStatus(WantStatusInitializing) // Set to initializing first
	n.paths.In = []PathInfo{}
	n.paths.Out = []PathInfo{}
	n.agentRunGuard = sync.Map{} // Reset on each deployment

	// Initialize system-reserved state fields using StoreState
	n.storeState(StateFieldActionByAgent, "")
	n.storeState(StateFieldAchievingPercent, 0)
	n.storeState(StateFieldCompleted, false)

	n.SetStatus(WantStatusIdle) // Transition to idle after initialization
}

// TryStartAgentRun atomically tries to mark an agent as "running".
// Returns true (and marks as running) only when the agent is completely idle —
// i.e. neither currently running nor permanently done.
// Used by both DoAgent (executeAgent guard) and MonitorAgent (PollingAgent tick guard).
func (n *Want) TryStartAgentRun(agentName string) bool {
	_, loaded := n.agentRunGuard.LoadOrStore(agentName, "running")
	return !loaded
}

// FinishAgentRun ends an agent's run and updates its guard state.
// permanent=false: clears the "running" flag so the agent can run again next tick (MonitorAgent).
// permanent=true:  stores "done" so the agent is never re-executed until Init() (DoAgent success).
func (n *Want) FinishAgentRun(agentName string, permanent bool) {
	if permanent {
		n.agentRunGuard.Store(agentName, "done")
	} else {
		n.agentRunGuard.Delete(agentName)
	}
}

func (n *Want) getHistoryManager() *HistoryManager {
	if n.history == nil {
		hm := NewHistoryManager()
		// Inject OTEL span-event hook for state changes
		hm.OnStateEntry = func(key string, value any) {
			if n.otelSpan == nil || !IsOTELEnabled() {
				return
			}
			valStr := fmt.Sprintf("%v", value)
			if len(valStr) > 256 {
				valStr = valStr[:256] + "…"
			}
			n.otelSpan.AddEvent("state_change", trace.WithAttributes(
				attribute.String("state.key", key),
				attribute.String("state.value", valStr),
			))
		}
		n.history = hm
	}
	return n.history
}

// BuildHistory constructs a WantHistory snapshot from the ring buffers.
// Use this instead of accessing want.History directly for API responses.
func (n *Want) BuildHistory() WantHistory {
	return n.getHistoryManager().GetHistory()
}

// AddMonitoringAgent is a helper to easily create and add a polling-based monitoring agent
// It creates a PollingAgent with the specified logic and registers it
func (w *Want) AddMonitoringAgent(name string, interval time.Duration, poll PollFunc) error {
	agent := NewPollingAgent(name, interval, name, string(MonitorAgentType), poll)
	return w.AddBackgroundAgent(agent)
}

// ============================================================================
// Config and Module Error Handling
// ============================================================================

// ModuleErrorPanic is a sentinel type used to immediately terminate Progress() execution
// When SetModuleErrorAndExit() is called, it panics with this type
// StartProgressionLoop() recovers from this panic and handles cleanup gracefully
type ModuleErrorPanic struct {
	Component string
	Message   string
}

func (e ModuleErrorPanic) Error() string {
	return fmt.Sprintf("module error [%s]: %s", e.Component, e.Message)
}

// ConfigErrorPanic is a sentinel type used to immediately terminate Progress() execution
// When SetConfigErrorAndExit() is called, it panics with this type
type ConfigErrorPanic struct {
	Field   string
	Message string
}

func (e ConfigErrorPanic) Error() string {
	return fmt.Sprintf("config error [%s]: %s", e.Field, e.Message)
}

// SetConfigError marks the want as having a configuration error
// ConfigError means the input values or spec are invalid and processing cannot continue
// The want can be recovered by updating the configuration (params/spec)
// Usage in Initialize():
//
//	if locals.Topic == "" {
//	    return w.SetConfigError("topic", "Missing required parameter 'topic'")
//	}
func (w *Want) SetConfigError(field string, message string) error {
	w.storeStateMulti(map[string]any{
		"config_error_field":   field,
		"config_error_message": message,
		"error":                message,
	})
	w.StoreLog("CONFIG_ERROR: %s - %s", field, message)
	w.SetStatus(WantStatusConfigError)
	return fmt.Errorf("config error [%s]: %s", field, message)
}

// setGovernanceWarning records a governance or label access violation as a warning.
// The want continues running — governance rules are best-effort, not enforced hard stops.
// Status transitions: reaching → reaching_with_warning, achieved → achieved_with_warning.
func (w *Want) setGovernanceWarning(component string, message string) {
	w.storeStateMulti(map[string]any{
		"governance_warning_component": component,
		"governance_warning_message":   message,
	})
	w.StoreLog("[GOVERNANCE_WARNING] %s - %s", component, message)
	switch w.Status {
	case WantStatusReaching, WantStatusReachingWithWarning:
		w.SetStatus(WantStatusReachingWithWarning)
	case WantStatusAchieved, WantStatusAchievedWithWarning:
		w.SetStatus(WantStatusAchievedWithWarning)
	}
}

// SetModuleError marks the want as having a module/implementation error
// ModuleError means there's an issue with the want type implementation itself
// (e.g., GetState failure, type cast failure, nil pointer dereference in framework code)
// This typically requires code changes to fix, not configuration changes
// Usage in Progress():
//
//	if locals == nil {
//	    return w.SetModuleError("GetLocals", "Failed to access type-specific locals")
//	}
func (w *Want) SetModuleError(component string, message string) error {
	w.storeStateMulti(map[string]any{
		"module_error_component": component,
		"module_error_message":   message,
		"error":                  message,
	})
	w.StoreLog("MODULE_ERROR: %s - %s", component, message)
	w.SetStatus(WantStatusModuleError)
	return fmt.Errorf("module error [%s]: %s", component, message)
}

// SetModuleErrorAndExit sets module error and immediately terminates Progress() execution
// This uses panic/recover to exit the goroutine cleanly from deep call stacks
// StartProgressionLoop() will recover from this panic and handle cleanup
// Use this when you want to immediately stop execution without returning through the call stack
//
// Usage in Progress():
//
//	locals := w.GetLocals()
//	if locals == nil {
//	    w.SetModuleErrorAndExit("Locals", "Failed to access type-specific locals")
//	    // Code after this line will NOT execute
//	}
func (w *Want) SetModuleErrorAndExit(component string, message string) {
	w.storeStateMulti(map[string]any{
		"module_error_component": component,
		"module_error_message":   message,
		"error":                  message,
	})
	w.StoreLog("MODULE_ERROR: %s - %s (exiting immediately)", component, message)
	w.SetStatus(WantStatusModuleError)
	panic(ModuleErrorPanic{Component: component, Message: message})
}

// SetConfigErrorAndExit sets config error and immediately terminates Progress() execution
// Similar to SetModuleErrorAndExit but for configuration errors
// StartProgressionLoop() will recover and keep the want in ConfigError state waiting for config update
func (w *Want) SetConfigErrorAndExit(field string, message string) {
	w.storeStateMulti(map[string]any{
		"config_error_field":   field,
		"config_error_message": message,
		"error":                message,
	})
	w.StoreLog("CONFIG_ERROR: %s - %s (exiting immediately)", field, message)
	w.SetStatus(WantStatusConfigError)
	panic(ConfigErrorPanic{Field: field, Message: message})
}

// ClearConfigError clears config error state and transitions to Idle
// Called when user updates the configuration that caused the error
func (w *Want) ClearConfigError() {
	if w.Status != WantStatusConfigError {
		return
	}

	// Clear error-related state
	w.storeStateMulti(map[string]any{
		"config_error_field":   nil,
		"config_error_message": nil,
		"error":                nil,
	})

	// Transition back to idle for re-execution
	w.SetStatus(WantStatusIdle)
	w.StoreLog("Config error cleared, transitioning to idle")
}

// IsRecoverableError returns true if the current status is a recoverable error state
// ConfigError is recoverable (by updating config), ModuleError is not
func (w *Want) IsRecoverableError() bool {
	return w.Status == WantStatusConfigError
}

// IsErrorState returns true if the want is in any error state
func (w *Want) IsErrorState() bool {
	return w.Status == WantStatusConfigError ||
		w.Status == WantStatusModuleError ||
		w.Status == WantStatusFailed
}

// ValidateRequiredParams validates that required parameters are present and returns error if any are missing
// Returns nil if all required params are present, otherwise calls SetConfigError and returns error
// Usage in Initialize():
//
//	if err := w.ValidateRequiredParams("topic", "output_path"); err != nil {
//	    return // Want status already set to ConfigError
//	}
func (w *Want) ValidateRequiredParams(paramNames ...string) error {
	for _, paramName := range paramNames {
		value, exists := w.Spec.Params[paramName]
		if !exists {
			return w.SetConfigError(paramName, fmt.Sprintf("Missing required parameter '%s'", paramName))
		}
		// Check for empty string values
		if strVal, ok := value.(string); ok && strVal == "" {
			return w.SetConfigError(paramName, fmt.Sprintf("Required parameter '%s' cannot be empty", paramName))
		}
	}
	return nil
}

// ValidateParamFormat validates a parameter against a validation function
// Usage in Initialize():
//
//	if err := w.ValidateParamFormat("event_time", func(v any) error {
//	    if str, ok := v.(string); ok {
//	        _, err := time.Parse(time.RFC3339, str)
//	        return err
//	    }
//	    return fmt.Errorf("must be a string")
//	}); err != nil {
//	    return
//	}
func (w *Want) ValidateParamFormat(paramName string, validator func(any) error) error {
	value, exists := w.Spec.Params[paramName]
	if !exists {
		return nil // Skip validation for non-existent params (use ValidateRequiredParams for required check)
	}
	if err := validator(value); err != nil {
		return w.SetConfigError(paramName, fmt.Sprintf("Invalid format for parameter '%s': %v", paramName, err))
	}
	return nil
}

// CheckLocalsInitialized checks if Locals is properly initialized and immediately
// terminates the goroutine via panic if not. The panic is recovered by
// StartProgressionLoop() which handles cleanup gracefully.
// Usage in Progress():
//
//	locals := CheckLocalsInitialized[MyLocals](w)
//	// No nil check needed - if Locals is invalid, execution stops immediately
func CheckLocalsInitialized[T any](w *Want) *T {
	if w.Locals == nil {
		w.SetModuleErrorAndExit("Locals", "Locals not initialized - Initialize() may not have been called")
	}
	locals, ok := w.Locals.(*T)
	if !ok {
		w.SetModuleErrorAndExit("Locals", fmt.Sprintf("Failed to cast Locals to expected type %T", (*T)(nil)))
	}
	return locals
}

// SetWantTypeDefinition sets the want type definition and initializes provided state fields
func (n *Want) SetWantTypeDefinition(typeDef *WantTypeDefinition) {
	if typeDef == nil {
		return
	}
	n.metadataMutex.Lock()
	n.WantTypeDefinition = typeDef
	n.metadataMutex.Unlock()

	// Initialize labels from definition
	n.SetStateLabels(typeDef)

	// Apply default FinalResultField from type definition if not set in spec
	if n.Spec.FinalResultField == "" && typeDef.FinalResultField != "" {
		n.Spec.FinalResultField = typeDef.FinalResultField
	}

	// Extract provided state field names and initialize with default values
	n.ProvidedStateFields = make([]string, 0, len(typeDef.State))
	for _, stateDef := range typeDef.State {
		n.ProvidedStateFields = append(n.ProvidedStateFields, stateDef.Name)

		// Initialize state field with initial value if provided
		if stateDef.InitialValue != nil {
			n.State.Store(stateDef.Name, stateDef.InitialValue)
		}
	}

	// Merge type definition's Requires into Spec.Requires (deduplicated).
	// This ensures ThinkAgent / MonitorAgent declared in the type YAML are started
	// even when the spec doesn't explicitly list them.
	if len(typeDef.Requires) > 0 {
		existing := make(map[string]bool, len(n.Spec.Requires))
		for _, r := range n.Spec.Requires {
			existing[r] = true
		}
		for _, r := range typeDef.Requires {
			if !existing[r] {
				n.Spec.Requires = append(n.Spec.Requires, r)
				existing[r] = true
			}
		}
	}

	// Inject state defs declared by plugin agents (via agent.yaml state_updates).
	// For each required capability that has registered plugin state updates, merge
	// the declared fields into typeDef.State (deduplicated by field name).
	if n.agentRegistry != nil && len(n.Spec.Requires) > 0 {
		existingStateFields := make(map[string]bool, len(typeDef.State))
		for _, sd := range typeDef.State {
			existingStateFields[sd.Name] = true
		}
		for _, capName := range n.Spec.Requires {
			pluginDefs := n.agentRegistry.GetPluginStateUpdatesForCapability(capName)
			for _, sd := range pluginDefs {
				if !existingStateFields[sd.Name] {
					typeDef.State = append(typeDef.State, sd)
					existingStateFields[sd.Name] = true
					// Initialize the new field too
					if sd.InitialValue != nil {
						n.State.Store(sd.Name, sd.InitialValue)
					}
					n.ProvidedStateFields = append(n.ProvidedStateFields, sd.Name)
				}
			}
		}
	}

	// Apply parameter defaults: spec.params (highest) > default > defaultGlobalParameter (lowest)
	if n.Spec.Params == nil {
		n.Spec.Params = make(map[string]any)
	}
	for _, paramDef := range typeDef.Parameters {
		if _, exists := n.Spec.Params[paramDef.Name]; exists {
			// Explicitly provided in spec.params — highest priority, keep as-is
			continue
		}
		if paramDef.Default != nil {
			// YAML default — second priority
			n.Spec.Params[paramDef.Name] = paramDef.Default
			continue
		}
		if paramDef.DefaultGlobalParameter != "" {
			// Global parameter fallback — lowest priority
			if v, ok := GetGlobalParameter(paramDef.DefaultGlobalParameter); ok {
				n.Spec.Params[paramDef.Name] = v
			}
		}
		if paramDef.DefaultGlobalParameterFrom != "" {
			// Indirect global parameter — read another param's value as the global param key
			if keyParam, exists := n.Spec.Params[paramDef.DefaultGlobalParameterFrom]; exists {
				if keyStr, ok := keyParam.(string); ok && keyStr != "" {
					if v, ok := GetGlobalParameter(keyStr); ok {
						n.Spec.Params[paramDef.Name] = v
					}
				}
			}
		}
	}

	// globalOverrideFrom: if the named global parameter exists and is a JSON object,
	// its fields are spread into spec.params with the highest priority — overriding
	// even explicitly provided params. Useful when a single slot object (e.g. selected_slot)
	// should fully drive all parameters of a want type.
	if typeDef.GlobalOverrideFrom != "" {
		if raw, ok := GetGlobalParameter(typeDef.GlobalOverrideFrom); ok {
			var obj map[string]any
			switch v := raw.(type) {
			case map[string]any:
				obj = v
			case string:
				if err := json.Unmarshal([]byte(v), &obj); err != nil {
					n.DirectLog("[PARAM] globalOverrideFrom: failed to parse %q as JSON: %v", typeDef.GlobalOverrideFrom, err)
				}
			}
			if obj != nil {
				for k, val := range obj {
					n.Spec.Params[k] = val
				}
				n.DirectLog("[PARAM] globalOverrideFrom: applied %d fields from global param %q", len(obj), typeDef.GlobalOverrideFrom)
			}
		}
	}

}

func (n *Want) GetIntParam(key string, defaultValue int) int {
	n.metadataMutex.RLock()
	value, ok := n.Spec.Params[key]
	n.metadataMutex.RUnlock()
	if ok {
		if intVal, ok := value.(int); ok {
			return intVal
		} else if floatVal, ok := value.(float64); ok {
			return int(floatVal)
		}
	}
	return defaultValue
}
func (n *Want) GetFloatParam(key string, defaultValue float64) float64 {
	n.metadataMutex.RLock()
	value, ok := n.Spec.Params[key]
	n.metadataMutex.RUnlock()
	if ok {
		if floatVal, ok := value.(float64); ok {
			return floatVal
		} else if intVal, ok := value.(int); ok {
			return float64(intVal)
		}
	}
	return defaultValue
}
func (n *Want) GetStringParam(key string, defaultValue string) string {
	n.metadataMutex.RLock()
	value, ok := n.Spec.Params[key]
	n.metadataMutex.RUnlock()
	if ok {
		if strVal, ok := value.(string); ok {
			return strVal
		}
	}
	return defaultValue
}
func (n *Want) GetBoolParam(key string, defaultValue bool) bool {
	n.metadataMutex.RLock()
	value, ok := n.Spec.Params[key]
	n.metadataMutex.RUnlock()
	if ok {
		if boolVal, ok := value.(bool); ok {
			return boolVal
		} else if strVal, ok := value.(string); ok {
			return strVal == "true" || strVal == "True" || strVal == "TRUE" || strVal == "1"
		}
	}
	return defaultValue
}

// GetGlobalParameter returns the value from parameters.yaml for the given key,
// or defaultValue if the key is absent.
func (n *Want) GetGlobalParameter(key string, defaultValue any) any {
	if v, ok := GetGlobalParameter(key); ok {
		return v
	}
	return defaultValue
}

// count := want.IncrementIntState("total_processed")  // Returns new count
func (n *Want) IncrementIntState(key string) int {
	var newValue int
	if val, exists := n.getState(key); exists {
		if intVal, ok := val.(int); ok {
			newValue = intVal + 1
		} else {
			newValue = 1
		}
	} else {
		newValue = 1
	}

	// Use StoreState to ensure proper batching and tracking
	n.storeState(key, newValue)

	return newValue
}
func (w *Want) GetSpec() *WantSpec {
	if w == nil {
		return nil
	}
	return &w.Spec
}
func (w *Want) GetMetadata() Metadata {
	if w == nil {
		return Metadata{}
	}
	w.metadataMutex.RLock()
	defer w.metadataMutex.RUnlock()

	// Deep copy metadata to ensure thread safety
	meta := w.Metadata
	if w.Metadata.Labels != nil {
		meta.Labels = make(map[string]string, len(w.Metadata.Labels))
		for k, v := range w.Metadata.Labels {
			meta.Labels[k] = v
		}
	}
	if w.Metadata.OwnerReferences != nil {
		meta.OwnerReferences = make([]OwnerReference, len(w.Metadata.OwnerReferences))
		copy(meta.OwnerReferences, w.Metadata.OwnerReferences)
	}
	return meta
}

// GetLabels returns a copy of the want's labels map in a thread-safe way
func (n *Want) GetLabels() map[string]string {
	n.metadataMutex.RLock()
	defer n.metadataMutex.RUnlock()

	if n.Metadata.Labels == nil {
		return make(map[string]string)
	}

	// Return a deep copy to prevent external modification
	copy := make(map[string]string, len(n.Metadata.Labels))
	for k, v := range n.Metadata.Labels {
		copy[k] = v
	}
	return copy
}

// matchesSelector checks if want labels match the selector criteria
func (n *Want) matchesSelector(wantLabels map[string]string, selector map[string]string) bool {
	for key, value := range selector {
		if wantLabels[key] != value {
			return false
		}
	}
	return true
}

// emitOwnerCompletionEventIfOwned emits an OwnerCompletionEvent if this want has an owner
// This is called automatically by SetStatus() when want reaches ACHIEVED
// It's part of the standard progression cycle completion pattern
func (n *Want) emitOwnerCompletionEventIfOwned() {
	if len(n.Metadata.OwnerReferences) == 0 {
		return
	}

	for _, ownerRef := range n.Metadata.OwnerReferences {
		if ownerRef.Controller && ownerRef.Kind == "Want" {
			event := &OwnerCompletionEvent{
				BaseEvent: BaseEvent{
					EventType:  EventTypeOwnerCompletion,
					SourceName: n.Metadata.Name,
					TargetName: ownerRef.Name,
					Timestamp:  time.Now(),
					Priority:   10,
				},
				ChildName: n.Metadata.Name,
			}
			n.GetSubscriptionSystem().Emit(context.Background(), event)
			break
		}
	}
}

// Use attempts to receive data from any available input channel.
// It first checks an internal cache (filled by UnusedExists) before attempting
// to receive from the channels directly.
//
// Timeout behavior:
//   - timeoutMilliseconds < 0: infinite wait (blocks until data arrives or channels close)
//   - timeoutMilliseconds == 0: non-blocking (returns immediately if no data available)
//   - timeoutMilliseconds > 0: wait up to specified milliseconds
//
// Returns: (channelIndex, data, done, ok)
//   - channelIndex: Index of the channel that provided data (-1 if no data available)
//   - data: The data received (nil if ok is false)
//   - done: True if the sender signalled completion
//   - ok: True if data was successfully received, false if timeout or no channels
//
// Usage:
//
//	index, data, done, ok := w.Use(1000)
//	if ok {
//	    if done { ... }
//	    fmt.Printf("Received data: %v\n", data)
//	}
func (n *Want) Use(timeoutMilliseconds int) (int, any, bool, bool) {
	var rawPacket any
	var originalIndex int
	var received bool

	// 1. Check internal cache first (filled by UnusedExists)
	n.cacheMutex.Lock()
	if n.cachedPacket != nil {
		cached := n.cachedPacket
		n.cachedPacket = nil // Consume from cache
		rawPacket = cached.Packet
		originalIndex = cached.OriginalIndex
		received = true
	}
	n.cacheMutex.Unlock()

	if !received {
		// Proceed with existing channel receive logic if cache is empty
		if len(n.paths.In) == 0 {
			return -1, nil, false, false
		}

		inCount := len(n.paths.In)
		cases := make([]reflect.SelectCase, 0, inCount+1)
		channelIndexMap := make([]int, 0, inCount+1) // Maps case index -> original channel index

		for i := 0; i < inCount; i++ {
			pathInfo := n.paths.In[i]
			if pathInfo.Channel != nil {
				cases = append(cases, reflect.SelectCase{
					Dir:  reflect.SelectRecv,
					Chan: reflect.ValueOf(pathInfo.Channel),
				})
				channelIndexMap = append(channelIndexMap, i) // Track which channel index this case corresponds to
			}
		}

		// If no valid channels found, return immediately
		if len(cases) == 0 {
			return -1, nil, false, false
		}

		// Handle timeout:
		if timeoutMilliseconds >= 0 {
			timeoutChan := time.After(time.Duration(timeoutMilliseconds) * time.Millisecond)
			cases = append(cases, reflect.SelectCase{
				Dir: reflect.SelectRecv, Chan: reflect.ValueOf(timeoutChan),
			})
			channelIndexMap = append(channelIndexMap, -1) // -1 for timeout case
		}

		chosen, recv, recvOK := reflect.Select(cases)

		// If timeout case was chosen (last index in cases), no data available
		if chosen == len(cases)-1 && timeoutMilliseconds >= 0 {
			return -1, nil, false, false
		}

		// If we got here, data was received or a channel was closed.
		if recvOK {
			originalIndex = channelIndexMap[chosen]
			rawPacket = recv.Interface()
			received = true

			// Store packet receive info for debugging
			n.storeState(fmt.Sprintf("packet_received_from_channel_%d", originalIndex), time.Now().Unix())
			n.storeState("last_packet_received_timestamp", getCurrentTimestamp())
		} else {
			// Channel was closed
			return -1, nil, false, false
		}
	}

	// Unwrap TransportPacket if present
	if tp, ok := rawPacket.(TransportPacket); ok {
		return originalIndex, tp.Payload, tp.Done, true
	}

	// Legacy behavior or unwrapped packet (shouldn't happen with new Provide)
	return originalIndex, rawPacket, false, true
}

// UseForever attempts to receive data from any available input channel,
// blocking indefinitely until data arrives or all channels are closed.
// This is a convenience wrapper around Use(-1) for infinite wait.
func (n *Want) UseForever() (int, any, bool, bool) {
	return n.Use(-1)
}

// UseTyped calls Use() and auto-parses the result into a *DataObject.
// typeName specifies the expected data type (must be loaded in DataTypeLoader).
// If typeName is empty or DataTypeLoader is unavailable, wraps raw data in a generic DataObject.
func (n *Want) UseTyped(typeName string, timeoutMilliseconds int) (int, *DataObject, bool, bool) {
	idx, raw, done, ok := n.Use(timeoutMilliseconds)
	if !ok || done {
		return idx, nil, done, ok
	}
	loader := GetGlobalDataTypeLoader()
	if loader == nil || typeName == "" {
		return idx, wrapRaw(raw), done, ok
	}
	obj, err := loader.Parse(typeName, raw)
	if err != nil {
		WarnLog("[UseTyped] Failed to parse data as type %q: %v", typeName, err)
		return idx, wrapRaw(raw), done, ok
	}
	return idx, obj, done, ok
}

// UseForeverTyped calls UseForever() and auto-parses the result into a *DataObject.
func (n *Want) UseForeverTyped(typeName string) (int, *DataObject, bool, bool) {
	idx, raw, done, ok := n.UseForever()
	if !ok || done {
		return idx, nil, done, ok
	}
	loader := GetGlobalDataTypeLoader()
	if loader == nil || typeName == "" {
		return idx, wrapRaw(raw), done, ok
	}
	obj, err := loader.Parse(typeName, raw)
	if err != nil {
		WarnLog("[UseForeverTyped] Failed to parse data as type %q: %v", typeName, err)
		return idx, wrapRaw(raw), done, ok
	}
	return idx, obj, done, ok
}

// Provide validates the DataObject against its schema and sends it downstream.
// Validation errors are logged as warnings but do not block sending.
func (n *Want) Provide(obj *DataObject) {
	if obj == nil {
		n.provideRaw(nil)
		return
	}
	loader := GetGlobalDataTypeLoader()
	if loader != nil && obj.TypeName() != "" {
		if err := loader.Validate(obj.TypeName(), obj.ToMap()); err != nil {
			WarnLog("[Provide] Validation warning for type %q: %v", obj.TypeName(), err)
		}
	}
	n.provideRaw(obj.ToMap())
}

// IncrementIntStateValue safely increments an integer state value
// If the state doesn't exist, starts from defaultStart value
// Returns the new value after increment
func (n *Want) IncrementIntStateValue(key string, defaultStart int) int {
	val, exists := n.getState(key)
	if !exists {
		n.storeState(key, defaultStart+1)
		return defaultStart + 1
	}

	currentVal, ok := AsInt(val)
	if !ok {
		// If not an int, reset to default and increment
		n.storeState(key, defaultStart+1)
		return defaultStart + 1
	}

	newValue := currentVal + 1
	n.storeState(key, newValue)
	return newValue
}

// AppendToStateArray safely appends a value to a state array
// If the state doesn't exist, creates a new array
func (n *Want) AppendToStateArray(key string, value any) error {
	stateVal, exists := n.getState(key)
	var array []any

	if exists {
		if arr, ok := AsArray(stateVal); ok {
			array = arr
		} else {
			// State exists but is not an array, create new array
			array = []any{}
		}
	} else {
		// State doesn't exist, create new array
		array = []any{}
	}

	array = append(array, value)
	n.storeState(key, array)
	return nil
}

// SafeAppendToStateHistory safely appends a state history entry to the ring buffer.
func (n *Want) SafeAppendToStateHistory(entry StateHistoryEntry) error {
	n.getHistoryManager().StateHistoryRing.Append(entry)
	return nil
}

// SafeAppendToLogHistory safely appends a log history entry to the ring buffer.
func (n *Want) SafeAppendToLogHistory(entry LogHistoryEntry) error {
	n.getHistoryManager().LogHistoryRing.Append(entry)
	return nil
}

// FindRunningAgentHistory finds the most recent "running" event for the given agent.
// Returns (event, index-in-snapshot, true) or (nil, -1, false) if not found.
func (n *Want) FindRunningAgentHistory(agentName string) (*AgentExecution, int, bool) {
	snapshot := n.getHistoryManager().AgentHistoryRing.Snapshot(0)
	for i := len(snapshot) - 1; i >= 0; i-- {
		if snapshot[i].AgentName == agentName && snapshot[i].Status == "running" {
			e := snapshot[i]
			return &e, i, true
		}
	}
	return nil, -1, false
}

// GetStateArrayElement safely gets an array element from state
// Returns the element or nil if not found or state is not an array
func (n *Want) GetStateArrayElement(key string, index int) any {
	stateVal, exists := n.getState(key)
	if !exists {
		return nil
	}

	array, ok := AsArray(stateVal)
	if !ok {
		return nil
	}

	if index < 0 || index >= len(array) {
		return nil
	}

	return array[index]
}

// GetLocals returns type-specific local state for a Want
// Returns the Locals interface; caller can type-assert as needed
func (w *Want) GetLocals() WantLocals {
	return w.Locals
}

// GetExplicitState returns the explicitly defined state fields
// This includes system-reserved fields and fields defined in ProvidedStateFields
func (w *Want) GetExplicitState() map[string]any {
	explicitState := make(map[string]any)
	currentState := w.GetAllState()
	reservedFields := SystemReservedStateFields()

	for k, v := range currentState {
		// Skip internal framework fields
		if strings.HasPrefix(k, "_") {
			continue
		}

		// Always include system-reserved fields
		isReservedField := false
		for _, reserved := range reservedFields {
			if k == reserved {
				isReservedField = true
				break
			}
		}

		if isReservedField {
			explicitState[k] = v
			continue
		}

		// Include fields defined in ProvidedStateFields
		if len(w.ProvidedStateFields) > 0 && Contains(w.ProvidedStateFields, k) {
			explicitState[k] = v
		}
	}

	return explicitState
}

// GetHiddenState returns the implicitly defined state fields
// These are fields that weren't explicitly defined in the want type spec
func (w *Want) GetHiddenState() map[string]any {
	hiddenState := make(map[string]any)
	currentState := w.GetAllState()
	reservedFields := SystemReservedStateFields()

	for k, v := range currentState {
		// Skip internal framework fields
		if strings.HasPrefix(k, "_") {
			continue
		}

		// Skip system-reserved fields (they're always explicit)
		isReservedField := false
		for _, reserved := range reservedFields {
			if k == reserved {
				isReservedField = true
				break
			}
		}

		if isReservedField {
			continue
		}

		// Include fields NOT in ProvidedStateFields
		if len(w.ProvidedStateFields) > 0 && !Contains(w.ProvidedStateFields, k) {
			hiddenState[k] = v
		}
	}

	return hiddenState
}

// GetHTTPClient returns the HTTP client for internal API calls
// Returns nil if no global ChainBuilder is available
func (w *Want) GetHTTPClient() *HTTPClient {
	cb := GetGlobalChainBuilder()
	if cb == nil {
		return nil
	}
	return cb.GetHTTPClient()
}

// Contains checks if a string slice contains a specific string value
// resolveNestedStateField resolves a dot-notation field path from Want state.
// For example, "slack_latest_message.text" first fetches the "slack_latest_message"
// state key (expected to be a map) and then navigates to the "text" sub-field.
// A plain key with no dots falls back to a normal GetState call.
func resolveNestedStateField(n *Want, field string) (any, bool) {
	parts := splitFirst(field, '.')
	top, ok := n.getState(parts[0])
	if !ok || top == nil {
		return nil, false
	}
	if len(parts) == 1 {
		return top, true
	}
	// Navigate nested map(s)
	cur := top
	for _, part := range parts[1:] {
		switch m := cur.(type) {
		case map[string]any:
			v, exists := m[part]
			if !exists {
				return nil, false
			}
			cur = v
		case map[any]any:
			v, exists := m[part]
			if !exists {
				return nil, false
			}
			cur = v
		default:
			return nil, false
		}
	}
	return cur, true
}

// splitFirst splits s on the first occurrence of sep and returns all parts.
func splitFirst(s string, sep byte) []string {
	for i := 0; i < len(s); i++ {
		if s[i] == sep {
			rest := s[i+1:]
			// recursively split remainder to support multi-level nesting
			return append([]string{s[:i]}, splitFirst(rest, sep)...)
		}
	}
	return []string{s}
}

func Contains(slice []string, item string) bool {
	for _, v := range slice {
		if v == item {
			return true
		}
	}
	return false
}

// CalculateWantHash computes a hash of want's metadata, spec, all state fields, and status
// This hash is used for change detection to avoid unnecessary frontend re-renders
func CalculateWantHash(w *Want) string {
	// Build hash data structure with all relevant fields
	hashData := struct {
		Metadata Metadata       `json:"metadata"`
		Spec     WantSpec       `json:"spec"`
		Status   WantStatus     `json:"status"`
		State    map[string]any `json:"state"` // All state fields
	}{
		Metadata: w.Metadata,
		Spec:     w.Spec,
		Status:   w.Status,
		State:    w.GetAllState(), // Include all state fields
	}

	// Serialize to JSON
	jsonData, err := json.Marshal(hashData)
	if err != nil {
		// If marshaling fails, return empty string
		return ""
	}

	// Compute SHA256 hash
	hash := sha256.Sum256(jsonData)
	return hex.EncodeToString(hash[:])
}

// WantFilters contains filter criteria for want list queries
type WantFilters struct {
	Type         string            // Filter by want type
	Labels       map[string]string // Filter by labels (key=value pairs, AND logic)
	UsingFilters map[string]string // Filter by using selectors (key=value pairs, AND logic)
}

// MatchesFilters checks if a want matches all specified filters
// Returns true if the want passes all filters (AND logic)
func (w *Want) MatchesFilters(filters WantFilters) bool {
	// Filter by type if specified
	if filters.Type != "" && w.Metadata.Type != filters.Type {
		return false
	}

	// Filter by labels if specified
	if len(filters.Labels) > 0 {
		for key, value := range filters.Labels {
			if w.Metadata.Labels == nil {
				return false
			}
			labelValue, exists := w.Metadata.Labels[key]
			if !exists || labelValue != value {
				return false
			}
		}
	}

	// Filter by using selectors if specified
	if len(filters.UsingFilters) > 0 {
		for key, value := range filters.UsingFilters {
			if w.Spec.Using == nil || len(w.Spec.Using) == 0 {
				return false
			}
			// Check if any using entry contains the key=value pair
			found := false
			for _, usingEntry := range w.Spec.Using {
				if usingValue, exists := usingEntry[key]; exists && usingValue == value {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
	}

	return true
}

// syncLocalsAfterInitialize persists the locals to internal state immediately after
// Initialize() is called. Without this, the next SyncLocalsState(toStruct=true) would
// read stale internal state (e.g., old _phase = "completed") and overwrite the
// freshly initialized locals.Phase back to the old value, causing the want to
// skip re-execution on restart.
func (n *Want) syncLocalsAfterInitialize() {
	if n.progressable == nil {
		return
	}
	progressableVal := reflect.ValueOf(n.progressable)
	if progressableVal.Kind() != reflect.Ptr {
		return
	}
	method := progressableVal.MethodByName("GetLocals")
	if !method.IsValid() || method.Type().NumIn() != 0 || method.Type().NumOut() != 1 {
		return
	}
	results := method.Call(nil)
	locals := results[0].Interface()
	SyncLocalsState(n, locals, false)
}

// FilterWants filters a list of wants based on the provided filters
func FilterWants(wants []*Want, filters WantFilters) []*Want {
	filtered := make([]*Want, 0, len(wants))
	for _, want := range wants {
		if want.MatchesFilters(filters) {
			filtered = append(filtered, want)
		}
	}
	return filtered
}
