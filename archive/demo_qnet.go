package main

import (
	"fmt"
)

func main() {
	fmt.Println("QNet Validation Demo")
	fmt.Println("===================")

	// Load YAML configuration
	config, err := loadConfigFromYAML("config-qnet.yaml")
	if err != nil {
		fmt.Printf("Error loading config-qnet.yaml: %v\n", err)
		return
	}

	fmt.Printf("Loaded %d nodes from configuration\n", len(config.Nodes))

	// Create chain builder
	builder := NewChainBuilder(config)

	// Register qnet node types
	RegisterQNetNodeTypes(builder)

	fmt.Println("\nBuilding chain with validation...")
	err = builder.Build()
	if err != nil {
		fmt.Printf("❌ Build failed: %v\n", err)
		return
	}

	fmt.Println("✅ Validation passed - all connectivity requirements satisfied!")

	fmt.Println("\nExecuting qnet simulation...")
	builder.Execute()

	fmt.Println("\n📊 Final Node States:")
	states := builder.GetAllNodeStates()
	for name, state := range states {
		fmt.Printf("  %s: %s (processed: %d)\n",
			name, state.Status, state.Stats.TotalProcessed)
	}

	fmt.Println("\n✅ QNet execution completed successfully!")
}
