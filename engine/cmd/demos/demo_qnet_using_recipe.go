package demos_moved

import (
	"fmt"
	_ "mywant/engine/types"
	. "mywant/engine/core"
	"os"
)

func RunDemoQnetUsingRecipe() {
	fmt.Println("🔗 QNet Demo with Using Fields from YAML (Not Hardcoded)")
	fmt.Println("=========================================================")
	fmt.Println("This demo shows using field connections defined in YAML:")
	fmt.Println("- queue-primary using: [role: source, stream: primary]")
	fmt.Println("- queue-secondary using: [role: source, stream: secondary]")
	fmt.Println("- combiner-main using: [role: processor, stage: first]")
	fmt.Println("- queue-final using: [role: merger]")
	fmt.Println("- collector-end using: [role: processor, stage: final]")
	fmt.Println()
	yamlFile := "yaml/config/config-qnet-using-recipe.yaml"
	if len(os.Args) > 1 {
		yamlFile = os.Args[1]
	}

	// Load YAML configuration with using fields
	config, err := LoadConfigFromYAML(yamlFile)
	if err != nil {
		fmt.Printf("Error loading %s: %v\n", yamlFile, err)
		return
	}

	fmt.Printf("Loaded %d wants from YAML configuration\n", len(config.Wants))

	// Show the using field connections defined in YAML
	fmt.Println("\n🔗 Using Field Connections (from YAML, not Go code):")
	for _, want := range config.Wants {
		if len(want.Spec.Using) > 0 {
			fmt.Printf("  %s -> using: %v\n", want.Metadata.Name, want.Spec.Using)
		}
	}
	fmt.Println()
	builder := NewChainBuilder(config)

	// Register qnet want types

	fmt.Println("🚀 Executing qnet with using fields from YAML...")
	builder.Execute()

	fmt.Println("\n📊 Final Node States:")
	fmt.Printf("  QNet with YAML-defined using fields completed")

	fmt.Println("\n✅ QNet execution completed!")
	fmt.Println("🔗 All using field connections were loaded from YAML, not hardcoded in Go!")
}
