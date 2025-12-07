package mywant

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"fmt"
	"io"
	"log"
	"mywant/engine/src/chain"
	"os"
	"path/filepath"
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

	// GlobalExecutionInterval defines the sleep interval between goroutine execution cycles
	// This prevents CPU spinning in tight execution loops for want execution goroutines
	// Set to 100ms to reduce CPU usage during concurrent want execution
	GlobalExecutionInterval = 100 * time.Millisecond

	// GlobalReconcileInterval defines the frequency of reconcile loop operations
	// This controls how often the system checks for memory file changes and writes statistics
	GlobalReconcileInterval = 100 * time.Millisecond
)

// ChangeEvent represents a configuration change
type ChangeEvent struct {
	Type     ChangeEventType
	WantName string
	Want     *Want
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
	reconcileStop    chan bool              // Stop signal for reconcile loop
	reconcileTrigger chan *TriggerCommand   // Unified channel for reconciliation and control triggers
	addWantsChan     chan []*Want           // Buffered channel for asynchronous want addition requests
	deleteWantsChan  chan []string          // Buffered channel for asynchronous want deletion requests (want IDs)
	reconcileMutex   sync.RWMutex           // Protect concurrent access
	inReconciliation bool                   // Flag to prevent recursive reconciliation
	running            bool                   // Execution state
	lastConfig         Config                 // Last known config state
	lastConfigHash     string                 // Hash of last config for change detection
	lastConfigFileHash string                 // Hash of config file for change detection (batch mode)

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
}

// runtimeWant holds the runtime state of a want
type runtimeWant struct {
	chain    chain.C_chain
	function interface{}
	want     *Want
}

// GetSpec returns a pointer to the runtimeWant's Spec field
// by delegating to the underlying want's GetSpec() method
func (rw *runtimeWant) GetSpec() *WantSpec {
	if rw == nil || rw.want == nil {
		return nil
	}
	return rw.want.GetSpec()
}

// GetMetadata returns a pointer to the runtimeWant's Metadata field
// by delegating to the underlying want's GetMetadata() method
func (rw *runtimeWant) GetMetadata() *Metadata {
	if rw == nil || rw.want == nil {
		return nil
	}
	return rw.want.GetMetadata()
}

// NewChainBuilder creates a new builder from configuration
func NewChainBuilder(config Config) *ChainBuilder {
	builder := NewChainBuilderWithPaths("", "")
	builder.config = config
	return builder
}

// NewChainBuilderWithPaths creates a new builder with config and memory file paths
func NewChainBuilderWithPaths(configPath, memoryPath string) *ChainBuilder {
	builder := &ChainBuilder{
		configPath:       configPath,
		memoryPath:       memoryPath,
		wants:            make(map[string]*runtimeWant),
		registry:         make(map[string]WantFactory),
		customRegistry:   NewCustomTargetTypeRegistry(),
		reconcileStop:    make(chan bool),
		reconcileTrigger: make(chan *TriggerCommand, 20), // Unified channel for reconciliation and control triggers
		addWantsChan:     make(chan []*Want, 10), // Buffered to allow concurrent submissions
		deleteWantsChan:  make(chan []string, 10), // Buffered to allow concurrent deletion requests
		pathMap:          make(map[string]Paths),
		channels:         make(map[string]chain.Chan),
		running:          false,
		warnedConnectionIssues: make(map[string]bool), // Track logged connectivity warnings
		labelToUsers:       make(map[string][]string),
		wantCompletedFlags: make(map[string]bool),
		waitGroup:        &sync.WaitGroup{},
		// Initialize suspend/resume control
		suspended:   false,
		suspendChan: make(chan bool),
		resumeChan:  make(chan bool),
		controlStop: make(chan bool),
	}

	// Note: Recipe scanning is done at server startup (main.go) via ScanAndRegisterCustomTypes()
	// This avoids duplicate scanning logs when multiple ChainBuilder instances are created
	// Recipe registry is passed via the environment during server initialization

	// Auto-register owner want types for target system support
	RegisterOwnerWantTypes(builder)

	return builder
}

// RegisterWantType allows registering custom want types
func (cb *ChainBuilder) RegisterWantType(wantType string, factory WantFactory) {
	cb.registry[wantType] = factory
}

// SetAgentRegistry sets the agent registry for the chain builder
func (cb *ChainBuilder) SetAgentRegistry(registry *AgentRegistry) {
	cb.agentRegistry = registry
}

// SetCustomTargetRegistry sets the custom target type registry for the chain builder
func (cb *ChainBuilder) SetCustomTargetRegistry(registry *CustomTargetTypeRegistry) {
	cb.customRegistry = registry
}

// SetConfigInternal sets the config for the builder (for server mode)
func (cb *ChainBuilder) SetConfigInternal(config Config) {
	cb.config = config
}

// SetServerMode sets the server mode flag to prevent memory file reload conflicts
func (cb *ChainBuilder) SetServerMode(isServer bool) {
	cb.isServerMode = isServer
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
	pathMap := make(map[string]*Paths)

	// Initialize empty paths for all runtime wants
	// log.Printf("[RECONCILE:PATHS] generatePathsFromConnections() called with %d runtime wants\n", len(cb.wants))
	for wantName := range cb.wants {
		pathMap[wantName] = &Paths{
			In:  []PathInfo{},
			Out: []PathInfo{},
		}
	}

	// Create connections based on using selectors
	for wantName, want := range cb.wants {
		paths := pathMap[wantName]

		// Process using connections for this want
		// if len(want.GetSpec().Using) > 0 {
		// 	log.Printf("[RECONCILE:PATHS] Want '%s' (type: %s) has %d using selectors\n",
		// 		wantName, want.GetMetadata().Type, len(want.GetSpec().Using))
		// }
		for _, usingSelector := range want.GetSpec().Using {
			matchCount := 0

			// Find wants that match this using selector (from runtime only - all wants should be here by connectPhase)
			for otherName, otherWant := range cb.wants {
				if wantName == otherName {
					continue // Skip self-matching
				}
				if cb.matchesSelector(otherWant.GetMetadata().Labels, usingSelector) {
					matchCount++
					// log.Printf("[RECONCILE:PATHS] Matched: '%s' (type: %s, labels: %v) matches selector %v for '%s'\n",
						// otherName, otherWant.GetMetadata().Type, otherWant.GetMetadata().Labels, usingSelector, wantName)

					// Create using path for current want
					// Create a channel directly typed as chain.Chan
					ch := make(chain.Chan, 10)
					inPath := PathInfo{
						Channel: ch,
						Name:    fmt.Sprintf("%s_to_%s", otherName, wantName),
						Active:  true,
					}
					paths.In = append(paths.In, inPath)

					// Log connection details, especially for Flight want
					// if strings.Contains(wantName, "flight") {
					// 	InfoLog("[FLIGHT-CONNECT] Flight want '%s' receiving input from '%s' on channel: %s\n", wantName, otherName, inPath.Name)
					// }

					// Create corresponding output path for the matching want
					otherPaths := pathMap[otherName]
					outPath := PathInfo{
						Channel:        inPath.Channel, // Same channel, shared connection
						Name:           inPath.Name,
						Active:         true,
						TargetWantName: wantName, // Set target want name for output path
					}
					otherPaths.Out = append(otherPaths.Out, outPath)
					// log.Printf("[RECONCILE:PATHS] Created output path for '%s': %s\n", otherName, outPath.Name)

					// Log output connections from other wants to Flight
					// if strings.Contains(otherName, "flight") {
					// 	InfoLog("[FLIGHT-CONNECT] Want '%s' sending output to '%s' via channel: %s\n", otherName, wantName, outPath.Name)
					// }
				}
			}
			// if matchCount == 0 {
			// 	log.Printf("[RECONCILE:PATHS] WARN: Want '%s' selector %v matched 0 wants!\n", wantName, usingSelector)
			// }
		}
	}

	// Convert pointers back to values for the return type
	result := make(map[string]Paths)
	for wantName, pathsPtr := range pathMap {
		result[wantName] = *pathsPtr
	}
	return result
}

