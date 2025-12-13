package main

import (
	"fmt"
	"mywant/engine/cmd/types"
	. "mywant/engine/src"
	"os"
)

func main() {
	fmt.Println("🔢 Fibonacci Sequence Recipe Demo")
	fmt.Println("==================================")
	fmt.Println("Loading fibonacci sequence from recipe:")
	fmt.Println("- Recipe defines: fibonacci_generator, fibonacci_filter wants")
	fmt.Println("- Filter collects fibonacci results without sink")
	fmt.Println()
	yamlFile := "config/config-fibonacci-recipe.yaml"
	if len(os.Args) > 1 {
		yamlFile = os.Args[1]
	}

	// Load configuration using generic recipe loader
	config, err := LoadConfigFromYAML(yamlFile)
	if err != nil {
		fmt.Printf("Error loading recipe config %s: %v\n", yamlFile, err)
		return
	}

	fmt.Printf("✅ Loaded %d wants from configuration\n", len(config.Wants))
	for _, want := range config.Wants {
		fmt.Printf("  - %s (%s)\n", want.Metadata.Name, want.Metadata.Type)
	}
	fmt.Println()
	builder := NewChainBuilder(config)

	// Register fibonacci want types
	types.RegisterFibonacciWantTypes(builder)

	fmt.Println("🏁 Executing recipe-based fibonacci sequence...")
	builder.Execute()

	// GetAllWantStates should now include child wants created during execution
	states := builder.GetAllWantStates()

	fmt.Printf("\n📊 Final Fibonacci Sequence Results (found %d wants):\n", len(states))
	if len(states) == 0 {
		fmt.Println("⚠️  No wants found. This may indicate a configuration issue.")
		return
	}

	for name, state := range states {
		if state != nil {
			// Show stats from fibonacci filter
			if state.Metadata.Type == "fibonacci filter" {
				if totalProcessed, exists := state.State["total_processed"]; exists {
					fmt.Printf("  🔢 Filter %s processed %v numbers\n", name, totalProcessed)
				}
				if filtered, exists := state.GetState("filtered"); exists {
					if fibList, ok := filtered.([]int); ok {
						fmt.Printf("  🎯 Filtered %d fibonacci numbers: ", len(fibList))
						for i, f := range fibList {
							if i > 0 {
								fmt.Print(", ")
							}
							fmt.Print(f)
							if i >= 9 { // Limit display to first 10
								fmt.Printf("... (and %d more)", len(fibList)-10)
								break
							}
						}
						fmt.Println()
					}
				}
			}

			// Show stats from fibonacci generator
			if state.Metadata.Type == "fibonacci numbers" {
				if totalProcessed, exists := state.State["total_processed"]; exists {
					fmt.Printf("  📈 Generator %s produced %v numbers\n", name, totalProcessed)
				}
			}
		}
	}

	fmt.Println("\n✅ Recipe-based fibonacci sequence completed!")
}
