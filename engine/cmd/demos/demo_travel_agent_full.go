package main

import (
	"context"
	"fmt"
	. "mywant/engine/src"
	"mywant/engine/cmd/types"
	"os"
)

func main() {
	fmt.Println("🏨 Full Travel Agent Demo (Complete Agent Integration)")
	fmt.Println("====================================================")

	// Get config file path from command line argument
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

	// Dynamically register AgentPremium for hotel
	agentPremium := types.NewAgentPremium(
		"agent_premium",
		[]string{"hotel_reservation"},
		[]string{"xxx"},
		"platinum",
	)

	agentPremium.Action = func(ctx context.Context, want *Want) error {
		fmt.Printf("[AGENT_PREMIUM_ACTION] Hotel agent called, delegating to AgentPremium.Exec()\n")
		return agentPremium.Exec(ctx, want)
	}

	agentRegistry.RegisterAgent(agentPremium)
	fmt.Printf("🔧 Dynamically registered AgentPremium: %s\n", agentPremium.GetName())

	// Dynamically register Restaurant Agent
	agentRestaurant := types.NewAgentPremium(
		"agent_restaurant_premium",
		[]string{"restaurant_reservation"},
		[]string{"xxx"},
		"premium",
	)

	agentRestaurant.Action = func(ctx context.Context, want *Want) error {
		fmt.Printf("[AGENT_RESTAURANT_ACTION] Restaurant agent called, processing reservation\n")
		return agentRestaurant.Exec(ctx, want)
	}

	agentRegistry.RegisterAgent(agentRestaurant)
	fmt.Printf("🔧 Dynamically registered Restaurant Agent: %s\n", agentRestaurant.GetName())

	// Dynamically register Buffet Agent
	agentBuffet := types.NewAgentPremium(
		"agent_buffet_premium",
		[]string{"buffet_reservation"},
		[]string{"xxx"},
		"premium",
	)

	agentBuffet.Action = func(ctx context.Context, want *Want) error {
		fmt.Printf("[AGENT_BUFFET_ACTION] Buffet agent called, processing reservation\n")
		return agentBuffet.Exec(ctx, want)
	}

	agentRegistry.RegisterAgent(agentBuffet)
	fmt.Printf("🔧 Dynamically registered Buffet Agent: %s\n", agentBuffet.GetName())

	// Set agent registry on the builder
	builder.SetAgentRegistry(agentRegistry)

	// Register travel want types
	types.RegisterTravelWantTypes(builder)

	fmt.Println("🚀 Executing complete agent-enabled travel planning...")
	builder.Execute()

	fmt.Println("✅ Full travel planning with all agents completed!")
}