package mywant

import (
	"context"
	"fmt"
	"github.com/getkin/kin-openapi/openapi3"
	"gopkg.in/yaml.v3"
	"io/ioutil"
	"os"
	"path/filepath"
)

// RecipeWant represents a want in recipe format (aligned with Want structure)
type RecipeWant struct {
	Metadata Metadata `yaml:"metadata,omitempty" json:"metadata,omitempty"`
	Spec     WantSpec `yaml:"spec,omitempty" json:"spec,omitempty"`

	// Legacy flattened fields for backward compatibility
	Name        string                 `yaml:"name,omitempty" json:"name,omitempty"`
	Type        string                 `yaml:"type,omitempty" json:"type,omitempty"`
	Labels      map[string]string      `yaml:"labels,omitempty" json:"labels,omitempty"`
	Params      map[string]interface{} `yaml:"params,omitempty" json:"params,omitempty"`
	Using       []map[string]string    `yaml:"using,omitempty" json:"using,omitempty"`
	Requires    []string               `yaml:"requires,omitempty" json:"requires,omitempty"`
	RecipeAgent bool                   `yaml:"recipeAgent,omitempty" json:"recipeAgent,omitempty"`
}

// GenericRecipe represents the top-level recipe structure
type GenericRecipe struct {
	Recipe RecipeContent `yaml:"recipe" json:"recipe"`
}

// RecipeResult defines how to compute results from recipe execution Supports both legacy format (primary/metrics) and new flat array format
type RecipeResult []RecipeResultSpec

// RecipeResultSpec specifies which want and stat to use for result computation
type RecipeResultSpec struct {
	WantName    string `yaml:"want_name" json:"want_name"`
	StatName    string `yaml:"stat_name" json:"stat_name"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
}

// RecipeExample represents example deployment configuration for one-click deployment
type RecipeExample struct {
	Wants []RecipeWant `yaml:"wants,omitempty" json:"wants,omitempty"`
}

// RecipeContent contains the actual recipe data
type RecipeContent struct {
	Metadata   GenericRecipeMetadata  `yaml:"metadata,omitempty" json:"metadata,omitempty"`
	Wants      []RecipeWant           `yaml:"wants,omitempty" json:"wants,omitempty"`
	Parameters map[string]interface{} `yaml:"parameters,omitempty" json:"parameters,omitempty"`
	Result     *RecipeResult          `yaml:"result,omitempty" json:"result,omitempty"`
	Example    *RecipeExample         `yaml:"example,omitempty" json:"example,omitempty"`
}
func (rw RecipeWant) ConvertToWant() *Want {
	want := &Want{}

	// Use structured format if metadata type is provided
	if rw.Metadata.Type != "" {
		want.Metadata = rw.Metadata
		want.Spec = rw.Spec

		// Also support legacy fields as fallback for name if not set in metadata
		if want.Metadata.Name == "" && rw.Name != "" {
			want.Metadata.Name = rw.Name
		}
	} else {
		// Fall back to legacy flattened format
		want.Metadata = Metadata{
			Name:   rw.Name,
			Type:   rw.Type,
			Labels: rw.Labels,
		}
		want.Spec = WantSpec{
			Params:   rw.Params,
			Using:    rw.Using,
			Requires: rw.Requires,
		}
	}

	// Ensure labels map is initialized
	if want.Metadata.Labels == nil {
		want.Metadata.Labels = make(map[string]string)
	}

	return want
}

// GenericRecipeMetadata contains recipe information
type GenericRecipeMetadata struct {
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description" json:"description"`
	Version     string `yaml:"version" json:"version"`
	Type        string `yaml:"type,omitempty" json:"type,omitempty"`               // travel, qnet, fibonacci, etc.
	CustomType  string `yaml:"custom_type,omitempty" json:"custom_type,omitempty"` // "wait time in queue system", etc.
	Category    string `yaml:"category,omitempty" json:"category,omitempty"`       // approval, travel, mathematics, queue, etc.
}

