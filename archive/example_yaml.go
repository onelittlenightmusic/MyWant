package main

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
)

// loadConfigFromYAML loads configuration from a YAML file
func loadConfigFromYAML(filename string) (Config, error) {
	var config Config

	data, err := os.ReadFile(filename)
	if err != nil {
		return config, fmt.Errorf("failed to read config file: %w", err)
	}

	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return config, fmt.Errorf("failed to parse YAML config: %w", err)
	}

	return config, nil
}

func main() {
	fmt.Println("Loading configuration from config_clean.yaml...")

	// Load configuration from YAML file
	config, err := loadConfigFromYAML("config_clean.yaml")
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		return
	}

	fmt.Printf("Loaded YAML configuration with %d nodes\n", len(config.Nodes))

	// Print node summary
	for _, node := range config.Nodes {
		fmt.Printf("  - %s (%s): %v\n", node.Metadata.Name, node.Metadata.Type, node.Metadata.Labels)
	}

	// Build and execute the chain
	builder := NewChainBuilder(config)

	// Register queueing network node types
	RegisterQNetNodeTypes(builder)

	err = builder.Build()
	if err != nil {
		fmt.Printf("Error building chain: %v\n", err)
		return
	}

	fmt.Println("\nExecuting declarative chain from YAML configuration...")
	builder.Execute()
}
