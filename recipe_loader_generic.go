package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"gopkg.in/yaml.v3"
)

// RecipeWant represents a want in recipe format (flattened structure)
type RecipeWant struct {
	Name   string                 `yaml:"name,omitempty"`
	Type   string                 `yaml:"type"`
	Labels map[string]string      `yaml:"labels,omitempty"`
	Params map[string]interface{} `yaml:"params,omitempty"`
	Using  []map[string]string    `yaml:"using,omitempty"`
}

// GenericRecipe represents any recipe structure
type GenericRecipe struct {
	Recipe     GenericRecipeMetadata   `yaml:"recipe"`
	Wants      []RecipeWant            `yaml:"wants,omitempty"`
	Coordinator *RecipeWant            `yaml:"coordinator,omitempty"`
	Parameters map[string]interface{}  `yaml:"parameters,omitempty"`
	
	// Legacy support for existing template formats (placeholder)
	Templates  map[string]interface{} `yaml:"templates,omitempty"`
}

// ConvertToWant converts a RecipeWant to a Want
func (rw RecipeWant) ConvertToWant() Want {
	want := Want{
		Metadata: Metadata{
			Name:   rw.Name,
			Type:   rw.Type,
			Labels: rw.Labels,
		},
		Spec: WantSpec{
			Params: rw.Params,
			Using:  rw.Using,
		},
	}
	
	// Ensure labels map is initialized
	if want.Metadata.Labels == nil {
		want.Metadata.Labels = make(map[string]string)
	}
	
	return want
}

// GenericRecipeMetadata contains recipe information
type GenericRecipeMetadata struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Version     string `yaml:"version"`
	Type        string `yaml:"type,omitempty"` // travel, qnet, fibonacci, etc.
}

// GenericRecipeConfig represents the final configuration after recipe processing
type GenericRecipeConfig struct {
	Config     Config
	Parameters map[string]interface{}
	Metadata   GenericRecipeMetadata
}

// GenericRecipeLoader manages loading and processing any type of recipe
type GenericRecipeLoader struct {
	recipes   map[string]GenericRecipe
	recipeDir string
}

// NewGenericRecipeLoader creates a new generic recipe loader
func NewGenericRecipeLoader(recipeDir string) *GenericRecipeLoader {
	if recipeDir == "" {
		recipeDir = "recipes"
	}
	
	loader := &GenericRecipeLoader{
		recipes:   make(map[string]GenericRecipe),
		recipeDir: recipeDir,
	}
	
	return loader
}

// LoadRecipe loads and processes a recipe file (no templating)
func (grl *GenericRecipeLoader) LoadRecipe(recipePath string, params map[string]interface{}) (*GenericRecipeConfig, error) {
	// Read recipe file
	data, err := ioutil.ReadFile(recipePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read recipe file: %v", err)
	}

	// Parse recipe directly
	var processedRecipe GenericRecipe
	if err := yaml.Unmarshal(data, &processedRecipe); err != nil {
		return nil, fmt.Errorf("failed to parse recipe: %v", err)
	}

	// Merge provided params with recipe defaults
	mergedParams := make(map[string]interface{})
	for k, v := range processedRecipe.Parameters {
		mergedParams[k] = v
	}
	for k, v := range params {
		mergedParams[k] = v
	}


	// Build final configuration
	config := Config{
		Wants: make([]Want, 0),
	}
	
	// Add all wants from recipe, generating names if missing
	if len(processedRecipe.Wants) > 0 {
		prefix := "want"
		if prefixVal, ok := mergedParams["prefix"]; ok {
			if prefixStr, ok := prefixVal.(string); ok {
				prefix = prefixStr
			}
		}
		
		for i, recipeWant := range processedRecipe.Wants {
			// Convert recipe want to Want struct
			want := recipeWant.ConvertToWant()
			
			// Generate name if missing
			if want.Metadata.Name == "" {
				want.Metadata.Name = fmt.Sprintf("%s-%s-%d", prefix, want.Metadata.Type, i+1)
			}
			config.Wants = append(config.Wants, want)
		}
	}
	
	// Add coordinator if present, generating name if missing
	if processedRecipe.Coordinator != nil {
		// Convert recipe coordinator to Want struct
		coordinator := processedRecipe.Coordinator.ConvertToWant()
		if coordinator.Metadata.Name == "" {
			prefix := "want"
			if prefixVal, ok := mergedParams["prefix"]; ok {
				if prefixStr, ok := prefixVal.(string); ok {
					prefix = prefixStr
				}
			}
			coordinator.Metadata.Name = fmt.Sprintf("%s-coordinator", prefix)
		}
		config.Wants = append(config.Wants, coordinator)
	}

	// Handle legacy templates if present (deprecated)
	if len(processedRecipe.Templates) > 0 {
		fmt.Printf("Warning: Legacy template format detected, skipping (deprecated)\n")
	}

	return &GenericRecipeConfig{
		Config:     config,
		Parameters: mergedParams,
		Metadata:   processedRecipe.Recipe,
	}, nil
}

// LoadConfigFromRecipe loads configuration from any recipe type
func (grl *GenericRecipeLoader) LoadConfigFromRecipe(recipePath string, params map[string]interface{}) (Config, error) {
	recipeConfig, err := grl.LoadRecipe(recipePath, params)
	if err != nil {
		return Config{}, err
	}

	return recipeConfig.Config, nil
}

