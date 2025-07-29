package main

import (
	"fmt"
)

func main() {
	fmt.Println("QNet Owner-Based Demo")
	fmt.Println("=====================")
	
	// Load YAML configuration with owner references
	config, err := loadConfigFromYAML("sample-owner-config-input.yaml")
	if err != nil {
		fmt.Printf("Error loading sample-owner-config-input.yaml: %v\n", err)
		return
	}

	fmt.Printf("Loaded %d nodes from configuration\n", len(config.Nodes))

	// Create chain builder
	builder := NewChainBuilder(config)
	
	// Register owner-based node types
	RegisterOwnerNodeTypes(builder)
	
	fmt.Println("\nExecuting owner-based chain with dynamic node creation...")
	
	// Memory dump will be automatically created as memory-*.yaml by the system
	
	// Execute using existing reconcile loop system
	builder.Execute()
	
	fmt.Println("\n📊 Final Node States:")
	states := builder.GetAllNodeStates()
	for name, state := range states {
		fmt.Printf("  %s: %s (processed: %d)\n", 
			name, state.Status, state.Stats.TotalProcessed)
	}
	
	// Memory snapshot is automatically saved to memory/memory-TIMESTAMP.yaml
	fmt.Println("✅ Owner-based execution completed successfully!")
}