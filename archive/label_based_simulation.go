package main

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"io/ioutil"
	"strings"
	"sync"
	"time"
)

func main() {
	fmt.Println("Label-Based Dynamic QNet Simulation")
	fmt.Println("===================================")
	
	// Load the label-based configuration
	yamlData, err := ioutil.ReadFile("config.yaml")
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
	
	// Register node factories with proper parameter handling
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
		return CreateEnhancedCombinerNode(2) // Will be adjusted dynamically
	})
	
	builder.RegisterNodeType("sink", func(params map[string]interface{}) interface{} {
		return CreateEnhancedSinkNode(1) // Will be adjusted dynamically
	})
	
	// Build nodes first
	err = builder.Build()
	if err != nil {
		fmt.Printf("Error building network: %v\n", err)
		return
	}
	
	// Generate paths from label-based connections
	fmt.Println("\nGenerating dynamic paths from label selectors...")
	nodePaths := builder.generatePathsFromConnections()
	
	// Initialize enhanced nodes with their dynamically generated path counts
	fmt.Println("Initializing nodes with dynamic path counts...")
	enhancedNodes := make(map[string]EnhancedBaseNode)
	
	for nodeName, node := range builder.nodes {
		if enhancedNode, ok := node.function.(EnhancedBaseNode); ok {
			paths := nodePaths[nodeName]
			enhancedNode.InitializePaths(len(paths.In), len(paths.Out))
			enhancedNodes[nodeName] = enhancedNode
			
			fmt.Printf("  %s: %dIn/%dOut paths\n", nodeName, len(paths.In), len(paths.Out))
		}
	}
	
	// Validate connectivity
	err = builder.validateConnections(nodePaths)
	if err != nil {
		fmt.Printf("❌ Validation failed: %v\n", err)
		return
	}
	fmt.Println("✅ All connectivity requirements satisfied!")
	
	// Display network topology
	fmt.Println("\nDynamic Network Topology:")
	fmt.Println("========================")
	for nodeName, paths := range nodePaths {
		node := builder.nodes[nodeName]
		fmt.Printf("%s (%s): %dIn/%dOut\n", 
			nodeName, node.metadata.Type, len(paths.In), len(paths.Out))
	}
	
	// Start simulation
	fmt.Println("\nStarting label-based simulation...")
	fmt.Println(strings.Repeat("-", 50))
	
	var wg sync.WaitGroup
	
	// Start all nodes as goroutines
	nodeNames := []string{"gen-primary", "gen-secondary", "queue-main-1", "queue-alt-1", "combiner-main", "queue-final", "collector-end"}
	wg.Add(len(nodeNames))
	
	for _, nodeName := range nodeNames {
		go func(name string) {
			defer wg.Done()
			
			enhancedNode := enhancedNodes[name]
			paths := nodePaths[name]
			
			fmt.Printf("[%s] Starting...\n", strings.ToUpper(name))
			
			for !enhancedNode.Process(paths) {
				time.Sleep(50 * time.Millisecond)
			}
			
			fmt.Printf("[%s] Finished\n", strings.ToUpper(name))
		}(nodeName)
	}
	
	// Wait for simulation to complete
	wg.Wait()
	
	// Display final statistics
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("LABEL-BASED SIMULATION COMPLETE - Final Statistics")
	fmt.Println(strings.Repeat("=", 60))
	
	for _, nodeName := range nodeNames {
		enhancedNode := enhancedNodes[nodeName]
		stats := enhancedNode.GetStats()
		meta := enhancedNode.GetConnectivityMetadata()
		paths := nodePaths[nodeName]
		
		fmt.Printf("\n%s (%s):\n", nodeName, meta.NodeType)
		fmt.Printf("  Connectivity: %dIn/%dOut (req: %dIn/%dOut)\n",
			len(paths.In), len(paths.Out), 
			meta.RequiredInputs, meta.RequiredOutputs)
		for key, value := range stats {
			fmt.Printf("  %s: %v\n", key, value)
		}
	}
	
	// Calculate performance metrics
	fmt.Println("\nLabel-Based Network Performance:")
	fmt.Println("===============================")
	
	gen1Stats := enhancedNodes["gen-primary"].GetStats()
	gen2Stats := enhancedNodes["gen-secondary"].GetStats()
	sinkStats := enhancedNodes["collector-end"].GetStats()
	combinerStats := enhancedNodes["combiner-main"].GetStats()
	
	totalGenerated := gen1Stats["generated"].(int) + gen2Stats["generated"].(int)
	totalReceived := sinkStats["received"].(int)
	
	fmt.Printf("Total Packets Generated: %d\n", totalGenerated)
	fmt.Printf("Total Packets Received:  %d\n", totalReceived)
	fmt.Printf("Packet Loss Rate:        %.2f%%\n", 
		float64(totalGenerated-totalReceived)/float64(totalGenerated)*100)
	fmt.Printf("Combiner Merged:         %d packets\n", combinerStats["merged_packets"].(int))
	
	// Display queue wait times summary
	fmt.Println("\nQueue Average Wait Times:")
	fmt.Println("========================")
	for _, nodeName := range nodeNames {
		if enhancedNode := enhancedNodes[nodeName]; enhancedNode.GetType() == "queue" {
			stats := enhancedNode.GetStats()
			if processed, ok := stats["processed"].(int); ok && processed > 0 {
				avgDelay := stats["average_delay"].(float64)
				serviceTime := stats["service_time"].(float64)
				fmt.Printf("%-15s: %.3fs wait time (%.2fs service time, %d packets)\n", 
					nodeName, avgDelay, serviceTime, processed)
			} else {
				fmt.Printf("%-15s: No packets processed\n", nodeName)
			}
		}
	}
	
	// Show label-based benefits
	fmt.Println("\nLabel-Based System Benefits Demonstrated:")
	fmt.Println("========================================")
	fmt.Println("✅ Configuration defined purely through labels")
	fmt.Println("✅ Paths generated dynamically at runtime")
	fmt.Println("✅ Order-independent node definition")
	fmt.Println("✅ Automatic connectivity validation")
	fmt.Println("✅ Self-documenting network topology")
	fmt.Println("✅ Flexible routing through label selectors")
	fmt.Println("✅ Real-time path statistics and monitoring")
	
	fmt.Println("\n🎉 Label-based dynamic QNet simulation successful!")
}