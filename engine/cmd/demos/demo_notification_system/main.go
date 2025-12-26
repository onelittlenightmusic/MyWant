package main

import (
	"fmt"
	types "mywant/engine/cmd/types"
	mywant "mywant/engine/src"
	"os"
	"path/filepath"
	"time"
)

func main() {
	fmt.Println("🔔 Notification System Demo")
	fmt.Println("============================")
	fmt.Println("This demo showcases the new generalized want notification system:")
	fmt.Println("- State subscriptions between wants")
	fmt.Println("- Real-time monitoring and alerting")
	fmt.Println("- Notification history and debugging")
	fmt.Println()

	// Load configuration
	configPath := "config/config-notification-demo.yaml"
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
	types.RegisterQNetWantTypes(builder)
	mywant.RegisterMonitorWantTypes(builder)

	fmt.Println("\nShowing initial notification system state:")
	showNotificationSystemState()

	// Execute the chain
	fmt.Println("\n🚀 Starting chain execution...")
	builder.Execute()

	// Wait a bit for notifications to flow
	fmt.Println("\n⏳ Waiting for notifications to flow...")
	time.Sleep(3 * time.Second)

	// Show notification system state after execution
	fmt.Println("\n📊 Final notification system state:")
	showNotificationSystemState()

	// Show notification history
	fmt.Println("\n📜 Recent notification history:")
	showNotificationHistory(10)

	fmt.Println("\n✅ Notification system demo completed!")
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
