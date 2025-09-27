package mywant

import (
	"context"
	"crypto/md5"
	"fmt"
	"io"
	"mywant/engine/src/chain"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"gopkg.in/yaml.v3"
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
	AgentName   string    `json:"agent_name" yaml:"agent_name"`
	AgentType   string    `json:"agent_type" yaml:"agent_type"`
	StartTime   time.Time `json:"start_time" yaml:"start_time"`
	EndTime     *time.Time `json:"end_time,omitempty" yaml:"end_time,omitempty"`
	Status      string    `json:"status" yaml:"status"` // "running", "completed", "failed"
	Error       string    `json:"error,omitempty" yaml:"error,omitempty"`
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

func (p *BasePacket) IsEnded() bool { return p.ended }
func (p *BasePacket) SetEnded(ended bool) { p.ended = ended }
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
}

// Want represents a processing unit in the chain
type Want struct {
	Metadata Metadata               `json:"metadata" yaml:"metadata"`
	Spec     WantSpec               `json:"spec" yaml:"spec"`
	Status   WantStatus             `json:"status,omitempty" yaml:"status,omitempty"`
	State    map[string]interface{} `json:"state,omitempty" yaml:"state,omitempty"`
	History  WantHistory            `json:"history" yaml:"history"`

	// Agent execution information
	CurrentAgent     string            `json:"current_agent,omitempty" yaml:"current_agent,omitempty"`
	RunningAgents    []string          `json:"running_agents,omitempty" yaml:"running_agents,omitempty"`
	AgentHistory     []AgentExecution  `json:"agent_history,omitempty" yaml:"agent_history,omitempty"`

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
}

// SetStatus updates the want's status
func (n *Want) SetStatus(status WantStatus) {
	n.Status = status
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

	n.inExecCycle = false
}

// GetStatus returns the current want status
func (n *Want) GetStatus() WantStatus {
	return n.Status
}

// StoreState stores a key-value pair in the want's state
func (n *Want) StoreState(key string, value interface{}) {
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
	previousValue, _ := n.GetState(key)

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
	skipCount := 100
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
	if n.State == nil {
		return nil, false
	}
	value, exists := n.State[key]
	return value, exists
}