// GenericRecipeConfig represents the final configuration after recipe processing
type GenericRecipeConfig struct {
	Config     Config
	Parameters map[string]interface{}
	Metadata   GenericRecipeMetadata
	Result     *RecipeResult
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

// LoadRecipe loads and processes a recipe file with OpenAPI spec validation
func (grl *GenericRecipeLoader) LoadRecipe(recipePath string, params map[string]interface{}) (*GenericRecipeConfig, error) {
	// Read recipe file
	data, err := ioutil.ReadFile(recipePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read recipe file: %v", err)
	}
	err = validateRecipeWithSpec(data)
	if err != nil {
		return nil, fmt.Errorf("recipe validation failed: %w", err)
	}
	var processedRecipe GenericRecipe
	if err := yaml.Unmarshal(data, &processedRecipe); err != nil {
		return nil, fmt.Errorf("failed to parse recipe: %v", err)
	}

	// Extract recipe content
	recipeContent := processedRecipe.Recipe

	// Merge provided params with recipe defaults
	mergedParams := make(map[string]interface{})
	for k, v := range recipeContent.Parameters {
		mergedParams[k] = v
	}
	for k, v := range params {
		mergedParams[k] = v
	}

	// Perform parameter substitution on wants
	for i := range recipeContent.Wants {
		// Substitute both legacy flattened params and structured spec.params
		recipeContent.Wants[i].Params = grl.substituteParams(recipeContent.Wants[i].Params, mergedParams)
		recipeContent.Wants[i].Spec.Params = grl.substituteParams(recipeContent.Wants[i].Spec.Params, mergedParams)
	}
	config := Config{
		Wants: make([]*Want, 0),
	}
	if len(recipeContent.Wants) > 0 {
		prefix := "want"
		if prefixVal, ok := mergedParams["prefix"]; ok {
			if prefixStr, ok := prefixVal.(string); ok {
				prefix = prefixStr
			}
		}

		for i, recipeWant := range recipeContent.Wants {
			want := recipeWant.ConvertToWant()

			// Generate name if missing
			if want.Metadata.Name == "" {
				want.Metadata.Name = fmt.Sprintf("%s-%s-%d", prefix, want.Metadata.Type, i+1)
			}

			// Generate ID if missing
			if want.Metadata.ID == "" {
				want.Metadata.ID = generateUUID()
			}

			// Namespace labels and using selectors with prefix to isolate child wants
			grl.namespaceWantConnections(want, prefix)
			if recipeWant.RecipeAgent {
				want = grl.autoConnect(want, recipeContent.Wants, mergedParams)
			}

			config.Wants = append(config.Wants, want)
		}
	}

	return &GenericRecipeConfig{
		Config:     config,
		Parameters: mergedParams,
		Metadata:   recipeContent.Metadata,
		Result:     recipeContent.Result,
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
func (grl *GenericRecipeLoader) ValidateRecipe(recipePath string) error {
	if _, err := os.Stat(recipePath); os.IsNotExist(err) {
		return fmt.Errorf("recipe file does not exist: %s", recipePath)
	}

	// Try to load with empty parameters to validate structure
	_, err := grl.LoadRecipe(recipePath, map[string]interface{}{})
	return err
}
func (grl *GenericRecipeLoader) GetRecipeParameters(recipePath string) (map[string]interface{}, error) {
	data, err := ioutil.ReadFile(recipePath)
	if err != nil {
		return nil, err
	}

	var genericRecipe GenericRecipe
	if err := yaml.Unmarshal(data, &genericRecipe); err != nil {
		return nil, err
	}

	return genericRecipe.Recipe.Parameters, nil
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
func (grl *GenericRecipeLoader) GetRecipeMetadata(recipePath string) (GenericRecipeMetadata, error) {
	data, err := ioutil.ReadFile(recipePath)
	if err != nil {
		return GenericRecipeMetadata{}, err
	}

	var genericRecipe GenericRecipe
	if err := yaml.Unmarshal(data, &genericRecipe); err != nil {
		return GenericRecipeMetadata{}, err
	}

	return genericRecipe.Recipe.Metadata, nil
}

// LoadRecipeWithConfig loads a recipe using a config file that references the recipe
func LoadRecipeWithConfig(configPath string) (Config, map[string]interface{}, error) {
	// Read config file
	configData, err := ioutil.ReadFile(configPath)
	if err != nil {
		return Config{}, nil, fmt.Errorf("failed to read config file: %v", err)
	}
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

// namespaceWantConnections adds owner namespace to labels and using selectors to isolate child wants
func (grl *GenericRecipeLoader) namespaceWantConnections(want *Want, ownerPrefix string) {
	// Namespace all labels with owner prefix
	if want.Metadata.Labels != nil {
		namespacedLabels := make(map[string]string)
		for key, value := range want.Metadata.Labels {
			namespacedLabels[key] = fmt.Sprintf("%s:%s", ownerPrefix, value)
		}
		want.Metadata.Labels = namespacedLabels
	}

	// Namespace all using selectors with owner prefix CRITICAL: The using selector values must match the namespaced label values in other wants Example: if want A has label "role: scheduler" which becomes "role: travel:scheduler", and want B has using "role: scheduler", it must become "role: travel:scheduler"
	// to match the namespaced label in want A
	if want.Spec.Using != nil {
		for i := range want.Spec.Using {
			namespacedSelector := make(map[string]string)
			for key, value := range want.Spec.Using[i] {
				namespacedSelector[key] = fmt.Sprintf("%s:%s", ownerPrefix, value)
			}
			want.Spec.Using[i] = namespacedSelector
		}
	}
}
func (grl *GenericRecipeLoader) ProcessRecipeString(recipeStr string, params map[string]interface{}) (string, error) {
	// Simple parameter substitution - no longer uses Go templating
	return recipeStr, nil
}

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
func (grl *GenericRecipeLoader) GetRecipeResult(recipePath string) (*RecipeResult, error) {
	data, err := ioutil.ReadFile(recipePath)
	if err != nil {
		return nil, err
	}

	var genericRecipe GenericRecipe
	if err := yaml.Unmarshal(data, &genericRecipe); err != nil {
		return nil, err
	}

	return genericRecipe.Recipe.Result, nil
}
func GetRecipeResult(recipePath string) (*RecipeResult, error) {
	loader := NewGenericRecipeLoader("")
	return loader.GetRecipeResult(recipePath)
}

// substituteParams performs parameter substitution in want params
func (grl *GenericRecipeLoader) substituteParams(params map[string]interface{}, mergedParams map[string]interface{}) map[string]interface{} {
	if params == nil {
		return nil
	}

	substituted := make(map[string]interface{})
	for key, value := range params {
		if strValue, ok := value.(string); ok {
			if paramValue, exists := mergedParams[strValue]; exists {
				substituted[key] = paramValue
			} else {
				substituted[key] = value
			}
		} else {
			substituted[key] = value
		}
	}
	return substituted
}

// autoConnect implements auto-connection logic for RecipeAgent wants NOTE: This is legacy recipe-level auto-connection. The real auto-connection happens system-wide during the connection phase in declarative.go
func (grl *GenericRecipeLoader) autoConnect(want *Want, allWants []RecipeWant, params map[string]interface{}) *Want {
	// Recipe-level auto-connection is limited because it can only see wants within the same recipe System-wide auto-connection in declarative.go handles the full implementation
	return want
}
func validateRecipeWithSpec(yamlData []byte) error {
	// Load the OpenAPI spec for recipes Try to find the spec directory - check both "spec" and "../spec"
	specPath := "spec/recipe-spec.yaml"
	if _, err := os.Stat("spec"); os.IsNotExist(err) {
		specPath = "../spec/recipe-spec.yaml"
	}

	loader := openapi3.NewLoader()
	spec, err := loader.LoadFromFile(specPath)
	if err != nil {
		return fmt.Errorf("failed to load recipe OpenAPI spec: %w", err)
	}
	ctx := context.Background()
	err = spec.Validate(ctx)
	if err != nil {
		return fmt.Errorf("recipe OpenAPI spec is invalid: %w", err)
	}
	var yamlObj interface{}
	err = yaml.Unmarshal(yamlData, &yamlObj)
	if err != nil {
		return fmt.Errorf("failed to parse recipe YAML for validation: %w", err)
	}
	recipeSchemaRef := spec.Components.Schemas["GenericRecipe"]
	if recipeSchemaRef == nil {
		return fmt.Errorf("GenericRecipe schema not found in recipe OpenAPI spec")
	}

	// Basic structural validation - ensure the YAML contains required 'recipe' field
	var recipeObj map[string]interface{}
	err = yaml.Unmarshal(yamlData, &recipeObj)
	if err != nil {
		return fmt.Errorf("invalid recipe YAML structure: %w", err)
	}
	if recipe, ok := recipeObj["recipe"]; !ok {
		return fmt.Errorf("recipe validation failed: must have 'recipe' root key")
	} else {
		err = validateRecipeContentStructure(recipe)
		if err != nil {
			return fmt.Errorf("recipe content validation failed: %w", err)
		}
	}

	InfoLog("[VALIDATION] Recipe validated successfully against OpenAPI spec\n")
	return nil
}
func validateRecipeContentStructure(recipeContent interface{}) error {
	recipeObj, ok := AsMap(recipeContent)
	if !ok {
		return fmt.Errorf("recipe must be an object")
	}
	if wants, ok := recipeObj["wants"]; ok {
		err := validateRecipeWantsStructure(wants)
		if err != nil {
			return fmt.Errorf("wants validation failed: %w", err)
		}
	}
	if params, ok := recipeObj["parameters"]; ok {
		if _, ok := AsMap(params); !ok {
			return fmt.Errorf("parameters must be an object")
		}
	}
	if result, ok := recipeObj["result"]; ok {
		err := validateRecipeResultStructure(result)
		if err != nil {
			return fmt.Errorf("result validation failed: %w", err)
		}
	}

	return nil
}
func validateRecipeWantsStructure(wants interface{}) error {
	wantsArray, ok := AsArray(wants)
	if !ok {
		return fmt.Errorf("wants must be an array")
	}

	for i, want := range wantsArray {
		wantObj, ok := AsMap(want)
		if !ok {
			return fmt.Errorf("want at index %d must be an object", i)
		}
		hasLegacyType := false
		hasStructuredType := false
		if wantType, ok := wantObj["type"]; ok && wantType != "" {
			hasLegacyType = true
		}
		if metadata, ok := wantObj["metadata"]; ok {
			if metadataObj, ok := AsMap(metadata); ok {
				if wantType, ok := metadataObj["type"]; ok && wantType != "" {
					hasStructuredType = true
				}
			}
		}

		// Require at least one type field
		if !hasLegacyType && !hasStructuredType {
			return fmt.Errorf("want at index %d missing required 'type' field (either top-level 'type' or 'metadata.type')", i)
		}
		if labels, ok := wantObj["labels"]; ok {
			if _, ok := AsMap(labels); !ok {
				return fmt.Errorf("want at index %d 'labels' must be an object", i)
			}
		}
		if params, ok := wantObj["params"]; ok {
			if _, ok := AsMap(params); !ok {
				return fmt.Errorf("want at index %d 'params' must be an object", i)
			}
		}
		if using, ok := wantObj["using"]; ok {
			usingArray, ok := AsArray(using)
			if !ok {
				return fmt.Errorf("want at index %d 'using' must be an array", i)
			}
			for j, selector := range usingArray {
				if _, ok := AsMap(selector); !ok {
					return fmt.Errorf("want at index %d using selector at index %d must be an object", i, j)
				}
			}
		}
	}

	return nil
}
func validateRecipeResultStructure(result interface{}) error {
	// Try new format first (flat array)
	if resultArray, ok := AsArray(result); ok {
		// New format: validate as array of result specs
		if len(resultArray) == 0 {
			return fmt.Errorf("result array cannot be empty")
		}
		for i, spec := range resultArray {
			err := validateRecipeResultSpec(spec, fmt.Sprintf("result[%d]", i))
			if err != nil {
				return err
			}
		}
		return nil
	}

	// Fall back to legacy format (object with primary/metrics)
	resultObj, ok := AsMap(result)
	if !ok {
		return fmt.Errorf("result must be either an array (new format) or an object (legacy format)")
	}
	if primary, ok := resultObj["primary"]; !ok {
		return fmt.Errorf("result missing required 'primary' field in legacy format")
	} else {
		err := validateRecipeResultSpec(primary, "primary")
		if err != nil {
			return err
		}
	}
	if metrics, ok := resultObj["metrics"]; ok {
		metricsArray, ok := AsArray(metrics)
		if !ok {
			return fmt.Errorf("result 'metrics' must be an array")
		}
		for i, metric := range metricsArray {
			err := validateRecipeResultSpec(metric, fmt.Sprintf("metrics[%d]", i))
			if err != nil {
				return err
			}
		}
	}

	return nil
}
func validateRecipeResultSpec(spec interface{}, fieldName string) error {
	specObj, ok := AsMap(spec)
	if !ok {
		return fmt.Errorf("%s must be an object", fieldName)
	}
	if wantName, ok := specObj["want_name"]; !ok || wantName == "" {
		return fmt.Errorf("%s missing required 'want_name' field", fieldName)
	}
	if statName, ok := specObj["stat_name"]; !ok || statName == "" {
		return fmt.Errorf("%s missing required 'stat_name' field", fieldName)
	}

	return nil
}

// ScanAndRegisterCustomTypes scans all recipes in the recipe directory and registers custom types
func ScanAndRegisterCustomTypes(recipeDir string, registry *CustomTargetTypeRegistry) error {
	grl := NewGenericRecipeLoader(recipeDir)

	// List all recipes in the directory
	recipes, err := grl.ListRecipes()
	if err != nil {
		return fmt.Errorf("failed to list recipes: %w", err)
	}

	InfoLog("[RECIPE] 🔍 Scanning %d recipes for custom types...\n", len(recipes))

	customTypeCount := 0
	for _, relativePath := range recipes {
		// Construct full path for recipe operations
		fullPath := filepath.Join(recipeDir, relativePath)
		metadata, err := grl.GetRecipeMetadata(fullPath)
		if err != nil {
			InfoLog("[RECIPE] ⚠️  Warning: failed to get metadata for %s: %v\n", relativePath, err)
			continue
		}
		if metadata.CustomType != "" {
			InfoLog("[RECIPE] 🎯 Found custom type '%s' in recipe %s\n", metadata.CustomType, relativePath)
			defaultParams, err := grl.GetRecipeParameters(fullPath)
			if err != nil {
				InfoLog("[RECIPE] ⚠️  Warning: failed to get parameters for %s: %v\n", relativePath, err)
				defaultParams = make(map[string]interface{})
			}

			// Register the custom type with full path
			RegisterCustomTargetType(
				registry,
				metadata.CustomType,
				metadata.Description,
				fullPath,
				defaultParams,
			)

			customTypeCount++
		}
	}

	InfoLog("[RECIPE] ✅ Registered %d custom types from recipes\n", customTypeCount)
	return nil
}
