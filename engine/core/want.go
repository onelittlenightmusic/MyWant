package mywant

import (
	"bytes"
	"context"
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
}

// WantSpec contains the desired state configuration for a want
type WantSpec struct {
	Params              map[string]any       `json:"params" yaml:"params"`
	Using               []map[string]string  `json:"using,omitempty" yaml:"using,omitempty"`
	Recipe              string               `json:"recipe,omitempty" yaml:"recipe,omitempty"`
	StateSubscriptions  []StateSubscription  `json:"stateSubscriptions,omitempty" yaml:"stateSubscriptions,omitempty"`
	NotificationFilters []NotificationFilter `json:"notificationFilters,omitempty" yaml:"notificationFilters,omitempty"`
	Requires            []string             `json:"requires,omitempty" yaml:"requires,omitempty"`
	When                []WhenSpec           `json:"when,omitempty" yaml:"when,omitempty"`
	FinalResultField    string               `json:"finalResultField,omitempty" yaml:"finalResultField,omitempty"`
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
	WantStatusIdle              WantStatus = "idle"
	WantStatusInitializing      WantStatus = "initializing"
	WantStatusReaching          WantStatus = "reaching"
	WantStatusSuspended         WantStatus = "suspended"
	WantStatusAchieved          WantStatus = "achieved"
	WantStatusFailed            WantStatus = "failed"
	WantStatusTerminated        WantStatus = "terminated"
	WantStatusDeleting          WantStatus = "deleting"
	WantStatusConfigError       WantStatus = "config_error"        // Invalid input values or spec configuration
	WantStatusModuleError       WantStatus = "module_error"        // Want type implementation error (GetState failure, cast failure, etc.)
	WantStatusPrepareAgent      WantStatus = "prepare_agent"       // Preparing agent runtime
	WantStatusWaitingUserAction WantStatus = "waiting_user_action" // Waiting for user action (e.g., reaction approval)
)

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
	Metadata    Metadata       `json:"metadata" yaml:"metadata"`
	Spec        WantSpec       `json:"spec" yaml:"spec"`
	Status      WantStatus     `json:"status,omitempty" yaml:"status,omitempty"`
	State       map[string]any `json:"state,omitempty" yaml:"state,omitempty"`
	HiddenState map[string]any `json:"hidden_state,omitempty" yaml:"hidden_state,omitempty"`
	History     WantHistory    `json:"history" yaml:"-"`
	Hash        string         `json:"hash,omitempty" yaml:"hash,omitempty"` // Hash for change detection (metadata, spec, all state fields, status)

	// History ring buffers - private, lock-free concurrent stores (populated at runtime, not serialized)
	stateHistoryRing     *ringBuffer[StateHistoryEntry] `json:"-" yaml:"-"`
	parameterHistoryRing *ringBuffer[StateHistoryEntry] `json:"-" yaml:"-"`
	logHistoryRing       *ringBuffer[LogHistoryEntry]   `json:"-" yaml:"-"`
	agentHistoryRing     *ringBuffer[AgentExecution]    `json:"-" yaml:"-"`

	// Lifecycle control
	PreservePendingState bool `json:"-" yaml:"-"` // If true, BeginProgressCycle won't wipe pendingStateChanges

	// Agent execution information
	CurrentAgent  string   `json:"current_agent,omitempty" yaml:"current_agent,omitempty"`
	RunningAgents []string `json:"running_agents,omitempty" yaml:"running_agents,omitempty"`

	// Internal fields for batching state changes during Exec cycles
	pendingStateChanges     map[string]any `json:"-" yaml:"-"`
	pendingParameterChanges map[string]any `json:"-" yaml:"-"`
	execCycleCount          int            `json:"-" yaml:"-"`
	inExecCycle             bool           `json:"-" yaml:"-"`
	pendingLogs             []string       `json:"-" yaml:"-"` // Buffer for logs during Exec cycle

	// Agent system
	agentRegistry     *AgentRegistry                `json:"-" yaml:"-"`
	runningAgents     map[string]context.CancelFunc `json:"-" yaml:"-"`
	agentStateChanges map[string]any                `json:"-" yaml:"-"`
	agentStateMutex   sync.RWMutex                  `json:"-" yaml:"-"`

	// Background agents for long-running operations
	backgroundAgents map[string]BackgroundAgent `json:"-" yaml:"-"`
	backgroundMutex  sync.RWMutex               `json:"-" yaml:"-"`

	// Unified subscription event system
	subscriptionSystem *UnifiedSubscriptionSystem `json:"-" yaml:"-"`

	// State synchronization
	stateMutex sync.RWMutex `json:"-" yaml:"-"`

	// Hook called after MergeState completes (used by Target to signal stateNotify)
	onMergeState func() `json:"-" yaml:"-"`

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
}

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

		// Automatically notify ChainBuilder when want reaches achieved status This enables receiver wants (like Coordinators) to self-notify completion
		if status == WantStatusAchieved {
			// CRITICAL: When want achieves, always set achieving_percentage to 100%
			// Use StoreState (not MergeState) to ensure it's a confirmed value that won't be overwritten
			// MergeState adds to pendingStateChanges which can be overwritten by later StoreState calls
			n.StoreState("achieving_percentage", 100.0)

			n.NotifyCompletion()
			// Automatically emit OwnerCompletionEvent to parent target if this want has an owner
			// This is part of the standard progression cycle completion pattern
			n.emitOwnerCompletionEventIfOwned()
		}
	}
}

