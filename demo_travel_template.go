package main

import (
	"fmt"
)

func main() {
	fmt.Println("🧳 Travel Planning Recipe Demo")
	fmt.Println("===============================")
	fmt.Println("Loading travel itinerary from recipe:")
	fmt.Println("- Recipe defines: restaurant, hotel, buffet wants")
	fmt.Println("- Coordinator orchestrates the complete itinerary")
	fmt.Println()

	// Load configuration using generic recipe loader
	config, params, err := LoadRecipeWithConfig("config-travel-template.yaml")
	if err != nil {
		fmt.Printf("Error loading recipe config: %v\n", err)
		return
	}

	fmt.Printf("📋 Recipe parameters: %+v\n", params)
	fmt.Printf("✅ Generated %d wants from recipe\n", len(config.Wants))
	for _, want := range config.Wants {
		fmt.Printf("  - %s (%s)\n", want.Metadata.Name, want.Metadata.Type)
	}
	fmt.Println()

	// Create chain builder
	builder := NewChainBuilder(config)

	// Register travel want types
	RegisterTravelWantTypes(builder)

	fmt.Println("🏁 Executing recipe-based travel planning...")
	builder.Execute()

	fmt.Println("\n📊 Final Want States:")
	states := builder.GetAllWantStates()
	for name, state := range states {
		fmt.Printf("  %s: %s (processed: %d)\n",
			name, state.Status, state.Stats.TotalProcessed)
	}

	fmt.Println("\n✅ Recipe-based travel planning completed!")
}