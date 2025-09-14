package main

import (
	"fmt"
	. "mywant/src"
)

func main() {
	fmt.Println("🧳 Travel Planning Demo")
	fmt.Println("=======================")
	fmt.Println("Creating one day travel itinerary with:")
	fmt.Println("- Dinner restaurant visit")
	fmt.Println("- Hotel stay overnight")  
	fmt.Println("- Breakfast buffet next morning")
	fmt.Println()
	
	// Load YAML configuration
	config, err := LoadConfigFromYAML("config/config-travel.yaml")
	if err != nil {
		fmt.Printf("Error loading config/config-travel.yaml: %v\n", err)
		return
	}

	fmt.Printf("Loaded %d travel wants from configuration\n", len(config.Wants))

	// Create chain builder
	builder := NewChainBuilder(config)
	
	// Register travel want types
	RegisterTravelWantTypes(builder)
	
	fmt.Println("\n🏁 Executing travel planning workflow...")
	builder.Execute()
	
	fmt.Println("\n📊 Final Want States:")
	states := builder.GetAllWantStates()
	for name, state := range states {
		fmt.Printf("  %s: %s (processed: %v)\n", 
			name, state.Status, state.State["total_processed"])
	}
	
	fmt.Println("\n✅ Travel planning completed successfully!")
}