// validateConnections validates that all wants have their connectivity requirements satisfied
// Note: Validation checks are performed but not logged to console (debug info not needed in normal operation)
// Reconciliation will run again once idle wants execute and create missing connections via child wants/autoconnection
func (cb *ChainBuilder) validateConnections(pathMap map[string]Paths) {
	for wantName, want := range cb.wants {
		paths := pathMap[wantName]

		// Check if this is an enhanced want that has connectivity requirements
		meta := want.want.GetConnectivityMetadata()

		inCount := len(paths.In)
		outCount := len(paths.Out)

		// Store connectivity status for reconcile loop decision
		// If required inputs not met, mark want as waiting for connections
		if inCount < meta.RequiredInputs {
		}
		if outCount < meta.RequiredOutputs {
		}

		// Validate constraints
		if meta.MaxInputs >= 0 && inCount > meta.MaxInputs {
		}
		if meta.MaxOutputs >= 0 && outCount > meta.MaxOutputs {
		}
	}
}

// isConnectivitySatisfied checks if a want's connectivity requirements are met
func (cb *ChainBuilder) isConnectivitySatisfied(wantName string, want *runtimeWant, pathMap map[string]Paths) bool {
	paths := pathMap[wantName]

	// Check if this is an enhanced want that has connectivity requirements
	meta := want.want.GetConnectivityMetadata()

	inCount := len(paths.In)
	outCount := len(paths.Out)

	// For all wants, enforce normal connectivity requirements
	// If want has required inputs, check if they're satisfied
	if meta.RequiredInputs > 0 && inCount < meta.RequiredInputs {
		return false
	}

	// If want has required outputs, check if they're satisfied
	if meta.RequiredOutputs > 0 && outCount < meta.RequiredOutputs {
		return false
	}

	return true
}

// createWantFunction creates the appropriate function based on want type using registry
func (cb *ChainBuilder) createWantFunction(want *Want) (interface{}, error) {
	wantType := want.Metadata.Type

	// Check if it's a custom target type first
	if cb.customRegistry.IsCustomType(wantType) {
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
		customTypes := cb.customRegistry.ListTypes()

		return nil, fmt.Errorf("Unknown want type: '%s'. Available standard types: %v. Available custom types: %v",
			wantType, availableTypes, customTypes)
	}

	wantInstance := factory(want.Metadata, want.Spec)

	// Extract Want pointer for agent registry and wrapping
	var wantPtr *Want
	if w, ok := wantInstance.(*Want); ok {
		wantPtr = w
	} else {
		// For types that embed Want, extract the Want pointer via reflection
		wantPtr = extractWantViaReflection(wantInstance)
		if wantPtr != nil {
		} else {
		}
	}

	// Set agent registry if available
	if cb.agentRegistry != nil && wantPtr != nil {
		wantPtr.SetAgentRegistry(cb.agentRegistry)
	}

	// Automatically wrap with OwnerAwareWant if the want has owner references
	// This enables parent-child coordination via subscription events
	if len(want.Metadata.OwnerReferences) > 0 {
		wantInstance = NewOwnerAwareWant(wantInstance, want.Metadata, wantPtr)
	}

	return wantInstance, nil
}

// TestCreateWantFunction tests want type creation without side effects (exported for validation)
func (cb *ChainBuilder) TestCreateWantFunction(want *Want) (interface{}, error) {
	return cb.createWantFunction(want)
}

