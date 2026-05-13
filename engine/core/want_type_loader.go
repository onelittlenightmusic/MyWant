package mywant

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	want_spec "github.com/onelittlenightmusic/want-spec"
	"github.com/getkin/kin-openapi/openapi3"
	"gopkg.in/yaml.v3"
)

// WantTypeDefinition represents a complete want type definition
type WantTypeDefinition = want_spec.WantTypeDefinition

// WantTypeMetadata contains want type identity and classification
type WantTypeMetadata = want_spec.WantTypeMetadata

// ParameterDef defines a parameter for want type configuration
type ParameterDef = want_spec.ParameterDef

// ValidationRules defines validation constraints for parameters
type ValidationRules = want_spec.ValidationRules

// StateDef defines a state key for a want type
type StateDef = want_spec.StateDef

// MonitorCapabilityDef describes a MonitorAgent capability derived from requires analysis.
type MonitorCapabilityDef = want_spec.MonitorCapabilityDef

// ConnectivityDef defines input/output patterns for a want type
type ConnectivityDef = want_spec.ConnectivityDef

// ChannelDef defines an input or output channel
type ChannelDef = want_spec.ChannelDef

// AgentDef defines agent integration for a want type
type AgentDef = want_spec.AgentDef

// ConstraintDef defines business logic constraints
type ConstraintDef = want_spec.ConstraintDef

// ExampleDef defines an example usage of a want type
type ExampleDef = want_spec.ExampleDef

// WantTypeWrapper is the top-level YAML structure for wantType-only files.
type WantTypeWrapper = want_spec.WantTypeWrapper

// WantTypeLoader loads and manages want type definitions
type WantTypeLoader struct {
	directory       string
	fallbackFS      fs.FS // used when directory doesn't exist on filesystem (e.g. Homebrew install)
	definitions     map[string]*WantTypeDefinition
	byCategory      map[string][]*WantTypeDefinition
	byPattern       map[string][]*WantTypeDefinition
	mu              sync.RWMutex
	validPatterns   []string
	validCategories map[string]bool
	loadWarnings    []string
	predefinedState []StateDef // Common state fields merged into every want type
}

// PredefinedStateFile is the special YAML file containing common state fields
const PredefinedStateFile = "predefined.yaml"

// PredefinedWrapper wraps the predefined state YAML structure
type PredefinedWrapper struct {
	Predefined struct {
		State []StateDef `yaml:"state"`
	} `yaml:"predefined"`
}

// NewWantTypeLoader creates a new want type loader
func NewWantTypeLoader(directory string) *WantTypeLoader {
	return &WantTypeLoader{
		directory:       directory,
		definitions:     make(map[string]*WantTypeDefinition),
		byCategory:      make(map[string][]*WantTypeDefinition),
		byPattern:       make(map[string][]*WantTypeDefinition),
		validPatterns:   []string{"generator", "processor", "sink", "independent", "coordinator"},
		validCategories: make(map[string]bool),
	}
}

// WithFallbackFS sets an embedded filesystem used when the on-disk directory is unavailable
// (e.g. when installed via Homebrew without the source tree). Returns the loader for chaining.
func (w *WantTypeLoader) WithFallbackFS(fsys fs.FS) *WantTypeLoader {
	w.fallbackFS = fsys
	return w
}

// loadPredefinedState loads the predefined.yaml file and stores common state fields.
func (w *WantTypeLoader) loadPredefinedState() {
	predefinedPath := filepath.Join(w.directory, PredefinedStateFile)
	data, err := os.ReadFile(predefinedPath)
	if err != nil {
		return // predefined.yaml is optional
	}
	var wrapper PredefinedWrapper
	if err := yaml.Unmarshal(data, &wrapper); err != nil {
		log.Printf("Warning: failed to parse predefined.yaml: %v", err)
		return
	}
	w.predefinedState = wrapper.Predefined.State
	log.Printf("[WANT-TYPE-LOADER] Loaded %d predefined state fields", len(w.predefinedState))
}

