package main

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"io/ioutil"
	"strings"
)

func main() {
	fmt.Println("Label-Based Connection System Demo")
	fmt.Println("==================================")

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

	fmt.Printf("Loaded %d nodes from configuration\n", len(config.Nodes))

	// Create enhanced chain builder
	builder := NewChainBuilder(config)

	// Register node factories for enhanced nodes
	builder.RegisterNodeType("sequence", func(params map[string]interface{}) interface{} {
		rate := 1.0
		count := 5

		if r, ok := params["rate"]; ok {
			if rf, ok := r.(float64); ok {
				rate = rf
			}
		}
		if c, ok := params["count"]; ok {
			if ci, ok := c.(int); ok {
				count = ci
			} else if cf, ok := c.(float64); ok {
				count = int(cf)
			}
		}

		return CreateEnhancedGeneratorNode(rate, count)
	})

	builder.RegisterNodeType("queue", func(params map[string]interface{}) interface{} {
		serviceTime := 1.0

		if st, ok := params["service_time"]; ok {
			if stf, ok := st.(float64); ok {
				serviceTime = stf
			}
		}

		return CreateEnhancedQueueNode(serviceTime)
	})

	builder.RegisterNodeType("combiner", func(params map[string]interface{}) interface{} {
		return CreateEnhancedCombinerNode(2) // Default 2 inputs, will be adjusted dynamically
	})

	builder.RegisterNodeType("sink", func(params map[string]interface{}) interface{} {
		return CreateEnhancedSinkNode(1) // Default 1 input, will be adjusted dynamically
	})

	// Build nodes first
	err = builder.Build()
	if err != nil {
		fmt.Printf("Error building network: %v\n", err)
		return
	}

	// Generate paths from label-based connections
	fmt.Println("\nGenerating paths from label selectors...")
	nodePaths := builder.generatePathsFromConnections()

	// Display generated connections
	fmt.Println("\nGenerated Network Topology:")
	fmt.Println("===========================")
	for nodeName, paths := range nodePaths {
		node := builder.nodes[nodeName]
		fmt.Printf("\n%s (%s):\n", nodeName, node.metadata.Type)

		// Show inputs
		if len(paths.In) > 0 {
			fmt.Printf("  Inputs (%d):\n", len(paths.In))
			for i, inPath := range paths.In {
				fmt.Printf("    %d. %s (active: %t)\n", i+1, inPath.Name, inPath.Active)
			}
		} else {
			fmt.Printf("  Inputs: none (source node)\n")
		}

		// Show outputs
		if len(paths.Out) > 0 {
			fmt.Printf("  Outputs (%d):\n", len(paths.Out))
			for i, outPath := range paths.Out {
				fmt.Printf("    %d. %s (active: %t)\n", i+1, outPath.Name, outPath.Active)
			}
		} else {
			fmt.Printf("  Outputs: none (sink node)\n")
		}

		// Show labels for reference
		fmt.Printf("  Labels: %v\n", node.metadata.Labels)
	}

	// Validate connectivity
	fmt.Println("\nValidating connectivity requirements...")
	err = builder.validateConnections(nodePaths)
	if err != nil {
		fmt.Printf("❌ Validation failed: %v\n", err)
		return
	}
	fmt.Println("✅ All connectivity requirements satisfied!")

	// Show connection matrix
	fmt.Println("\nConnection Matrix:")
	fmt.Println("==================")
	fmt.Printf("%-20s -> %-20s\n", "Source", "Target")
	fmt.Println(strings.Repeat("-", 45))

	for sourceName, paths := range nodePaths {
		if len(paths.Out) == 0 {
			fmt.Printf("%-20s -> (terminal)\n", sourceName)
		}
		for _, outPath := range paths.Out {
			// Extract target name from path name (format: "to_targetname")
			targetName := outPath.Name[3:] // Remove "to_" prefix
			fmt.Printf("%-20s -> %-20s\n", sourceName, targetName)
		}
	}

	// Show label-based resolution process
	fmt.Println("\nLabel Resolution Examples:")
	fmt.Println("=========================")

	for _, node := range config.Nodes {
		if len(node.Spec.Connections.Outputs) > 0 {
			fmt.Printf("\nNode '%s' outputs:\n", node.Metadata.Name)
			for i, outputConn := range node.Spec.Connections.Outputs {
				fmt.Printf("  Output %d selector: %v\n", i+1, outputConn.Selector)

				// Show which nodes match this selector
				matches := builder.findNodesBySelector(outputConn.Selector)
				fmt.Printf("  Matches %d nodes: ", len(matches))
				for j, match := range matches {
					if j > 0 {
						fmt.Printf(", ")
					}
					fmt.Printf("%s", match.metadata.Name)
				}
				fmt.Printf("\n")
			}
		}
	}

	fmt.Println("\nLabel-based connection system working correctly! 🎉")
	fmt.Println("\nKey Benefits:")
	fmt.Println("✓ Order-independent node definition")
	fmt.Println("✓ Flexible label-based routing")
	fmt.Println("✓ Dynamic path generation")
	fmt.Println("✓ Automatic connectivity validation")
	fmt.Println("✓ Self-documenting topology through labels")
}
