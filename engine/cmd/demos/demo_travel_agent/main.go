package main

import (
	"context"
	"fmt"
	"mywant/engine/cmd/types"
	. "mywant/engine/src"
	"os"
)

func main() {
	fmt.Println("🏨 Travel Agent Demo (Recipe-based with Agent Integration)")
	fmt.Println("=========================================================")
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run demo_travel_agent.go <config-file-path>")
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

	// Dynamically register AgentPremium
	agentPremium := types.NewAgentPremium(
		"agent_premium",
		[]string{"hotel_reservation"}, // Match the required capability in recipe
		[]string{"xxx"},
		"platinum",
	)
	agentPremium.Action = func(ctx context.Context, want *Want) error {
		fmt.Printf("[AGENT_PREMIUM_ACTION] Simple action called, delegating to AgentPremium.Exec()\n")
		_, err := agentPremium.Exec(ctx, want)
		return err
	}

	agentRegistry.RegisterAgent(agentPremium)
	fmt.Printf("🔧 Dynamically registered AgentPremium: %s\n", agentPremium.GetName())
	builder.SetAgentRegistry(agentRegistry)

	// Register travel want types
	types.RegisterTravelWantTypes(builder)

	fmt.Println("🚀 Executing agent-enabled travel planning...")
	builder.Execute()

	fmt.Println("✅ Travel planning with agents completed!")
}
