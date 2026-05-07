package main

import (
	"fmt"
	. "mywant/engine/core"
	_ "mywant/engine/types"
	"os"
)

func main() {
	fmt.Println("🔄 Queue System Generic Recipe Demo")
	fmt.Println("====================================")
	fmt.Println("Loading queue system from new generic recipe format:")
	fmt.Println("- Recipe defines: generator, queue, sink wants")
	fmt.Println("- Uses unified generic recipe structure")
	fmt.Println()
	yamlFile := "../examples/configs/config-queue-system-recipe.yaml"
	if len(os.Args) > 1 {
		yamlFile = os.Args[1]
	}

	// Load configuration using generic recipe loader
	config, params, err := LoadRecipeWithConfig(yamlFile)
	if err != nil {
		fmt.Printf("Error loading recipe config %s: %v\n", yamlFile, err)
		return
	}

	fmt.Printf("📋 Recipe parameters: %+v\n", params)
	fmt.Printf("✅ Generated %d wants from recipe\n", len(config.Wants))
	for _, want := range config.Wants {
		fmt.Printf("  - %s (%s)\n", want.Metadata.Name, want.Metadata.Type)
	}
	fmt.Println()
	builder := NewChainBuilder(config)

	// Register qnet want types (includes sequence, queue, sink)

	fmt.Println("🏁 Executing generic recipe-based queue system...")
	builder.Execute()

	fmt.Println("\n📊 Final Execution State:")
	fmt.Printf("  Generic recipe queue system completed")

	fmt.Println("\n✅ Generic recipe queue system completed!")
}
