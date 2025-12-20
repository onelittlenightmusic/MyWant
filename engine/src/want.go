package mywant

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"
)

// OwnerReference represents a reference to an owner object
type OwnerReference struct {
	APIVersion         string `json:"apiVersion" yaml:"apiVersion"`
	Kind               string `json:"kind" yaml:"kind"`
	Name               string `json:"name" yaml:"name"`
	ID                 string `json:"id" yaml:"id"`
	Controller         bool   `json:"controller,omitempty" yaml:"controller,omitempty"`
	BlockOwnerDeletion bool   `json:"blockOwnerDeletion,omitempty" yaml:"blockOwnerDeletion,omitempty"`
}

// Metadata contains want identification and classification info
type Metadata struct {
	ID              string            `json:"id,omitempty" yaml:"id,omitempty"`
	Name            string            `json:"name" yaml:"name"`
	Type            string            `json:"type" yaml:"type"`
	Labels          map[string]string `json:"labels" yaml:"labels"`
	OwnerReferences []OwnerReference  `json:"ownerReferences,omitempty" yaml:"ownerReferences,omitempty"`
	UpdatedAt       int64             `json:"updatedAt" yaml:"-"` // Server-managed timestamp, ignored on load
}

// WantSpec contains the desired state configuration for a want
type WantSpec struct {
	Params              map[string]any `json:"params" yaml:"params"`
	Using               []map[string]string    `json:"using,omitempty" yaml:"using,omitempty"`
	Recipe              string                 `json:"recipe,omitempty" yaml:"recipe,omitempty"`
	StateSubscriptions  []StateSubscription    `json:"stateSubscriptions,omitempty" yaml:"stateSubscriptions,omitempty"`
	NotificationFilters []NotificationFilter   `json:"notificationFilters,omitempty" yaml:"notificationFilters,omitempty"`
	Requires            []string               `json:"requires,omitempty" yaml:"requires,omitempty"`
}

// WantHistory contains both parameter and state history
type WantHistory struct {
	ParameterHistory    []StateHistoryEntry         `json:"parameterHistory" yaml:"parameterHistory"`
	StateHistory        []StateHistoryEntry         `json:"stateHistory" yaml:"stateHistory"`
	AgentHistory        []AgentExecution            `json:"agentHistory,omitempty" yaml:"agentHistory,omitempty"`
	GroupedAgentHistory map[string][]AgentExecution `json:"groupedAgentHistory,omitempty" yaml:"groupedAgentHistory,omitempty"`
	LogHistory          []LogHistoryEntry           `json:"logHistory,omitempty" yaml:"logHistory,omitempty"`
}

// LogHistoryEntry represents a collection of log messages from a single Exec cycle
type LogHistoryEntry struct {
	Timestamp int64  `json:"timestamp" yaml:"timestamp"`
	Logs      string `json:"logs" yaml:"logs"` // Multiple log lines concatenated
}

// WantStatus represents the current state of a want
type WantStatus string

