package main

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
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
	fmt.Println("Simple Network with Combiner")
	fmt.Println("============================")

	// Load config
	config, err := loadYAML("simple_combiner_config.yaml")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Loaded %d nodes\n", len(config.Nodes))

	// Build and run
	builder := NewChainBuilder(config)
	RegisterSimpleQNetNodeTypes(builder)

	if err := builder.Build(); err != nil {
		fmt.Printf("Build error: %v\n", err)
		return
	}

	fmt.Println("Running simulation with two streams merging...")
	builder.Execute()
	fmt.Println("Done!")
}
