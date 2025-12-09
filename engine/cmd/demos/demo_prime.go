package main

import (
	"fmt"
	"mywant/engine/cmd/types"
	. "mywant/engine/src"
	"os"
)

func main() {
	fmt.Println("Prime Sieve Demo (YAML Config)")
	fmt.Println("==============================")
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run demo_prime.go <config-file-path>")
		os.Exit(1)
	}
	configPath := os.Args[1]

	// Load YAML configuration
	config, err := LoadConfigFromYAML(configPath)
	if err != nil {
		fmt.Printf("Error loading %s: %v\n", configPath, err)
		return
	}

	fmt.Printf("Loaded %d wants from configuration\n", len(config.Wants))
	builder := NewChainBuilder(config)

	// Register prime node types
	types.RegisterPrimeWantTypes(builder)

	fmt.Println("\nExecuting prime sieve with reconcile loop...")
	builder.Execute()

	fmt.Println("\n📊 Final Execution State:")
	fmt.Printf("  Prime number processing completed")

	fmt.Println("\n✅ Prime sieve execution completed!")
}
