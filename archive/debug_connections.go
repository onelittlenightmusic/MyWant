package main

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"io/ioutil"
)

func main() {
	fmt.Println("Debug Label-Based Connections")
	fmt.Println("=============================")
	
	// Load the label-based configuration
	yamlData, err := ioutil.ReadFile("enhanced_config.yaml")
	if err != nil {
		fmt.Printf("Error reading config file: %v\n", err)
		return
	}
	
	var config Config
	err = yaml.Unmarshal(yamlData, &config)
	if err != nil {
		fmt.Printf("Error parsing YAML: %v\n", err)
		return
	}
	
	fmt.Printf("Loaded %d nodes from configuration\n\n", len(config.Nodes))
	
	// Debug each node's configuration
	for i, node := range config.Nodes {
		fmt.Printf("Node %d: %s\n", i+1, node.Metadata.Name)
		fmt.Printf("  Type: %s\n", node.Metadata.Type)
		fmt.Printf("  Labels: %v\n", node.Metadata.Labels)
		fmt.Printf("  Params: %v\n", node.Spec.Params)
		
		// Check connections
		fmt.Printf("  Input connections: %d\n", len(node.Spec.Connections.Inputs))
		for j, input := range node.Spec.Connections.Inputs {
			fmt.Printf("    Input %d selector: %v\n", j+1, input.Selector)
		}
		
		fmt.Printf("  Output connections: %d\n", len(node.Spec.Connections.Outputs))
		for j, output := range node.Spec.Connections.Outputs {
			fmt.Printf("    Output %d selector: %v\n", j+1, output.Selector)
		}
		fmt.Println()
	}
	
	// Create builder and register factories
	builder := NewChainBuilder(config)
	
	// Simple factories that just create the objects without using params yet
	builder.RegisterNodeType("sequence", func(params map[string]interface{}) interface{} {
		return CreateEnhancedGeneratorNode(1.0, 5)
	})
	
	builder.RegisterNodeType("queue", func(params map[string]interface{}) interface{} {
		return CreateEnhancedQueueNode(1.0)
	})
	
	builder.RegisterNodeType("combiner", func(params map[string]interface{}) interface{} {
		return CreateEnhancedCombinerNode(2)
	})
	
	builder.RegisterNodeType("sink", func(params map[string]interface{}) interface{} {
		return CreateEnhancedSinkNode(1)
	})
	
	// Build nodes
	fmt.Println("Building nodes...")
	err = builder.Build()
	if err != nil {
		fmt.Printf("Error building network: %v\n", err)
		return
	}
	
	fmt.Printf("Created %d runtime nodes\n", len(builder.nodes))
	
	// Test selector matching
	fmt.Println("\nTesting selector matching:")
	for _, node := range config.Nodes {
		for j, output := range node.Spec.Connections.Outputs {
			fmt.Printf("Node '%s' output %d selector %v matches:\n", 
				node.Metadata.Name, j+1, output.Selector)
			
			matches := builder.findNodesBySelector(output.Selector)
			if len(matches) == 0 {
				fmt.Println("  No matches!")
			} else {
				for _, match := range matches {
					fmt.Printf("  - %s (labels: %v)\n", 
						match.metadata.Name, match.metadata.Labels)
				}
			}
		}
	}
}