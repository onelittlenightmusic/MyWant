package mywant

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// WantTypeDefinition represents a complete want type definition
type WantTypeDefinition struct {
	Metadata      WantTypeMetadata  `json:"metadata" yaml:"metadata"`
	Parameters    []ParameterDef    `json:"parameters" yaml:"parameters"`
	State         []StateDef        `json:"state" yaml:"state"`
	Connectivity  ConnectivityDef   `json:"connectivity" yaml:"connectivity"`
	Require       *RequireSpec      `json:"require,omitempty" yaml:"require,omitempty"` // Structured connectivity requirement
	UsageLimit    *UsageLimitSpec   `json:"usageLimit,omitempty" yaml:"usageLimit,omitempty"` // Deprecated: use require instead
	Agents        []AgentDef        `json:"agents" yaml:"agents"`
	Constraints   []ConstraintDef   `json:"constraints" yaml:"constraints"`
	Examples      []ExampleDef      `json:"examples" yaml:"examples"`
	RelatedTypes  []string          `json:"relatedTypes" yaml:"relatedTypes"`
	SeeAlso       []string          `json:"seeAlso" yaml:"seeAlso"`
}

// WantTypeMetadata contains want type identity and classification
type WantTypeMetadata struct {
	Name        string `json:"name" yaml:"name"`
	Title       string `json:"title" yaml:"title"`
	Description string `json:"description" yaml:"description"`
	Version     string `json:"version" yaml:"version"`
	Category    string `json:"category" yaml:"category"`
	Pattern     string `json:"pattern" yaml:"pattern"` // generator, processor, sink, coordinator, independent
}

// ParameterDef defines a parameter for want type configuration
type ParameterDef struct {
	Name        string           `json:"name" yaml:"name"`
	Description string           `json:"description" yaml:"description"`
	Type        string           `json:"type" yaml:"type"` // Go type: int, float64, string, bool, []string, map[string]any
	Default     any      `json:"default,omitempty" yaml:"default,omitempty"`
	Required    bool             `json:"required" yaml:"required"`
	Validation  ValidationRules  `json:"validation,omitempty" yaml:"validation,omitempty"`
	Example     any      `json:"example,omitempty" yaml:"example,omitempty"`
}

// ValidationRules defines validation constraints for parameters
type ValidationRules struct {
	Min     *float64      `json:"min,omitempty" yaml:"min,omitempty"`
	Max     *float64      `json:"max,omitempty" yaml:"max,omitempty"`
	Pattern string        `json:"pattern,omitempty" yaml:"pattern,omitempty"`
	Enum    []any `json:"enum,omitempty" yaml:"enum,omitempty"`
}

// StateDef defines a state key for a want type
type StateDef struct {
	Name         string      `json:"name" yaml:"name"`
	Description  string      `json:"description" yaml:"description"`
	Type         string      `json:"type" yaml:"type"`
	Persistent   bool        `json:"persistent" yaml:"persistent"`
	InitialValue any         `json:"initialValue,omitempty" yaml:"initialValue,omitempty"`
	Example      any         `json:"example,omitempty" yaml:"example,omitempty"`
}

// ConnectivityDef defines input/output patterns for a want type
type ConnectivityDef struct {
	Inputs  []ChannelDef `json:"inputs" yaml:"inputs"`
	Outputs []ChannelDef `json:"outputs" yaml:"outputs"`
}

// ChannelDef defines an input or output channel
type ChannelDef struct {
	Name        string `json:"name" yaml:"name"`
	Type        string `json:"type" yaml:"type"` // want, agent, state, event
	Description string `json:"description" yaml:"description"`
	Required    bool   `json:"required,omitempty" yaml:"required,omitempty"`
	Multiple    bool   `json:"multiple,omitempty" yaml:"multiple,omitempty"`
}

// AgentDef defines agent integration for a want type
type AgentDef struct {
	Name        string `json:"name" yaml:"name"`
	Role        string `json:"role" yaml:"role"` // monitor, action, validator, transformer
	Description string `json:"description" yaml:"description"`
	Example     string `json:"example,omitempty" yaml:"example,omitempty"`
}

// ConstraintDef defines business logic constraints
type ConstraintDef struct {
	Description string `json:"description" yaml:"description"`
	Validation  string `json:"validation" yaml:"validation"`
}

// ExampleDef defines an example usage of a want type
// The Want field contains the full want configuration that can be deployed via the want API
type ExampleDef struct {
	Name             string                 `json:"name" yaml:"name"`
	Description      string                 `json:"description" yaml:"description"`
	Want             map[string]any `json:"want" yaml:"want"`             // Full want configuration (metadata + spec)
	ExpectedBehavior string                 `json:"expectedBehavior" yaml:"expectedBehavior"`
}

