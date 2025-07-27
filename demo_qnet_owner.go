package main

import (
	"fmt"
	"time"
)

func main() {
	fmt.Println("QNet Owner-Based Demo")
	fmt.Println("=====================")
	
	// Load YAML configuration with owner references
	config, err := loadConfigFromYAML("sample-owner-config.yaml")
	if err != nil {
		fmt.Printf("Error loading sample-owner-config.yaml: %v\n", err)
		return
	}

	fmt.Printf("Loaded %d nodes from configuration\n", len(config.Nodes))

	// Create chain builder
	builder := NewChainBuilder(config)
	
	// Register owner-based node types
	RegisterOwnerNodeTypes(builder)
	
	fmt.Println("\nExecuting owner-based chain with dynamic node creation...")
	
	// Set memory file for snapshot creation
	memoryFile := fmt.Sprintf("memory/sample_owner-memory-%s.yaml", 
		time.Now().Format("20060102-150405"))
	builder.SetMemoryPath(memoryFile)
	
	// Execute using existing reconcile loop system
	builder.Execute()
	
	fmt.Println("\n📊 Final Node States:")
	states := builder.GetAllNodeStates()
	for name, state := range states {
		fmt.Printf("  %s: %s (processed: %d)\n", 
			name, state.Status, state.Stats.TotalProcessed)
	}
	
	fmt.Printf("\n💾 Memory snapshot saved to: %s\n", memoryFile)
	fmt.Println("✅ Owner-based execution completed successfully!")
}