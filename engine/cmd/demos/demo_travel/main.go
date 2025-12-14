package main

import (
    "fmt"
    "mywant/engine/cmd/types"
    . "mywant/engine/src"
    "os"
)

func main() {
    fmt.Println("🧳 Travel Planning Demo")
    fmt.Println("=======================")
    fmt.Println("Creating one day travel itinerary with:")
    fmt.Println("- Dinner restaurant visit")
    fmt.Println("- Hotel stay overnight")
    fmt.Println("- Breakfast buffet next morning")
    fmt.Println()
    if len(os.Args) < 2 {
        fmt.Println("Usage: go run demo_travel.go <config-file-path>")
        os.Exit(1)
    }
    configPath := os.Args[1]

    // Load YAML configuration
    config, err := LoadConfigFromYAML(configPath)
    if err != nil {
        fmt.Printf("Error loading %s: %v\n", configPath, err)
        return
    }

    fmt.Printf("Loaded %d travel wants from configuration\n", len(config.Wants))
    builder := NewChainBuilder(config)

    // Register travel want types
    types.RegisterTravelWantTypes(builder)

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
