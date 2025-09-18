package main

import (
	"fmt"
	. "mywant/src"
	"os"
)

func main() {
	fmt.Println("Fibonacci Sequence Demo (YAML Config)")
	fmt.Println("=====================================")

	// Get YAML file from command line argument
	yamlFile := "config/config-fibonacci.yaml"
	if len(os.Args) > 1 {
		yamlFile = os.Args[1]
	}

	// Load YAML configuration
	config, err := LoadConfigFromYAML(yamlFile)
	if err != nil {
		fmt.Printf("Error loading %s: %v\n", yamlFile, err)
		return
	}

	fmt.Printf("Loaded %d wants from configuration\n", len(config.Wants))

	// Create chain builder
	builder := NewChainBuilder(config)

	// Register fibonacci node types
	RegisterFibonacciWantTypes(builder)

	// Register owner/target types for Target system support
	RegisterOwnerWantTypes(builder)

	fmt.Println("\nExecuting fibonacci sequence with reconcile loop...")
	builder.Execute()

	fmt.Println("\n📊 Final Execution State:")
	fmt.Printf("  Fibonacci sequence execution completed")

	fmt.Println("\n✅ Fibonacci sequence execution completed!")
}
