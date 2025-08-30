package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"text/template"
	"bytes"
	"gopkg.in/yaml.v3"
)

// GenericRecipe represents any recipe structure
type GenericRecipe struct {
	Recipe     GenericRecipeMetadata   `yaml:"recipe"`
	Wants      []Want                  `yaml:"wants,omitempty"`
	Coordinator *Want                  `yaml:"coordinator,omitempty"`
	Parameters map[string]interface{}  `yaml:"parameters,omitempty"`
	
	// Legacy support for existing template formats (placeholder)
	Templates  map[string]interface{} `yaml:"templates,omitempty"`
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
	funcMap   template.FuncMap
}

// NewGenericRecipeLoader creates a new generic recipe loader
func NewGenericRecipeLoader(recipeDir string) *GenericRecipeLoader {
	if recipeDir == "" {
		recipeDir = "recipes"
	}
	
	loader := &GenericRecipeLoader{
		recipes:   make(map[string]GenericRecipe),
		recipeDir: recipeDir,
		funcMap: template.FuncMap{
			"default": func(defaultVal interface{}, val interface{}) interface{} {
				if val == nil || val == "" {
					return defaultVal
				}
				return val
			},
		},
	}
	
	return loader
}

// LoadRecipe loads and processes a recipe file
func (grl *GenericRecipeLoader) LoadRecipe(recipePath string, params map[string]interface{}) (*GenericRecipeConfig, error) {
	// Read recipe file
	data, err := ioutil.ReadFile(recipePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read recipe file: %v", err)
	}

	// Parse recipe with Go text/template
	tmpl, err := template.New("generic-recipe").Funcs(grl.funcMap).Parse(string(data))
	if err != nil {
		return nil, fmt.Errorf("failed to parse recipe: %v", err)
	}

	// Load recipe structure to get default parameters
	var genericRecipe GenericRecipe
	if err := yaml.Unmarshal(data, &genericRecipe); err != nil {
		// Try to parse without recipe processing first to get defaults
		fmt.Printf("Warning: Could not parse recipe for defaults: %v\n", err)
		genericRecipe.Parameters = make(map[string]interface{})
	}

	// Merge provided params with recipe defaults
	mergedParams := make(map[string]interface{})
	for k, v := range genericRecipe.Parameters {
		mergedParams[k] = v
	}
	for k, v := range params {
		mergedParams[k] = v
	}

	// Execute recipe with merged parameters
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, mergedParams); err != nil {
		return nil, fmt.Errorf("failed to execute recipe: %v", err)
	}

	// Parse the processed YAML
	var processedRecipe GenericRecipe
	if err := yaml.Unmarshal(buf.Bytes(), &processedRecipe); err != nil {
		return nil, fmt.Errorf("failed to parse processed recipe: %v", err)
	}

	// Build final configuration
	config := Config{
		Wants: make([]Want, 0),
	}
	
	// Add all wants from recipe
	if len(processedRecipe.Wants) > 0 {
		config.Wants = append(config.Wants, processedRecipe.Wants...)
	}
	
	// Add coordinator if present
	if processedRecipe.Coordinator != nil {
		config.Wants = append(config.Wants, *processedRecipe.Coordinator)
	}

	// Handle legacy template format if present
	if len(processedRecipe.Templates) > 0 {
		fmt.Printf("Warning: Legacy template format detected, skipping for now\n")
		// Legacy template support would require integrating with template_loader.go
		// For now, we focus on the new unified recipe format
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

// ProcessRecipeString processes a recipe string with parameters
func (grl *GenericRecipeLoader) ProcessRecipeString(recipeStr string, params map[string]interface{}) (string, error) {
	tmpl, err := template.New("string-recipe").Funcs(grl.funcMap).Parse(recipeStr)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, params); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// Legacy travel template functions (moved from travel_template_loader.go)
// LoadTravelTemplate loads and processes a travel recipe file using generic loader
func LoadTravelTemplate(templatePath string, params map[string]interface{}) (*GenericRecipeConfig, error) {
	loader := NewGenericRecipeLoader("")
	return loader.LoadRecipe(templatePath, params)
}

// LoadConfigFromTemplate loads configuration from a travel recipe
func LoadConfigFromTemplate(templatePath string, params map[string]interface{}) (Config, error) {
	loader := NewGenericRecipeLoader("")
	return loader.LoadConfigFromRecipe(templatePath, params)
}

// ValidateTemplate checks if the recipe file exists and is valid
func ValidateTemplate(templatePath string) error {
	loader := NewGenericRecipeLoader("")
	return loader.ValidateRecipe(templatePath)
}

// GetTemplateParameters extracts available parameters from recipe
func GetTemplateParameters(templatePath string) (map[string]interface{}, error) {
	loader := NewGenericRecipeLoader("")
	return loader.GetRecipeParameters(templatePath)
}

// Legacy types for backward compatibility
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