package main

import (
	"fmt"
	"gochain/chain"
	"os"
	"path/filepath"
	"sync"
	"time"
	"crypto/md5"
	"io"
	
	"gopkg.in/yaml.v3"
)

// ChainFunction represents a generalized chain function interface
type ChainFunction interface {
	CreateFunction() func(inputs []chain.Chan, outputs []chain.Chan) bool
}

// ChainNode represents a node that can create chain functions
type ChainNode interface {
	CreateFunction() func(inputs []chain.Chan, outputs []chain.Chan) bool
	GetNode() *Node
}

// createStartFunction converts generalized function to start function
func createStartFunction(generalizedFn func(inputs []chain.Chan, outputs []chain.Chan) bool) func(chain.Chan) bool {
	return func(out chain.Chan) bool {
		return generalizedFn([]chain.Chan{}, []chain.Chan{out})
	}
}

// createProcessFunction converts generalized function to process function
func createProcessFunction(generalizedFn func(inputs []chain.Chan, outputs []chain.Chan) bool) func(chain.Chan, chain.Chan) bool {
	return func(in chain.Chan, out chain.Chan) bool {
		return generalizedFn([]chain.Chan{in}, []chain.Chan{out})
	}
}

// createEndFunction converts generalized function to end function
func createEndFunction(generalizedFn func(inputs []chain.Chan, outputs []chain.Chan) bool) func(chain.Chan) bool {
	return func(in chain.Chan) bool {
		return generalizedFn([]chain.Chan{in}, []chain.Chan{})
	}
}

// isSinkNode checks if a node type is a sink node
func isSinkNode(nodeType string) bool {
	sinkTypes := []string{"collector", "sink", "prime_sink", "fibonacci_sink"}
	for _, sinkType := range sinkTypes {
		if nodeType == sinkType {
			return true
		}
	}
	return false
}

// OwnerReference represents a reference to an owner object
type OwnerReference struct {
	APIVersion          string `json:"apiVersion" yaml:"apiVersion"`
	Kind                string `json:"kind" yaml:"kind"`
	Name                string `json:"name" yaml:"name"`
	Controller          bool   `json:"controller,omitempty" yaml:"controller,omitempty"`
	BlockOwnerDeletion  bool   `json:"blockOwnerDeletion,omitempty" yaml:"blockOwnerDeletion,omitempty"`
}

// Metadata contains node identification and classification info
type Metadata struct {
	Name            string            `json:"name" yaml:"name"`
	Type            string            `json:"type" yaml:"type"`
	Labels          map[string]string `json:"labels" yaml:"labels"`
	OwnerReferences []OwnerReference  `json:"ownerReferences,omitempty" yaml:"ownerReferences,omitempty"`
}

// NodeSpec contains the desired state configuration for a node
type NodeSpec struct {
	Template string                 `json:"template,omitempty" yaml:"template,omitempty"`
	Params   map[string]interface{} `json:"params" yaml:"params"`
	Inputs   []map[string]string    `json:"inputs,omitempty" yaml:"inputs,omitempty"`
}

// Node represents a processing unit in the chain
type Node struct {
	Metadata Metadata               `json:"metadata" yaml:"metadata"`
	Spec     NodeSpec               `json:"spec" yaml:"spec"`
	Stats    NodeStats              `json:"stats,omitempty" yaml:"stats,omitempty"`
	Status   NodeStatus             `json:"status,omitempty" yaml:"status,omitempty"`
	State    map[string]interface{} `json:"state,omitempty" yaml:"state,omitempty"`
}

// SetStatus updates the node's status
func (n *Node) SetStatus(status NodeStatus) {
	n.Status = status
}

// GetStatus returns the current node status
func (n *Node) GetStatus() NodeStatus {
	return n.Status
}

// StoreState stores a key-value pair in the node's state
func (n *Node) StoreState(key string, value interface{}) {
	if n.State == nil {
		n.State = make(map[string]interface{})
	}
	n.State[key] = value
}

// GetState retrieves a value from the node's state
func (n *Node) GetState(key string) (interface{}, bool) {
	if n.State == nil {
		return nil, false
	}
	value, exists := n.State[key]
	return value, exists
}

