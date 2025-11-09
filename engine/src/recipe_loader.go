package mywant

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
)

// RecipeParameter defines a configurable parameter for recipes
type RecipeParameter struct {
	Name        string      `yaml:"name"`
	Type        string      `yaml:"type"`
	Default     interface{} `yaml:"default"`
	Description string      `yaml:"description"`
}

// WantRecipe defines a recipe for creating child wants (legacy format)
type WantRecipe struct {
	Metadata struct {
		Name   string            `yaml:"name"`
		Type   string            `yaml:"type"`
		Labels map[string]string `yaml:"labels"`
	} `yaml:"metadata"`
	Spec struct {
		Params map[string]interface{} `yaml:"params"`
		Using  []map[string]string    `yaml:"using,omitempty"`
	} `yaml:"spec"`
	// Store type tag information separately
	TypeHints map[string]string `yaml:"-"` // param_name -> type_tag
}

// DRYWantSpec defines minimal want specification in DRY format
type DRYWantSpec struct {
	Name   string                 `yaml:"name"`
	Type   string                 `yaml:"type"`
	Labels map[string]string      `yaml:"labels,omitempty"`
	Params map[string]interface{} `yaml:"params,omitempty"`
	Using  []map[string]string    `yaml:"using,omitempty"`
	// Store type tag information separately
	TypeHints map[string]string `yaml:"-"` // param_name -> type_tag
}

// DRYRecipeDefaults defines common defaults for all wants in a recipe
type DRYRecipeDefaults struct {
	Metadata struct {
		Labels map[string]string `yaml:"labels,omitempty"`
	} `yaml:"metadata,omitempty"`
	Spec struct {
		Params map[string]interface{} `yaml:"params,omitempty"`
	} `yaml:"spec,omitempty"`
}

// LegacyRecipeResult defines how to fetch a result from child wants (legacy format)
type LegacyRecipeResult struct {
	Want     string `yaml:"want"`     // Name pattern or label selector for the child want
	StatName string `yaml:"statName"` // Name of the statistic to fetch (e.g., "AverageWaitTime", "TotalProcessed")
}

// ChildRecipe defines a complete recipe for creating child wants
type ChildRecipe struct {
	Description string `yaml:"description"`

	// Legacy parameter format support
	Parameters []RecipeParameter `yaml:"parameters,omitempty"`

	// New minimized parameter format support
	Params map[string]interface{} `yaml:"params,omitempty"`

	Result *LegacyRecipeResult `yaml:"result,omitempty"` // Optional result fetching configuration

	// Legacy format support
	Children []WantRecipe `yaml:"children,omitempty"`

	// New DRY format support
	Defaults *DRYRecipeDefaults `yaml:"defaults,omitempty"`
	Wants    []DRYWantSpec      `yaml:"wants,omitempty"`
}

// RecipeConfig holds all available recipes
type RecipeConfigLegacy struct {
	Recipes map[string]ChildRecipe `yaml:"recipes"`
}

// RecipeLoader manages loading and instantiating want recipes (legacy compatibility)
type RecipeLoader struct {
	recipes   map[string]ChildRecipe
	recipeDir string
}

// NewRecipeLoader creates a new recipe loader
func NewRecipeLoader(recipeDir string) *RecipeLoader {
	return &RecipeLoader{
		recipes:   make(map[string]ChildRecipe),
		recipeDir: recipeDir,
	}
}

// LoadRecipes loads all recipe files from the recipe directory
func (rl *RecipeLoader) LoadRecipes() error {
	if _, err := os.Stat(rl.recipeDir); os.IsNotExist(err) {
		InfoLog("[RECIPE] Recipe directory %s does not exist, using hardcoded recipes\n", rl.recipeDir)
		return rl.loadDefaultRecipes()
	}

	InfoLog("[RECIPE] Loading recipes from directory: %s\n", rl.recipeDir)
	err := filepath.Walk(rl.recipeDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if filepath.Ext(path) == ".yaml" || filepath.Ext(path) == ".yml" {
			InfoLog("[RECIPE] Loading recipe file: %s\n", path)
			return rl.loadRecipeFile(path)
		}
		return nil
	})

	// Show final recipe count
	InfoLog("[RECIPE] Total recipes loaded: %d\n", len(rl.recipes))
	for name := range rl.recipes {
		InfoLog("[RECIPE] Available recipe: %s\n", name)
	}

	return err
}

