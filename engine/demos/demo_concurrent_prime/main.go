package main

import (
	"fmt"
	"log"
	"sync"
	"time"

	. "mywant/engine/core"
	_ "mywant/engine/types"
)

func main() {
	log.Println("=== Concurrent Prime Sieve Deployment Test ===")
	log.Println("Deploying 10 prime sieve instances in parallel...")

	// Create ChainBuilder
	cb := NewChainBuilder(Config{})

	// Register prime types

	// Start reconcile loop
	cb.Start()
	defer cb.Stop()

	// Give reconcile loop time to start
	time.Sleep(100 * time.Millisecond)

	// Deploy 10 prime sieve instances in parallel
	var wg sync.WaitGroup
	errors := make(chan error, 10)

	deployStart := time.Now()
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			// Create want from recipe
			params := map[string]interface{}{
				"start_number": 2,
				"end_number":   100,
				"prefix":       fmt.Sprintf("prime-%d", idx),
			}

			recipeConfig, err := LoadRecipe("../yaml/recipes/prime-sieve.yaml", params)
			if err != nil {
				errors <- fmt.Errorf("instance %d: failed to instantiate: %v", idx, err)
				return
			}
			wants := recipeConfig.Config.Wants

			// Add wants
			err = cb.AddWantsAsync(wants)
			if err != nil {
				errors <- fmt.Errorf("instance %d: failed to add wants: %v", idx, err)
				return
			}

			log.Printf("✅ Instance %d: deployed successfully", idx)
		}(i)
	}

	// Wait for all deployments to complete
	wg.Wait()
	close(errors)

	// Check for deployment errors
	deploymentErrors := 0
	for err := range errors {
		log.Printf("❌ Deployment error: %v", err)
		deploymentErrors++
	}

	if deploymentErrors > 0 {
		log.Fatalf("Failed to deploy %d instances", deploymentErrors)
	}

	log.Printf("✅ All 10 instances deployed in %v", time.Since(deployStart))

	// Wait for execution to complete
	log.Println("\nWaiting for all prime sequence wants to achieve...")
	maxWait := 30 * time.Second
	checkInterval := 1 * time.Second
	startTime := time.Now()

	for time.Since(startTime) < maxWait {
		time.Sleep(checkInterval)

		// Check all prime sequence wants
		allAchieved := true
		primeSequenceCount := 0
		notAchievedWants := []string{}

		wants := cb.GetWants()
		for _, want := range wants {
			if want.Metadata.Type == "prime sequence" {
				primeSequenceCount++
				if want.GetStatus() != WantStatusAchieved {
					allAchieved = false
					notAchievedWants = append(notAchievedWants, want.Metadata.Name)

					// Print detailed state
					achievedState, _ := want.GetState("achieved")
					totalProcessed, _ := want.GetState("total_processed")
					log.Printf("⏳ Want %s not achieved - status=%s, achieved=%v, total_processed=%v",
						want.Metadata.Name, want.GetStatus(), achievedState, totalProcessed)
				}
			}
		}

		elapsed := time.Since(startTime)
		if allAchieved && primeSequenceCount > 0 {
			log.Printf("✅ All %d prime sequence wants achieved in %v!", primeSequenceCount, elapsed)

			// Print summary
			log.Println("\n=== Success Summary ===")
			for _, want := range wants {
				if want.Metadata.Type == "prime sequence" {
					primeCount, _ := want.GetState("primeCount")
					totalProcessed, _ := want.GetState("total_processed")
					log.Printf("  %s: found %v primes (processed %v numbers)",
						want.Metadata.Name, primeCount, totalProcessed)
				}
			}
			return
		}

		log.Printf("⏳ Progress [%v]: %d/%d achieved, not achieved: %v",
			elapsed.Round(time.Second), primeSequenceCount-len(notAchievedWants), primeSequenceCount, notAchievedWants)
	}

	// Timeout - print detailed diagnostics
	log.Println("\n❌ TIMEOUT - Detailed diagnostics:")

	notAchievedCount := 0
	for _, want := range cb.GetWants() {
		if want.Metadata.Type == "prime sequence" {
			isAchieved := want.GetStatus() == WantStatusAchieved

			if !isAchieved {
				notAchievedCount++
				log.Printf("\n--- Want: %s (ID: %s) ---", want.Metadata.Name, want.Metadata.ID)
				log.Printf("  Type: %s", want.Metadata.Type)
				log.Printf("  IsAchieved: %v", isAchieved)
				log.Printf("  State: %+v", want.State)
				log.Printf("  Status: %+v", want.Status)
				log.Printf("  Using: %+v", want.Spec.Using)

				// Check input connections
				log.Printf("  Input channels: %d", len(want.GetPaths().In))
				for _, path := range want.GetPaths().In {
					log.Printf("    - %s: active=%v", path.Name, path.Active)
				}
			}
		}
	}

	log.Printf("\n❌ %d prime sequence wants did not achieve after %v", notAchievedCount, maxWait)
	log.Fatal("Test failed")
}
