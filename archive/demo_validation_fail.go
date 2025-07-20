package main

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
)

func main() {
	fmt.Println("QNet Validation Failure Demo")
	fmt.Println("============================")
	
	// Load invalid YAML configuration
	data, err := os.ReadFile("invalid_config.yaml")
	if err != nil {
		fmt.Printf("Error reading invalid_config.yaml: %v\n", err)
		return
	}

	var config Config
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		fmt.Printf("Error parsing YAML: %v\n", err)
		return
	}

	fmt.Printf("Loaded %d nodes from invalid configuration\n", len(config.Nodes))

	// Create chain builder
	builder := NewChainBuilder(config)
	
	// Register qnet node types
	RegisterQNetNodeTypes(builder)
	
	fmt.Println("\nBuilding chain with validation (should fail)...")
	err = builder.Build()
	if err != nil {
		fmt.Printf("❌ Expected validation failure: %v\n", err)
		fmt.Println("\n🔧 Validation is working correctly!")
		return
	}
	
	fmt.Println("❌ Validation should have failed but didn't!")
}