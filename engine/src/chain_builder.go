package mywant

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"fmt"
	"io"
	"log"
	"mywant/engine/src/chain"
	"mywant/engine/src/pubsub"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
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
	pathMap      map[string]Paths      // Want path mapping
	channels     map[string]chain.Chan // Inter-want channels
	channelMutex sync.RWMutex          // Protect channel access

	// Suspend/Resume control
	suspended    bool         // Current suspension state
	suspendChan  chan bool    // Channel to signal suspension
	resumeChan   chan bool    // Channel to signal resume
	controlMutex sync.RWMutex // Protect suspension state access
	controlStop  chan bool    // Stop signal for control loop

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

	// API logging
	apiLogs      []APILogEntry // API operation logs
	apiLogsMutex sync.RWMutex  // Protect concurrent access to logs
	maxLogSize   int           // Maximum number of log entries to keep (default: 1000)

	// HTTP client for internal API calls
	httpClient *HTTPClient // HTTP client for agents to call internal APIs

	// PubSub system for label-based packet delivery
	pubsub pubsub.PubSub // PubSub system for asynchronous packet delivery via labels

	// Managed PubSub adapter channels (topic + consumerID -> channel)
	pubsubChannels map[string]chain.Chan
	pubsubMutex    sync.RWMutex

	// Global Label Registry
	labelRegistry      map[string]map[string]bool // key -> value -> true
	labelRegistryMutex sync.RWMutex               // Protects labelRegistry
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
		channels:               make(map[string]chain.Chan),
		running:                false,
		warnedConnectionIssues: make(map[string]bool), // Track logged connectivity warnings
		labelToUsers:           make(map[string][]string),
		wantCompletedFlags:     make(map[string]bool),
		waitGroup:              &sync.WaitGroup{},
		suspended:              false,
		suspendChan:            make(chan bool),
		resumeChan:             make(chan bool),
		controlStop:            make(chan bool),
		apiLogs:                make([]APILogEntry, 0),
		maxLogSize:             1000,
		pubsubChannels:         make(map[string]chain.Chan),
		labelRegistry:          make(map[string]map[string]bool),
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

// RegisterWantType allows registering custom want types
func (cb *ChainBuilder) RegisterWantType(wantType string, factory WantFactory) {
	cb.registry[wantType] = factory
}

// RegisterWantTypeFromYAML registers a want type and loads connectivity metadata from YAML
func (cb *ChainBuilder) RegisterWantTypeFromYAML(wantType string, factory WantFactory, yamlDefPath string) error {
	// Load YAML definition
	def, err := LoadWantTypeDefinition(yamlDefPath)
	if err != nil {
		return fmt.Errorf("failed to load want type definition from %s: %w", yamlDefPath, err)
	}

	// Register the factory
	cb.registry[wantType] = factory

	// Store want type definition for later use during want creation
	if cb.wantTypeDefinitions == nil {
		cb.wantTypeDefinitions = make(map[string]*WantTypeDefinition)
	}
	cb.wantTypeDefinitions[wantType] = def

	// Store connectivity metadata for later use during want creation
	if cb.connectivityRegistry == nil {
		cb.connectivityRegistry = make(map[string]ConnectivityMetadata)
	}

	// Use connect field if available, then require, otherwise fall back to usageLimit
	if def.Connect != nil {
		// Convert RequireSpec to ConnectivityMetadata
		cb.connectivityRegistry[wantType] = def.Connect.ToConnectivityMetadata(wantType)
	} else if def.Require != nil {
		// Convert RequireSpec to ConnectivityMetadata
		cb.connectivityRegistry[wantType] = def.Require.ToConnectivityMetadata(wantType)
	} else if def.UsageLimit != nil {
		// Legacy support for UsageLimit
		cb.connectivityRegistry[wantType] = def.UsageLimit.ToConnectivityMetadata(wantType)
	}

	return nil
}

// StoreWantTypeDefinition stores a want type definition without registering a factory
// Used when definitions are already registered separately and we just need to make them available for state initialization
// Also registers aliases for special naming patterns (e.g., "queue" -> "qnet queue")
func (cb *ChainBuilder) StoreWantTypeDefinition(def *WantTypeDefinition) {
	if def == nil {
		return
	}

	// Store want type definition for later use during want creation
	if cb.wantTypeDefinitions == nil {
		cb.wantTypeDefinitions = make(map[string]*WantTypeDefinition)
	}

	wantType := def.Metadata.Name
	cb.wantTypeDefinitions[wantType] = def

	// Automatically register factory if a Go implementation exists in the registry
	if _, ok := typeImplementationRegistry[wantType]; ok {
		if _, alreadyRegistered := cb.registry[wantType]; !alreadyRegistered {
			cb.RegisterWantType(wantType, createGenericFactory(wantType))
		}
	}

	// Store connectivity metadata for later use during want creation
	if cb.connectivityRegistry == nil {
		cb.connectivityRegistry = make(map[string]ConnectivityMetadata)
	}

	// Use connect field if available, then require, otherwise fall back to usageLimit
	metadata := func() ConnectivityMetadata {
		if def.Connect != nil {
			// Convert RequireSpec to ConnectivityMetadata
			return def.Connect.ToConnectivityMetadata(wantType)
		} else if def.Require != nil {
			// Convert RequireSpec to ConnectivityMetadata
			return def.Require.ToConnectivityMetadata(wantType)
		} else if def.UsageLimit != nil {
			// Legacy support for UsageLimit
			return def.UsageLimit.ToConnectivityMetadata(wantType)
		}
		return ConnectivityMetadata{}
	}()
	cb.connectivityRegistry[wantType] = metadata

	// Register aliases for special naming patterns
	// This handles naming mismatches between YAML definitions and code registrations
	var aliases []string
	switch wantType {
	case "queue":
		// Queue type can be referenced as "qnet queue" in code
		aliases = []string{"qnet queue"}
	case "combiner":
		// Combiner type can be referenced as "qnet combiner" in code
		aliases = []string{"qnet combiner"}
	case "numbers":
		// Numbers type can be referenced as "qnet numbers" in code
		aliases = []string{"qnet numbers"}
	}

	// Store definitions and metadata under all aliases
	for _, alias := range aliases {
		cb.wantTypeDefinitions[alias] = def
		cb.connectivityRegistry[alias] = metadata
		
		// Also register factory for alias if it exists
		if _, ok := typeImplementationRegistry[wantType]; ok {
			if _, alreadyRegistered := cb.registry[alias]; !alreadyRegistered {
				cb.RegisterWantType(alias, createGenericFactory(wantType))
			}
		}
	}
}

func (cb *ChainBuilder) SetAgentRegistry(registry *AgentRegistry) {
	cb.agentRegistry = registry
}

func (cb *ChainBuilder) GetAgentRegistry() *AgentRegistry {
	return cb.agentRegistry
}

func (cb *ChainBuilder) SetCustomTargetRegistry(registry *CustomTargetTypeRegistry) {
	cb.customRegistry = registry
}
func (cb *ChainBuilder) SetConfigInternal(config Config) {
	cb.config = config
}
func (cb *ChainBuilder) SetServerMode(isServer bool) {
	cb.isServerMode = isServer
}

// SetHTTPClient sets the HTTP client for internal API calls
func (cb *ChainBuilder) SetHTTPClient(client *HTTPClient) {
	cb.httpClient = client
}