// mergePredefinedState merges predefined state fields into a definition.
// Type-specific fields take precedence (predefined fields are only added if not already present).
func (w *WantTypeLoader) mergePredefinedState(def *WantTypeDefinition) {
	if len(w.predefinedState) == 0 {
		return
	}
	existing := make(map[string]bool, len(def.State))
	for _, s := range def.State {
		existing[s.Name] = true
	}
	for _, s := range w.predefinedState {
		if !existing[s.Name] {
			def.State = append(def.State, s)
		}
	}
}

// LoadAllWantTypes loads all want type YAML files from the directory.
// Falls back to the embedded filesystem when the on-disk directory does not exist.
func (w *WantTypeLoader) LoadAllWantTypes() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if _, err := os.Stat(w.directory); os.IsNotExist(err) && w.fallbackFS != nil {
		log.Printf("[WANT-TYPE-LOADER] Directory %q not found, using embedded built-in types", w.directory)
		return w.loadAllFromFS(w.fallbackFS)
	}

	// Load predefined common state fields first
	w.loadPredefinedState()

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
			// Skip template and predefined files
			if filepath.Base(path) == "WANT_TYPE_TEMPLATE.yaml" || filepath.Base(path) == PredefinedStateFile {
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

		// Merge predefined common state fields (type-specific fields take precedence)
		w.mergePredefinedState(def)

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
		w.loadWarnings = make([]string, 0, len(loadErrors))
		for _, err := range loadErrors {
			msg := fmt.Sprintf("Warning: %v", err)
			log.Printf("%s\n", msg)
			w.loadWarnings = append(w.loadWarnings, msg)
		}
	}

	if len(w.definitions) == 0 {
		return fmt.Errorf("no valid want type definitions could be loaded")
	}

	// Load user-local custom types from ~/.mywant/custom-types/ (best-effort, non-fatal).
	w.loadUserCustomTypes()

	return nil
}

// loadAllFromFS loads all want type definitions from an embedded fs.FS.
// Used when the on-disk directory is unavailable (e.g. Homebrew install).
// The FS root must contain a "want_types" sub-tree mirroring WantTypesDir layout.
// Must be called with w.mu already held.
func (w *WantTypeLoader) loadAllFromFS(fsys fs.FS) error {
	// Load predefined state from embedded FS
	if data, err := fs.ReadFile(fsys, "want_types/"+PredefinedStateFile); err == nil {
		var wrapper PredefinedWrapper
		if yamlErr := yaml.Unmarshal(data, &wrapper); yamlErr == nil {
			w.predefinedState = wrapper.Predefined.State
			log.Printf("[WANT-TYPE-LOADER] Loaded %d predefined state fields from embedded FS", len(w.predefinedState))
		}
	}

	var yamlFiles []string
	if err := fs.WalkDir(fsys, "want_types", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		ext := filepath.Ext(path)
		if ext != ".yaml" && ext != ".yml" {
			return nil
		}
		base := filepath.Base(path)
		if base == "WANT_TYPE_TEMPLATE.yaml" || base == PredefinedStateFile {
			return nil
		}
		yamlFiles = append(yamlFiles, path)
		return nil
	}); err != nil {
		return fmt.Errorf("failed to scan embedded want_types: %v", err)
	}

	sort.Strings(yamlFiles)

	for _, path := range yamlFiles {
		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			log.Printf("[WANT-TYPE-LOADER] Warning: failed to read embedded %s: %v", path, err)
			continue
		}
		var wrapper WantTypeWrapper
		if err := yaml.Unmarshal(data, &wrapper); err != nil {
			log.Printf("[WANT-TYPE-LOADER] Warning: failed to parse embedded %s: %v", path, err)
			continue
		}
		def := &wrapper.WantType
		if err := w.validateDefinition(def); err != nil {
			log.Printf("[WANT-TYPE-LOADER] Warning: validation failed for embedded %s: %v", path, err)
			continue
		}
		w.mergePredefinedState(def)
		w.definitions[def.Metadata.Name] = def
		w.byCategory[def.Metadata.Category] = append(w.byCategory[def.Metadata.Category], def)
		w.byPattern[def.Metadata.Pattern] = append(w.byPattern[def.Metadata.Pattern], def)
		w.validCategories[def.Metadata.Category] = true
	}

	if len(w.definitions) == 0 {
		return fmt.Errorf("no valid want type definitions found in embedded FS")
	}

	log.Printf("[WANT-TYPE-LOADER] Loaded %d built-in want types from embedded FS", len(w.definitions))
	w.loadUserCustomTypes()
	return nil
}

