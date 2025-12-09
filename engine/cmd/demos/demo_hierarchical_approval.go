package main

import (
	"fmt"
	"mywant/engine/cmd/types"
	mywant "mywant/engine/src"
	"os"
	"time"
)

func main() {
	fmt.Printf("🎯 Starting Hierarchical Approval Demo\n")
	fmt.Printf("======================================\n")
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run demo_hierarchical_approval.go <config-file-path>")
		os.Exit(1)
	}
	configPath := os.Args[1]

	// Load configuration
	config, err := mywant.LoadConfigFromYAML(configPath)
	if err != nil {
		fmt.Printf("Error loading %s: %v\n", configPath, err)
		os.Exit(1)
	}
	builder := mywant.NewChainBuilder(config)

	// Custom target types are auto-registered during builder creation

	// Register approval want types
	types.RegisterApprovalWantTypes(builder)

	fmt.Printf("📋 Configuration loaded with %d wants\n", len(config.Wants))

	// Display wants for debugging
	for i, want := range config.Wants {
		fmt.Printf("  %d. %s (%s)\n", i+1, want.Metadata.Name, want.Metadata.Type)
		if want.Metadata.Labels != nil {
			for k, v := range want.Metadata.Labels {
				fmt.Printf("     %s: %s\n", k, v)
			}
		}
	}

	// Execute the chain
	fmt.Printf("\n🚀 Executing hierarchical approval workflow...\n")
	builder.Execute()

	fmt.Printf("\n✅ Hierarchical approval workflow completed successfully!\n")

	// Display final statistics - simplified for now
	fmt.Printf("\n📊 Final Statistics:\n")
	fmt.Printf("===================\n")
	fmt.Printf("Hierarchical approval system executed successfully\n")

	// Keep the demo running for a moment to show results
	time.Sleep(2 * time.Second)
	fmt.Printf("\n🏁 Demo completed\n")
}
