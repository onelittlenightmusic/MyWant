package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "mywant/engine/cmd/types"
	. "mywant/engine/src"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: demo_flight <config-file>")
		os.Exit(1)
	}

	configFile := os.Args[1]

	// Load configuration
	config, err := LoadConfigFromYAML(configFile)
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("=== Flight Booking Demo ===")
	fmt.Println("This demo shows automatic flight rebooking when delays occur")
	fmt.Println()
	builder := NewChainBuilder(config)

	// Register travel-related want types (includes flight)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Execute in a goroutine
	done := make(chan bool)
	go func() {
		builder.Execute()
		done <- true
	}()

	// Wait for signal or completion
	fmt.Println("Monitoring flight status... (Press Ctrl+C to stop)")
	fmt.Println()

	// Run for a longer duration to see status changes
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	timeout := time.After(60 * time.Second)

loop:
	for {
		select {
		case <-sigChan:
			fmt.Println("\n\nShutdown signal received, stopping...")
			break loop
		case <-done:
			fmt.Println("\n\nExecution completed")
			break loop
		case <-timeout:
			fmt.Println("\n\nDemo timeout reached")
			break loop
		case <-ticker.C:
			// Print status update
			printFlightStatus(builder)
		}
	}

	// Cleanup
	cleanupFlights(builder)

	fmt.Println("\n=== Demo Complete ===")
}

func printFlightStatus(builder *ChainBuilder) {
	wants := builder.GetConfig().Wants
	if len(wants) == 0 {
		return
	}

	want := wants[0]
	if flightID, exists := want.State["flight_id"]; exists {
		status, _ := want.State["flight_status"]
		message, _ := want.State["status_message"]
		fmt.Printf("[Status] Flight ID: %v | Status: %v | Message: %v\n", flightID, status, message)
	}
}

func cleanupFlights(builder *ChainBuilder) {
	fmt.Println("Cleaning up flights...")

	// Access wants and clean up any FlightWant instances
	for _, want := range builder.GetConfig().Wants {
		if want.Metadata.Type == "flight" {
			// The cleanup will be handled by defer in the want's lifecycle
			fmt.Printf("Cleaned up flight want: %s\n", want.Metadata.Name)
		}
	}
}
