package main

import (
	"fmt"
	_ "mywant/engine/types"
	. "mywant/engine/core"
	"os"
)

func main() {
	fmt.Println("🔄 QNet Recipe Demo with Parameterized Using Fields")
	fmt.Println("===================================================")
	fmt.Println("Loading qnet system from recipe with:")
	fmt.Println("- Dual generators (primary/secondary streams)")
	fmt.Println("- Parallel processing queues")
	fmt.Println("- Stream combiner with recipe-defined using fields")
	fmt.Println("- Final processing and collection")
	fmt.Println("- All using connections defined in recipe YAML")
	fmt.Println()
	yamlFile := "yaml/config/config-qnet-recipe.yaml"
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
		if len(want.Spec.Using) > 0 {
			fmt.Printf("    using: %v\n", want.Spec.Using)
		}
	}
	fmt.Println()
	builder := NewChainBuilder(config)

	// Register qnet want types (includes sequence, queue, combiner, sink)

	fmt.Println("🏁 Executing recipe-based qnet with parameterized using fields...")
	builder.Execute()

	fmt.Println("\n📊 Final Execution State:")
	fmt.Printf("  Recipe-based qnet with parameterized connections completed")

	fmt.Println("\n✅ Recipe-based qnet execution completed!")
	fmt.Println("🔗 All using field connections were defined in the recipe YAML!")
}
