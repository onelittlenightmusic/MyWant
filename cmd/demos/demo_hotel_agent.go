package main

import (
	"fmt"
	"log"
	"os"
	"time"

	mywant "mywant/src"
)

func main() {
	fmt.Println("🏨 Starting Hotel Agent Demo")

	// Load configuration
	config, err := mywant.LoadConfigFromYAML("config/config-hotel-agent.yaml")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Create chain builder
	builder := mywant.NewChainBuilder(config)

	// Create and configure agent registry
	agentRegistry := mywant.NewAgentRegistry()

	// Load capabilities and agents
	if err := agentRegistry.LoadCapabilities("capabilities/"); err != nil {
		log.Fatalf("Failed to load capabilities: %v", err)
	}

	if err := agentRegistry.LoadAgents("agents/"); err != nil {
		log.Fatalf("Failed to load agents: %v", err)
	}

	// Register hotel want types with agent registry
	mywant.RegisterHotelWantTypes(builder, agentRegistry)

	fmt.Printf("📋 Loaded %d wants\n", len(config.Wants))

	// List loaded capabilities and agents
	fmt.Println("\n🔧 Loaded Capabilities:")
	for _, want := range config.Wants {
		if len(want.Spec.Requires) > 0 {
			fmt.Printf("  Want '%s' requires: %v\n", want.Metadata.Name, want.Spec.Requires)
			for _, req := range want.Spec.Requires {
				agents := agentRegistry.FindAgentsByGives(req)
				fmt.Printf("    Agents for '%s': ", req)
				for _, agent := range agents {
					fmt.Printf("%s(%s) ", agent.GetName(), agent.GetType())
				}
				fmt.Println()
			}
		}
	}

	// Execute the chain
	fmt.Println("\n🚀 Executing chain...")
	builder.Execute()

	// Wait a bit for agents to complete
	fmt.Println("\n⏳ Waiting for agents to complete...")
	time.Sleep(2 * time.Second)

	// Show final state
	fmt.Println("\n📊 Final States:")
	for _, want := range config.Wants {
		fmt.Printf("Want '%s':\n", want.Metadata.Name)
		fmt.Printf("  Status: %s\n", want.Status)
		state := want.GetAllState()
		for k, v := range state {
			fmt.Printf("  %s: %v\n", k, v)
		}
		fmt.Println()
	}

	fmt.Println("✅ Hotel Agent Demo completed")
	os.Exit(0)
}