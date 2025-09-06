package mywant

import (
	"fmt"
	"sync"
)

// CustomTargetTypeRegistry manages custom target want types that inherit from base Target
type CustomTargetTypeRegistry struct {
	customTypes map[string]CustomTargetTypeConfig
	mutex       sync.RWMutex
}

// CustomTargetTypeConfig defines a custom target want type
type CustomTargetTypeConfig struct {
	Name              string                 // "wait time in queue system"
	Description       string                 // Human description
	DefaultRecipe     string                 // Default recipe path
	DefaultParams     map[string]interface{} // Default parameters
	CreateTargetFunc  func(metadata Metadata, spec WantSpec) *Target
}

// NewCustomTargetTypeRegistry creates a new custom target type registry
func NewCustomTargetTypeRegistry() *CustomTargetTypeRegistry {
	return &CustomTargetTypeRegistry{
		customTypes: make(map[string]CustomTargetTypeConfig),
	}
}

// Register registers a new custom target type
func (r *CustomTargetTypeRegistry) Register(config CustomTargetTypeConfig) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	
	r.customTypes[config.Name] = config
	fmt.Printf("🎯 Registered custom target type: '%s'\n", config.Name)
}

// Get retrieves a custom target type configuration
func (r *CustomTargetTypeRegistry) Get(typeName string) (CustomTargetTypeConfig, bool) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	
	config, exists := r.customTypes[typeName]
	return config, exists
}

// IsCustomType checks if a type name is a registered custom type
func (r *CustomTargetTypeRegistry) IsCustomType(typeName string) bool {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	
	_, exists := r.customTypes[typeName]
	return exists
}

// ListTypes returns all registered custom type names
func (r *CustomTargetTypeRegistry) ListTypes() []string {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	
	types := make([]string, 0, len(r.customTypes))
	for typeName := range r.customTypes {
		types = append(types, typeName)
	}
	return types
}

// QueueSystemTarget represents a custom target for queue system analysis
type QueueSystemTarget struct {
	*Target
	WaitTimeThreshold float64
	MaxQueueLength    int
}

// NewQueueSystemTarget creates a new queue system target want
func NewQueueSystemTarget(metadata Metadata, spec WantSpec) *Target {
	// Create base target with queue system specific defaults
	baseSpec := spec
	
	// Set default recipe if not specified
	if baseSpec.Recipe == "" {
		baseSpec.Recipe = "recipes/queue-system.yaml"
	}
	
	// Merge with queue system defaults
	if baseSpec.Params == nil {
		baseSpec.Params = make(map[string]interface{})
	}
	
	// Set queue system specific defaults
	setDefaultParam(baseSpec.Params, "max_display", 200)
	setDefaultParam(baseSpec.Params, "service_time", 0.1)
	setDefaultParam(baseSpec.Params, "deterministic", false)
	setDefaultParam(baseSpec.Params, "count", 1000)
	setDefaultParam(baseSpec.Params, "rate", 10.0)
	
	// Create the base target with enhanced spec
	target := NewTarget(metadata, baseSpec)
	
	// Add queue system specific configuration
	target.Description = "Queue system with wait time analysis"
	
	return target
}

// Helper function to set default parameters
func setDefaultParam(params map[string]interface{}, key string, defaultValue interface{}) {
	if _, exists := params[key]; !exists {
		params[key] = defaultValue
	}
}

// RegisterCustomTargetType dynamically registers a single custom target type
func RegisterCustomTargetType(registry *CustomTargetTypeRegistry, typeName, description, recipePath string, defaultParams map[string]interface{}) {
	registry.Register(CustomTargetTypeConfig{
		Name:          typeName,
		Description:   description,
		DefaultRecipe: recipePath,
		DefaultParams: defaultParams,
		CreateTargetFunc: func(metadata Metadata, spec WantSpec) *Target {
			if spec.Recipe == "" {
				spec.Recipe = recipePath
			}
			
			// Merge default params with provided params
			if spec.Params == nil {
				spec.Params = make(map[string]interface{})
			}
			for key, value := range defaultParams {
				setDefaultParam(spec.Params, key, value)
			}
			
			target := NewTarget(metadata, spec)
			target.Description = description
			return target
		},
	})
}