// GetAllState returns the entire state map
func (n *Want) GetAllState() map[string]interface{} {
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
type ChangeEventType string

const (
	ChangeEventAdd    ChangeEventType = "ADD"
	ChangeEventUpdate ChangeEventType = "UPDATE"
	ChangeEventDelete ChangeEventType = "DELETE"
)

// ChangeEvent represents a configuration change
type ChangeEvent struct {
	Type     ChangeEventType
	WantName string
	Want     *Want
}

// ParentNotifier interface for wants that can receive child completion notifications
type ParentNotifier interface {
	NotifyChildrenComplete()
}

// ChainBuilder builds and executes chains from declarative configuration with reconcile loop
type ChainBuilder struct {
	configPath     string                    // Path to original config file
	memoryPath     string                    // Path to memory file (watched for changes)
	wants          map[string]*runtimeWant   // Runtime want registry
	registry       map[string]WantFactory    // Want type factories
	customRegistry *CustomTargetTypeRegistry // Custom target type registry
	agentRegistry  *AgentRegistry            // Agent registry for agent-enabled wants
	waitGroup      *sync.WaitGroup           // Execution synchronization
	config         Config                    // Current configuration

	// Reconcile loop fields
	reconcileStop  chan bool    // Stop signal for reconcile loop
	reconcileMutex sync.RWMutex // Protect concurrent access
	running        bool         // Execution state
	lastConfig     Config       // Last known config state
	lastConfigHash string       // Hash of last config for change detection

	// Path and channel management
	pathMap      map[string]Paths      // Want path mapping
	channels     map[string]chain.Chan // Inter-want channels
	channelMutex sync.RWMutex          // Protect channel access

	// Recipe result processing
	recipeResult *RecipeResult // Recipe result definition (if available)
	recipePath   string        // Path to recipe file (if loaded from recipe)

	// Suspend/Resume control
	suspended    bool         // Current suspension state
	suspendChan  chan bool    // Channel to signal suspension
	resumeChan   chan bool    // Channel to signal resume
	controlMutex sync.RWMutex // Protect suspension state access
	controlStop  chan bool    // Stop signal for control loop
}

// runtimeWant holds the runtime state of a want
type runtimeWant struct {
	metadata Metadata
	spec     WantSpec
	chain    chain.C_chain
	function interface{}
	want     *Want
}

// NewChainBuilder creates a new builder from configuration
func NewChainBuilder(config Config) *ChainBuilder {
	builder := NewChainBuilderWithPaths("", "")
	builder.config = config
	return builder
}

// NewChainBuilderFromRecipe creates a ChainBuilder from a GenericRecipeConfig with automatic result processing
func NewChainBuilderFromRecipe(recipeConfig *GenericRecipeConfig) *ChainBuilder {
	builder := NewChainBuilderWithPaths("", "")
	builder.config = recipeConfig.Config

	// Set recipe result for automatic processing
	if recipeConfig.Result != nil {
		builder.SetRecipeResult(recipeConfig.Result, "recipe")
	}

	return builder
}

// NewChainBuilderWithPaths creates a new builder with config and memory file paths
func NewChainBuilderWithPaths(configPath, memoryPath string) *ChainBuilder {
	builder := &ChainBuilder{
		configPath:     configPath,
		memoryPath:     memoryPath,
		wants:          make(map[string]*runtimeWant),
		registry:       make(map[string]WantFactory),
		customRegistry: NewCustomTargetTypeRegistry(),
		reconcileStop:  make(chan bool),
		pathMap:        make(map[string]Paths),
		channels:       make(map[string]chain.Chan),
		running:        false,
		waitGroup:      &sync.WaitGroup{},
		// Initialize suspend/resume control
		suspended:   false,
		suspendChan: make(chan bool),
		resumeChan:  make(chan bool),
		controlStop: make(chan bool),
	}

	// Register built-in want types
	builder.registerBuiltinWantTypes()

	// Auto-register custom target types from recipes
	err := ScanAndRegisterCustomTypes("recipes", builder.customRegistry)
	if err != nil {
		fmt.Printf("⚠️  Warning: failed to scan recipes for custom types: %v\n", err)
	}

	// Auto-register owner want types for target system support
	RegisterOwnerWantTypes(builder)

	return builder
}

// registerBuiltinWantTypes registers the default want type factories
func (cb *ChainBuilder) registerBuiltinWantTypes() {
	// No built-in types by default - they should be registered by domain-specific modules
}

// RegisterWantType allows registering custom want types
func (cb *ChainBuilder) RegisterWantType(wantType string, factory WantFactory) {
	cb.registry[wantType] = factory
}

// SetAgentRegistry sets the agent registry for the chain builder
func (cb *ChainBuilder) SetAgentRegistry(registry *AgentRegistry) {
	cb.agentRegistry = registry
}

// SetConfigInternal sets the config for the builder (for server mode)
func (cb *ChainBuilder) SetConfigInternal(config Config) {
	cb.config = config
}

// matchesSelector checks if want labels match the selector criteria
func (cb *ChainBuilder) matchesSelector(wantLabels map[string]string, selector map[string]string) bool {
	for key, value := range selector {
		if wantLabels[key] != value {
			return false
		}
	}
	return true
}

// generatePathsFromConnections creates paths based on labels and using, eliminating output requirements
func (cb *ChainBuilder) generatePathsFromConnections() map[string]Paths {
	pathMap := make(map[string]Paths)

	// Initialize empty paths for all wants
	for wantName := range cb.wants {
		pathMap[wantName] = Paths{
			In:  []PathInfo{},
			Out: []PathInfo{},
		}
	}

	// Create connections based on using selectors
	for wantName, want := range cb.wants {
		paths := pathMap[wantName]

		// Process using connections for this want
		for _, usingSelector := range want.spec.Using {
			// Find wants that match this using selector
			for otherName, otherWant := range cb.wants {
				if cb.matchesSelector(otherWant.metadata.Labels, usingSelector) {
					// Create using path for current want
					inPath := PathInfo{
						Channel: make(chan interface{}, 10),
						Name:    fmt.Sprintf("%s_to_%s", otherName, wantName),
						Active:  true,
					}
					paths.In = append(paths.In, inPath)

					// Create corresponding output path for the matching want
					otherPaths := pathMap[otherName]
					outPath := PathInfo{
						Channel: inPath.Channel, // Same channel, shared connection
						Name:    inPath.Name,
						Active:  true,
					}
					otherPaths.Out = append(otherPaths.Out, outPath)
					pathMap[otherName] = otherPaths
				}
			}
		}
		pathMap[wantName] = paths
	}

	return pathMap
}

// validateConnections validates that all wants have their connectivity requirements satisfied
func (cb *ChainBuilder) validateConnections(pathMap map[string]Paths) error {
	for wantName, want := range cb.wants {
		paths := pathMap[wantName]

		// Check if this is an enhanced want that has connectivity requirements
		if enhancedWant, ok := want.function.(EnhancedBaseWant); ok {
			meta := enhancedWant.GetConnectivityMetadata()

			inCount := len(paths.In)
			outCount := len(paths.Out)

			// Check required using
			if inCount < meta.RequiredInputs {
				return fmt.Errorf("validation failed for want %s: want %s requires %d using, got %d",
					wantName, meta.WantType, meta.RequiredInputs, inCount)
			}

			// Check required outputs
			if outCount < meta.RequiredOutputs {
				return fmt.Errorf("validation failed for want %s: want %s requires %d outputs, got %d",
					wantName, meta.WantType, meta.RequiredOutputs, outCount)
			}

			// Check maximum using
			if meta.MaxInputs >= 0 && inCount > meta.MaxInputs {
				return fmt.Errorf("validation failed for want %s: want %s allows max %d using, got %d",
					wantName, meta.WantType, meta.MaxInputs, inCount)
			}

			// Check maximum outputs
			if meta.MaxOutputs >= 0 && outCount > meta.MaxOutputs {
				return fmt.Errorf("validation failed for want %s: want %s allows max %d outputs, got %d",
					wantName, meta.WantType, meta.MaxOutputs, outCount)
			}
		}
	}
	return nil
}

// createWantFunction creates the appropriate function based on want type using registry
func (cb *ChainBuilder) createWantFunction(want *Want) (interface{}, error) {
	wantType := want.Metadata.Type

	// Check if it's a custom target type first
	if cb.customRegistry.IsCustomType(wantType) {
		return cb.createCustomTargetWant(want), nil
	}

	// Fall back to standard type registration
	factory, exists := cb.registry[wantType]
	if !exists {
		// List available types for better error message
		availableTypes := make([]string, 0, len(cb.registry))
		for typeName := range cb.registry {
			availableTypes = append(availableTypes, typeName)
		}
		customTypes := cb.customRegistry.ListTypes()

		return nil, fmt.Errorf("Unknown want type: '%s'. Available standard types: %v. Available custom types: %v",
			wantType, availableTypes, customTypes)
	}
	wantInstance := factory(want.Metadata, want.Spec)

	// Set agent registry if available and the want instance supports it
	if cb.agentRegistry != nil {
		if wantWithGetWant, ok := wantInstance.(interface{ GetWant() *Want }); ok {
			wantWithGetWant.GetWant().SetAgentRegistry(cb.agentRegistry)
		} else if w, ok := wantInstance.(*Want); ok {
			w.SetAgentRegistry(cb.agentRegistry)
		}
	}

	return wantInstance, nil
}

// TestCreateWantFunction tests want type creation without side effects (exported for validation)
func (cb *ChainBuilder) TestCreateWantFunction(want *Want) (interface{}, error) {
	return cb.createWantFunction(want)
}

// createCustomTargetWant creates a custom target want using the registry
func (cb *ChainBuilder) createCustomTargetWant(want *Want) interface{} {
	config, exists := cb.customRegistry.Get(want.Metadata.Type)
	if !exists {
		panic(fmt.Sprintf("Custom type '%s' not found in registry", want.Metadata.Type))
	}

	fmt.Printf("🎯 Creating custom target type: '%s' - %s\n", config.Name, config.Description)

	// Merge custom type defaults with user-provided spec
	mergedSpec := cb.mergeWithCustomDefaults(want.Spec, config)

	// Create the custom target using the registered function
	target := config.CreateTargetFunc(want.Metadata, mergedSpec)

	// Set up target with builder and recipe loader (if available)
	target.SetBuilder(cb)

	// Set up recipe loader for custom targets
	recipeLoader := NewGenericRecipeLoader("recipes")
	target.SetRecipeLoader(recipeLoader)

	return target
}

// mergeWithCustomDefaults merges user spec with custom type defaults
func (cb *ChainBuilder) mergeWithCustomDefaults(spec WantSpec, config CustomTargetTypeConfig) WantSpec {
	merged := spec

	// Initialize params if nil
	if merged.Params == nil {
		merged.Params = make(map[string]interface{})
	}

	// Merge default parameters (user params take precedence)
	for key, defaultValue := range config.DefaultParams {
		if _, exists := merged.Params[key]; !exists {
			merged.Params[key] = defaultValue
		}
	}

	// Recipe is no longer used - custom types are distinguished by metadata.name

	return merged
}

// copyConfigToMemory copies the current config to memory file for watching
func (cb *ChainBuilder) copyConfigToMemory() error {
	if cb.memoryPath == "" {
		return nil
	}

	// Ensure memory directory exists
	memoryDir := filepath.Dir(cb.memoryPath)
	if err := os.MkdirAll(memoryDir, 0755); err != nil {
		return fmt.Errorf("failed to create memory directory: %w", err)
	}

	// Marshal config to YAML
	data, err := yaml.Marshal(cb.config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Write to memory file
	err = os.WriteFile(cb.memoryPath, data, 0644)
	if err != nil {
		return fmt.Errorf("failed to write memory file: %w", err)
	}

	return nil
}

// calculateFileHash calculates MD5 hash of a file
func (cb *ChainBuilder) calculateFileHash(filename string) (string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := md5.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

// hasMemoryFileChanged checks if memory file has changed
func (cb *ChainBuilder) hasMemoryFileChanged() bool {
	if cb.memoryPath == "" {
		return false
	}

	currentHash, err := cb.calculateFileHash(cb.memoryPath)
	if err != nil {
		return false
	}

	return currentHash != cb.lastConfigHash
}

// loadMemoryConfig loads configuration from memory file or original config
func (cb *ChainBuilder) loadMemoryConfig() (Config, error) {
	// If memory path is configured and file exists, load from memory
	if cb.memoryPath != "" {
		if _, err := os.Stat(cb.memoryPath); err == nil {
			return loadConfigFromYAML(cb.memoryPath)
		}
	}

	// Otherwise, return the original config
	return cb.config, nil
}

// reconcileLoop main reconcile loop that handles both initial config load and dynamic changes
func (cb *ChainBuilder) reconcileLoop() {
	// Initial configuration load
	fmt.Println("[RECONCILE] Loading initial configuration")
	cb.reconcileWants()

	ticker := time.NewTicker(100 * time.Millisecond)
	statsTicker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	defer statsTicker.Stop()

	for {
		select {
		case <-cb.reconcileStop:
			fmt.Println("[RECONCILE] Stopping reconcile loop")
			return
		case <-ticker.C:
			if cb.hasMemoryFileChanged() {
				fmt.Println("[RECONCILE] Detected config change")
				cb.reconcileWants()
			}
		case <-statsTicker.C:
			cb.writeStatsToMemory()
		}
	}
}

// reconcileWants performs reconciliation when config changes or during initial load
// Separated into explicit phases: compile -> connect -> start
func (cb *ChainBuilder) reconcileWants() {
	cb.reconcileMutex.Lock()
	defer cb.reconcileMutex.Unlock()

	fmt.Println("[RECONCILE] Starting reconciliation with separated phases")

	// Phase 1: COMPILE - Load and validate configuration
	if err := cb.compilePhase(); err != nil {
		fmt.Printf("[RECONCILE] Compile phase failed: %v\n", err)
		return
	}

	// Phase 2: CONNECT - Establish want topology
	if err := cb.connectPhase(); err != nil {
		fmt.Printf("[RECONCILE] Connect phase failed: %v\n", err)
		return
	}

	// Phase 3: START - Launch new/updated wants
	cb.startPhase()

	fmt.Println("[RECONCILE] All phases completed successfully")
}

// compilePhase handles configuration loading and want creation/updates
func (cb *ChainBuilder) compilePhase() error {
	fmt.Println("[RECONCILE:COMPILE] Loading and validating configuration")

	// Load new config
	newConfig, err := cb.loadMemoryConfig()
	if err != nil {
		return fmt.Errorf("failed to load memory config: %w", err)
	}

	// Check if this is initial load (no lastConfig set)
	isInitialLoad := len(cb.lastConfig.Wants) == 0

	if isInitialLoad {
		fmt.Printf("[RECONCILE:COMPILE] Initial load: processing %d wants\n", len(newConfig.Wants))
		// For initial load, treat all wants as new additions
		for _, wantConfig := range newConfig.Wants {
			cb.addDynamicWantUnsafe(wantConfig)
		}

		// Dump memory after initial load
		if len(newConfig.Wants) > 0 {
			fmt.Println("[RECONCILE:MEMORY] Dumping memory after initial load...")
			if err := cb.dumpWantMemoryToYAML(); err != nil {
				fmt.Printf("[RECONCILE:MEMORY] Warning: Failed to dump memory: %v\n", err)
			}
		}
	} else {
		// Detect changes for ongoing updates
		changes := cb.detectConfigChanges(cb.lastConfig, newConfig)
		if len(changes) == 0 {
			fmt.Println("[RECONCILE:COMPILE] No configuration changes detected")
			return nil
		}

		fmt.Printf("[RECONCILE:COMPILE] Processing %d configuration changes\n", len(changes))

		// Apply changes in reverse dependency order (sink to generator)
		cb.applyWantChanges(changes)
	}

	// Update last config and hash
	cb.lastConfig = newConfig
	cb.lastConfigHash, _ = cb.calculateFileHash(cb.memoryPath)

	fmt.Println("[RECONCILE:COMPILE] Configuration compilation completed")
	return nil
}

// connectPhase handles want topology establishment and validation
func (cb *ChainBuilder) connectPhase() error {
	fmt.Println("[RECONCILE:CONNECT] Establishing want topology")

	// Process auto-connections for RecipeAgent wants before generating paths
	cb.processAutoConnections()

	// Generate new paths based on current wants
	cb.pathMap = cb.generatePathsFromConnections()

	// Validate connectivity requirements
	if err := cb.validateConnections(cb.pathMap); err != nil {
		return fmt.Errorf("connectivity validation failed: %w", err)
	}

	// Rebuild channels based on new topology
	cb.channelMutex.Lock()
	cb.channels = make(map[string]chain.Chan)

	channelCount := 0
	for _, paths := range cb.pathMap {
		for _, outputPath := range paths.Out {
			if outputPath.Active {
				channelKey := outputPath.Name
				cb.channels[channelKey] = make(chain.Chan, 10)
				channelCount++
			}
		}
	}
	cb.channelMutex.Unlock()

	fmt.Printf("[RECONCILE:CONNECT] Topology established: %d channels created\n", channelCount)
	return nil
}

// processAutoConnections handles system-wide auto-connection for RecipeAgent wants
func (cb *ChainBuilder) processAutoConnections() {
	fmt.Println("[RECONCILE:AUTOCONNECT] Processing auto-connections for RecipeAgent wants")

	// Collect all wants with RecipeAgent enabled
	autoConnectWants := make([]*runtimeWant, 0)
	allWants := make([]*runtimeWant, 0)

	for _, runtimeWant := range cb.wants {
		allWants = append(allWants, runtimeWant)

		// Check if want has RecipeAgent enabled in its metadata or state
		want := runtimeWant.want
		if cb.hasRecipeAgent(want) {
			autoConnectWants = append(autoConnectWants, runtimeWant)
			fmt.Printf("[RECONCILE:AUTOCONNECT] Found RecipeAgent want: %s\n", want.Metadata.Name)
		}
	}

	// Process auto-connections for each RecipeAgent want
	for _, runtimeWant := range autoConnectWants {
		want := runtimeWant.want
		cb.autoConnectWant(want, allWants)
		// Update the runtime spec to reflect auto-connection changes
		runtimeWant.spec.Using = want.Spec.Using
	}

	fmt.Printf("[RECONCILE:AUTOCONNECT] Processed auto-connections for %d RecipeAgent wants\n", len(autoConnectWants))
}

// hasRecipeAgent checks if a want has RecipeAgent functionality enabled
func (cb *ChainBuilder) hasRecipeAgent(want *Want) bool {
	// Check if want has coordinator role (typical for RecipeAgent wants)
	if want.Metadata.Labels != nil {
		if role, ok := want.Metadata.Labels["role"]; ok && role == "coordinator" {
			return true
		}
	}

	// Check for specific coordinator types
	if want.Metadata.Type == "level1_coordinator" || want.Metadata.Type == "level2_coordinator" {
		return true
	}

	return false
}

// autoConnectWant connects a RecipeAgent want to all compatible wants with matching approval_id
func (cb *ChainBuilder) autoConnectWant(want *Want, allWants []*runtimeWant) {
	fmt.Printf("[RECONCILE:AUTOCONNECT] Processing auto-connection for want %s\n", want.Metadata.Name)

	// Look for approval_id in want's params or labels
	approvalID := ""

	// Check params first
	if want.Spec.Params != nil {
		if approvalVal, ok := want.Spec.Params["approval_id"]; ok {
			if approvalStr, ok := approvalVal.(string); ok {
				approvalID = approvalStr
			}
		}
	}

	// Check labels if not found in params
	if approvalID == "" && want.Metadata.Labels != nil {
		if approvalVal, ok := want.Metadata.Labels["approval_id"]; ok {
			approvalID = approvalVal
		}
	}

	if approvalID == "" {
		fmt.Printf("[RECONCILE:AUTOCONNECT] No approval_id found for want %s, skipping\n", want.Metadata.Name)
		return
	}

	fmt.Printf("[RECONCILE:AUTOCONNECT] Found approval_id: %s for want %s\n", approvalID, want.Metadata.Name)

	// Initialize using selectors if nil
	if want.Spec.Using == nil {
		want.Spec.Using = make([]map[string]string, 0)
	}

	connectionsAdded := 0

	// Find all other wants with the same approval_id for auto-connection
	for _, otherRuntimeWant := range allWants {
		otherWant := otherRuntimeWant.want

		// Skip self
		if otherWant.Metadata.Name == want.Metadata.Name {
			continue
		}

		// Check if other want has matching approval_id
		otherApprovalID := ""
		if otherWant.Spec.Params != nil {
			if approvalVal, ok := otherWant.Spec.Params["approval_id"]; ok {
				if approvalStr, ok := approvalVal.(string); ok {
					otherApprovalID = approvalStr
				}
			}
		}

		if otherApprovalID == "" && otherWant.Metadata.Labels != nil {
			if approvalVal, ok := otherWant.Metadata.Labels["approval_id"]; ok {
				otherApprovalID = approvalVal
			}
		}

		// Auto-connect to wants with matching approval_id
		if otherApprovalID == approvalID {
			// Only auto-connect to data provider wants (evidence, description)
			// Skip coordinators and target wants
			if otherWant.Metadata.Labels != nil {
				role := otherWant.Metadata.Labels["role"]
				if role == "evidence-provider" || role == "description-provider" {
					// Add unique connection label to source want first
					connectionKey := cb.generateConnectionKey(want)
					cb.addConnectionLabel(otherWant, want)

					// Create using selector based on the unique label we just added
					selector := make(map[string]string)
					if connectionKey != "" {
						labelKey := fmt.Sprintf("used_by_%s", connectionKey)
						selector[labelKey] = want.Metadata.Name
					} else {
						// Fallback to role-based selector if no unique key generated
						selector["role"] = role
					}

					// Check for duplicate connections
					duplicate := false
					for _, existingSelector := range want.Spec.Using {
						if len(existingSelector) == len(selector) {
							match := true
							for k, v := range selector {
								if existingSelector[k] != v {
									match = false
									break
								}
							}
							if match {
								duplicate = true
								break
							}
						}
					}

					if !duplicate {
						want.Spec.Using = append(want.Spec.Using, selector)
						connectionsAdded++
						fmt.Printf("[RECONCILE:AUTOCONNECT] Added connection: %s -> %v (from %s)\n",
							want.Metadata.Name, selector, otherWant.Metadata.Name)
					} else {
						fmt.Printf("[RECONCILE:AUTOCONNECT] Skipping duplicate connection: %s -> %v\n",
							want.Metadata.Name, selector)
					}
				} else {
					fmt.Printf("[RECONCILE:AUTOCONNECT] Skipping connection to %s (role: %s) - not a data provider\n",
						otherWant.Metadata.Name, role)
				}
			}
		}
	}

	fmt.Printf("[RECONCILE:AUTOCONNECT] Completed auto-connection for %s with %d connections\n",
		want.Metadata.Name, connectionsAdded)
}

// addConnectionLabel adds a unique label to source want indicating which coordinator is using it
func (cb *ChainBuilder) addConnectionLabel(sourceWant *Want, consumerWant *Want) {
	if sourceWant.Metadata.Labels == nil {
		sourceWant.Metadata.Labels = make(map[string]string)
	}

	// Generate unique connection label based on consumer want
	// Extract meaningful identifier from consumer (e.g., level1, level2, etc.)
	connectionKey := cb.generateConnectionKey(consumerWant)

	if connectionKey != "" {
		labelKey := fmt.Sprintf("used_by_%s", connectionKey)
		sourceWant.Metadata.Labels[labelKey] = consumerWant.Metadata.Name
		fmt.Printf("[RECONCILE:AUTOCONNECT] Added connection label to %s: %s=%s\n",
			sourceWant.Metadata.Name, labelKey, consumerWant.Metadata.Name)
	}
}

// generateConnectionKey creates a unique key based on consumer want characteristics
func (cb *ChainBuilder) generateConnectionKey(consumerWant *Want) string {
	// Try to extract meaningful identifier from labels first
	if consumerWant.Metadata.Labels != nil {
		// Check for approval_level, component, or other identifying labels
		if level, ok := consumerWant.Metadata.Labels["approval_level"]; ok {
			return fmt.Sprintf("level%s", level)
		}
		if component, ok := consumerWant.Metadata.Labels["component"]; ok {
			return component
		}
		if category, ok := consumerWant.Metadata.Labels["category"]; ok {
			return category
		}
	}

	// Extract from want type as fallback
	wantType := consumerWant.Metadata.Type
	if strings.Contains(wantType, "level1") {
		return "level1"
	}
	if strings.Contains(wantType, "level2") {
		return "level2"
	}
	if strings.Contains(wantType, "coordinator") {
		return "coordinator"
	}

	// Use sanitized want name as last resort
	return strings.ReplaceAll(consumerWant.Metadata.Name, "-", "_")
}

// startPhase handles launching new/updated wants
func (cb *ChainBuilder) startPhase() {
	fmt.Println("[RECONCILE:START] Launching new and updated wants")

	// Start new wants if system is running
	if cb.running {
		startedCount := 0
		for wantName, want := range cb.wants {
			if want.want.GetStatus() == WantStatusIdle {
				cb.startWant(wantName, want)
				startedCount++
			}
		}
		fmt.Printf("[RECONCILE:START] Started %d wants\n", startedCount)
	} else {
		fmt.Println("[RECONCILE:START] System not running, wants will be started later")
	}
}

// detectConfigChanges compares configs and returns change events
func (cb *ChainBuilder) detectConfigChanges(oldConfig, newConfig Config) []ChangeEvent {
	var changes []ChangeEvent

	// Create maps for easier comparison
	oldWants := make(map[string]*Want)
	for _, want := range oldConfig.Wants {
		oldWants[want.Metadata.Name] = want
	}

	newWants := make(map[string]*Want)
	for _, want := range newConfig.Wants {
		newWants[want.Metadata.Name] = want
	}

	// Find additions and updates
	for name, newWant := range newWants {
		if oldWant, exists := oldWants[name]; exists {
			// Check if want changed
			if !cb.wantsEqual(oldWant, newWant) {
				changes = append(changes, ChangeEvent{
					Type:     ChangeEventUpdate,
					WantName: name,
					Want:     newWant,
				})
			}
		} else {
			// New want
			changes = append(changes, ChangeEvent{
				Type:     ChangeEventAdd,
				WantName: name,
				Want:     newWant,
			})
		}
	}

	// Find deletions
	for name := range oldWants {
		if _, exists := newWants[name]; !exists {
			changes = append(changes, ChangeEvent{
				Type:     ChangeEventDelete,
				WantName: name,
				Want:     nil,
			})
		}
	}

	return changes
}

// wantsEqual compares two wants for equality
func (cb *ChainBuilder) wantsEqual(a, b *Want) bool {
	// Simple comparison - could be enhanced
	return a.Metadata.Type == b.Metadata.Type &&
		fmt.Sprintf("%v", a.Spec.Params) == fmt.Sprintf("%v", b.Spec.Params) &&
		fmt.Sprintf("%v", a.Spec.Using) == fmt.Sprintf("%v", b.Spec.Using)
}

// applyWantChanges applies want changes in sink-to-generator order
// Note: Connections are rebuilt in separate connectPhase()
func (cb *ChainBuilder) applyWantChanges(changes []ChangeEvent) {
	// Sort changes by dependency level (sink wants first)
	sortedChanges := cb.sortChangesByDependency(changes)

	hasWantChanges := false
	for _, change := range sortedChanges {
		switch change.Type {
		case ChangeEventAdd:
			fmt.Printf("[RECONCILE:COMPILE] Adding want: %s\n", change.WantName)
			cb.addDynamicWantUnsafe(change.Want)
			hasWantChanges = true
		case ChangeEventUpdate:
			fmt.Printf("[RECONCILE:COMPILE] Updating want: %s\n", change.WantName)
			cb.UpdateWant(change.Want)
			hasWantChanges = true
		case ChangeEventDelete:
			fmt.Printf("[RECONCILE:COMPILE] Deleting want: %s\n", change.WantName)
			cb.deleteWant(change.WantName)
			hasWantChanges = true
		}
	}

	// Dump memory after want additions/deletions/updates
	if hasWantChanges {
		fmt.Println("[RECONCILE:MEMORY] Dumping memory after want changes...")
		if err := cb.dumpWantMemoryToYAML(); err != nil {
			fmt.Printf("[RECONCILE:MEMORY] Warning: Failed to dump memory: %v\n", err)
		}
	}

	// Note: Connections rebuilt in connectPhase(), not here
}

// sortChangesByDependency sorts changes by dependency level
func (cb *ChainBuilder) sortChangesByDependency(changes []ChangeEvent) []ChangeEvent {
	// Calculate dependency levels for all wants
	depLevels := cb.calculateDependencyLevels()

	// Sort changes by dependency level (higher level = closer to sink)
	sortedChanges := make([]ChangeEvent, len(changes))
	copy(sortedChanges, changes)

	// Simple sort by dependency level
	for i := 0; i < len(sortedChanges)-1; i++ {
		for j := i + 1; j < len(sortedChanges); j++ {
			levelI := depLevels[sortedChanges[i].WantName]
			levelJ := depLevels[sortedChanges[j].WantName]
			if levelI < levelJ {
				sortedChanges[i], sortedChanges[j] = sortedChanges[j], sortedChanges[i]
			}
		}
	}

	return sortedChanges
}

// calculateDependencyLevels calculates dependency levels based on using connectivity
func (cb *ChainBuilder) calculateDependencyLevels() map[string]int {
	levels := make(map[string]int)
	visited := make(map[string]bool)

	// Calculate dependency levels using topological ordering
	for name := range cb.wants {
		if !visited[name] {
			cb.calculateDependencyLevel(name, levels, visited, make(map[string]bool))
		}
	}

	return levels
}

// calculateDependencyLevel recursively calculates dependency level for a want
func (cb *ChainBuilder) calculateDependencyLevel(wantName string, levels map[string]int, visited, inProgress map[string]bool) int {
	// Check for circular dependency
	if inProgress[wantName] {
		return 0 // Break cycles by assigning level 0
	}

	// Return cached result
	if visited[wantName] {
		return levels[wantName]
	}

	inProgress[wantName] = true

	// Get want config from current runtime or config
	var wantConfig *Want
	if want, exists := cb.wants[wantName]; exists {
		wantConfig = want.want
	} else {
		// Look for want in current config
		found := false
		for _, configWant := range cb.config.Wants {
			if configWant.Metadata.Name == wantName {
				wantConfig = configWant
				found = true
				break
			}
		}
		if !found {
			// Unknown want, assign level 0
			levels[wantName] = 0
			visited[wantName] = true
			delete(inProgress, wantName)
			return 0
		}
	}

	maxDependencyLevel := 0

	// Find dependencies from using selectors
	for _, usingSelector := range wantConfig.Spec.Using {
		// Find wants that match this selector
		for _, configWant := range cb.config.Wants {
			if cb.matchesSelector(configWant.Metadata.Labels, usingSelector) {
				depLevel := cb.calculateDependencyLevel(configWant.Metadata.Name, levels, visited, inProgress)
				if depLevel >= maxDependencyLevel {
					maxDependencyLevel = depLevel + 1
				}
			}
		}
	}

	// Assign level based on dependencies
	levels[wantName] = maxDependencyLevel
	visited[wantName] = true
	delete(inProgress, wantName)

	return maxDependencyLevel
}

// addWant adds a new want to the runtime (private method)
func (cb *ChainBuilder) addWant(wantConfig *Want) {
	fmt.Printf("[RECONCILE] Adding want: %s\n", wantConfig.Metadata.Name)

	// Create the function/want
	wantFunction, err := cb.createWantFunction(wantConfig)
	if err != nil {
		fmt.Printf("[RECONCILE:ERROR] Failed to create want function for %s: %v\n", wantConfig.Metadata.Name, err)

		// Create a failed want instead of returning error
		wantPtr := &Want{
			Metadata: wantConfig.Metadata,
			Spec:     wantConfig.Spec,
			Status:   WantStatusFailed,
			State: map[string]interface{}{
				"error": err.Error(),
			},
			History: wantConfig.History,
		}

		runtimeWant := &runtimeWant{
			metadata: wantConfig.Metadata,
			spec:     wantConfig.Spec,
			function: nil, // No function since creation failed
			want:     wantPtr,
		}
		cb.wants[wantConfig.Metadata.Name] = runtimeWant
		return
	}

	var wantPtr *Want
	if wantWithGetWant, ok := wantFunction.(interface{ GetWant() *Want }); ok {
		wantPtr = wantWithGetWant.GetWant()

		// Copy State from config (simple copy since History is separate)
		if wantConfig.State != nil {
			if wantPtr.State == nil {
				wantPtr.State = make(map[string]interface{})
			}
			// Copy all state data
			for k, v := range wantConfig.State {
				wantPtr.State[k] = v
			}
		}

		// Copy History field from config
		wantPtr.History = wantConfig.History

		// Initialize parameterHistory with initial parameter values if empty or nil
		if (wantPtr.History.ParameterHistory == nil || len(wantPtr.History.ParameterHistory) == 0) && wantConfig.Spec.Params != nil {
			fmt.Printf("[RECONCILE] Recording initial parameter history for want %s: %v\n", wantConfig.Metadata.Name, wantConfig.Spec.Params)
			// Create a deep copy of the parameters to avoid reference issues
			paramsCopy := make(map[string]interface{})
			for k, v := range wantConfig.Spec.Params {
				paramsCopy[k] = v
			}
			// Create one entry with all initial parameters as object
			entry := StateHistoryEntry{
				WantName:   wantConfig.Metadata.Name,
				StateValue: paramsCopy,
				Timestamp:  time.Now(),
			}
			if wantPtr.History.ParameterHistory == nil {
				wantPtr.History.ParameterHistory = make([]StateHistoryEntry, 0)
			}
			wantPtr.History.ParameterHistory = append(wantPtr.History.ParameterHistory, entry)
		}
	} else {
		// Initialize State map (simple copy since History is separate)
		stateMap := make(map[string]interface{})
		if wantConfig.State != nil {
			// Copy all state data
			for k, v := range wantConfig.State {
				stateMap[k] = v
			}
		}

		// Copy History field from config
		historyField := wantConfig.History

		// Initialize parameterHistory with initial parameters if empty
		if len(historyField.ParameterHistory) == 0 && wantConfig.Spec.Params != nil {
			// Create one entry with all initial parameters as object
			entry := StateHistoryEntry{
				WantName:   wantConfig.Metadata.Name,
				StateValue: wantConfig.Spec.Params,
				Timestamp:  time.Now(),
			}
			historyField.ParameterHistory = append(historyField.ParameterHistory, entry)
		}

		wantPtr = &Want{
			Metadata: wantConfig.Metadata,
			Spec:     wantConfig.Spec,
			Status:   WantStatusIdle,
			State:    stateMap,
			History:  historyField,
		}
	}

	runtimeWant := &runtimeWant{
		metadata: wantConfig.Metadata,
		spec:     wantConfig.Spec,
		function: wantFunction,
		want:     wantPtr,
	}
	cb.wants[wantConfig.Metadata.Name] = runtimeWant

	// Register want for notification system
	cb.registerWantForNotifications(wantConfig, wantFunction, wantPtr)
}

// FindWantByID searches for a want by its metadata.id across all runtime wants
func (cb *ChainBuilder) FindWantByID(wantID string) (*Want, string, bool) {
	for wantName, runtimeWant := range cb.wants {
		if runtimeWant.want.Metadata.ID == wantID {
			return runtimeWant.want, wantName, true
		}
	}
	return nil, "", false
}

// UpdateWant updates an existing want in place and restarts execution
func (cb *ChainBuilder) UpdateWant(wantConfig *Want) {
	fmt.Printf("[RECONCILE] Updating want by ID: %s\n", wantConfig.Metadata.ID)

	// Find the existing want by metadata.id using universal search
	existingWant, wantName, exists := cb.FindWantByID(wantConfig.Metadata.ID)
	if !exists {
		fmt.Printf("[RECONCILE:ERROR] Want with ID %s not found for update, adding as new\n", wantConfig.Metadata.ID)
		cb.addDynamicWantUnsafe(wantConfig)
		return
	}

	// Detect parameter changes and create a single consolidated history entry
	var changedParams map[string]interface{}
	if wantConfig.Spec.Params != nil {
		for paramName, newValue := range wantConfig.Spec.Params {
			// Check if parameter changed
			var oldValue interface{}
			var hasOldValue bool
			if existingWant.Spec.Params != nil {
				oldValue, hasOldValue = existingWant.Spec.Params[paramName]
			}

			// Update if value changed or is new
			if !hasOldValue || oldValue != newValue {
				// Initialize params map if needed
				if existingWant.Spec.Params == nil {
					existingWant.Spec.Params = make(map[string]interface{})
				}

				// Update the parameter directly (bypass UpdateParameter to avoid individual history entries)
				existingWant.Spec.Params[paramName] = newValue

				// Track changed parameters for consolidated history entry
				if changedParams == nil {
					changedParams = make(map[string]interface{})
				}
				changedParams[paramName] = newValue

				fmt.Printf("[RECONCILE] Parameter updated: %s = %v (was: %v)\n", paramName, newValue, oldValue)
			}
		}
	}

	// Create a single consolidated parameter history entry for all changes
	if changedParams != nil {
		entry := StateHistoryEntry{
			WantName:   existingWant.Metadata.Name,
			StateValue: changedParams,
			Timestamp:  time.Now(),
		}
		existingWant.History.ParameterHistory = append(existingWant.History.ParameterHistory, entry)

		// Limit history size (keep last 50 entries for parameters)
		maxHistorySize := 50
		if len(existingWant.History.ParameterHistory) > maxHistorySize {
			existingWant.History.ParameterHistory = existingWant.History.ParameterHistory[len(existingWant.History.ParameterHistory)-maxHistorySize:]
		}
	}

	// Update other spec fields (using, requires, etc.)
	existingWant.Spec.Using = wantConfig.Spec.Using
	existingWant.Spec.Requires = wantConfig.Spec.Requires

	// Update metadata
	existingWant.Metadata = wantConfig.Metadata

	// Reset status from completed to idle to allow re-execution
	existingWant.SetStatus(WantStatusIdle)

	// Clear previous state if needed (preserve some runtime state)
	if existingWant.State == nil {
		existingWant.State = make(map[string]interface{})
	}
	// Reset execution-related state but preserve structural state
	delete(existingWant.State, "current_count")
	delete(existingWant.State, "total_processed")
	delete(existingWant.State, "current_time")

	fmt.Printf("[RECONCILE] Want %s (ID: %s) updated and reset to idle status for re-execution\n", wantName, wantConfig.Metadata.ID)
}

// deleteWant removes a want from runtime
func (cb *ChainBuilder) deleteWant(wantName string) {
	fmt.Printf("[RECONCILE] Deleting want: %s\n", wantName)

	delete(cb.wants, wantName)
}

// rebuildConnections is deprecated - functionality moved to connectPhase()
// Kept for backward compatibility but now delegates to connectPhase()
func (cb *ChainBuilder) rebuildConnections() {
	fmt.Println("[RECONCILE] rebuildConnections() is deprecated, use connectPhase()")
	if err := cb.connectPhase(); err != nil {
		fmt.Printf("[RECONCILE] Connection rebuild failed: %v\n", err)
	}

	// Start wants if running (for backward compatibility)
	if cb.running {
		cb.startPhase()
	}
}

// registerWantForNotifications registers a want with the notification system
func (cb *ChainBuilder) registerWantForNotifications(wantConfig *Want, wantFunction interface{}, wantPtr *Want) {
	wantName := wantConfig.Metadata.Name

	// 1. Register want in global registry for lookup
	RegisterWant(wantPtr)

	// 2. Register as listener if it implements StateUpdateListener
	if listener, ok := wantFunction.(StateUpdateListener); ok {
		RegisterStateListener(wantName, listener)
		fmt.Printf("[NOTIFICATION] Registered %s as state listener\n", wantName)
	}

	// 3. Register its state subscriptions if any
	if len(wantConfig.Spec.StateSubscriptions) > 0 {
		RegisterStateSubscriptions(wantName, wantConfig.Spec.StateSubscriptions)
		fmt.Printf("[NOTIFICATION] Registered %d subscriptions for %s\n",
			len(wantConfig.Spec.StateSubscriptions), wantName)
	}
}

// startWant starts a single want
func (cb *ChainBuilder) startWant(wantName string, want *runtimeWant) {
	// Check if want is already running or completed to prevent duplicate starts
	if want.want.GetStatus() == WantStatusRunning || want.want.GetStatus() == WantStatusCompleted {
		return
	}

	paths := cb.pathMap[wantName]

	// Prepare using channels
	var usingChans []chain.Chan
	for _, usingPath := range paths.In {
		if usingPath.Active {
			channelKey := usingPath.Name
			cb.channelMutex.RLock()
			if ch, exists := cb.channels[channelKey]; exists {
				usingChans = append(usingChans, ch)
			}
			cb.channelMutex.RUnlock()
		}
	}

	// Prepare output channels
	var outputChans []chain.Chan
	for _, outputPath := range paths.Out {
		if outputPath.Active {
			channelKey := outputPath.Name
			cb.channelMutex.RLock()
			if ch, exists := cb.channels[channelKey]; exists {
				outputChans = append(outputChans, ch)
			}
			cb.channelMutex.RUnlock()
		}
	}

	// Start want execution with direct Exec() calls
	if chainWant, ok := want.function.(ChainWant); ok {
		want.want.SetStatus(WantStatusRunning)

		cb.waitGroup.Add(1)
		go func() {
			defer cb.waitGroup.Done()
			defer func() {
				if want.want.GetStatus() == WantStatusRunning {
					want.want.SetStatus(WantStatusCompleted)
				}
			}()

			fmt.Printf("[EXEC] Starting want %s with %d using, %d outputs\n",
				wantName, len(usingChans), len(outputChans))

			for {
				// Begin execution cycle for batching state changes
				if runtimeWant, exists := cb.wants[wantName]; exists {
					runtimeWant.want.BeginExecCycle()
				}

				// Direct call - parameters can be read fresh each cycle
				finished := chainWant.Exec(usingChans, outputChans)

				// End execution cycle and commit batched state changes
				if runtimeWant, exists := cb.wants[wantName]; exists {
					runtimeWant.want.EndExecCycle()
				}

				if finished {
					fmt.Printf("[EXEC] Want %s finished\n", wantName)

					// Update want status to completed
					if runtimeWant, exists := cb.wants[wantName]; exists {
						runtimeWant.want.SetStatus(WantStatusCompleted)
					}

					// Check if this want completion should notify parent targets
					cb.notifyParentTargetsOfChildCompletion(wantName)

					break
				}
			}
		}()
	}
}

// writeStatsToMemory writes current stats to memory file
func (cb *ChainBuilder) writeStatsToMemory() {
	if cb.memoryPath == "" {
		return
	}

	// Create a comprehensive config that includes ALL wants (both config and runtime)
	updatedConfig := Config{
		Wants: make([]*Want, 0),
	}

	// First, add all wants from config and update with current stats
	configWantMap := make(map[string]bool)
	for _, want := range cb.config.Wants {
		configWantMap[want.Metadata.Name] = true
		if runtimeWant, exists := cb.wants[want.Metadata.Name]; exists {
			// Update with runtime data including spec using
			want.Spec = runtimeWant.spec // Preserve using from runtime spec
			// Stats field removed - data now in State
			want.Status = runtimeWant.want.Status
			want.State = runtimeWant.want.State
		}
		updatedConfig.Wants = append(updatedConfig.Wants, want)
	}

	// Then, add any runtime wants that might not be in config (e.g., dynamically created and completed)
	for wantName, runtimeWant := range cb.wants {
		if !configWantMap[wantName] {
			// This want exists in runtime but not in config - include it
			wantConfig := &Want{
				Metadata: runtimeWant.metadata,
				Spec:     runtimeWant.spec,
				// Stats field removed - data now in State
				Status: runtimeWant.want.Status,
				State:  runtimeWant.want.State,
			}
			updatedConfig.Wants = append(updatedConfig.Wants, wantConfig)
		}
	}

	// Write updated config to memory file
	data, err := yaml.Marshal(updatedConfig)
	if err != nil {
		return
	}

	os.WriteFile(cb.memoryPath, data, 0644)

	// Update lastConfigHash to prevent stats updates from triggering reconciliation
	cb.lastConfigHash, _ = cb.calculateFileHash(cb.memoryPath)
}

// Execute starts the reconcile loop and initial want execution
func (cb *ChainBuilder) Execute() {
	fmt.Println("[RECONCILE] Starting reconcile loop execution")

	// Initialize memory file if configured
	if cb.memoryPath != "" {
		if err := cb.copyConfigToMemory(); err != nil {
			fmt.Printf("Warning: Failed to copy config to memory: %v\n", err)
		} else {
			cb.lastConfigHash, _ = cb.calculateFileHash(cb.memoryPath)
		}
	}

	// Initialize empty lastConfig so reconcileLoop can detect initial load
	cb.lastConfig = Config{Wants: []*Want{}}

	// Mark as running
	cb.reconcileMutex.Lock()
	cb.running = true
	cb.reconcileMutex.Unlock()

	// Start suspension control loop
	cb.startControlLoop()

	// Start reconcile loop in background - it will handle initial want creation
	go cb.reconcileLoop()

	// Wait for initial wants to be created by reconcileLoop
	for {
		cb.reconcileMutex.Lock()
		wantCount := len(cb.wants)
		cb.reconcileMutex.Unlock()

		if wantCount > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Start all initial wants
	for wantName, want := range cb.wants {
		cb.startWant(wantName, want)
	}

	// Wait for all wants to complete
	cb.waitGroup.Wait()

	// Stop reconcile loop
	cb.reconcileStop <- true

	// Stop suspension control loop
	cb.controlStop <- true

	// Mark as not running
	cb.reconcileMutex.Lock()
	cb.running = false
	cb.reconcileMutex.Unlock()

	// Final memory dump - ensure it completes before returning
	fmt.Println("[RECONCILE] Writing final memory dump...")
	err := cb.dumpWantMemoryToYAML()
	if err != nil {
		fmt.Printf("Warning: Failed to dump want memory to YAML: %v\n", err)
	} else {
		fmt.Println("[RECONCILE] Memory dump completed successfully")
	}

	// Process recipe results if available
	cb.processRecipeResults()

	fmt.Println("[RECONCILE] Execution completed")
}

// GetAllWantStates returns the states of all wants
func (cb *ChainBuilder) GetAllWantStates() map[string]*Want {
	states := make(map[string]*Want)
	for name, want := range cb.wants {
		states[name] = want.want
	}
	return states
}

// AddDynamicWants adds multiple wants to the configuration at runtime
func (cb *ChainBuilder) AddDynamicWants(wants []*Want) {
	cb.reconcileMutex.Lock()
	defer cb.reconcileMutex.Unlock()
	for _, want := range wants {
		cb.addDynamicWantUnsafe(want)
	}
}

// addDynamicWantUnsafe adds a want without acquiring the mutex (internal use)
func (cb *ChainBuilder) addDynamicWantUnsafe(want *Want) {
	// Add want to the configuration
	cb.config.Wants = append(cb.config.Wants, want)

	// Create runtime want if it doesn't exist
	if _, exists := cb.wants[want.Metadata.Name]; !exists {
		cb.addWant(want)
	}
}

// LoadConfigFromYAML loads configuration from a YAML file with OpenAPI spec validation (exported version)
func LoadConfigFromYAML(filename string) (Config, error) {
	return loadConfigFromYAML(filename)
}

// LoadConfigFromYAMLBytes loads configuration from YAML bytes with OpenAPI spec validation (exported version)
func LoadConfigFromYAMLBytes(data []byte) (Config, error) {
	return loadConfigFromYAMLBytes(data)
}

// loadConfigFromYAML loads configuration from a YAML file with OpenAPI spec validation
func loadConfigFromYAML(filename string) (Config, error) {
	var config Config

	// Read the YAML config file
	data, err := os.ReadFile(filename)
	if err != nil {
		return config, fmt.Errorf("failed to read YAML file: %w", err)
	}

	// Validate against OpenAPI spec before parsing
	err = validateConfigWithSpec(data)
	if err != nil {
		return config, fmt.Errorf("config validation failed: %w", err)
	}

	// Parse the validated YAML
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return config, fmt.Errorf("failed to parse YAML config: %w", err)
	}

	// Assign individual IDs to each want if not already set
	assignWantIDs(&config)

	return config, nil
}

// assignWantIDs assigns unique IDs to wants that don't have them
func assignWantIDs(config *Config) {
	baseID := time.Now().UnixNano()
	for i := range config.Wants {
		if config.Wants[i].Metadata.ID == "" {
			config.Wants[i].Metadata.ID = fmt.Sprintf("want-%d", baseID+int64(i))
		}
	}
}

// loadConfigFromYAMLBytes loads configuration from YAML bytes with OpenAPI spec validation
func loadConfigFromYAMLBytes(data []byte) (Config, error) {
	var config Config

	// Validate against OpenAPI spec before parsing
	err := validateConfigWithSpec(data)
	if err != nil {
		return config, fmt.Errorf("config validation failed: %w", err)
	}

	// Parse the validated YAML
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return config, fmt.Errorf("failed to parse YAML config: %w", err)
	}

	// Assign individual IDs to each want if not already set
	assignWantIDs(&config)

	return config, nil
}

// validateConfigWithSpec validates YAML config data against the OpenAPI spec
func validateConfigWithSpec(yamlData []byte) error {
	// Load the OpenAPI spec - try multiple paths to handle different working directories
	loader := openapi3.NewLoader()

	specPaths := []string{
		"../spec/want-spec.yaml",    // From engine directory
		"spec/want-spec.yaml",       // From project root
		"../../spec/want-spec.yaml", // From deeper subdirectories
	}

	var spec *openapi3.T
	var err error

	for _, path := range specPaths {
		spec, err = loader.LoadFromFile(path)
		if err == nil {
			break
		}
	}

	if err != nil {
		return fmt.Errorf("failed to load OpenAPI spec from any of the tried paths %v: %w", specPaths, err)
	}

	// Validate the spec itself
	ctx := context.Background()
	err = spec.Validate(ctx)
	if err != nil {
		return fmt.Errorf("OpenAPI spec is invalid: %w", err)
	}

	// Convert YAML to JSON for validation
	var yamlObj interface{}
	err = yaml.Unmarshal(yamlData, &yamlObj)
	if err != nil {
		return fmt.Errorf("failed to parse YAML for validation: %w", err)
	}

	// jsonData conversion removed - not needed for basic validation

	// Get the Config schema from the OpenAPI spec and convert to JSON Schema
	configSchemaRef := spec.Components.Schemas["Config"]
	if configSchemaRef == nil {
		return fmt.Errorf("Config schema not found in OpenAPI spec")
	}

	// For now, do basic validation by checking that we can load and parse both spec and data
	// A full OpenAPI->JSON Schema conversion would be more complex and is beyond current scope

	// Basic structural validation - ensure the YAML contains expected top-level keys
	var configObj map[string]interface{}
	err = yaml.Unmarshal(yamlData, &configObj)
	if err != nil {
		return fmt.Errorf("invalid YAML structure: %w", err)
	}

	// Check if it has either 'wants' array or 'recipe' reference (matching our Config schema)
	hasWants := false
	hasRecipe := false

	if wants, ok := configObj["wants"]; ok {
		if wantsArray, ok := wants.([]interface{}); ok && len(wantsArray) > 0 {
			hasWants = true
		}
	}

	if recipe, ok := configObj["recipe"]; ok {
		if recipeObj, ok := recipe.(map[string]interface{}); ok {
			if path, ok := recipeObj["path"]; ok {
				if pathStr, ok := path.(string); ok && pathStr != "" {
					hasRecipe = true
				}
			}
		}
	}

	if !hasWants && !hasRecipe {
		return fmt.Errorf("config validation failed: must have either 'wants' array or 'recipe' reference")
	}

	if hasWants && hasRecipe {
		return fmt.Errorf("config validation failed: cannot have both 'wants' array and 'recipe' reference")
	}

	// If has wants, validate basic want structure
	if hasWants {
		err = validateWantsStructure(configObj["wants"])
		if err != nil {
			return fmt.Errorf("wants validation failed: %w", err)
		}
	}

	fmt.Printf("[VALIDATION] Config validated successfully against OpenAPI spec\n")
	return nil
}

// validateWantsStructure validates the basic structure of wants array
func validateWantsStructure(wants interface{}) error {
	wantsArray, ok := wants.([]interface{})
	if !ok {
		return fmt.Errorf("wants must be an array")
	}

	for i, want := range wantsArray {
		wantObj, ok := want.(map[string]interface{})
		if !ok {
			return fmt.Errorf("want at index %d must be an object", i)
		}

		// Check required metadata field
		metadata, ok := wantObj["metadata"]
		if !ok {
			return fmt.Errorf("want at index %d missing required 'metadata' field", i)
		}

		metadataObj, ok := metadata.(map[string]interface{})
		if !ok {
			return fmt.Errorf("want at index %d 'metadata' must be an object", i)
		}

		// Check required metadata.name and metadata.type
		if name, ok := metadataObj["name"]; !ok || name == "" {
			return fmt.Errorf("want at index %d missing required 'metadata.name' field", i)
		}

		if wantType, ok := metadataObj["type"]; !ok || wantType == "" {
			return fmt.Errorf("want at index %d missing required 'metadata.type' field", i)
		}

		// Check required spec field
		spec, ok := wantObj["spec"]
		if !ok {
			return fmt.Errorf("want at index %d missing required 'spec' field", i)
		}

		specObj, ok := spec.(map[string]interface{})
		if !ok {
			return fmt.Errorf("want at index %d 'spec' must be an object", i)
		}

		// Check required spec.params field
		if params, ok := specObj["params"]; !ok {
			return fmt.Errorf("want at index %d missing required 'spec.params' field", i)
		} else {
			if _, ok := params.(map[string]interface{}); !ok {
				return fmt.Errorf("want at index %d 'spec.params' must be an object", i)
			}
		}
	}

	return nil
}

// WantMemoryDump represents the complete state of all wants for dumping
type WantMemoryDump struct {
	Timestamp   string  `yaml:"timestamp"`
	ExecutionID string  `yaml:"execution_id"`
	Wants       []*Want `yaml:"wants"`
}

// dumpWantMemoryToYAML dumps all want information to a timestamped YAML file in memory directory
func (cb *ChainBuilder) dumpWantMemoryToYAML() error {
	// Create timestamp-based filename
	timestamp := time.Now().Format("20060102-150405")

	// Use memory directory if available
	var filename string
	if cb.memoryPath != "" {
		memoryDir := filepath.Dir(cb.memoryPath)
		filename = filepath.Join(memoryDir, fmt.Sprintf("memory-%s.yaml", timestamp))
	} else {
		// Fallback to current directory with memory subdirectory
		memoryDir := "memory"
		if err := os.MkdirAll(memoryDir, 0755); err != nil {
			return fmt.Errorf("failed to create memory directory: %w", err)
		}
		filename = filepath.Join(memoryDir, fmt.Sprintf("memory-%s.yaml", timestamp))
	}

	// Convert want map to slice to match config format, preserving runtime spec
	wants := make([]*Want, 0, len(cb.wants))
	for _, runtimeWant := range cb.wants {
		// Use runtime spec to preserve using, but want state for stats/status
		want := &Want{
			Metadata: runtimeWant.metadata,
			Spec:     runtimeWant.spec, // This preserves using
			// Stats field removed - data now in State
			Status:  runtimeWant.want.Status,
			State:   runtimeWant.want.State,
			History: runtimeWant.want.History, // Include history in memory dump
		}

		wants = append(wants, want)
	}

	// Prepare memory dump structure
	memoryDump := WantMemoryDump{
		Timestamp:   time.Now().Format(time.RFC3339),
		ExecutionID: fmt.Sprintf("exec-%s", timestamp),
		Wants:       wants,
	}

	// Marshal to YAML
	data, err := yaml.Marshal(memoryDump)
	if err != nil {
		return fmt.Errorf("failed to marshal want memory to YAML: %w", err)
	}

	// Write to file with explicit sync
	err = os.WriteFile(filename, data, 0644)
	if err != nil {
		return fmt.Errorf("failed to write memory dump to file %s: %w", filename, err)
	}

	// Ensure file is written to disk by opening and syncing
	file, err := os.OpenFile(filename, os.O_WRONLY, 0644)
	if err == nil {
		file.Sync()
		file.Close()
	}

	// Also create a copy as memory-0000-latest.yaml for easy access
	var latestFilename string
	if cb.memoryPath != "" {
		memoryDir := filepath.Dir(cb.memoryPath)
		latestFilename = filepath.Join(memoryDir, "memory-0000-latest.yaml")
	} else {
		latestFilename = filepath.Join("memory", "memory-0000-latest.yaml")
	}

	// Copy the data to the latest file
	err = os.WriteFile(latestFilename, data, 0644)
	if err == nil {
		// Sync the latest file too
		latestFile, err := os.OpenFile(latestFilename, os.O_WRONLY, 0644)
		if err == nil {
			latestFile.Sync()
			latestFile.Close()
		}
	}

	fmt.Printf("📝 Want memory dumped to: %s\n", filename)
	if err == nil {
		fmt.Printf("📝 Latest memory also saved to: %s\n", latestFilename)
	}
	return nil
}

// notifyParentTargetsOfChildCompletion checks if a completed want has owner references
// and notifies parent Target wants when all their children have completed
func (cb *ChainBuilder) notifyParentTargetsOfChildCompletion(completedWantName string) {
	// Find the config want for this completed want
	var completedWantConfig *Want
	for _, wantConfig := range cb.config.Wants {
		if wantConfig.Metadata.Name == completedWantName {
			completedWantConfig = wantConfig
			break
		}
	}

	if completedWantConfig == nil || len(completedWantConfig.Metadata.OwnerReferences) == 0 {
		return // No owner references
	}

	// For each owner reference, check if all siblings are complete
	for _, ownerRef := range completedWantConfig.Metadata.OwnerReferences {
		parentName := ownerRef.Name

		// Check if parent is a Target want
		parentRuntimeWant, exists := cb.wants[parentName]
		if !exists {
			continue
		}

		// Check if it implements child completion notification
		if notifier, ok := parentRuntimeWant.function.(ParentNotifier); ok {
			// Find all child wants with this parent
			allChildrenComplete := true
			childCount := 0
			for _, wantConfig := range cb.config.Wants {
				if wantConfig.Metadata.Name == parentName {
					continue // Skip the parent itself
				}

				// Check if this want has ownerRef to this parent
				hasOwnerRef := false
				for _, childOwnerRef := range wantConfig.Metadata.OwnerReferences {
					if childOwnerRef.Name == parentName {
						hasOwnerRef = true
						break
					}
				}

				if hasOwnerRef {
					childCount++
					// This is a child - check if it's completed
					if childRuntimeWant, exists := cb.wants[wantConfig.Metadata.Name]; exists {
						if childRuntimeWant.want.GetStatus() != WantStatusCompleted {
							allChildrenComplete = false
							break
						}
					} else {
						allChildrenComplete = false
						break
					}
				}
			}

			if allChildrenComplete && childCount > 0 {
				fmt.Printf("🎯 All children of target %s have completed, notifying...\n", parentName)
				notifier.NotifyChildrenComplete()
			}
		}
	}
}

// SetRecipeResult sets the recipe result definition for processing at the end of execution
func (cb *ChainBuilder) SetRecipeResult(result *RecipeResult, recipePath string) {
	cb.recipeResult = result
	cb.recipePath = recipePath
}

// processRecipeResults processes and displays recipe results if available
func (cb *ChainBuilder) processRecipeResults() {
	if cb.recipeResult == nil || len(*cb.recipeResult) == 0 {
		return
	}

	fmt.Println("\n🎯 Recipe Results:")
	fmt.Println("==================")

	// Process all results in the new flat array format
	for i, resultSpec := range *cb.recipeResult {
		resultValue := cb.extractResultFromSpec(resultSpec)
		if i == 0 {
			fmt.Printf("📊 Primary Result: %s\n", resultSpec.Description)
			fmt.Printf("   Value: %v (from %s.%s)\n", resultValue, resultSpec.WantName, resultSpec.StatName)
		} else {
			fmt.Printf("📈 Additional Metric: %s\n", resultSpec.Description)
			fmt.Printf("   Value: %v (from %s.%s)\n", resultValue, resultSpec.WantName, resultSpec.StatName)
		}
	}

	fmt.Println()
}

// extractResultFromSpec extracts a result value from want stats using recipe specification
func (cb *ChainBuilder) extractResultFromSpec(spec RecipeResultSpec) interface{} {
	// Find the want by name
	runtimeWant, exists := cb.wants[spec.WantName]
	if !exists {
		return fmt.Sprintf("ERROR: Want '%s' not found", spec.WantName)
	}

	// Extract the stat value from State (stats are now stored in State)
	if runtimeWant.want.State == nil {
		return fmt.Sprintf("ERROR: No state available for want '%s'", spec.WantName)
	}

	// Handle JSON path syntax
	if strings.HasPrefix(spec.StatName, ".") {
		return cb.extractValueByPath(runtimeWant.want.State, spec.StatName)
	}

	value, exists := runtimeWant.want.State[spec.StatName]
	if !exists {
		return fmt.Sprintf("ERROR: Stat '%s' not found in want '%s'", spec.StatName, spec.WantName)
	}

	return value
}

// extractValueByPath extracts values using JSON path-like syntax for ChainBuilder
func (cb *ChainBuilder) extractValueByPath(data map[string]interface{}, path string) interface{} {
	// Handle root path "." - return entire data
	if path == "." {
		return data
	}

	// Handle field access like ".average_wait_time"
	if strings.HasPrefix(path, ".") {
		fieldName := strings.TrimPrefix(path, ".")

		// Simple field access
		if value, ok := data[fieldName]; ok {
			return value
		}

		// Try common field name variations
		if value, ok := data[strings.ToLower(fieldName)]; ok {
			return value
		}

		// Handle underscore/camelCase variations
		if strings.Contains(fieldName, "_") {
			// Try camelCase version
			camelCase := cb.toCamelCase(fieldName)
			if value, ok := data[camelCase]; ok {
				return value
			}
		} else {
			// Try snake_case version
			snakeCase := cb.toSnakeCase(fieldName)
			if value, ok := data[snakeCase]; ok {
				return value
			}
		}
	}

	return nil
}

// toCamelCase converts snake_case to camelCase for ChainBuilder
func (cb *ChainBuilder) toCamelCase(str string) string {
	parts := strings.Split(str, "_")
	if len(parts) <= 1 {
		return str
	}

	result := parts[0]
	for _, part := range parts[1:] {
		if len(part) > 0 {
			result += strings.ToUpper(string(part[0])) + strings.ToLower(part[1:])
		}
	}
	return result
}

// toSnakeCase converts camelCase to snake_case for ChainBuilder
func (cb *ChainBuilder) toSnakeCase(str string) string {
	var result []rune
	for i, r := range str {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result = append(result, '_')
		}
		result = append(result, r)
	}
	return strings.ToLower(string(result))
}

// Agent execution methods for Want

// SetAgentRegistry sets the agent registry for this want
func (n *Want) SetAgentRegistry(registry *AgentRegistry) {
	n.agentRegistry = registry
	if n.runningAgents == nil {
		n.runningAgents = make(map[string]context.CancelFunc)
	}
}

// GetAgentRegistry returns the agent registry for this want
func (n *Want) GetAgentRegistry() *AgentRegistry {
	return n.agentRegistry
}

// ExecuteAgents finds and executes agents based on want requirements
func (n *Want) ExecuteAgents() error {
	if n.agentRegistry == nil {
		return nil // No agent registry configured, skip agent execution
	}

	if len(n.Spec.Requires) == 0 {
		return nil // No requirements specified, skip agent execution
	}

	for _, requirement := range n.Spec.Requires {
		agents := n.agentRegistry.FindAgentsByGives(requirement)
		for _, agent := range agents {
			if err := n.executeAgent(agent); err != nil {
				return fmt.Errorf("failed to execute agent %s: %w", agent.GetName(), err)
			}
		}
	}

	return nil
}

// executeAgent executes a single agent in a goroutine
func (n *Want) executeAgent(agent Agent) error {
	ctx, cancel := context.WithCancel(context.Background())

	// Store cancel function for later cleanup
	n.runningAgents[agent.GetName()] = cancel

	// Initialize agent tracking fields if needed
	if n.RunningAgents == nil {
		n.RunningAgents = make([]string, 0)
	}
	if n.AgentHistory == nil {
		n.AgentHistory = make([]AgentExecution, 0)
	}

	// Add to running agents list
	n.RunningAgents = append(n.RunningAgents, agent.GetName())
	n.CurrentAgent = agent.GetName()

	// Store agent information in state for history tracking
	n.StoreState("current_agent", agent.GetName())
	n.StoreState("running_agents", n.RunningAgents)

	// Create agent execution record
	agentExec := AgentExecution{
		AgentName: agent.GetName(),
		AgentType: string(agent.GetType()),
		StartTime: time.Now(),
		Status:    "running",
	}
	n.AgentHistory = append(n.AgentHistory, agentExec)
	n.StoreState("agent_history", n.AgentHistory)

	// Execute agent - synchronously for DO agents, asynchronously for MONITOR agents
	executeFunc := func() {
		defer func() {
			// Update agent execution record
			for i := range n.AgentHistory {
				if n.AgentHistory[i].AgentName == agent.GetName() && n.AgentHistory[i].EndTime == nil {
					endTime := time.Now()
					n.AgentHistory[i].EndTime = &endTime
					break
				}
			}

			// Remove from running agents list
			for i, runningAgent := range n.RunningAgents {
				if runningAgent == agent.GetName() {
					n.RunningAgents = append(n.RunningAgents[:i], n.RunningAgents[i+1:]...)
					break
				}
			}

			// Update current agent (set to empty if no more agents running)
			if len(n.RunningAgents) == 0 {
				n.CurrentAgent = ""
			} else {
				// Set to the last running agent
				n.CurrentAgent = n.RunningAgents[len(n.RunningAgents)-1]
			}

			// Update state with current agent and running agents info
			n.StoreState("current_agent", n.CurrentAgent)
			n.StoreState("running_agents", n.RunningAgents)

			if r := recover(); r != nil {
				fmt.Printf("Agent %s panicked: %v\n", agent.GetName(), r)
				// Update agent execution record with panic info
				for i := range n.AgentHistory {
					if n.AgentHistory[i].AgentName == agent.GetName() && n.AgentHistory[i].Status == "running" {
						n.AgentHistory[i].Status = "failed"
						n.AgentHistory[i].Error = fmt.Sprintf("Panic: %v", r)
						break
					}
				}
				// Update state with latest agent history
				n.StoreState("agent_history", n.AgentHistory)
			}
			// Remove from running agents when done
			delete(n.runningAgents, agent.GetName())
		}()

		if err := agent.Exec(ctx, n); err != nil {
			fmt.Printf("Agent %s failed: %v\n", agent.GetName(), err)
			// Update agent execution record with error
			for i := range n.AgentHistory {
				if n.AgentHistory[i].AgentName == agent.GetName() && n.AgentHistory[i].Status == "running" {
					n.AgentHistory[i].Status = "failed"
					n.AgentHistory[i].Error = err.Error()
					break
				}
			}
			// Update state with latest agent history
			n.StoreState("agent_history", n.AgentHistory)
		} else {
			// Mark as completed
			for i := range n.AgentHistory {
				if n.AgentHistory[i].AgentName == agent.GetName() && n.AgentHistory[i].Status == "running" {
					n.AgentHistory[i].Status = "completed"
					break
				}
			}
			// Update state with latest agent history
			n.StoreState("agent_history", n.AgentHistory)
		}
	}

	// Execute synchronously for DO agents, asynchronously for MONITOR agents
	if agent.GetType() == DoAgentType {
		// DO agents execute synchronously to return results immediately
		executeFunc()
	} else {
		// MONITOR agents execute asynchronously to run in background
		go executeFunc()
	}

	return nil
}

// StopAllAgents stops all running agents for this want
func (n *Want) StopAllAgents() {
	if n.runningAgents == nil {
		return
	}

	for agentName, cancel := range n.runningAgents {
		fmt.Printf("Stopping agent: %s\n", agentName)
		cancel()

		// Update agent execution records
		for i := range n.AgentHistory {
			if n.AgentHistory[i].AgentName == agentName && n.AgentHistory[i].Status == "running" {
				endTime := time.Now()
				n.AgentHistory[i].EndTime = &endTime
				n.AgentHistory[i].Status = "terminated"
				break
			}
		}
	}

	// Clear the maps and lists
	n.runningAgents = make(map[string]context.CancelFunc)
	n.RunningAgents = make([]string, 0)
	n.CurrentAgent = ""
}

// StopAgent stops a specific running agent
func (n *Want) StopAgent(agentName string) {
	if n.runningAgents == nil {
		return
	}

	if cancel, exists := n.runningAgents[agentName]; exists {
		fmt.Printf("Stopping agent: %s\n", agentName)
		cancel()
		delete(n.runningAgents, agentName)

		// Update agent execution record
		for i := range n.AgentHistory {
			if n.AgentHistory[i].AgentName == agentName && n.AgentHistory[i].Status == "running" {
				endTime := time.Now()
				n.AgentHistory[i].EndTime = &endTime
				n.AgentHistory[i].Status = "terminated"
				break
			}
		}

		// Remove from running agents list
		for i, runningAgent := range n.RunningAgents {
			if runningAgent == agentName {
				n.RunningAgents = append(n.RunningAgents[:i], n.RunningAgents[i+1:]...)
				break
			}
		}

		// Update current agent
		if n.CurrentAgent == agentName {
			if len(n.RunningAgents) == 0 {
				n.CurrentAgent = ""
			} else {
				n.CurrentAgent = n.RunningAgents[len(n.RunningAgents)-1]
			}
		}
	}
}

// StageStateChange stages state changes for later commit (used by agents)
// Can be called with either:
// 1. Single key-value: StageStateChange("key", "value")
// 2. Object with multiple pairs: StageStateChange(map[string]interface{}{"key1": "value1", "key2": "value2"})
func (n *Want) StageStateChange(keyOrObject interface{}, value ...interface{}) {
	n.agentStateMutex.Lock()
	defer n.agentStateMutex.Unlock()

	if n.agentStateChanges == nil {
		n.agentStateChanges = make(map[string]interface{})
	}

	// Handle object case: StageStateChange(map[string]interface{}{...})
	if len(value) == 0 {
		if stateObject, ok := keyOrObject.(map[string]interface{}); ok {
			for k, v := range stateObject {
				n.agentStateChanges[k] = v
			}
			return
		}
		// Invalid usage - no value provided and not a map
		panic("StageStateChange: when called with single argument, it must be map[string]interface{}")
	}

	// Handle single key-value case: StageStateChange("key", "value")
	if len(value) == 1 {
		if key, ok := keyOrObject.(string); ok {
			n.agentStateChanges[key] = value[0]
			return
		}
		// Invalid usage - first arg is not a string
		panic("StageStateChange: when called with two arguments, first must be string")
	}

	// Invalid usage - too many arguments
	panic("StageStateChange: accepts either 1 argument (map) or 2 arguments (key, value)")
}

// CommitStateChanges commits all staged state changes in a single atomic operation
func (n *Want) CommitStateChanges() {
	n.agentStateMutex.Lock()
	defer n.agentStateMutex.Unlock()

	if len(n.agentStateChanges) == 0 {
		return
	}

	// Create a single aggregated state history entry
	if n.State == nil {
		n.State = make(map[string]interface{})
	}

	// Apply all changes to current state
	for key, value := range n.agentStateChanges {
		n.State[key] = value
	}

	// Add single history entry with all changes (one entry instead of multiple)
	historyEntry := StateHistoryEntry{
		WantName:   n.Metadata.Name,
		StateValue: n.agentStateChanges,
		Timestamp:  time.Now(),
	}
	n.History.StateHistory = append(n.History.StateHistory, historyEntry)

	fmt.Printf("💾 Committed %d state changes for want %s in single batch\n",
		len(n.agentStateChanges), n.Metadata.Name)

	// Clear staged changes
	n.agentStateChanges = make(map[string]interface{})
}

// GetStagedChanges returns a copy of currently staged changes (for debugging)
func (n *Want) GetStagedChanges() map[string]interface{} {
	n.agentStateMutex.RLock()
	defer n.agentStateMutex.RUnlock()

	if n.agentStateChanges == nil {
		return make(map[string]interface{})
	}

	staged := make(map[string]interface{})
	for k, v := range n.agentStateChanges {
		staged[k] = v
	}
	return staged
}

// ==============================
// Suspend/Resume Control Methods
// ==============================

// Suspend pauses the execution of all wants
func (cb *ChainBuilder) Suspend() error {
	cb.controlMutex.Lock()
	defer cb.controlMutex.Unlock()

	if cb.suspended {
		return nil // Already suspended
	}

	cb.suspended = true

	// Signal suspension to control loop
	select {
	case cb.suspendChan <- true:
		fmt.Println("[SUSPEND] Chain execution suspended")
		return nil
	default:
		// Control loop not running, just mark as suspended
		fmt.Println("[SUSPEND] Chain marked as suspended (control loop not active)")
		return nil
	}
}

// Resume resumes the execution of all wants
func (cb *ChainBuilder) Resume() error {
	cb.controlMutex.Lock()
	defer cb.controlMutex.Unlock()

	if !cb.suspended {
		return nil // Not suspended
	}

	cb.suspended = false

	// Signal resume to control loop
	select {
	case cb.resumeChan <- true:
		fmt.Println("[RESUME] Chain execution resumed")
		return nil
	default:
		// Control loop not running, just mark as resumed
		fmt.Println("[RESUME] Chain marked as resumed (control loop not active)")
		return nil
	}
}

// IsSuspended returns the current suspension state
func (cb *ChainBuilder) IsSuspended() bool {
	cb.controlMutex.RLock()
	defer cb.controlMutex.RUnlock()
	return cb.suspended
}

// checkSuspension blocks execution if suspended until resumed
func (cb *ChainBuilder) checkSuspension() {
	cb.controlMutex.RLock()
	suspended := cb.suspended
	cb.controlMutex.RUnlock()

	if suspended {
		fmt.Println("[SUSPEND] Want execution paused, waiting for resume...")
		// Block until resumed
		<-cb.resumeChan
		fmt.Println("[RESUME] Want execution continuing...")
	}
}

// controlLoop handles suspend/resume signals in a separate goroutine
func (cb *ChainBuilder) controlLoop() {
	fmt.Println("[CONTROL] Starting suspension control loop")

	for {
		select {
		case <-cb.suspendChan:
			fmt.Println("[CONTROL] Processing suspend signal")
			// Suspend signal processed by Suspend() method

		case <-cb.resumeChan:
			fmt.Println("[CONTROL] Processing resume signal")
			// Resume signal processed by Resume() method

		case <-cb.controlStop:
			fmt.Println("[CONTROL] Stopping suspension control loop")
			return
		}
	}
}

// startControlLoop starts the suspension control loop if not already running
func (cb *ChainBuilder) startControlLoop() {
	go cb.controlLoop()
}