// loadRecipeFile loads a single recipe file (simplified)
func (rl *RecipeLoader) loadRecipeFile(filename string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read recipe file %s: %w", filename, err)
	}

	// Parse recipe file directly
	var recipeFile RecipeFile
	if err := yaml.Unmarshal(data, &recipeFile); err != nil {
		return fmt.Errorf("failed to parse recipe file %s: %w", filename, err)
	}

	// Convert to ChildRecipe format
	config := ChildRecipe{
		Description: "Recipe from simplified format",
		Params:      recipeFile.Parameters,
		Wants:       recipeFile.Wants,
	}

	// Use the filename (without extension) as the recipe name
	baseName := filepath.Base(filename)
	recipeName := baseName[:len(baseName)-len(filepath.Ext(baseName))]
	rl.recipes[recipeName] = config
	InfoLog("[RECIPE] Loaded recipe: %s\n", recipeName)

	// Debug: Show recipe params and wants count
	InfoLog("[RECIPE-PARAMS] Recipe params: %+v\n", config.Params)
	InfoLog("[RECIPE-WANTS] Recipe wants count: %d\n", len(config.Wants))

	return nil
}

// RecipeFile represents the new simplified recipe file format
type RecipeFile struct {
	Parameters map[string]interface{} `yaml:"parameters"`
	Wants      []DRYWantSpec          `yaml:"wants"`
}

// loadDefaultRecipes provides fallback hardcoded recipes (simplified)
func (rl *RecipeLoader) loadDefaultRecipes() error {
	// Since we have recipe files in the recipes directory,
	// we don't need complex default recipes anymore
	InfoLog("[RECIPE] No recipe directory found, but using simplified defaults\n")
	return nil
}

// GetRecipe returns a recipe by name
func (rl *RecipeLoader) GetRecipe(name string) (ChildRecipe, error) {
	recipe, exists := rl.recipes[name]
	if !exists {
		return ChildRecipe{}, fmt.Errorf("recipe %s not found", name)
	}
	InfoLog("[RECIPE-SOURCE] Using recipe '%s' with %d wants\n", name, len(recipe.Wants)+len(recipe.Children))
	return recipe, nil
}

// ListRecipes returns all available recipe names
func (rl *RecipeLoader) ListRecipes() []string {
	names := make([]string, 0, len(rl.recipes))
	for name := range rl.recipes {
		names = append(names, name)
	}
	return names
}

// InstantiateRecipe creates actual Want instances from a recipe
func (rl *RecipeLoader) InstantiateRecipe(recipeName string, prefix string, params map[string]interface{}) ([]*Want, error) {
	childRecipe, err := rl.GetRecipe(recipeName)
	if err != nil {
		return nil, err
	}

	// Merge default parameters with provided parameters
	recipeParams := make(map[string]interface{})
	recipeParams["prefix"] = prefix

	// Set default values from recipe parameters
	if childRecipe.Params != nil {
		// New format: params map
		for paramName, defaultValue := range childRecipe.Params {
			recipeParams[paramName] = defaultValue
		}
	}

	// Override with provided parameters
	for key, value := range params {
		recipeParams[key] = value
	}

	var wants []*Want

	// Process wants with automatic naming and labeling
	if len(childRecipe.Wants) > 0 {
		// New recipe format - no templating needed
		for i, dryWantSpec := range childRecipe.Wants {
			want, err := rl.instantiateDRYWant(dryWantSpec, childRecipe.Defaults, recipeParams, prefix, i+1)
			if err != nil {
				return nil, fmt.Errorf("failed to instantiate want from recipe: %w", err)
			}
			wants = append(wants, want)
		}
	} else if len(childRecipe.Children) > 0 {
		// Legacy format - uses old want structure
		for _, wantRecipe := range childRecipe.Children {
			want, err := rl.instantiateWantFromTemplate(wantRecipe, recipeParams, prefix)
			if err != nil {
				return nil, fmt.Errorf("failed to instantiate want from legacy recipe: %w", err)
			}
			wants = append(wants, want)
		}
	}

	return wants, nil
}

