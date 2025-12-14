package main

import (
    "fmt"
    "mywant/engine/cmd/types"
    . "mywant/engine/src"
    "os"
)

func main() {
    fmt.Println("🔄 Fibonacci Loop Demo (Advanced Architecture)")
    fmt.Println("==============================================")
    fmt.Println("This demo showcases a fibonacci sequence generator using:")
    fmt.Println("• Seed Generator: Provides initial values (0, 1)")
    fmt.Println("• Fibonacci Computer: Calculates next numbers in sequence")
    fmt.Println("• Merger: Creates feedback loop combining seeds + computed values")
    fmt.Println("• Sink: Collects and displays the complete sequence")
    fmt.Println("")
    if len(os.Args) < 2 {
        fmt.Println("Usage: go run demo_fibonacci_loop.go <config-file-path>")
        os.Exit(1)
    }
    configPath := os.Args[1]

    // Load YAML configuration
    config, err := LoadConfigFromYAML(configPath)
    if err != nil {
        fmt.Printf("Error loading %s: %v\n", configPath, err)
        return
    }

    fmt.Printf("Loaded %d wants from configuration\n", len(config.Wants))
    fmt.Println("")
    builder := NewChainBuilder(config)

    // Register fibonacci loop node types
    types.RegisterFibonacciLoopWantTypes(builder)

    fmt.Println("🚀 Executing fibonacci loop with reconcile system...")
    fmt.Println("")
    builder.Execute()

    fmt.Println("📊 Final Execution State:")
    fmt.Printf("  Fibonacci loop processing completed")

    fmt.Println("")
    fmt.Println("✅ Fibonacci loop execution completed successfully!")
    fmt.Println("🔄 The feedback loop architecture successfully generated the sequence!")
}
