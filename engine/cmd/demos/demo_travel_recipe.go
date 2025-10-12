package main

import (
	"fmt"
	"mywant/engine/cmd/types"
	. "mywant/engine/src"
	"os"
)

func main() {
	fmt.Println("🧳 Travel Planning Recipe Demo")
	fmt.Println("===============================")
	fmt.Println("Loading travel itinerary from recipe:")
	fmt.Println("- Recipe defines: restaurant, hotel, buffet wants")
	fmt.Println("- Coordinator orchestrates the complete itinerary")
	fmt.Println()

	// Get YAML file from command line argument
	yamlFile := "config/config-travel-recipe.yaml"
	if len(os.Args) > 1 {
		yamlFile = os.Args[1]
	}

	// Load configuration using generic recipe loader
	config, err := LoadConfigFromYAML(yamlFile)
	if err != nil {
		fmt.Printf("Error loading recipe config %s: %v\n", yamlFile, err)
		return
	}

	fmt.Printf("✅ Generated %d wants from recipe\n", len(config.Wants))
	for _, want := range config.Wants {
		fmt.Printf("  - %s (%s)\n", want.Metadata.Name, want.Metadata.Type)
	}
	fmt.Println()

	// Create chain builder (without auto-registration of owner types)
	builder := NewChainBuilderWithoutOwnerTypes(config)

	// Register travel want types FIRST (before owner wrapper)
	types.RegisterTravelWantTypes(builder)

	// THEN register owner types to wrap the already-registered travel types
	RegisterOwnerWantTypes(builder)

	fmt.Println("🏁 Executing recipe-based travel planning...")
	builder.Execute()

	fmt.Println("\n📊 Final Execution State:")
	fmt.Printf("  Recipe-based travel planning completed")

	fmt.Println("\n✅ Recipe-based travel planning completed!")
}
