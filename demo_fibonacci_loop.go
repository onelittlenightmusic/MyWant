package main

import (
	"fmt"
)

func main() {
	fmt.Println("🔄 Fibonacci Loop Demo (Advanced Architecture)")
	fmt.Println("==============================================")
	fmt.Println("This demo showcases a fibonacci sequence generator using:")
	fmt.Println("• Seed Generator: Provides initial values (0, 1)")
	fmt.Println("• Fibonacci Computer: Calculates next numbers in sequence")
	fmt.Println("• Merger: Creates feedback loop combining seeds + computed values")
	fmt.Println("• Sink: Collects and displays the complete sequence")
	fmt.Println("")
	
	// Load YAML configuration
	config, err := loadConfigFromYAML("config-fibonacci-loop.yaml")
	if err != nil {
		fmt.Printf("Error loading config-fibonacci-loop.yaml: %v\n", err)
		return
	}

	fmt.Printf("Loaded %d nodes from configuration\n", len(config.Nodes))
	fmt.Println("")

	// Create chain builder
	builder := NewChainBuilder(config)
	
	// Register fibonacci loop node types
	RegisterFibonacciLoopNodeTypes(builder)
	
	fmt.Println("🚀 Executing fibonacci loop with reconcile system...")
	fmt.Println("")
	builder.Execute()
	
	fmt.Println("📊 Final Node States:")
	states := builder.GetAllNodeStates()
	for name, state := range states {
		fmt.Printf("  %s: %s (processed: %d)\n", 
			name, state.Status, state.Stats.TotalProcessed)
	}
	
	fmt.Println("")
	fmt.Println("✅ Fibonacci loop execution completed successfully!")
	fmt.Println("🔄 The feedback loop architecture successfully generated the sequence!")
}