// instantiateDRYWant creates a single Want from a DRY want spec with automatic naming and labeling
func (rl *RecipeLoader) instantiateDRYWant(dryWant DRYWantSpec, defaults *DRYRecipeDefaults, params map[string]interface{}, prefix string, wantIndex int) (*Want, error) {
	// Generate automatic name: prefix-type-index
	generatedName := fmt.Sprintf("%s-%s-%d", prefix, dryWant.Type, wantIndex)

	// Use only recipe-defined labels (no hardcoded generation)
	finalLabels := make(map[string]string)

	// Start with defaults if available
	if defaults != nil && defaults.Metadata.Labels != nil {
		for key, value := range defaults.Metadata.Labels {
			finalLabels[key] = value
		}
	}

	// Apply recipe-defined labels (no automatic generation)
	if dryWant.Labels != nil {
		for key, value := range dryWant.Labels {
			finalLabels[key] = value
		}
	}

	// Resolve parameter values by looking up from recipe parameters
	resolvedParams := rl.resolveParams(dryWant.Params, params)

	// Create the Want directly without templating
	want := &Want{
		Metadata: Metadata{
			Name:   generatedName,
			Type:   dryWant.Type,
			Labels: finalLabels,
			OwnerReferences: []OwnerReference{
				{
					APIVersion:         "MyWant/v1",
					Kind:               "Want",
					Name:               prefix,
					Controller:         true,
					BlockOwnerDeletion: true,
				},
			},
		},
		Spec: WantSpec{
			Params: resolvedParams,
			Using:  dryWant.Using,
		},
		Status: WantStatusIdle,
		State:  make(map[string]interface{}),
	}

	return want, nil
}

// generateLabels is deprecated - labels should be defined in recipe files
// This function is kept for legacy compatibility but should not be used for new recipes
func (rl *RecipeLoader) generateLabels(wantType string, index int) map[string]string {
	// Return empty map - all labels should be explicitly defined in recipes
	labels := make(map[string]string)
	InfoLog("[RECIPE] ⚠️  DEPRECATED: generateLabels called for type %s - labels should be defined in recipe YAML\n", wantType)
	return labels
}

// resolveParams resolves parameter values by looking them up in the recipe parameters
func (rl *RecipeLoader) resolveParams(wantParams map[string]interface{}, recipeParams map[string]interface{}) map[string]interface{} {
	resolvedParams := make(map[string]interface{})

	for paramName, paramValue := range wantParams {
		if paramKey, ok := paramValue.(string); ok {
			// Look up the parameter key in recipe parameters
			if resolvedValue, exists := recipeParams[paramKey]; exists {
				resolvedParams[paramName] = resolvedValue
			} else {
				// If not found in recipe params, use the literal value
				resolvedParams[paramName] = paramValue
			}
		} else {
			// Non-string values are used directly
			resolvedParams[paramName] = paramValue
		}
	}

	return resolvedParams
}

// mergeDRYDefaults merges DRY recipe defaults with individual want specifications (legacy)
func (rl *RecipeLoader) mergeDRYDefaults(dryWant DRYWantSpec, defaults *DRYRecipeDefaults, targetName string) WantRecipe {
	// Create a complete WantRecipe by merging defaults with the DRY want spec
	wantRecipe := WantRecipe{
		Metadata: struct {
			Name   string            `yaml:"name"`
			Type   string            `yaml:"type"`
			Labels map[string]string `yaml:"labels"`
		}{
			Name:   dryWant.Name,
			Type:   dryWant.Type,
			Labels: make(map[string]string),
		},
		Spec: struct {
			Params map[string]interface{} `yaml:"params"`
			Using  []map[string]string    `yaml:"using,omitempty"`
		}{
			Params: make(map[string]interface{}),
			Using:  dryWant.Using, // Copy using directly
		},
		TypeHints: make(map[string]string),
	}

	// Merge default labels first, then override with node-specific labels
	if defaults != nil && defaults.Metadata.Labels != nil {
		for key, value := range defaults.Metadata.Labels {
			wantRecipe.Metadata.Labels[key] = value
		}
	}

	// Override with node-specific labels
	if dryWant.Labels != nil {
		for key, value := range dryWant.Labels {
			wantRecipe.Metadata.Labels[key] = value
		}
	}

	// Merge default params first, then override with node-specific params
	if defaults != nil && defaults.Spec.Params != nil {
		for key, value := range defaults.Spec.Params {
			wantRecipe.Spec.Params[key] = value
		}
	}

	// Override with node-specific params
	if dryWant.Params != nil {
		for key, value := range dryWant.Params {
			wantRecipe.Spec.Params[key] = value
		}
	}

	// Copy type hints from DRY node
	if dryWant.TypeHints != nil {
		for key, value := range dryWant.TypeHints {
			wantRecipe.TypeHints[key] = value
		}
	}

	InfoLog("[DRY-MERGE] Merged want '%s' with defaults, final params: %+v\n", dryWant.Name, wantRecipe.Spec.Params)

	return wantRecipe
}