// createCustomTargetWant creates a custom target want using the registry
func (cb *ChainBuilder) createCustomTargetWant(want *Want) (interface{}, error) {
	config, exists := cb.customRegistry.Get(want.Metadata.Type)
	if !exists {
		return nil, fmt.Errorf("custom type '%s' not found in registry", want.Metadata.Type)
	}

	InfoLog("🎯 Creating custom target type: '%s' - %s\n", config.Name, config.Description)

	// Merge custom type defaults with user-provided spec
	mergedSpec := cb.mergeWithCustomDefaults(want.Spec, config)

	// Create the custom target using the registered function
	target := config.CreateTargetFunc(want.Metadata, mergedSpec)

	// Set up target with builder and recipe loader (if available)
	target.SetBuilder(cb)

	// Set up recipe loader for custom targets
	recipeLoader := NewGenericRecipeLoader("recipes")
	target.SetRecipeLoader(recipeLoader)

	// Automatically wrap with OwnerAwareWant if the custom target has owner references
	// This enables parent-child coordination via subscription events (critical for nested targets)
	var wantInstance interface{} = target
	if len(want.Metadata.OwnerReferences) > 0 {
		wantInstance = NewOwnerAwareWant(wantInstance, want.Metadata, &target.Want)
	}

	return wantInstance, nil
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

// hasConfigFileChanged checks if config file has changed
// Used in batch mode to detect updates to the original config file
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
	cb.reconcileWants()

	// Use GlobalReconcileInterval (100ms) for memory file checks and statistics
	// This is separate from GlobalExecutionInterval (20ms) which is used for want execution
	ticker := time.NewTicker(GlobalReconcileInterval)
	statsTicker := time.NewTicker(GlobalReconcileInterval)
	defer ticker.Stop()
	defer statsTicker.Stop()

	for {
		select {
		case <-cb.reconcileStop:
			return
		case trigger := <-cb.reconcileTrigger:
			// Handle reconciliation, control, and retrigger triggers from unified channel
			if trigger == nil {
				continue
			}

			// Only log retrigger types to reduce log spam
		if trigger.Type == "check_completed_retrigger" {
			InfoLog("[RETRIGGER:RECEIVED] Received check_completed_retrigger trigger\n")
		}

			switch trigger.Type {
			case "control":
				if trigger.ControlCommand != nil {
					cmd := trigger.ControlCommand
					// Control trigger received (logging removed to reduce spam)
					cb.distributeControlCommand(cmd)
				}

			case "check_completed_retrigger":
				// Processing retrigger check (logging removed)
				cb.checkAndRetriggerCompletedWants()
				// Retrigger check finished

			default:
				// Handle standard reconciliation trigger
				// Standard reconciliation trigger (logging removed)
				cb.reconcileWants()
			}
		case newWants := <-cb.addWantsChan:
			// Add wants to config and runtime
			cb.reconcileMutex.Lock()
			for _, want := range newWants {
				// Allow multiple wants with the same name (they may be different instances)
				cb.config.Wants = append(cb.config.Wants, want)
				cb.addWant(want)
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
				// Wants deleted (logging removed)
				// Trigger reconciliation to regenerate paths after want deletion
				cb.reconcileWants()
			}
		case <-ticker.C:
			// In batch mode, watch config file for changes
			// In server mode, skip file watching (API is the source of truth)
			if !cb.isServerMode && cb.hasConfigFileChanged() {
				// Load config from original config file (not from memory file)
				// Memory file is only for persistence, not for reloading configuration
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

// reconcileWants performs reconciliation when config changes or during initial load
// Separated into explicit phases: compile -> connect -> start
func (cb *ChainBuilder) reconcileWants() {
	cb.reconcileMutex.Lock()
	defer cb.reconcileMutex.Unlock()

	// Set flag to prevent recursive reconciliation
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

	// Use current config as source of truth during runtime
	// Memory file is only loaded on initial startup
	newConfig := cb.config

	// Check if this is initial load (no lastConfig set)
	isInitialLoad := len(cb.lastConfig.Wants) == 0

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
	} else {
		// Detect changes for ongoing updates
		changes := cb.detectConfigChanges(cb.lastConfig, newConfig)
		if len(changes) == 0 {
			return nil
		}


		// Apply changes in reverse dependency order (sink to generator)
		cb.applyWantChanges(changes)
	}

	// Update last config and hash
	// IMPORTANT: Make a deep copy to avoid reference aliasing issues
	// When both lastConfig and config point to the same Want objects,
	// updates to one appear to update both, breaking change detection
	cb.lastConfig = cb.deepCopyConfig(newConfig)
	cb.lastConfigHash, _ = cb.calculateFileHash(cb.memoryPath)
	// Also track config file hash for batch mode watching
	cb.lastConfigFileHash, _ = cb.calculateFileHash(cb.configPath)

	return nil
}

// connectPhase handles want topology establishment and validation
func (cb *ChainBuilder) connectPhase() error {


	// Process auto-connections for RecipeAgent wants before generating paths
	cb.processAutoConnections()

	// Build parameter subscription connectivity for Target wants
	// This references the owner-children relationships already established in OwnerReferences metadata
	targetCount := 0
	for _, runtimeWant := range cb.wants {
		if target, ok := runtimeWant.function.(*Target); ok {
			targetCount++

			// Find children by checking OwnerReferences.UID in other wants
			childCount := 0
			ownerID := runtimeWant.want.Metadata.ID
			for _, childRuntime := range cb.wants {
				for _, ownerRef := range childRuntime.want.Metadata.OwnerReferences {
					// Compare with owner's ID for proper unique identification
					if ownerRef.ID == ownerID {
						childCount++
						break
					}
				}
			}


			if target.RecipePath != "" && target.recipeLoader != nil {
				// Parse recipe to build parameter subscription map based on ownership
				if err := cb.buildTargetParameterSubscriptions(target); err != nil {
				}
			}
		}
	}

	// Generate new paths based on current wants
	cb.pathMap = cb.generatePathsFromConnections()

	// log.Printf("[RECONCILE:SYNC] Path generation complete. Synchronizing %d wants with their paths\n", len(cb.pathMap))

	// CRITICAL FIX: Synchronize generated paths to individual Want structs
	// This ensures child wants can access their output channels when they execute
	// NOTE: Caller (reconcileWants) already holds reconcileMutex, so we don't lock here
	for wantName, paths := range cb.pathMap {
		if runtimeWant, exists := cb.wants[wantName]; exists {
			// Update the want's paths field with the generated paths
			// This makes output/input channels available to the want during execution
			runtimeWant.want.paths.In = paths.In
			runtimeWant.want.paths.Out = paths.Out
			// log.Printf("[RECONCILE:SYNC] Synchronized paths for '%s': In=%d, Out=%d\n",
			// 	wantName, len(paths.In), len(paths.Out))
		} else {
			log.Printf("[RECONCILE:SYNC] WARN: Runtime want '%s' not found in cb.wants!\n", wantName)
		}
	}

	// Build label-to-users mapping for completed want retrigger detection
	// This pre-computes which wants depend on each want via label selectors
	cb.buildLabelToUsersMapping()

	// Validate connectivity requirements (non-blocking - logs warnings only)
	// Wants with missing connections will remain idle until reconciliation connects them
	cb.validateConnections(cb.pathMap)

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

	return nil
}

// buildTargetParameterSubscriptions builds parameter subscription map using actual runtime child want names
func (cb *ChainBuilder) buildTargetParameterSubscriptions(target *Target) error {
	// Read and parse the recipe file
	recipeData, err := os.ReadFile(target.RecipePath)
	if err != nil {
		return fmt.Errorf("failed to read recipe file: %w", err)
	}

	var recipeDoc struct {
		Recipe struct {
			Parameters map[string]interface{} `yaml:"parameters"`
			Wants      []struct {
				Metadata struct {
					Name string `yaml:"name"`
					Type string `yaml:"type"`
				} `yaml:"metadata"`
				Spec struct {
					Params map[string]interface{} `yaml:"params"`
				} `yaml:"spec"`
			} `yaml:"wants"`
		} `yaml:"recipe"`
	}

	if err := yaml.Unmarshal(recipeData, &recipeDoc); err != nil {
		return fmt.Errorf("failed to parse recipe YAML: %w", err)
	}

	// Initialize subscription map (clear existing to avoid duplicates on reconnection)
	target.parameterSubscriptions = make(map[string][]string)

	// Find actual runtime children by OwnerReference.UID
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

	// Build subscription map using actual runtime child want names
	for _, recipeWant := range recipeDoc.Recipe.Wants {
		// Find the actual runtime child by type
		childType := recipeWant.Metadata.Type
		runtimeChild, exists := runtimeChildren[childType]
		if !exists {
			// Child not yet created, skip
			continue
		}

		actualChildName := runtimeChild.Metadata.Name

		// Iterate through child's params to find which parent params it uses
		for _, childParamValue := range recipeWant.Spec.Params {
			// Check if this param value references a parent parameter (simple string match)
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

// processAutoConnections handles system-wide auto-connection for RecipeAgent wants
func (cb *ChainBuilder) processAutoConnections() {

	// Collect all wants with RecipeAgent enabled
	autoConnectWants := make([]*runtimeWant, 0)
	allWants := make([]*runtimeWant, 0)

	for _, runtimeWant := range cb.wants {
		allWants = append(allWants, runtimeWant)

		// Check if want has RecipeAgent enabled in its metadata or state
		want := runtimeWant.want
		if cb.hasRecipeAgent(want) {
			autoConnectWants = append(autoConnectWants, runtimeWant)
		}
	}

	// Process auto-connections for each RecipeAgent want
	for _, runtimeWant := range autoConnectWants {
		want := runtimeWant.want
		cb.autoConnectWant(want, allWants)
		// Note: want object itself has been updated, no need to sync to separate spec copy
	}

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
		return
	}


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

					} else {

					}
				} else {
				}
			}
		}
	}

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

	// Start new wants if system is running
	if cb.running {
		startedCount := 0

		// First pass: start idle wants (only if connectivity requirements are met)
		for wantName, want := range cb.wants {
			if want.want.GetStatus() == WantStatusIdle {
				// Check if connectivity requirements are met
				paths := cb.pathMap[wantName]
				if w, ok := want.function.(*Want); ok {
					meta := w.GetConnectivityMetadata()
					inCount := len(paths.In)
					outCount := len(paths.Out)

					// Log for coordinator startup
					if wantName == "dynamic-travel-coordinator-5" {
						InfoLog("[RECONCILE:STARTUP] Coordinator Idle→Running: inCount=%d (required=%d), outCount=%d (required=%d)\n", inCount, meta.RequiredInputs, outCount, meta.RequiredOutputs)
					}

					// Skip if required connections are not met
					if inCount < meta.RequiredInputs || outCount < meta.RequiredOutputs {
						if wantName == "dynamic-travel-coordinator-5" {
							InfoLog("[RECONCILE:STARTUP] Coordinator SKIPPED - connectivity not met\n")
						}
						continue
					}
				} else {
				continue
				}

				cb.startWant(wantName, want)
				startedCount++
			}
		}

		// Second pass: restart completed wants if their upstream is running/idle
		for wantName, want := range cb.wants {
			if want.want.GetStatus() == WantStatusAchieved {
				if cb.shouldRestartCompletedWant(wantName, want) {
					want.want.SetStatus(WantStatusIdle)
					cb.startWant(wantName, want)
					startedCount++
				}
			}
		}


		// Third pass: start any idle wants that now have required connections available
		// This allows wants with inputs from just-completed upstream wants to execute
		additionalStarted := 0
		for wantName, want := range cb.wants {
			if want.want.GetStatus() == WantStatusIdle {
				// Check if connectivity requirements are now met
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
	} else {
	}
}

// shouldRestartCompletedWant checks if a completed want should restart
// because its upstream wants are running or idle (have new data)
func (cb *ChainBuilder) shouldRestartCompletedWant(wantName string, want *runtimeWant) bool {
	// Check if any upstream wants (using) are running or idle
	for _, usingSelector := range want.GetSpec().Using {
		for otherName, otherWant := range cb.wants {
			if otherName == wantName {
				continue
			}
			// Check if upstream matches selector and is running/idle
			if cb.matchesSelector(otherWant.GetMetadata().Labels, usingSelector) {
				status := otherWant.want.GetStatus()
				if status == WantStatusReaching || status == WantStatusIdle {
					return true
				}
			}
		}
	}
	return false
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
			} else {
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
	// Compare metadata
	if a.Metadata.Type != b.Metadata.Type {
		return false
	}

	// Compare labels
	if !mapsEqual(a.Metadata.Labels, b.Metadata.Labels) {
		return false
	}

	// Compare spec
	if fmt.Sprintf("%v", a.Spec.Params) != fmt.Sprintf("%v", b.Spec.Params) {
		return false
	}

	if fmt.Sprintf("%v", a.Spec.Using) != fmt.Sprintf("%v", b.Spec.Using) {
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

// deepCopyConfig creates a deep copy of a Config to prevent reference aliasing
// This is critical for change detection to work correctly
func (cb *ChainBuilder) deepCopyConfig(src Config) Config {
	// Copy the wants slice with new Want objects
	copiedWants := make([]*Want, 0, len(src.Wants))
	for _, want := range src.Wants {
		// Deep copy the want
		copiedWant := &Want{
			Metadata: Metadata{
				ID:     want.Metadata.ID,
				Name:   want.Metadata.Name,
				Type:   want.Metadata.Type,
				Labels: copyStringMap(want.Metadata.Labels),
			},
			Spec: WantSpec{
				Params:              copyInterfaceMap(want.Spec.Params),
				Using:               copyUsing(want.Spec.Using),
				StateSubscriptions:  copyStateSubscriptions(want.Spec.StateSubscriptions),
				NotificationFilters: copyNotificationFilters(want.Spec.NotificationFilters),
				Requires:            copyStringSlice(want.Spec.Requires),
			},
		}
		copiedWants = append(copiedWants, copiedWant)
	}

	return Config{Wants: copiedWants}
}

// Helper functions for deep copying
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

func copyInterfaceMap(src map[string]interface{}) map[string]interface{} {
	if src == nil {
		return nil
	}
	dst := make(map[string]interface{}, len(src))
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
// Note: Connections are rebuilt in separate connectPhase()
func (cb *ChainBuilder) applyWantChanges(changes []ChangeEvent) {
	// Sort changes by dependency level (sink wants first)
	sortedChanges := cb.sortChangesByDependency(changes)

	hasWantChanges := false
	for _, change := range sortedChanges {
		switch change.Type {
		case ChangeEventAdd:
			cb.addDynamicWantUnsafe(change.Want)
			hasWantChanges = true
		case ChangeEventUpdate:
			// Update config first
			cb.UpdateWant(change.Want)

			// Sync ALL fields from config want to runtime want
			// This is the single synchronization point for config→runtime
			if runtimeWant, exists := cb.wants[change.WantName]; exists {
				// Find the updated want in config (UpdateWant replaces it in the array)
				var updatedConfigWant *Want
				for _, cfgWant := range cb.config.Wants {
					if cfgWant.Metadata.ID == change.Want.Metadata.ID {
						updatedConfigWant = cfgWant
						break
					}
				}

				if updatedConfigWant != nil {

					// Update the spec reference to point to the new config want's spec
					// This ensures generatePathsFromConnections sees the updated spec
					// spec is now part of want object, removed redundant copy

					// Sync the want's spec from the updated config want
					runtimeWant.want.Spec.Using = copyUsing(updatedConfigWant.Spec.Using)
					runtimeWant.want.Spec.Requires = copyStringSlice(updatedConfigWant.Spec.Requires)
					runtimeWant.want.Spec.Params = copyInterfaceMap(updatedConfigWant.Spec.Params)
					runtimeWant.want.Spec.StateSubscriptions = copyStateSubscriptions(updatedConfigWant.Spec.StateSubscriptions)
					runtimeWant.want.Spec.NotificationFilters = copyNotificationFilters(updatedConfigWant.Spec.NotificationFilters)

					// Sync metadata
					// metadata is now part of want object, removed redundant copy
					runtimeWant.want.Metadata.Labels = copyStringMap(updatedConfigWant.Metadata.Labels)
					runtimeWant.want.Metadata.Name = updatedConfigWant.Metadata.Name
					runtimeWant.want.Metadata.Type = updatedConfigWant.Metadata.Type
					runtimeWant.want.Metadata.ID = updatedConfigWant.Metadata.ID

					// Reset status to Idle so want can be re-executed with new configuration
					runtimeWant.want.SetStatus(WantStatusIdle)

				} else {
				}
			}
			hasWantChanges = true
		case ChangeEventDelete:
			cb.deleteWant(change.WantName)
			hasWantChanges = true
		}
	}

	// Dump memory after want additions/deletions/updates (silent - routine operation)
	if hasWantChanges {
		cb.dumpWantMemoryToYAML()
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
	// Create the function/want
	wantFunction, err := cb.createWantFunction(wantConfig)
	if err != nil {

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

		// Initialize subscription system even for failed wants
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

	// If no Want was found in the want function, create a new one
	// (This handles want types that don't embed or return a Want)
	if wantPtr == nil {

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
	} else {
		// Update the extracted Want with metadata and config info
		wantPtr.Metadata = wantConfig.Metadata
		wantPtr.Spec = wantConfig.Spec
		wantPtr.Status = WantStatusIdle

		// Merge state data if provided
		if wantConfig.State != nil {
			if wantPtr.State == nil {
				wantPtr.State = make(map[string]interface{})
			}
			for k, v := range wantConfig.State {
				wantPtr.State[k] = v
			}
		}

		// Update history
		wantPtr.History = wantConfig.History

		// Initialize parameterHistory with initial parameters if empty
		if len(wantPtr.History.ParameterHistory) == 0 && wantConfig.Spec.Params != nil {
			entry := StateHistoryEntry{
				WantName:   wantConfig.Metadata.Name,
				StateValue: wantConfig.Spec.Params,
				Timestamp:  time.Now(),
			}
			wantPtr.History.ParameterHistory = append(wantPtr.History.ParameterHistory, entry)
		}
	}

	// Initialize subscription system for the want
	wantPtr.InitializeSubscriptionSystem()

	runtimeWant := &runtimeWant{
						function: wantFunction,
		want:     wantPtr,
	}
	cb.wants[wantConfig.Metadata.Name] = runtimeWant

	// Register want for notification system
	cb.registerWantForNotifications(wantConfig, wantFunction, wantPtr)
}

// FindWantByID searches for a want by its metadata.id across all runtime wants
func (cb *ChainBuilder) FindWantByID(wantID string) (*Want, string, bool) {
	// First search in runtime wants
	for wantName, runtimeWant := range cb.wants {
		if runtimeWant.want.Metadata.ID == wantID {
			return runtimeWant.want, wantName, true
		}
	}

	// If not found in runtime, search in config wants
	// This handles newly created wants that haven't been promoted to runtime yet
	for _, configWant := range cb.config.Wants {
		if configWant.Metadata.ID == wantID {
			return configWant, configWant.Metadata.Name, true
		}
	}

	return nil, "", false
}

// UpdateWant updates an existing want's configuration (params, labels, using fields, etc.)
// Automatically triggers reconciliation to process topology changes
// Works in both backend API and batch modes
func (cb *ChainBuilder) UpdateWant(wantConfig *Want) {

	// Find the existing want by metadata.id using universal search
	_, _, exists := cb.FindWantByID(wantConfig.Metadata.ID)
	if !exists {
		// Want not found, add as new
		cb.addDynamicWantUnsafe(wantConfig)
		return
	}

	// Only update config - runtime want will be synchronized in applyWantChanges() via compilePhase
	configUpdated := false
	for i, cfgWant := range cb.config.Wants {
		if cfgWant.Metadata.ID == wantConfig.Metadata.ID {
			cb.config.Wants[i] = wantConfig
			configUpdated = true
			break
		}
	}
	if !configUpdated {
	}

	// Trigger immediate reconciliation via channel (unless already in reconciliation)
	if !cb.inReconciliation {
		select {
		case cb.reconcileTrigger <- &TriggerCommand{Type: "reconcile"}:
			// Trigger sent successfully
		default:
			// Channel already has a pending trigger, skip
		}
	}
}

// deleteWant removes a want from runtime and signals its goroutines to stop
func (cb *ChainBuilder) deleteWant(wantName string) {
	// Send stop signal to the want's goroutine
	if runtimeWant, exists := cb.wants[wantName]; exists {
		if runtimeWant.want.stopChannel == nil {
			runtimeWant.want.stopChannel = make(chan struct{})
		}

		// Close the stop channel to signal all goroutines listening on it
		// This is safe to call multiple times and will wake up all select statements
		close(runtimeWant.want.stopChannel)
	}

	// Remove want from registry
	delete(cb.wants, wantName)
}

// registerWantForNotifications registers a want with the notification system
func (cb *ChainBuilder) registerWantForNotifications(wantConfig *Want, wantFunction interface{}, wantPtr *Want) {
	// Register want in global registry for lookup
	RegisterWant(wantPtr)
}

// startWant starts a single want
func (cb *ChainBuilder) startWant(wantName string, want *runtimeWant) {
	// Check if want is already running or completed to prevent duplicate starts
	if want.want.GetStatus() == WantStatusReaching || want.want.GetStatus() == WantStatusAchieved {
		return
	}

	// Check if this want's connectivity requirements are satisfied
	// If not, skip execution - the reconcile loop will retry once connections are available
	if !cb.isConnectivitySatisfied(wantName, want, cb.pathMap) {
		// Set to Idle state so the Third pass in reconcile can retry this want
		want.want.SetStatus(WantStatusIdle)
		return
	}

	// Get paths for this want - they should already be built by the Reconcile loop
	// For wants with no 'using' selectors (entry points), paths will be empty which is fine
	paths, pathsExist := cb.pathMap[wantName]

	// Check if this want has any using selectors (dependencies on other wants)
	hasUsingSelectors := len(want.want.Spec.Using) > 0

	// Only require paths if the want has 'using' selectors
	// Entry point wants (like Target) don't need paths
	if hasUsingSelectors && (!pathsExist || len(paths.In) == 0) {
		return
	}

	// Check if connectivity requirements are satisfied before executing
	// If not satisfied, trigger reconciliation to re-attempt path generation
	if !cb.isConnectivitySatisfied(wantName, want, cb.pathMap) {
		// Set to Idle state so the Third pass in reconcile can retry this want
		want.want.SetStatus(WantStatusIdle)
		// Trigger reconciliation to retry path generation and connection
		if err := cb.TriggerReconcile(); err != nil {
		}
		return
	}

	// Initialize pathMap entry if it doesn't exist yet (for entry point wants)
	if !pathsExist {
		cb.pathMap[wantName] = Paths{In: []PathInfo{}, Out: []PathInfo{}}
		paths = cb.pathMap[wantName]
	}

	// Prepare active input and output channels - set them in the want's paths
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

	// Start want execution with direct Exec() calls
	if chainWant, ok := want.function.(ChainWant); ok {
		want.want.SetStatus(WantStatusReaching)

		// Initialize stop channel if not already initialized
		if want.want.stopChannel == nil {
			want.want.stopChannel = make(chan struct{})
		}

		// Initialize control channel for suspend/resume/stop/restart operations
		want.want.InitializeControlChannel()

		cb.waitGroup.Add(1)
		go func() {
			defer cb.waitGroup.Done()
			defer func() {
				if want.want.GetStatus() == WantStatusReaching {
					want.want.SetStatus(WantStatusAchieved)
				}
			}()

			for {
				// Check if stop signal was sent (non-blocking check)
				select {
				case <-want.want.stopChannel:
					// Stop signal received - exit gracefully
					want.want.SetStatus(WantStatusTerminated)
					return
				default:
					// Continue execution
				}

				// Check for control signals (suspend/resume/stop/restart)
				if cmd, received := want.want.CheckControlSignal(); received {
					switch cmd.Trigger {
					case ControlTriggerSuspend:
						// Mark as suspended and change status to suspended
						want.want.SetSuspended(true)
						want.want.SetStatus(WantStatusSuspended)
						// Continue to next iteration (execution will be skipped while suspended)

					case ControlTriggerResume:
						// Resume execution and restore running status
						want.want.SetSuspended(false)
						want.want.SetStatus(WantStatusReaching)

					case ControlTriggerStop:
						// Stop execution immediately
						want.want.SetStatus(WantStatusTerminated)
						return

					case ControlTriggerRestart:
						// Restart execution from beginning
						// Reset any execution state if needed
						want.want.SetSuspended(false)
					}
				}

				// Skip execution if want is suspended
				if want.want.IsSuspended() {
					// Sleep but don't execute - just check for control signals
					time.Sleep(GlobalExecutionInterval)
					continue
				}

				// Begin execution cycle for batching state changes
				cb.reconcileMutex.RLock()
				runtimeWant, exists := cb.wants[wantName]
				cb.reconcileMutex.RUnlock()
				if !exists {
					// Want was deleted from registry - exit gracefully
					want.want.SetStatus(WantStatusTerminated)
					return
				}

				// Set the resolved paths with actual channels before execution using proper setter
				runtimeWant.want.SetPaths(activeInputPaths, activeOutputPaths)
				runtimeWant.want.BeginExecCycle()

				// Check again before executing (want might have been deleted while we were setting up)
				select {
				case <-want.want.stopChannel:
					// Stop signal received before execution - abort
					runtimeWant.want.EndExecCycle()
					want.want.SetStatus(WantStatusTerminated)
					return
				default:
					// Continue with execution
				}

				// Direct call - no parameters needed, channels are in want.paths
				finished := chainWant.Exec()

				// End execution cycle and commit batched state changes
				cb.reconcileMutex.RLock()
				runtimeWant, exists = cb.wants[wantName]
				cb.reconcileMutex.RUnlock()
				if exists {
					runtimeWant.want.EndExecCycle()
				}

				if finished {

					// Update want status to completed
					cb.reconcileMutex.RLock()
					runtimeWant, exists := cb.wants[wantName]
					cb.reconcileMutex.RUnlock()
					if exists {
						runtimeWant.want.SetStatus(WantStatusAchieved)
					}

					// Trigger reconciliation after want completes
					// This allows Target wants that created children to be properly connected
					// and allows idle children to be started
					if err := cb.TriggerReconcile(); err != nil {
					}

					// Exit the execution loop - execution is complete
					return
				}

				// Sleep to prevent CPU spinning in tight execution loops
				time.Sleep(GlobalExecutionInterval)
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
			want.Spec = *runtimeWant.want.GetSpec() // Preserve using from runtime spec
			// Stats field removed - data now in State
			want.Status = runtimeWant.want.Status

			// Lock state when copying to prevent concurrent map access
			runtimeWant.want.stateMutex.RLock()
			stateCopy := make(map[string]interface{})
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
			stateCopy := make(map[string]interface{})
			for k, v := range runtimeWant.want.State {
				stateCopy[k] = v
			}
			runtimeWant.want.stateMutex.RUnlock()

			wantConfig := &Want{
				Metadata: *runtimeWant.GetMetadata(),
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

	os.WriteFile(cb.memoryPath, data, 0644)

	// Update lastConfigHash to prevent stats updates from triggering reconciliation
	cb.lastConfigHash, _ = cb.calculateFileHash(cb.memoryPath)
}

// Execute starts the reconcile loop and initial want execution
// For server mode, this runs indefinitely. For batch mode, it waits for completion.
func (cb *ChainBuilder) Execute() {
	cb.ExecuteWithMode(false) // Default: batch mode (waits for completion)
}

// ExecuteWithMode starts execution with specified mode
// serverMode=true: runs indefinitely for server mode
// serverMode=false: waits for wants to complete (batch mode)
func (cb *ChainBuilder) ExecuteWithMode(serverMode bool) {

	// Initialize memory file if configured
	if cb.memoryPath != "" {
		if err := cb.copyConfigToMemory(); err == nil {
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

	// Server mode: run indefinitely, never stop reconcile loop
	if serverMode {
		// Keep running forever - reconcile loop handles all want lifecycle
		select {} // Block forever
	}

	// Batch mode: wait for initial wants and completion
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
	// Create a snapshot of wants while holding lock to avoid concurrent map iteration
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

	// Mark as not running
	cb.reconcileMutex.Lock()
	cb.running = false
	cb.reconcileMutex.Unlock()

	// Final memory dump - ensure it completes before returning (silent - routine operation)
	cb.dumpWantMemoryToYAML()
}

// GetAllWantStates returns the states of all wants
func (cb *ChainBuilder) GetAllWantStates() map[string]*Want {
	states := make(map[string]*Want)
	for name, want := range cb.wants {
		states[name] = want.want
	}
	return states
}

// AddWantsAsync sends wants to be added asynchronously through the reconcile loop
// This is the preferred method for adding wants from within executing Target wants
// to avoid deadlock since the caller may already hold locks
func (cb *ChainBuilder) AddWantsAsync(wants []*Want) error {
	select {
	case cb.addWantsChan <- wants:
		return nil
	default:
		return fmt.Errorf("failed to send wants to reconcile loop (channel full)")
	}
}

// AddWantsAsyncWithTracking sends wants asynchronously and returns their IDs for tracking
// The caller can use the returned IDs to poll with AreWantsAdded() to confirm addition
func (cb *ChainBuilder) AddWantsAsyncWithTracking(wants []*Want) ([]string, error) {
	// Extract IDs from wants
	ids := make([]string, len(wants))
	for i, want := range wants {
		if want.Metadata.ID == "" {
			return nil, fmt.Errorf("want %s has no ID for tracking", want.Metadata.Name)
		}
		ids[i] = want.Metadata.ID
	}

	// Send wants asynchronously
	if err := cb.AddWantsAsync(wants); err != nil {
		return nil, err
	}

	return ids, nil
}

// AreWantsAdded checks if all wants with the given IDs have been added to the runtime
// Returns true only if ALL wants are present in the runtime
func (cb *ChainBuilder) AreWantsAdded(wantIDs []string) bool {
	if len(wantIDs) == 0 {
		return true
	}

	cb.reconcileMutex.RLock()
	defer cb.reconcileMutex.RUnlock()

	// Check if all wants are in the runtime
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

// DeleteWantsAsync sends want IDs to be deleted asynchronously through the reconcile loop
// This is the preferred method for deleting wants to avoid race conditions
func (cb *ChainBuilder) DeleteWantsAsync(wantIDs []string) error {
	select {
	case cb.deleteWantsChan <- wantIDs:
		return nil
	default:
		return fmt.Errorf("failed to send wants to delete through reconcile loop (channel full)")
	}
}

// DeleteWantsAsyncWithTracking sends want IDs to be deleted asynchronously and returns them for tracking
// The caller can use the returned IDs to poll with AreWantsDeleted() to confirm deletion
func (cb *ChainBuilder) DeleteWantsAsyncWithTracking(wantIDs []string) ([]string, error) {
	if err := cb.DeleteWantsAsync(wantIDs); err != nil {
		return nil, err
	}
	return wantIDs, nil
}

// AreWantsDeleted checks if all wants with the given IDs have been removed from the runtime
// Returns true only if ALL wants are no longer present in the runtime
func (cb *ChainBuilder) AreWantsDeleted(wantIDs []string) bool {
	if len(wantIDs) == 0 {
		return true
	}

	cb.reconcileMutex.RLock()
	defer cb.reconcileMutex.RUnlock()

	// Check if all wants are deleted (not in runtime)
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

// addDynamicWantUnsafe adds a want without acquiring the mutex (internal use only)
// Must be called while holding reconcileMutex or within addWantsChan handler
func (cb *ChainBuilder) addDynamicWantUnsafe(want *Want) error {
	// Check for duplicate name and skip if exists
	if _, exists := cb.wants[want.Metadata.Name]; exists {
		return nil
	}

	// Add want to the configuration
	cb.config.Wants = append(cb.config.Wants, want)

	// Create runtime want
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
		// Try to find the memory directory - check both "memory" and "../memory"
		memoryDir := "memory"
		if _, err := os.Stat(memoryDir); os.IsNotExist(err) {
			memoryDir = "../memory"
		}
		if err := os.MkdirAll(memoryDir, 0755); err != nil {
			return fmt.Errorf("failed to create memory directory: %w", err)
		}
		filename = filepath.Join(memoryDir, fmt.Sprintf("memory-%s.yaml", timestamp))
	}

	// Note: Caller must hold reconcileMutex lock for safe concurrent access
	// Convert want map to slice to match config format, preserving runtime spec
	wants := make([]*Want, 0, len(cb.wants))
	for _, runtimeWant := range cb.wants {
		// Use GetAllState() which safely handles mutex locking internally
		stateCopy := runtimeWant.want.GetAllState()

		// Use runtime spec to preserve using, but want state for stats/status
		want := &Want{
			Metadata: *runtimeWant.GetMetadata(),
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

// ==============================
// Suspend/Resume Control Methods
// ==============================

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

// RestartWant restarts execution of a specific want
func (cb *ChainBuilder) RestartWant(wantID string) error {
	cmd := &ControlCommand{
		Trigger:   ControlTriggerRestart,
		WantID:    wantID,
		Timestamp: time.Now(),
		Reason:    "Restarted via API",
	}
	return cb.SendControlCommand(cmd)
}

// SendControlCommand sends a control command to the reconcile loop for distribution
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

// distributeControlCommand distributes a control command to target want(s)
// and propagates to child wants if the target is a parent want
func (cb *ChainBuilder) distributeControlCommand(cmd *ControlCommand) {
	cb.reconcileMutex.RLock()
	defer cb.reconcileMutex.RUnlock()

	// Find the target want by metadata.id (WantID is the metadata ID)
	// We need to search through all wants since cb.wants uses wantName as key
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

	// Send control command to the target want's control channel
	if err := targetRuntime.want.SendControlCommand(cmd); err != nil {
	} else {
	}

	// TODO: Propagate control to child wants if this is a parent want
	// This will require finding parent-child relationships in the Target want implementation
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

// DeleteWantByID removes a want from runtime by its ID
// If the want has children (based on ownerReferences), they will be deleted first (cascade deletion)
func (cb *ChainBuilder) DeleteWantByID(wantID string) error {
	// Phase 1: Find the want name by ID (with lock held briefly)
	cb.reconcileMutex.Lock()

	var wantName string
	wantsCopy := make([]string, 0, len(cb.wants))

	// Collect all want names first
	for name := range cb.wants {
		wantsCopy = append(wantsCopy, name)
	}
	cb.reconcileMutex.Unlock()

	// Search through the collected names for the target want
	cb.reconcileMutex.RLock()
	for _, name := range wantsCopy {
		if runtimeWant, exists := cb.wants[name]; exists {
			if runtimeWant.want.Metadata.ID == wantID {
				wantName = name
				break
			}
		}
	}
	cb.reconcileMutex.RUnlock()

	if wantName == "" {
		return fmt.Errorf("want with ID %s not found in runtime", wantID)
	}

	// Phase 2: Find all children first (cascade deletion) with read lock
	var childrenToDelete []string

	cb.reconcileMutex.RLock()
	for name, runtimeWant := range cb.wants {
		if runtimeWant.want.Metadata.OwnerReferences != nil {
			for _, ownerRef := range runtimeWant.want.Metadata.OwnerReferences {
				if ownerRef.ID == wantID {
					childrenToDelete = append(childrenToDelete, name)
					break
				}
			}
		}
	}
	cb.reconcileMutex.RUnlock()

	// Phase 3: Delete children first (with write lock for each deletion)
	for _, childName := range childrenToDelete {
		cb.reconcileMutex.Lock()
		cb.deleteWant(childName)

		// Also remove child from config to keep it in sync with cb.wants
		// Use ID-based comparison to avoid name collisions
		if runtimeChild, exists := cb.wants[childName]; exists {
			childID := runtimeChild.want.Metadata.ID
			for i, cfgWant := range cb.config.Wants {
				if cfgWant.Metadata.ID == childID {
					cb.config.Wants = append(cb.config.Wants[:i], cb.config.Wants[i+1:]...)
					break
				}
			}
		}

		cb.reconcileMutex.Unlock()
	}

	// Phase 4: Delete the parent want (with write lock)
	cb.reconcileMutex.Lock()
	cb.deleteWant(wantName)

	// Also remove from config so detectConfigChanges sees the deletion
	// Use ID-based comparison to ensure we delete only the correct want
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

// selectorToKey converts a label selector map to a unique string key
// Used for label-to-users mapping in completed want detection
// Example: {role: "coordinator", stage: "final"} → "role:coordinator,stage:final"
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

	// Build key string
	var parts []string
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s:%s", k, selector[k]))
	}
	return strings.Join(parts, ",")
}

// buildLabelToUsersMapping constructs mapping from label selectors to want names
// This enables O(1) lookup of which wants depend on a given want
// Called after path generation to pre-compute dependencies
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

// checkAndRetriggerCompletedWants checks for completed wants and notifies their dependents
// Called from reconcileLoop when a completed want retrigger check trigger is received
// This is the core async mechanism for retrigger detection
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

	// Process each completed want (using want ID)
	anyWantRetriggered := false
	for wantID, isCompleted := range completedSnapshot {

		if isCompleted {
			InfoLog("[RETRIGGER:CHECK] Checking users for completed want ID '%s'\n", wantID)
			users := cb.findUsersOfCompletedWant(wantID)
			InfoLog("[RETRIGGER:CHECK] Found %d users for want ID '%s'\n", len(users), wantID)

			if len(users) > 0 {
				InfoLog("[RETRIGGER] Want ID '%s' completed, found %d users to retrigger\n", wantID, len(users))

				for _, userName := range users {
					// Reset dependent want to Idle so it can be re-executed
					// This allows the want to pick up new data from the completed source
					if runtimeWant, ok := wantSnapshot[userName]; ok {
						runtimeWant.want.SetStatus(WantStatusIdle)
						anyWantRetriggered = true
					}
				}
			}
		}
	}

	// If any want was retriggered, queue a reconciliation trigger
	// (cannot call reconcileWants() directly due to mutex re-entrancy)
	if anyWantRetriggered {
		select {
		case cb.reconcileTrigger <- &TriggerCommand{Type: "reconcile"}:
			// Trigger queued successfully
		default:
			// Channel full, ignore (next reconciliation cycle will handle it)
		}
	}
}

// findUsersOfCompletedWant finds all wants that depend on a given completed want
// Takes want ID as parameter and uses it to look up the want in cb.wants by name
// Uses the pre-computed labelToUsers mapping for O(1) lookup
// Returns slice of want names that use the completed want's labels
func (cb *ChainBuilder) findUsersOfCompletedWant(completedWantID string) []string {
	// Find the completed want by iterating through wants to find one with matching ID
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
	labels := completedWant.Metadata.Labels
	if labels == nil {
		return []string{}
	}

	// For each label in the completed want, find users
	users := make(map[string]bool) // De-duplicate users

	// Generate selector keys from completed want's labels
	// and look up users in the pre-computed mapping
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

	// Convert to slice
	userList := make([]string, 0, len(users))
	for userName := range users {
		userList = append(userList, userName)
	}
	return userList
}

// UpdateCompletedFlag updates the completed flag for a want based on its status
// Called from Want.SetStatus() to track which wants are completed
// Uses mutex to protect concurrent access
// MarkWantCompleted is the new preferred method for wants to notify the ChainBuilder of completion
// MarkWantCompleted marks a want as completed using want ID
// Called by receiver wants (e.g., Coordinators) when they reach completion state
// Replaces the previous pattern where senders would call UpdateCompletedFlag
func (cb *ChainBuilder) MarkWantCompleted(wantID string, status WantStatus) {
	cb.completedFlagsMutex.Lock()
	defer cb.completedFlagsMutex.Unlock()

	isCompleted := (status == WantStatusAchieved)
	cb.wantCompletedFlags[wantID] = isCompleted
	log.Printf("[WANT-COMPLETED] Want ID '%s' notified completion with status=%s\n", wantID, status)
}

// UpdateCompletedFlag updates completion flag using want ID
// Deprecated: Use MarkWantCompleted instead
func (cb *ChainBuilder) UpdateCompletedFlag(wantID string, status WantStatus) {
	cb.completedFlagsMutex.Lock()
	defer cb.completedFlagsMutex.Unlock()

	isCompleted := (status == WantStatusAchieved)
	cb.wantCompletedFlags[wantID] = isCompleted
	log.Printf("[UPDATE-COMPLETED-FLAG] Want ID '%s' status=%s, isCompleted=%v\n", wantID, status, isCompleted)
}

// IsCompleted returns whether a want is currently in completed state
// Safe to call from any goroutine with RLock protection
func (cb *ChainBuilder) IsCompleted(wantID string) bool {
	cb.completedFlagsMutex.RLock()
	defer cb.completedFlagsMutex.RUnlock()
	return cb.wantCompletedFlags[wantID]
}

// TriggerCompletedWantRetriggerCheck sends a non-blocking trigger to the reconcile loop
// to check for completed wants and notify their dependents
// Uses the unified reconcileTrigger channel with Type="check_completed_retrigger"
func (cb *ChainBuilder) TriggerCompletedWantRetriggerCheck() {
	select {
	case cb.reconcileTrigger <- &TriggerCommand{
		Type: "check_completed_retrigger",
	}:
		// Trigger sent successfully
		InfoLog("[RETRIGGER:SEND] Non-blocking retrigger check trigger sent to reconcile loop\n")
	default:
		// Channel is full (rare), trigger is already pending
		InfoLog("[RETRIGGER:SEND] Warning: reconcileTrigger channel full, skipping trigger\n")
	}
}

// SetGlobalChainBuilder sets the global ChainBuilder instance
// Called from server initialization to make ChainBuilder accessible to wants
func SetGlobalChainBuilder(cb *ChainBuilder) {
	globalChainBuilder = cb
}

// GetGlobalChainBuilder returns the global ChainBuilder instance
// Returns nil if not yet initialized
func GetGlobalChainBuilder() *ChainBuilder {
	return globalChainBuilder
}
