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
	fmt.Println("Simple Queueing Network Example")
	fmt.Println("===============================")
	
	// Load config
	config, err := loadYAML("simple_config.yaml")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	
	// Build and run
	builder := NewChainBuilder(config)
	RegisterSimpleQNetNodeTypes(builder)
	
	if err := builder.Build(); err != nil {
		fmt.Printf("Build error: %v\n", err)
		return
	}
	
	fmt.Println("Running simulation...")
	builder.Execute()
	fmt.Println("Done!")
}