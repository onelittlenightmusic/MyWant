package mywant

import (
	"os"
	"path/filepath"
)

// UserRecipesDir returns the path to the user's recipe directory (~/.mywant/recipes).
// This is where recipes saved via "Save as Recipe" are stored.
func UserRecipesDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return RecipesDir
	}
	return filepath.Join(home, ".mywant", "recipes")
}

// UserCustomTypesDir returns the path to the user's custom want type directory (~/.mywant/custom-types).
// YAML files placed here are loaded at startup and are available for hot-reload registration.
func UserCustomTypesDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".mywant", "custom-types")
}

// YAML directory structure constants
// These constants define the paths to all YAML configuration files
// ensuring consistency across the application.
const (
	// YamlBaseDir is the root directory for all YAML configuration files
	YamlBaseDir = "yaml"

	// RecipesDir contains recipe template files
	RecipesDir = "yaml/recipes"

	// AgentsDir contains agent definition files
	AgentsDir = "yaml/agents"

	// ConfigDir contains user configuration files
	ConfigDir = "yaml/config"

	// CapabilitiesDir contains capability definition files
	CapabilitiesDir = "yaml/capabilities"

	// WantTypesDir contains want type definition files
	WantTypesDir = "yaml/want_types"

	// DataTypesDir contains data type definition files (JSON Schema format)
	DataTypesDir = "yaml/data"

	// SpecDir contains OpenAPI specification files
	SpecDir = "yaml/spec"

	// MemoryDir contains memory persistence files (not moved)
	MemoryDir = "engine/memory"
)
