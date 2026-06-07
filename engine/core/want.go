package mywant

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"maps"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	want_spec "github.com/onelittlenightmusic/want-spec"
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
	StateFieldAchieved         = "achieved"             // Boolean flag set true when want reaches achieved status
)

// SystemReservedStateFields returns the list of state fields automatically managed by the framework.
// These must NOT be recommended as data-flow sources in field-match recommendations.
func SystemReservedStateFields() []string {
	return []string{
		StateFieldActionByAgent,
		StateFieldAchievingPercent,
		StateFieldCompleted,
		StateFieldAchieved,
		"agent_result",
		"desired_dispatch",
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
type OwnerReference = want_spec.OwnerReference

// CorrelationEntry represents the correlation relationship between two Wants.
type CorrelationEntry = want_spec.CorrelationEntry

// Metadata contains want identification and classification info
type Metadata = want_spec.Metadata

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
type WantSpec = want_spec.WantSpec

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

	// Track whether Provide() was called in the current progress cycle.
	// Used to suppress auto-provide for wants that explicitly send output.
	providedThisCycle bool `json:"-" yaml:"-"`

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
func (n *Want) MarshalYAML() (any, error) {
	type Alias Want
	stateMap := make(map[string]any)
	n.State.Range(func(key, value any) bool {
		// Imported fields are not owned by this want — global state is the source of truth.
		// Exclude them from persistence to prevent stale copies.
		if _, isImported := n.importedLocalKey(key.(string)); isImported {
			return true
		}
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
			n.storeState("achieved", true)

			n.NotifyCompletion()
			// Automatically emit OwnerCompletionEvent to parent target if this want has an owner
			// This is part of the standard progression cycle completion pattern
			n.emitOwnerCompletionEventIfOwned()
		}

		// OpenTelemetry: manage want lifecycle span
		n.otelOnStatusChange(oldStatus, status)
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
	// Goal-labeled fields are excluded: they represent configuration set by Initialize()
	// from params, and must survive want restarts so that Initialize() can read them as
	// fallbacks (e.g. session_id for coding want).
	if n.Spec.ResetOnRestart == nil || *n.Spec.ResetOnRestart {
		cb := GetGlobalChainBuilder()
		if cb != nil {
			typeDef := cb.GetWantTypeDefinition(n.Metadata.Type)
			if typeDef != nil {
				resetState := make(map[string]any)
				for _, sd := range typeDef.State {
					if label, exists := n.StateLabels[sd.Name]; exists && label == LabelGoal {
						continue // goal state is re-set by Initialize(); don't wipe it here
					}
					resetState[sd.Name] = sd.InitialValue
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
	n.providedThisCycle = false
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

		// Auto-provide: if this want has downstream consumers and Progress() did not
		// call Provide() explicitly, send the want's current state automatically.
		// This lets isSatisfied-style check wants notify downstream consumers without
		// requiring explicit Provide() calls in every want type implementation.
		if !n.providedThisCycle && len(n.paths.Out) > 0 {
			n.provideRaw(n.GetExplicitState())
		}
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

// checkCountBounds returns false when actual is below the required minimum (if > 0)
// or above the allowed maximum (if > 0).
func checkCountBounds(actual, required, max int) bool {
	if required > 0 && actual < required {
		return false
	}
	if max > 0 && actual > max {
		return false
	}
	return true
}

// checkPreconditions verifies that path preconditions are satisfied
// Returns true if all required providers/users are connected, false otherwise
func (n *Want) checkPreconditions(paths Paths) bool {
	cm := n.ConnectivityMetadata
	return checkCountBounds(len(paths.In), cm.RequiredInputs, cm.MaxInputs) &&
		checkCountBounds(len(paths.Out), cm.RequiredOutputs, cm.MaxOutputs)
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

		// Stop lingering agents from a previous cycle BEFORE prepareForRestart() clears
		// agentRunGuard — otherwise a simultaneous ticker tick can race with the new cycle.
		n.stopAgents("before restart")

		// Reset per-run state, call Initialize(), sync locals, start background agents.
		n.initializeForRun()

		for {
			// 1. Check stop channel
			select {
			case <-n.stopChannel:
				n.SetStatus(WantStatusTerminated)
				n.stopAgents("on stop")
				return
			default:
				// Continue execution
			}

			// 2. Check control signals
			if cmd, received := n.CheckControlSignal(); received {
				switch n.handleControlSignal(cmd) {
				case loopSignalReturn:
					return
				case loopSignalContinue:
					continue
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
					n.stopAgents("on failed (pre-progress)")
					return
				}
			}

			// 3.1. Check if want is achieved (before precondition check)
			if n.progressable != nil && n.progressable.IsAchieved() {
				n.SetStatus(WantStatusAchieved)
				// CRITICAL: run one final cycle to flush FinalResultField and propagate state.
				n.BeginProgressCycle()
				n.progressable.Progress()
				n.EndProgressCycle()

				// Dynamic dispatch coordinators keep monitoring after achievement.
				if _, hasDirMap := n.GetParameter("direction_map"); hasDirMap {
					time.Sleep(GlobalExecutionInterval)
					continue
				}

				n.FlushThinkingAgents(context.Background())
				n.stopAgents("on achieved (pre-progress)")
				return
			}

			// 3.2. Check for terminal/error status
			status := n.GetStatus()
			if status == WantStatusFailed || status == WantStatusTerminated || status == WantStatusModuleError || status == WantStatusCancelled {
				n.stopAgents("on terminal state")
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

			// 3.9. Gate Progress() on imports: if any imported field is nil the
			// upstream provider has not yet produced a value. Stay idle and poll
			// until all imports resolve. This replaces the old using:when: packet
			// mechanism which was not idempotent across restarts.
			if n.hasUnresolvedImports() {
				n.SetStatus(WantStatusIdle)
				time.Sleep(GlobalExecutionInterval)
				continue
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
				n.stopAgents("on stop (pre-exec)")
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

			// Execute Progress() with panic/recover to handle immediate termination.
			// SetModuleErrorAndExit() / SetConfigErrorAndExit() use panic to exit deep stacks.
			exitLoop := n.runProgressWithRecovery()

			// 8. End execution cycle (commit batched changes)
			n.EndProgressCycle()

			if exitLoop {
				return
			}

			// 8.5-8.6. Check status after Progress() (error, cancel, achieved, failed).
			switch n.checkPostProgressStatus() {
			case loopSignalReturn:
				return
			case loopSignalContinue:
				continue
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

// StoreState writes a key-value pair directly into this want's State and stages it
// for history recording in the next EndProgressCycle() call.
//
// STATE OWNERSHIP RULE: Only call StoreState on the want that owns the state.
//   - In Progress() / Initialize(): call on receiver (b.StoreState / o.StoreState).
//   - In a DoAgent / MonitorAgent Exec: call on the want passed into the agent.
//   - NEVER call want_A.storeState(...) from code that runs on behalf of want_B.
//     Cross-want state writes bypass locking assumptions and can corrupt state history.
func (n *Want) StoreState(key string, value any) {
	// Imported fields are read-only: global state is the source of truth.
	for _, localKey := range n.Spec.Imports {
		if localKey == key {
			return
		}
	}

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
		m, ok := v.(map[string]any)
		return m, ok
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
				maps.Copy(merged, oldMap)
				maps.Copy(merged, valueMap)
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

func (n *Want) StoreLog(message string, args ...any) {
	formatted := fmt.Sprintf(message, args...)
	InfoLog("[%s] %s", n.Metadata.Name, formatted)
	n.getHistoryManager().AddLogEntry(formatted)
	n.otelEmitWantInfo(formatted)
}

func (n *Want) getState(key string) (any, bool) {
	// Imported fields are resolved live:
	//   - child wants  → parent's current state (siblings share state through the parent)
	//   - top-level    → global state store
	if globalKey, ok := n.importedLocalKey(key); ok {
		if parent := n.getParentWant(); parent != nil {
			return parent.getState(globalKey)
		}
		return GetGlobalState(globalKey)
	}
	return n.State.Load(key)
}

// importedLocalKey returns the global state key for a given local state key if it is imported,
// or ("", false) if the key is not an import.
func (n *Want) importedLocalKey(localKey string) (string, bool) {
	for globalKey, lk := range n.Spec.Imports {
		if lk == localKey {
			return globalKey, true
		}
	}
	return "", false
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
		if rv.Kind() == reflect.Pointer && !rv.IsNil() {
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

func (n *Want) InitializeSubscriptionSystem() {
	if n.subscriptionSystem == nil {
		n.subscriptionSystem = NewUnifiedSubscriptionSystem()
	}
}
func (n *Want) GetSubscriptionSystem() *UnifiedSubscriptionSystem {
	return GetGlobalSubscriptionSystem()
}

// getRawState returns the raw persisted state without imports overlay.
// Safe to call from within reconciliation (does NOT acquire reconcileMutex).
func (n *Want) getRawState() map[string]any {
	state := make(map[string]any)
	n.State.Range(func(key, value any) bool {
		state[key.(string)] = value
		return true
	})
	return state
}

func (n *Want) GetAllState() map[string]any {
	state := n.getRawState()
	// Overlay imported fields with live values so the API response always reflects
	// the current value without requiring a want restart.
	//   - child wants  → read from parent's current state
	//   - top-level    → read from global state store
	parent := n.getParentWant()
	for globalKey, localKey := range n.Spec.Imports {
		if parent != nil {
			if val, ok := parent.getState(globalKey); ok {
				state[localKey] = val
			}
		} else {
			if val, ok := GetGlobalState(globalKey); ok {
				state[localKey] = val
			}
		}
	}
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
	n.storeState("achieved", false)

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
		hm.OnStateEntry = func(key string, value any) { n.otelEmitStateChange(key, value) }
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

	// Apply parameter defaults:
	//   1. spec.params explicit literal value   (highest)
	//   2. spec.params {fromGlobalParam: key}   (want-level global ref)
	//   3. want-type YAML default value
	//   4. want-type defaultGlobalParameter      (lowest)
	if n.Spec.Params == nil {
		n.Spec.Params = make(map[string]any)
	}

	// Priority 2: resolve any {fromGlobalParam: key} entries in spec.params in-place.
	for k, v := range n.Spec.Params {
		if ref, ok := v.(map[string]any); ok {
			if key, ok := ref["fromGlobalParam"].(string); ok && key != "" {
				if resolved, ok := GetGlobalParameter(key); ok {
					n.Spec.Params[k] = resolved
				} else {
					// Key exists but no value yet — remove so lower priorities can fill
					delete(n.Spec.Params, k)
				}
			}
		}
	}

	for _, paramDef := range typeDef.Parameters {
		if _, exists := n.Spec.Params[paramDef.Name]; exists {
			// Priority 1 or 2 already resolved — keep as-is
			continue
		}
		if paramDef.Default != nil {
			// Priority 3: YAML default
			n.Spec.Params[paramDef.Name] = paramDef.Default
			continue
		}
		if paramDef.DefaultGlobalParameter != "" {
			// Priority 4: type-level global parameter fallback
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
					n.StoreLog("[PARAM] globalOverrideFrom: failed to parse %q as JSON: %v", typeDef.GlobalOverrideFrom, err)
				}
			}
			if obj != nil {
				maps.Copy(n.Spec.Params, obj)
				n.StoreLog("[PARAM] globalOverrideFrom: applied %d fields from global param %q", len(obj), typeDef.GlobalOverrideFrom)
			}
		}
	}

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
		maps.Copy(meta.Labels, w.Metadata.Labels)
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
	result := make(map[string]string, len(n.Metadata.Labels))
	maps.Copy(result, n.Metadata.Labels)
	return result
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

// isStateFieldVisible returns false for internal (underscore-prefixed) fields.
func isStateFieldVisible(k string) bool {
	return !strings.HasPrefix(k, "_")
}

// GetExplicitState returns the explicitly defined state fields
// This includes system-reserved fields and fields defined in ProvidedStateFields
func (w *Want) GetExplicitState() map[string]any {
	explicitState := make(map[string]any)
	reservedFields := SystemReservedStateFields()

	for k, v := range w.GetAllState() {
		if !isStateFieldVisible(k) {
			continue
		}
		if Contains(reservedFields, k) {
			explicitState[k] = v
			continue
		}
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
	reservedFields := SystemReservedStateFields()

	for k, v := range w.GetAllState() {
		if !isStateFieldVisible(k) {
			continue
		}
		if Contains(reservedFields, k) {
			continue
		}
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