// GetHTTPClient returns the HTTP client for internal API calls
func (cb *ChainBuilder) GetHTTPClient() *HTTPClient {
	return cb.httpClient
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

					cb.pubsubMutex.RLock()
					adaptedChan, exists := cb.pubsubChannels[adapterKey]
					isSubscribed := cb.pubsub.IsSubscribed(topic, wantName)
					cb.pubsubMutex.RUnlock()

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
						cb.pubsubMutex.Lock()
						cb.pubsubChannels[adapterKey] = adaptedChan
						cb.pubsubMutex.Unlock()
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
func (cb *ChainBuilder) createWantFunction(want *Want) (any, error) {
	wantType := want.Metadata.Type

	// Check if it's a custom type first
	if cb.customRegistry != nil && cb.customRegistry.IsCustomType(wantType) {
		return cb.createCustomTargetWant(want)
	}

	// Fall back to standard type registration
	factory, exists := cb.registry[wantType]
	if !exists {
		// List available types for better error message
		availableTypes := make([]string, 0, len(cb.registry))
		for typeName := range cb.registry {
			availableTypes = append(availableTypes, typeName)
		}
		customTypes := make([]string, 0)
		if cb.customRegistry != nil {
			customTypes = cb.customRegistry.ListTypes()
		}

		return nil, fmt.Errorf("Unknown want type: '%s'. Available standard types: %v. Available custom types: %v",
			wantType, availableTypes, customTypes)
	}

	factoryResult := factory(want.Metadata, want.Spec)

	// Extract *Want from the Progressable result via reflection
	// All factories now return Progressable implementations that embed Want
	var wantPtr *Want
	if w, err := extractWantFromProgressable(factoryResult); err == nil {
		wantPtr = w
	} else {
		return nil, fmt.Errorf("factory returned Progressable but could not extract Want: %v", err)
	}

	// Automatically set want type definition if available
	// This initializes ProvidedStateFields and sets initial state values
	if wantPtr != nil && cb.wantTypeDefinitions != nil {
		if typeDef, exists := cb.wantTypeDefinitions[wantType]; exists {
			wantPtr.SetWantTypeDefinition(typeDef)
		}
	}

	if cb.agentRegistry != nil && wantPtr != nil {
		wantPtr.SetAgentRegistry(cb.agentRegistry)
	}

	// Automatically wrap with OwnerAwareWant if the want has owner references This enables parent-child coordination via subscription events
	if len(want.Metadata.OwnerReferences) > 0 {
		return NewOwnerAwareWant(factoryResult, want.Metadata, wantPtr), nil
	}

	return factoryResult, nil
}

// TestCreateWantFunction tests want type creation without side effects (exported for validation)
func (cb *ChainBuilder) TestCreateWantFunction(want *Want) (any, error) {
	return cb.createWantFunction(want)
}
func (cb *ChainBuilder) createCustomTargetWant(want *Want) (any, error) {
	config, exists := cb.customRegistry.Get(want.Metadata.Type)
	if !exists {
		availableTypes := cb.customRegistry.ListTypes()
		return nil, fmt.Errorf("custom type '%s' not found in registry. Available: %v", want.Metadata.Type, availableTypes)
	}

	// Merge custom type defaults with user-provided spec
	mergedSpec := cb.mergeWithCustomDefaults(want.Spec, config)
	target := config.CreateTargetFunc(want.Metadata, mergedSpec)
	target.SetBuilder(cb)
	recipeLoader := NewGenericRecipeLoader("recipes")
	target.SetRecipeLoader(recipeLoader)

	// Set the custom_target want type definition to enable state fields
	// Custom targets are based on the custom_target want type, so we set that definition
	if cb.wantTypeDefinitions != nil {
		if typeDef, exists := cb.wantTypeDefinitions["custom_target"]; exists {
			target.Want.SetWantTypeDefinition(typeDef)
		}
	}

	// Automatically wrap with OwnerAwareWant if the custom target has owner references This enables parent-child coordination via subscription events (critical for nested targets)
	var wantInstance any = target
	if len(want.Metadata.OwnerReferences) > 0 {
		wantInstance = NewOwnerAwareWant(wantInstance, want.Metadata, &target.Want)
	}

	return wantInstance, nil
}

// mergeWithCustomDefaults merges user spec with custom type defaults
func (cb *ChainBuilder) mergeWithCustomDefaults(spec WantSpec, config CustomTargetTypeConfig) WantSpec {
	merged := spec
	if merged.Params == nil {
		merged.Params = make(map[string]any)
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

// hasConfigFileChanged checks if config file has changed Used in batch mode to detect updates to the original config file
func (cb *ChainBuilder) hasConfigFileChanged() bool {
	if cb.configPath == "" {
		return false
	}

	currentHash, err := cb.calculateFileHash(cb.configPath)
	if err != nil {
		return false
	}

	// Use a separate hash field for config file to avoid collision with memory file hash
	return currentHash != cb.lastConfigFileHash
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
	} else {
		// Detect changes for ongoing updates
		changes := cb.detectConfigChanges(cb.lastConfig, newConfig)

		// Apply changes if any
		if len(changes) > 0 {
			// Apply changes in reverse dependency order (sink to generator)
			cb.applyWantChanges(changes)
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

	// Store constructed channels for potential external access
	cb.storeChannelsByPath()

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

// storeChannelsByPath stores constructed channels for potential external access
// The channels themselves were created in generatePathsFromConnections() and should NOT be recreated here
func (cb *ChainBuilder) storeChannelsByPath() {
	cb.channelMutex.Lock()
	defer cb.channelMutex.Unlock()

	cb.channels = make(map[string]chain.Chan)

	// Map channels by their path names (from pathMap, which contains the original channels)
	for _, paths := range cb.pathMap {
		for _, outputPath := range paths.Out {
			if outputPath.Active && outputPath.Channel != nil {
				cb.channels[outputPath.Name] = outputPath.Channel
			}
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
func (cb *ChainBuilder) processAutoConnections() {

	// Collect all wants with RecipeAgent enabled
	autoConnectWants := make([]*runtimeWant, 0)
	allWants := make([]*runtimeWant, 0)

	for _, runtimeWant := range cb.wants {
		allWants = append(allWants, runtimeWant)
		want := runtimeWant.want
		if cb.hasRecipeAgent(want) {
			autoConnectWants = append(autoConnectWants, runtimeWant)
		}
	}
	for _, runtimeWant := range autoConnectWants {
		want := runtimeWant.want
		cb.autoConnectWant(want, allWants)
		// Note: want object itself has been updated, no need to sync to separate spec copy
	}

}

// hasRecipeAgent checks if a want has RecipeAgent functionality enabled
func (cb *ChainBuilder) hasRecipeAgent(want *Want) bool {
	labels := want.GetLabels()
	if len(labels) > 0 {
		if role, ok := labels["role"]; ok && role == "coordinator" {
			return true
		}
	}
	if want.Metadata.Type == "level1_coordinator" || want.Metadata.Type == "level2_coordinator" {
		return true
	}

	return false
}

// autoConnectWant connects a RecipeAgent want to all compatible wants with matching approval_id
func (cb *ChainBuilder) autoConnectWant(want *Want, allWants []*runtimeWant) {
	approvalID := cb.extractApprovalID(want)
	if approvalID == "" {
		return
	}

	if want.Spec.Using == nil {
		want.Spec.Using = make([]map[string]string, 0)
	}

	for _, otherRuntimeWant := range allWants {
		cb.tryAutoConnectToWant(want, otherRuntimeWant.want)
	}
}

// extractApprovalID extracts approval_id from want params or labels
func (cb *ChainBuilder) extractApprovalID(want *Want) string {
	// Try params first
	if want.Spec.Params != nil {
		approvalID := ExtractMapString(want.Spec.Params, "approval_id")
		if approvalID != "" {
			return approvalID
		}
	}

	// Fall back to labels
	labels := want.GetLabels()
	if len(labels) > 0 {
		if approvalID, ok := labels["approval_id"]; ok && approvalID != "" {
			return approvalID
		}
	}

	return ""
}

// tryAutoConnectToWant attempts to auto-connect a want to another want
func (cb *ChainBuilder) tryAutoConnectToWant(want *Want, otherWant *Want) {
	// Skip self
	if otherWant.Metadata.Name == want.Metadata.Name {
		return
	}

	approvalID := cb.extractApprovalID(want)
	otherApprovalID := cb.extractApprovalID(otherWant)

	// Must have matching approval_id
	if approvalID == "" || otherApprovalID != approvalID {
		return
	}

	// Must be a data provider want
	if !cb.isDataProviderWant(otherWant) {
		return
	}

	cb.addAutoConnection(want, otherWant)
}

// isDataProviderWant checks if a want is a data provider (evidence or description)
func (cb *ChainBuilder) isDataProviderWant(want *Want) bool {
	labels := want.GetLabels()
	if len(labels) == 0 {
		return false
	}

	role := labels["role"]
	return role == "evidence-provider" || role == "description-provider"
}

// addAutoConnection adds an auto-connection from otherWant to want if not duplicate
func (cb *ChainBuilder) addAutoConnection(want *Want, otherWant *Want) {
	connectionKey := cb.generateConnectionKey(want)
	cb.addConnectionLabel(otherWant, want)

	selector := cb.buildConnectionSelector(want, otherWant, connectionKey)

	// Check for duplicate selector
	if cb.hasDuplicateSelector(want, selector) {
		return
	}

	want.Spec.Using = append(want.Spec.Using, selector)
}

// buildConnectionSelector builds a connection selector map
func (cb *ChainBuilder) buildConnectionSelector(want *Want, otherWant *Want, connectionKey string) map[string]string {
	selector := make(map[string]string)

	if connectionKey != "" {
		labelKey := fmt.Sprintf("used_by_%s", connectionKey)
		selector[labelKey] = want.Metadata.Name
	} else {
		// Fallback to role-based selector
		labels := otherWant.GetLabels()
		role := labels["role"]
		selector["role"] = role
	}

	return selector
}

// hasDuplicateSelector checks if a selector already exists in want's using list
func (cb *ChainBuilder) hasDuplicateSelector(want *Want, selector map[string]string) bool {
	for _, existingSelector := range want.Spec.Using {
		if cb.selectorsMatch(existingSelector, selector) {
			return true
		}
	}
	return false
}

// selectorsMatch checks if two selectors are equal
func (cb *ChainBuilder) selectorsMatch(selector1, selector2 map[string]string) bool {
	if len(selector1) != len(selector2) {
		return false
	}

	for k, v := range selector2 {
		if selector1[k] != v {
			return false
		}
	}

	return true
}
func (cb *ChainBuilder) addConnectionLabel(sourceWant *Want, consumerWant *Want) {
	sourceWant.metadataMutex.Lock()
	if sourceWant.Metadata.Labels == nil {
		sourceWant.Metadata.Labels = make(map[string]string)
	}

	// Generate unique connection label based on consumer want Extract meaningful identifier from consumer (e.g., level1, level2, etc.)
	connectionKey := cb.generateConnectionKey(consumerWant)

	if connectionKey != "" {
		labelKey := fmt.Sprintf("used_by_%s", connectionKey)
		sourceWant.Metadata.Labels[labelKey] = consumerWant.Metadata.Name
	}
	sourceWant.metadataMutex.Unlock()
}

// generateConnectionKey creates a unique key based on consumer want characteristics
func (cb *ChainBuilder) generateConnectionKey(consumerWant *Want) string {
	// Try to extract meaningful identifier from labels first
	labels := consumerWant.GetLabels()
	if len(labels) > 0 {
		if level, ok := labels["approval_level"]; ok {
			return fmt.Sprintf("level%s", level)
		}
		if component, ok := labels["component"]; ok {
			return component
		}
		if category, ok := labels["category"]; ok {
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

// wantsEqual compares two wants for equality
func (cb *ChainBuilder) wantsEqual(a, b *Want) bool {
	// Compare metadata
	if a.Metadata.Type != b.Metadata.Type {
		return false
	}

	if !mapsEqual(a.GetLabels(), b.GetLabels()) {
		return false
	}

	if !reflect.DeepEqual(a.Metadata.OwnerReferences, b.Metadata.OwnerReferences) {
		return false
	}

	// Compare spec
	if !reflect.DeepEqual(a.Spec.Params, b.Spec.Params) {
		return false
	}

	if !reflect.DeepEqual(a.Spec.Using, b.Spec.Using) {
		return false
	}

	if !reflect.DeepEqual(a.Spec.When, b.Spec.When) {
		return false
	}

	return true
}

// mapsEqual compares two string maps for equality
func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}

	for k, v := range a {
		if b[k] != v {
			return false
		}
	}

	return true
}

// deepCopyConfig creates a deep copy of a Config to prevent reference aliasing This is critical for change detection to work correctly
func (cb *ChainBuilder) deepCopyConfig(src Config) Config {
	// Copy the wants slice with new Want objects
	copiedWants := make([]*Want, 0, len(src.Wants))
	for _, want := range src.Wants {
		// Deep copy the want
		copiedWant := &Want{
			Metadata: Metadata{
				ID:              want.Metadata.ID,
				Name:            want.Metadata.Name,
				Type:            want.Metadata.Type,
				Labels:          want.GetLabels(),
				OwnerReferences: copyOwnerReferences(want.Metadata.OwnerReferences),
			},
			Spec: WantSpec{
				Params:              copyInterfaceMap(want.Spec.Params),
				Using:               copyUsing(want.Spec.Using),
				StateSubscriptions:  copyStateSubscriptions(want.Spec.StateSubscriptions),
				NotificationFilters: copyNotificationFilters(want.Spec.NotificationFilters),
				Requires:            copyStringSlice(want.Spec.Requires),
				When:                copyWhen(want.Spec.When),
			},
		}
		copiedWants = append(copiedWants, copiedWant)
	}

	return Config{Wants: copiedWants}
}

// Helper functions for deep copying
func copyWhen(src []WhenSpec) []WhenSpec {
	if src == nil {
		return nil
	}
	dst := make([]WhenSpec, len(src))
	copy(dst, src)
	return dst
}

func copyStringMap(src map[string]string) map[string]string {
	if src == nil {
		return nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func copyInterfaceMap(src map[string]any) map[string]any {
	if src == nil {
		return nil
	}
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func copyUsing(src []map[string]string) []map[string]string {
	if src == nil {
		return nil
	}
	dst := make([]map[string]string, 0, len(src))
	for _, selector := range src {
		copiedSelector := copyStringMap(selector)
		dst = append(dst, copiedSelector)
	}
	return dst
}

func copyStringSlice(src []string) []string {
	if src == nil {
		return nil
	}
	dst := make([]string, len(src))
	copy(dst, src)
	return dst
}

func copyOwnerReferences(src []OwnerReference) []OwnerReference {
	if src == nil {
		return nil
	}
	dst := make([]OwnerReference, len(src))
	copy(dst, src)
	return dst
}

func copyStateSubscriptions(src []StateSubscription) []StateSubscription {
	if src == nil {
		return nil
	}
	dst := make([]StateSubscription, 0, len(src))
	for _, sub := range src {
		copiedSub := StateSubscription{
			WantName:   sub.WantName,
			StateKeys:  copyStringSlice(sub.StateKeys),
			Conditions: copyStringSlice(sub.Conditions),
			BufferSize: sub.BufferSize,
		}
		dst = append(dst, copiedSub)
	}
	return dst
}

func copyNotificationFilters(src []NotificationFilter) []NotificationFilter {
	if src == nil {
		return nil
	}
	dst := make([]NotificationFilter, 0, len(src))
	for _, filter := range src {
		copiedFilter := NotificationFilter{
			SourcePattern: filter.SourcePattern,
			StateKeys:     copyStringSlice(filter.StateKeys),
			ValuePattern:  filter.ValuePattern,
		}
		dst = append(dst, copiedFilter)
	}
	return dst
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

					// Reset status to Idle so want can be re-executed
					runtimeWant.want.RestartWant()
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
		wantPtr.metadataMutex.Lock()
		wantPtr.Metadata = wantConfig.Metadata
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

// deleteWantByID removes a want from runtime and signals its goroutines to stop
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

// writeStatsToMemory writes current stats to memory file
func (cb *ChainBuilder) writeStatsToMemory() {
	if cb.memoryPath == "" {
		return
	}
	updatedConfig := Config{
		Wants: make([]*Want, 0),
	}

	// First, add all wants from config and update with current stats
	configWantMap := make(map[string]bool)
	for _, want := range cb.config.Wants {
		configWantMap[want.Metadata.Name] = true
		if runtimeWant, exists := cb.wants[want.Metadata.Name]; exists {
			// Update with runtime data including spec using
			want.Spec = *runtimeWant.want.GetSpec() // Preserve using from runtime spec
			// Stats field removed - data now in State
			want.Status = runtimeWant.want.Status

			// Lock state when copying to prevent concurrent map access
			runtimeWant.want.stateMutex.RLock()
			stateCopy := make(map[string]any)
			for k, v := range runtimeWant.want.State {
				stateCopy[k] = v
			}
			runtimeWant.want.stateMutex.RUnlock()

			want.State = stateCopy
			want.History = runtimeWant.want.History // Include history in stats writes
		}
		updatedConfig.Wants = append(updatedConfig.Wants, want)
	}

	// Then, add any runtime wants that might not be in config (e.g., dynamically created and completed)
	for wantName, runtimeWant := range cb.wants {
		if !configWantMap[wantName] {
			// This want exists in runtime but not in config - include it

			// Lock state when copying to prevent concurrent map access
			runtimeWant.want.stateMutex.RLock()
			stateCopy := make(map[string]any)
			for k, v := range runtimeWant.want.State {
				stateCopy[k] = v
			}
			runtimeWant.want.stateMutex.RUnlock()

			wantConfig := &Want{
				Metadata: runtimeWant.GetMetadata(),
				Spec:     *runtimeWant.GetSpec(),
				// Stats field removed - data now in State
				Status:  runtimeWant.want.Status,
				State:   stateCopy,
				History: runtimeWant.want.History, // Include history in stats writes
			}
			updatedConfig.Wants = append(updatedConfig.Wants, wantConfig)
		}
	}

	// Write updated config to memory file
	data, err := yaml.Marshal(updatedConfig)
	if err != nil {
		return
	}

	// Calculate hash of stats data for change detection
	statsHash := fmt.Sprintf("%x", md5.Sum(data))

	// Skip write if stats haven't changed
	if statsHash == cb.lastStatsHash {
		return
	}

	os.WriteFile(cb.memoryPath, data, 0644)

	// Update stats hash and config hash to prevent stats updates from triggering reconciliation
	cb.lastStatsHash = statsHash
	cb.lastConfigHash, _ = cb.calculateFileHash(cb.memoryPath)
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

	// Start suspension control loop
	cb.startControlLoop()

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

	// Stop suspension control loop
	cb.controlStop <- true
	cb.reconcileMutex.Lock()
	cb.running = false
	cb.reconcileMutex.Unlock()

	// Final memory dump - ensure it completes before returning (silent - routine operation)
	cb.dumpWantMemoryToYAML()
}

// GetAllWantStates returns a map of all current want objects across all executions
func (cb *ChainBuilder) GetAllWantStates() map[string]*Want {
	// Deadlock-resilient: Uses TryRLock to avoid hanging if called recursively during reconciliation
	if cb.reconcileMutex.TryRLock() {
		defer cb.reconcileMutex.RUnlock()

		states := make(map[string]*Want)
		for name, want := range cb.wants {
			states[name] = want.want
		}
		return states
	}

	// If we can't get the lock, we are likely in the middle of a reconciliation
	// that holds the write lock. In this case, we return a snapshot of the current
	// wants without locking. This is safe because we are on the same thread/context
	// or the data is being updated atomically.
	states := make(map[string]*Want)
	for name, want := range cb.wants {
		states[name] = want.want
	}
	return states
}

// AddWantsAsync adds wants to the execution queue asynchronously
func (cb *ChainBuilder) AddWantsAsync(wants []*Want) error {
	if len(wants) == 0 {
		return nil
	}
	select {
	case cb.addWantsChan <- wants:
		return nil
	default:
		return fmt.Errorf("failed to send wants to reconcile loop (channel full)")
	}
}
func (cb *ChainBuilder) AddWantsAsyncWithTracking(wants []*Want) ([]string, error) {
	// Extract IDs from wants
	ids := make([]string, len(wants))
	for i, want := range wants {
		if want.Metadata.ID == "" {
			return nil, fmt.Errorf("want %s has no ID for tracking", want.Metadata.Name)
		}
		ids[i] = want.Metadata.ID
	}

	// Pre-check: Verify no duplicate names in existing wants
	cb.reconcileMutex.RLock()
	for _, newWant := range wants {
		for _, rw := range cb.wants {
			if rw.want.Metadata.Name == newWant.Metadata.Name {
				cb.reconcileMutex.RUnlock()
				return nil, fmt.Errorf("want with name '%s' already exists", newWant.Metadata.Name)
			}
		}
	}
	cb.reconcileMutex.RUnlock()

	if err := cb.AddWantsAsync(wants); err != nil {
		return nil, err
	}

	return ids, nil
}

// AreWantsAdded checks if all wants with the given IDs have been added to the runtime Returns true only if ALL wants are present in the runtime
func (cb *ChainBuilder) AreWantsAdded(wantIDs []string) bool {
	if len(wantIDs) == 0 {
		return true
	}

	cb.reconcileMutex.RLock()
	defer cb.reconcileMutex.RUnlock()
	for _, id := range wantIDs {
		found := false
		for _, rw := range cb.wants {
			if rw.want.Metadata.ID == id {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}

// DeleteWantsAsync sends want IDs to be deleted asynchronously through the reconcile loop This is the preferred method for deleting wants to avoid race conditions
func (cb *ChainBuilder) DeleteWantsAsync(wantIDs []string) error {
	select {
	case cb.deleteWantsChan <- wantIDs:
		return nil
	default:
		return fmt.Errorf("failed to send wants to delete through reconcile loop (channel full)")
	}
}

// DeleteWantsAsyncWithTracking sends want IDs to be deleted asynchronously and returns them for tracking The caller can use the returned IDs to poll with AreWantsDeleted() to confirm deletion
func (cb *ChainBuilder) DeleteWantsAsyncWithTracking(wantIDs []string) ([]string, error) {
	if err := cb.DeleteWantsAsync(wantIDs); err != nil {
		return nil, err
	}
	return wantIDs, nil
}

// AreWantsDeleted checks if all wants with the given IDs have been removed from the runtime Returns true only if ALL wants are no longer present in the runtime
func (cb *ChainBuilder) AreWantsDeleted(wantIDs []string) bool {
	if len(wantIDs) == 0 {
		return true
	}

	cb.reconcileMutex.RLock()
	defer cb.reconcileMutex.RUnlock()
	for _, id := range wantIDs {
		for _, rw := range cb.wants {
			if rw.want.Metadata.ID == id {
				// Found the want, so it's not deleted yet
				return false
			}
		}
	}

	// All wants are gone
	return true
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

	InfoLog("[CONFIG-YAML] 📖 Loading config from: %s\n", filename)

	// Read the YAML config file
	data, err := os.ReadFile(filename)
	if err != nil {
		return config, fmt.Errorf("failed to read YAML file: %w", err)
	}
	err = validateConfigWithSpec(data)
	if err != nil {
		return config, fmt.Errorf("config validation failed: %w", err)
	}
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return config, fmt.Errorf("failed to parse YAML config: %w", err)
	}

	InfoLog("[CONFIG-YAML] ✅ Loaded %d wants from config\n", len(config.Wants))
	for i, want := range config.Wants {
		recipe := ""
		if want.Spec.Recipe != "" {
			recipe = fmt.Sprintf(", recipe=%s", want.Spec.Recipe)
		}
		InfoLog("[CONFIG-YAML]   [%d] %s (type=%s%s)\n", i, want.Metadata.Name, want.Metadata.Type, recipe)
	}

	// Assign individual IDs to each want if not already set
	assignWantIDs(&config)

	return config, nil
}

// generateUUID generates a UUID v4 for want IDs
func generateUUID() string {
	uuid := make([]byte, 16)
	rand.Read(uuid)
	uuid[6] = (uuid[6] & 0x0f) | 0x40 // Version 4
	uuid[8] = (uuid[8] & 0x3f) | 0x80 // Variant
	return fmt.Sprintf("want-%x-%x-%x-%x-%x",
		uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16])
}

// assignWantIDs assigns unique IDs to wants that don't have them
func assignWantIDs(config *Config) {
	for i := range config.Wants {
		if config.Wants[i].Metadata.ID == "" {
			config.Wants[i].Metadata.ID = generateUUID()
		}
	}
}

// loadConfigFromYAMLBytes loads configuration from YAML bytes with OpenAPI spec validation
func loadConfigFromYAMLBytes(data []byte) (Config, error) {
	var config Config
	err := validateConfigWithSpec(data)
	if err != nil {
		return config, fmt.Errorf("config validation failed: %w", err)
	}
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return config, fmt.Errorf("failed to parse YAML config: %w", err)
	}

	// Assign individual IDs to each want if not already set
	assignWantIDs(&config)

	return config, nil
}
func validateConfigWithSpec(yamlData []byte) error {
	// Load the OpenAPI spec - try multiple paths to handle different working directories
	loader := openapi3.NewLoader()

	specPaths := []string{
		filepath.Join(SpecDir, "want-spec.yaml"),
		filepath.Join("..", SpecDir, "want-spec.yaml"),
		filepath.Join("../..", SpecDir, "want-spec.yaml"),
		"../spec/want-spec.yaml",    // Legacy engine directory
		"spec/want-spec.yaml",       // Legacy project root
		"../../spec/want-spec.yaml", // Legacy deeper subdirectories
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
	ctx := context.Background()
	err = spec.Validate(ctx)
	if err != nil {
		return fmt.Errorf("OpenAPI spec is invalid: %w", err)
	}
	var yamlObj any
	err = yaml.Unmarshal(yamlData, &yamlObj)
	if err != nil {
		return fmt.Errorf("failed to parse YAML for validation: %w", err)
	}

	// jsonData conversion removed - not needed for basic validation
	configSchemaRef := spec.Components.Schemas["Config"]
	if configSchemaRef == nil {
		return fmt.Errorf("Config schema not found in OpenAPI spec")
	}

	// For now, do basic validation by checking that we can load and parse both spec and data A full OpenAPI->JSON Schema conversion would be more complex and is beyond current scope

	// Basic structural validation - ensure the YAML contains expected top-level keys
	var configObj map[string]any
	err = yaml.Unmarshal(yamlData, &configObj)
	if err != nil {
		return fmt.Errorf("invalid YAML structure: %w", err)
	}
	hasWants := false
	hasRecipe := false

	if wants, ok := configObj["wants"]; ok {
		if wantsArray, ok := wants.([]any); ok && len(wantsArray) > 0 {
			hasWants = true
		}
	}

	if recipe, ok := configObj["recipe"]; ok {
		if recipeObj, ok := recipe.(map[string]any); ok {
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

	return nil
}
func validateWantsStructure(wants any) error {
	wantsArray, ok := wants.([]any)
	if !ok {
		return fmt.Errorf("wants must be an array")
	}

	for i, want := range wantsArray {
		wantObj, ok := want.(map[string]any)
		if !ok {
			return fmt.Errorf("want at index %d must be an object", i)
		}
		metadata, ok := wantObj["metadata"]
		if !ok {
			return fmt.Errorf("want at index %d missing required 'metadata' field", i)
		}

		metadataObj, ok := metadata.(map[string]any)
		if !ok {
			return fmt.Errorf("want at index %d 'metadata' must be an object", i)
		}
		if name, ok := metadataObj["name"]; !ok || name == "" {
			return fmt.Errorf("want at index %d missing required 'metadata.name' field", i)
		}

		if wantType, ok := metadataObj["type"]; !ok || wantType == "" {
			return fmt.Errorf("want at index %d missing required 'metadata.type' field", i)
		}
		spec, ok := wantObj["spec"]
		if !ok {
			return fmt.Errorf("want at index %d missing required 'spec' field", i)
		}

		specObj, ok := spec.(map[string]any)
		if !ok {
			return fmt.Errorf("want at index %d 'spec' must be an object", i)
		}
		if params, ok := specObj["params"]; !ok {
			return fmt.Errorf("want at index %d missing required 'spec.params' field", i)
		} else {
			if _, ok := params.(map[string]any); !ok {
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
	timestamp := time.Now().Format("20060102-150405")

	// Use memory directory if available
	var filename string
	if cb.memoryPath != "" {
		memoryDir := filepath.Dir(cb.memoryPath)
		filename = filepath.Join(memoryDir, fmt.Sprintf("memory-%s.yaml", timestamp))
	} else {
		// Fallback to current directory with memory subdirectory Try to find the memory directory - check both "memory" and "../memory"
		memoryDir := "memory"
		if _, err := os.Stat(memoryDir); os.IsNotExist(err) {
			memoryDir = "../memory"
		}
		if err := os.MkdirAll(memoryDir, 0755); err != nil {
			return fmt.Errorf("failed to create memory directory: %w", err)
		}
		filename = filepath.Join(memoryDir, fmt.Sprintf("memory-%s.yaml", timestamp))
	}

	// Note: Caller must hold reconcileMutex lock for safe concurrent access Convert want map to slice to match config format, preserving runtime spec
	wants := make([]*Want, 0, len(cb.wants))
	for _, runtimeWant := range cb.wants {
		// Use GetAllState() which safely handles mutex locking internally
		stateCopy := runtimeWant.want.GetAllState()

		// Use runtime spec to preserve using, but want state for stats/status
		want := &Want{
			Metadata: runtimeWant.GetMetadata(),
			Spec:     *runtimeWant.GetSpec(), // This preserves using
			// Stats field removed - data now in State
			Status:  runtimeWant.want.Status,
			State:   stateCopy,                // Use copy to avoid concurrent modification
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
		// Use the same memoryDir logic as above
		memoryDir := "memory"
		if _, err := os.Stat(memoryDir); os.IsNotExist(err) {
			memoryDir = "../memory"
		}
		latestFilename = filepath.Join(memoryDir, "memory-0000-latest.yaml")
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

	return nil
}

// ============================== Suspend/Resume Control Methods ==============================

// SuspendWant suspends execution of a specific want and propagates to children
func (cb *ChainBuilder) SuspendWant(wantID string) error {
	cmd := &ControlCommand{
		Trigger:   ControlTriggerSuspend,
		WantID:    wantID,
		Timestamp: time.Now(),
		Reason:    "Suspended via API",
	}
	return cb.SendControlCommand(cmd)
}

// ResumeWant resumes execution of a specific want and propagates to children
func (cb *ChainBuilder) ResumeWant(wantID string) error {
	cmd := &ControlCommand{
		Trigger:   ControlTriggerResume,
		WantID:    wantID,
		Timestamp: time.Now(),
		Reason:    "Resumed via API",
	}
	return cb.SendControlCommand(cmd)
}

// StopWant stops execution of a specific want
func (cb *ChainBuilder) StopWant(wantID string) error {
	cmd := &ControlCommand{
		Trigger:   ControlTriggerStop,
		WantID:    wantID,
		Timestamp: time.Now(),
		Reason:    "Stopped via API",
	}
	return cb.SendControlCommand(cmd)
}

// RestartWant restarts execution of a specific want by setting its status to Idle
// This triggers the reconcile loop to re-run the want
func (cb *ChainBuilder) RestartWant(wantID string) error {
	// Find and restart the want by calling its RestartWant() method
	cb.reconcileMutex.RLock()
	var targetWant *Want
	for _, runtime := range cb.wants {
		if runtime.want.Metadata.ID == wantID {
			targetWant = runtime.want
			break
		}
	}
	cb.reconcileMutex.RUnlock()

	if targetWant == nil {
		return fmt.Errorf("want with ID %s not found", wantID)
	}

	// Call Want's RestartWant method which sets status to Idle
	targetWant.RestartWant()
	InfoLog("[RESTART:DEBUG] Want '%s' status now: %s\n", targetWant.Metadata.Name, targetWant.GetStatus())

	// Trigger reconciliation immediately to detect the Idle status and restart the goroutine
	select {
	case cb.reconcileTrigger <- &TriggerCommand{Type: "reconcile"}:
		InfoLog("[RESTART:DEBUG] Reconciliation trigger sent for '%s'\n", targetWant.Metadata.Name)
	default:
		InfoLog("[RESTART:DEBUG] Reconciliation trigger channel full for '%s'\n", targetWant.Metadata.Name)
	}

	return nil
}
func (cb *ChainBuilder) SendControlCommand(cmd *ControlCommand) error {
	select {
	case cb.reconcileTrigger <- &TriggerCommand{
		Type:           "control",
		ControlCommand: cmd,
	}:
		return nil
	default:
		return fmt.Errorf("failed to send control command - trigger channel full")
	}
}

// initializeSystemScheduler creates and starts the system scheduler Want
// This is called during startPhase to ensure the scheduler is always running
func (cb *ChainBuilder) initializeSystemScheduler() {
	// Check if scheduler already exists
	for _, want := range cb.wants {
		if want.want.Metadata.Type == "scheduler" {
			return // Scheduler already exists, nothing to do
		}
	}

	// Create a new Scheduler Want
	schedulerWant := &Want{
		Metadata: Metadata{
			ID:           generateUUID(),
			Name:         "system-scheduler",
			Type:         "scheduler",
			IsSystemWant: true, // Mark as system-managed want
			Labels: map[string]string{
				"system": "true",
				"role":   "scheduler",
			},
		},
		Spec: WantSpec{
			Params: Dict{
				"scan_interval": 60,
			},
		},
	}

	// Add the scheduler want asynchronously
	if err := cb.AddWantsAsync([]*Want{schedulerWant}); err != nil {
		InfoLog("[SYSTEM] Failed to initialize Scheduler Want: %v\n", err)
		return
	}

	InfoLog("[SYSTEM] System Scheduler Want initialized\n")
}

// Suspend pauses the execution of all wants (deprecated - use SuspendWant instead)
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
		return nil
	default:
		// Control loop not running, just mark as suspended
		return nil
	}
}

// Resume resumes the execution of all wants (deprecated - use ResumeWant instead)
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
		return nil
	default:
		// Control loop not running, just mark as resumed
		return nil
	}
}

// IsSuspended returns the current suspension state
func (cb *ChainBuilder) IsSuspended() bool {
	cb.controlMutex.RLock()
	defer cb.controlMutex.RUnlock()
	return cb.suspended
}

// distributeControlCommand distributes a control command to target want(s) and propagates to child wants if the target is a parent want
func (cb *ChainBuilder) distributeControlCommand(cmd *ControlCommand) {
	cb.reconcileMutex.RLock()
	defer cb.reconcileMutex.RUnlock()
	var targetRuntime *runtimeWant
	for _, runtime := range cb.wants {
		if runtime.want.Metadata.ID == cmd.WantID {
			targetRuntime = runtime
			break
		}
	}

	if targetRuntime == nil {
		return
	}
	if err := targetRuntime.want.SendControlCommand(cmd); err != nil {
	} else {
	}

	// TODO: Propagate control to child wants if this is a parent want This will require finding parent-child relationships in the Target want implementation
}

// Stop stops execution by clearing all wants from the configuration
func (cb *ChainBuilder) Stop() error {
	// Clear the config wants which will trigger reconciliation to clean up
	cb.reconcileMutex.Lock()
	cb.config.Wants = []*Want{}
	cb.reconcileMutex.Unlock()

	// Trigger reconciliation to process the empty config
	select {
	case cb.reconcileTrigger <- &TriggerCommand{Type: "reconcile"}:
	default:
	}

	return nil
}

// Start restarts execution by triggering reconciliation of existing configuration
func (cb *ChainBuilder) Start() error {
	// Trigger reconciliation - this will reload from memory and restart wants
	select {
	case cb.reconcileTrigger <- &TriggerCommand{Type: "reconcile"}:
		return nil
	default:
		return fmt.Errorf("failed to trigger reconciliation - channel full")
	}
}

// IsRunning returns whether the chain has any active wants
func (cb *ChainBuilder) IsRunning() bool {
	cb.reconcileMutex.RLock()
	defer cb.reconcileMutex.RUnlock()
	return len(cb.wants) > 0
}

// TriggerReconcile triggers the reconciliation loop to process current config
func (cb *ChainBuilder) TriggerReconcile() error {
	select {
	case cb.reconcileTrigger <- &TriggerCommand{Type: "reconcile"}:
		return nil
	default:
		return fmt.Errorf("failed to trigger reconciliation - channel full")
	}
}

// DeleteWantByID removes a want from runtime by its ID If the want has children (based on ownerReferences), they will be deleted first (cascade deletion)
func (cb *ChainBuilder) DeleteWantByID(wantID string) error {
	// Phase 1: Identify if parent want exists
	cb.reconcileMutex.RLock()
	var found bool
	for _, rw := range cb.wants {
		if rw.want.Metadata.ID == wantID {
			found = true
			break
		}
	}
	cb.reconcileMutex.RUnlock()

	if !found {
		// Also check config if not in runtime
		cb.reconcileMutex.RLock()
		for _, cfgWant := range cb.config.Wants {
			if cfgWant.Metadata.ID == wantID {
				found = true
				break
			}
		}
		cb.reconcileMutex.RUnlock()
	}

	if !found {
		return fmt.Errorf("want with ID %s not found", wantID)
	}

	// Phase 2: Find all children first (cascade deletion) with read lock
	var childrenIDsToDelete []string

	cb.reconcileMutex.RLock()
	for _, runtimeWant := range cb.wants {
		if runtimeWant.want.Metadata.OwnerReferences != nil {
			for _, ownerRef := range runtimeWant.want.Metadata.OwnerReferences {
				if ownerRef.ID == wantID {
					childrenIDsToDelete = append(childrenIDsToDelete, runtimeWant.want.Metadata.ID)
					break
				}
			}
		}
	}
	cb.reconcileMutex.RUnlock()

	// Phase 3: Delete children first (with write lock for each deletion)
	for _, childID := range childrenIDsToDelete {
		cb.reconcileMutex.Lock()
		cb.deleteWantByID(childID)

		// Also remove child from config to keep it in sync with cb.wants
		for i, cfgWant := range cb.config.Wants {
			if cfgWant.Metadata.ID == childID {
				cb.config.Wants = append(cb.config.Wants[:i], cb.config.Wants[i+1:]...)
				break
			}
		}
		cb.reconcileMutex.Unlock()
	}

	// Phase 4: Delete the parent want (with write lock)
	cb.reconcileMutex.Lock()
	cb.deleteWantByID(wantID)

	// Also remove from config so detectConfigChanges sees the deletion
	for i, cfgWant := range cb.config.Wants {
		if cfgWant.Metadata.ID == wantID {
			cb.config.Wants = append(cb.config.Wants[:i], cb.config.Wants[i+1:]...)
			break
		}
	}

	cb.reconcileMutex.Unlock()

	return nil
}

// controlLoop handles suspend/resume signals in a separate goroutine
func (cb *ChainBuilder) controlLoop() {
	for {
		select {
		case <-cb.suspendChan:
			// Suspend signal processed by Suspend() method

		case <-cb.resumeChan:
			// Resume signal processed by Resume() method

		case <-cb.controlStop:
			return
		}
	}
}

// startControlLoop starts the suspension control loop if not already running
func (cb *ChainBuilder) startControlLoop() {
	go cb.controlLoop()
}

// selectorToKey converts a label selector map to a unique string key Used for label-to-users mapping in completed want detection Example: {role: "coordinator", stage: "final"} → "role:coordinator,stage:final"
func (cb *ChainBuilder) selectorToKey(selector map[string]string) string {
	if len(selector) == 0 {
		return ""
	}

	// Sort keys for consistent ordering
	keys := make([]string, 0, len(selector))
	for k := range selector {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s:%s", k, selector[k]))
	}
	return strings.Join(parts, ",")
}
func (cb *ChainBuilder) buildLabelToUsersMapping() {
	cb.labelToUsers = make(map[string][]string)

	for wantName, runtimeWant := range cb.wants {
		want := runtimeWant.want
		spec := want.GetSpec()
		if spec == nil || spec.Using == nil {
			continue
		}

		// For each "using" selector, record this want as a user
		for _, selector := range spec.Using {
			selectorKey := cb.selectorToKey(selector)
			if selectorKey != "" {
				cb.labelToUsers[selectorKey] = append(cb.labelToUsers[selectorKey], wantName)
			}
		}
	}
}

// RetriggerReceiverWant is called when a packet is provided to a receiver
// This is more reliable because it directly reflects execution state
func (cb *ChainBuilder) RetriggerReceiverWant(wantName string) {
	cb.reconcileMutex.RLock()
	runtimeWant, exists := cb.wants[wantName]
	cb.reconcileMutex.RUnlock()

	if !exists {
		InfoLog("[RETRIGGER-RECEIVER] WARNING: receiver want '%s' not found\n", wantName)
		return
	}

	want := runtimeWant.want

	// Use want's retrigger decision function to determine if retrigger is needed
	// This encapsulates the logic: check goroutine state and pending packets
	if want.ShouldRetrigger() {
		// Restart the want's execution (sets status to Idle)
		// The reconcile loop's startPhase() will detect the Idle status and restart the want
		// This avoids duplicate execution and keeps retrigger logic in one place
		want.RestartWant()

		if err := cb.TriggerReconcile(); err != nil {
			InfoLog("[RETRIGGER-RECEIVER] WARNING: failed to trigger reconcile for '%s': %v\n", wantName, err)
		}
	}
}
func (cb *ChainBuilder) checkAndRetriggerCompletedWants() {

	// Take snapshot of completed flags to avoid holding lock during notification
	cb.completedFlagsMutex.RLock()
	completedSnapshot := make(map[string]bool)
	for name, isCompleted := range cb.wantCompletedFlags {
		completedSnapshot[name] = isCompleted
	}
	cb.completedFlagsMutex.RUnlock()

	// Take snapshot of wants to avoid holding lock during SetStatus
	cb.reconcileMutex.RLock()
	wantSnapshot := make(map[string]*runtimeWant)
	for name, rw := range cb.wants {
		wantSnapshot[name] = rw
	}
	cb.reconcileMutex.RUnlock()
	anyWantRetriggered := false
	for wantID, isCompleted := range completedSnapshot {

		if isCompleted {
			InfoLog("[RETRIGGER:CHECK] Checking users for completed want ID '%s'\n", wantID)
			users := cb.findUsersOfCompletedWant(wantID)
			InfoLog("[RETRIGGER:CHECK] Found %d users for want ID '%s'\n", len(users), wantID)

			if len(users) > 0 {
				InfoLog("[RETRIGGER] Want ID '%s' completed, found %d users to retrigger\n", wantID, len(users))

				for _, userName := range users {
					// Restart dependent want so it can be re-executed This allows the want to pick up new data from the completed source
					if runtimeWant, ok := wantSnapshot[userName]; ok {
						runtimeWant.want.RestartWant()
						anyWantRetriggered = true
					}
				}
			}
		}
	}

	// If any want was retriggered, queue a reconciliation trigger (cannot call reconcileWants() directly due to mutex re-entrancy)
	if anyWantRetriggered {
		select {
		case cb.reconcileTrigger <- &TriggerCommand{Type: "reconcile"}:
			// Trigger queued successfully
		default:
			// Channel full, ignore (next reconciliation cycle will handle it)
		}
	}
}
func (cb *ChainBuilder) findUsersOfCompletedWant(completedWantID string) []string {
	var runtimeWant *runtimeWant
	for _, rw := range cb.wants {
		if rw.want.Metadata.ID == completedWantID {
			runtimeWant = rw
			break
		}
	}

	if runtimeWant == nil {
		return []string{}
	}

	completedWant := runtimeWant.want
	labels := completedWant.GetLabels()
	if len(labels) == 0 {
		return []string{}
	}

	// For each label in the completed want, find users
	users := make(map[string]bool) // De-duplicate users

	// Generate selector keys from completed want's labels and look up users in the pre-computed mapping
	for labelKey, labelValue := range labels {
		selector := map[string]string{labelKey: labelValue}
		selectorKey := cb.selectorToKey(selector)
		if selectorKey != "" {
			if usersForSelector, exists := cb.labelToUsers[selectorKey]; exists {
				for _, userName := range usersForSelector {
					users[userName] = true
				}
			}
		}
	}
	userList := make([]string, 0, len(users))
	for userName := range users {
		userList = append(userList, userName)
	}
	return userList
}

// UpdateCompletedFlag updates the completed flag for a want based on its status Called from Want.SetStatus() to track which wants are completed Uses mutex to protect concurrent access MarkWantCompleted is the new preferred method for wants to notify the ChainBuilder of completion
// MarkWantCompleted marks a want as completed using want ID Called by receiver wants (e.g., Coordinators) when they reach completion state Replaces the previous pattern where senders would call UpdateCompletedFlag
func (cb *ChainBuilder) MarkWantCompleted(wantID string, status WantStatus) {
	cb.completedFlagsMutex.Lock()
	defer cb.completedFlagsMutex.Unlock()

	isCompleted := (status == WantStatusAchieved)
	cb.wantCompletedFlags[wantID] = isCompleted
	log.Printf("[WANT-COMPLETED] Want ID '%s' notified completion with status=%s\n", wantID, status)
}

// UpdateCompletedFlag updates completion flag using want ID Deprecated: Use MarkWantCompleted instead
func (cb *ChainBuilder) UpdateCompletedFlag(wantID string, status WantStatus) {
	cb.completedFlagsMutex.Lock()
	defer cb.completedFlagsMutex.Unlock()

	isCompleted := (status == WantStatusAchieved)
	cb.wantCompletedFlags[wantID] = isCompleted
	log.Printf("[UPDATE-COMPLETED-FLAG] Want ID '%s' status=%s, isCompleted=%v\n", wantID, status, isCompleted)
}

// IsCompleted returns whether a want is currently in completed state Safe to call from any goroutine with RLock protection
func (cb *ChainBuilder) IsCompleted(wantID string) bool {
	cb.completedFlagsMutex.RLock()
	defer cb.completedFlagsMutex.RUnlock()
	return cb.wantCompletedFlags[wantID]
}

// TriggerCompletedWantRetriggerCheck sends a non-blocking trigger to the reconcile loop to check for completed wants and notify their dependents Uses the unified reconcileTrigger channel with Type="check_completed_retrigger"
func (cb *ChainBuilder) TriggerCompletedWantRetriggerCheck() {
	select {
	case cb.reconcileTrigger <- &TriggerCommand{
		Type: "check_completed_retrigger",
	}:
		// Trigger sent successfully InfoLog("[RETRIGGER:SEND] Non-blocking retrigger check trigger sent to reconcile loop\n")
	default:
		// Channel is full (rare), trigger is already pending
		InfoLog("[RETRIGGER:SEND] Warning: reconcileTrigger channel full, skipping trigger\n")
	}
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

// LogAPIOperation logs an API operation (POST, PUT, DELETE, etc.)
func (cb *ChainBuilder) LogAPIOperation(method, endpoint, resource, status string, statusCode int, errorMsg, details string) {
	entry := APILogEntry{
		Timestamp:  time.Now(),
		Method:     method,
		Endpoint:   endpoint,
		Resource:   resource,
		Status:     status,
		StatusCode: statusCode,
		ErrorMsg:   errorMsg,
		Details:    details,
	}

	cb.apiLogsMutex.Lock()
	defer cb.apiLogsMutex.Unlock()

	cb.apiLogs = append(cb.apiLogs, entry)

	// Keep only the most recent maxLogSize entries
	if len(cb.apiLogs) > cb.maxLogSize {
		cb.apiLogs = cb.apiLogs[len(cb.apiLogs)-cb.maxLogSize:]
	}
}

// GetAPILogs returns a copy of all API logs
func (cb *ChainBuilder) GetAPILogs() []APILogEntry {
	cb.apiLogsMutex.RLock()
	defer cb.apiLogsMutex.RUnlock()

	// Return a copy to prevent external modification
	logs := make([]APILogEntry, len(cb.apiLogs))
	copy(logs, cb.apiLogs)
	return logs
}

// ClearAPILogs clears all API logs
func (cb *ChainBuilder) ClearAPILogs() {
	cb.apiLogsMutex.Lock()
	defer cb.apiLogsMutex.Unlock()
	cb.apiLogs = make([]APILogEntry, 0)
}

// QueueOperation queues an operation for processing by reconcile loop (non-blocking with default case)
// Returns error if channel is full
func (cb *ChainBuilder) QueueOperation(op *WantOperation) error {
	if op == nil {
		return fmt.Errorf("operation cannot be nil")
	}
	select {
	case cb.operationChan <- op:
		return nil
	default:
		return fmt.Errorf("operation queue full (buffer size: 20)")
	}
}

// QueueWantAdd queues a want addition operation
func (cb *ChainBuilder) QueueWantAdd(wants []*Want) error {
	if len(wants) == 0 {
		return fmt.Errorf("wants list cannot be empty")
	}
	return cb.QueueOperation(&WantOperation{
		Type:       "add",
		EntityType: "want",
		Wants:      wants,
	})
}

// QueueWantDelete queues a want deletion operation
func (cb *ChainBuilder) QueueWantDelete(ids []string) error {
	if len(ids) == 0 {
		return fmt.Errorf("want IDs cannot be empty")
	}
	return cb.QueueOperation(&WantOperation{
		Type:       "delete",
		EntityType: "want",
		IDs:        ids,
	})
}

// QueueWantSuspend queues a want suspension operation
func (cb *ChainBuilder) QueueWantSuspend(ids []string) error {
	if len(ids) == 0 {
		return fmt.Errorf("want IDs cannot be empty")
	}
	return cb.QueueOperation(&WantOperation{
		Type:       "suspend",
		EntityType: "want",
		IDs:        ids,
	})
}

// QueueWantResume queues a want resume operation
func (cb *ChainBuilder) QueueWantResume(ids []string) error {
	if len(ids) == 0 {
		return fmt.Errorf("want IDs cannot be empty")
	}
	return cb.QueueOperation(&WantOperation{
		Type:       "resume",
		EntityType: "want",
		IDs:        ids,
	})
}

// QueueWantStop queues a want stop operation
func (cb *ChainBuilder) QueueWantStop(ids []string) error {
	if len(ids) == 0 {
		return fmt.Errorf("want IDs cannot be empty")
	}
	return cb.QueueOperation(&WantOperation{
		Type:       "stop",
		EntityType: "want",
		IDs:        ids,
	})
}

// QueueWantStart queues a want start operation
func (cb *ChainBuilder) QueueWantStart(ids []string) error {
	if len(ids) == 0 {
		return fmt.Errorf("want IDs cannot be empty")
	}
	return cb.QueueOperation(&WantOperation{
		Type:       "start",
		EntityType: "want",
		IDs:        ids,
	})
}

// QueueWantAddLabel queues a label addition operation
func (cb *ChainBuilder) QueueWantAddLabel(wantID, key, value string) error {
	if wantID == "" || key == "" {
		return fmt.Errorf("want ID and label key cannot be empty")
	}
	cb.AddLabelToRegistry(key, value)
	return cb.QueueOperation(&WantOperation{
		Type:       "addLabel",
		EntityType: "want",
		IDs:        []string{wantID},
		Data: map[string]any{
			"key":   key,
			"value": value,
		},
	})
}

// QueueWantRemoveLabel queues a label removal operation
func (cb *ChainBuilder) QueueWantRemoveLabel(wantID, key string) error {
	if wantID == "" || key == "" {
		return fmt.Errorf("want ID and label key cannot be empty")
	}
	return cb.QueueOperation(&WantOperation{
		Type:       "removeLabel",
		EntityType: "want",
		IDs:        []string{wantID},
		Data: map[string]any{
			"key": key,
		},
	})
}

// processWantOperation processes a queued want operation (suspend, resume, stop, start, labels, etc.)
func (cb *ChainBuilder) processWantOperation(op *WantOperation) {
	if op == nil {
		return
	}

	// Helper function to send error back to callback channel non-blocking
	sendError := func(err error) {
		if op.Callback != nil {
			select {
			case op.Callback <- err:
			default:
				// Channel full or closed, silently drop (non-blocking)
			}
		}
	}

	switch op.Type {
	case "add":
		// Add new wants
		if len(op.Wants) > 0 {
			cb.reconcileMutex.Lock()
			for _, want := range op.Wants {
				cb.config.Wants = append(cb.config.Wants, want)
			}
			cb.reconcileMutex.Unlock()
			// Trigger reconciliation to connect and start new wants
			cb.reconcileWants()
		}

	case "delete":
		// Delete wants
		if len(op.IDs) > 0 {
			deletedCount := 0
			for _, wantID := range op.IDs {
				if err := cb.DeleteWantByID(wantID); err != nil {
					// Continue deleting others even if one fails
				} else {
					deletedCount++
				}
			}
			if deletedCount > 0 {
				// Trigger reconciliation after deletion
				cb.reconcileWants()
			}
		}

	case "suspend":
		// Suspend wants
		for _, wantID := range op.IDs {
			if err := cb.SuspendWant(wantID); err != nil {
				sendError(err)
				return
			}
		}

	case "resume":
		// Resume wants
		for _, wantID := range op.IDs {
			if err := cb.ResumeWant(wantID); err != nil {
				sendError(err)
				return
			}
		}

	case "stop":
		// Stop wants
		for _, wantID := range op.IDs {
			if err := cb.StopWant(wantID); err != nil {
				sendError(err)
				return
			}
		}

	case "start":
		// Start/restart wants
		for _, wantID := range op.IDs {
			if err := cb.RestartWant(wantID); err != nil {
				sendError(err)
				return
			}
		}

	case "addLabel":
		// Add label to want
		if len(op.IDs) > 0 && op.Data != nil {
			wantID := op.IDs[0]
			key, keyOk := op.Data["key"].(string)
			value, valueOk := op.Data["value"].(string)

			if !keyOk || !valueOk {
				sendError(fmt.Errorf("label key and value must be strings"))
				return
			}

			if want, _, found := cb.FindWantByID(wantID); found && want != nil {
				want.metadataMutex.Lock()
				if want.Metadata.Labels == nil {
					want.Metadata.Labels = make(map[string]string)
				}
				want.Metadata.Labels[key] = value
				want.metadataMutex.Unlock()
				want.Metadata.UpdatedAt = time.Now().Unix()
			} else {
				sendError(fmt.Errorf("want with ID %s not found", wantID))
				return
			}
		}

	case "removeLabel":
		// Remove label from want
		if len(op.IDs) > 0 && op.Data != nil {
			wantID := op.IDs[0]
			key, keyOk := op.Data["key"].(string)

			if !keyOk {
				sendError(fmt.Errorf("label key must be a string"))
				return
			}

			if want, _, found := cb.FindWantByID(wantID); found && want != nil {
				want.metadataMutex.Lock()
				if want.Metadata.Labels != nil {
					delete(want.Metadata.Labels, key)
				}
				want.metadataMutex.Unlock()
				want.Metadata.UpdatedAt = time.Now().Unix()
			} else {
				sendError(fmt.Errorf("want with ID %s not found", wantID))
				return
			}
		}

	default:
		sendError(fmt.Errorf("unknown operation type: %s", op.Type))
	}

	// Send success (nil error) to callback
	sendError(nil)
}

// AddLabelToRegistry explicitly registers a label in the global registry
func (cb *ChainBuilder) AddLabelToRegistry(key, value string) {
	cb.labelRegistryMutex.Lock()
	defer cb.labelRegistryMutex.Unlock()

	if cb.labelRegistry == nil {
		cb.labelRegistry = make(map[string]map[string]bool)
	}
	if cb.labelRegistry[key] == nil {
		cb.labelRegistry[key] = make(map[string]bool)
	}
	cb.labelRegistry[key][value] = true
}

// registerLabelsFromWant extracts all labels from a want and registers them
func (cb *ChainBuilder) registerLabelsFromWant(want *Want) {
	if want == nil {
		return
	}
	labels := want.GetLabels()
	if len(labels) == 0 {
		return
	}

	for k, v := range labels {
		cb.AddLabelToRegistry(k, v)
	}
}

// GetRegisteredLabels returns a snapshot of all registered labels (both manual and from wants)
func (cb *ChainBuilder) GetRegisteredLabels() (keys []string, values map[string][]string) {
	cb.labelRegistryMutex.RLock()
	defer cb.labelRegistryMutex.RUnlock()

	keys = make([]string, 0, len(cb.labelRegistry))
	values = make(map[string][]string)

	for k, vals := range cb.labelRegistry {
		keys = append(keys, k)
		vList := make([]string, 0, len(vals))
		for v := range vals {
			vList = append(vList, v)
		}
		sort.Strings(vList)
		values[k] = vList
	}
	sort.Strings(keys)

	return keys, values
}

// Queue methods for labels
