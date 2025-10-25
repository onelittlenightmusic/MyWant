package main

import (
	"fmt"
	"mywant/engine/cmd/types"
	. "mywant/engine/src"
	"os"
	"strconv"
	"time"
)

func main() {
	fmt.Println("🧳 Travel Planning Recipe Demo (Long Duration)")
	fmt.Println("=============================================")
	fmt.Println("Loading travel itinerary from recipe with flight delay monitoring:")
	fmt.Println("- Recipe defines: restaurant, hotel, buffet wants with flight rebooking")
	fmt.Println("- Flight monitoring enables automatic rebooking on delay detection")
	fmt.Println("- Demo runs for extended duration to observe complete cycle")
	fmt.Println()

	// Get YAML file from command line argument
	yamlFile := "config/config-travel-recipe.yaml"
	if len(os.Args) > 1 {
		yamlFile = os.Args[1]
	}

	// Get duration from command line (default 120 seconds for full delay cycle)
	// Mock server timeline:
	// T+0s: confirmed
	// T+40s: delayed_one_day (trigger rebooking)
	// T+80s: confirmed again
	durationSeconds := 120
	if len(os.Args) > 2 {
		if d, err := strconv.Atoi(os.Args[2]); err == nil && d > 0 {
			durationSeconds = d
		}
	}

	fmt.Printf("Running demo for %d seconds to observe delay detection & rebooking\n", durationSeconds)
	fmt.Println()

	// Load configuration using generic recipe loader
	config, err := LoadConfigFromYAML(yamlFile)
	if err != nil {
		fmt.Printf("Error loading recipe config %s: %v\n", yamlFile, err)
		return
	}

	fmt.Printf("✅ Generated %d wants from recipe\n", len(config.Wants))
	for _, want := range config.Wants {
		fmt.Printf("  - %s (%s)\n", want.Metadata.Name, want.Metadata.Type)
	}
	fmt.Println()

	// Create chain builder with standard constructor
	// Note: Registration order no longer matters - OwnerAware wrapping happens automatically at creation time
	builder := NewChainBuilder(config)

	// Create and configure agent registry - same as server mode
	agentRegistry := NewAgentRegistry()

	// Load capabilities from YAML files (from project root, accounting for engine subdirectory)
	// Try both relative paths since go run -C engine changes the working directory
	capPaths := []string{"../capabilities/", "capabilities/"}
	var capPath string
	for _, p := range capPaths {
		fmt.Printf("  Trying to load capabilities from %s...\n", p)
		if err := agentRegistry.LoadCapabilities(p); err == nil {
			// Check if we actually loaded any capabilities
			loadedCount := len(agentRegistry.GetAllCapabilities())
			fmt.Printf("    Loaded %d capabilities from %s\n", loadedCount, p)
			if loadedCount > 0 {
				capPath = p
				break
			}
		} else {
			fmt.Printf("    Error: %v\n", err)
		}
	}
	if capPath == "" {
		fmt.Printf("⚠️  No capabilities loaded from paths: %v\n", capPaths)
	} else {
		fmt.Printf("✅ Loaded capabilities from %s\n", capPath)
	}

	// Load agents from YAML files (optional - may fail due to spec validation paths)
	agentPath := "agents/"
	if err := agentRegistry.LoadAgents(agentPath); err != nil {
		// Try parent directory
		if err := agentRegistry.LoadAgents("../agents/"); err != nil {
			fmt.Printf("⚠️  Could not load agents from YAML (may be due to spec path issues)\n")
		}
	}

	// List loaded capabilities for debugging
	allCaps := agentRegistry.GetAllCapabilities()
	fmt.Printf("📋 Loaded %d capabilities: ", len(allCaps))
	for _, cap := range allCaps {
		fmt.Printf("%s(gives:%v) ", cap.Name, cap.Gives)
	}
	fmt.Printf("\n")

	// Manually register the AgentFlightAPI agent with the loaded capability
	// This ensures we have the flight API agent available with proper implementation
	flightAPIAgent := types.NewAgentFlightAPI(
		"agent_flight_api",
		[]string{"flight_api_agency"}, // Must match capability name that provides create_flight
		[]string{},
		"http://localhost:8081",
	)
	agentRegistry.RegisterAgent(flightAPIAgent)
	fmt.Printf("✅ Registered agent_flight_api with flight_api_agency capability\n")

	// Debug: show what agents can provide "create_flight"
	createFlightAgents := agentRegistry.FindAgentsByGives("create_flight")
	fmt.Printf("🔍 Agents that provide 'create_flight': %d\n", len(createFlightAgents))

	// Register agent on builder
	builder.SetAgentRegistry(agentRegistry)

	// Register domain-specific want types
	types.RegisterTravelWantTypes(builder)

	fmt.Println("🏁 Executing recipe-based travel planning with extended monitoring...")
	fmt.Println()
	fmt.Println("📊 Monitoring execution for delay detection & rebooking (press Ctrl+C to stop):")
	fmt.Println("----------------------------------------------------------------------")

	// Start execution in background
	startTime := time.Now()
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	// Create a channel to control execution
	done := make(chan bool, 1)

	// Start execution in background
	go func() {
		builder.Execute()
		done <- true
	}()

	// Monitor loop - waits for execution to complete or timeout
	for {
		select {
		case <-done:
			// Execution completed
			elapsed := time.Since(startTime).Seconds()
			fmt.Printf("\n[T+%.0fs] ✅ All wants completed!\n", elapsed)
			fmt.Println("\n📊 Final Execution State:")
			fmt.Println("  Recipe-based travel planning completed")
			fmt.Println("\n✅ Recipe-based travel planning completed!")
			return

		case <-ticker.C:
			elapsed := time.Since(startTime).Seconds()

			// Check if we've reached the timeout
			if elapsed >= float64(durationSeconds) {
				fmt.Printf("\n⏱️  Duration timeout reached (%.0fs)\n", elapsed)
				fmt.Println("\n📊 Final Execution State:")
				fmt.Println("  Monitoring period completed")
				fmt.Println("\n✅ Extended monitoring completed!")
				return
			}
		}
	}
}
