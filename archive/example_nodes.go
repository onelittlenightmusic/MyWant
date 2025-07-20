package main

import (
	"fmt"
	"os"
	"gopkg.in/yaml.v3"
)

// Simple YAML loader
func loadYAML(filename string) (Config, error) {
	var config Config
	data, err := os.ReadFile(filename)
	if err != nil {
		return config, err
	}
	return config, yaml.Unmarshal(data, &config)
}

func main() {
	fmt.Println("Node-Based Queueing Network")
	fmt.Println("===========================")
	
	// Load config
	config, err := loadYAML("simple_combiner_config.yaml")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	
	fmt.Printf("Loaded %d nodes\n", len(config.Nodes))
	
	// Build and run
	builder := NewChainBuilder(config)
	RegisterNodeBasedQNetTypes(builder)
	
	if err := builder.Build(); err != nil {
		fmt.Printf("Build error: %v\n", err)
		return
	}
	
	fmt.Println("Running node-based simulation...")
	builder.Execute()
	fmt.Println("Simulation complete!")
}