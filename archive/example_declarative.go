package main

import (
	"encoding/json"
	"fmt"
	"os"
)

// loadConfigFromJSON loads configuration from a JSON file
func loadConfigFromJSON(filename string) (Config, error) {
	var config Config
	
	data, err := os.ReadFile(filename)
	if err != nil {
		return config, fmt.Errorf("failed to read config file: %w", err)
	}
	
	err = json.Unmarshal(data, &config)
	if err != nil {
		return config, fmt.Errorf("failed to parse JSON config: %w", err)
	}
	
	return config, nil
}

func main() {
	fmt.Println("Loading configuration from config.json...")
	
	// Load configuration from JSON file
	config, err := loadConfigFromJSON("config.json")
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		return
	}
	
	fmt.Printf("Loaded configuration with %d nodes\n", len(config.Nodes))
	
	// Build and execute the chain
	builder := NewChainBuilder(config)
	
	// Register queueing network node types
	RegisterQNetNodeTypes(builder)
	
	err = builder.Build()
	if err != nil {
		fmt.Printf("Error building chain: %v\n", err)
		return
	}
	
	fmt.Println("Executing declarative chain from JSON configuration...")
	builder.Execute()
}