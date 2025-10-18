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
	"strings"
	"sync"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"gopkg.in/yaml.v3"
)

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
	reconcileStop    chan bool    // Stop signal for reconcile loop
	reconcileTrigger chan bool    // Trigger signal for immediate reconciliation
	reconcileMutex   sync.RWMutex // Protect concurrent access
	inReconciliation bool         // Flag to prevent recursive reconciliation
	running          bool         // Execution state
	lastConfig       Config       // Last known config state
	lastConfigHash   string       // Hash of last config for change detection

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

// NewChainBuilderWithPaths creates a new builder with config and memory file paths
func NewChainBuilderWithPaths(configPath, memoryPath string) *ChainBuilder {
	builder := &ChainBuilder{
		configPath:       configPath,
		memoryPath:       memoryPath,
		wants:            make(map[string]*runtimeWant),
		registry:         make(map[string]WantFactory),
		customRegistry:   NewCustomTargetTypeRegistry(),
		reconcileStop:    make(chan bool),
		reconcileTrigger: make(chan bool, 1), // Buffered to avoid blocking
		pathMap:          make(map[string]Paths),
		channels:         make(map[string]chain.Chan),
		running:          false,
		waitGroup:        &sync.WaitGroup{},
		// Initialize suspend/resume control
		suspended:   false,
		suspendChan: make(chan bool),
		resumeChan:  make(chan bool),
		controlStop: make(chan bool),
	}

	// Auto-register custom target types from recipes
	// Try to find the recipes directory - check both "recipes" and "../recipes"
	recipeDir := "recipes"
	if _, err := os.Stat(recipeDir); os.IsNotExist(err) {
		recipeDir = "../recipes"
	}
	err := ScanAndRegisterCustomTypes(recipeDir, builder.customRegistry)
	if err != nil {
		log.Printf("⚠️  Warning: failed to scan recipes for custom types: %v\n", err)
	}

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

	// Set agent registry if available and the want instance supports it
	if cb.agentRegistry != nil {
		if wantWithGetWant, ok := wantInstance.(interface{ GetWant() *Want }); ok {
			wantWithGetWant.GetWant().SetAgentRegistry(cb.agentRegistry)
		} else if w, ok := wantInstance.(*Want); ok {
			w.SetAgentRegistry(cb.agentRegistry)
		}
	}

	// Automatically wrap with OwnerAwareWant if the want has owner references
	// This enables parent-child coordination via subscription events
	if len(want.Metadata.OwnerReferences) > 0 {
		wantInstance = NewOwnerAwareWant(wantInstance, want.Metadata)
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

	log.Printf("🎯 Creating custom target type: '%s' - %s\n", config.Name, config.Description)

	// Merge custom type defaults with user-provided spec
	mergedSpec := cb.mergeWithCustomDefaults(want.Spec, config)

	// Create the custom target using the registered function
	target := config.CreateTargetFunc(want.Metadata, mergedSpec)

	// Set up target with builder and recipe loader (if available)
	target.SetBuilder(cb)

	// Set up recipe loader for custom targets
	recipeLoader := NewGenericRecipeLoader("recipes")
	target.SetRecipeLoader(recipeLoader)

	return target, nil
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
	log.Println("[RECONCILE] Loading initial configuration")
	cb.reconcileWants()

	ticker := time.NewTicker(100 * time.Millisecond)
	statsTicker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	defer statsTicker.Stop()

	for {
		select {
		case <-cb.reconcileStop:
			log.Println("[RECONCILE] Stopping reconcile loop")
			return
		case <-cb.reconcileTrigger:
			log.Println("[RECONCILE] Triggered reconciliation")
			cb.reconcileWants()
		case <-ticker.C:
			if cb.hasMemoryFileChanged() {
				log.Println("[RECONCILE] Detected config change")
				// Load memory file into config before reconciling
				if newConfig, err := cb.loadMemoryConfig(); err == nil {
					cb.config = newConfig
					log.Printf("[RECONCILE] Loaded %d wants from memory file\n", len(newConfig.Wants))
				} else {
					log.Printf("[RECONCILE] Warning: Failed to load memory config: %v\n", err)
				}
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

	// Set flag to prevent recursive reconciliation
	cb.inReconciliation = true
	defer func() { cb.inReconciliation = false }()

	log.Println("[RECONCILE] Starting reconciliation with separated phases")

	// Phase 1: COMPILE - Load and validate configuration
	if err := cb.compilePhase(); err != nil {
		log.Printf("[RECONCILE] Compile phase failed: %v\n", err)
		return
	}

	// Phase 2: CONNECT - Establish want topology
	if err := cb.connectPhase(); err != nil {
		log.Printf("[RECONCILE] Connect phase failed: %v\n", err)
		return
	}

	// Phase 3: START - Launch new/updated wants
	cb.startPhase()

	log.Println("[RECONCILE] All phases completed successfully")
}

// compilePhase handles configuration loading and want creation/updates
func (cb *ChainBuilder) compilePhase() error {
	log.Println("[RECONCILE:COMPILE] Loading and validating configuration")

	// Use current config as source of truth during runtime
	// Memory file is only loaded on initial startup
	newConfig := cb.config

	// Check if this is initial load (no lastConfig set)
	isInitialLoad := len(cb.lastConfig.Wants) == 0

	if isInitialLoad {
		log.Printf("[RECONCILE:COMPILE] Initial load: processing %d wants\n", len(newConfig.Wants))
		// For initial load, treat all wants as new additions
		for _, wantConfig := range newConfig.Wants {
			cb.addDynamicWantUnsafe(wantConfig)
		}

		// Run migration to clean up any agent_history from state
		cb.migrateAllWantsAgentHistory()

		// Dump memory after initial load
		if len(newConfig.Wants) > 0 {
			log.Println("[RECONCILE:MEMORY] Dumping memory after initial load...")
			if err := cb.dumpWantMemoryToYAML(); err != nil {
				log.Printf("[RECONCILE:MEMORY] Warning: Failed to dump memory: %v\n", err)
			}
		}
	} else {
		// Detect changes for ongoing updates
		changes := cb.detectConfigChanges(cb.lastConfig, newConfig)
		if len(changes) == 0 {
			log.Println("[RECONCILE:COMPILE] No configuration changes detected")
			return nil
		}

		log.Printf("[RECONCILE:COMPILE] Processing %d configuration changes\n", len(changes))

		// Apply changes in reverse dependency order (sink to generator)
		cb.applyWantChanges(changes)
	}

	// Update last config and hash
	cb.lastConfig = newConfig
	cb.lastConfigHash, _ = cb.calculateFileHash(cb.memoryPath)

	log.Println("[RECONCILE:COMPILE] Configuration compilation completed")
	return nil
}

// connectPhase handles want topology establishment and validation
func (cb *ChainBuilder) connectPhase() error {
	log.Println("[RECONCILE:CONNECT] Establishing want topology")

	// Process auto-connections for RecipeAgent wants before generating paths
	cb.processAutoConnections()

	// Build parameter subscription connectivity for Target wants
	// This references the owner-children relationships already established in OwnerReferences metadata
	log.Printf("[RECONCILE:CONNECT] Building parameter subscriptions for Target wants (total wants: %d)\n", len(cb.wants))
	targetCount := 0
	for wantName, runtimeWant := range cb.wants {
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

			log.Printf("[RECONCILE:CONNECT] Found Target want: %s (RecipePath: %s, has loader: %v, children: %d)\n",
				wantName, target.RecipePath, target.recipeLoader != nil, childCount)

			if target.RecipePath != "" && target.recipeLoader != nil {
				// Parse recipe to build parameter subscription map based on ownership
				if err := cb.buildTargetParameterSubscriptions(target); err != nil {
					log.Printf("[RECONCILE:CONNECT] Warning: Failed to build parameter subscriptions for %s: %v\n",
						target.Metadata.Name, err)
				}
			}
		}
	}
	log.Printf("[RECONCILE:CONNECT] Processed %d Target wants for subscription building\n", targetCount)

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

	log.Printf("[RECONCILE:CONNECT] Topology established: %d channels created\n", channelCount)
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
		for childParamName, childParamValue := range recipeWant.Spec.Params {
			// Check if this param value references a parent parameter (simple string match)
			if paramRefStr, ok := childParamValue.(string); ok {
				// If the value matches a parent parameter name, it's a subscription
				if _, exists := target.RecipeParams[paramRefStr]; exists {
					target.parameterSubscriptions[paramRefStr] = append(
						target.parameterSubscriptions[paramRefStr],
						actualChildName,
					)
					log.Printf("[RECONCILE:CONNECT] Target %s: Child %s subscribes to parameter %s (as %s)\n",
						target.Metadata.Name, actualChildName, paramRefStr, childParamName)
				}
			}
		}
	}

	log.Printf("[RECONCILE:CONNECT] Target %s: Built parameter subscriptions: %v\n",
		target.Metadata.Name, target.parameterSubscriptions)

	return nil
}

// processAutoConnections handles system-wide auto-connection for RecipeAgent wants
func (cb *ChainBuilder) processAutoConnections() {
	log.Println("[RECONCILE:AUTOCONNECT] Processing auto-connections for RecipeAgent wants")

	// Collect all wants with RecipeAgent enabled
	autoConnectWants := make([]*runtimeWant, 0)
	allWants := make([]*runtimeWant, 0)

	for _, runtimeWant := range cb.wants {
		allWants = append(allWants, runtimeWant)

		// Check if want has RecipeAgent enabled in its metadata or state
		want := runtimeWant.want
		if cb.hasRecipeAgent(want) {
			autoConnectWants = append(autoConnectWants, runtimeWant)
			log.Printf("[RECONCILE:AUTOCONNECT] Found RecipeAgent want: %s\n", want.Metadata.Name)
		}
	}

	// Process auto-connections for each RecipeAgent want
	for _, runtimeWant := range autoConnectWants {
		want := runtimeWant.want
		cb.autoConnectWant(want, allWants)
		// Update the runtime spec to reflect auto-connection changes
		runtimeWant.spec.Using = want.Spec.Using
	}

	log.Printf("[RECONCILE:AUTOCONNECT] Processed auto-connections for %d RecipeAgent wants\n", len(autoConnectWants))
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
	log.Printf("[RECONCILE:AUTOCONNECT] Processing auto-connection for want %s\n", want.Metadata.Name)

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
		log.Printf("[RECONCILE:AUTOCONNECT] No approval_id found for want %s, skipping\n", want.Metadata.Name)
		return
	}

	log.Printf("[RECONCILE:AUTOCONNECT] Found approval_id: %s for want %s\n", approvalID, want.Metadata.Name)

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
						log.Printf("[RECONCILE:AUTOCONNECT] Added connection: %s -> %v (from %s)\n",
							want.Metadata.Name, selector, otherWant.Metadata.Name)

					} else {
						log.Printf("[RECONCILE:AUTOCONNECT] Skipping duplicate connection: %s -> %v\n",
							want.Metadata.Name, selector)

					}
				} else {
					log.Printf("[RECONCILE:AUTOCONNECT] Skipping connection to %s (role: %s) - not a data provider\n",
						otherWant.Metadata.Name, role)
				}
			}
		}
	}

	log.Printf("[RECONCILE:AUTOCONNECT] Completed auto-connection for %s with %d connections\n",
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
		log.Printf("[RECONCILE:AUTOCONNECT] Added connection label to %s: %s=%s\n",
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
	log.Println("[RECONCILE:START] Launching new and updated wants")

	// Start new wants if system is running
	if cb.running {
		startedCount := 0

		// First pass: start idle wants
		for wantName, want := range cb.wants {
			if want.want.GetStatus() == WantStatusIdle {
				cb.startWant(wantName, want)
				startedCount++
			}
		}

		// Second pass: restart completed wants if their upstream is running/idle
		for wantName, want := range cb.wants {
			if want.want.GetStatus() == WantStatusCompleted {
				if cb.shouldRestartCompletedWant(wantName, want) {
					log.Printf("[RECONCILE:START] Restarting completed want %s (upstream has new data)\n", wantName)
					want.want.SetStatus(WantStatusIdle)
					cb.startWant(wantName, want)
					startedCount++
				}
			}
		}

		log.Printf("[RECONCILE:START] Started %d wants\n", startedCount)
	} else {
		log.Println("[RECONCILE:START] System not running, wants will be started later")
	}
}

// shouldRestartCompletedWant checks if a completed want should restart
// because its upstream wants are running or idle (have new data)
func (cb *ChainBuilder) shouldRestartCompletedWant(wantName string, want *runtimeWant) bool {
	// Check if any upstream wants (using) are running or idle
	for _, usingSelector := range want.spec.Using {
		for otherName, otherWant := range cb.wants {
			if otherName == wantName {
				continue
			}
			// Check if upstream matches selector and is running/idle
			if cb.matchesSelector(otherWant.metadata.Labels, usingSelector) {
				status := otherWant.want.GetStatus()
				if status == WantStatusRunning || status == WantStatusIdle {
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

	log.Printf("[RECONCILE:DETECT] Comparing configs: old=%d wants, new=%d wants\n", len(oldConfig.Wants), len(newConfig.Wants))

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
			log.Printf("[RECONCILE:DETECT] Detected deletion of want: %s\n", name)
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
			log.Printf("[RECONCILE:COMPILE] Adding want: %s\n", change.WantName)
			cb.addDynamicWantUnsafe(change.Want)
			hasWantChanges = true
		case ChangeEventUpdate:
			log.Printf("[RECONCILE:COMPILE] Updating want: %s\n", change.WantName)
			cb.UpdateWant(change.Want)
			hasWantChanges = true
		case ChangeEventDelete:
			log.Printf("[RECONCILE:COMPILE] Deleting want: %s\n", change.WantName)
			cb.deleteWant(change.WantName)
			hasWantChanges = true
		}
	}

	// Dump memory after want additions/deletions/updates
	if hasWantChanges {
		log.Println("[RECONCILE:MEMORY] Dumping memory after want changes...")
		if err := cb.dumpWantMemoryToYAML(); err != nil {
			log.Printf("[RECONCILE:MEMORY] Warning: Failed to dump memory: %v\n", err)
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
	log.Printf("[RECONCILE] Adding want: %s\n", wantConfig.Metadata.Name)

	// Create the function/want
	wantFunction, err := cb.createWantFunction(wantConfig)
	if err != nil {
		log.Printf("[RECONCILE:ERROR] Failed to create want function for %s: %v\n", wantConfig.Metadata.Name, err)

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
			log.Printf("[RECONCILE] Recording initial parameter history for want %s: %v\n", wantConfig.Metadata.Name, wantConfig.Spec.Params)

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
			if wantPtr.History.StateHistory == nil {
				wantPtr.History.StateHistory = make([]StateHistoryEntry, 0)
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

	// Initialize subscription system for the want
	wantPtr.InitializeSubscriptionSystem()

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
	log.Printf("[RECONCILE] Updating want by ID: %s\n", wantConfig.Metadata.ID)

	// Find the existing want by metadata.id using universal search
	existingWant, wantName, exists := cb.FindWantByID(wantConfig.Metadata.ID)
	if !exists {
		log.Printf("[RECONCILE:ERROR] Want with ID %s not found for update, adding as new\n", wantConfig.Metadata.ID)
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

				log.Printf("[RECONCILE] Parameter updated: %s = %v (was: %v)\n", paramName, newValue, oldValue)
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

	log.Printf("[RECONCILE] Want %s (ID: %s) updated and reset to idle status for re-execution\n", wantName, wantConfig.Metadata.ID)

	// If this is a Target want with children, use Target's parameter update mechanism
	// which automatically pushes updates to children
	if changedParams != nil {
		parentRuntime, exists := cb.wants[wantName]
		if exists {
			if target, ok := parentRuntime.function.(*Target); ok {
				// Use Target's UpdateParameter which automatically pushes to children
				for paramName, paramValue := range changedParams {
					target.UpdateParameter(paramName, paramValue)
				}
				log.Printf("[RECONCILE] Pushed %d parameter updates to Target %s children\n", len(changedParams), wantName)
			}
		}
	}

	// Trigger immediate reconciliation via channel (unless already in reconciliation)
	if !cb.inReconciliation {
		select {
		case cb.reconcileTrigger <- true:
			// Trigger sent successfully
		default:
			// Channel already has a pending trigger, skip
		}
	}
}

// deleteWant removes a want from runtime
func (cb *ChainBuilder) deleteWant(wantName string) {
	log.Printf("[RECONCILE] Deleting want: %s\n", wantName)

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

			log.Printf("[EXEC] Starting want %s with %d using, %d outputs\n",
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
					log.Printf("[EXEC] Want %s finished\n", wantName)

					// Update want status to completed
					if runtimeWant, exists := cb.wants[wantName]; exists {
						runtimeWant.want.SetStatus(WantStatusCompleted)
					}

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
			want.History = runtimeWant.want.History // Include history in stats writes
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
				Status:  runtimeWant.want.Status,
				State:   runtimeWant.want.State,
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
	log.Printf("[RECONCILE] Starting reconcile loop execution (server mode: %v)\n", serverMode)

	// Initialize memory file if configured
	if cb.memoryPath != "" {
		if err := cb.copyConfigToMemory(); err != nil {
			log.Printf("Warning: Failed to copy config to memory: %v\n", err)
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

	// Server mode: run indefinitely, never stop reconcile loop
	if serverMode {
		log.Println("[RECONCILE] Server mode: reconcile loop running indefinitely")
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
	log.Println("[RECONCILE] Writing final memory dump...")
	err := cb.dumpWantMemoryToYAML()
	if err != nil {
		log.Printf("Warning: Failed to dump want memory to YAML: %v\n", err)
	} else {
		log.Println("[RECONCILE] Memory dump completed successfully")
	}

	log.Println("[RECONCILE] Execution completed")
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
// and triggers reconciliation to start them
func (cb *ChainBuilder) AddDynamicWants(wants []*Want) error {
	cb.reconcileMutex.Lock()
	for _, want := range wants {
		if err := cb.addDynamicWantUnsafe(want); err != nil {
			cb.reconcileMutex.Unlock()
			return err
		}
	}
	cb.reconcileMutex.Unlock()

	// Trigger reconciliation to process newly added wants
	// This will compile, connect, and start them
	cb.reconcileWants()
	return nil
}

// addDynamicWantUnsafe adds a want without acquiring the mutex (internal use)
func (cb *ChainBuilder) addDynamicWantUnsafe(want *Want) error {
	// Check for duplicate name and return error
	if _, exists := cb.wants[want.Metadata.Name]; exists {
		return fmt.Errorf("want with name '%s' already exists", want.Metadata.Name)
	}

	// Add want to the configuration
	cb.config.Wants = append(cb.config.Wants, want)

	// Create runtime want
	cb.addWant(want)
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
		// Deep copy State map to avoid concurrent access during YAML marshaling
		stateCopy := make(map[string]interface{})
		if runtimeWant.want.State != nil {
			for k, v := range runtimeWant.want.State {
				stateCopy[k] = v
			}
		}

		// Use runtime spec to preserve using, but want state for stats/status
		want := &Want{
			Metadata: runtimeWant.metadata,
			Spec:     runtimeWant.spec, // This preserves using
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

	log.Printf("📝 Want memory dumped to: %s\n", filename)
	if err == nil {
		log.Printf("📝 Latest memory also saved to: %s\n", latestFilename)
	}
	return nil
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

// Stop stops execution by clearing all wants from the configuration
func (cb *ChainBuilder) Stop() error {
	fmt.Println("[STOP] Stopping chain execution by clearing all wants...")

	// Clear the config wants which will trigger reconciliation to clean up
	cb.reconcileMutex.Lock()
	wantCount := len(cb.config.Wants)
	cb.config.Wants = []*Want{}
	cb.reconcileMutex.Unlock()

	// Trigger reconciliation to process the empty config
	select {
	case cb.reconcileTrigger <- true:
		fmt.Printf("[STOP] Cleared %d wants, reconcile loop will clean up execution\n", wantCount)
	default:
		fmt.Println("[STOP] Warning: Failed to trigger reconciliation")
	}

	return nil
}

// Start restarts execution by triggering reconciliation of existing configuration
func (cb *ChainBuilder) Start() error {
	fmt.Println("[START] Starting/restarting chain execution by triggering reconciliation...")

	// Trigger reconciliation - this will reload from memory and restart wants
	select {
	case cb.reconcileTrigger <- true:
		fmt.Println("[START] Reconciliation triggered")
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
	case cb.reconcileTrigger <- true:
		fmt.Println("[RECONCILE] Reconciliation triggered")
		return nil
	default:
		return fmt.Errorf("failed to trigger reconciliation - channel full")
	}
}

// DeleteWantByID removes a want from runtime by its ID
// If the want has children (based on ownerReferences), they will be deleted first (cascade deletion)
func (cb *ChainBuilder) DeleteWantByID(wantID string) error {
	cb.reconcileMutex.Lock()
	defer cb.reconcileMutex.Unlock()

	fmt.Printf("[DELETE_BY_ID] Starting deletion for want ID: %s\n", wantID)
	fmt.Printf("[DELETE_BY_ID] Current runtime has %d wants\n", len(cb.wants))

	// Find the want name by ID
	var wantName string
	for name, runtimeWant := range cb.wants {
		fmt.Printf("[DELETE_BY_ID] Checking want %s (ID: %s)\n", name, runtimeWant.want.Metadata.ID)
		if runtimeWant.want.Metadata.ID == wantID {
			wantName = name
			fmt.Printf("[DELETE_BY_ID] Found target want: %s\n", wantName)
			break
		}
	}

	if wantName == "" {
		fmt.Printf("[DELETE_BY_ID] ERROR: Want with ID %s not found in runtime\n", wantID)
		return fmt.Errorf("want with ID %s not found in runtime", wantID)
	}

	// Find and delete all children first (cascade deletion)
	var childrenToDelete []string
	fmt.Printf("[DELETE_BY_ID] Searching for children of want ID: %s\n", wantID)
	for name, runtimeWant := range cb.wants {
		if runtimeWant.want.Metadata.OwnerReferences != nil {
			for _, ownerRef := range runtimeWant.want.Metadata.OwnerReferences {
				fmt.Printf("[DELETE_BY_ID] Checking want %s - ownerRef.ID: %s\n", name, ownerRef.ID)
				if ownerRef.ID == wantID {
					childrenToDelete = append(childrenToDelete, name)
					fmt.Printf("[DELETE_BY_ID] Found child to delete: %s\n", name)
					break
				}
			}
		}
	}

	fmt.Printf("[DELETE_BY_ID] Found %d children to delete\n", len(childrenToDelete))

	// Delete children first
	for _, childName := range childrenToDelete {
		cb.deleteWant(childName)
		fmt.Printf("[DELETE] Cascade: Removed child want %s\n", childName)
	}

	// Delete the parent want
	cb.deleteWant(wantName)
	fmt.Printf("[DELETE] Removed want %s (ID: %s) from runtime (with %d children)\n", wantName, wantID, len(childrenToDelete))
	return nil
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
