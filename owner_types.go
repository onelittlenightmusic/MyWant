package main

import (
	"fmt"
	"gochain/chain"
	"sync"
)

// Global registry to track target instances for child notification
var targetRegistry = make(map[string]*Target)
var targetRegistryMutex sync.RWMutex

// Target represents a parent node that creates and manages child nodes
type Target struct {
	Node
	MaxDisplay int
	paths      Paths
	childNodes []Node
	childrenDone chan bool
	builder    *ChainBuilder // Reference to builder for dynamic node creation
}

// NewTarget creates a new target node
func NewTarget(metadata Metadata, params map[string]interface{}) *Target {
	target := &Target{
		Node: Node{
			Metadata: metadata,
			Spec:     NodeSpec{Params: params},
			Stats:    NodeStats{},
			Status:   NodeStatusIdle,
			State:    make(map[string]interface{}),
		},
		MaxDisplay: 1000,
		childNodes: make([]Node, 0),
		childrenDone: make(chan bool, 1),
	}
	
	if max, ok := params["max_display"]; ok {
		if maxInt, ok := max.(int); ok {
			target.MaxDisplay = maxInt
		} else if maxFloat, ok := max.(float64); ok {
			target.MaxDisplay = int(maxFloat)
		}
	}
	
	// Register the target instance for child notification
	targetRegistryMutex.Lock()
	targetRegistry[metadata.Name] = target
	targetRegistryMutex.Unlock()
	
	return target
}

// SetBuilder sets the ChainBuilder reference for dynamic node creation
func (t *Target) SetBuilder(builder *ChainBuilder) {
	t.builder = builder
}

// CreateChildNodes dynamically creates child nodes based on the target configuration
func (t *Target) CreateChildNodes() []Node {
	// Create generator node
	generatorNode := Node{
		Metadata: Metadata{
			Name: "number-generator",
			Type: "sequence",
			Labels: map[string]string{
				"role":     "source",
				"owner":    "child",
				"category": "producer",
			},
			OwnerReferences: []OwnerReference{
				{
					APIVersion:         "gochain/v1",
					Kind:               "Node",
					Name:               t.Metadata.Name,
					Controller:         true,
					BlockOwnerDeletion: true,
				},
			},
		},
		Spec: NodeSpec{
			Params: map[string]interface{}{
				"count": t.MaxDisplay,
				"rate":  10.0,
			},
			// Generator is a source node - no inputs needed
		},
	}

	// Create queue node
	queueNode := Node{
		Metadata: Metadata{
			Name: "number-queue",
			Type: "queue",
			Labels: map[string]string{
				"role":     "processor",
				"owner":    "child",
				"category": "queue",
			},
			OwnerReferences: []OwnerReference{
				{
					APIVersion:         "gochain/v1",
					Kind:               "Node",
					Name:               t.Metadata.Name,
					Controller:         true,
					BlockOwnerDeletion: true,
				},
			},
		},
		Spec: NodeSpec{
			Params: map[string]interface{}{
				"service_time": 0.1,
			},
			Inputs: []map[string]string{
				{"category": "producer"},
			},
		},
	}

	// Create sink node
	sinkNode := Node{
		Metadata: Metadata{
			Name: "number-sink",
			Type: "sink",
			Labels: map[string]string{
				"role":     "collector",
				"category": "display",
			},
			OwnerReferences: []OwnerReference{
				{
					APIVersion:         "gochain/v1",
					Kind:               "Node",
					Name:               t.Metadata.Name,
					Controller:         true,
					BlockOwnerDeletion: true,
				},
			},
		},
		Spec: NodeSpec{
			Params: map[string]interface{}{
				"display_format": "Number: %d",
			},
			Inputs: []map[string]string{
				{"role": "processor"}, // Gets input from queue
			},
		},
	}

	t.childNodes = []Node{generatorNode, queueNode, sinkNode}
	return t.childNodes
}

// CreateFunction implements the ChainNode interface for Target
func (t *Target) CreateFunction() func(inputs []chain.Chan, outputs []chain.Chan) bool {
	return func(inputs []chain.Chan, outputs []chain.Chan) bool {
		fmt.Printf("🎯 Target %s: Managing child nodes with owner references\n", t.Metadata.Name)
		
		// Dynamically create child nodes
		if t.builder != nil {
			fmt.Printf("🎯 Target %s: Creating child nodes dynamically...\n", t.Metadata.Name)
			childNodes := t.CreateChildNodes()
			
			// Add child nodes to the builder's configuration
			for _, childNode := range childNodes {
				fmt.Printf("🔧 Adding child node: %s (type: %s)\n", childNode.Metadata.Name, childNode.Metadata.Type)
			}
			t.builder.AddDynamicNodes(childNodes)
			
			// Rebuild connections to include new nodes
			fmt.Printf("🔧 Rebuilding connections with dynamic nodes...\n")
			t.builder.rebuildConnections()
		}
		
		// Target waits for signal that all children have finished
		fmt.Printf("🎯 Target %s: Waiting for all child nodes to complete...\n", t.Metadata.Name)
		<-t.childrenDone
		fmt.Printf("🎯 Target %s: All child nodes completed, target finishing\n", t.Metadata.Name)
		
		return true
	}
}

