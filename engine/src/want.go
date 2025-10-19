package mywant

import (
	"context"
	"fmt"
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
}

// WantSpec contains the desired state configuration for a want
type WantSpec struct {
	Params              map[string]interface{} `json:"params" yaml:"params"`
	Using               []map[string]string    `json:"using,omitempty" yaml:"using,omitempty"`
	StateSubscriptions  []StateSubscription    `json:"stateSubscriptions,omitempty" yaml:"stateSubscriptions,omitempty"`
	NotificationFilters []NotificationFilter   `json:"notificationFilters,omitempty" yaml:"notificationFilters,omitempty"`
	Requires            []string               `json:"requires,omitempty" yaml:"requires,omitempty"`
}

// WantHistory contains both parameter and state history
type WantHistory struct {
	ParameterHistory []StateHistoryEntry `json:"parameterHistory" yaml:"parameterHistory"`
	StateHistory     []StateHistoryEntry `json:"stateHistory" yaml:"stateHistory"`
	AgentHistory     []AgentExecution    `json:"agentHistory,omitempty" yaml:"agentHistory,omitempty"`
}

// WantStatus represents the current state of a want
type WantStatus string

const (
	WantStatusIdle       WantStatus = "idle"
	WantStatusRunning    WantStatus = "running"
	WantStatusCompleted  WantStatus = "completed"
	WantStatusFailed     WantStatus = "failed"
	WantStatusTerminated WantStatus = "terminated"
)

// WantStats is deprecated - use State field instead
// Keeping type alias for backward compatibility during transition
type WantStats = map[string]interface{}

// getCurrentTimestamp returns current Unix timestamp
func getCurrentTimestamp() int64 {
	return time.Now().Unix()
}

// WantFactory defines the interface for creating want functions
type WantFactory func(metadata Metadata, spec WantSpec) interface{}

// Want represents a processing unit in the chain
type Want struct {
	Metadata Metadata               `json:"metadata" yaml:"metadata"`
	Spec     WantSpec               `json:"spec" yaml:"spec"`
	Status   WantStatus             `json:"status,omitempty" yaml:"status,omitempty"`
	State    map[string]interface{} `json:"state,omitempty" yaml:"state,omitempty"`
	History  WantHistory            `json:"history" yaml:"history"`

	// Agent execution information
	CurrentAgent  string   `json:"current_agent,omitempty" yaml:"current_agent,omitempty"`
	RunningAgents []string `json:"running_agents,omitempty" yaml:"running_agents,omitempty"`

	// Internal fields for batching state changes during Exec cycles
	pendingStateChanges     map[string]interface{} `json:"-" yaml:"-"`
	pendingParameterChanges map[string]interface{} `json:"-" yaml:"-"`
	execCycleCount          int                    `json:"-" yaml:"-"`
	inExecCycle             bool                   `json:"-" yaml:"-"`

	// Agent system
	agentRegistry     *AgentRegistry                `json:"-" yaml:"-"`
	runningAgents     map[string]context.CancelFunc `json:"-" yaml:"-"`
	agentStateChanges map[string]interface{}        `json:"-" yaml:"-"`
	agentStateMutex   sync.RWMutex                  `json:"-" yaml:"-"`

	// Unified subscription event system
	subscriptionSystem *UnifiedSubscriptionSystem `json:"-" yaml:"-"`

	// State synchronization
	stateMutex sync.RWMutex `json:"-" yaml:"-"`

	// Stop channel for graceful shutdown of want's goroutines
	stopChannel chan struct{} `json:"-" yaml:"-"`
}

// SetStatus updates the want's status and emits StatusChange event
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
	}
}