const (
	WantStatusIdle       WantStatus = "idle"
	WantStatusReaching   WantStatus = "reaching"
	WantStatusSuspended  WantStatus = "suspended"
	WantStatusAchieved   WantStatus = "achieved"
	WantStatusFailed     WantStatus = "failed"
	WantStatusTerminated WantStatus = "terminated"
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
	Type             string           // "reconcile" or "control"
	ControlCommand   *ControlCommand  // Non-nil for control triggers
	ReconcileTrigger bool             // Non-zero for reconciliation triggers
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
	Metadata Metadata               `json:"metadata" yaml:"metadata"`
	Spec     WantSpec               `json:"spec" yaml:"spec"`
	Status   WantStatus             `json:"status,omitempty" yaml:"status,omitempty"`
	State    map[string]any `json:"state,omitempty" yaml:"state,omitempty"`
	History  WantHistory            `json:"history" yaml:"history"`

	// Agent execution information
	CurrentAgent  string   `json:"current_agent,omitempty" yaml:"current_agent,omitempty"`
	RunningAgents []string `json:"running_agents,omitempty" yaml:"running_agents,omitempty"`

	// Internal fields for batching state changes during Exec cycles
	pendingStateChanges     map[string]any `json:"-" yaml:"-"`
	pendingParameterChanges map[string]any `json:"-" yaml:"-"`
	execCycleCount          int                    `json:"-" yaml:"-"`
	inExecCycle             bool                   `json:"-" yaml:"-"`
	pendingLogs             []string               `json:"-" yaml:"-"` // Buffer for logs during Exec cycle

	// Agent system
	agentRegistry     *AgentRegistry                `json:"-" yaml:"-"`
	runningAgents     map[string]context.CancelFunc `json:"-" yaml:"-"`
	agentStateChanges map[string]any        `json:"-" yaml:"-"`
	agentStateMutex   sync.RWMutex                  `json:"-" yaml:"-"`

	// Unified subscription event system
	subscriptionSystem *UnifiedSubscriptionSystem `json:"-" yaml:"-"`

	// State synchronization
	stateMutex sync.RWMutex `json:"-" yaml:"-"`

	// Stop channel for graceful shutdown of want's goroutines
	stopChannel chan struct{} `json:"-" yaml:"-"`

	// Control channel for suspend/resume/stop/restart operations
	controlChannel chan *ControlCommand `json:"-" yaml:"-"`

	// Control state tracking
	suspended bool       `json:"-" yaml:"-"` // Current suspension state
	controlMu sync.Mutex `json:"-" yaml:"-"` // Protect control state access

	// Fields for eliminating duplicate methods in want types
	WantType             string               `json:"-" yaml:"-"`
	paths                Paths                `json:"-" yaml:"-"`
	ConnectivityMetadata ConnectivityMetadata `json:"-" yaml:"-"`

	// Type-specific local state managed by WantLocals interface
	Locals WantLocals `json:"-" yaml:"-"`

	// Progressable function - concrete want implementation (e.g., RestaurantWant, QueueWant)
	progressable Progressable `json:"-" yaml:"-"`

	// Goroutine execution tracking - Want owns this state for proper encapsulation
	// ChainBuilder sets this via SetGoroutineActive() to inform Want when goroutine starts/stops
	goroutineActive   bool       `json:"-" yaml:"-"`
	goroutineActiveMu sync.RWMutex `json:"-" yaml:"-"`

	// Packet cache for non-consuming checks
	cachedPacket *CachedPacket `json:"-" yaml:"-"`
	cacheMutex   sync.Mutex   `json:"-" yaml:"-"`
}