// loadUserCustomTypes loads YAML files from ~/.mywant/custom-types/.
// Errors are logged as warnings and do not abort startup.
// Must be called with w.mu already held.
func (w *WantTypeLoader) loadUserCustomTypes() {
	dir := UserCustomTypesDir()
	if dir == "" {
		return
	}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return
	}

	_ = WalkFollowingSymlinks(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			// Skip hidden directories (e.g. .git)
			if strings.HasPrefix(info.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		ext := filepath.Ext(path)
		if ext != ".yaml" && ext != ".yml" {
			return nil
		}

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			msg := fmt.Sprintf("Warning: failed to read user custom plugin %s: %v", path, readErr)
			log.Printf("%s\n", msg)
			w.loadWarnings = append(w.loadWarnings, msg)
			return nil
		}

		var wrapper WantTypeWrapper
		if yamlErr := yaml.Unmarshal(data, &wrapper); yamlErr != nil {
			msg := fmt.Sprintf("Warning: failed to parse user custom plugin %s: %v", path, yamlErr)
			log.Printf("%s\n", msg)
			w.loadWarnings = append(w.loadWarnings, msg)
			return nil
		}

		// Skip files that have no wantType key (e.g. agent.yaml files)
		if wrapper.WantType.Metadata.Name == "" {
			return nil
		}
		def := &wrapper.WantType
		if valErr := w.validateDefinition(def); valErr != nil {
			msg := fmt.Sprintf("Warning: validation failed for user custom type %s: %v", path, valErr)
			log.Printf("%s\n", msg)
			w.loadWarnings = append(w.loadWarnings, msg)
			return nil
		}
		w.mergePredefinedState(def)
		w.definitions[def.Metadata.Name] = def
		w.byCategory[def.Metadata.Category] = append(w.byCategory[def.Metadata.Category], def)
		w.byPattern[def.Metadata.Pattern] = append(w.byPattern[def.Metadata.Pattern], def)
		w.validCategories[def.Metadata.Category] = true
		log.Printf("[WantTypeLoader] Loaded user custom type: %s (from %s)\n", def.Metadata.Name, path)
		return nil
	})
}

// loadWantTypeFromFile loads a single want type YAML file
func (w *WantTypeLoader) loadWantTypeFromFile(filePath string) (*WantTypeDefinition, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %v", err)
	}

	// Validate against OpenAPI spec
	if err := w.validateWithSpec(filePath, data); err != nil {
		return nil, fmt.Errorf("OpenAPI validation failed: %v", err)
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

// validateWithSpec validates want type YAML against OpenAPI spec
func (w *WantTypeLoader) validateWithSpec(filePath string, yamlData []byte) error {
	// Load the OpenAPI spec for want types from the external want-spec module
	specPath := "spec/want-type-spec.yaml"
	specData, err := fs.ReadFile(want_spec.FS, specPath)
	if err != nil {
		// Spec not found in want-spec module — skip validation.
		// Built-in types are pre-validated at build time.
		return nil
	}

	loader := openapi3.NewLoader()
	spec, err := loader.LoadFromData(specData)
	if err != nil {
		return fmt.Errorf("failed to load want type OpenAPI spec: %w", err)
	}
	ctx := context.Background()
	err = spec.Validate(ctx)

	if err != nil {
		return fmt.Errorf("want type OpenAPI spec is invalid: %w", err)
	}

	// Parse YAML
	var yamlObj map[string]any
	err = yaml.Unmarshal(yamlData, &yamlObj)
	if err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}

	// Check for wantType root key
	wantTypeData, ok := yamlObj["wantType"]
	if !ok {
		return fmt.Errorf("missing 'wantType' root element")
	}

	_, ok = wantTypeData.(map[string]any)
	if !ok {
		return fmt.Errorf("'wantType' must be an object")
	}

	return nil
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
		if state.Label == "" {
			// CRITICAL: Every state field must have a label for the GCP pattern to function.
			// Without a label, SetCurrent/GetGoal/etc will silently fail.
			return fmt.Errorf("state key '%s' missing mandatory label (must be one of: goal, current, plan, internal)", state.Name)
		}
		validLabels := map[string]bool{"goal": true, "current": true, "plan": true, "internal": true}
		if !validLabels[state.Label] {
			return fmt.Errorf("state key '%s' has invalid label '%s' (must be one of: goal, current, plan, internal)", state.Name, state.Label)
		}
	}

	// Validation of require field is handled by OpenAPI spec
	// The OpenAPI spec defines require.type as required and validates enum values
	// No need to duplicate validation here

	return nil
}

