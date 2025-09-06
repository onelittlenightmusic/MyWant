package main

import (
	"fmt"
	"MyWant/chain"
	"os"
	"path/filepath"
	"sync"
	"time"
	"crypto/md5"
	"io"
	"context"
	
	"gopkg.in/yaml.v3"
	"github.com/getkin/kin-openapi/openapi3"
)

// ChainFunction represents a generalized chain function interface
type ChainFunction interface {
	CreateFunction() func(using []chain.Chan, outputs []chain.Chan) bool
}

// ChainWant represents a want that can create chain functions
type ChainWant interface {
	CreateFunction() func(using []chain.Chan, outputs []chain.Chan) bool
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
	APIVersion          string `json:"apiVersion" yaml:"apiVersion"`
	Kind                string `json:"kind" yaml:"kind"`
	Name                string `json:"name" yaml:"name"`
	Controller          bool   `json:"controller,omitempty" yaml:"controller,omitempty"`
	BlockOwnerDeletion  bool   `json:"blockOwnerDeletion,omitempty" yaml:"blockOwnerDeletion,omitempty"`
}

// Metadata contains want identification and classification info
type Metadata struct {
	Name            string            `json:"name" yaml:"name"`
	Type            string            `json:"type" yaml:"type"`
	Labels          map[string]string `json:"labels" yaml:"labels"`
	OwnerReferences []OwnerReference  `json:"ownerReferences,omitempty" yaml:"ownerReferences,omitempty"`
}

// WantSpec contains the desired state configuration for a want
type WantSpec struct {
	Template     string                 `json:"template,omitempty" yaml:"template,omitempty"` // Deprecated, use Recipe instead
	Recipe       string                 `json:"recipe,omitempty" yaml:"recipe,omitempty"`
	RecipeParams map[string]interface{} `json:"recipe_params,omitempty" yaml:"recipe_params,omitempty"`
	Params       map[string]interface{} `json:"params" yaml:"params"`
	Using        []map[string]string    `json:"using,omitempty" yaml:"using,omitempty"`
}

// Want represents a processing unit in the chain
type Want struct {
	Metadata Metadata               `json:"metadata" yaml:"metadata"`
	Spec     WantSpec               `json:"spec" yaml:"spec"`
	Stats    WantStats              `json:"stats,omitempty" yaml:"stats,omitempty"`
	Status   WantStatus             `json:"status,omitempty" yaml:"status,omitempty"`
	State    map[string]interface{} `json:"state,omitempty" yaml:"state,omitempty"`
}

// SetStatus updates the want's status
func (n *Want) SetStatus(status WantStatus) {
	n.Status = status
}

// GetStatus returns the current want status
func (n *Want) GetStatus() WantStatus {
	return n.Status
}

// StoreState stores a key-value pair in the want's state
func (n *Want) StoreState(key string, value interface{}) {
	if n.State == nil {
		n.State = make(map[string]interface{})
	}
	n.State[key] = value
}

// GetState retrieves a value from the want's state
func (n *Want) GetState(key string) (interface{}, bool) {
	if n.State == nil {
		return nil, false
	}
	value, exists := n.State[key]
	return value, exists
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
	n.StoreState("final_stats", n.Stats)
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
	n.StoreState("stats_at_failure", n.Stats)
}

