package mywant

import (
	"fmt"
	"mywant/chain"
	"strings"
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
	RecipePath     string // Path to the recipe file to use for child creation
	RecipeParams   map[string]interface{} // Parameters to pass to recipe
	paths          Paths
	childWants     []Want
	childrenDone   chan bool
	builder        *ChainBuilder    // Reference to builder for dynamic want creation
	recipeLoader   *GenericRecipeLoader    // Reference to generic recipe loader
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
		RecipePath:     "recipes/queue-system.yaml", // Default recipe path
		RecipeParams:   make(map[string]interface{}),
		childWants:     make([]Want, 0),
		childrenDone:   make(chan bool, 1),
	}
	
	// Extract target-specific configuration from params
	target.MaxDisplay = extractIntParam(spec.Params, "max_display", target.MaxDisplay)
	
	// Extract recipe path from spec.Recipe field
	if spec.Recipe != "" {
		target.RecipePath = spec.Recipe
	}
	
	// Extract recipe parameters from spec.RecipeParams or spec.Params
	if spec.RecipeParams != nil {
		target.RecipeParams = spec.RecipeParams
	} else {
		// Fallback: separate recipe parameters from target-specific parameters
		targetSpecificParams := map[string]bool{
			"max_display": true,
		}
		
		// Collect recipe parameters (excluding target-specific ones)
		target.RecipeParams = make(map[string]interface{})
		for key, value := range spec.Params {
			if !targetSpecificParams[key] {
				target.RecipeParams[key] = value
			}
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

// SetRecipeLoader sets the GenericRecipeLoader reference for recipe-based child creation
func (t *Target) SetRecipeLoader(loader *GenericRecipeLoader) {
	t.recipeLoader = loader
	// Resolve recipe parameters using recipe defaults when loader is available
	t.resolveRecipeParameters()
}

// resolveRecipeParameters uses the recipe system to resolve parameters with proper defaults
func (t *Target) resolveRecipeParameters() {
	if t.recipeLoader == nil {
		return
	}
	
	// Get recipe parameters to access default values
	recipeParams, err := t.recipeLoader.GetRecipeParameters(t.RecipePath)
	if err != nil {
		fmt.Printf("⚠️  Could not resolve recipe parameters for %s: %v\n", t.RecipePath, err)
		return
	}
	
	// Create resolved parameters map starting with recipe defaults
	resolvedParams := make(map[string]interface{})
	
	// Set default values from recipe parameter definitions
	for key, value := range recipeParams {
		resolvedParams[key] = value
	}
	
	// Override with provided recipe parameters
	for key, value := range t.RecipeParams {
		resolvedParams[key] = value
	}
	
	// Add target-specific parameters that may be referenced by recipes
	resolvedParams["targetName"] = t.Metadata.Name
	if _, hasCount := resolvedParams["count"]; !hasCount {
		resolvedParams["count"] = t.MaxDisplay
	}
	
	// Update recipe parameters with resolved values
	t.RecipeParams = resolvedParams
}

// CreateChildWants dynamically creates child wants based on external recipes
func (t *Target) CreateChildWants() []Want {
	// Use recipe loader if available
	if t.recipeLoader != nil {
		config, err := t.recipeLoader.LoadConfigFromRecipe(t.RecipePath, t.RecipeParams)
		if err != nil {
			fmt.Printf("⚠️  Failed to load config from recipe %s: %v, falling back to hardcoded creation\n", t.RecipePath, err)
		} else {
			fmt.Printf("✅ Successfully loaded config from recipe %s with %d wants\n", t.RecipePath, len(config.Wants))
			
			// Add owner references to all child wants
			for i := range config.Wants {
				config.Wants[i].Metadata.OwnerReferences = []OwnerReference{
					{
						APIVersion:         "MyWant/v1",
						Kind:               "Want",
						Name:               t.Metadata.Name,
						Controller:         true,
						BlockOwnerDeletion: true,
					},
				}
				// Add owner label for easier identification
				if config.Wants[i].Metadata.Labels == nil {
					config.Wants[i].Metadata.Labels = make(map[string]string)
				}
				config.Wants[i].Metadata.Labels["owner"] = "child"
			}
			
			t.childWants = config.Wants
			return t.childWants
		}
	}

	// Fallback to hardcoded recipe creation
	fmt.Printf("⚠️  Using hardcoded recipe creation for target %s\n", t.Metadata.Name)
	
	// Create generator want (hardcoded fallback)
	generatorWant := Want{
		Metadata: Metadata{
			Name: t.Metadata.Name + "-generator",
			Type: "numbers",
			Labels: map[string]string{
				"role":     "source",
				"owner":    "child",
				"category": "producer",
			},
			OwnerReferences: []OwnerReference{
				{
					APIVersion:         "MyWant/v1",
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
					APIVersion:         "MyWant/v1",
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
					APIVersion:         "MyWant/v1",
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
		
		// Compute and store recipe result
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

// computeTemplateResult computes the result from child wants using recipe-defined result specs
func (t *Target) computeTemplateResult() {
	if t.recipeLoader == nil {
		fmt.Printf("⚠️  Target %s: No recipe loader available for result computation\n", t.Metadata.Name)
		t.computeFallbackResult()
		return
	}

	// Get recipe result definition
	recipeResult, err := t.recipeLoader.GetRecipeResult(t.RecipePath)
	if err != nil {
		fmt.Printf("⚠️  Target %s: Failed to load recipe result definition: %v\n", t.Metadata.Name, err)
		t.computeFallbackResult()
		return
	}

	if recipeResult == nil {
		fmt.Printf("⚠️  Target %s: No result definition in recipe, using fallback\n", t.Metadata.Name)
		t.computeFallbackResult()
		return
	}

	// Get all wants that might be child wants for this target
	allWantStates := t.builder.GetAllWantStates()
	childWantsByName := make(map[string]*Want)
	
	// Build map of child wants by name
	for _, want := range allWantStates {
		for _, ownerRef := range want.Metadata.OwnerReferences {
			if ownerRef.Controller && ownerRef.Kind == "Want" && ownerRef.Name == t.Metadata.Name {
				// Extract the actual want name without prefix
				wantName := want.Metadata.Type
				if want.Metadata.Name != "" {
					// Try to extract base name from prefixed name (e.g., "vacation-hotel-2" -> "hotel")
					parts := strings.Split(want.Metadata.Name, "-")
					if len(parts) >= 2 {
						wantName = parts[len(parts)-2] // Get the type part before the number
					}
				}
				childWantsByName[wantName] = want
				// Also store by exact name for recipes that specify exact names
				if want.Metadata.Name != "" {
					childWantsByName[want.Metadata.Name] = want
				}
				break
			}
		}
	}

	fmt.Printf("🧮 Target %s: Found %d child wants for recipe-defined result computation\n", t.Metadata.Name, len(childWantsByName))

	// Compute primary result using recipe specification
	primaryResult := t.getResultFromSpec(recipeResult.Primary, childWantsByName)
	
	// Initialize Stats map if not exists
	if t.Stats == nil {
		t.Stats = make(WantStats)
	}
	
	// Store primary result in dynamic stats
	t.Stats["recipeResult"] = primaryResult
	t.Stats["primaryResult"] = primaryResult
	
	// Also store in State for backward compatibility
	t.State["recipeResult"] = primaryResult
	t.State["primaryResult"] = primaryResult
	fmt.Printf("✅ Target %s: Primary result (%s from %s): %v\n", t.Metadata.Name, recipeResult.Primary.StatName, recipeResult.Primary.WantName, primaryResult)

	// Compute additional metrics
	metrics := make(map[string]interface{})
	for _, metricSpec := range recipeResult.Metrics {
		metricValue := t.getResultFromSpec(metricSpec, childWantsByName)
		metrics[metricSpec.WantName+"_"+metricSpec.StatName] = metricValue
		t.Stats[metricSpec.WantName+"_"+metricSpec.StatName] = metricValue
		fmt.Printf("📊 Target %s: Metric %s (%s from %s): %v\n", t.Metadata.Name, metricSpec.Description, metricSpec.StatName, metricSpec.WantName, metricValue)
	}
	t.State["metrics"] = metrics

	// Store additional metadata
	t.State["recipePath"] = t.RecipePath
	t.State["childCount"] = len(childWantsByName)

	// Store result in a standardized format for memory dumps
	t.State["result"] = fmt.Sprintf("%s: %v", recipeResult.Primary.Description, primaryResult)

	fmt.Printf("✅ Target %s: Recipe-defined result computation completed\n", t.Metadata.Name)
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
	// Note: Demo programs should also call RegisterQNetWantTypes if they use qnet types
	
	// Initialize generic recipe loader
	recipeLoader := NewGenericRecipeLoader("recipes")
	
	// Register target type with recipe support
	builder.RegisterWantType("target", func(metadata Metadata, spec WantSpec) interface{} {
		target := NewTarget(metadata, spec)
		target.SetBuilder(builder)           // Set builder reference for dynamic want creation
		target.SetRecipeLoader(recipeLoader) // Set recipe loader for external recipes
		return target
	})
	
	// Override all want types to use OwnerAwareWant wrapper for wants with owner references
	originalGeneratorFactory := builder.registry["numbers"]
	builder.RegisterWantType("numbers", func(metadata Metadata, spec WantSpec) interface{} {
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

// getResultFromSpec extracts a specific result value from child wants using recipe specification
func (t *Target) getResultFromSpec(spec RecipeResultSpec, childWants map[string]*Want) interface{} {
	want, exists := childWants[spec.WantName]
	if !exists {
		fmt.Printf("⚠️  Target %s: Want '%s' not found for result computation\n", t.Metadata.Name, spec.WantName)
		return 0
	}

	// Try to get the specified stat from the want's dynamic Stats map
	if want.Stats != nil {
		// Try exact stat name first
		if value, ok := want.Stats[spec.StatName]; ok {
			return value
		}
		// Try lowercase version
		if value, ok := want.Stats[strings.ToLower(spec.StatName)]; ok {
			return value
		}
		// Try common variations
		if spec.StatName == "TotalProcessed" {
			if value, ok := want.Stats["total_processed"]; ok {
				return value
			}
			if value, ok := want.Stats["totalprocessed"]; ok {
				return value
			}
		}
	}
	
	// Fallback: try to get from State map
	if value, ok := want.State[spec.StatName]; ok {
		return value
	}
	if value, ok := want.State[strings.ToLower(spec.StatName)]; ok {
		return value
	}
	
	fmt.Printf("⚠️  Target %s: Stat '%s' not found in want '%s' (available stats: %v)\n", t.Metadata.Name, spec.StatName, spec.WantName, want.Stats)
	return 0
}

// computeFallbackResult provides simple aggregation when recipe result definition is not available
func (t *Target) computeFallbackResult() {
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

	fmt.Printf("🧮 Target %s: Using fallback result computation for %d child wants\n", t.Metadata.Name, len(childWants))

	// Simple aggregate result from child wants using dynamic stats
	totalProcessed := 0
	for _, child := range childWants {
		if child.Stats != nil {
			if processed, ok := child.Stats["total_processed"]; ok {
				if processedInt, ok := processed.(int); ok {
					totalProcessed += processedInt
				}
			} else if processed, ok := child.Stats["TotalProcessed"]; ok {
				if processedInt, ok := processed.(int); ok {
					totalProcessed += processedInt
				}
			}
		}
	}

	// Store result in target's state
	t.State["recipeResult"] = totalProcessed
	t.State["recipePath"] = t.RecipePath
	t.State["childCount"] = len(childWants)
	fmt.Printf("✅ Target %s: Fallback result computed - processed %d items from %d child wants\n", t.Metadata.Name, totalProcessed, len(childWants))

	// Store result in a standardized format for memory dumps
	t.State["result"] = fmt.Sprintf("processed: %d", totalProcessed)
}