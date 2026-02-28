package mywant

import (
	"fmt"
	"log"
	"mywant/engine/core/chain"
	"mywant/engine/core/pubsub"
	"os"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gopkg.in/yaml.v3"
)

// Global ChainBuilder instance for accessing retrigger functions
var globalChainBuilder *ChainBuilder

type ChangeEventType string

const (
	ChangeEventAdd    ChangeEventType = "ADD"
	ChangeEventUpdate ChangeEventType = "UPDATE"
	ChangeEventDelete ChangeEventType = "DELETE"

	// GlobalExecutionInterval defines the sleep interval between goroutine execution cycles This prevents CPU spinning in tight execution loops for want execution goroutines Set to 100ms to reduce CPU usage during concurrent want execution
	GlobalExecutionInterval = 10 * time.Millisecond

	// GlobalReconcileInterval defines the frequency of reconcile loop operations (file change detection, config reloading, etc.)
	GlobalReconcileInterval = 100 * time.Millisecond
	// GlobalStatsInterval defines the frequency of stats writing to memory file (YAML serialization and file I/O)
	GlobalStatsInterval = 1 * time.Second
)

// ChangeEvent represents a configuration change
type ChangeEvent struct {
	Type   ChangeEventType
	WantID string
	Want   *Want
}

// APILogEntry represents a log entry for API operations
type APILogEntry struct {
	Timestamp  time.Time `json:"timestamp"`
	Method     string    `json:"method"`     // POST, PUT, DELETE, etc.
	Endpoint   string    `json:"endpoint"`   // /api/v1/wants, /api/v1/recipes, etc.
	Resource   string    `json:"resource"`   // Want/Recipe name or ID
	Status     string    `json:"status"`     // "success" or "error"
	StatusCode int       `json:"statusCode"` // HTTP status code
	ErrorMsg   string    `json:"errorMsg,omitempty"`
	Details    string    `json:"details,omitempty"`
}

// WantOperation represents a unified operation request (add, delete, suspend, resume, etc.)
type WantOperation struct {
	Type       string         // "add", "delete", "suspend", "resume", "stop", "start", "update", "addLabel", "removeLabel", "addUsing", "removeUsing"
	EntityType string         // "want", "recipe", "agent", "capability", "label"
	Wants      []*Want        // For add/update operations
	IDs        []string       // For delete/suspend/resume/stop/start operations (want/recipe IDs)
	Data       map[string]any // Additional operation parameters (labels, dependencies, etc.)
	Callback   chan<- error   // Optional callback channel for completion notification (non-blocking)
}

// ChainBuilder builds and executes chains from declarative configuration with reconcile loop
type ChainBuilder struct {
	configPath           string                          // Path to original config file
	memoryPath           string                          // Path to memory file (watched for changes)
	wants                map[string]*runtimeWant         // Runtime want registry
	registry             map[string]WantFactory          // Want type factories
	connectivityRegistry map[string]ConnectivityMetadata // Want type connectivity metadata from YAML
	wantTypeDefinitions  map[string]*WantTypeDefinition  // Want type definitions (state fields, etc.)
	customRegistry       *CustomTargetTypeRegistry       // Custom target type registry
	agentRegistry        *AgentRegistry                  // Agent registry for agent-enabled wants
	waitGroup            *sync.WaitGroup                 // Execution synchronization
	config               Config                          // Current configuration

	// Reconcile loop fields
	reconcileStop      chan bool            // Stop signal for reconcile loop
	reconcileTrigger   chan *TriggerCommand // Unified channel for reconciliation and control triggers
	addWantsChan       chan []*Want         // Buffered channel for asynchronous want addition requests (DEPRECATED: use operationChan instead)
	deleteWantsChan    chan []string        // Buffered channel for asynchronous want deletion requests (DEPRECATED: use operationChan instead)
	operationChan      chan *WantOperation  // Unified channel for all want/recipe operations (add, delete, suspend, resume, etc.)
	reconcileMutex     sync.RWMutex         // Protect concurrent access
	inReconciliation   bool                 // Flag to prevent recursive reconciliation
	running            bool                 // Execution state
	lastConfig         Config               // Last known config state
	lastConfigHash     string               // Hash of last config for change detection
	lastConfigFileHash string               // Hash of config file for change detection (batch mode)
	lastStatsHash      string               // Hash of last written stats for change detection

	// Path and channel management
	pathMap map[string]Paths // Want path mapping

	// Suspend/Resume control
	suspended atomic.Bool // Current suspension state (atomic, no mutex needed)

	// Connectivity warning tracking (to prevent duplicate logs in reconciliation loop)
	warnedConnectionIssues map[string]bool // Track which wants have already logged connectivity warnings

	// Completed want retrigger detection
	labelToUsers        map[string][]string // label selector key → want names that use this label
	wantCompletedFlags  map[string]bool     // want ID → is completed?
	completedFlagsMutex sync.RWMutex        // Protects wantCompletedFlags

	// Server mode flag
	isServerMode bool // True when running as API server (globalBuilder), false for batch/CLI mode

	// Initial load tracking
	hasInitialized bool // True after initial config load phase completes

	// API logging (lock-free ring buffer, fixed capacity)
	apiLogs *ringBuffer[APILogEntry]

	// HTTP client for internal API calls
	httpClient *HTTPClient // HTTP client for agents to call internal APIs

	// PubSub system for label-based packet delivery
	pubsub pubsub.PubSub // PubSub system for asynchronous packet delivery via labels

	// Managed PubSub adapter channels (topic + consumerID -> channel)
	// Always accessed inside reconcileMutex, no separate lock needed.
	pubsubChannels map[string]chain.Chan

	// Global Label Registry
	labelRegistry      map[string]map[string]bool // key -> value -> true
	labelRegistryMutex sync.RWMutex               // Protects labelRegistry

	// Correlation phase: set of Want IDs whose Correlation must be recomputed.
	// Populated by compilePhase, consumed and cleared by correlationPhase.
	// Always accessed inside reconcileMutex, no separate lock needed.
	dirtyWantIDs map[string]struct{}

	// State Access Index: fieldPath (wantID.fieldName) -> list of wantIDs that access it.
	// Used for efficient correlation calculation and dependency tracking.
	// Always accessed inside reconcileMutex, no separate lock needed.
	stateAccessIndex map[string][]string // "wantID.fieldName" -> []wantIDs
}

// runtimeWant holds the runtime state of a want
type runtimeWant struct {
	function any
	want     *Want
}

func (rw *runtimeWant) GetSpec() *WantSpec {
	if rw == nil || rw.want == nil {
		return nil
	}
	return rw.want.GetSpec()
}
func (rw *runtimeWant) GetMetadata() Metadata {
	if rw == nil || rw.want == nil {
		return Metadata{}
	}
	return rw.want.GetMetadata()
}

// NewChainBuilder creates a new builder from configuration
func NewChainBuilder(config Config) *ChainBuilder {
	builder := NewChainBuilderWithPaths("", "")
	builder.config = config
	return builder
}

// GetConfig returns the current builder Config (read-only copy)
func (cb *ChainBuilder) GetConfig() Config {
	return cb.config
}

// GetWants returns all runtime wants in the chain
func (cb *ChainBuilder) GetWants() []*Want {
	cb.reconcileMutex.RLock()
	defer cb.reconcileMutex.RUnlock()

	wants := make([]*Want, 0, len(cb.wants))
	for _, rw := range cb.wants {
		wants = append(wants, rw.want)
	}
	return wants
}

