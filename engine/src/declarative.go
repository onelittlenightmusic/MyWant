package mywant

import (
	"context"
	"fmt"
	"mywant/engine/src/chain"
	"sync"
	"time"
)

// Re-export chain types for easier access
type Chan = chain.Chan

// NotificationType distinguishes different notification scenarios
type NotificationType string

const (
	NotificationOwnerChild   NotificationType = "owner-child"  // Current Target system (child → parent)
	NotificationSubscription NotificationType = "subscription" // New peer-to-peer (any → any)
	NotificationBroadcast    NotificationType = "broadcast"    // Global notifications (any → all)
	NotificationParameter    NotificationType = "parameter"    // Parameter changes (parent → child)
)

// StateNotification contains complete notification information
type StateNotification struct {
	SourceWantName   string           `json:"sourceWantName"`
	TargetWantName   string           `json:"targetWantName"`
	StateKey         string           `json:"stateKey"`
	StateValue       interface{}      `json:"stateValue"`
	PreviousValue    interface{}      `json:"previousValue,omitempty"`
	Timestamp        time.Time        `json:"timestamp"`
	NotificationType NotificationType `json:"notificationType"`
}

// StateUpdateListener allows wants to receive state change notifications
type StateUpdateListener interface {
	OnStateUpdate(notification StateNotification) error
}

// ParameterChangeListener allows wants to receive parameter change notifications
type ParameterChangeListener interface {
	OnParameterChange(notification StateNotification) error
}

// StateSubscription defines what state changes to monitor
type StateSubscription struct {
	WantName   string   `json:"wantName" yaml:"wantName"`                         // Which want to monitor
	StateKeys  []string `json:"stateKeys,omitempty" yaml:"stateKeys,omitempty"`   // Specific keys (empty = all keys)
	Conditions []string `json:"conditions,omitempty" yaml:"conditions,omitempty"` // Optional conditions like "value > 100"
	BufferSize int      `json:"bufferSize,omitempty" yaml:"bufferSize,omitempty"` // For rate limiting
}

// NotificationFilter allows filtering received notifications
type NotificationFilter struct {
	SourcePattern string   `json:"sourcePattern" yaml:"sourcePattern"`                   // Regex pattern for source names
	StateKeys     []string `json:"stateKeys,omitempty" yaml:"stateKeys,omitempty"`       // Only these keys
	ValuePattern  string   `json:"valuePattern,omitempty" yaml:"valuePattern,omitempty"` // Value conditions
}

// StateHistoryEntry represents a state change entry in the generic history system
type StateHistoryEntry struct {
	WantName   string      `json:"wantName" yaml:"want_name"`
	StateValue interface{} `json:"stateValue" yaml:"state_value"`
	Timestamp  time.Time   `json:"timestamp" yaml:"timestamp"`
}

// ParameterUpdate represents a parameter change notification
type ParameterUpdate struct {
	WantName   string      `json:"want_name"`
	ParamName  string      `json:"param_name"`
	ParamValue interface{} `json:"param_value"`
	Timestamp  time.Time   `json:"timestamp"`
}

// AgentExecution represents information about an agent execution
type AgentExecution struct {
	AgentName string     `json:"agent_name" yaml:"agent_name"`
	AgentType string     `json:"agent_type" yaml:"agent_type"`
	StartTime time.Time  `json:"start_time" yaml:"start_time"`
	EndTime   *time.Time `json:"end_time,omitempty" yaml:"end_time,omitempty"`
	Status    string     `json:"status" yaml:"status"` // "running", "completed", "failed"
	Error     string     `json:"error,omitempty" yaml:"error,omitempty"`
}

// ParameterUpdateListener represents a want that can receive parameter updates
type ParameterUpdateListener interface {
	OnParameterUpdate(update ParameterUpdate) bool
}

// ChainFunction represents a generalized chain function interface
type ChainFunction interface {
	Exec(using []chain.Chan, outputs []chain.Chan) bool
}

// Packet interface for all packet types in the system
type Packet interface {
	IsEnded() bool
	GetData() interface{}
	SetEnded(bool)
}