// Config holds the complete declarative configuration
type Config struct {
	Wants []Want `json:"wants" yaml:"wants"`
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
	MaxInputs       int    // -1 for unlimited
	MaxOutputs      int    // -1 for unlimited
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

// WantStats holds statistical information for a want
// WantStats represents dynamic statistics as key-value pairs
type WantStats map[string]interface{}

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
	waitGroup      *sync.WaitGroup           // Execution synchronization
	config         Config                    // Current configuration
	
	// Reconcile loop fields
	reconcileStop  chan bool                 // Stop signal for reconcile loop
	reconcileMutex sync.RWMutex             // Protect concurrent access
	running        bool                      // Execution state
	lastConfig     Config                    // Last known config state
	lastConfigHash string                    // Hash of last config for change detection
	
	// Path and channel management
	pathMap        map[string]Paths          // Want path mapping
	channels       map[string]chain.Chan     // Inter-want channels
	channelMutex   sync.RWMutex             // Protect channel access
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
		configPath:     configPath,
		memoryPath:     memoryPath,
		wants:          make(map[string]*runtimeWant),
		registry:       make(map[string]WantFactory),
		reconcileStop:  make(chan bool),
		pathMap:        make(map[string]Paths),
		channels:       make(map[string]chain.Chan),
		running:        false,
		waitGroup:      &sync.WaitGroup{},
	}
	
	// Register built-in want types
	builder.registerBuiltinWantTypes()
	
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
func (cb *ChainBuilder) createWantFunction(want Want) interface{} {
	factory, exists := cb.registry[want.Metadata.Type]
	if !exists {
		panic(fmt.Sprintf("Unknown want type: %s", want.Metadata.Type))
	}
	return factory(want.Metadata, want.Spec)
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
func (cb *ChainBuilder) reconcileWants() {
	cb.reconcileMutex.Lock()
	defer cb.reconcileMutex.Unlock()
	
	// Load new config
	newConfig, err := cb.loadMemoryConfig()
	if err != nil {
		fmt.Printf("[RECONCILE] Failed to load memory config: %v\n", err)
		return
	}
	
	// Check if this is initial load (no lastConfig set)
	isInitialLoad := len(cb.lastConfig.Wants) == 0
	
	if isInitialLoad {
		fmt.Printf("[RECONCILE] Initial load: creating %d wants\n", len(newConfig.Wants))
		// For initial load, treat all wants as new additions
		for _, wantConfig := range newConfig.Wants {
			cb.addDynamicWantUnsafe(wantConfig)
		}
		// Rebuild connections after all wants are created
		cb.rebuildConnections()
	} else {
		// Detect changes for ongoing updates
		changes := cb.detectConfigChanges(cb.lastConfig, newConfig)
		if len(changes) == 0 {
			return
		}
		
		fmt.Printf("[RECONCILE] Applying %d changes\n", len(changes))
		
		// Apply changes in reverse dependency order (sink to generator)
		cb.applyChangesInReverseOrder(changes)
	}
	
	// Update last config and hash
	cb.lastConfig = newConfig
	cb.lastConfigHash, _ = cb.calculateFileHash(cb.memoryPath)
}

// detectConfigChanges compares configs and returns change events
func (cb *ChainBuilder) detectConfigChanges(oldConfig, newConfig Config) []ChangeEvent {
	var changes []ChangeEvent
	
	// Create maps for easier comparison
	oldWants := make(map[string]Want)
	for _, want := range oldConfig.Wants {
		oldWants[want.Metadata.Name] = want
	}
	
	newWants := make(map[string]Want)
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
					Want:     &newWant,
				})
			}
		} else {
			// New want
			changes = append(changes, ChangeEvent{
				Type:     ChangeEventAdd,
				WantName: name,
				Want:     &newWant,
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
func (cb *ChainBuilder) wantsEqual(a, b Want) bool {
	// Simple comparison - could be enhanced
	return a.Metadata.Type == b.Metadata.Type &&
		fmt.Sprintf("%v", a.Spec.Params) == fmt.Sprintf("%v", b.Spec.Params) &&
		fmt.Sprintf("%v", a.Spec.Using) == fmt.Sprintf("%v", b.Spec.Using)
}

// applyChangesInReverseOrder applies changes in sink-to-generator order
func (cb *ChainBuilder) applyChangesInReverseOrder(changes []ChangeEvent) {
	// Sort changes by dependency level (sink wants first)
	sortedChanges := cb.sortChangesByDependency(changes)
	
	for _, change := range sortedChanges {
		switch change.Type {
		case ChangeEventAdd:
			cb.addDynamicWantUnsafe(*change.Want)
		case ChangeEventUpdate:
			cb.updateWant(*change.Want)
		case ChangeEventDelete:
			cb.deleteWant(change.WantName)
		}
	}
	
	// Rebuild connections after all changes
	cb.rebuildConnections()
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
	var wantConfig Want
	if want, exists := cb.wants[wantName]; exists {
		wantConfig = *want.want
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
func (cb *ChainBuilder) addWant(wantConfig Want) {
	fmt.Printf("[RECONCILE] Adding want: %s\n", wantConfig.Metadata.Name)
	
	
	// Create the function/want
	wantFunction := cb.createWantFunction(wantConfig)
	
	var wantPtr *Want
	if wantWithGetWant, ok := wantFunction.(interface{ GetWant() *Want }); ok {
		wantPtr = wantWithGetWant.GetWant()
	} else {
		wantPtr = &Want{
			Metadata: wantConfig.Metadata,
			Spec:     wantConfig.Spec,
			Stats:    WantStats{},
			Status:   WantStatusIdle,
			State:    make(map[string]interface{}),
		}
	}
	
	runtimeWant := &runtimeWant{
		metadata: wantConfig.Metadata,
		spec:     wantConfig.Spec,
		function: wantFunction,
		want:     wantPtr,
	}
	cb.wants[wantConfig.Metadata.Name] = runtimeWant
}

// updateWant updates an existing want
func (cb *ChainBuilder) updateWant(wantConfig Want) {
	fmt.Printf("[RECONCILE] Updating want: %s\n", wantConfig.Metadata.Name)
	
	// For now, delete and recreate
	cb.deleteWant(wantConfig.Metadata.Name)
	cb.addDynamicWantUnsafe(wantConfig)
}

// deleteWant removes a want from runtime
func (cb *ChainBuilder) deleteWant(wantName string) {
	fmt.Printf("[RECONCILE] Deleting want: %s\n", wantName)
	
	delete(cb.wants, wantName)
}

// rebuildConnections rebuilds all connections after changes
func (cb *ChainBuilder) rebuildConnections() {
	fmt.Println("[RECONCILE] Rebuilding connections")
	
	// Generate new paths
	cb.pathMap = cb.generatePathsFromConnections()
	
	// Validate connectivity
	if err := cb.validateConnections(cb.pathMap); err != nil {
		fmt.Printf("[RECONCILE] Validation failed: %v\n", err)
		return
	}
	
	// Rebuild channels
	cb.channelMutex.Lock()
	cb.channels = make(map[string]chain.Chan)
	
	for _, paths := range cb.pathMap {
		for _, outputPath := range paths.Out {
			if outputPath.Active {
				channelKey := outputPath.Name
				cb.channels[channelKey] = make(chain.Chan, 10)
			}
		}
	}
	cb.channelMutex.Unlock()
	
	// Start new wants if system is running
	if cb.running {
		cb.startNewWants()
	}
}

// startNewWants starts newly added wants
func (cb *ChainBuilder) startNewWants() {
	for wantName, want := range cb.wants {
		if want.want.GetStatus() == WantStatusIdle {
			cb.startWant(wantName, want)
		}
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
	
	// Start want execution
	if chainWant, ok := want.function.(ChainWant); ok {
		generalizedFn := chainWant.CreateFunction()
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
				if generalizedFn(usingChans, outputChans) {
					fmt.Printf("[EXEC] Want %s finished\n", wantName)
					
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
		Wants: make([]Want, 0),
	}
	
	// First, add all wants from config and update with current stats
	configWantMap := make(map[string]bool)
	for _, want := range cb.config.Wants {
		configWantMap[want.Metadata.Name] = true
		if runtimeWant, exists := cb.wants[want.Metadata.Name]; exists {
			// Update with runtime data including spec using
			want.Spec = runtimeWant.spec  // Preserve using from runtime spec
			want.Stats = runtimeWant.want.Stats
			want.Status = runtimeWant.want.Status
			want.State = runtimeWant.want.State
		}
		updatedConfig.Wants = append(updatedConfig.Wants, want)
	}
	
	// Then, add any runtime wants that might not be in config (e.g., dynamically created and completed)
	for wantName, runtimeWant := range cb.wants {
		if !configWantMap[wantName] {
			// This want exists in runtime but not in config - include it
			wantConfig := Want{
				Metadata: runtimeWant.metadata,
				Spec:     runtimeWant.spec,
				Stats:    runtimeWant.want.Stats,
				Status:   runtimeWant.want.Status,
				State:    runtimeWant.want.State,
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
	cb.lastConfig = Config{Wants: []Want{}}
	
	// Mark as running
	cb.reconcileMutex.Lock()
	cb.running = true
	cb.reconcileMutex.Unlock()
	
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
func (cb *ChainBuilder) AddDynamicWants(wants []Want) {
	cb.reconcileMutex.Lock()
	defer cb.reconcileMutex.Unlock()
	for _, want := range wants {
		cb.addDynamicWantUnsafe(want)
	}
}


// addDynamicWantUnsafe adds a want without acquiring the mutex (internal use)
func (cb *ChainBuilder) addDynamicWantUnsafe(want Want) {
	// Add want to the configuration
	cb.config.Wants = append(cb.config.Wants, want)
	
	// Create runtime want if it doesn't exist
	if _, exists := cb.wants[want.Metadata.Name]; !exists {
		cb.addWant(want)
	}
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
	
	return config, nil
}

// validateConfigWithSpec validates YAML config data against the OpenAPI spec
func validateConfigWithSpec(yamlData []byte) error {
	// Load the OpenAPI spec
	loader := openapi3.NewLoader()
	spec, err := loader.LoadFromFile("spec.yaml")
	if err != nil {
		return fmt.Errorf("failed to load OpenAPI spec: %w", err)
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
	Timestamp   string `yaml:"timestamp"`
	ExecutionID string `yaml:"execution_id"`
	Wants       []Want `yaml:"wants"`
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
	wants := make([]Want, 0, len(cb.wants))
	for _, runtimeWant := range cb.wants {
		// Use runtime spec to preserve using, but want state for stats/status
		want := Want{
			Metadata: runtimeWant.metadata,
			Spec:     runtimeWant.spec,  // This preserves using
			Stats:    runtimeWant.want.Stats,
			Status:   runtimeWant.want.Status,
			State:    runtimeWant.want.State,
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
	
	fmt.Printf("📝 Want memory dumped to: %s\n", filename)
	return nil
}

// notifyParentTargetsOfChildCompletion checks if a completed want has owner references
// and notifies parent Target wants when all their children have completed
func (cb *ChainBuilder) notifyParentTargetsOfChildCompletion(completedWantName string) {
	// Find the config want for this completed want
	var completedWantConfig *Want
	for _, wantConfig := range cb.config.Wants {
		if wantConfig.Metadata.Name == completedWantName {
			completedWantConfig = &wantConfig
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
					// This is a child - check if it's completed
					if childRuntimeWant, exists := cb.wants[wantConfig.Metadata.Name]; exists {
						if childRuntimeWant.want.GetStatus() != WantStatusCompleted {
							allChildrenComplete = false
							break
						}
					}
				}
			}
			
			if allChildrenComplete {
				fmt.Printf("🎯 All children of target %s have completed, notifying...\n", parentName)
				notifier.NotifyChildrenComplete()
			}
		}
	}
}