// ValidateRecipe checks if the recipe file exists and is valid
func (grl *GenericRecipeLoader) ValidateRecipe(recipePath string) error {
	if _, err := os.Stat(recipePath); os.IsNotExist(err) {
		return fmt.Errorf("recipe file does not exist: %s", recipePath)
	}

	// Try to load with empty parameters to validate structure
	_, err := grl.LoadRecipe(recipePath, map[string]interface{}{})
	return err
}

// GetRecipeParameters extracts available parameters from recipe
func (grl *GenericRecipeLoader) GetRecipeParameters(recipePath string) (map[string]interface{}, error) {
	data, err := ioutil.ReadFile(recipePath)
	if err != nil {
		return nil, err
	}

	var genericRecipe GenericRecipe
	if err := yaml.Unmarshal(data, &genericRecipe); err != nil {
		return nil, err
	}

	return genericRecipe.Parameters, nil
}

// ListRecipes returns all recipe files in the recipe directory
func (grl *GenericRecipeLoader) ListRecipes() ([]string, error) {
	recipes := make([]string, 0)
	
	if _, err := os.Stat(grl.recipeDir); os.IsNotExist(err) {
		return recipes, nil
	}

	err := filepath.Walk(grl.recipeDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		
		if filepath.Ext(path) == ".yaml" || filepath.Ext(path) == ".yml" {
			relPath, err := filepath.Rel(grl.recipeDir, path)
			if err != nil {
				return err
			}
			recipes = append(recipes, relPath)
		}
		return nil
	})
	
	return recipes, err
}

// GetRecipeMetadata extracts metadata from a recipe without full processing
func (grl *GenericRecipeLoader) GetRecipeMetadata(recipePath string) (GenericRecipeMetadata, error) {
	data, err := ioutil.ReadFile(recipePath)
	if err != nil {
		return GenericRecipeMetadata{}, err
	}

	var genericRecipe GenericRecipe
	if err := yaml.Unmarshal(data, &genericRecipe); err != nil {
		return GenericRecipeMetadata{}, err
	}

	return genericRecipe.Recipe, nil
}

// LoadRecipeWithConfig loads a recipe using a config file that references the recipe
func LoadRecipeWithConfig(configPath string) (Config, map[string]interface{}, error) {
	// Read config file
	configData, err := ioutil.ReadFile(configPath)
	if err != nil {
		return Config{}, nil, fmt.Errorf("failed to read config file: %v", err)
	}

	// Parse config structure
	var configFile struct {
		Recipe struct {
			Path       string                 `yaml:"path"`
			Parameters map[string]interface{} `yaml:"parameters"`
		} `yaml:"recipe"`
	}
	
	if err := yaml.Unmarshal(configData, &configFile); err != nil {
		return Config{}, nil, fmt.Errorf("failed to parse config file: %v", err)
	}

	// Load recipe using generic loader
	loader := NewGenericRecipeLoader("")
	recipeConfig, err := loader.LoadRecipe(configFile.Recipe.Path, configFile.Recipe.Parameters)
	if err != nil {
		return Config{}, nil, err
	}

	return recipeConfig.Config, recipeConfig.Parameters, nil
}

// ProcessRecipeString is deprecated (no longer uses templating)
func (grl *GenericRecipeLoader) ProcessRecipeString(recipeStr string, params map[string]interface{}) (string, error) {
	// Simple parameter substitution - no longer uses Go templating
	return recipeStr, nil
}

// Legacy recipe functions (deprecated template names)
// LoadTravelTemplate is deprecated, use LoadRecipe instead
func LoadTravelTemplate(recipePath string, params map[string]interface{}) (*GenericRecipeConfig, error) {
	loader := NewGenericRecipeLoader("")
	return loader.LoadRecipe(recipePath, params)
}

// LoadConfigFromTemplate is deprecated, use LoadRecipe instead
func LoadConfigFromTemplate(recipePath string, params map[string]interface{}) (Config, error) {
	loader := NewGenericRecipeLoader("")
	return loader.LoadConfigFromRecipe(recipePath, params)
}

// ValidateTemplate is deprecated, use ValidateRecipe instead
func ValidateTemplate(recipePath string) error {
	loader := NewGenericRecipeLoader("")
	return loader.ValidateRecipe(recipePath)
}

// GetTemplateParameters is deprecated, use GetRecipeParameters instead
func GetTemplateParameters(recipePath string) (map[string]interface{}, error) {
	loader := NewGenericRecipeLoader("")
	return loader.GetRecipeParameters(recipePath)
}

// Legacy types for backward compatibility (deprecated)
type TemplateConfig = GenericRecipeConfig
type TemplateMetadata = GenericRecipeMetadata
type TravelTemplate = GenericRecipe

// New recipe functions with cleaner naming
func LoadRecipe(recipePath string, params map[string]interface{}) (*GenericRecipeConfig, error) {
	loader := NewGenericRecipeLoader("")
	return loader.LoadRecipe(recipePath, params)
}

func LoadConfigFromRecipe(recipePath string, params map[string]interface{}) (Config, error) {
	loader := NewGenericRecipeLoader("")
	return loader.LoadConfigFromRecipe(recipePath, params)
}

func ValidateRecipe(recipePath string) error {
	loader := NewGenericRecipeLoader("")
	return loader.ValidateRecipe(recipePath)
}

func GetRecipeParameters(recipePath string) (map[string]interface{}, error) {
	loader := NewGenericRecipeLoader("")
	return loader.GetRecipeParameters(recipePath)
}