// CachedPacket holds a packet and its original channel index for the caching mechanism.
type CachedPacket struct {
	Packet        any
	OriginalIndex int
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
			n.NotifyCompletion()
			// Automatically emit OwnerCompletionEvent to parent target if this want has an owner
			// This is part of the standard progression cycle completion pattern
			n.emitOwnerCompletionEventIfOwned()
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

// ReconcileStateFromConfig copies state from a config source atomically with proper mutex protection This method encapsulates all stateMutex access for state reconciliation, ensuring thread safety and preventing deadlocks from external callers
func (n *Want) ReconcileStateFromConfig(sourceState map[string]any) {
	if sourceState == nil {
		return
	}

	// CRITICAL: Protect State map access with stateMutex during reconciliation
	n.stateMutex.Lock()
	defer n.stateMutex.Unlock()
	if n.State == nil {
		n.State = make(map[string]any)
	}

	// Copy all state data atomically
	for k, v := range sourceState {
		n.State[k] = v
	}
}
func (n *Want) SetStateAtomic(stateData map[string]any) {
	if stateData == nil {
		return
	}

	// Acquire mutex to protect State map access
	n.stateMutex.Lock()
	defer n.stateMutex.Unlock()
	if n.State == nil {
		n.State = make(map[string]any)
	}
	for k, v := range stateData {
		n.State[k] = v
	}
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
	n.pendingStateChanges = make(map[string]any)
	n.pendingParameterChanges = make(map[string]any)
	n.pendingLogs = make([]string, 0)
}

// EndProgressCycle completes the execution cycle and commits all batched state and parameter changes
func (n *Want) EndProgressCycle() {
	if !n.inExecCycle {
		return
	}
	n.stateMutex.Lock()
	changeCount := len(n.pendingStateChanges)
	n.stateMutex.Unlock()

	if changeCount > 0 {
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
	n.goroutineActiveMu.Lock()
	defer n.goroutineActiveMu.Unlock()
	n.goroutineActive = active
}

// ShouldRetrigger determines if retrigger should happen when a packet arrives
// Returns true if goroutine is NOT running AND there are pending packets
// This encapsulates the retrigger decision logic within Want itself
func (n *Want) ShouldRetrigger() bool {
	n.goroutineActiveMu.RLock()
	isGoroutineActive := n.goroutineActive
	n.goroutineActiveMu.RUnlock()

	// Only retrigger if goroutine is NOT running
	if !isGoroutineActive {
		// Check for pending packets (non-blocking)
		// Use 0 timeout since packet should already be in channel if we're called after Provide()
		hasUnused := n.UnusedExists(0)
		InfoLog("[SHOULD-RETRIGGER] '%s' - goroutine inactive, unused packets available: %v\n", n.Metadata.Name, hasUnused)
		return hasUnused
	}
	InfoLog("[SHOULD-RETRIGGER] '%s' - goroutine is active, skipping retrigger\n", n.Metadata.Name)
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
				return
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
			n.progressable.Progress()
			// 8. End execution cycle (commit batched changes)
			n.EndProgressCycle()

			// 8.5. Check if want is achieved AFTER execution cycle (catch state changes from Progress)
			if n.progressable != nil && n.progressable.IsAchieved() {
				n.SetStatus(WantStatusAchieved)
				return
			}

			// 9. Sleep to prevent CPU spinning
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
func (n *Want) getSignificantStateChanges(oldState, newState map[string]any) map[string]any {
	significantChanges := make(map[string]any)
	for key, newVal := range newState {
		// Skip status-like fields
		isStatusField := len(key) >= 7 && key[len(key)-7:] == "_status"
		isMetadataField := key == "updated_at" || key == "last_poll_time" ||
			key == "status_changed_at" || key == "status_changed" ||
			key == "status_change_history_count"

		if isStatusField || isMetadataField {
			continue
		}

		// Include if it's a new key or has a different value
		oldVal, exists := oldState[key]
		if !exists || !n.valuesEqual(oldVal, newVal) {
			significantChanges[key] = newVal
		}
	}

	return significantChanges
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
	n.controlMu.Lock()
	defer n.controlMu.Unlock()
	return n.suspended
}
func (n *Want) SetSuspended(suspended bool) {
	n.controlMu.Lock()
	defer n.controlMu.Unlock()
	n.suspended = suspended
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
//   want.StoreLog("Calculation complete: result = 42")
func (n *Want) StoreLog(message string) {
	// Only buffer logs if we're in an Exec cycle
	if !n.inExecCycle {
		return
	}
	n.pendingLogs = append(n.pendingLogs, message)
}
func (n *Want) GetState(key string) (any, bool) {
	n.stateMutex.RLock()
	defer n.stateMutex.RUnlock()

	if n.State == nil {
		return nil, false
	}

	value, exists := n.State[key]
	return value, exists
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

// GetStateAs safely extracts and casts a state value to a specific type using generics
// Example: schedule, ok := GetStateAs[RestaurantSchedule](want, "agent_result")
func GetStateAs[T any](n *Want, key string) (T, bool) {
	value, exists := n.GetState(key)
	if !exists {
		var zero T
		return zero, false
	}
	typedVal, ok := value.(T)
	return typedVal, ok
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
	// CRITICAL: Protect all History.StateHistory access with stateMutex to prevent concurrent slice mutations
	n.stateMutex.Lock()
	defer n.stateMutex.Unlock()

	if n.State == nil {
		n.State = make(map[string]any)
	}
	stateSnapshot := make(map[string]any)
	for key, value := range n.State {
		// Skip agent metadata fields - they cause false history duplicates since they change multiple times during agent execution but don't represent actual want state changes
		if key == "current_agent" || key == "running_agents" {
			continue
		}
		stateSnapshot[key] = value
	}

	// DIFFERENTIAL CHECK: Only record if state has actually changed from last history entry
	if len(n.History.StateHistory) > 0 {
		lastEntry := n.History.StateHistory[len(n.History.StateHistory)-1]
		lastState, ok := lastEntry.StateValue.(map[string]any)
		if !ok {
			// If lastState is not the expected type, proceed with recording This handles initialization or type changes
			lastState = make(map[string]any)
		}

		// Compare current state with last recorded state
		if n.stateSnapshotsEqual(lastState, stateSnapshot) {
			// State hasn't changed, skip recording
			return
		}

		// SMART MERGING: If only status fields changed, merge into the previous entry This prevents duplicate history entries when only status_* fields are updated
		if n.isOnlyStatusChange(lastState, stateSnapshot) {
			if lastStateMap, ok := n.History.StateHistory[len(n.History.StateHistory)-1].StateValue.(map[string]any); ok {
				// Copy status and metadata fields from the new snapshot to the last entry
				for key, newVal := range stateSnapshot {
					// Copy status-like fields and metadata
					isStatusField := len(key) >= 7 && key[len(key)-7:] == "_status"
					isMetadataField := key == "updated_at" || key == "last_poll_time" ||
						key == "status_changed_at" || key == "status_changed" ||
						key == "status_change_history_count"

					if isStatusField || isMetadataField {
						lastStateMap[key] = newVal
					}
				}
				// Update timestamp to reflect the latest status change
				n.History.StateHistory[len(n.History.StateHistory)-1].Timestamp = time.Now()
			}
			return
		}
	}

	entry := StateHistoryEntry{
		WantName:   n.Metadata.Name,
		StateValue: stateSnapshot,
		Timestamp:  time.Now(),
	}
	if n.History.StateHistory == nil {
		n.History.StateHistory = make([]StateHistoryEntry, 0)
	}
	n.History.StateHistory = append(n.History.StateHistory, entry)
}
func (n *Want) addAggregatedParameterHistory() {
	if len(n.pendingParameterChanges) == 0 {
		return
	}
	entry := StateHistoryEntry{
		WantName:   n.Metadata.Name,
		StateValue: n.pendingParameterChanges,
		Timestamp:  time.Now(),
	}

	// CRITICAL: Protect History.ParameterHistory access with stateMutex to prevent concurrent slice mutations
	n.stateMutex.Lock()
	defer n.stateMutex.Unlock()
	n.History.ParameterHistory = append(n.History.ParameterHistory, entry)

	// Limit history size (keep last 50 entries for parameters)
	maxHistorySize := 50
	if len(n.History.ParameterHistory) > maxHistorySize {
		n.History.ParameterHistory = n.History.ParameterHistory[len(n.History.ParameterHistory)-maxHistorySize:]
	}

	// Clear pending parameter changes after adding to history
	for k := range n.pendingParameterChanges {
		delete(n.pendingParameterChanges, k)
	}
}
func (n *Want) addAggregatedLogHistory() {
	if len(n.pendingLogs) == 0 {
		return
	}

	// Concatenate all log messages with newlines This preserves the order and allows reading individual lines
	logsText := ""
	for i, log := range n.pendingLogs {
		if i > 0 {
			logsText += "\n"
		}
		logsText += log
	}
	entry := LogHistoryEntry{
		Timestamp: time.Now().Unix(),
		Logs:      logsText,
	}

	// CRITICAL: Protect History.LogHistory access with stateMutex to prevent concurrent slice mutations
	n.stateMutex.Lock()
	if n.History.LogHistory == nil {
		n.History.LogHistory = make([]LogHistoryEntry, 0)
	}
	n.History.LogHistory = append(n.History.LogHistory, entry)

	// Limit history size (keep last 100 entries for logs)
	maxHistorySize := 100
	if len(n.History.LogHistory) > maxHistorySize {
		n.History.LogHistory = n.History.LogHistory[len(n.History.LogHistory)-maxHistorySize:]
	}

	// Clear pending logs after adding to history
	n.pendingLogs = make([]string, 0)
	n.stateMutex.Unlock()

	// Write logs to the actual log file via InfoLog (after releasing lock to avoid holding it during I/O) Split by newlines and output each line separately so each gets a timestamp
	lines := strings.Split(logsText, "\n")
	for _, line := range lines {
		if line != "" { // Skip empty lines
			InfoLog("[%s] %s", n.Metadata.Name, line)
		}
	}
}

// copyCurrentState creates a copy of the current state
func (n *Want) copyCurrentState() map[string]any {
	n.stateMutex.Lock()
	defer n.stateMutex.Unlock()

	stateCopy := make(map[string]any)
	for key, value := range n.State {
		stateCopy[key] = value
	}
	return stateCopy
}
func (n *Want) addToParameterHistory(paramName string, paramValue any, previousValue any) {
	paramMap := map[string]any{
		paramName: paramValue,
	}

	entry := StateHistoryEntry{
		WantName:   n.Metadata.Name,
		StateValue: paramMap,
		Timestamp:  time.Now(),
	}

	// CRITICAL: Protect History.ParameterHistory access with stateMutex to prevent concurrent slice mutations
	n.stateMutex.Lock()
	defer n.stateMutex.Unlock()
	n.History.ParameterHistory = append(n.History.ParameterHistory, entry)

	// Limit history size (keep last 50 entries for parameters)
	maxHistorySize := 50
	if len(n.History.ParameterHistory) > maxHistorySize {
		n.History.ParameterHistory = n.History.ParameterHistory[len(n.History.ParameterHistory)-maxHistorySize:]
	}

	fmt.Printf("[PARAM HISTORY] Want %s: %s changed from %v to %v\n",
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
			fmt.Printf("[MIGRATION] Removed agent_history from state for want %s\n", n.Metadata.Name)
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
func (n *Want) Provide(packet any) error {
	paths := n.GetPaths()
	if paths == nil || len(paths.Out) == 0 {
		return nil // No outputs to send to
	}

	sentCount := 0
	for _, pathInfo := range paths.Out {
		if pathInfo.Channel == nil {
			continue // Skip nil channels
		}
		select {
		case pathInfo.Channel <- packet:
			// Sent successfully
			sentCount++
		default:
			// Channel is full, try blocking send
			pathInfo.Channel <- packet
			sentCount++
		}
	}

	// Store packet send info for debugging
	n.StoreState("last_provide_sent_count", sentCount)
	n.StoreState("last_provide_paths_out_count", len(paths.Out))
	n.StoreState("provide_packet_sent_timestamp", getCurrentTimestamp())

	// Trigger retrigger for each receiver that got the packet
	cb := GetGlobalChainBuilder()
	if cb != nil {
		for _, pathInfo := range paths.Out {
			if pathInfo.Channel == nil {
				continue
			}
			targetWantName := pathInfo.TargetWantName
			cb.RetriggerReceiverWant(targetWantName)
		}
	}

	return nil
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

	startTime := time.Now()

	// 4. Perform the select.
	chosen, recv, ok := reflect.Select(cases)

	// 5. Process the result.
	isTimeout := (timeoutMs > 0 && chosen == len(cases)-1)
	isDefault := (timeoutMs <= 0 && chosen == len(cases)-1)

	if isTimeout || isDefault {
		if isTimeout {
			InfoLog("[UnusedExists] TIMEOUT after %dms (no packets found).\n", timeoutMs)
		}
		return false
	}

	if !ok {
		InfoLog("[UnusedExists] A channel was closed (index: %d).\n", chosen)
		return false
	}

	// 6. A packet was received. Get its original index and cache it.
	originalIndex := channelIndexMap[chosen]
	n.cacheMutex.Lock()
	n.cachedPacket = &CachedPacket{
		Packet:        recv.Interface(),
		OriginalIndex: originalIndex,
	}
	n.cacheMutex.Unlock()

	elapsed := time.Now().Sub(startTime).Milliseconds()
	InfoLog("[UnusedExists] FOUND and CACHED packet from channel index %d in %dms\n", originalIndex, elapsed)

	return true
}



// Init initializes the Want base type with metadata and spec, plus type-specific fields This is a helper method used by all want constructors to reduce boilerplate Usage in want types: func NewMyWant(metadata Metadata, spec WantSpec) *MyWant {
// w := &MyWant{Want: Want{}} w.Init(metadata, spec)  // Common initialization w.WantType = "my_type"  // Type-specific fields w.ConnectivityMetadata = ConnectivityMetadata{...}
func (n *Want) Init() {
	n.Status = WantStatusIdle
	n.State = make(map[string]any)
	n.paths.In = []PathInfo{}
	n.paths.Out = []PathInfo{}
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
//   count := want.IncrementIntState("total_processed")  // Returns new count
func (n *Want) IncrementIntState(key string) int {
	if n.State == nil {
		n.State = make(map[string]any)
	}

	var newValue int
	if val, exists := n.State[key]; exists {
		if intVal, ok := val.(int); ok {
			newValue = intVal + 1
		} else {
			newValue = 1
		}
	} else {
		newValue = 1
	}

	n.State[key] = newValue
	if n.pendingStateChanges == nil {
		n.pendingStateChanges = make(map[string]any)
	}
	n.pendingStateChanges[key] = newValue

	return newValue
}
func (w *Want) GetSpec() *WantSpec {
	if w == nil {
		return nil
	}
	return &w.Spec
}
func (w *Want) GetMetadata() *Metadata {
	if w == nil {
		return nil
	}
	return &w.Metadata
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
// Returns: (channelIndex, data, ok)
//   - channelIndex: Index of the channel that provided data (-1 if no data available)
//   - data: The data received (nil if ok is false)
//   - ok: True if data was successfully received, false if timeout or no channels
//
// Usage:
//   index, data, ok := w.Use(1000)  // Wait up to 1 second
//   if ok {
//       fmt.Printf("Received data from channel %d: %v\n", index, data)
//   }
func (n *Want) Use(timeoutMilliseconds int) (int, any, bool) {
	// 1. Check internal cache first (filled by UnusedExists)
	n.cacheMutex.Lock()
	if n.cachedPacket != nil {
		cached := n.cachedPacket
		n.cachedPacket = nil // Consume from cache
		n.cacheMutex.Unlock()
		// Return the original index and packet from the cache
		return cached.OriginalIndex, cached.Packet, true
	}
	n.cacheMutex.Unlock()

	// Proceed with existing channel receive logic if cache is empty
	if len(n.paths.In) == 0 {
		return -1, nil, false
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
		return -1, nil, false
	}

	// Handle timeout:
	// - Negative timeout: infinite wait (no timeout case added)
	// - Zero timeout: non-blocking (add immediate timeout)
	// - Positive timeout: wait up to specified milliseconds
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
		return -1, nil, false
	}

	// If we got here, data was received or a channel was closed.
	if recvOK {
		originalIndex := channelIndexMap[chosen]

		// Store packet receive info for debugging
		n.StoreState(fmt.Sprintf("packet_received_from_channel_%d", originalIndex), time.Now().Unix())
		n.StoreState("last_packet_received_timestamp", getCurrentTimestamp())

		return originalIndex, recv.Interface(), true
	}

	// Channel was closed (recvOK is false)
	// The original code returned channelIndexMap[chosen], nil, false
	// Let's refine this to return -1 when channel is closed, to avoid confusion with valid channel index
	return -1, nil, false // Indicates channel closed or error
}

// UseForever attempts to receive data from any available input channel,
// blocking indefinitely until data arrives or all channels are closed.
// This is a convenience wrapper around Use(-1) for infinite wait.
//
// Returns: (channelIndex, data, ok)
//   - channelIndex: Index of the channel that provided data (-1 if channels closed)
//   - data: The data received (nil if ok is false)
//   - ok: True if data was successfully received, false if all channels are closed
//
// Usage:
//   index, data, ok := w.UseForever()
//   if ok {
//       fmt.Printf("Received data from channel %d: %v\n", index, data)
//   } else {
//       // All input channels are closed
//   }
func (n *Want) UseForever() (int, any, bool) {
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

// SafeAppendToStateHistory safely appends a state history entry
// Initializes StateHistory slice if nil
func (n *Want) SafeAppendToStateHistory(entry StateHistoryEntry) error {
	if n.History.StateHistory == nil {
		n.History.StateHistory = make([]StateHistoryEntry, 0)
	}
	n.History.StateHistory = append(n.History.StateHistory, entry)
	return nil
}

// SafeAppendToLogHistory safely appends a log history entry
// Initializes LogHistory slice if nil
func (n *Want) SafeAppendToLogHistory(entry LogHistoryEntry) error {
	if n.History.LogHistory == nil {
		n.History.LogHistory = make([]LogHistoryEntry, 0)
	}
	n.History.LogHistory = append(n.History.LogHistory, entry)
	return nil
}

// FindRunningAgentHistory finds a running agent in the history by name
// Returns the agent execution entry, index, or error if not found
func (n *Want) FindRunningAgentHistory(agentName string) (*AgentExecution, int, bool) {
	if n.History.AgentHistory == nil {
		return nil, -1, false
	}

	for i, agentExec := range n.History.AgentHistory {
		if agentExec.AgentName == agentName && agentExec.Status == "running" {
			return &agentExec, i, true
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

