package main

import (
	"fmt"
	. "mywant/engine/core"
	_ "mywant/engine/types"
	"os"
)

func main() {
	fmt.Println("🔢 Prime Number Sieve Recipe Demo")
	fmt.Println("==================================")
	fmt.Println("Loading prime sieve from recipe:")
	fmt.Println("- Recipe defines: prime_generator, prime_filter wants")
	fmt.Println("- Target parent collects filtered prime results")
	fmt.Println()
	yamlFile := "yaml/config/config-prime-recipe.yaml"
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

	// Register prime want types

	fmt.Println("🏁 Executing recipe-based prime sieve...")
	builder.Execute()
	states := builder.GetAllWantStates()

	fmt.Println("\n📊 Final Prime Sieve Results:")
	for name, state := range states {
		if state != nil {
			// Show stats from prime filter
			if state.Metadata.Type == "prime_filter" {
				totalProcessed := GetCurrent(state, "total_processed", 0)
				if totalProcessed > 0 {
					fmt.Printf("  🔢 Filter %s processed %v numbers\n", name, totalProcessed)
				}
				primeList := GetCurrent(state, "foundPrimes", []int{})
				if len(primeList) > 0 {
					fmt.Printf("  🎯 Found %d primes: ", len(primeList))
					for i, p := range primeList {
						if i > 0 {
							fmt.Print(", ")
						}
						fmt.Print(p)
						if i >= 9 {
							fmt.Printf("... (and %d more)", len(primeList)-10)
							break
						}
					}
					fmt.Println()
				}
			}
			if state.Metadata.Type == "prime_numbers" {
				if totalProcessed, exists := state.GetAllState()["total_processed"]; exists {
					fmt.Printf("  📈 Generator %s produced %v numbers\n", name, totalProcessed)
				}
			}
		}
	}

	fmt.Println("\n✅ Recipe-based prime sieve completed!")
}