// UpdateParameter updates a parameter and propagates the change to children
func (n *Want) UpdateParameter(paramName string, paramValue interface{}) {
	// Get previous value for history tracking
	var previousValue interface{}
	if n.Spec.Params != nil {
		previousValue = n.Spec.Params[paramName]
	}

	// Update the parameter in spec
	if n.Spec.Params == nil {
		n.Spec.Params = make(map[string]interface{})
	}
	n.Spec.Params[paramName] = paramValue

	// Batch parameter changes during Exec cycles (like state changes)
	if n.inExecCycle {
		if n.pendingParameterChanges == nil {
			n.pendingParameterChanges = make(map[string]interface{})
		}
		n.pendingParameterChanges[paramName] = paramValue
	} else {
		// Add to parameter history immediately if not in exec cycle
		n.addToParameterHistory(paramName, paramValue, previousValue)
	}

	// Create parameter change notification to propagate to children
	notification := StateNotification{
		SourceWantName:   n.Metadata.Name,
		StateKey:         paramName,
		StateValue:       paramValue,
		Timestamp:        time.Now(),
		NotificationType: NotificationParameter,
	}

	// Send parameter change notification to children
	sendParameterNotifications(notification)
}

// BeginExecCycle starts a new execution cycle for batching state changes
func (n *Want) BeginExecCycle() {
	n.inExecCycle = true
	n.execCycleCount++
	if n.pendingStateChanges == nil {
		n.pendingStateChanges = make(map[string]interface{})
	}
	if n.pendingParameterChanges == nil {
		n.pendingParameterChanges = make(map[string]interface{})
	}
	// Clear pending changes for new cycle
	for k := range n.pendingStateChanges {
		delete(n.pendingStateChanges, k)
	}
	for k := range n.pendingParameterChanges {
		delete(n.pendingParameterChanges, k)
	}
}

// EndExecCycle completes the execution cycle and commits all batched state and parameter changes
func (n *Want) EndExecCycle() {
	if !n.inExecCycle {
		return
	}

	// Handle state changes
	n.AggregateChanges()

	n.inExecCycle = false
}

func (n *Want) AggregateChanges() {
	// Lock to safely copy and clear pending changes
	n.stateMutex.Lock()
	changesCopy := make(map[string]interface{})
	if len(n.pendingStateChanges) > 0 {
		// Copy pending changes before releasing lock
		for key, value := range n.pendingStateChanges {
			changesCopy[key] = value
		}
		// Clear pending changes after copying to prevent re-recording on next cycle
		n.pendingStateChanges = make(map[string]interface{})
	}
	n.stateMutex.Unlock()

	// Apply changes outside the lock
	if len(changesCopy) > 0 {
		// Create a single aggregated state history entry with complete state snapshot
		if n.State == nil {
			n.State = make(map[string]interface{})
		}

		// Apply all pending changes to actual state
		for key, value := range changesCopy {
			n.State[key] = value
		}

		// Create one history entry with the complete state snapshot
		n.addAggregatedStateHistory()
	}
	// Handle parameter changes
	if len(n.pendingParameterChanges) > 0 {
		// Create one aggregated parameter history entry
		n.addAggregatedParameterHistory()

		// Clear pending parameter changes after aggregating
		n.pendingParameterChanges = make(map[string]interface{})
	}
}

// valuesEqual compares two interface{} values for equality
// Handles different types properly including strings, numbers, booleans, etc.
func (n *Want) valuesEqual(val1, val2 interface{}) bool {
	// Handle nil cases
	if val1 == nil && val2 == nil {
		return true
	}
	if val1 == nil || val2 == nil {
		return false
	}

	// Try direct comparison first (works for strings, numbers, booleans)
	return fmt.Sprintf("%v", val1) == fmt.Sprintf("%v", val2)
}

