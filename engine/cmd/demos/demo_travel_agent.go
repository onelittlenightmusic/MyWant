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

	// Get config file path from command line argument
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

	// Create chain builder
	builder := NewChainBuilder(config)

	// Create and configure agent registry
	agentRegistry := NewAgentRegistry()

	// Load capabilities and agents
	if err := agentRegistry.LoadCapabilities("capabilities/"); err != nil {
		fmt.Printf("Warning: Failed to load capabilities: %v\n", err)
	}

	if err := agentRegistry.LoadAgents("agents/"); err != nil {
		fmt.Printf("Warning: Failed to load agents: %v\n", err)
	}

	// Dynamically register AgentPremium
	agentPremium := types.NewAgentPremium(
		"agent_premium",
		[]string{"hotel_reservation"}, // Match the required capability in recipe
		[]string{"xxx"},
		"platinum",
	)

	// Set a simple action that delegates to the AgentPremium.Exec() method
	agentPremium.Action = func(ctx context.Context, want *Want) error {
		fmt.Printf("[AGENT_PREMIUM_ACTION] Simple action called, delegating to AgentPremium.Exec()\n")
		return agentPremium.Exec(ctx, want)
	}

	agentRegistry.RegisterAgent(agentPremium)
	fmt.Printf("🔧 Dynamically registered AgentPremium: %s\n", agentPremium.GetName())

	// Dynamically register AgentRestaurant
	agentRestaurant := types.NewAgentRestaurant(
		"agent_restaurant",
		[]string{"restaurant_reservation"}, // Match the required capability in recipe
		[]string{"xxx"},
	)

	// Set a simple action that delegates to the AgentRestaurant.Exec() method
	agentRestaurant.Action = func(ctx context.Context, want *Want) error {
		fmt.Printf("[AGENT_RESTAURANT_ACTION] Simple action called, delegating to AgentRestaurant.Exec()\n")
		return agentRestaurant.Exec(ctx, want)
	}

	agentRegistry.RegisterAgent(agentRestaurant)
	fmt.Printf("🔧 Dynamically registered AgentRestaurant: %s\n", agentRestaurant.GetName())

	// Set agent registry on the builder
	builder.SetAgentRegistry(agentRegistry)

	// Register travel want types
	types.RegisterTravelWantTypes(builder)

	fmt.Println("🚀 Executing agent-enabled travel planning...")
	builder.Execute()

	fmt.Println("✅ Travel planning with agents completed!")
}