// OnProcessEnd handles state storage when the node process ends
func (n *Node) OnProcessEnd(finalState map[string]interface{}) {
	n.SetStatus(NodeStatusCompleted)
	
	// Store final state
	for key, value := range finalState {
		n.StoreState(key, value)
	}
	
	// Store completion timestamp
	n.StoreState("completion_time", fmt.Sprintf("%d", getCurrentTimestamp()))
	
	// Store final statistics
	n.StoreState("final_stats", n.Stats)
}

// OnProcessFail handles state storage when the node process fails
func (n *Node) OnProcessFail(errorState map[string]interface{}, err error) {
	n.SetStatus(NodeStatusFailed)
	
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
	Nodes []Node `json:"nodes" yaml:"nodes"`
}

// PathInfo represents connection information for a single path
type PathInfo struct {
	Channel chan interface{}
	Name    string
	Active  bool
}

// Paths manages all input and output connections for a node
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

// ConnectivityMetadata defines node connectivity requirements and constraints
type ConnectivityMetadata struct {
	RequiredInputs  int
	RequiredOutputs int
	MaxInputs       int    // -1 for unlimited
	MaxOutputs      int    // -1 for unlimited
	NodeType        string
	Description     string
}

// EnhancedBaseNode interface for path-aware nodes with connectivity validation
type EnhancedBaseNode interface {
	InitializePaths(inCount, outCount int)
	GetConnectivityMetadata() ConnectivityMetadata
	GetStats() map[string]interface{}
	Process(paths Paths) bool
	GetType() string
}

// NodeStats holds statistical information for a node
type NodeStats struct {
	AverageWaitTime float64 `json:"average_wait_time"`
	TotalProcessed  int     `json:"total_processed"`
	TotalWaitTime   float64 `json:"total_wait_time"`
}

// NodeStatus represents the current state of a node
type NodeStatus string

const (
	NodeStatusIdle       NodeStatus = "idle"
	NodeStatusRunning    NodeStatus = "running"
	NodeStatusCompleted  NodeStatus = "completed"
	NodeStatusFailed     NodeStatus = "failed"
	NodeStatusTerminated NodeStatus = "terminated"
)


// getCurrentTimestamp returns current Unix timestamp
func getCurrentTimestamp() int64 {
	return time.Now().Unix()
}

// NodeFactory defines the interface for creating node functions
type NodeFactory func(metadata Metadata, spec NodeSpec) interface{}

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
	NodeName string
	Node     *Node
}

// ParentNotifier interface for nodes that can receive child completion notifications
type ParentNotifier interface {
	NotifyChildrenComplete()
}

// ChainBuilder builds and executes chains from declarative configuration with reconcile loop
type ChainBuilder struct {
	configPath     string                    // Path to original config file
	memoryPath     string                    // Path to memory file (watched for changes)
	nodes          map[string]*runtimeNode   // Runtime node registry
	registry       map[string]NodeFactory    // Node type factories
	waitGroup      *sync.WaitGroup           // Execution synchronization
	config         Config                    // Current configuration
	
	// Reconcile loop fields
	reconcileStop  chan bool                 // Stop signal for reconcile loop
	reconcileMutex sync.RWMutex             // Protect concurrent access
	running        bool                      // Execution state
	lastConfig     Config                    // Last known config state
	lastConfigHash string                    // Hash of last config for change detection
	
	// Path and channel management
	pathMap        map[string]Paths          // Node path mapping
	channels       map[string]chain.Chan     // Inter-node channels
	channelMutex   sync.RWMutex             // Protect channel access
}

// runtimeNode holds the runtime state of a node
type runtimeNode struct {
	metadata Metadata
	spec     NodeSpec
	chain    chain.C_chain
	function interface{}
	node     *Node
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
		nodes:          make(map[string]*runtimeNode),
		registry:       make(map[string]NodeFactory),
		reconcileStop:  make(chan bool),
		pathMap:        make(map[string]Paths),
		channels:       make(map[string]chain.Chan),
		running:        false,
		waitGroup:      &sync.WaitGroup{},
	}
	
	// Register built-in node types
	builder.registerBuiltinNodeTypes()
	
	return builder
}

