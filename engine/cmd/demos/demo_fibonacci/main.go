package main

import (
    "fmt"
    . "mywant/engine/src"
    "os"
)

func main() {
    fmt.Println("Fibonacci Sequence Demo (YAML Config)")
    fmt.Println("=====================================")
    if len(os.Args) < 2 {
        fmt.Println("Usage: go run demo_fibonacci.go <config-file-path>")
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
    builder := NewChainBuilder(config)

    fmt.Println("\nExecuting fibonacci sequence with reconcile loop...")
    builder.Execute()

    fmt.Println("\n📊 Final Execution State:")
    fmt.Printf("  Fibonacci sequence execution completed")

    fmt.Println("\n✅ Fibonacci sequence execution completed!")
}