// WantTypeWrapper is the top-level YAML structure
type WantTypeWrapper struct {
	WantType WantTypeDefinition `yaml:"wantType"`
}

// WantTypeLoader loads and manages want type definitions
type WantTypeLoader struct {
	directory       string
	definitions     map[string]*WantTypeDefinition
	byCategory      map[string][]*WantTypeDefinition
	byPattern       map[string][]*WantTypeDefinition
	mu              sync.RWMutex
	validPatterns   []string
	validCategories map[string]bool
}

// NewWantTypeLoader creates a new want type loader
func NewWantTypeLoader(directory string) *WantTypeLoader {
	return &WantTypeLoader{
		directory:       directory,
		definitions:     make(map[string]*WantTypeDefinition),
		byCategory:      make(map[string][]*WantTypeDefinition),
		byPattern:       make(map[string][]*WantTypeDefinition),
		validPatterns:   []string{"generator", "processor", "sink", "coordinator", "independent"},
		validCategories: make(map[string]bool),
	}
}

// LoadAllWantTypes loads all want type YAML files from the directory
func (w *WantTypeLoader) LoadAllWantTypes() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	var yamlFiles []string
	err := filepath.Walk(w.directory, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories and non-YAML files
		if info.IsDir() {
			return nil
		}

		if filepath.Ext(path) == ".yaml" || filepath.Ext(path) == ".yml" {
			// Skip template files
			if filepath.Base(path) == "WANT_TYPE_TEMPLATE.yaml" {
				return nil
			}
			yamlFiles = append(yamlFiles, path)
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to scan want_types directory: %v", err)
	}

	if len(yamlFiles) == 0 {
		return fmt.Errorf("no want type YAML files found in %s", w.directory)
	}

	// Sort for consistent loading order
	sort.Strings(yamlFiles)

	// Load each YAML file
	var loadErrors []error
	for _, filePath := range yamlFiles {
		def, err := w.loadWantTypeFromFile(filePath)
		if err != nil {
			loadErrors = append(loadErrors, fmt.Errorf("failed to load %s: %v", filePath, err))
			continue
		}

		// Register definition
		w.definitions[def.Metadata.Name] = def

		// Index by category
		w.byCategory[def.Metadata.Category] = append(w.byCategory[def.Metadata.Category], def)

		// Index by pattern
		w.byPattern[def.Metadata.Pattern] = append(w.byPattern[def.Metadata.Pattern], def)

		// Track valid categories
		w.validCategories[def.Metadata.Category] = true
	}

	if len(loadErrors) > 0 {
		// Log errors but don't fail if at least some files loaded
		for _, err := range loadErrors {
			fmt.Printf("Warning: %v\n", err)
		}
	}

	if len(w.definitions) == 0 {
		return fmt.Errorf("no valid want type definitions could be loaded")
	}

	return nil
}

// loadWantTypeFromFile loads a single want type YAML file
func (w *WantTypeLoader) loadWantTypeFromFile(filePath string) (*WantTypeDefinition, error) {
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %v", err)
	}

	var wrapper WantTypeWrapper
	err = yaml.Unmarshal(data, &wrapper)
	if err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %v", err)
	}

	def := &wrapper.WantType
	err = w.validateDefinition(def)
	if err != nil {
		return nil, fmt.Errorf("validation failed: %v", err)
	}

	return def, nil
}
func (w *WantTypeLoader) validateDefinition(def *WantTypeDefinition) error {
	if def.Metadata.Name == "" {
		return fmt.Errorf("metadata.name is required")
	}
	if def.Metadata.Title == "" {
		return fmt.Errorf("metadata.title is required")
	}
	if def.Metadata.Description == "" {
		return fmt.Errorf("metadata.description is required")
	}
	if def.Metadata.Version == "" {
		return fmt.Errorf("metadata.version is required")
	}
	if def.Metadata.Category == "" {
		return fmt.Errorf("metadata.category is required")
	}
	if def.Metadata.Pattern == "" {
		return fmt.Errorf("metadata.pattern is required")
	}
	validPattern := false
	for _, vp := range w.validPatterns {
		if def.Metadata.Pattern == vp {
			validPattern = true
			break
		}
	}
	if !validPattern {
		return fmt.Errorf("invalid pattern: %s (must be one of: %v)", def.Metadata.Pattern, w.validPatterns)
	}
	for _, param := range def.Parameters {
		if param.Name == "" {
			return fmt.Errorf("parameter missing name")
		}
		if param.Type == "" {
			return fmt.Errorf("parameter %s missing type", param.Name)
		}
	}
	for _, state := range def.State {
		if state.Name == "" {
			return fmt.Errorf("state key missing name")
		}
		if state.Type == "" {
			return fmt.Errorf("state key %s missing type", state.Name)
		}
	}

	// Validation of require field is handled by OpenAPI spec
	// The OpenAPI spec defines require.type as required and validates enum values
	// No need to duplicate validation here

	return nil
}
func (w *WantTypeLoader) GetDefinition(name string) *WantTypeDefinition {
	w.mu.RLock()
	defer w.mu.RUnlock()

	return w.definitions[name]
}
func (w *WantTypeLoader) GetAll() []*WantTypeDefinition {
	w.mu.RLock()
	defer w.mu.RUnlock()

	defs := make([]*WantTypeDefinition, 0, len(w.definitions))
	for _, def := range w.definitions {
		defs = append(defs, def)
	}

	// Sort by name for consistent output
	sort.Slice(defs, func(i, j int) bool {
		return defs[i].Metadata.Name < defs[j].Metadata.Name
	})

	return defs
}

