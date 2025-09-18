package main

import (
	"fmt"
)

func main() {
	fmt.Println("Prime Sieve Demo (YAML Config)")
	fmt.Println("==============================")

	// Load YAML configuration
	config, err := loadConfigFromYAML("config-prime.yaml")
	if err != nil {
		fmt.Printf("Error loading config-prime.yaml: %v\n", err)
		return
	}

	fmt.Printf("Loaded %d nodes from configuration\n", len(config.Nodes))

	// Create chain builder
	builder := NewChainBuilder(config)

	// Register prime node types
	RegisterPrimeNodeTypes(builder)

	fmt.Println("\nBuilding prime sieve chain...")
	err = builder.Build()
	if err != nil {
		fmt.Printf("❌ Build failed: %v\n", err)
		return
	}

	fmt.Println("✅ Build successful!")

	fmt.Println("\nExecuting prime sieve...")
	builder.Execute()

	fmt.Println("\n📊 Final Node States:")
	states := builder.GetAllNodeStates()
	for name, state := range states {
		fmt.Printf("  %s: %s (processed: %d)\n",
			name, state.Status, state.Stats.TotalProcessed)
	}

	fmt.Println("\n✅ Prime sieve execution completed!")
}
