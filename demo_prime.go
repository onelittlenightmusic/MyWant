package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Println("Prime Sieve Demo (YAML Config)")
	fmt.Println("==============================")
	
	// Get YAML file from command line argument
	yamlFile := "config/config-prime.yaml"
	if len(os.Args) > 1 {
		yamlFile = os.Args[1]
	}
	
	// Load YAML configuration
	config, err := loadConfigFromYAML(yamlFile)
	if err != nil {
		fmt.Printf("Error loading %s: %v\n", yamlFile, err)
		return
	}

	fmt.Printf("Loaded %d wants from configuration\n", len(config.Wants))

	// Create chain builder
	builder := NewChainBuilder(config)
	
	// Register prime node types
	RegisterPrimeWantTypes(builder)

	fmt.Println("\nExecuting prime sieve with reconcile loop...")
	builder.Execute()

	fmt.Println("\n📊 Final Execution State:")
	fmt.Printf("  Prime number processing completed")

	fmt.Println("\n✅ Prime sieve execution completed!")
}