// instantiateWantFromTemplate creates a single Want from a WantRecipe (simplified, no templating)
func (rl *RecipeLoader) instantiateWantFromTemplate(wantRecipe WantRecipe, params map[string]interface{}, targetName string) (*Want, error) {
	// Resolve parameter values directly
	resolvedParams := rl.resolveParams(wantRecipe.Spec.Params, params)

	// Create the actual Want with owner references
	want := &Want{
		Metadata: Metadata{
			Name:   wantRecipe.Metadata.Name,
			Type:   wantRecipe.Metadata.Type,
			Labels: wantRecipe.Metadata.Labels,
			OwnerReferences: []OwnerReference{
				{
					APIVersion:         "MyWant/v1",
					Kind:               "Want",
					Name:               targetName,
					Controller:         true,
					BlockOwnerDeletion: true,
				},
			},
		},
		Spec: WantSpec{
			Params: resolvedParams,
			Using:  wantRecipe.Spec.Using,
		},
		Status: WantStatusIdle,
		State:  make(map[string]interface{}),
	}

	return want, nil
}

// GetLegacyRecipeResult fetches a result value from child nodes based on recipe configuration
func (rl *RecipeLoader) GetLegacyRecipeResult(recipeName string, targetName string, wants []*Want) (interface{}, error) {
	childRecipe, err := rl.GetRecipe(recipeName)
	if err != nil {
		return nil, err
	}

	if childRecipe.Result == nil {
		return nil, fmt.Errorf("recipe %s does not define a result configuration", recipeName)
	}

	// Find the target want based on the result configuration
	var targetWant *Want
	for _, want := range wants {
		// Check if this want matches the result configuration
		if rl.matchesResultWant(want, childRecipe.Result.Want, targetName) {
			targetWant = want
			break
		}
	}

	if targetWant == nil {
		return nil, fmt.Errorf("no want found matching result selector '%s' for recipe %s", childRecipe.Result.Want, recipeName)
	}

	// Extract the requested statistic from the want
	return rl.extractWantStat(targetWant, childRecipe.Result.StatName)
}

// matchesResultWant checks if a want matches the result want selector (simplified, no templating)
func (rl *RecipeLoader) matchesResultWant(want *Want, wantSelector string, targetName string) bool {
	// Simple string replacement for basic cases
	resolvedSelector := wantSelector
	if wantSelector == "{{.targetName}}-queue" {
		resolvedSelector = targetName + "-queue"
	}

	// Check if it matches the want name exactly
	if want.Metadata.Name == resolvedSelector {
		return true
	}

	// Check if it matches based on labels (category, role, etc.)
	for key, value := range want.Metadata.Labels {
		if key == resolvedSelector || value == resolvedSelector {
			return true
		}
	}

	return false
}

// extractWantStat extracts a specific statistic from a want
func (rl *RecipeLoader) extractWantStat(want *Want, statName string) (interface{}, error) {
	switch statName {
	case "AverageWaitTime", "averagewaittime":
		return want.State["average_wait_time"], nil
	case "TotalProcessed", "totalprocessed":
		return want.State["total_processed"], nil
	case "TotalWaitTime", "totalwaittime":
		return want.State["total_wait_time"], nil
	default:
		return nil, fmt.Errorf("unknown stat name: %s", statName)
	}
}
