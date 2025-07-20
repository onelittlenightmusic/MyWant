package main

import (
	"fmt"
	"gochain/chain"
	"os"
	"sync"
	"time"
	
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

// Metadata contains node identification and classification info
type Metadata struct {
	Name   string            `json:"name" yaml:"name"`
	Type   string            `json:"type" yaml:"type"`
	Labels map[string]string `json:"labels" yaml:"labels"`
}

// NodeSpec contains the desired state configuration for a node
type NodeSpec struct {
	Params map[string]interface{} `json:"params" yaml:"params"`
	Inputs []map[string]string    `json:"inputs,omitempty" yaml:"inputs,omitempty"`
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
type NodeFactory func(metadata Metadata, params map[string]interface{}) interface{}

// ChainBuilder builds and executes chains from declarative configuration
type ChainBuilder struct {
	config    Config
	nodes     map[string]*runtimeNode
	registry  map[string]NodeFactory
	waitGroup *sync.WaitGroup
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
	builder := &ChainBuilder{
		config:   config,
		nodes:    make(map[string]*runtimeNode),
		registry: make(map[string]NodeFactory),
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
			if meta.NodeType != "generator" && outCount < meta.RequiredOutputs {
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
	return factory(node.Metadata, node.Spec.Params)
}

// Build constructs the chain network from the configuration
func (cb *ChainBuilder) Build() error {
	// First pass: create all nodes
	for _, nodeConfig := range cb.config.Nodes {
		// Create the function/node and extract the Node from it
		nodeFunction := cb.createNodeFunction(nodeConfig)
		
		var nodePtr *Node
		// Try to extract the Node from the created function/object
		if nodeWithGetNode, ok := nodeFunction.(interface{ GetNode() *Node }); ok {
			nodePtr = nodeWithGetNode.GetNode()
		} else {
			// If we can't extract the Node, create a basic one
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

	// Generate paths from label-based connections
	pathMap := cb.generatePathsFromConnections()
	
	// Validate connectivity requirements
	err := cb.validateConnections(pathMap)
	if err != nil {
		return err
	}

	// Path-based chain building using validated connections
	// Create channel map for inter-node communication
	channels := make(map[string]chain.Chan)
	
	// Debug: Print pathMap
	fmt.Println("\n📍 Path Mapping:")
	for nodeName, paths := range pathMap {
		fmt.Printf("  %s:\n", nodeName)
		fmt.Printf("    Inputs: %v\n", paths.In)
		fmt.Printf("    Outputs: %v\n", paths.Out)
	}
	
	// Create channels for each connection in pathMap
	for _, paths := range pathMap {
		for _, outputPath := range paths.Out {
			if outputPath.Active {
				// Use the path name directly as the channel key
				channelKey := outputPath.Name
				channels[channelKey] = make(chain.Chan, 10)
				fmt.Printf("📡 Created channel: %s\n", channelKey)
			}
		}
	}
	
	// Build execution functions for each node with proper input/output channels
	nodeExecutors := make(map[string]func())
	
	for nodeName, node := range cb.nodes {
		paths := pathMap[nodeName]
		
		// Prepare input channels
		var inputChans []chain.Chan
		for _, inputPath := range paths.In {
			if inputPath.Active {
				// Use the path name directly as the channel key
				channelKey := inputPath.Name
				if ch, exists := channels[channelKey]; exists {
					inputChans = append(inputChans, ch)
					fmt.Printf("🔗 Found input channel for %s: %s\n", nodeName, channelKey)
				} else {
					fmt.Printf("❌ Missing input channel for %s: %s\n", nodeName, channelKey)
				}
			}
		}
		
		// Prepare output channels
		var outputChans []chain.Chan
		for _, outputPath := range paths.Out {
			if outputPath.Active {
				// Use the path name directly as the channel key
				channelKey := outputPath.Name
				if ch, exists := channels[channelKey]; exists {
					outputChans = append(outputChans, ch)
					fmt.Printf("🔗 Found output channel for %s: %s\n", nodeName, channelKey)
				} else {
					fmt.Printf("❌ Missing output channel for %s: %s\n", nodeName, channelKey)
				}
			}
		}
		
		// Create executor function for this node
		if chainNode, ok := node.function.(ChainNode); ok {
			generalizedFn := chainNode.CreateFunction()
			// Capture channels in closure to avoid variable scope issues
			capturedInputs := make([]chain.Chan, len(inputChans))
			capturedOutputs := make([]chain.Chan, len(outputChans))
			copy(capturedInputs, inputChans)
			copy(capturedOutputs, outputChans)
			
			nodeExecutors[nodeName] = func() {
				fmt.Printf("[EXEC] Starting node %s with %d inputs, %d outputs\n", 
					nodeName, len(capturedInputs), len(capturedOutputs))
				for {
					if generalizedFn(capturedInputs, capturedOutputs) {
						fmt.Printf("[EXEC] Node %s finished\n", nodeName)
						break
					}
				}
			}
		}
	}
	
	// Execute all nodes concurrently
	var wg sync.WaitGroup
	for nodeName, executor := range nodeExecutors {
		wg.Add(1)
		go func(name string, exec func()) {
			defer wg.Done()
			fmt.Printf("[STARTING] Node %s\n", name)
			exec()
			fmt.Printf("[FINISHED] Node %s\n", name)
		}(nodeName, executor)
	}
	
	// Store for cleanup
	cb.waitGroup = &wg

	return nil
}

// Execute runs the built chain network
func (cb *ChainBuilder) Execute() {
	// Set all nodes to running status before execution
	for _, node := range cb.nodes {
		node.node.SetStatus(NodeStatusRunning)
	}
	
	// Wait for all goroutines to complete if using path-based execution
	if cb.waitGroup != nil {
		cb.waitGroup.Wait()
	} else {
		// Fallback to legacy chain.Run() for backward compatibility
		chain.Run()
	}
	
	// After execution, mark all nodes as completed if they haven't failed
	for _, node := range cb.nodes {
		if node.node.GetStatus() == NodeStatusRunning {
			node.node.OnProcessEnd(map[string]interface{}{
				"execution_completed": true,
			})
		}
	}
	
	// Dump all node information to YAML file
	err := cb.dumpNodeMemoryToYAML()
	if err != nil {
		fmt.Printf("Warning: Failed to dump node memory to YAML: %v\n", err)
	}
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

// dumpNodeMemoryToYAML dumps all node information to a timestamped YAML file
func (cb *ChainBuilder) dumpNodeMemoryToYAML() error {
	// Create timestamp-based filename
	timestamp := time.Now().Format("20060102-150405")
	filename := fmt.Sprintf("memory-%s.yaml", timestamp)
	
	// Convert node map to slice to match config format
	nodeStates := cb.GetAllNodeStates()
	nodes := make([]Node, 0, len(nodeStates))
	for _, nodeState := range nodeStates {
		nodes = append(nodes, *nodeState)
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
	
	// Write to file
	err = os.WriteFile(filename, data, 0644)
	if err != nil {
		return fmt.Errorf("failed to write memory dump to file %s: %w", filename, err)
	}
	
	fmt.Printf("📝 Node memory dumped to: %s\n", filename)
	return nil
}