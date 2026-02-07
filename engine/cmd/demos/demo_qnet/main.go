package main

import (
	"fmt"
	_ "mywant/engine/cmd/types"
	mywant "mywant/engine/src"
	"os"
)

func main() {
	fmt.Println("QNet Validation Demo")
	fmt.Println("===================")
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run demo_qnet.go <config-file-path>")
		os.Exit(1)
	}
	configPath := os.Args[1]

	// Load YAML configuration
	config, err := mywant.LoadConfigFromYAML(configPath)
	if err != nil {
		fmt.Printf("Error loading %s: %v\n", configPath, err)
		return
	}

	fmt.Printf("Loaded %d nodes from configuration\n", len(config.Wants))
	builder := mywant.NewChainBuilder(config)

	// Register qnet node types
	

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
