package main

import (
    "fmt"
    "mywant/engine/cmd/types"
    mywant "mywant/engine/src"
    "os"
)

func main() {
    fmt.Println("🎯 QNet Target Want Demo with Dynamic Recipe Loading")
    fmt.Println("====================================================")
    fmt.Println("This demo shows a target want that dynamically generates")
    fmt.Println("other wants from the qnet-pipeline recipe at runtime.")
    fmt.Println()
    yamlFile := "config/config-qnet.yaml"
    if len(os.Args) > 1 {
        yamlFile = os.Args[1]
    }

    // Load YAML configuration with target want
    config, err := mywant.LoadConfigFromYAML(yamlFile)
    if err != nil {
        fmt.Printf("Error loading %s: %v\n", yamlFile, err)
        return
    }

    fmt.Printf("📝 Loaded %d wants from configuration\n", len(config.Wants))

    // Show target want details
    for _, want := range config.Wants {
        if want.Metadata.Type == "target" {
            fmt.Printf("🎯 Target Want: %s\n", want.Metadata.Name)
            fmt.Printf("  Type: %s\n", want.Metadata.Type)
            fmt.Printf("  Parameters: %v\n", want.Spec.Params)
        }
    }
    builder := mywant.NewChainBuilder(config)

    // Register domain-specific want types
    types.RegisterQNetWantTypes(builder) // For qnet types (numbers, queue, sink, etc.)

    fmt.Println("\n🚀 Executing target-based chain with dynamic recipe loading...")
    fmt.Println("The target want will:")
    fmt.Println("1. Load the qnet-pipeline recipe")
    fmt.Println("2. Dynamically create child wants (generators, queues, combiner, sink)")
    fmt.Println("3. Add owner references to all child wants")
    fmt.Println("4. Wait for all children to complete")
    fmt.Println("5. Compute aggregate results")
    fmt.Println()

    // Memory dump will be automatically created as memory-*.yaml by the system

    // Execute using existing reconcile loop system
    builder.Execute()

    fmt.Println("\n📊 Final Want States:")
    states := builder.GetAllWantStates()
    for name, state := range states {
        processed := 0
        if state.State != nil {
            if val, ok := state.State["total_processed"]; ok {
                if intVal, ok := val.(int); ok {
                    processed = intVal
                }
            }
        }
        fmt.Printf("  %s: %s (processed: %d)\n",
            name, state.Status, processed)
    }

    // Show target results
    fmt.Println("\n🎯 Target Want Results:")
    for name, state := range states {
        if state.Metadata.Type == "target" {
            fmt.Printf("  Target %s:\n", name)
            if result, ok := state.State["result"]; ok {
                fmt.Printf("    Result: %v\n", result)
            }
            if recipePath, ok := state.State["recipePath"]; ok {
                fmt.Printf("    Recipe: %v\n", recipePath)
            }
            if childCount, ok := state.State["childCount"]; ok {
                fmt.Printf("    Children: %v\n", childCount)
            }
        }
    }

    // Memory snapshot is automatically saved to memory/memory-TIMESTAMP.yaml
    fmt.Println("✅ Target-based dynamic recipe execution completed successfully!")
}
