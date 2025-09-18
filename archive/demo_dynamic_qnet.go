package main

import (
	"fmt"
	"time"
)

func main() {
	fmt.Println("🚀 Dynamic QNet Demo")
	fmt.Println("===================")
	fmt.Println("Building queueing network entirely through dynamic additions using config-qnet.yaml")

	// Load configuration from YAML file
	config, err := loadConfigFromYAML("config-qnet.yaml")
	if err != nil {
		fmt.Printf("❌ Failed to load config: %v\n", err)
		return
	}

	// Create dynamic chain builder
	builder := NewDynamicChainBuilder()
	RegisterQNetNodeTypes(builder)

	// Start execution mode with no predefined nodes
	builder.ExecuteDynamic()
	time.Sleep(200 * time.Millisecond)

	// Add all nodes from config-qnet.yaml at once to avoid precheck timing issues
	fmt.Printf("\n🔧 Adding all %d nodes from config-qnet.yaml at once...\n", len(config.Nodes))

	for i, node := range config.Nodes {
		stepNum := i + 1
		fmt.Printf("🔧 Step %d: Adding %s (%s)...\n", stepNum, node.Metadata.Name, node.Metadata.Type)

		err := builder.AddNode(node)
		if err != nil {
			fmt.Printf("❌ Failed to add %s: %v\n", node.Metadata.Name, err)
			return
		}
		fmt.Printf("✅ %s added!\n", node.Metadata.Name)

		// No sleep - add all nodes quickly to avoid timing issues
	}

	// Show current chain topology
	fmt.Println("\n📊 Dynamic QNet Topology Built from config-qnet.yaml:")
	fmt.Println("   gen-primary → queue-primary ↘")
	fmt.Println("                                combiner → queue-final → collector")
	fmt.Println("   gen-secondary → queue-secondary ↗")

	// Show current state
	fmt.Println("\n📈 Current Node States:")
	states := builder.GetAllNodeStates()
	for name, state := range states {
		fmt.Printf("  • %s: %s (processed: %d)\n",
			name, state.GetStatus(), state.Stats.TotalProcessed)
	}

	// Wait for processing to complete
	fmt.Println("\n⏱️  Waiting for queueing network to complete...")

	// Add timeout mechanism to force completion for demonstration
	done := make(chan bool, 1)
	go func() {
		builder.WaitForCompletion()
		done <- true
	}()

	go func() {
		time.Sleep(120 * time.Second)
		fmt.Println("\n⏰ Forcing completion after 120 seconds for demonstration...")
		builder.Stop()
		done <- true
	}()

	<-done

	// Show final results
	fmt.Println("\n🎯 Final QNet Results:")
	finalStates := builder.GetAllNodeStates()
	totalPackets := 0
	for name, state := range finalStates {
		fmt.Printf("  • %s: %s (processed: %d)\n",
			name, state.GetStatus(), state.Stats.TotalProcessed)
		if name == "collector-end" {
			totalPackets = int(state.Stats.TotalProcessed)
		}
	}

	fmt.Printf("\n🎉 Dynamic QNet completed successfully!\n")
	fmt.Printf("📦 Total packets processed: %d\n", totalPackets)
	fmt.Printf("🏗️  Network built entirely through %d dynamic additions from config-qnet.yaml\n", len(finalStates))
	fmt.Println("✨ Topology: Primary & Secondary Streams → Queues → Combiner → Final Queue → Collector")

	// Dump node memory to YAML file
	fmt.Println("\n📝 Dumping dynamic QNet node memory to YAML...")
	err = builder.dumpNodeMemoryToYAML()
	if err != nil {
		fmt.Printf("❌ Failed to dump node memory: %v\n", err)
	} else {
		fmt.Println("✅ Dynamic QNet node memory dumped successfully!")
	}
}
