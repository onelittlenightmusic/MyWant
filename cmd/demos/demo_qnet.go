package main

import (
	"fmt"
	mywant "mywant/src"
)

func main() {
	fmt.Println("QNet Validation Demo")
	fmt.Println("===================")
	
	// Load YAML configuration
	config, err := mywant.LoadConfigFromYAML("config/config-qnet.yaml")
	if err != nil {
		fmt.Printf("Error loading config/config-qnet.yaml: %v\n", err)
		return
	}

	fmt.Printf("Loaded %d nodes from configuration\n", len(config.Wants))

	// Create chain builder
	builder := mywant.NewChainBuilder(config)
	
	// Register qnet node types
	RegisterQNetWantTypes(builder)
	
	fmt.Println("\nExecuting qnet simulation with reconcile loop...")
	builder.Execute()
	
	fmt.Println("\n📊 Final Node States:")
	states := builder.GetAllWantStates()
	for name, state := range states {
		fmt.Printf("  %s: %s (processed: %v)\n", 
			name, state.Status, state.State["total_processed"])
	}
	
	fmt.Println("\n✅ QNet execution completed successfully!")
}