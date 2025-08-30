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

// Target represents a parent want that creates and manages child wants
type Target struct {
	Want
	MaxDisplay     int
	TemplateName   string // Name of the template to use for child creation
	TemplateParams map[string]interface{} // Parameters to pass to template
	paths          Paths
	childWants     []Want
	childrenDone   chan bool
	builder        *ChainBuilder    // Reference to builder for dynamic want creation
	templateLoader *TemplateLoader  // Reference to template loader
}

// NewTarget creates a new target want
func NewTarget(metadata Metadata, spec WantSpec) *Target {
	target := &Target{
		Want: Want{
			Metadata: metadata,
			Spec:     spec,
			Stats:    WantStats{},
			Status:   WantStatusIdle,
			State:    make(map[string]interface{}),
		},
		MaxDisplay:     1000,
		TemplateName:   "wait time in queue system", // Default template
		TemplateParams: make(map[string]interface{}),
		childWants:     make([]Want, 0),
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

// SetBuilder sets the ChainBuilder reference for dynamic want creation
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

// CreateChildWants dynamically creates child wants based on external templates
func (t *Target) CreateChildWants() []Want {
	// Use template loader if available
	if t.templateLoader != nil {
		wants, err := t.templateLoader.InstantiateTemplate(t.TemplateName, t.Metadata.Name, t.TemplateParams)
		if err != nil {
			fmt.Printf("⚠️  Failed to instantiate template %s: %v, falling back to hardcoded creation\n", t.TemplateName, err)
		} else {
			fmt.Printf("✅ Successfully instantiated template %s with %d wants\n", t.TemplateName, len(wants))
			t.childWants = wants
			return t.childWants
		}
	}

	// Fallback to hardcoded template creation
	fmt.Printf("⚠️  Using hardcoded template creation for target %s\n", t.Metadata.Name)
	
	// Create generator want (hardcoded fallback)
	generatorWant := Want{
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
					Kind:               "Want",
					Name:               t.Metadata.Name,
					Controller:         true,
					BlockOwnerDeletion: true,
				},
			},
		},
		Spec: WantSpec{
			Params: map[string]interface{}{
				"count": t.MaxDisplay,
				"rate":  10.0,
			},
		},
	}

	// Create queue want (hardcoded fallback)
	queueWant := Want{
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
					Kind:               "Want",
					Name:               t.Metadata.Name,
					Controller:         true,
					BlockOwnerDeletion: true,
				},
			},
		},
		Spec: WantSpec{
			Params: map[string]interface{}{
				"service_time": 0.1,
			},
			Using: []map[string]string{
				{"category": "producer"},
			},
		},
	}

	// Create sink want (hardcoded fallback)
	sinkWant := Want{
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
					Kind:               "Want",
					Name:               t.Metadata.Name,
					Controller:         true,
					BlockOwnerDeletion: true,
				},
			},
		},
		Spec: WantSpec{
			Params: map[string]interface{}{
				"display_format": "Number: %d",
			},
			Using: []map[string]string{
				{"role": "processor"},
			},
		},
	}

	t.childWants = []Want{generatorWant, queueWant, sinkWant}
	return t.childWants
}

// CreateFunction implements the ChainNode interface for Target
func (t *Target) CreateFunction() func(inputs []chain.Chan, outputs []chain.Chan) bool {
	return func(inputs []chain.Chan, outputs []chain.Chan) bool {
		fmt.Printf("🎯 Target %s: Managing child nodes with owner references\n", t.Metadata.Name)
		
		// Dynamically create child wants
		if t.builder != nil {
			fmt.Printf("🎯 Target %s: Creating child wants dynamically...\n", t.Metadata.Name)
			childWants := t.CreateChildWants()
			
			// Add child wants to the builder's configuration
			for _, childWant := range childWants {
				fmt.Printf("🔧 Adding child want: %s (type: %s)\n", childWant.Metadata.Name, childWant.Metadata.Type)
			}
			t.builder.AddDynamicWants(childWants)
			
			// Rebuild connections to include new wants
			fmt.Printf("🔧 Rebuilding connections with dynamic wants...\n")
			t.builder.rebuildConnections()
		}
		
		// Target waits for signal that all children have finished
		fmt.Printf("🎯 Target %s: Waiting for all child wants to complete...\n", t.Metadata.Name)
		<-t.childrenDone
		fmt.Printf("🎯 Target %s: All child wants completed, computing result...\n", t.Metadata.Name)
		
		// Compute and store template result
		t.computeTemplateResult()
		
		fmt.Printf("🎯 Target %s: Result computed, target finishing\n", t.Metadata.Name)
		return true
	}
}

// GetWant returns the underlying want
func (t *Target) GetWant() *Want {
	return &t.Want
}

// NotifyChildrenComplete signals that all child wants have completed
func (t *Target) NotifyChildrenComplete() {
	select {
	case t.childrenDone <- true:
		// Signal sent successfully
	default:
		// Channel already has signal, ignore
	}
}