// PacketHandler defines callbacks for packet processing events
type PacketHandler interface {
	OnEnded(packet Packet) error
}

// BasePacket provides common packet functionality
type BasePacket struct {
	ended bool
	data  interface{}
}

func (p *BasePacket) IsEnded() bool        { return p.ended }
func (p *BasePacket) SetEnded(ended bool)  { p.ended = ended }
func (p *BasePacket) GetData() interface{} { return p.data }

// ChainWant represents a want that can execute directly
type ChainWant interface {
	Exec(using []chain.Chan, outputs []chain.Chan) bool
	GetWant() *Want
}

// createStartFunction converts generalized function to start function
func createStartFunction(generalizedFn func(using []chain.Chan, outputs []chain.Chan) bool) func(chain.Chan) bool {
	return func(out chain.Chan) bool {
		return generalizedFn([]chain.Chan{}, []chain.Chan{out})
	}
}

// createProcessFunction converts generalized function to process function
func createProcessFunction(generalizedFn func(using []chain.Chan, outputs []chain.Chan) bool) func(chain.Chan, chain.Chan) bool {
	return func(in chain.Chan, out chain.Chan) bool {
		return generalizedFn([]chain.Chan{in}, []chain.Chan{out})
	}
}

// createEndFunction converts generalized function to end function
func createEndFunction(generalizedFn func(using []chain.Chan, outputs []chain.Chan) bool) func(chain.Chan) bool {
	return func(in chain.Chan) bool {
		return generalizedFn([]chain.Chan{in}, []chain.Chan{})
	}
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
	n.stateMutex.Lock()
	defer n.stateMutex.Unlock()

	// Handle state changes
	if len(n.pendingStateChanges) > 0 {
		// Create a single aggregated state history entry with complete state snapshot
		if n.State == nil {
			n.State = make(map[string]interface{})
		}

		// Apply all pending changes to actual state
		for key, value := range n.pendingStateChanges {
			n.State[key] = value
		}

		// Create one history entry with the complete state snapshot
		n.addAggregatedStateHistory()
	}
	// Handle parameter changes
	if len(n.pendingParameterChanges) > 0 {
		// Create one aggregated parameter history entry
		n.addAggregatedParameterHistory()
	}
}

// GetStatus returns the current want status
func (n *Want) GetStatus() WantStatus {
	return n.Status
}