// registerBuiltinNodeTypes registers the default node type factories
func (cb *ChainBuilder) registerBuiltinNodeTypes() {
	// No built-in types by default - they should be registered by domain-specific modules
}

// RegisterNodeType allows registering custom node types
func (cb *ChainBuilder) RegisterNodeType(nodeType string, factory NodeFactory) {
	cb.registry[nodeType] = factory
}

// matchesSelector checks if node labels match the selector criteria
func (cb *ChainBuilder) matchesSelector(nodeLabels map[string]string, selector map[string]string) bool {
	for key, value := range selector {
		if nodeLabels[key] != value {
			return false
		}
	}
	return true
}


// generatePathsFromConnections creates paths based on labels and inputs, eliminating output requirements
func (cb *ChainBuilder) generatePathsFromConnections() map[string]Paths {
	pathMap := make(map[string]Paths)
	
	// Initialize empty paths for all nodes
	for nodeName := range cb.nodes {
		pathMap[nodeName] = Paths{
			In:  []PathInfo{},
			Out: []PathInfo{},
		}
	}
	
	// Create connections based on input selectors
	for nodeName, node := range cb.nodes {
		paths := pathMap[nodeName]
		
		// Process input connections for this node
		for _, inputSelector := range node.spec.Inputs {
			// Find nodes that match this input selector
			for otherName, otherNode := range cb.nodes {
				if cb.matchesSelector(otherNode.metadata.Labels, inputSelector) {
					// Create input path for current node
					inPath := PathInfo{
						Channel: make(chan interface{}, 10),
						Name:    fmt.Sprintf("%s_to_%s", otherName, nodeName),
						Active:  true,
					}
					paths.In = append(paths.In, inPath)
					
					// Create corresponding output path for the matching node
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
		pathMap[nodeName] = paths
	}
	
	return pathMap
}

// validateConnections validates that all nodes have their connectivity requirements satisfied
func (cb *ChainBuilder) validateConnections(pathMap map[string]Paths) error {
	for nodeName, node := range cb.nodes {
		paths := pathMap[nodeName]
		
		// Check if this is an enhanced node that has connectivity requirements
		if enhancedNode, ok := node.function.(EnhancedBaseNode); ok {
			meta := enhancedNode.GetConnectivityMetadata()
			
			inCount := len(paths.In)
			outCount := len(paths.Out)
			
			// Check required inputs
			if inCount < meta.RequiredInputs {
				return fmt.Errorf("validation failed for node %s: node %s requires %d inputs, got %d", 
					nodeName, meta.NodeType, meta.RequiredInputs, inCount)
			}
			
			// Check required outputs - modified to not require outputs for generators
			if meta.NodeType != "sequence" && outCount < meta.RequiredOutputs {
				return fmt.Errorf("validation failed for node %s: node %s requires %d outputs, got %d", 
					nodeName, meta.NodeType, meta.RequiredOutputs, outCount)
			}
			
			// Check maximum inputs
			if meta.MaxInputs >= 0 && inCount > meta.MaxInputs {
				return fmt.Errorf("validation failed for node %s: node %s allows max %d inputs, got %d", 
					nodeName, meta.NodeType, meta.MaxInputs, inCount)
			}
			
			// Check maximum outputs
			if meta.MaxOutputs >= 0 && outCount > meta.MaxOutputs {
				return fmt.Errorf("validation failed for node %s: node %s allows max %d outputs, got %d", 
					nodeName, meta.NodeType, meta.MaxOutputs, outCount)
			}
		}
	}
	return nil
}

// createNodeFunction creates the appropriate function based on node type using registry
func (cb *ChainBuilder) createNodeFunction(node Node) interface{} {
	factory, exists := cb.registry[node.Metadata.Type]
	if !exists {
		panic(fmt.Sprintf("Unknown node type: %s", node.Metadata.Type))
	}
	return factory(node.Metadata, node.Spec)
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
	cb.reconcileNodes()
	
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
				cb.reconcileNodes()
			}
		case <-statsTicker.C:
			cb.writeStatsToMemory()
		}
	}
}

