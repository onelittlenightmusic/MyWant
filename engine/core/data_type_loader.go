package mywant

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/google/jsonschema-go/jsonschema"
	"gopkg.in/yaml.v3"
)

// resolvedEntry holds a resolved JSON schema and its raw schema for default application.
type resolvedEntry struct {
	schema   *jsonschema.Schema
	resolved *jsonschema.Resolved
}

// DataTypeLoader loads JSON Schema definitions from yaml/data/*.yaml
// and provides Parse() for auto-converting raw any → *DataObject.
//
// Global pattern mirrors globalChainBuilder in chain_builder.go.
var globalDataTypeLoader *DataTypeLoader

// GetGlobalDataTypeLoader returns the global DataTypeLoader instance.
func GetGlobalDataTypeLoader() *DataTypeLoader {
	return globalDataTypeLoader
}

// SetGlobalDataTypeLoader sets the global DataTypeLoader instance.
func SetGlobalDataTypeLoader(l *DataTypeLoader) {
	globalDataTypeLoader = l
}

// DataTypeLoader loads JSON Schema definitions and provides typed parsing.
type DataTypeLoader struct {
	schemas map[string]*resolvedEntry
	mu      sync.RWMutex
}

// NewDataTypeLoader creates a new DataTypeLoader.
func NewDataTypeLoader() *DataTypeLoader {
	return &DataTypeLoader{
		schemas: make(map[string]*resolvedEntry),
	}
}

// LoadFromDir loads all *.yaml files from dir.
func (l *DataTypeLoader) LoadFromDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("failed to read data type dir %s: %w", dir, err)
	}

	var lastErr error
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		ext := filepath.Ext(name)
		if ext != ".yaml" && ext != ".yml" {
			continue
		}
		path := filepath.Join(dir, name)
		if err := l.LoadFromFile(path); err != nil {
			WarnLog("[DATA-TYPE-LOADER] Failed to load %s: %v", path, err)
			lastErr = err
		}
	}
	return lastErr
}

// LoadFromFile loads a single YAML file as a JSON Schema definition.
// The schema's "title" field is used as the type name key.
func (l *DataTypeLoader) LoadFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %w", path, err)
	}

	// Step 1: Unmarshal YAML into map[string]any
	var yamlMap map[string]any
	if err := yaml.Unmarshal(data, &yamlMap); err != nil {
		return fmt.Errorf("failed to unmarshal YAML from %s: %w", path, err)
	}

	// Step 2: Marshal to JSON
	jsonBytes, err := json.Marshal(yamlMap)
	if err != nil {
		return fmt.Errorf("failed to marshal to JSON from %s: %w", path, err)
	}

	// Step 3: Unmarshal JSON into jsonschema.Schema
	var schema jsonschema.Schema
	if err := json.Unmarshal(jsonBytes, &schema); err != nil {
		return fmt.Errorf("failed to unmarshal JSON schema from %s: %w", path, err)
	}

	// Step 4: Use schema.Title as the type name key
	typeName := schema.Title
	if typeName == "" {
		return fmt.Errorf("schema in %s has no title field (required for type name)", path)
	}

	// Step 5: Resolve the schema
	resolved, err := schema.Resolve(nil)
	if err != nil {
		return fmt.Errorf("failed to resolve schema %q from %s: %w", typeName, path, err)
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	l.schemas[typeName] = &resolvedEntry{
		schema:   &schema,
		resolved: resolved,
	}

	InfoLog("[DATA-TYPE-LOADER] Loaded schema %q from %s", typeName, path)
	return nil
}

// Has returns true if a schema for typeName is loaded.
func (l *DataTypeLoader) Has(typeName string) bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	_, ok := l.schemas[typeName]
	return ok
}

// Validate validates raw data against named schema, returns error if invalid.
func (l *DataTypeLoader) Validate(typeName string, raw any) error {
	l.mu.RLock()
	entry, ok := l.schemas[typeName]
	l.mu.RUnlock()
	if !ok {
		return fmt.Errorf("no schema loaded for type %q", typeName)
	}

	dataMap, err := toDataMap(raw)
	if err != nil {
		return fmt.Errorf("failed to convert raw to map for validation: %w", err)
	}

	return entry.resolved.Validate(dataMap)
}