// RestartWant restarts the want's execution by setting its status to Idle
// This triggers the reconcile loop to re-run the want
// Used for scheduled restarts and other re-execution scenarios
func (n *Want) RestartWant() {
	n.StoreLog("[RESTART] Want '%s' restarting execution\n", n.Metadata.Name)
	n.SetStatus(WantStatusIdle)
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
	n.StoreStateMulti(sourceState)
}
func (n *Want) SetStateAtomic(stateData map[string]any) {
	if stateData == nil {
		return
	}

	// Use StoreStateMulti to ensure proper batching and mutex protection
	n.StoreStateMulti(stateData)
}

// UpdateParameter updates a parameter and propagates the change to children
func (n *Want) UpdateParameter(paramName string, paramValue any) {
	var previousValue any
	if n.Spec.Params != nil {
		previousValue = n.Spec.Params[paramName]
	}

	// Update the parameter in spec
	if n.Spec.Params == nil {
		n.Spec.Params = make(map[string]any)
	}
	n.Spec.Params[paramName] = paramValue

	// Batch parameter changes during Exec cycles (like state changes)
	if n.inExecCycle {
		if n.pendingParameterChanges == nil {
			n.pendingParameterChanges = make(map[string]any)
		}
		n.pendingParameterChanges[paramName] = paramValue
	} else {
		n.addToParameterHistory(paramName, paramValue, previousValue)
	}
	notification := StateNotification{
		SourceWantName:   n.Metadata.Name,
		StateKey:         paramName,
		StateValue:       paramValue,
		Timestamp:        time.Now(),
		NotificationType: NotificationParameter,
	}
	sendParameterNotifications(notification)
}

// BeginProgressCycle starts a new execution cycle for batching state changes
func (n *Want) BeginProgressCycle() {
	n.inExecCycle = true
	n.execCycleCount++
	// Always create fresh maps to avoid concurrent map access issues This is safer than iterating and deleting from existing maps
	// pendingStateChanges is only wiped if PreservePendingState is false
	if !n.PreservePendingState {
		n.pendingStateChanges = make(map[string]any)
	}
	n.pendingParameterChanges = make(map[string]any)
	n.pendingLogs = make([]string, 0)
}