// StoreState stores a key-value pair in the want's state
func (n *Want) StoreState(key string, value interface{}) {
	n.stateMutex.Lock()
	defer n.stateMutex.Unlock()

	// If we're in an exec cycle, batch the changes
	if n.inExecCycle {
		if n.pendingStateChanges == nil {
			n.pendingStateChanges = make(map[string]interface{})
		}
		n.pendingStateChanges[key] = value
		return
	}

	// Otherwise, store immediately (legacy behavior)
	// Get previous value for notification
	previousValue, _ := n.getStateUnsafe(key)

	// Store the state - preserve existing State to maintain parameterHistory
	if n.State == nil {
		n.State = make(map[string]interface{})
	}
	n.State[key] = value

	// Add to state history
	n.addToStateHistory(key, value, previousValue)

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
// Respects the execution cycle skipping logic (skip every N cycles)
func (n *Want) addAggregatedStateHistory() {
	// Skip history recording based on execution cycle count
	// Default skip count is 100, so record only every 100th cycle
	// Can be overridden via want parameters or config
	skipCount := 100
	if n.Spec.Params != nil {
		if customSkip, exists := n.Spec.Params["state_history_skip_count"]; exists {
			if skipVal, ok := customSkip.(int); ok {
				skipCount = skipVal
			}
		}
	}
	if skipCount > 0 && n.execCycleCount%skipCount != 0 {
		return // Skip this cycle
	}

	if n.State == nil {
		n.State = make(map[string]interface{})
	}

	// Create a single entry with the complete state as object
	stateSnapshot := n.copyCurrentState()
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

// Helper function to get state keys for debugging
func getStateKeys(state map[string]interface{}) []string {
	keys := make([]string, 0, len(state))
	for k := range state {
		keys = append(keys, k)
	}
	return keys
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

// notifyParentStateUpdate is a placeholder that will be overridden by owner_types
var notifyParentStateUpdate = func(targetName, childName, key string, value interface{}) {
	// Default: do nothing (will be overridden when owner_types is imported)
}

// setNotifyParentStateUpdate allows owner_types to override the notification function
func setNotifyParentStateUpdate(fn func(string, string, string, interface{})) {
	notifyParentStateUpdate = fn
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

// migrateAllWantsAgentHistory runs agent history migration on all wants
func (cb *ChainBuilder) migrateAllWantsAgentHistory() {
	// Note: This function is called from compilePhase which is already protected by reconcileMutex
	migratedCount := 0
	for _, runtimeWant := range cb.wants {
		if runtimeWant.want != nil {
			// Check if migration is needed before running it
			if runtimeWant.want.State != nil {
				if _, exists := runtimeWant.want.State["agent_history"]; exists {
					runtimeWant.want.migrateAgentHistoryFromState()
					migratedCount++
				}
			}
		}
	}

	if migratedCount > 0 {
		fmt.Printf("[MIGRATION] Agent history migration completed for %d wants\n", migratedCount)
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

// OnProcessEnd handles state storage when the want process ends
func (n *Want) OnProcessEnd(finalState map[string]interface{}) {
	n.SetStatus(WantStatusCompleted)

	// Store final state
	for key, value := range finalState {
		n.StoreState(key, value)
	}

	// Store completion timestamp
	n.StoreState("completion_time", fmt.Sprintf("%d", getCurrentTimestamp()))

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

// Config holds the complete declarative configuration
type Config struct {
	Wants []*Want `json:"wants" yaml:"wants"`
}

// PathInfo represents connection information for a single path
type PathInfo struct {
	Channel chan interface{}
	Name    string
	Active  bool
}

// Paths manages all input and output connections for a want
type Paths struct {
	In  []PathInfo
	Out []PathInfo
}

// GetInCount returns the total number of input paths
func (p *Paths) GetInCount() int {
	return len(p.In)
}

// GetOutCount returns the total number of output paths
func (p *Paths) GetOutCount() int {
	return len(p.Out)
}

// GetActiveInCount returns the number of active input paths
func (p *Paths) GetActiveInCount() int {
	count := 0
	for _, path := range p.In {
		if path.Active {
			count++
		}
	}
	return count
}

// GetActiveOutCount returns the number of active output paths
func (p *Paths) GetActiveOutCount() int {
	count := 0
	for _, path := range p.Out {
		if path.Active {
			count++
		}
	}
	return count
}

// ConnectivityMetadata defines want connectivity requirements and constraints
type ConnectivityMetadata struct {
	RequiredInputs  int
	RequiredOutputs int
	MaxInputs       int // -1 for unlimited
	MaxOutputs      int // -1 for unlimited
	WantType        string
	Description     string
}

// EnhancedBaseWant interface for path-aware wants with connectivity validation
type EnhancedBaseWant interface {
	InitializePaths(inCount, outCount int)
	GetConnectivityMetadata() ConnectivityMetadata
	GetStats() map[string]interface{}
	Process(paths Paths) bool
	GetType() string
}

// WantStats is deprecated - use State field instead
// Keeping type alias for backward compatibility during transition
type WantStats = map[string]interface{}

// WantStatus represents the current state of a want
type WantStatus string

const (
	WantStatusIdle       WantStatus = "idle"
	WantStatusRunning    WantStatus = "running"
	WantStatusCompleted  WantStatus = "completed"
	WantStatusFailed     WantStatus = "failed"
	WantStatusTerminated WantStatus = "terminated"
)

// getCurrentTimestamp returns current Unix timestamp
func getCurrentTimestamp() int64 {
	return time.Now().Unix()
}

// WantFactory defines the interface for creating want functions
type WantFactory func(metadata Metadata, spec WantSpec) interface{}

// ChangeEventType represents the type of change detected
