package mywant

import (
	"os"
	"path/filepath"
)

// WalkFollowingSymlinks walks the file tree rooted at root, following symlinks
// to directories. filepath.Walk does not follow symlinks, so symlinked
// directories (e.g. from `make install-skills`) would otherwise be skipped.
func WalkFollowingSymlinks(root string, fn filepath.WalkFunc) error {
	return walkSymlinks(root, fn)
}

func walkSymlinks(path string, fn filepath.WalkFunc) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fn(path, nil, err)
	}
	// Resolve symlink
	if info.Mode()&os.ModeSymlink != 0 {
		realInfo, statErr := os.Stat(path)
		if statErr != nil {
			return fn(path, info, statErr)
		}
		info = realInfo
	}
	if err2 := fn(path, info, nil); err2 != nil {
		if info.IsDir() && err2 == filepath.SkipDir {
			return nil
		}
		return err2
	}
	if !info.IsDir() {
		return nil
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return fn(path, info, err)
	}
	for _, entry := range entries {
		if err2 := walkSymlinks(filepath.Join(path, entry.Name()), fn); err2 != nil {
			return err2
		}
	}
	return nil
}

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

	// AchievementsDir contains achievement seed files (locked achievements + rules)
	AchievementsDir = "yaml/achievements"

	// SystemWantsFile defines system wants injected fresh on every server startup
	SystemWantsFile = "yaml/system_wants.yaml"
)