// EndProgressCycle completes the execution cycle and commits all batched state and parameter changes
func (n *Want) EndProgressCycle() {
	if !n.inExecCycle {
		return
	}

	// CRITICAL: If achieved, ALWAYS enforce achieving_percentage = 100
	// This handles cases where SetStatus(achieved) was called in this cycle
	// but Status update might race with earlier achieving_percentage calculations
	if n.Status == WantStatusAchieved {
		n.stateMutex.Lock()
		n.pendingStateChanges["achieving_percentage"] = 100.0
		n.stateMutex.Unlock()
	}

	// Auto-override final_result from FinalResultField if configured
	if field := n.Spec.FinalResultField; field != "" {
		if val, ok := n.GetState(field); ok && val != nil {
			// Skip zero values to avoid overwriting with initial defaults
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
				n.StoreState("final_result", val)
			}
		}
	}

	n.AggregateChanges()
	if len(n.pendingLogs) > 0 {
		n.addAggregatedLogHistory()
	}

	n.inExecCycle = false
}

func (n *Want) AggregateChanges() {
	// Lock to safely copy and clear pending changes
	n.stateMutex.Lock()
	changesCopy := make(map[string]any)
	if len(n.pendingStateChanges) > 0 {
		// Copy pending changes before releasing lock
		for key, value := range n.pendingStateChanges {
			changesCopy[key] = value
		}
		// Clear pending changes after copying to prevent re-recording on next cycle
		n.pendingStateChanges = make(map[string]any)
	}

	// Apply changes INSIDE the lock to prevent concurrent map read/write
	if len(changesCopy) > 0 {
		if n.State == nil {
			n.State = make(map[string]any)
		}

		// Apply all pending changes to actual state (CRITICAL: must hold lock!)
		for key, value := range changesCopy {
			n.State[key] = value
		}
	}
	n.stateMutex.Unlock()
	if len(changesCopy) > 0 {
		n.addAggregatedStateHistory()
	}
	if len(n.pendingParameterChanges) > 0 {
		n.addAggregatedParameterHistory()

		// Clear pending parameter changes after aggregating
		n.pendingParameterChanges = make(map[string]any)
	}

	// Commit any pending logs at this checkpoint
	if len(n.pendingLogs) > 0 {
		n.addAggregatedLogHistory()
	}
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
			if n.GetStatus() == WantStatusReaching {
				n.SetStatus(WantStatusAchieved)
			}
		}()

		// Initialize the progressable before starting execution
		// This resets state for fresh execution (especially important for restarts)
		if n.progressable != nil {
			n.progressable.Initialize()
		}

		// Phase 0: Initial agent execution
		// Ensure background agents are started BEFORE entering the loop where achievement is checked.
		if err := n.ExecuteAgents(); err != nil {
			n.StoreLog("ERROR: Failed to execute agents during loop startup: %v", err)
		}

		for {
			// 1. Check stop channel
			select {
			case <-n.stopChannel:
				n.SetStatus(WantStatusTerminated)
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
					if n.progressable != nil {
						n.progressable.Initialize()
					}
				}
			}

			// 3. Skip execution if suspended
			if n.IsSuspended() {
				time.Sleep(GlobalExecutionInterval)
				continue
			}

			// 3.1. Check if want is achieved (before precondition check)
			if n.progressable != nil && n.progressable.IsAchieved() {
				n.SetStatus(WantStatusAchieved)
				// CRITICAL: Even if already achieved, we must run one cycle to ensure 
				// FinalResultField is processed and state is aggregated.
				n.BeginProgressCycle()
				n.EndProgressCycle()

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
			if status == WantStatusFailed || status == WantStatusTerminated || status == WantStatusModuleError {
				// Terminal error states - exit goroutine
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

			// 4.5. Execute required agents (if not already running)
			// This ensures persistent agents like ThinkAgents are always kept running
			// and handles any dynamic requirement changes.
			if err := n.ExecuteAgents(); err != nil {
				n.StoreLog("ERROR: Failed to execute agents: %v", err)
			}

			// 5. Begin execution cycle (batching mode)
			n.BeginProgressCycle()

			// 6. Check stop channel before execution
			select {
			case <-n.stopChannel:
				n.EndProgressCycle()
				n.SetStatus(WantStatusTerminated)
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
				n.progressable.Progress()
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
			if currentStatus == WantStatusConfigError {
				// Config error - wait for config update (will be handled in next iteration)
				continue
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
func (n *Want) StoreState(key string, value any) {
	// CRITICAL: Always use mutex to protect both State and pendingStateChanges Agent goroutines can call StoreState concurrently, so we must synchronize access
	n.stateMutex.Lock()
	previousValue, exists := n.getStateUnsafe(key)
	if exists && n.valuesEqual(previousValue, value) {
		// No change, skip entirely
		n.stateMutex.Unlock()
		return
	}

	// Value has changed, store it Store the state - preserve existing State to maintain parameterHistory
	if n.State == nil {
		n.State = make(map[string]any)
	}
	n.State[key] = value

	// Stage the change in pending state changes This allows us to batch related changes and create minimal history entries
	if n.pendingStateChanges == nil {
		n.pendingStateChanges = make(map[string]any)
	}
	n.pendingStateChanges[key] = value
	n.stateMutex.Unlock()
	notification := StateNotification{
		SourceWantName: n.Metadata.Name,
		StateKey:       key,
		StateValue:     value,
		PreviousValue:  previousValue,
		Timestamp:      time.Now(),
	}
	sendStateNotifications(notification)
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
	n.stateMutex.Lock()
	defer n.stateMutex.Unlock()

	// Stage all updates in pending state changes for batch processing
	if n.pendingStateChanges == nil {
		n.pendingStateChanges = make(map[string]any)
	}

	// Helper function to extract map from any
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

		if isValueMap {
			// Check pending state first, then fall back to persisted State
			var existingMap map[string]any
			if existingVal, exists := n.pendingStateChanges[key]; exists {
				existingMap, _ = extractMap(existingVal)
			} else if stateVal, exists := n.State[key]; exists {
				existingMap, _ = extractMap(stateVal)
			}

			// If we found an existing map, merge it
			if existingMap != nil {
				// Deep merge: combine existing and new map entries
				merged := make(map[string]any, len(existingMap)+len(valueMap))
				for k, v := range existingMap {
					merged[k] = v
				}
				for k, v := range valueMap {
					merged[k] = v
				}
				n.pendingStateChanges[key] = merged
				continue
			}
		}
		// For non-map values or when no existing value, just set directly
		n.pendingStateChanges[key] = value
	}

	// Signal hook (e.g. Target.stateNotify) — buffered channel so non-blocking
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

// HasParent returns true if this want has a parent coordinator.
func (n *Want) HasParent() bool {
	return n.getParentWant() != nil
}

func (n *Want) GetParentState(key string) (any, bool) {
	parent := n.getParentWant()
	if parent == nil {
		return nil, false
	}
	return parent.GetState(key)
}

func (n *Want) StoreParentState(key string, value any) {
	parent := n.getParentWant()
	if parent == nil {
		log.Printf("[WARN] Want '%s' has no parent, cannot store parent state '%s'", n.Metadata.Name, key)
		return
	}
	parent.StoreState(key, value)
}

func (n *Want) MergeParentState(updates map[string]any) {
	parent := n.getParentWant()
	if parent == nil {
		log.Printf("[WARN] Want '%s' has no parent, cannot merge parent state", n.Metadata.Name)
		return
	}
	parent.MergeState(updates)
}

// "description_received": true, "description_text": "some text", "description_provided": true, })
func (n *Want) StoreStateMulti(updates map[string]any) {
	// CRITICAL: Use mutex to protect both State and pendingStateChanges
	n.stateMutex.Lock()

	// Collect all notifications before releasing the lock
	var notifications []StateNotification
	for key, value := range updates {
		previousValue, exists := n.getStateUnsafe(key)
		if exists && n.valuesEqual(previousValue, value) {
			// No change, skip this key
			continue
		}

		// Value has changed, store it Store the state - preserve existing State to maintain history
		if n.State == nil {
			n.State = make(map[string]any)
		}
		n.State[key] = value

		// Stage the change in pending state changes
		if n.pendingStateChanges == nil {
			n.pendingStateChanges = make(map[string]any)
		}
		n.pendingStateChanges[key] = value

		// Prepare notification for this change
		notification := StateNotification{
			SourceWantName: n.Metadata.Name,
			StateKey:       key,
			StateValue:     value,
			PreviousValue:  previousValue,
			Timestamp:      time.Now(),
		}
		notifications = append(notifications, notification)
	}

	n.stateMutex.Unlock()
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
	// Only buffer logs if we're in an Exec cycle
	if !n.inExecCycle {
		return
	}
	n.pendingLogs = append(n.pendingLogs, fmt.Sprintf(message, args...))
}

func (n *Want) GetState(key string) (any, bool) {
	n.stateMutex.RLock()
	defer n.stateMutex.RUnlock()

	// CRITICAL: Check pendingStateChanges first to see latest updates
	// This ensures GetState returns values from concurrent MergeState() calls immediately
	// without waiting for EndProgressCycle() to dump to disk
	if n.pendingStateChanges != nil {
		if value, exists := n.pendingStateChanges[key]; exists {
			return value, true
		}
	}

	// Fallback to persisted State if not in pending
	if n.State == nil {
		return nil, false
	}

	value, exists := n.State[key]
	return value, exists
}

// GetPendingStateChanges returns a copy of pending state changes (changes not yet committed)
// This is useful for external agent execution to return only changed fields
func (n *Want) GetPendingStateChanges() map[string]any {
	n.stateMutex.RLock()
	defer n.stateMutex.RUnlock()

	if n.pendingStateChanges == nil {
		return make(map[string]any)
	}

	// Return a copy to avoid concurrent access issues
	changes := make(map[string]any, len(n.pendingStateChanges))
	for k, v := range n.pendingStateChanges {
		changes[k] = v
	}
	return changes
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
	value, exists := n.GetState(key)
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
	value, exists := n.GetState(key)
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
	value, exists := n.GetState(key)
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
	value, exists := n.GetState(key)
	if !exists {
		return defaultValue, false
	}
	if strVal, ok := value.(string); ok {
		return strVal, true
	}
	return defaultValue, false
}

func (n *Want) GetStateTime(key string, defaultValue time.Time) (time.Time, bool) {
	value, exists := n.GetState(key)
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
	value, exists := n.GetState(key)
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
		val, exists := n.GetState(key)
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

// ENHANCEMENT: Merges status-only changes into the previous entry instead of creating new entries
func (n *Want) addAggregatedStateHistory() {
	n.initHistoryRings()

	// Read state under short read-lock, then release before ring buffer operations
	n.stateMutex.RLock()
	if n.State == nil {
		n.stateMutex.RUnlock()
		return
	}
	stateSnapshot := make(map[string]any)
	reservedFields := SystemReservedStateFields()
	for key, value := range n.State {
		// Skip internal fields starting with underscore - they are internal markers only
		if strings.HasPrefix(key, "_") {
			continue
		}
		// Always include system-reserved fields in history
		isReservedField := false
		for _, reserved := range reservedFields {
			if key == reserved {
				isReservedField = true
				break
			}
		}
		if isReservedField {
			stateSnapshot[key] = value
			continue
		}
		// If ProvidedStateFields is defined, only include those fields in history
		if len(n.ProvidedStateFields) > 0 && !Contains(n.ProvidedStateFields, key) {
			continue
		}
		stateSnapshot[key] = value
	}
	n.stateMutex.RUnlock()

	// Extract action_by_agent from state snapshot for StateHistoryEntry
	var actionByAgent string
	if agentType, ok := stateSnapshot["action_by_agent"].(string); ok {
		actionByAgent = agentType
	}

	// DIFFERENTIAL CHECK: Only record if state has actually changed from last history entry
	if lastEntry, ok := n.stateHistoryRing.PeekLast(); ok {
		lastState, ok := lastEntry.StateValue.(map[string]any)
		if !ok {
			lastState = make(map[string]any)
		}

		// Compare current state with last recorded state
		if n.stateSnapshotsEqual(lastState, stateSnapshot) {
			return
		}

		// SMART MERGING: If only status fields changed, merge into the previous entry
		if n.isOnlyStatusChange(lastState, stateSnapshot) {
			captured := actionByAgent
			snapshotCopy := stateSnapshot
			n.stateHistoryRing.UpdateLast(func(e *StateHistoryEntry) {
				if lastStateMap, ok := e.StateValue.(map[string]any); ok {
					for key, newVal := range snapshotCopy {
						isStatusField := len(key) >= 7 && key[len(key)-7:] == "_status"
						isMetadataField := key == "updated_at" || key == "last_poll_time" ||
							key == "status_changed_at" || key == "status_changed" ||
							key == "status_change_history_count"
						if isStatusField || isMetadataField {
							lastStateMap[key] = newVal
						}
					}
				}
				e.Timestamp = time.Now()
				if captured != "" {
					e.ActionByAgent = captured
				}
			})
			return
		}
	}

	entry := StateHistoryEntry{
		WantName:      n.Metadata.Name,
		StateValue:    stateSnapshot,
		Timestamp:     time.Now(),
		ActionByAgent: actionByAgent,
	}
	n.stateHistoryRing.Append(entry)
}
func (n *Want) addAggregatedParameterHistory() {
	if len(n.pendingParameterChanges) == 0 {
		return
	}
	n.initHistoryRings()
	entry := StateHistoryEntry{
		WantName:   n.Metadata.Name,
		StateValue: n.pendingParameterChanges,
		Timestamp:  time.Now(),
	}
	n.parameterHistoryRing.Append(entry)

	// Clear pending parameter changes after adding to history
	for k := range n.pendingParameterChanges {
		delete(n.pendingParameterChanges, k)
	}
}
func (n *Want) addAggregatedLogHistory() {
	if len(n.pendingLogs) == 0 {
		return
	}
	n.initHistoryRings()

	// Concatenate all log messages with newlines This preserves the order and allows reading individual lines
	logsText := ""
	for i, log := range n.pendingLogs {
		if i > 0 {
			logsText += "\n"
		}
		logsText += log
	}
	entry := LogHistoryEntry{
		Timestamp: time.Now(),
		Logs:      logsText,
	}
	n.logHistoryRing.Append(entry)
	n.pendingLogs = make([]string, 0)

	// Write logs to the actual log file via InfoLog
	// Split by newlines and output each line separately so each gets a timestamp
	lines := strings.Split(logsText, "\n")
	for _, line := range lines {
		if line != "" { // Skip empty lines
			InfoLog("[%s] %s", n.Metadata.Name, line)
		}
	}
}

func (n *Want) addToParameterHistory(paramName string, paramValue any, previousValue any) {
	n.initHistoryRings()
	paramMap := Dict{
		paramName: paramValue,
	}
	entry := StateHistoryEntry{
		WantName:   n.Metadata.Name,
		StateValue: paramMap,
		Timestamp:  time.Now(),
	}
	n.parameterHistoryRing.Append(entry)
	DebugLog("[PARAM HISTORY] Want %s: %s changed from %v to %v\n",
		n.Metadata.Name, paramName, previousValue, paramValue)
}
func (n *Want) GetParameter(paramName string) (any, bool) {
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

// migrateAgentHistoryFromState removes agent_history from state if it exists This ensures agent history is only stored in the top-level AgentHistory field
func (n *Want) migrateAgentHistoryFromState() {
	n.stateMutex.Lock()
	defer n.stateMutex.Unlock()

	if n.State != nil {
		if _, exists := n.State["agent_history"]; exists {
			delete(n.State, "agent_history")
			log.Printf("[MIGRATION] Removed agent_history from state for want %s\n", n.Metadata.Name)
		}
	}
}
func (n *Want) getStateUnsafe(key string) (any, bool) {
	if n.State == nil {
		return nil, false
	}
	value, exists := n.State[key]
	return value, exists
}
func (n *Want) GetAllState() map[string]any {
	n.stateMutex.RLock()
	defer n.stateMutex.RUnlock()

	if n.State == nil {
		return make(map[string]any)
	}
	stateCopy := make(map[string]any)
	for k, v := range n.State {
		stateCopy[k] = v
	}
	return stateCopy
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
		n.StoreState(key, value)
	}
	n.StoreState("completion_time", fmt.Sprintf("%d", getCurrentTimestamp()))

	// Commit any pending state changes into a single batched history entry
	n.CommitStateChanges()

	// Stop all running agents
	n.StopAllAgents()

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
		n.StoreState(key, value)
	}
	n.StoreState("error", err.Error())
	n.StoreState("failure_time", fmt.Sprintf("%d", getCurrentTimestamp()))

	// Commit any pending state changes into a single batched history entry
	n.CommitStateChanges()

	// Stop all running agents
	n.StopAllAgents()

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

// Provide sends a data packet to PubSub topic for subscribers
func (n *Want) Provide(payload any) {
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
	n.State = make(map[string]any)
	n.paths.In = []PathInfo{}
	n.paths.Out = []PathInfo{}
	n.initHistoryRings()

	// Initialize system-reserved state fields using StoreState
	n.StoreState(StateFieldActionByAgent, "")
	n.StoreState(StateFieldAchievingPercent, 0)
	n.StoreState(StateFieldCompleted, false)

	n.SetStatus(WantStatusIdle) // Transition to idle after initialization
}

// initHistoryRings lazily allocates ring buffers for history tracking.
// Safe to call multiple times (no-op if already initialized).
func (n *Want) initHistoryRings() {
	if n.stateHistoryRing == nil {
		n.stateHistoryRing = newRingBuffer[StateHistoryEntry](200)
	}
	if n.parameterHistoryRing == nil {
		n.parameterHistoryRing = newRingBuffer[StateHistoryEntry](50)
	}
	if n.logHistoryRing == nil {
		n.logHistoryRing = newRingBuffer[LogHistoryEntry](100)
	}
	if n.agentHistoryRing == nil {
		n.agentHistoryRing = newRingBuffer[AgentExecution](100)
	}
}

// BuildHistory constructs a WantHistory snapshot from the ring buffers.
// Use this instead of accessing want.History directly for API responses.
func (n *Want) BuildHistory() WantHistory {
	n.initHistoryRings()
	return WantHistory{
		StateHistory:     n.stateHistoryRing.Snapshot(0),
		ParameterHistory: n.parameterHistoryRing.Snapshot(0),
		LogHistory:       n.logHistoryRing.Snapshot(0),
		AgentHistory:     n.agentHistoryRing.Snapshot(0),
	}
}

// AddMonitoringAgent is a helper to easily create and add a polling-based monitoring agent
// It creates a PollingAgent with the specified logic and registers it
func (w *Want) AddMonitoringAgent(name string, interval time.Duration, poll PollFunc) error {
	agent := NewPollingAgent(name, interval, "MonitorAgent", poll)
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
	w.StoreStateMulti(map[string]any{
		"config_error_field":   field,
		"config_error_message": message,
		"error":                message,
	})
	w.StoreLog("CONFIG_ERROR: %s - %s", field, message)
	w.SetStatus(WantStatusConfigError)
	return fmt.Errorf("config error [%s]: %s", field, message)
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
	w.StoreStateMulti(map[string]any{
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
	w.StoreStateMulti(map[string]any{
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
	w.StoreStateMulti(map[string]any{
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
	w.StoreStateMulti(map[string]any{
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

	n.stateMutex.Lock()
	defer n.stateMutex.Unlock()

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
			// Using direct map access as we hold the lock
			if n.State == nil {
				n.State = make(map[string]any)
			}
			n.State[stateDef.Name] = stateDef.InitialValue
		}
	}
}

func (n *Want) GetIntParam(key string, defaultValue int) int {
	if value, ok := n.Spec.Params[key]; ok {
		if intVal, ok := value.(int); ok {
			return intVal
		} else if floatVal, ok := value.(float64); ok {
			return int(floatVal)
		}
	}
	return defaultValue
}
func (n *Want) GetFloatParam(key string, defaultValue float64) float64 {
	if value, ok := n.Spec.Params[key]; ok {
		if floatVal, ok := value.(float64); ok {
			return floatVal
		} else if intVal, ok := value.(int); ok {
			return float64(intVal)
		}
	}
	return defaultValue
}
func (n *Want) GetStringParam(key string, defaultValue string) string {
	if value, ok := n.Spec.Params[key]; ok {
		if strVal, ok := value.(string); ok {
			return strVal
		}
	}
	return defaultValue
}
func (n *Want) GetBoolParam(key string, defaultValue bool) bool {
	if value, ok := n.Spec.Params[key]; ok {
		if boolVal, ok := value.(bool); ok {
			return boolVal
		} else if strVal, ok := value.(string); ok {
			return strVal == "true" || strVal == "True" || strVal == "TRUE" || strVal == "1"
		}
	}
	return defaultValue
}

// count := want.IncrementIntState("total_processed")  // Returns new count
func (n *Want) IncrementIntState(key string) int {
	var newValue int
	if val, exists := n.GetState(key); exists {
		if intVal, ok := val.(int); ok {
			newValue = intVal + 1
		} else {
			newValue = 1
		}
	} else {
		newValue = 1
	}

	// Use StoreState to ensure proper batching and tracking
	n.StoreState(key, newValue)

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
			n.StoreState(fmt.Sprintf("packet_received_from_channel_%d", originalIndex), time.Now().Unix())
			n.StoreState("last_packet_received_timestamp", getCurrentTimestamp())
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

// IncrementIntStateValue safely increments an integer state value
// If the state doesn't exist, starts from defaultStart value
// Returns the new value after increment
func (n *Want) IncrementIntStateValue(key string, defaultStart int) int {
	val, exists := n.GetState(key)
	if !exists {
		n.StoreState(key, defaultStart+1)
		return defaultStart + 1
	}

	currentVal, ok := AsInt(val)
	if !ok {
		// If not an int, reset to default and increment
		n.StoreState(key, defaultStart+1)
		return defaultStart + 1
	}

	newValue := currentVal + 1
	n.StoreState(key, newValue)
	return newValue
}

// AppendToStateArray safely appends a value to a state array
// If the state doesn't exist, creates a new array
func (n *Want) AppendToStateArray(key string, value any) error {
	stateVal, exists := n.GetState(key)
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
	n.StoreState(key, array)
	return nil
}

// SafeAppendToStateHistory safely appends a state history entry to the ring buffer.
func (n *Want) SafeAppendToStateHistory(entry StateHistoryEntry) error {
	n.initHistoryRings()
	n.stateHistoryRing.Append(entry)
	return nil
}

// SafeAppendToLogHistory safely appends a log history entry to the ring buffer.
func (n *Want) SafeAppendToLogHistory(entry LogHistoryEntry) error {
	n.initHistoryRings()
	n.logHistoryRing.Append(entry)
	return nil
}

// FindRunningAgentHistory finds the most recent "running" event for the given agent.
// Returns (event, index-in-snapshot, true) or (nil, -1, false) if not found.
func (n *Want) FindRunningAgentHistory(agentName string) (*AgentExecution, int, bool) {
	n.initHistoryRings()
	snapshot := n.agentHistoryRing.Snapshot(0)
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
	stateVal, exists := n.GetState(key)
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
		State:    w.State, // Include all state fields
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
