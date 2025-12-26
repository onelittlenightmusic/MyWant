package main

import (
	"fmt"
	. "mywant/engine/src"
	"mywant/engine/cmd/types"
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

	// Try to load with recipe first, then fallback to direct config
	config, err := LoadConfigFromYAML(configPath)
	if err != nil {
		fmt.Printf("Error loading %s: %v\n", configPath, err)
		return
	}

	// If config has wants with recipes, expand them
	expandedWants := make([]*Want, 0)
	for _, want := range config.Wants {
		if want.Spec.Recipe != "" {
			// Load and expand recipe
			// Find the recipe base directory by looking for recipes folder
			recipeDir := ""
			// Try ../ first (running from engine directory)
			if _, err := os.Stat("../recipes"); err == nil {
				recipeDir = "../"
			} else if _, err := os.Stat("./recipes"); err == nil {
				recipeDir = "./"
			} else {
				recipeDir = "../"
			}

			// Construct full recipe path
			fullRecipePath := recipeDir + want.Spec.Recipe

			loader := NewGenericRecipeLoader(recipeDir)
			recipeConfig, err := loader.LoadRecipe(fullRecipePath, want.Spec.Params)
			if err != nil {
				fmt.Printf("Error loading recipe %s: %v\n", want.Spec.Recipe, err)
				continue
			}
			// Add expanded wants from recipe
			expandedWants = append(expandedWants, recipeConfig.Config.Wants...)
		} else {
			expandedWants = append(expandedWants, want)
		}
	}
	config.Wants = expandedWants

	fmt.Printf("Loaded %d wants from configuration\n", len(config.Wants))
	builder := NewChainBuilder(config)

	// Register fibonacci want types
	types.RegisterFibonacciWantTypes(builder)

	fmt.Println("\nExecuting fibonacci sequence with reconcile loop...")
	builder.Execute()

	fmt.Println("\n📊 Final Execution State:")

	// Print state of all wants to verify achieving_percentage
	allWants := builder.GetAllWantStates()
	fmt.Printf("\nTotal wants: %d\n", len(allWants))
	for _, want := range allWants {
		if want.Metadata.Type != "scheduler" { // Skip the system scheduler
			achieving, _ := want.State["achieving_percentage"]
			finalResult, _ := want.State["final_result"]
			achieved := want.Status == "completed"

			fmt.Printf("\n  Want: %s (type: %s)\n", want.Metadata.Name, want.Metadata.Type)
			fmt.Printf("    Status: %s\n", want.Status)
			fmt.Printf("    Achieving %%: %v\n", achieving)
			fmt.Printf("    Final Result: %v\n", finalResult)
			fmt.Printf("    IsAchieved: %v\n", achieved)

			// Print error if failed
			if want.Status == "failed" && want.State != nil {
				if errVal, ok := want.State["error"]; ok {
					fmt.Printf("    Error: %v\n", errVal)
				}
			}
		}
	}

	fmt.Println("\n✅ Fibonacci sequence execution completed!")
}