// reconcileNodes performs reconciliation when config changes or during initial load
func (cb *ChainBuilder) reconcileNodes() {
	cb.reconcileMutex.Lock()
	defer cb.reconcileMutex.Unlock()
	
	// Load new config
	newConfig, err := cb.loadMemoryConfig()
	if err != nil {
		fmt.Printf("[RECONCILE] Failed to load memory config: %v\n", err)
		return
	}
	
	// Check if this is initial load (no lastConfig set)
	isInitialLoad := len(cb.lastConfig.Nodes) == 0
	
	if isInitialLoad {
		fmt.Printf("[RECONCILE] Initial load: creating %d nodes\n", len(newConfig.Nodes))
		// For initial load, treat all nodes as new additions
		for _, nodeConfig := range newConfig.Nodes {
			cb.addDynamicNodeUnsafe(nodeConfig)
		}
		// Rebuild connections after all nodes are created
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
	oldNodes := make(map[string]Node)
	for _, node := range oldConfig.Nodes {
		oldNodes[node.Metadata.Name] = node
	}
	
	newNodes := make(map[string]Node)
	for _, node := range newConfig.Nodes {
		newNodes[node.Metadata.Name] = node
	}
	
	// Find additions and updates
	for name, newNode := range newNodes {
		if oldNode, exists := oldNodes[name]; exists {
			// Check if node changed
			if !cb.nodesEqual(oldNode, newNode) {
				changes = append(changes, ChangeEvent{
					Type:     ChangeEventUpdate,
					NodeName: name,
					Node:     &newNode,
				})
			}
		} else {
			// New node
			changes = append(changes, ChangeEvent{
				Type:     ChangeEventAdd,
				NodeName: name,
				Node:     &newNode,
			})
		}
	}
	
	// Find deletions
	for name := range oldNodes {
		if _, exists := newNodes[name]; !exists {
			changes = append(changes, ChangeEvent{
				Type:     ChangeEventDelete,
				NodeName: name,
				Node:     nil,
			})
		}
	}
	
	return changes
}

// nodesEqual compares two nodes for equality
func (cb *ChainBuilder) nodesEqual(a, b Node) bool {
	// Simple comparison - could be enhanced
	return a.Metadata.Type == b.Metadata.Type &&
		fmt.Sprintf("%v", a.Spec.Params) == fmt.Sprintf("%v", b.Spec.Params) &&
		fmt.Sprintf("%v", a.Spec.Inputs) == fmt.Sprintf("%v", b.Spec.Inputs)
}

// applyChangesInReverseOrder applies changes in sink-to-generator order
func (cb *ChainBuilder) applyChangesInReverseOrder(changes []ChangeEvent) {
	// Sort changes by dependency level (sink nodes first)
	sortedChanges := cb.sortChangesByDependency(changes)
	
	for _, change := range sortedChanges {
		switch change.Type {
		case ChangeEventAdd:
			cb.addDynamicNodeUnsafe(*change.Node)
		case ChangeEventUpdate:
			cb.updateNode(*change.Node)
		case ChangeEventDelete:
			cb.deleteNode(change.NodeName)
		}
	}
	
	// Rebuild connections after all changes
	cb.rebuildConnections()
}

// sortChangesByDependency sorts changes by dependency level
func (cb *ChainBuilder) sortChangesByDependency(changes []ChangeEvent) []ChangeEvent {
	// Calculate dependency levels for all nodes
	depLevels := cb.calculateDependencyLevels()
	
	// Sort changes by dependency level (higher level = closer to sink)
	sortedChanges := make([]ChangeEvent, len(changes))
	copy(sortedChanges, changes)
	
	// Simple sort by dependency level
	for i := 0; i < len(sortedChanges)-1; i++ {
		for j := i + 1; j < len(sortedChanges); j++ {
			levelI := depLevels[sortedChanges[i].NodeName]
			levelJ := depLevels[sortedChanges[j].NodeName]
			if levelI < levelJ {
				sortedChanges[i], sortedChanges[j] = sortedChanges[j], sortedChanges[i]
			}
		}
	}
	
	return sortedChanges
}

// calculateDependencyLevels calculates dependency levels for nodes
func (cb *ChainBuilder) calculateDependencyLevels() map[string]int {
	levels := make(map[string]int)
	
	// Simple heuristic: sinks have highest level, generators have lowest
	for name, node := range cb.nodes {
		if isSinkNode(node.metadata.Type) {
			levels[name] = 100
		} else if node.metadata.Type == "sequence" {
			levels[name] = 1
		} else {
			levels[name] = 50 // Middle nodes
		}
	}
	
	return levels
}

// addNode adds a new node to the runtime (private method)
func (cb *ChainBuilder) addNode(nodeConfig Node) {
	fmt.Printf("[RECONCILE] Adding node: %s\n", nodeConfig.Metadata.Name)
	
	
	// Create the function/node
	nodeFunction := cb.createNodeFunction(nodeConfig)
	
	var nodePtr *Node
	if nodeWithGetNode, ok := nodeFunction.(interface{ GetNode() *Node }); ok {
		nodePtr = nodeWithGetNode.GetNode()
	} else {
		nodePtr = &Node{
			Metadata: nodeConfig.Metadata,
			Spec:     nodeConfig.Spec,
			Stats:    NodeStats{},
			Status:   NodeStatusIdle,
			State:    make(map[string]interface{}),
		}
	}
	
	runtimeNode := &runtimeNode{
		metadata: nodeConfig.Metadata,
		spec:     nodeConfig.Spec,
		function: nodeFunction,
		node:     nodePtr,
	}
	cb.nodes[nodeConfig.Metadata.Name] = runtimeNode
}

// updateNode updates an existing node
func (cb *ChainBuilder) updateNode(nodeConfig Node) {
	fmt.Printf("[RECONCILE] Updating node: %s\n", nodeConfig.Metadata.Name)
	
	// For now, delete and recreate
	cb.deleteNode(nodeConfig.Metadata.Name)
	cb.addDynamicNodeUnsafe(nodeConfig)
}

// deleteNode removes a node from runtime
func (cb *ChainBuilder) deleteNode(nodeName string) {
	fmt.Printf("[RECONCILE] Deleting node: %s\n", nodeName)
	
	delete(cb.nodes, nodeName)
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
	
	// Start new nodes if system is running
	if cb.running {
		cb.startNewNodes()
	}
}

// startNewNodes starts newly added nodes
func (cb *ChainBuilder) startNewNodes() {
	for nodeName, node := range cb.nodes {
		if node.node.GetStatus() == NodeStatusIdle {
			cb.startNode(nodeName, node)
		}
	}
}

// startNode starts a single node
func (cb *ChainBuilder) startNode(nodeName string, node *runtimeNode) {
	// Check if node is already running or completed to prevent duplicate starts
	if node.node.GetStatus() == NodeStatusRunning || node.node.GetStatus() == NodeStatusCompleted {
		return
	}
	
	paths := cb.pathMap[nodeName]
	
	// Prepare input channels
	var inputChans []chain.Chan
	for _, inputPath := range paths.In {
		if inputPath.Active {
			channelKey := inputPath.Name
			cb.channelMutex.RLock()
			if ch, exists := cb.channels[channelKey]; exists {
				inputChans = append(inputChans, ch)
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
	
	// Start node execution
	if chainNode, ok := node.function.(ChainNode); ok {
		generalizedFn := chainNode.CreateFunction()
		node.node.SetStatus(NodeStatusRunning)
		
		cb.waitGroup.Add(1)
		go func() {
			defer cb.waitGroup.Done()
			defer func() {
				if node.node.GetStatus() == NodeStatusRunning {
					node.node.SetStatus(NodeStatusCompleted)
				}
			}()
			
			fmt.Printf("[EXEC] Starting node %s with %d inputs, %d outputs\n", 
				nodeName, len(inputChans), len(outputChans))
			
			for {
				if generalizedFn(inputChans, outputChans) {
					fmt.Printf("[EXEC] Node %s finished\n", nodeName)
					
					// Check if this node completion should notify parent targets
					cb.notifyParentTargetsOfChildCompletion(nodeName)
					
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
	
	// Create a comprehensive config that includes ALL nodes (both config and runtime)
	updatedConfig := Config{
		Nodes: make([]Node, 0),
	}
	
	// First, add all nodes from config and update with current stats
	configNodeMap := make(map[string]bool)
	for _, node := range cb.config.Nodes {
		configNodeMap[node.Metadata.Name] = true
		if runtimeNode, exists := cb.nodes[node.Metadata.Name]; exists {
			// Update with runtime data including spec inputs
			node.Spec = runtimeNode.spec  // Preserve inputs from runtime spec
			node.Stats = runtimeNode.node.Stats
			node.Status = runtimeNode.node.Status
			node.State = runtimeNode.node.State
		}
		updatedConfig.Nodes = append(updatedConfig.Nodes, node)
	}
	
	// Then, add any runtime nodes that might not be in config (e.g., dynamically created and completed)
	for nodeName, runtimeNode := range cb.nodes {
		if !configNodeMap[nodeName] {
			// This node exists in runtime but not in config - include it
			nodeConfig := Node{
				Metadata: runtimeNode.metadata,
				Spec:     runtimeNode.spec,
				Stats:    runtimeNode.node.Stats,
				Status:   runtimeNode.node.Status,
				State:    runtimeNode.node.State,
			}
			updatedConfig.Nodes = append(updatedConfig.Nodes, nodeConfig)
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

// Execute starts the reconcile loop and initial node execution
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
	cb.lastConfig = Config{Nodes: []Node{}}
	
	// Mark as running
	cb.reconcileMutex.Lock()
	cb.running = true
	cb.reconcileMutex.Unlock()
	
	// Start reconcile loop in background - it will handle initial node creation
	go cb.reconcileLoop()
	
	// Wait for initial nodes to be created by reconcileLoop
	for {
		cb.reconcileMutex.Lock()
		nodeCount := len(cb.nodes)
		cb.reconcileMutex.Unlock()
		
		if nodeCount > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	
	// Start all initial nodes
	for nodeName, node := range cb.nodes {
		cb.startNode(nodeName, node)
	}
	
	// Wait for all nodes to complete
	cb.waitGroup.Wait()
	
	// Stop reconcile loop
	cb.reconcileStop <- true
	
	// Mark as not running
	cb.reconcileMutex.Lock()
	cb.running = false
	cb.reconcileMutex.Unlock()
	
	// Final memory dump - ensure it completes before returning
	fmt.Println("[RECONCILE] Writing final memory dump...")
	err := cb.dumpNodeMemoryToYAML()
	if err != nil {
		fmt.Printf("Warning: Failed to dump node memory to YAML: %v\n", err)
	} else {
		fmt.Println("[RECONCILE] Memory dump completed successfully")
	}
	
	
	fmt.Println("[RECONCILE] Execution completed")
}


// GetNodeState returns the state of a specific node
func (cb *ChainBuilder) GetNodeState(nodeName string) (*Node, bool) {
	node, exists := cb.nodes[nodeName]
	if !exists {
		return nil, false
	}
	return node.node, true
}

// GetAllNodeStates returns the states of all nodes
func (cb *ChainBuilder) GetAllNodeStates() map[string]*Node {
	states := make(map[string]*Node)
	for name, node := range cb.nodes {
		states[name] = node.node
	}
	return states
}

// SetMemoryPath sets the memory file path for snapshots
func (cb *ChainBuilder) SetMemoryPath(memoryPath string) {
	cb.memoryPath = memoryPath
}

// AddDynamicNodes adds multiple nodes to the configuration at runtime
func (cb *ChainBuilder) AddDynamicNodes(nodes []Node) {
	cb.reconcileMutex.Lock()
	defer cb.reconcileMutex.Unlock()
	for _, node := range nodes {
		cb.addDynamicNodeUnsafe(node)
	}
}

// AddDynamicNode adds a single node to the configuration at runtime
func (cb *ChainBuilder) AddDynamicNode(node Node) {
	cb.reconcileMutex.Lock()
	defer cb.reconcileMutex.Unlock()
	cb.addDynamicNodeUnsafe(node)
}

// addDynamicNodeUnsafe adds a node without acquiring the mutex (internal use)
func (cb *ChainBuilder) addDynamicNodeUnsafe(node Node) {
	// Add node to the configuration
	cb.config.Nodes = append(cb.config.Nodes, node)
	
	// Create runtime node if it doesn't exist
	if _, exists := cb.nodes[node.Metadata.Name]; !exists {
		cb.addNode(node)
	}
}


// loadConfigFromYAML loads configuration from a YAML file
func loadConfigFromYAML(filename string) (Config, error) {
	var config Config
	
	data, err := os.ReadFile(filename)
	if err != nil {
		return config, fmt.Errorf("failed to read YAML file: %w", err)
	}
	
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return config, fmt.Errorf("failed to parse YAML config: %w", err)
	}
	
	
	return config, nil
}

// NodeMemoryDump represents the complete state of all nodes for dumping
type NodeMemoryDump struct {
	Timestamp   string `yaml:"timestamp"`
	ExecutionID string `yaml:"execution_id"`
	Nodes       []Node `yaml:"nodes"`
}

// dumpNodeMemoryToYAML dumps all node information to a timestamped YAML file in memory directory
func (cb *ChainBuilder) dumpNodeMemoryToYAML() error {
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
	
	// Convert node map to slice to match config format, preserving runtime spec
	nodes := make([]Node, 0, len(cb.nodes))
	for _, runtimeNode := range cb.nodes {
		// Use runtime spec to preserve inputs, but node state for stats/status
		node := Node{
			Metadata: runtimeNode.metadata,
			Spec:     runtimeNode.spec,  // This preserves inputs
			Stats:    runtimeNode.node.Stats,
			Status:   runtimeNode.node.Status,
			State:    runtimeNode.node.State,
		}
		nodes = append(nodes, node)
	}
	
	// Prepare memory dump structure
	memoryDump := NodeMemoryDump{
		Timestamp:   time.Now().Format(time.RFC3339),
		ExecutionID: fmt.Sprintf("exec-%s", timestamp),
		Nodes:       nodes,
	}
	
	// Marshal to YAML
	data, err := yaml.Marshal(memoryDump)
	if err != nil {
		return fmt.Errorf("failed to marshal node memory to YAML: %w", err)
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
	
	fmt.Printf("📝 Node memory dumped to: %s\n", filename)
	return nil
}

// notifyParentTargetsOfChildCompletion checks if a completed node has owner references
// and notifies parent Target nodes when all their children have completed
func (cb *ChainBuilder) notifyParentTargetsOfChildCompletion(completedNodeName string) {
	// Find the config node for this completed node
	var completedNodeConfig *Node
	for _, nodeConfig := range cb.config.Nodes {
		if nodeConfig.Metadata.Name == completedNodeName {
			completedNodeConfig = &nodeConfig
			break
		}
	}
	
	if completedNodeConfig == nil || len(completedNodeConfig.Metadata.OwnerReferences) == 0 {
		return // No owner references
	}
	
	// For each owner reference, check if all siblings are complete
	for _, ownerRef := range completedNodeConfig.Metadata.OwnerReferences {
		parentName := ownerRef.Name
		
		// Check if parent is a Target node
		parentRuntimeNode, exists := cb.nodes[parentName]
		if !exists {
			continue
		}
		
		// Check if it implements child completion notification
		if notifier, ok := parentRuntimeNode.function.(ParentNotifier); ok {
			// Find all child nodes with this parent
			allChildrenComplete := true
			for _, nodeConfig := range cb.config.Nodes {
				if nodeConfig.Metadata.Name == parentName {
					continue // Skip the parent itself
				}
				
				// Check if this node has ownerRef to this parent
				hasOwnerRef := false
				for _, childOwnerRef := range nodeConfig.Metadata.OwnerReferences {
					if childOwnerRef.Name == parentName {
						hasOwnerRef = true
						break
					}
				}
				
				if hasOwnerRef {
					// This is a child - check if it's completed
					if childRuntimeNode, exists := cb.nodes[nodeConfig.Metadata.Name]; exists {
						if childRuntimeNode.node.GetStatus() != NodeStatusCompleted {
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