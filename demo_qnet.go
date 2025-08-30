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

	fmt.Printf("Loaded %d nodes from configuration\n", len(config.Wants))

	// Create chain builder
	builder := NewChainBuilder(config)
	
	// Register qnet node types
	RegisterQNetWantTypes(builder)
	
	fmt.Println("\nExecuting qnet simulation with reconcile loop...")
	builder.Execute()
	
	fmt.Println("\n📊 Final Node States:")
	states := builder.GetAllWantStates()
	for name, state := range states {
		fmt.Printf("  %s: %s (processed: %d)\n", 
			name, state.Status, state.Stats.TotalProcessed)
	}
	
	fmt.Println("\n✅ QNet execution completed successfully!")
}