// stateSnapshotsEqual compares two state snapshots (maps) for deep equality
// Returns true if both maps have identical keys and values
func (n *Want) stateSnapshotsEqual(snapshot1, snapshot2 map[string]interface{}) bool {
	// Check if lengths match
	if len(snapshot1) != len(snapshot2) {
		return false
	}

	// Check if all keys and values match
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

// GetStatus returns the current want status
func (n *Want) GetStatus() WantStatus {
	return n.Status
}

// StoreState stores a key-value pair in the want's state
// Only adds to state history if the value has actually changed (differential tracking)
func (n *Want) StoreState(key string, value interface{}) {
	n.stateMutex.Lock()
	defer n.stateMutex.Unlock()

	// Get previous value to check if it's actually different
	previousValue, exists := n.getStateUnsafe(key)

	// Check if the value has actually changed (DIFFERENTIAL CHECK)
	if exists && n.valuesEqual(previousValue, value) {
		// No change, skip entirely - don't even stage it
		return
	}

	// If we're in an exec cycle, batch only the CHANGED values
	if n.inExecCycle {
		if n.pendingStateChanges == nil {
			n.pendingStateChanges = make(map[string]interface{})
		}
		// Only stage if value is new or different (differential tracking)
		n.pendingStateChanges[key] = value
		return
	}

	// Value has changed, store it
	// Store the state - preserve existing State to maintain parameterHistory
	if n.State == nil {
		n.State = make(map[string]interface{})
	}
	n.State[key] = value

	// Outside exec cycle: Stage the change instead of creating immediate history entries
	// This allows us to batch related changes and create minimal history entries
	if n.pendingStateChanges == nil {
		n.pendingStateChanges = make(map[string]interface{})
	}
	n.pendingStateChanges[key] = value

	// Create notification
	notification := StateNotification{
		SourceWantName: n.Metadata.Name,
		StateKey:       key,
		StateValue:     value,
		PreviousValue:  previousValue,
		Timestamp:      time.Now(),
	}

	// Send notifications through the generalized system
	sendStateNotifications(notification)
}

// addAggregatedStateHistory creates a single history entry with complete state as YAML
// Uses differential checking to prevent duplicate entries when state hasn't actually changed
// Only creates a history entry if the state differs from the last recorded state
func (n *Want) addAggregatedStateHistory() {
	if n.State == nil {
		n.State = make(map[string]interface{})
	}

	// Create a single entry with the complete state as object
	stateSnapshot := n.copyCurrentState()

	// DIFFERENTIAL CHECK: Only record if state has actually changed from last history entry
	if len(n.History.StateHistory) > 0 {
		lastEntry := n.History.StateHistory[len(n.History.StateHistory)-1]

		// Convert interface{} to map[string]interface{} for comparison
		lastState, ok := lastEntry.StateValue.(map[string]interface{})
		if !ok {
			// If lastState is not the expected type, proceed with recording
			// This handles initialization or type changes
			lastState = make(map[string]interface{})
		}

		// Compare current state with last recorded state
		if n.stateSnapshotsEqual(lastState, stateSnapshot) {
			// State hasn't changed, skip recording
			return
		}
	}

	entry := StateHistoryEntry{
		WantName:   n.Metadata.Name,
		StateValue: stateSnapshot,
		Timestamp:  time.Now(),
	}

	// Append the new entry to History field
	if n.History.StateHistory == nil {
		n.History.StateHistory = make([]StateHistoryEntry, 0)
	}
	n.History.StateHistory = append(n.History.StateHistory, entry)
}

// addAggregatedParameterHistory creates a single history entry with all parameter changes as object
func (n *Want) addAggregatedParameterHistory() {
	if len(n.pendingParameterChanges) == 0 {
		return
	}

	// Create a single entry with all parameter changes as object
	entry := StateHistoryEntry{
		WantName:   n.Metadata.Name,
		StateValue: n.pendingParameterChanges,
		Timestamp:  time.Now(),
	}

	// Append the new entry to parameter history
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

// copyCurrentState creates a copy of the current state
func (n *Want) copyCurrentState() map[string]interface{} {
	n.stateMutex.Lock()
	defer n.stateMutex.Unlock()

	stateCopy := make(map[string]interface{})
	for key, value := range n.State {
		stateCopy[key] = value
	}
	return stateCopy
}

// addToStateHistory adds a state change to the want's history
func (n *Want) addToStateHistory(key string, value interface{}, previousValue interface{}) {
	if n.State == nil {
		n.State = make(map[string]interface{})
	}

	// Create new history entry (individual field tracking)
	stateMap := map[string]interface{}{
		key: value,
	}
	entry := StateHistoryEntry{
		WantName:   n.Metadata.Name,
		StateValue: stateMap,
		Timestamp:  time.Now(),
	}

	// Add to state history in History field
	if n.History.StateHistory == nil {
		n.History.StateHistory = make([]StateHistoryEntry, 0)
	}
	n.History.StateHistory = append(n.History.StateHistory, entry)

	// Limit history size (keep last 100 entries)
	maxHistorySize := 100
	if len(n.History.StateHistory) > maxHistorySize {
		n.History.StateHistory = n.History.StateHistory[len(n.History.StateHistory)-maxHistorySize:]
	}
}

// addToParameterHistory adds a parameter change to the want's parameter history (for non-exec-cycle changes)
func (n *Want) addToParameterHistory(paramName string, paramValue interface{}, previousValue interface{}) {
	// Create a single-parameter entry in aggregated format (like stateHistory)
	paramMap := map[string]interface{}{
		paramName: paramValue,
	}

	entry := StateHistoryEntry{
		WantName:   n.Metadata.Name,
		StateValue: paramMap,
		Timestamp:  time.Now(),
	}

	// Add to parameter history in History field
	n.History.ParameterHistory = append(n.History.ParameterHistory, entry)

	// Limit history size (keep last 50 entries for parameters)
	maxHistorySize := 50
	if len(n.History.ParameterHistory) > maxHistorySize {
		n.History.ParameterHistory = n.History.ParameterHistory[len(n.History.ParameterHistory)-maxHistorySize:]
	}

	fmt.Printf("[PARAM HISTORY] Want %s: %s changed from %v to %v\n",
		n.Metadata.Name, paramName, previousValue, paramValue)
}

// GetParameter gets a parameter value from the want's spec
func (n *Want) GetParameter(paramName string) (interface{}, bool) {
	if n.Spec.Params == nil {
		return nil, false
	}
	value, exists := n.Spec.Params[paramName]
	return value, exists
}

// InitializeSubscriptionSystem initializes the subscription system for this want
func (n *Want) InitializeSubscriptionSystem() {
	if n.subscriptionSystem == nil {
		n.subscriptionSystem = NewUnifiedSubscriptionSystem()
	}
}

// GetSubscriptionSystem returns the GLOBAL subscription system
// All wants share the same subscription system to enable cross-want event delivery
func (n *Want) GetSubscriptionSystem() *UnifiedSubscriptionSystem {
	return GetGlobalSubscriptionSystem()
}

// GetState retrieves a value from the want's state
func (n *Want) GetState(key string) (interface{}, bool) {
	n.stateMutex.RLock()
	defer n.stateMutex.RUnlock()
	return n.getStateUnsafe(key)
}

// migrateAgentHistoryFromState removes agent_history from state if it exists
// This ensures agent history is only stored in the top-level AgentHistory field
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

// getStateUnsafe returns state without locking (for internal use when already locked)
func (n *Want) getStateUnsafe(key string) (interface{}, bool) {
	if n.State == nil {
		return nil, false
	}
	value, exists := n.State[key]
	return value, exists
}

// GetAllState returns the entire state map
func (n *Want) GetAllState() map[string]interface{} {
	n.stateMutex.RLock()
	defer n.stateMutex.RUnlock()

	if n.State == nil {
		return make(map[string]interface{})
	}
	// Return a copy to prevent external modifications
	stateCopy := make(map[string]interface{})
	for k, v := range n.State {
		stateCopy[k] = v
	}
	return stateCopy
}

// GetStopChannel returns the stop channel for the want
func (n *Want) GetStopChannel() chan struct{} {
	if n.stopChannel == nil {
		n.stopChannel = make(chan struct{})
	}
	return n.stopChannel
}

// OnProcessEnd handles state storage when the want process ends
func (n *Want) OnProcessEnd(finalState map[string]interface{}) {
	n.SetStatus(WantStatusCompleted)

	// Store final state
	for key, value := range finalState {
		n.StoreState(key, value)
	}

	// Store completion timestamp
	n.StoreState("completion_time", fmt.Sprintf("%d", getCurrentTimestamp()))

	// Commit any pending state changes into a single batched history entry
	n.CommitStateChanges()

	// Store final statistics
	// Stats are now stored directly in State - no separate stats field

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
func (n *Want) OnProcessFail(errorState map[string]interface{}, err error) {
	n.SetStatus(WantStatusFailed)

	// Store error state
	for key, value := range errorState {
		n.StoreState(key, value)
	}

	// Store error information
	n.StoreState("error", err.Error())
	n.StoreState("failure_time", fmt.Sprintf("%d", getCurrentTimestamp()))

	// Commit any pending state changes into a single batched history entry
	n.CommitStateChanges()

	// Store statistics at failure
	// Stats are now stored directly in State - no separate stats field

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
