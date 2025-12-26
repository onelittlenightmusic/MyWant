package mywant

import (
	"fmt"
	"sync"
)

// CustomTargetTypeRegistry manages custom target want types and recipes
type CustomTargetTypeRegistry struct {
	customTypes map[string]CustomTargetTypeConfig
	recipes     map[string]*GenericRecipe // Store full recipe content
	mutex       sync.RWMutex
}

// CustomTargetTypeConfig defines a custom target want type
type CustomTargetTypeConfig struct {
	Name             string         // "wait time in queue system"
	Description      string         // Human description
	DefaultRecipe    string         // Default recipe path
	DefaultParams    map[string]any // Default parameters
	CreateTargetFunc func(metadata Metadata, spec WantSpec) *Target
}

// NewCustomTargetTypeRegistry creates a new custom target type registry
func NewCustomTargetTypeRegistry() *CustomTargetTypeRegistry {
	return &CustomTargetTypeRegistry{
		customTypes: make(map[string]CustomTargetTypeConfig),
		recipes:     make(map[string]*GenericRecipe),
	}
}

// Register registers a new custom target type
func (r *CustomTargetTypeRegistry) Register(config CustomTargetTypeConfig) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	r.customTypes[config.Name] = config
	InfoLog("[RECIPE] 🎯 Registered custom target type: '%s'\n", config.Name)
}
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

// Recipe CRUD operations
func (r *CustomTargetTypeRegistry) CreateRecipe(recipeID string, recipe *GenericRecipe) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if _, exists := r.recipes[recipeID]; exists {
		return fmt.Errorf("recipe with ID '%s' already exists", recipeID)
	}

	// Basic validation
	if err := r.validateRecipe(recipe); err != nil {
		return fmt.Errorf("recipe validation failed: %v", err)
	}

	r.recipes[recipeID] = recipe
	InfoLog("[RECIPE] 📝 Created recipe: '%s'\n", recipeID)
	return nil
}
func (r *CustomTargetTypeRegistry) GetRecipe(recipeID string) (*GenericRecipe, bool) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	recipe, exists := r.recipes[recipeID]
	return recipe, exists
}

// UpdateRecipe updates an existing recipe
func (r *CustomTargetTypeRegistry) UpdateRecipe(recipeID string, recipe *GenericRecipe) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if _, exists := r.recipes[recipeID]; !exists {
		return fmt.Errorf("recipe with ID '%s' not found", recipeID)
	}

	// Basic validation
	if err := r.validateRecipe(recipe); err != nil {
		return fmt.Errorf("recipe validation failed: %v", err)
	}

	r.recipes[recipeID] = recipe
	InfoLog("[RECIPE] 📝 Updated recipe: '%s'\n", recipeID)
	return nil
}

// DeleteRecipe removes a recipe from the registry
func (r *CustomTargetTypeRegistry) DeleteRecipe(recipeID string) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if _, exists := r.recipes[recipeID]; !exists {
		return fmt.Errorf("recipe with ID '%s' not found", recipeID)
	}

	delete(r.recipes, recipeID)
	InfoLog("[RECIPE] 🗑️  Deleted recipe: '%s'\n", recipeID)
	return nil
}

// ListRecipes returns all recipe IDs and their metadata
func (r *CustomTargetTypeRegistry) ListRecipes() map[string]*GenericRecipe {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	recipes := make(map[string]*GenericRecipe)
	for id, recipe := range r.recipes {
		recipes[id] = recipe
	}
	return recipes
}
func (r *CustomTargetTypeRegistry) validateRecipe(recipe *GenericRecipe) error {
	if recipe == nil {
		return fmt.Errorf("recipe cannot be nil")
	}
	if recipe.Recipe.Metadata.Name == "" {
		return fmt.Errorf("recipe name is required")
	}
	if len(recipe.Recipe.Wants) == 0 {
		return fmt.Errorf("recipe must contain at least one want")
	}

	for i, want := range recipe.Recipe.Wants {
		hasType := want.Type != "" || (want.Metadata.Type != "")
		if !hasType {
			return fmt.Errorf("want %d: type is required", i)
		}
	}

	return nil
}

// QueueSystemTarget represents a custom target for queue system analysis
type QueueSystemTarget struct {
	*Target
	WaitTimeThreshold float64
	MaxQueueLength    int
}

// NewQueueSystemTarget creates a new queue system target want
func NewQueueSystemTarget(metadata Metadata, spec WantSpec) *Target {
	baseSpec := spec

	// Merge with queue system defaults
	if baseSpec.Params == nil {
		baseSpec.Params = make(map[string]any)
	}
	setDefaultParam(baseSpec.Params, "max_display", 200)
	setDefaultParam(baseSpec.Params, "service_time", 0.1)
	setDefaultParam(baseSpec.Params, "deterministic", false)
	setDefaultParam(baseSpec.Params, "count", 1000)
	setDefaultParam(baseSpec.Params, "rate", 10.0)
	target := NewTarget(metadata, baseSpec)
	target.Description = "Queue system with wait time analysis"

	return target
}

// Helper function to set default parameters
func setDefaultParam(params map[string]any, key string, defaultValue any) {
	if _, exists := params[key]; !exists {
		params[key] = defaultValue
	}
}

// RegisterCustomTargetType dynamically registers a single custom target type
func RegisterCustomTargetType(registry *CustomTargetTypeRegistry, typeName, description, recipePath string, defaultParams map[string]any) {
	registry.Register(CustomTargetTypeConfig{
		Name:          typeName,
		Description:   description,
		DefaultRecipe: recipePath,
		DefaultParams: defaultParams,
		CreateTargetFunc: func(metadata Metadata, spec WantSpec) *Target {
			// Merge default params with provided params
			if spec.Params == nil {
				spec.Params = make(map[string]any)
			}
			for key, value := range defaultParams {
				setDefaultParam(spec.Params, key, value)
			}

			target := NewTarget(metadata, spec)
			target.Description = description
			target.RecipePath = recipePath // Set the correct recipe path for this custom type
			return target
		},
	})
}