// ListByCategory returns all want type definitions in a category
func (w *WantTypeLoader) ListByCategory(category string) []*WantTypeDefinition {
	w.mu.RLock()
	defer w.mu.RUnlock()

	defs := w.byCategory[category]
	if defs == nil {
		return []*WantTypeDefinition{}
	}
	result := make([]*WantTypeDefinition, len(defs))
	copy(result, defs)
	return result
}

// ListByPattern returns all want type definitions with a pattern
func (w *WantTypeLoader) ListByPattern(pattern string) []*WantTypeDefinition {
	w.mu.RLock()
	defer w.mu.RUnlock()

	defs := w.byPattern[pattern]
	if defs == nil {
		return []*WantTypeDefinition{}
	}
	result := make([]*WantTypeDefinition, len(defs))
	copy(result, defs)
	return result
}
func (w *WantTypeLoader) GetCategories() []string {
	w.mu.RLock()
	defer w.mu.RUnlock()

	categories := make([]string, 0, len(w.validCategories))
	for cat := range w.validCategories {
		categories = append(categories, cat)
	}
	sort.Strings(categories)
	return categories
}
func (w *WantTypeLoader) GetPatterns() []string {
	w.mu.RLock()
	defer w.mu.RUnlock()

	return w.validPatterns
}
func (w *WantTypeLoader) GetStats() map[string]any {
	w.mu.RLock()
	defer w.mu.RUnlock()

	stats := map[string]any{
		"total":            len(w.definitions),
		"categories":       len(w.validCategories),
		"patterns":         len(w.validPatterns),
		"byCategory":       make(map[string]int),
		"byPattern":        make(map[string]int),
	}

	// Count by category
	categoryCounts := stats["byCategory"].(map[string]int)
	for cat, defs := range w.byCategory {
		categoryCounts[cat] = len(defs)
	}

	// Count by pattern
	patternCounts := stats["byPattern"].(map[string]int)
	for pattern, defs := range w.byPattern {
		patternCounts[pattern] = len(defs)
	}

	return stats
}
func (w *WantTypeLoader) ValidateParameterValues(typeName string, params map[string]any) error {
	def := w.GetDefinition(typeName)
	if def == nil {
		return fmt.Errorf("unknown want type: %s", typeName)
	}
	paramIndex := make(map[string]*ParameterDef)
	for i, p := range def.Parameters {
		paramIndex[p.Name] = &def.Parameters[i]
	}
	for _, paramDef := range def.Parameters {
		if paramDef.Required {
			if _, exists := params[paramDef.Name]; !exists {
				return fmt.Errorf("required parameter '%s' not provided", paramDef.Name)
			}
		}
	}
	for paramName, paramValue := range params {
		paramDef, exists := paramIndex[paramName]
		if !exists {
			return fmt.Errorf("unknown parameter '%s' for want type '%s'", paramName, typeName)
		}

		// Type checking would go here (simplified for now)
		_ = paramDef
		_ = paramValue
	}

	return nil
}

// LoadWantTypeDefinition loads a single want type definition from a YAML file
func LoadWantTypeDefinition(yamlPath string) (*WantTypeDefinition, error) {
	content, err := ioutil.ReadFile(yamlPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", yamlPath, err)
	}

	// Parse the YAML structure
	var data map[string]any
	err = yaml.Unmarshal(content, &data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse YAML from %s: %w", yamlPath, err)
	}

	// Extract wantType root element
	wantTypeData, exists := data["wantType"]
	if !exists {
		return nil, fmt.Errorf("missing 'wantType' root element in %s", yamlPath)
	}

	// Convert to JSON and back to properly unmarshal into struct
	jsonBytes, err := yaml.Marshal(wantTypeData)
	if err != nil {
		return nil, fmt.Errorf("failed to convert YAML to JSON in %s: %w", yamlPath, err)
	}

	var def WantTypeDefinition
	err = yaml.Unmarshal(jsonBytes, &def)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal want type definition from %s: %w", yamlPath, err)
	}

	return &def, nil
}
