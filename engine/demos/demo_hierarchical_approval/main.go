package main

import (
	"fmt"
	mywant "mywant/engine/core"
	_ "mywant/engine/types"
	"os"
	"path/filepath"
	"time"
)

func main() {
	fmt.Printf("🎯 Starting Hierarchical Approval Demo\n")
	fmt.Printf("======================================\n")
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run demo_hierarchical_approval.go <config-file-path>")
		os.Exit(1)
	}
	configPath := os.Args[1]

	// Load configuration
	config, err := mywant.LoadConfigFromYAML(configPath)
	if err != nil {
		fmt.Printf("Error loading %s: %v\n", configPath, err)
		os.Exit(1)
	}
	builder := mywant.NewChainBuilder(config)

	// Scan and register custom target types from recipes directory
	customRegistry := mywant.NewCustomTargetTypeRegistry()

	// Determine absolute path to recipes directory
	// When running from demo: go run -C engine ../cmd/demos/demo_hierarchical_approval ../config/...
	// Current directory is engine/, so recipes should be at ../recipes
	recipePaths := []string{"bundled/recipes", "../engine/bundled/recipes"}

	var recipeErr error
	for _, path := range recipePaths {
		abs, _ := filepath.Abs(path)
		fmt.Printf("  Trying recipes path: %s (abs: %s)\n", path, abs)
		recipeErr = mywant.ScanAndRegisterCustomTypes(path, customRegistry)
		if recipeErr == nil {
			fmt.Printf("  ✅ Successfully loaded recipes from: %s\n", path)
			builder.SetCustomTargetRegistry(customRegistry)
			break
		}
	}
	if recipeErr != nil {
		fmt.Printf("Warning: Could not scan custom types from any recipes path: %v\n", recipeErr)
		// Don't exit - recipes directory might not be accessible
	}

	// Register approval want types

	fmt.Printf("📋 Configuration loaded with %d wants\n", len(config.Wants))

	// Display wants for debugging
	for i, want := range config.Wants {
		fmt.Printf("  %d. %s (%s)\n", i+1, want.Metadata.Name, want.Metadata.Type)
		if want.Metadata.Labels != nil {
			for k, v := range want.Metadata.Labels {
				fmt.Printf("     %s: %s\n", k, v)
			}
		}
	}

	// Execute the chain
	fmt.Printf("\n🚀 Executing hierarchical approval workflow...\n")
	builder.Execute()

	fmt.Printf("\n✅ Hierarchical approval workflow completed successfully!\n")

	// Display final statistics - simplified for now
	fmt.Printf("\n📊 Final Statistics:\n")
	fmt.Printf("===================\n")
	fmt.Printf("Hierarchical approval system executed successfully\n")

	// Keep the demo running for a moment to show results
	time.Sleep(2 * time.Second)
	fmt.Printf("\n🏁 Demo completed\n")
}
