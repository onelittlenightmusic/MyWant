package main

import (
	"fmt"
	_ "mywant/engine/cmd/types"
	mywant "mywant/engine/src"
	"os"
	"path/filepath"
	"time"
)

func main() {
	fmt.Println("🎯 Target Notification Integration Demo")
	fmt.Println("=======================================")
	fmt.Println("This demo showcases Target wants using the generalized notification system")
	fmt.Println()

	// Load configuration
	configPath := "yaml/config/config-target-notification-test.yaml"
	if len(os.Args) > 1 {
		configPath = os.Args[1]
	}
	absPath, err := filepath.Abs(configPath)
	if err != nil {
		fmt.Printf("Error getting absolute path: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Loading configuration from: %s\n", absPath)

	// Load config
	config, err := mywant.LoadConfigFromYAML(absPath)
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Loaded %d wants from configuration\n", len(config.Wants))
	builder := mywant.NewChainBuilder(config)

	// Register want types
	fmt.Println("\nRegistering want types...")
	

	// Show initial state
	fmt.Println("\n📊 Initial notification system state:")
	showNotificationSystemState()

	// Execute the chain
	fmt.Println("\n🚀 Starting Target-based chain execution...")
	builder.Execute()

	// Wait for execution and notifications
	fmt.Println("\n⏳ Waiting for Target notifications...")
	time.Sleep(3 * time.Second)

	// Show final state
	fmt.Println("\n📊 Final notification system state:")
	showNotificationSystemState()

	// Show notification history
	fmt.Println("\n📜 Recent Target notification history:")
	showNotificationHistory(15)

	fmt.Println("\n✅ Target notification integration demo completed!")
}

func showNotificationSystemState() {
	// Show registered listeners
	listeners := mywant.GetRegisteredListeners()
	fmt.Printf("📡 Registered state listeners (%d):\n", len(listeners))
	for _, listener := range listeners {
		fmt.Printf("  - %s\n", listener)
	}

	// Show subscriptions
	subscriptions := mywant.GetSubscriptions()
	fmt.Printf("\n📋 State subscriptions (%d subscribers):\n", len(subscriptions))
	for subscriber, subs := range subscriptions {
		fmt.Printf("  %s subscribes to:\n", subscriber)
		for _, sub := range subs {
			if len(sub.StateKeys) > 0 {
				fmt.Printf("    - %s (keys: %v)\n", sub.WantName, sub.StateKeys)
			} else {
				fmt.Printf("    - %s (all keys)\n", sub.WantName)
			}
		}
	}
}

func showNotificationHistory(limit int) {
	history := mywant.GetNotificationHistory(limit)
	if len(history) == 0 {
		fmt.Println("  No notifications recorded")
		return
	}

	fmt.Printf("  Last %d notifications:\n", len(history))
	for i, notif := range history {
		fmt.Printf("    %d. [%s] %s → %s: %s = %v (type: %s)\n",
			i+1,
			notif.Timestamp.Format("15:04:05"),
			notif.SourceWantName,
			notif.TargetWantName,
			notif.StateKey,
			notif.StateValue,
			notif.NotificationType)
	}
}