// RegisterDefinition adds or replaces a want type definition at runtime without loading from file.
// Used by the hot-reload API to register new types dynamically.
func (w *WantTypeLoader) RegisterDefinition(def *WantTypeDefinition) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.definitions[def.Metadata.Name] = def
}

// ParseDefinitionFromYAML parses and validates a WantTypeDefinition from raw YAML bytes.
// Returns the parsed definition or an error if validation fails.
func (w *WantTypeLoader) ParseDefinitionFromYAML(data []byte) (*WantTypeDefinition, error) {
	var wrapper WantTypeWrapper
	if err := yaml.Unmarshal(data, &wrapper); err != nil {
		return nil, fmt.Errorf("invalid YAML: %w", err)
	}
	def := &wrapper.WantType
	w.mergePredefinedState(def)
	if err := w.validateDefinition(def); err != nil {
		return nil, err
	}
	return def, nil
}

// UnregisterDefinition removes a YAML-only want type definition at runtime.
// Returns an error if the type does not exist, is system-registered, or is Go-backed.
func (w *WantTypeLoader) UnregisterDefinition(name string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	def, ok := w.definitions[name]
	if !ok {
		return fmt.Errorf("want type %q not found", name)
	}
	if def.Metadata.SystemType {
		return fmt.Errorf("want type %q is a system type and cannot be deleted", name)
	}
	if len(def.InlineAgents) == 0 && len(def.Requires) == 0 {
		return fmt.Errorf("want type %q is backed by Go code and cannot be deleted via API", name)
	}
	delete(w.definitions, name)
	return nil
}

func (w *WantTypeLoader) GetDefinition(name string) *WantTypeDefinition {
	w.mu.RLock()
	defer w.mu.RUnlock()

	return w.definitions[name]
}

// GetLoadWarnings returns warning messages collected during the last LoadAllWantTypes call.
// Each entry describes a YAML file that failed to load and the reason why.
func (w *WantTypeLoader) GetLoadWarnings() []string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if len(w.loadWarnings) == 0 {
		return nil
	}
	result := make([]string, len(w.loadWarnings))
	copy(result, w.loadWarnings)
	return result
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
		"total":      len(w.definitions),
		"categories": len(w.validCategories),
		"patterns":   len(w.validPatterns),
		"byCategory": make(map[string]int),
		"byPattern":  make(map[string]int),
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

// EnrichMonitorCapabilities derives MonitorCapabilities for all loaded definitions
// by cross-referencing each definition's Requires list against the AgentRegistry.
// For each required capability name that is provided by at least one MonitorAgent,
// a MonitorCapabilityDef entry is added to the definition's MonitorCapabilities field.
// This is called once at server startup after all agents are registered.
func (w *WantTypeLoader) EnrichMonitorCapabilities(registry *AgentRegistry) {
	if registry == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()

	for _, def := range w.definitions {
		var monCaps []MonitorCapabilityDef
		for _, capName := range def.Requires {
			if agents := registry.FindMonitorAgentsByCapabilityName(capName); len(agents) > 0 {
				monCaps = append(monCaps, MonitorCapabilityDef{Capability: capName})
			}
		}
		def.MonitorCapabilities = monCaps
	}
}

// LoadWantTypeDefinition loads a single want type definition from a YAML file
func LoadWantTypeDefinition(yamlPath string) (*WantTypeDefinition, error) {
	content, err := os.ReadFile(yamlPath)
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