// GetNode returns the underlying node
func (t *Target) GetNode() *Node {
	return &t.Node
}

// NotifyChildrenComplete signals that all child nodes have completed
func (t *Target) NotifyChildrenComplete() {
	select {
	case t.childrenDone <- true:
		// Signal sent successfully
	default:
		// Channel already has signal, ignore
	}
}

// addChildNodesToMemory adds child nodes to the memory configuration
func (t *Target) addChildNodesToMemory() error {
	// This is a placeholder - in a real implementation, this would
	// interact with the ChainBuilder to add nodes to the memory file
	// For now, we'll assume the reconcile loop will pick up the nodes
	fmt.Printf("🔧 Adding %d child nodes to memory configuration\n", len(t.childNodes))
	return nil
}

// NotifyTargetCompletion notifies a target that its child has completed
func NotifyTargetCompletion(targetName string, childName string) {
	targetRegistryMutex.RLock()
	target, exists := targetRegistry[targetName]
	targetRegistryMutex.RUnlock()
	
	if exists {
		fmt.Printf("📢 Child %s notifying target %s of completion\n", childName, targetName)
		target.NotifyChildrenComplete()
	}
}

// OwnerAwareNode wraps any node type to add parent notification capability
type OwnerAwareNode struct {
	BaseNode   interface{} // The original node (Generator, Queue, Sink, etc.)
	TargetName string
	NodeName   string
}

// NewOwnerAwareNode creates a wrapper that adds parent notification to any node
func NewOwnerAwareNode(baseNode interface{}, metadata Metadata) *OwnerAwareNode {
	// Find target name from owner references
	targetName := ""
	for _, ownerRef := range metadata.OwnerReferences {
		if ownerRef.Controller && ownerRef.Kind == "Node" {
			targetName = ownerRef.Name
			break
		}
	}
	
	return &OwnerAwareNode{
		BaseNode:   baseNode,
		TargetName: targetName,
		NodeName:   metadata.Name,
	}
}

// CreateFunction wraps the base node's function to add completion notification
func (oan *OwnerAwareNode) CreateFunction() func(inputs []chain.Chan, outputs []chain.Chan) bool {
	// Get the original function from the base node
	var originalFunc func(inputs []chain.Chan, outputs []chain.Chan) bool
	
	if chainNode, ok := oan.BaseNode.(ChainNode); ok {
		originalFunc = chainNode.CreateFunction()
	} else {
		// Fallback for non-ChainNode types
		return func(inputs []chain.Chan, outputs []chain.Chan) bool {
			fmt.Printf("⚠️  Node %s: No CreateFunction available\n", oan.NodeName)
			return true
		}
	}
	
	return func(inputs []chain.Chan, outputs []chain.Chan) bool {
		// Call the original function
		result := originalFunc(inputs, outputs)
		
		// If node completed successfully and we have a target, notify it
		if result && oan.TargetName != "" {
			fmt.Printf("💬 Child %s completed, notifying target %s\n", oan.NodeName, oan.TargetName)
			NotifyTargetCompletion(oan.TargetName, oan.NodeName)
		}
		
		return result
	}
}

// GetNode returns the underlying node from the base node
func (oan *OwnerAwareNode) GetNode() *Node {
	if chainNode, ok := oan.BaseNode.(ChainNode); ok {
		return chainNode.GetNode()
	}
	return nil
}

// RegisterOwnerNodeTypes registers the owner-based node types with a ChainBuilder
func RegisterOwnerNodeTypes(builder *ChainBuilder) {
	// Register existing qnet types first
	RegisterQNetNodeTypes(builder)
	
	// Register target type
	builder.RegisterNodeType("target", func(metadata Metadata, params map[string]interface{}) interface{} {
		target := NewTarget(metadata, params)
		target.SetBuilder(builder) // Set builder reference for dynamic node creation
		return target
	})
	
	// Override all node types to use OwnerAwareNode wrapper for nodes with owner references
	originalGeneratorFactory := builder.registry["sequence"]
	builder.RegisterNodeType("sequence", func(metadata Metadata, params map[string]interface{}) interface{} {
		baseNode := originalGeneratorFactory(metadata, params)
		if len(metadata.OwnerReferences) > 0 {
			return NewOwnerAwareNode(baseNode, metadata)
		}
		return baseNode
	})
	
	originalQueueFactory := builder.registry["queue"]
	builder.RegisterNodeType("queue", func(metadata Metadata, params map[string]interface{}) interface{} {
		baseNode := originalQueueFactory(metadata, params)
		if len(metadata.OwnerReferences) > 0 {
			return NewOwnerAwareNode(baseNode, metadata)
		}
		return baseNode
	})
	
	originalSinkFactory := builder.registry["sink"]
	builder.RegisterNodeType("sink", func(metadata Metadata, params map[string]interface{}) interface{} {
		baseNode := originalSinkFactory(metadata, params)
		if len(metadata.OwnerReferences) > 0 {
			return NewOwnerAwareNode(baseNode, metadata)
		}
		return baseNode
	})
}