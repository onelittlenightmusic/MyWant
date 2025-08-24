package main

import (
	"fmt"
	"gochain/chain"
	"sync"
)

// Global registry to track target instances for child notification
var targetRegistry = make(map[string]*Target)
var targetRegistryMutex sync.RWMutex

// extractIntParam extracts an integer parameter with type conversion and default fallback
func extractIntParam(params map[string]interface{}, key string, defaultValue int) int {
	if value, ok := params[key]; ok {
		if intVal, ok := value.(int); ok {
			return intVal
		} else if floatVal, ok := value.(float64); ok {
			return int(floatVal)
		}
	}
	return defaultValue
}

// Target represents a parent node that creates and manages child nodes
type Target struct {
	Node
	MaxDisplay     int
	TemplateName   string // Name of the template to use for child creation
	TemplateParams map[string]interface{} // Parameters to pass to template
	paths          Paths
	childNodes     []Node
	childrenDone   chan bool
	builder        *ChainBuilder    // Reference to builder for dynamic node creation
	templateLoader *TemplateLoader  // Reference to template loader
}

// NewTarget creates a new target node
func NewTarget(metadata Metadata, spec NodeSpec) *Target {
	target := &Target{
		Node: Node{
			Metadata: metadata,
			Spec:     spec,
			Stats:    NodeStats{},
			Status:   NodeStatusIdle,
			State:    make(map[string]interface{}),
		},
		MaxDisplay:     1000,
		TemplateName:   "number-processing-pipeline", // Default template
		TemplateParams: make(map[string]interface{}),
		childNodes:     make([]Node, 0),
		childrenDone:   make(chan bool, 1),
	}
	
	// Extract target-specific configuration from params
	target.MaxDisplay = extractIntParam(spec.Params, "max_display", target.MaxDisplay)
	
	// Extract template name from spec.template field
	if spec.Template != "" {
		target.TemplateName = spec.Template
	}
	
	// Separate template parameters from target-specific parameters
	targetSpecificParams := map[string]bool{
		"max_display": true,
	}
	
	// Collect template parameters (excluding target-specific ones)
	target.TemplateParams = make(map[string]interface{})
	for key, value := range spec.Params {
		if !targetSpecificParams[key] {
			target.TemplateParams[key] = value
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

// SetTemplateLoader sets the TemplateLoader reference for template-based child creation
func (t *Target) SetTemplateLoader(loader *TemplateLoader) {
	t.templateLoader = loader
	// Resolve template parameters using template defaults when loader is available
	t.resolveTemplateParameters()
}

// resolveTemplateParameters uses the template system to resolve parameters with proper defaults
func (t *Target) resolveTemplateParameters() {
	if t.templateLoader == nil {
		return
	}
	
	// Get template to access its parameter definitions
	template, err := t.templateLoader.GetTemplate(t.TemplateName)
	if err != nil {
		fmt.Printf("⚠️  Could not resolve template parameters for %s: %v\n", t.TemplateName, err)
		return
	}
	
	// Create resolved parameters map starting with template defaults
	resolvedParams := make(map[string]interface{})
	
	// Set default values from template parameter definitions
	for _, param := range template.Parameters {
		resolvedParams[param.Name] = param.Default
	}
	
	// Override with provided template parameters
	for key, value := range t.TemplateParams {
		resolvedParams[key] = value
	}
	
	// Add target-specific parameters that may be referenced by templates
	resolvedParams["targetName"] = t.Metadata.Name
	if _, hasCount := resolvedParams["count"]; !hasCount {
		resolvedParams["count"] = t.MaxDisplay
	}
	
	// Update template parameters with resolved values
	t.TemplateParams = resolvedParams
}

// CreateChildNodes dynamically creates child nodes based on external templates
func (t *Target) CreateChildNodes() []Node {
	// Use template loader if available
	if t.templateLoader != nil {
		nodes, err := t.templateLoader.InstantiateTemplate(t.TemplateName, t.Metadata.Name, t.TemplateParams)
		if err != nil {
			fmt.Printf("⚠️  Failed to instantiate template %s: %v, falling back to hardcoded creation\n", t.TemplateName, err)
		} else {
			fmt.Printf("✅ Successfully instantiated template %s with %d nodes\n", t.TemplateName, len(nodes))
			t.childNodes = nodes
			return t.childNodes
		}
	}

	// Fallback to hardcoded template creation
	fmt.Printf("⚠️  Using hardcoded template creation for target %s\n", t.Metadata.Name)
	
	// Create generator node (hardcoded fallback)
	generatorNode := Node{
		Metadata: Metadata{
			Name: t.Metadata.Name + "-generator",
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
		},
	}

	// Create queue node (hardcoded fallback)
	queueNode := Node{
		Metadata: Metadata{
			Name: t.Metadata.Name + "-queue",
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

	// Create sink node (hardcoded fallback)
	sinkNode := Node{
		Metadata: Metadata{
			Name: t.Metadata.Name + "-sink",
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
				{"role": "processor"},
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
	
	// Initialize template loader
	templateLoader := NewTemplateLoader("templates")
	if err := templateLoader.LoadTemplates(); err != nil {
		fmt.Printf("⚠️  Failed to load templates: %v, using defaults\n", err)
	}
	
	// Register target type with template support
	builder.RegisterNodeType("target", func(metadata Metadata, spec NodeSpec) interface{} {
		target := NewTarget(metadata, spec)
		target.SetBuilder(builder)           // Set builder reference for dynamic node creation
		target.SetTemplateLoader(templateLoader) // Set template loader for external templates
		return target
	})
	
	// Override all node types to use OwnerAwareNode wrapper for nodes with owner references
	originalGeneratorFactory := builder.registry["sequence"]
	builder.RegisterNodeType("sequence", func(metadata Metadata, spec NodeSpec) interface{} {
		baseNode := originalGeneratorFactory(metadata, spec)
		if len(metadata.OwnerReferences) > 0 {
			return NewOwnerAwareNode(baseNode, metadata)
		}
		return baseNode
	})
	
	originalQueueFactory := builder.registry["queue"]
	builder.RegisterNodeType("queue", func(metadata Metadata, spec NodeSpec) interface{} {
		baseNode := originalQueueFactory(metadata, spec)
		if len(metadata.OwnerReferences) > 0 {
			return NewOwnerAwareNode(baseNode, metadata)
		}
		return baseNode
	})
	
	originalSinkFactory := builder.registry["sink"]
	builder.RegisterNodeType("sink", func(metadata Metadata, spec NodeSpec) interface{} {
		baseNode := originalSinkFactory(metadata, spec)
		if len(metadata.OwnerReferences) > 0 {
			return NewOwnerAwareNode(baseNode, metadata)
		}
		return baseNode
	})
}