// NewChainBuilderWithPaths creates a new builder with config and memory file paths
func NewChainBuilderWithPaths(configPath, memoryPath string) *ChainBuilder {
	builder := &ChainBuilder{
		configPath:             configPath,
		memoryPath:             memoryPath,
		wants:                  make(map[string]*runtimeWant),
		registry:               make(map[string]WantFactory),
		wantTypeDefinitions:    make(map[string]*WantTypeDefinition),
		customRegistry:         NewCustomTargetTypeRegistry(),
		reconcileStop:          make(chan bool),
		reconcileTrigger:       make(chan *TriggerCommand, 20), // Unified channel for reconciliation and control triggers
		addWantsChan:           make(chan []*Want, 10),         // Buffered to allow concurrent submissions (DEPRECATED)
		deleteWantsChan:        make(chan []string, 10),        // Buffered to allow concurrent deletion requests (DEPRECATED)
		operationChan:          make(chan *WantOperation, 20),  // Unified channel for all operations
		pathMap:                make(map[string]Paths),
		running:                false,
		warnedConnectionIssues: make(map[string]bool), // Track logged connectivity warnings
		labelToUsers:           make(map[string][]string),
		wantCompletedFlags:     make(map[string]bool),
		waitGroup:              &sync.WaitGroup{},
		apiLogs:                newRingBuffer[APILogEntry](1000),
		pubsubChannels:         make(map[string]chain.Chan),
		labelRegistry:          make(map[string]map[string]bool),
		dirtyWantIDs:           make(map[string]struct{}),
		stateAccessIndex:       make(map[string][]string),
	}

	// Register all types that have Go implementations
	builder.RegisterAllKnownImplementations()

	// Note: Recipe scanning is done at server startup (main.go) via ScanAndRegisterCustomTypes() This avoids duplicate scanning logs when multiple ChainBuilder instances are created Recipe registry is passed via the environment during server initialization

	// Initialize PubSub system for label-based packet delivery
	builder.pubsub = pubsub.NewInMemoryPubSub()
	log.Printf("[ChainBuilder] Initialized PubSub system for label-based packet delivery")

	// Auto-register owner want types for target system support
	RegisterOwnerWantTypes(builder)

	// Auto-register scheduler want types
	RegisterSchedulerWantTypes(builder)

	return builder
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
// OPTIMIZATION: Reuses existing channels instead of creating new ones on every reconciliation cycle
func (cb *ChainBuilder) generatePathsFromConnections() map[string]Paths {
	pathMap := make(map[string]*Paths)
	for wantName := range cb.wants {
		pathMap[wantName] = &Paths{
			In:  []PathInfo{},
			Out: []PathInfo{},
		}
	}

	// Phase 3: Direct channel path creation removed
	// All connections now established via PubSub subscriptions in addPubSubPaths()
	// Using selectors are matched against provider labels via PubSub topics
	// Add PubSub subscription-backed paths for labeled wants
	cb.addPubSubPaths(pathMap)

	result := make(map[string]Paths)
	for wantName, pathsPtr := range pathMap {
		result[wantName] = *pathsPtr
	}

	return result
}

// addPubSubPaths adds PubSub subscription-backed input paths for wants that use labeled selectors.
// This enables late-arriving wants (e.g., Coordinators added after Providers) to receive packets.
// IMPORTANT: Only adds PubSub paths if a direct path doesn't already exist for the same provider.
// This prevents path duplication in nested recipes.
func (cb *ChainBuilder) addPubSubPaths(pathMap map[string]*Paths) {
	// For each want that has using selectors, subscribe to matching PubSub topics
	for wantName, want := range cb.wants {
		paths := pathMap[wantName]

		for selectorIdx, selector := range want.GetSpec().Using {
			if len(selector) == 0 {
				continue // Skip empty selectors
			}

			// Find matching provider wants
			matchCount := 0
			for providerName, providerWant := range cb.wants {
				if wantName == providerName {
					continue // Skip self
				}

				// Check if provider's labels match this selector
				if cb.matchesSelector(providerWant.want.Metadata.Labels, selector) {
					matchCount++

					// Subscribe to PubSub topic for this provider's labels
					topic := serializeLabels(providerWant.want.Metadata.Labels)
					adapterKey := fmt.Sprintf("%s:%s", topic, wantName)

					// reconcileMutex is already held by reconcileWants; no separate lock needed.
					adaptedChan, exists := cb.pubsubChannels[adapterKey]
					isSubscribed := cb.pubsub.IsSubscribed(topic, wantName)

					if !exists || !isSubscribed {
						// Create new subscription if it doesn't exist
						sub, err := cb.pubsub.Subscribe(topic, wantName)
						if err != nil {
							log.Printf("[PubSub] Failed to subscribe %s to topic %s: %v",
								wantName, topic, err)
							continue
						}

						// Create adapter channel that converts PubSub messages to TransportPackets
						adaptedChan = cb.adaptPubSubChannel(sub.Chan())
						cb.pubsubChannels[adapterKey] = adaptedChan
						log.Printf("[PubSub] Created NEW adapter channel for '%s' on topic '%s'", wantName, topic)
					}

					pubsubPath := PathInfo{
						Channel: adaptedChan,
						Name:    fmt.Sprintf("pubsub_%s_to_%s", topic, wantName),
						Active:  true,
					}

					// Add to input paths if not already present
					isDuplicate := false
					for _, existingPath := range paths.In {
						if existingPath.Name == pubsubPath.Name {
							isDuplicate = true
							break
						}
					}

					if !isDuplicate {
						paths.In = append(paths.In, pubsubPath)
						log.Printf("[PubSub] Want '%s' subscribed to topic '%s' (selector %d), paths.In count: %d",
							wantName, topic, selectorIdx, len(paths.In))
					}
				}
			}

			if matchCount == 0 && len(want.GetMetadata().Labels) == 0 {
				// No match yet - want might be added later dynamically
				log.Printf("[PubSub] Want '%s' has selector %v but no matching providers yet (will auto-connect on discovery)",
					wantName, selector)
			}
		}
	}
}

// adaptPubSubChannel converts a PubSub message channel to a TransportPacket channel.
// Runs in a goroutine to bridge the two channel types.
func (cb *ChainBuilder) adaptPubSubChannel(msgChan <-chan *pubsub.Message) chain.Chan {
	adapted := make(chain.Chan, 30) // Same buffer size as PubSub consumer buffer

	go func() {
		for msg := range msgChan {
			tp := TransportPacket{
				Payload: msg.Payload,
				Done:    msg.Done,
			}

			// Blocking send - do not drop messages
			// PubSub already handles backpressure, adapter should preserve all messages
			adapted <- tp
		}
		close(adapted)
	}()

	return adapted
}
func (cb *ChainBuilder) validateConnections(pathMap map[string]Paths) {
	for wantName, want := range cb.wants {
		paths := pathMap[wantName]
		meta := want.want.GetConnectivityMetadata()

		inCount := len(paths.In)
		outCount := len(paths.Out)
		if inCount < meta.RequiredInputs {
		}
		if outCount < meta.RequiredOutputs {
		}
		if meta.MaxInputs >= 0 && inCount > meta.MaxInputs {
		}
		if meta.MaxOutputs >= 0 && outCount > meta.MaxOutputs {
		}
	}
}

// isConnectivitySatisfied checks if a want's connectivity requirements are met
func (cb *ChainBuilder) isConnectivitySatisfied(wantName string, want *runtimeWant, pathMap map[string]Paths) bool {
	paths := pathMap[wantName]
	meta := want.want.GetConnectivityMetadata()

	inCount := len(paths.In)
	outCount := len(paths.Out)

	// For all wants, enforce normal connectivity requirements If want has required inputs, check if they're satisfied
	if meta.RequiredInputs > 0 && inCount < meta.RequiredInputs {
		return false
	}

	// If want has required outputs, check if they're satisfied
	if meta.RequiredOutputs > 0 && outCount < meta.RequiredOutputs {
		return false
	}

	return true
}

// reconcileLoop main reconcile loop that handles both initial config load and dynamic changes
func (cb *ChainBuilder) reconcileLoop() {
	// Initial configuration load
	cb.reconcileWants()

	// Use GlobalReconcileInterval (100ms) for config file change detection and reconciliation
	// Use GlobalStatsInterval (1 second) for stats writing to reduce I/O overhead
	ticker := time.NewTicker(GlobalReconcileInterval)
	statsTicker := time.NewTicker(GlobalStatsInterval)
	defer ticker.Stop()
	defer statsTicker.Stop()

	for {
		select {
		case <-cb.reconcileStop:
			return
		case trigger := <-cb.reconcileTrigger:
			if trigger == nil {
				continue
			}

			// Only log retrigger types to reduce log spam
			if trigger.Type == "check_completed_retrigger" {
			}

			switch trigger.Type {
			case "control":
				if trigger.ControlCommand != nil {
					cmd := trigger.ControlCommand
					// Control trigger received (logging removed to reduce spam)
					cb.distributeControlCommand(cmd)
				}

			case "check_completed_retrigger":
				cb.checkAndRetriggerCompletedWants()
				// Retrigger check finished

			default:
				cb.reconcileWants()
			}
		case newWants := <-cb.addWantsChan:
			cb.reconcileMutex.Lock()
			for _, want := range newWants {
				// Allow multiple wants with the same name (they may be different instances)
				cb.config.Wants = append(cb.config.Wants, want)
			}
			cb.reconcileMutex.Unlock()
			// Trigger reconciliation to connect and start new wants
			cb.reconcileWants()
		case wantIDs := <-cb.deleteWantsChan:
			// Delete wants asynchronously (non-blocking)
			deletedCount := 0
			for _, wantID := range wantIDs {
				if err := cb.DeleteWantByID(wantID); err != nil {
					// Failed to delete want (logging removed)
				} else {
					deletedCount++
				}
			}
			if deletedCount > 0 {
				// Wants deleted (logging removed) Trigger reconciliation to regenerate paths after want deletion
				cb.reconcileWants()
			}
		case op := <-cb.operationChan:
			// Process unified want/recipe/agent/capability/label operations
			if op != nil {
				cb.processWantOperation(op)
			}
		case <-ticker.C:
			// In batch mode, watch config file for changes In server mode, skip file watching (API is the source of truth)
			if !cb.isServerMode && cb.hasConfigFileChanged() {
				// Load config from original config file (not from memory file) Memory file is only for persistence, not for reloading configuration
				if newConfig, err := loadConfigFromYAML(cb.configPath); err == nil {
					cb.config = newConfig
					// Update the hash after loading
					cb.lastConfigFileHash, _ = cb.calculateFileHash(cb.configPath)
					cb.reconcileWants()
				}
			}
		case <-statsTicker.C:
			cb.writeStatsToMemory()
		}
	}
}

// reconcileWants performs reconciliation when config changes or during initial load Separated into explicit phases: compile -> connect -> start
func (cb *ChainBuilder) reconcileWants() {
	cb.reconcileMutex.Lock()
	defer cb.reconcileMutex.Unlock()
	cb.inReconciliation = true
	defer func() { cb.inReconciliation = false }()

	// Phase 1: COMPILE - Load and validate configuration
	if err := cb.compilePhase(); err != nil {
		return
	}

	// Phase 2: CONNECT - Establish want topology
	if err := cb.connectPhase(); err != nil {
		return
	}

	// Phase 3: START - Launch new/updated wants
	cb.startPhase()

	// Phase 4: ACCESS - Build structural state access index (Dictionary)
	cb.buildStateAccessIndex()

	// Phase 5: CORRELATE - Annotate changed Wants with inter-Want correlation
	cb.correlationPhase()
}

// compilePhase handles configuration loading and want creation/updates
func (cb *ChainBuilder) compilePhase() error {

	// Use current config as source of truth during runtime Memory file is only loaded on initial startup
	newConfig := cb.config
	isInitialLoad := !cb.hasInitialized

	if isInitialLoad {
		// For initial load, treat all wants as new additions
		for _, wantConfig := range newConfig.Wants {
			cb.addDynamicWantUnsafe(wantConfig)
		}

		// Run migration to clean up any agent_history from state
		cb.migrateAllWantsAgentHistory()

		// Dump memory after initial load (silent - routine operation)
		if len(newConfig.Wants) > 0 {
			cb.dumpWantMemoryToYAML()
		}

		// Mark initial load as complete
		cb.hasInitialized = true

		// All wants are new on initial load — mark all as dirty for correlationPhase.
		// cb.wants is keyed by name, so store names (not IDs) in dirtyWantIDs.
		for name := range cb.wants {
			cb.dirtyWantIDs[name] = struct{}{}
		}
	} else {
		// Detect changes for ongoing updates
		changes := cb.detectConfigChanges(cb.lastConfig, newConfig)

		// Apply changes if any
		if len(changes) > 0 {
			// Apply changes in reverse dependency order (sink to generator)
			cb.applyWantChanges(changes)

			// Mark changed (added/updated) wants as dirty for correlationPhase.
			// cb.wants is keyed by name; change.Want.Metadata.Name is the name.
			// Deleted wants are removed from cb.wants so correlationPhase ignores them.
			for _, change := range changes {
				if change.Type != ChangeEventDelete && change.Want != nil {
					cb.dirtyWantIDs[change.Want.Metadata.Name] = struct{}{}
				}
			}
		}

		// CRITICAL FIX: Ensure all wants in newConfig.Wants exist in cb.wants
		// This handles wants added via addWantsChan which are already in cb.config.Wants
		// but might have been missed by detectConfigChanges due to timing/aliasing
		for _, wantConfig := range newConfig.Wants {
			exists := false
			for _, rw := range cb.wants {
				if rw.want.Metadata.ID == wantConfig.Metadata.ID {
					exists = true
					break
				}
			}
			if !exists {
				cb.addWant(wantConfig)
				cb.dirtyWantIDs[wantConfig.Metadata.Name] = struct{}{}
			}
		}
	}

	// Update last config and hash IMPORTANT: Make a deep copy to avoid reference aliasing issues When both lastConfig and config point to the same Want objects, updates to one appear to update both, breaking change detection
	cb.lastConfig = cb.deepCopyConfig(newConfig)
	cb.lastConfigHash, _ = cb.calculateFileHash(cb.memoryPath)
	// Also track config file hash for batch mode watching
	cb.lastConfigFileHash, _ = cb.calculateFileHash(cb.configPath)

	return nil
}

// connectPhase handles want topology establishment and validation
func (cb *ChainBuilder) connectPhase() error {
	cb.processAutoConnections()
	cb.processTargets()

	// Generate new paths based on current wants
	cb.pathMap = cb.generatePathsFromConnections()

	// Synchronize generated paths to individual Want structs
	cb.synchronizePathsToWants()

	cb.buildLabelToUsersMapping()
	cb.validateConnections(cb.pathMap)

	return nil
}

// notifyParentOfChanges identifies added or removed owners and notifies them
func (cb *ChainBuilder) notifyParentOfChanges(want *Want, oldRefs, newRefs []OwnerReference) {
	// Map current owners for easy lookup
	oldOwners := make(map[string]bool)
	for _, ref := range oldRefs {
		if ref.Controller && ref.Kind == "Want" {
			oldOwners[ref.Name] = true
		}
	}

	newOwners := make(map[string]bool)
	for _, ref := range newRefs {
		if ref.Controller && ref.Kind == "Want" {
			newOwners[ref.Name] = true
		}
	}

	// 1. Notify removed parents (Disown)
	for name := range oldOwners {
		if !newOwners[name] {
			if parentRuntime, exists := cb.wants[name]; exists {
				if target, ok := parentRuntime.function.(*Target); ok {
					target.DisownChild(want.Metadata.ID)
				}
			}
		}
	}

	// 2. Notify added parents (Adopt)
	for name := range newOwners {
		if !oldOwners[name] {
			if parentRuntime, exists := cb.wants[name]; exists {
				if target, ok := parentRuntime.function.(*Target); ok {
					target.AdoptChild(want)
				}
			}
		}
	}
}

// notifyParentOfAdoption identifies the owner target of a want and triggers dynamic adoption (DEPRECATED: used by notifyParentOfChanges)
func (cb *ChainBuilder) notifyParentOfAdoption(want *Want) {
	cb.notifyParentOfChanges(want, nil, want.Metadata.OwnerReferences)
}

// processTargets processes all Target wants and builds their parameter subscriptions
func (cb *ChainBuilder) processTargets() {
	for _, runtimeWant := range cb.wants {
		if target, ok := runtimeWant.function.(*Target); ok {
			if target.RecipePath != "" && target.recipeLoader != nil {
				if err := cb.buildTargetParameterSubscriptions(target); err != nil {
					log.Printf("[ERROR] Failed to build target parameter subscriptions: %v\n", err)
				}
			}
		}
	}
}

// synchronizePathsToWants synchronizes generated paths to individual Want structs
// This ensures child wants can access their output channels when they execute
// NOTE: Caller (reconcileWants) already holds reconcileMutex, so we don't lock here
func (cb *ChainBuilder) synchronizePathsToWants() {
	for wantName, paths := range cb.pathMap {
		if runtimeWant, exists := cb.wants[wantName]; exists {
			// Update the want's paths field with the generated paths
			// This makes output/input channels available to the want during execution
			runtimeWant.want.paths.In = paths.In
			runtimeWant.want.paths.Out = paths.Out
		}
	}
}

func (cb *ChainBuilder) buildTargetParameterSubscriptions(target *Target) error {
	// Read and parse the recipe file
	recipeData, err := os.ReadFile(target.RecipePath)
	if err != nil {
		return fmt.Errorf("failed to read recipe file: %w", err)
	}

	var recipeDoc struct {
		Recipe struct {
			Parameters map[string]any `yaml:"parameters"`
			Wants      []struct {
				Metadata struct {
					Name string `yaml:"name"`
					Type string `yaml:"type"`
				} `yaml:"metadata"`
				Spec struct {
					Params map[string]any `yaml:"params"`
				} `yaml:"spec"`
			} `yaml:"wants"`
		} `yaml:"recipe"`
	}

	if err := yaml.Unmarshal(recipeData, &recipeDoc); err != nil {
		return fmt.Errorf("failed to parse recipe YAML: %w", err)
	}
	target.parameterSubscriptions = make(map[string][]string)
	targetID := target.Metadata.ID
	runtimeChildren := make(map[string]*Want) // Map: want type -> runtime want
	for childWantName, childRuntime := range cb.wants {
		for _, ownerRef := range childRuntime.want.Metadata.OwnerReferences {
			if ownerRef.ID == targetID {
				// This is a child of this target
				childType := childRuntime.want.Metadata.Type
				runtimeChildren[childType] = childRuntime.want
				runtimeChildren[childWantName] = childRuntime.want // Also store by name for lookup
				break
			}
		}
	}
	for _, recipeWant := range recipeDoc.Recipe.Wants {
		childType := recipeWant.Metadata.Type
		runtimeChild, exists := runtimeChildren[childType]
		if !exists {
			// Child not yet created, skip
			continue
		}

		actualChildName := runtimeChild.Metadata.Name

		// Iterate through child's params to find which parent params it uses
		for _, childParamValue := range recipeWant.Spec.Params {
			if paramRefStr, ok := childParamValue.(string); ok {
				// If the value matches a parent parameter name, it's a subscription
				if _, exists := target.RecipeParams[paramRefStr]; exists {
					target.parameterSubscriptions[paramRefStr] = append(
						target.parameterSubscriptions[paramRefStr],
						actualChildName,
					)
				}
			}
		}
	}

	return nil
}

// startPhase handles launching new/updated wants
func (cb *ChainBuilder) startPhase() {

	// Start new wants if system is running
	if cb.running {
		startedCount := 0

		// Debug: Log all idle wants found
		for wantName, want := range cb.wants {
			if want.want.GetStatus() == WantStatusIdle && strings.Contains(wantName, "date") {
				if DebugLoggingEnabled {
					log.Printf("[START-PHASE:DEBUG] Found Idle want: '%s'\n", wantName)
				}
			}
		}

		// First pass: start idle wants (only if connectivity requirements are met)
		for wantName, want := range cb.wants {
			if want.want.GetStatus() == WantStatusIdle {
				paths := cb.pathMap[wantName]
				meta := want.want.GetConnectivityMetadata()
				inCount := len(paths.In)
				outCount := len(paths.Out)

				// DEBUG: Log nested want startup
				if strings.Contains(wantName, "level 2 approval") || strings.Contains(wantName, "evidence") || strings.Contains(wantName, "description") {
					log.Printf("[RECONCILE:STARTUP] %s - inCount=%d (required=%d), outCount=%d (required=%d)\n",
						wantName, inCount, meta.RequiredInputs, outCount, meta.RequiredOutputs)
				}

				// Skip if required connections are not met
				if inCount < meta.RequiredInputs || outCount < meta.RequiredOutputs {
					// DEBUG: Log why nested wants are skipped
					if strings.Contains(wantName, "level 2 approval") || strings.Contains(wantName, "evidence") || strings.Contains(wantName, "description") {
						log.Printf("[RECONCILE:STARTUP] %s - SKIPPED (inCount < required or outCount < required)\n", wantName)
					}
					continue
				}

				if strings.Contains(wantName, "date") {
					if DebugLoggingEnabled {
						log.Printf("[START-PHASE:DEBUG] About to call startWant for '%s'\n", wantName)
					}
				}
				cb.startWant(wantName, want)
				startedCount++
			}
		}

		// NOTE: Second pass removed - no longer needed
		// All packet transmission goes through SendPacketMulti() which calls RetriggerReceiverWant()
		// RetriggerReceiverWant() uses tryRetriggerWithUnusedPackets(100) to detect packets within 100ms
		// This covers all normal retrigger scenarios - no need for periodic reconcile-based retrigger

		// Third pass: start any idle wants that now have required connections available This allows wants with inputs from just-completed upstream wants to execute
		additionalStarted := 0
		for wantName, want := range cb.wants {
			if want.want.GetStatus() == WantStatusIdle {
				paths := cb.pathMap[wantName]
				meta := want.want.GetConnectivityMetadata()
				inCount := len(paths.In)
				outCount := len(paths.Out)

				// Start if required connections are now available
				if inCount >= meta.RequiredInputs && outCount >= meta.RequiredOutputs {
					cb.startWant(wantName, want)
					additionalStarted++
				}
			}
		}

		// Initialize system scheduler if not already present
		cb.initializeSystemScheduler()
	} else {
	}
}

// detectConfigChanges compares configs and returns change events
func (cb *ChainBuilder) detectConfigChanges(oldConfig, newConfig Config) []ChangeEvent {
	var changes []ChangeEvent
	oldWants := make(map[string]*Want)
	for _, want := range oldConfig.Wants {
		oldWants[want.Metadata.ID] = want
	}

	newWants := make(map[string]*Want)
	for _, want := range newConfig.Wants {
		newWants[want.Metadata.ID] = want
	}
	for id, newWant := range newWants {
		if oldWant, exists := oldWants[id]; exists {
			if !cb.wantsEqual(oldWant, newWant) {
				changes = append(changes, ChangeEvent{
					Type:   ChangeEventUpdate,
					WantID: id,
					Want:   newWant,
				})
			}
		} else {
			// New want
			changes = append(changes, ChangeEvent{
				Type:   ChangeEventAdd,
				WantID: id,
				Want:   newWant,
			})
		}
	}
	for id := range oldWants {
		if _, exists := newWants[id]; !exists {
			changes = append(changes, ChangeEvent{
				Type:   ChangeEventDelete,
				WantID: id,
				Want:   nil,
			})
		}
	}

	return changes
}

// applyWantChanges applies want changes in sink-to-generator order
func (cb *ChainBuilder) applyWantChanges(changes []ChangeEvent) {
	sortedChanges := cb.sortChangesByDependency(changes)

	hasWantChanges := false
	for _, change := range sortedChanges {
		switch change.Type {
		case ChangeEventAdd:
			// Just add to runtime mapping, it's already in cb.config.Wants
			cb.addRuntimeWantOnly(change.Want)
			cb.notifyParentOfAdoption(change.Want)
			hasWantChanges = true
		case ChangeEventUpdate:
			// Sync fields from config to runtime
			// Find runtime want by ID
			var runtimeWant *runtimeWant
			for _, rw := range cb.wants {
				if rw.want.Metadata.ID == change.WantID {
					runtimeWant = rw
					break
				}
			}

			if runtimeWant != nil {
				// updatedConfigWant is the one from newConfig (cb.config)
				updatedConfigWant := change.Want
				if updatedConfigWant != nil {
					// Identify changed parents before syncing
					oldOwnerRefs := copyOwnerReferences(runtimeWant.want.Metadata.OwnerReferences)
					newOwnerRefs := updatedConfigWant.Metadata.OwnerReferences

					// Sync ALL fields
					runtimeWant.want.Spec = updatedConfigWant.Spec
					runtimeWant.want.metadataMutex.Lock()
					runtimeWant.want.Metadata = updatedConfigWant.Metadata
					runtimeWant.want.metadataMutex.Unlock()
					runtimeWant.want.State = updatedConfigWant.State

					// Re-initialize progressable linkage
					if progressable, ok := runtimeWant.function.(Progressable); ok {
						runtimeWant.want.SetProgressable(progressable)
					}

					// Notify parents of changes (adoption or disowning)
					cb.notifyParentOfChanges(updatedConfigWant, oldOwnerRefs, newOwnerRefs)

					// Clear config error state if present (allows recovery via config update)
					if runtimeWant.want.Status == WantStatusConfigError {
						runtimeWant.want.ClearConfigError()
					} else {
						// Reset status to Idle so want can be re-executed
						runtimeWant.want.RestartWant()
					}
				}
			}
			hasWantChanges = true
		case ChangeEventDelete:
			cb.deleteWantByID(change.WantID)
			hasWantChanges = true
		}
	}

	if hasWantChanges {
		cb.dumpWantMemoryToYAML()
	}
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
			levelI := depLevels[sortedChanges[i].WantID]
			levelJ := depLevels[sortedChanges[j].WantID]
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
	for _, rw := range cb.wants {
		id := rw.want.Metadata.ID
		if !visited[id] {
			cb.calculateDependencyLevel(id, levels, visited, make(map[string]bool))
		}
	}

	return levels
}

// calculateDependencyLevel recursively calculates dependency level for a want
func (cb *ChainBuilder) calculateDependencyLevel(wantID string, levels map[string]int, visited, inProgress map[string]bool) int {
	if inProgress[wantID] {
		return 0 // Break cycles by assigning level 0
	}
	if visited[wantID] {
		return levels[wantID]
	}

	inProgress[wantID] = true
	var wantConfig *Want
	// Find in runtime first
	for _, rw := range cb.wants {
		if rw.want.Metadata.ID == wantID {
			wantConfig = rw.want
			break
		}
	}

	// Then find in config if not in runtime
	if wantConfig == nil {
		for _, configWant := range cb.config.Wants {
			if configWant.Metadata.ID == wantID {
				wantConfig = configWant
				break
			}
		}
	}

	if wantConfig == nil {
		// Unknown want, assign level 0
		levels[wantID] = 0
		visited[wantID] = true
		delete(inProgress, wantID)
		return 0
	}

	maxDependencyLevel := 0
	for _, usingSelector := range wantConfig.Spec.Using {
		for _, configWant := range cb.config.Wants {
			if cb.matchesSelector(configWant.GetLabels(), usingSelector) {
				depLevel := cb.calculateDependencyLevel(configWant.Metadata.ID, levels, visited, inProgress)
				if depLevel >= maxDependencyLevel {
					maxDependencyLevel = depLevel + 1
				}
			}
		}
	}

	// Assign level based on dependencies
	levels[wantID] = maxDependencyLevel
	visited[wantID] = true
	delete(inProgress, wantID)

	return maxDependencyLevel
}
func (cb *ChainBuilder) addWant(wantConfig *Want) {
	// Check for duplicate name
	if existingWant, exists := cb.wants[wantConfig.Metadata.Name]; exists {
		// Duplicate name detected - reject the new want to protect existing one
		InfoLog("[WARN] Rejecting want '%s' (ID: %s): name already exists (existing ID: %s)\n",
			wantConfig.Metadata.Name, wantConfig.Metadata.ID, existingWant.want.Metadata.ID)
		return
	}

	// Apply want type definition defaults (including Requires) if available
	if cb.wantTypeDefinitions != nil {
		if typeDef, exists := cb.wantTypeDefinitions[wantConfig.Metadata.Type]; exists {
			// Apply Requires from want type definition if not already set in wantConfig
			if len(wantConfig.Spec.Requires) == 0 && len(typeDef.Requires) > 0 {
				wantConfig.Spec.Requires = typeDef.Requires
				DebugLog("[CHAIN-BUILDER] Applied requires from want type definition for '%s': %v\n",
					wantConfig.Metadata.Type, typeDef.Requires)
			}
			// Apply FinalResultField from want type definition if not already set
			if wantConfig.Spec.FinalResultField == "" && typeDef.FinalResultField != "" {
				wantConfig.Spec.FinalResultField = typeDef.FinalResultField
			}
		}
	}

	wantFunction, err := cb.createWantFunction(wantConfig)
	if err != nil {
		wantPtr := &Want{
			Metadata: wantConfig.Metadata,
			Spec:     wantConfig.Spec,
			Status:   WantStatusFailed,
			State: Dict{
				"error": err.Error(),
			},
			History: wantConfig.History,
		}
		wantPtr.InitializeSubscriptionSystem()

		runtimeWant := &runtimeWant{
			function: nil, // No function since creation failed
			want:     wantPtr,
		}
		cb.wants[wantConfig.Metadata.Name] = runtimeWant
		return
	}

	// Try to extract Want pointer from the want function (if it has one embedded)
	var wantPtr *Want

	// First try direct *Want pointer
	if w, ok := wantFunction.(*Want); ok {
		wantPtr = w
	} else {
		// Try to extract embedded Want via reflection
		wantPtr = extractWantViaReflection(wantFunction)
		if wantPtr != nil {
		}
	}

	// If no Want was found in the want function, create a new one (This handles want types that don't embed or return a Want)
	if wantPtr == nil {
		stateMap := make(map[string]any)
		if wantConfig.State != nil {
			// Use StoreStateMulti for proper state batching
			if len(wantConfig.State) > 0 {
				wantPtr.StoreStateMulti(wantConfig.State)
			}
		}

		// Copy History field from config
		historyField := wantConfig.History
		if len(historyField.ParameterHistory) == 0 && wantConfig.Spec.Params != nil {
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
	} else {
		// Update the extracted Want with metadata and config info
		// Preserve any labels added by CreateTargetFunc (e.g. recipe-based) that are not in wantConfig
		wantPtr.metadataMutex.Lock()
		preservedLabels := wantPtr.Metadata.Labels
		wantPtr.Metadata = wantConfig.Metadata
		if len(preservedLabels) > 0 {
			if wantPtr.Metadata.Labels == nil {
				wantPtr.Metadata.Labels = make(map[string]string)
			}
			for k, v := range preservedLabels {
				if _, exists := wantPtr.Metadata.Labels[k]; !exists {
					wantPtr.Metadata.Labels[k] = v
				}
			}
		}
		wantPtr.metadataMutex.Unlock()
		wantPtr.Spec = wantConfig.Spec
		wantPtr.SetStatus(WantStatusIdle)

		// Merge state data if provided
		if wantConfig.State != nil {
			// Use StoreStateMulti for proper state batching
			if len(wantConfig.State) > 0 {
				wantPtr.StoreStateMulti(wantConfig.State)
			}
		}

		// Update history
		wantPtr.History = wantConfig.History
		if len(wantPtr.History.ParameterHistory) == 0 && wantConfig.Spec.Params != nil {
			entry := StateHistoryEntry{
				WantName:   wantConfig.Metadata.Name,
				StateValue: wantConfig.Spec.Params,
				Timestamp:  time.Now(),
			}
			wantPtr.History.ParameterHistory = append(wantPtr.History.ParameterHistory, entry)
		}
	}
	wantPtr.InitializeSubscriptionSystem()

	// Inject agent registry if available
	if cb.agentRegistry != nil {
		wantPtr.SetAgentRegistry(cb.agentRegistry)
	}

	// CRITICAL FIX: Load ConnectivityMetadata from registry
	// This ensures require field from YAML is properly applied
	// Special handling for custom target types: they manage their own children and should have no connectivity requirements
	if cb.customRegistry != nil && cb.customRegistry.IsCustomType(wantConfig.Metadata.Type) {
		// Custom target types (prime sieve, level 1 approval, etc.) don't need connectivity requirements
		wantPtr.ConnectivityMetadata = ConnectivityMetadata{
			RequiredInputs:  0,
			MaxInputs:       -1,
			RequiredOutputs: 0,
			MaxOutputs:      -1,
			WantType:        wantConfig.Metadata.Type,
			Description:     "Custom target (manages child wants)",
		}
		log.Printf("[ADD-WANT] Want '%s' (type: %s): Custom target - ConnectivityMetadata RequiredInputs=0, RequiredOutputs=0",
			wantConfig.Metadata.Name, wantConfig.Metadata.Type)
	} else if meta, exists := cb.connectivityRegistry[wantConfig.Metadata.Type]; exists {
		wantPtr.ConnectivityMetadata = meta
		log.Printf("[ADD-WANT] Want '%s' (type: %s): Applied ConnectivityMetadata RequiredInputs=%d, RequiredOutputs=%d",
			wantConfig.Metadata.Name, wantConfig.Metadata.Type, meta.RequiredInputs, meta.RequiredOutputs)
	} else {
		// No connectivity metadata found - use defaults (no requirements)
		wantPtr.ConnectivityMetadata = ConnectivityMetadata{
			RequiredInputs:  0,
			MaxInputs:       -1,
			RequiredOutputs: 0,
			MaxOutputs:      -1,
			WantType:        wantConfig.Metadata.Type,
			Description:     "No connectivity requirements",
		}
	}

	runtimeWant := &runtimeWant{
		function: wantFunction,
		want:     wantPtr,
	}

	// Initial execution of required agents (Think, Monitor, Do)
	// This ensures background agents are started as soon as the want is created.
	if err := wantPtr.ExecuteAgents(); err != nil {
		wantPtr.StoreLog("ERROR: Failed to execute agents on add: %v", err)
	}

	// Safely add to wants map, avoiding accidental overwrites of existing wants with same name but different ID
	// Use unique name if necessary, but ideally we should transition cb.wants to use ID as key
	if existing, exists := cb.wants[wantConfig.Metadata.Name]; exists {
		if existing.want.Metadata.ID != wantConfig.Metadata.ID {
			// Name collision between different wants - use ID-suffixed name as temporary measure
			uniqueName := fmt.Sprintf("%s-%s", wantConfig.Metadata.Name, wantConfig.Metadata.ID)
			cb.wants[uniqueName] = runtimeWant
		} else {
			// Same want, safe to update
			cb.wants[wantConfig.Metadata.Name] = runtimeWant
		}
	} else {
		cb.wants[wantConfig.Metadata.Name] = runtimeWant
	}

	// Register want for notification system
	cb.registerWantForNotifications(wantConfig, wantFunction, wantPtr)

	// Automatically register labels in the global registry
	cb.registerLabelsFromWant(wantConfig)
}
func (cb *ChainBuilder) FindWantByID(wantID string) (*Want, string, bool) {
	cb.reconcileMutex.RLock()
	defer cb.reconcileMutex.RUnlock()

	// First search in runtime wants
	for wantName, runtimeWant := range cb.wants {
		if runtimeWant.want.Metadata.ID == wantID {
			return runtimeWant.want, wantName, true
		}
	}

	// If not found in runtime, search in config wants This handles newly created wants that haven't been promoted to runtime yet
	for _, configWant := range cb.config.Wants {
		if configWant.Metadata.ID == wantID {
			return configWant, configWant.Metadata.Name, true
		}
	}

	return nil, "", false
}

// FindWantByName searches for a want by its metadata name
func (cb *ChainBuilder) FindWantByName(wantName string) (*Want, bool) {
	cb.reconcileMutex.RLock()
	defer cb.reconcileMutex.RUnlock()

	// First search in runtime wants
	if runtimeWant, exists := cb.wants[wantName]; exists {
		return runtimeWant.want, true
	}

	// If not found in runtime, search in config wants
	for _, configWant := range cb.config.Wants {
		if configWant.Metadata.Name == wantName {
			return configWant, true
		}
	}

	return nil, false
}

// UpdateWant updates an existing want's configuration (params, labels, using fields, etc.) Automatically triggers reconciliation to process topology changes Works in both backend API and batch modes
func (cb *ChainBuilder) UpdateWant(wantConfig *Want) {
	cb.reconcileMutex.Lock()
	defer cb.reconcileMutex.Unlock()

	if DebugLoggingEnabled {
		log.Printf("[UPDATE-WANT] Updating want ID: %s (name: %s)\n", wantConfig.Metadata.ID, wantConfig.Metadata.Name)
	}

	// Search in config wants and update
	foundInConfig := false
	for i, cfgWant := range cb.config.Wants {
		if cfgWant.Metadata.ID == wantConfig.Metadata.ID {
			cb.config.Wants[i] = wantConfig
			foundInConfig = true
			break
		}
	}

	if !foundInConfig {
		if DebugLoggingEnabled {
			log.Printf("[UPDATE-WANT] Not found in config, adding as new dynamic want\n")
		}
		cb.addDynamicWantUnsafe(wantConfig)
		return
	}

	// Update label registry
	cb.registerLabelsFromWant(wantConfig)

	// Trigger immediate reconciliation via channel
	if !cb.inReconciliation {
		select {
		case cb.reconcileTrigger <- &TriggerCommand{Type: "reconcile"}:
			// Trigger sent successfully
		default:
			// Channel already has a pending trigger, skip
		}
	}
}

// deleteWantByID removes a want from runtime and signals its goroutines to stop.
// Cascade-deletes child wants that have an OwnerReference pointing to this want.
func (cb *ChainBuilder) deleteWantByID(wantID string) {
	var targetWantName string
	for name, rw := range cb.wants {
		if rw.want.Metadata.ID == wantID {
			targetWantName = name
			// Call OnDelete() if the want implements OnDeletable interface
			if deletable, ok := rw.function.(OnDeletable); ok {
				deletable.OnDelete()
			}

			if rw.want.stopChannel == nil {
				rw.want.stopChannel = make(chan struct{})
			}
			close(rw.want.stopChannel)
			break
		}
	}
	if targetWantName != "" {
		delete(cb.wants, targetWantName)
	}

	// Cascade: collect child want IDs owned by this want
	var childIDs []string
	for _, rw := range cb.wants {
		for _, ownerRef := range rw.want.Metadata.OwnerReferences {
			if ownerRef.ID == wantID && ownerRef.BlockOwnerDeletion {
				childIDs = append(childIDs, rw.want.Metadata.ID)
				break
			}
		}
	}
	for _, childID := range childIDs {
		cb.deleteWantByID(childID)
	}
}

// registerWantForNotifications registers a want with the notification system
func (cb *ChainBuilder) registerWantForNotifications(wantConfig *Want, wantFunction any, wantPtr *Want) {
	// Register want in global registry for lookup
	RegisterWant(wantPtr)
}

// startWant starts a single want
func (cb *ChainBuilder) startWant(wantName string, want *runtimeWant) {
	// Check if already in a terminal state
	status := want.want.GetStatus()
	if status == WantStatusAchieved || status == WantStatusFailed {
		log.Printf("[START-WANT] '%s' already in terminal state (%s), skipping\n", wantName, status)
		return
	}

	// Check if already running (reaching)
	if status == WantStatusReaching {
		return
	}

	// DEBUG: Log nested wants status
	if strings.Contains(wantName, "level 2 approval") || strings.Contains(wantName, "evidence") || strings.Contains(wantName, "description") {
		log.Printf("[START-WANT-DEBUG] %s: Checking connectivity requirements (RequiredInputs=%d, RequiredOutputs=%d)\n",
			wantName,
			want.want.GetConnectivityMetadata().RequiredInputs,
			want.want.GetConnectivityMetadata().RequiredOutputs)
	}

	// Check connectivity satisfaction
	if !cb.isConnectivitySatisfied(wantName, want, cb.pathMap) {
		if strings.Contains(wantName, "date") {
			if DebugLoggingEnabled {
				log.Printf("[START-WANT:SKIP] '%s' - connectivity not satisfied\n", wantName)
			}
		}
		want.want.RestartWant()
		return
	}

	// Check paths existence for wants with using selectors
	hasUsingSelectors := len(want.want.Spec.Using) > 0
	paths, pathsExist := cb.pathMap[wantName]
	if hasUsingSelectors && (!pathsExist || len(paths.In) == 0) {
		if strings.Contains(wantName, "date") {
			if DebugLoggingEnabled {
				log.Printf("[START-WANT:SKIP] '%s' - using selectors but no paths\n", wantName)
			}
		}
		return
	}

	// Double-check connectivity and trigger reconciliation if needed
	if !cb.isConnectivitySatisfied(wantName, want, cb.pathMap) {
		if strings.Contains(wantName, "date") {
			if DebugLoggingEnabled {
				log.Printf("[START-WANT:SKIP] '%s' - double-check connectivity failed\n", wantName)
			}
		}
		want.want.RestartWant()
		if err := cb.TriggerReconcile(); err != nil {
			// Log error but continue
		}
		return
	}

	// Ensure paths exist in pathMap
	// Thread-safe: Lock protects concurrent pathMap write from want goroutines reading via getActivePaths()
	if !pathsExist {
		cb.reconcileMutex.Lock()
		cb.pathMap[wantName] = Paths{In: []PathInfo{}, Out: []PathInfo{}}
		cb.reconcileMutex.Unlock()
	}

	// Start execution if want is Progressable
	if progressable, ok := want.function.(Progressable); ok {
		want.want.SetStatus(WantStatusReaching)
		if DebugLoggingEnabled {
			log.Printf("[START-WANT] '%s' transitioned to Reaching status\n", wantName)
		}
		want.want.InitializeControlChannel()

		// Set the concrete progressable implementation on the want
		want.want.SetProgressable(progressable)

		// Create closure that captures wantName and returns latest paths
		// This allows want to track topology changes during execution
		getPathsFunc := func() Paths {
			return cb.getActivePaths(wantName)
		}

		// Mark goroutine as active BEFORE starting it
		// Want owns the goroutine state via SetGoroutineActive()
		want.want.SetGoroutineActive(true)
		if DebugLoggingEnabled {
			log.Printf("[START-WANT] '%s' goroutine marked as active, starting progression loop\n", wantName)
		}

		// Manage goroutine lifecycle with waitGroup
		cb.waitGroup.Add(1)
		want.want.StartProgressionLoop(
			getPathsFunc,
			func() {
				// Mark goroutine as inactive when it finishes
				want.want.SetGoroutineActive(false)

				cb.waitGroup.Done() // Signal completion
			},
		)
	}
}

// getActivePaths returns the active input and output paths for a given want
func (cb *ChainBuilder) getActivePaths(wantName string) Paths {
	// Deadlock-resilient: Uses TryRLock to avoid hanging if called recursively during reconciliation
	if cb.reconcileMutex.TryRLock() {
		defer cb.reconcileMutex.RUnlock()
		return cb.getActivePathsLocked(wantName)
	}

	// If we couldn't get the RLock, we are likely in the middle of a reconciliation
	// that holds the Write Lock. In this case, we proceed WITHOUT locking to avoid deadlock.
	return cb.getActivePathsLocked(wantName)
}

// getActivePathsLocked returns paths assuming lock is held or safe
func (cb *ChainBuilder) getActivePathsLocked(wantName string) Paths {
	paths, pathsExist := cb.pathMap[wantName]
	if !pathsExist {
		return Paths{In: []PathInfo{}, Out: []PathInfo{}}
	}

	var activeInputPaths []PathInfo
	for _, inputPath := range paths.In {
		if inputPath.Active {
			activeInputPaths = append(activeInputPaths, inputPath)
		}
	}

	var activeOutputPaths []PathInfo
	for _, outputPath := range paths.Out {
		if outputPath.Active {
			activeOutputPaths = append(activeOutputPaths, outputPath)
		}
	}

	return Paths{
		In:  activeInputPaths,
		Out: activeOutputPaths,
	}
}

// Execute starts the reconcile loop and initial want execution For server mode, this runs indefinitely. For batch mode, it waits for completion.
func (cb *ChainBuilder) Execute() {
	cb.ExecuteWithMode(false) // Default: batch mode (waits for completion)
}

// ExecuteWithMode starts execution with specified mode serverMode=true: runs indefinitely for server mode serverMode=false: waits for wants to complete (batch mode)
func (cb *ChainBuilder) ExecuteWithMode(serverMode bool) {
	if cb.memoryPath != "" {
		if err := cb.copyConfigToMemory(); err == nil {
			cb.lastConfigHash, _ = cb.calculateFileHash(cb.memoryPath)
		}
	}
	cb.lastConfig = Config{Wants: []*Want{}}
	cb.reconcileMutex.Lock()
	cb.running = true
	cb.reconcileMutex.Unlock()

	// Start reconcile loop in background - it will handle initial want creation
	go cb.reconcileLoop()

	// Server mode: run indefinitely, never stop reconcile loop
	if serverMode {
		// Keep running forever - reconcile loop handles all want lifecycle
		select {} // Block forever
	}

	// Batch mode: wait for initial wants and completion Wait for initial wants to be created by reconcileLoop
	for {
		cb.reconcileMutex.Lock()
		wantCount := len(cb.wants)
		cb.reconcileMutex.Unlock()

		if wantCount > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Start all initial wants Create a snapshot of wants while holding lock to avoid concurrent map iteration
	cb.reconcileMutex.RLock()
	wantSnapshot := make(map[string]*runtimeWant, len(cb.wants))
	for wantName, want := range cb.wants {
		wantSnapshot[wantName] = want
	}
	cb.reconcileMutex.RUnlock()

	// Iterate over snapshot (no lock needed)
	for wantName, want := range wantSnapshot {
		cb.startWant(wantName, want)
	}

	// Wait for all wants to complete
	cb.waitGroup.Wait()

	// Stop reconcile loop
	cb.reconcileStop <- true
	cb.reconcileMutex.Lock()
	cb.running = false
	cb.reconcileMutex.Unlock()

	// Final memory dump - ensure it completes before returning (silent - routine operation)
	cb.dumpWantMemoryToYAML()
}

// Shutdown stops all wants and the reconcile loop
func (cb *ChainBuilder) Shutdown() {
	cb.reconcileMutex.Lock()
	defer cb.reconcileMutex.Unlock()

	log.Printf("[ChainBuilder] Shutting down: stopping %d wants\n", len(cb.wants))

	for _, rw := range cb.wants {
		// Call OnDelete() if the want implements OnDeletable interface
		if deletable, ok := rw.function.(OnDeletable); ok {
			deletable.OnDelete()
		}

		if rw.want.stopChannel == nil {
			rw.want.stopChannel = make(chan struct{})
		}

		// Use select with default to avoid panicking if already closed
		select {
		case <-rw.want.stopChannel:
			// already closed
		default:
			close(rw.want.stopChannel)
		}
	}

	// Stop reconcile loop
	select {
	case cb.reconcileStop <- true:
	default:
	}

	cb.running = false

	// Final memory dump - ensure it completes before returning (silent - routine operation)
	cb.dumpWantMemoryToYAML()
}

// addRuntimeWantOnly adds a want to the runtime mapping only (doesn't trigger reconcile)
func (cb *ChainBuilder) addRuntimeWantOnly(want *Want) {
	// Skip if want already exists in runtime to prevent double instantiation during config changes
	// Check by ID to be safe
	for _, rw := range cb.wants {
		if rw.want.Metadata.ID == want.Metadata.ID {
			return
		}
	}
	cb.addWant(want)
}

// addDynamicWantUnsafe adds a want to the builder and configuration without triggering reconciliation
func (cb *ChainBuilder) addDynamicWantUnsafe(want *Want) error {
	// Check by ID to be safe
	for _, rw := range cb.wants {
		if rw.want.Metadata.ID == want.Metadata.ID {
			return nil
		}
	}
	cb.config.Wants = append(cb.config.Wants, want)
	cb.addWant(want)

	// Trigger reconciliation to process the new want
	if !cb.inReconciliation {
		select {
		case cb.reconcileTrigger <- &TriggerCommand{Type: "reconcile"}:
		default:
			// Channel already has a pending trigger, skip
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

// extractWantFromProgressable extracts the embedded *Want from an Progressable type using reflection
// This handles concrete types like *RestaurantWant that embed Want, including nested embeddings
// (e.g., RestaurantWant -> BaseTravelWant -> Want)
func extractWantFromProgressable(progressable Progressable) (*Want, error) {
	val := reflect.ValueOf(progressable)
	if val.Kind() != reflect.Ptr {
		return nil, fmt.Errorf("progressable must be a pointer type")
	}

	// Get the struct value
	elem := val.Elem()
	if elem.Kind() != reflect.Struct {
		return nil, fmt.Errorf("progressable must point to a struct")
	}

	// Use VisibleFields to find embedded Want field, including nested embeddings
	visibleFields := reflect.VisibleFields(elem.Type())
	for _, fieldType := range visibleFields {
		// Check if this is an embedded Want field
		if fieldType.Anonymous && fieldType.Type.Name() == "Want" {
			// Get the field value by navigating through the struct hierarchy
			fieldVal := elem.FieldByIndex(fieldType.Index)

			// Found the embedded Want field
			if fieldVal.Kind() == reflect.Struct && fieldVal.CanAddr() {
				return fieldVal.Addr().Interface().(*Want), nil
			}
		}
	}

	return nil, fmt.Errorf("could not find embedded Want field in %T", progressable)
}

func SetGlobalChainBuilder(cb *ChainBuilder) {
	globalChainBuilder = cb
}
func GetGlobalChainBuilder() *ChainBuilder {
	return globalChainBuilder
}

// Queue methods for labels
