package main

import (
	"fmt"
)

func main() {
	fmt.Println("🔄 Queue System Generic Recipe Demo")
	fmt.Println("====================================")
	fmt.Println("Loading queue system from new generic recipe format:")
	fmt.Println("- Recipe defines: generator, queue, sink wants")
	fmt.Println("- Uses unified generic recipe structure")
	fmt.Println()

	// Load configuration using generic recipe loader
	config, params, err := LoadRecipeWithConfig("config-queue-system-template.yaml")
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

	// Register qnet want types (includes sequence, queue, sink)
	RegisterQNetWantTypes(builder)

	fmt.Println("🏁 Executing generic recipe-based queue system...")
	builder.Execute()

	fmt.Println("\n📊 Final Want States:")
	states := builder.GetAllWantStates()
	for name, state := range states {
		fmt.Printf("  %s: %s (processed: %d)\n",
			name, state.Status, state.Stats.TotalProcessed)
	}

	fmt.Println("\n✅ Generic recipe queue system completed!")
}