// computeTemplateResult computes the result from child wants and stores it
func (t *Target) computeTemplateResult() {
	if t.templateLoader == nil {
		fmt.Printf("⚠️  Target %s: No template loader available for result computation\n", t.Metadata.Name)
		return
	}

	// Get all wants that might be child wants for this target
	allWantStates := t.builder.GetAllWantStates()
	var childWants []Want
	
	// Filter to only wants owned by this target
	for _, want := range allWantStates {
		for _, ownerRef := range want.Metadata.OwnerReferences {
			if ownerRef.Controller && ownerRef.Kind == "Want" && ownerRef.Name == t.Metadata.Name {
				childWants = append(childWants, *want)
				break
			}
		}
	}

	fmt.Printf("🧮 Target %s: Found %d child wants for result computation\n", t.Metadata.Name, len(childWants))

	// Compute template result
	result, err := t.templateLoader.GetTemplateResult(t.TemplateName, t.Metadata.Name, childWants)
	if err != nil {
		fmt.Printf("⚠️  Target %s: Failed to compute template result: %v\n", t.Metadata.Name, err)
		return
	}

	// Store result in target's state
	t.State["templateResult"] = result
	fmt.Printf("✅ Target %s: Template result computed: %v\n", t.Metadata.Name, result)

	// Also store result in a standardized format for memory dumps
	if resultFloat, ok := result.(float64); ok {
		t.State["result"] = fmt.Sprintf("%.6f", resultFloat)
	} else {
		t.State["result"] = fmt.Sprintf("%v", result)
	}
}

// addChildWantsToMemory adds child wants to the memory configuration
func (t *Target) addChildWantsToMemory() error {
	// This is a placeholder - in a real implementation, this would
	// interact with the ChainBuilder to add wants to the memory file
	// For now, we'll assume the reconcile loop will pick up the wants
	fmt.Printf("🔧 Adding %d child wants to memory configuration\n", len(t.childWants))
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

// OwnerAwareWant wraps any want type to add parent notification capability
type OwnerAwareWant struct {
	BaseWant   interface{} // The original want (Generator, Queue, Sink, etc.)
	TargetName string
	WantName   string
}

// NewOwnerAwareWant creates a wrapper that adds parent notification to any want
func NewOwnerAwareWant(baseWant interface{}, metadata Metadata) *OwnerAwareWant {
	// Find target name from owner references
	targetName := ""
	for _, ownerRef := range metadata.OwnerReferences {
		if ownerRef.Controller && ownerRef.Kind == "Want" {
			targetName = ownerRef.Name
			break
		}
	}
	
	return &OwnerAwareWant{
		BaseWant:   baseWant,
		TargetName: targetName,
		WantName:   metadata.Name,
	}
}

// CreateFunction wraps the base want's function to add completion notification
func (oaw *OwnerAwareWant) CreateFunction() func(inputs []chain.Chan, outputs []chain.Chan) bool {
	// Get the original function from the base want
	var originalFunc func(inputs []chain.Chan, outputs []chain.Chan) bool
	
	if chainWant, ok := oaw.BaseWant.(ChainWant); ok {
		originalFunc = chainWant.CreateFunction()
	} else {
		// Fallback for non-ChainWant types
		return func(inputs []chain.Chan, outputs []chain.Chan) bool {
			fmt.Printf("⚠️  Want %s: No CreateFunction available\n", oaw.WantName)
			return true
		}
	}
	
	return func(inputs []chain.Chan, outputs []chain.Chan) bool {
		// Call the original function
		result := originalFunc(inputs, outputs)
		
		// If want completed successfully and we have a target, notify it
		if result && oaw.TargetName != "" {
			fmt.Printf("💬 Child %s completed, notifying target %s\n", oaw.WantName, oaw.TargetName)
			NotifyTargetCompletion(oaw.TargetName, oaw.WantName)
		}
		
		return result
	}
}

// GetWant returns the underlying want from the base want
func (oaw *OwnerAwareWant) GetWant() *Want {
	if chainWant, ok := oaw.BaseWant.(ChainWant); ok {
		return chainWant.GetWant()
	}
	return nil
}

// RegisterOwnerWantTypes registers the owner-based want types with a ChainBuilder
func RegisterOwnerWantTypes(builder *ChainBuilder) {
	// Register existing qnet types first
	RegisterQNetWantTypes(builder)
	
	// Initialize template loader
	templateLoader := NewTemplateLoader("templates")
	if err := templateLoader.LoadTemplates(); err != nil {
		fmt.Printf("⚠️  Failed to load templates: %v, using defaults\n", err)
	}
	
	// Register target type with template support
	builder.RegisterWantType("target", func(metadata Metadata, spec WantSpec) interface{} {
		target := NewTarget(metadata, spec)
		target.SetBuilder(builder)           // Set builder reference for dynamic want creation
		target.SetTemplateLoader(templateLoader) // Set template loader for external templates
		return target
	})
	
	// Override all want types to use OwnerAwareWant wrapper for wants with owner references
	originalGeneratorFactory := builder.registry["sequence"]
	builder.RegisterWantType("sequence", func(metadata Metadata, spec WantSpec) interface{} {
		baseWant := originalGeneratorFactory(metadata, spec)
		if len(metadata.OwnerReferences) > 0 {
			return NewOwnerAwareWant(baseWant, metadata)
		}
		return baseWant
	})
	
	originalQueueFactory := builder.registry["queue"]
	builder.RegisterWantType("queue", func(metadata Metadata, spec WantSpec) interface{} {
		baseWant := originalQueueFactory(metadata, spec)
		if len(metadata.OwnerReferences) > 0 {
			return NewOwnerAwareWant(baseWant, metadata)
		}
		return baseWant
	})
	
	originalSinkFactory := builder.registry["sink"]
	builder.RegisterWantType("sink", func(metadata Metadata, spec WantSpec) interface{} {
		baseWant := originalSinkFactory(metadata, spec)
		if len(metadata.OwnerReferences) > 0 {
			return NewOwnerAwareWant(baseWant, metadata)
		}
		return baseWant
	})
}