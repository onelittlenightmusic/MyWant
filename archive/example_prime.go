package main

import (
	"fmt"
	"os"
	"strings"
	"gopkg.in/yaml.v3"
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
	fmt.Println("Loading prime sieve configuration from prime_config.yaml...")
	
	// Load configuration from YAML file
	config, err := loadConfigFromYAML("prime_config.yaml")
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		return
	}
	
	fmt.Printf("Loaded prime sieve configuration with %d nodes\n", len(config.Nodes))
	
	// Print node summary
	for _, node := range config.Nodes {
		fmt.Printf("  - %s (%s): %v\n", node.Metadata.Name, node.Metadata.Type, node.Metadata.Labels)
	}
	
	// Build and execute the chain
	builder := NewChainBuilder(config)
	
	// Register prime sieve node types
	RegisterPrimeNodeTypes(builder)
	
	err = builder.Build()
	if err != nil {
		fmt.Printf("Error building chain: %v\n", err)
		return
	}
	
	fmt.Println("\nExecuting declarative prime sieve from YAML configuration...")
	fmt.Println("Sieve of Eratosthenes - finding primes from 2 to 100:")
	fmt.Println(strings.Repeat("=", 50))
	
	builder.Execute()
}