// Parse converts raw any → *DataObject:
//  1. If raw is already *DataObject, return as-is
//  2. Convert raw to map[string]any if needed (via JSON round-trip for structs)
//  3. Validate against schema (log warning on error, don't fail)
//  4. Apply schema property defaults for missing keys
//  5. Return &DataObject{typeName, data}
func (l *DataTypeLoader) Parse(typeName string, raw any) (*DataObject, error) {
	// Step 1: Return as-is if already a *DataObject
	if obj, ok := raw.(*DataObject); ok {
		return obj, nil
	}

	// Step 2: Convert to map[string]any
	dataMap, err := toDataMap(raw)
	if err != nil {
		return nil, fmt.Errorf("failed to convert raw value to map: %w", err)
	}

	l.mu.RLock()
	entry, hasSchema := l.schemas[typeName]
	l.mu.RUnlock()

	if hasSchema {
		// Step 3: Validate (log warning, don't fail)
		if err := entry.resolved.Validate(dataMap); err != nil {
			WarnLog("[DATA-TYPE-LOADER] Validation warning for type %q: %v", typeName, err)
		}

		// Step 4: Apply schema property defaults for missing keys using ApplyDefaults
		if err := entry.resolved.ApplyDefaults(&dataMap); err != nil {
			WarnLog("[DATA-TYPE-LOADER] Failed to apply defaults for type %q: %v", typeName, err)
		}
	}

	// Step 5: Return DataObject
	return &DataObject{typeName: typeName, data: dataMap}, nil
}

// toDataMap converts any → map[string]any.
// - Already a map: return as-is
// - *DataObject: return obj.ToMap()
// - Otherwise: JSON round-trip (marshal then unmarshal)
func toDataMap(raw any) (map[string]any, error) {
	if raw == nil {
		return make(map[string]any), nil
	}
	// Already a map[string]any
	if m, ok := raw.(map[string]any); ok {
		return m, nil
	}
	// DataObject
	if obj, ok := raw.(*DataObject); ok {
		return obj.ToMap(), nil
	}
	// JSON round-trip for structs and other types
	jsonBytes, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal raw value: %w", err)
	}
	var result map[string]any
	if err := json.Unmarshal(jsonBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal to map: %w", err)
	}
	return result, nil
}

// wrapRaw converts any to *DataObject with typeName="" using toDataMap.
func wrapRaw(raw any) *DataObject {
	dataMap, err := toDataMap(raw)
	if err != nil {
		dataMap = make(map[string]any)
	}
	return &DataObject{typeName: "", data: dataMap}
}

// LoadFromFS loads data type YAML files from an embedded fs.FS.
// fsRoot is the subdirectory within fsys (e.g. "data").
func (l *DataTypeLoader) LoadFromFS(fsys fs.FS, fsRoot string) error {
	return fs.WalkDir(fsys, fsRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		ext := filepath.Ext(d.Name())
		if ext != ".yaml" && ext != ".yml" {
			return nil
		}
		data, readErr := fs.ReadFile(fsys, path)
		if readErr != nil {
			WarnLog("[DATA-TYPE-LOADER] Failed to read embedded %s: %v", path, readErr)
			return nil
		}
		var yamlMap map[string]any
		if yamlErr := yaml.Unmarshal(data, &yamlMap); yamlErr != nil {
			WarnLog("[DATA-TYPE-LOADER] Failed to unmarshal embedded %s: %v", path, yamlErr)
			return nil
		}
		jsonBytes, jsonErr := json.Marshal(yamlMap)
		if jsonErr != nil {
			WarnLog("[DATA-TYPE-LOADER] Failed to marshal embedded %s: %v", path, jsonErr)
			return nil
		}
		var schema jsonschema.Schema
		if jsonErr2 := json.Unmarshal(jsonBytes, &schema); jsonErr2 != nil {
			WarnLog("[DATA-TYPE-LOADER] Failed to unmarshal JSON schema from embedded %s: %v", path, jsonErr2)
			return nil
		}
		typeName := schema.Title
		if typeName == "" {
			return nil
		}
		resolved, resolveErr := schema.Resolve(nil)
		if resolveErr != nil {
			WarnLog("[DATA-TYPE-LOADER] Failed to resolve embedded schema %q: %v", typeName, resolveErr)
			return nil
		}
		l.mu.Lock()
		l.schemas[typeName] = &resolvedEntry{schema: &schema, resolved: resolved}
		l.mu.Unlock()
		InfoLog("[DATA-TYPE-LOADER] Loaded embedded schema %q", typeName)
		return nil
	})
}
