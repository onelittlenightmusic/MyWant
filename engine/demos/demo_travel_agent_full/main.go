package main

import (
	"fmt"
	. "mywant/engine/core"
	_ "mywant/engine/types"
	"os"
)

func main() {
	fmt.Println("🏨 Full Travel Agent Demo (Complete Agent Integration)")
	fmt.Println("====================================================")
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run demo_travel_agent_full.go <config-file-path>")
		os.Exit(1)
	}
	configPath := os.Args[1]

	// Load configuration using automatic recipe loading
	config, err := LoadConfigFromYAML(configPath)
	if err != nil {
		fmt.Printf("Error loading %s: %v\n", configPath, err)
		return
	}

	fmt.Printf("📋 Loaded configuration with %d wants\n", len(config.Wants))
	for _, want := range config.Wants {
		fmt.Printf("  - %s (%s)\n", want.Metadata.Name, want.Metadata.Type)
		if len(want.Spec.Requires) > 0 {
			fmt.Printf("    Requires: %v\n", want.Spec.Requires)
		}
	}
	builder := NewChainBuilder(config)
	agentRegistry := NewAgentRegistry()

	// Load capabilities and agents
	if err := agentRegistry.LoadCapabilities("yaml/capabilities/"); err != nil {
		fmt.Printf("Warning: Failed to load capabilities: %v\n", err)
	}

	if err := agentRegistry.LoadAgents("yaml/agents/"); err != nil {
		fmt.Printf("Warning: Failed to load agents: %v\n", err)
	}

	// Activate all auto-registered agent implementations (including premium agents)
	RegisterAllKnownAgentImplementations(agentRegistry)

	builder.SetAgentRegistry(agentRegistry)

	fmt.Println("🚀 Executing complete agent-enabled travel planning...")
	builder.Execute()

	fmt.Println("✅ Full travel planning with all agents completed!")
}
