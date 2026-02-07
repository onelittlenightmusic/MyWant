package main

import (
	"fmt"
	"log"
	"sync"
	"time"

	. "mywant/engine/src"
	_ "mywant/engine/cmd/types"
)

func main() {
	log.Println("=== Concurrent Prime Sieve Deployment Test ===")
	log.Println("Deploying 10 prime sieve instances in parallel...")

	// Load recipe
	recipe, err := LoadRecipeFromFile("../yaml/recipes/prime-sieve.yaml")
	if err != nil {
		log.Fatalf("Failed to load recipe: %v", err)
	}

	// Create ChainBuilder
	cb := NewChainBuilder(Config{
		Wants: []*Want{},
	})

	// Register prime types
	

	// Start reconcile loop
	go cb.reconcileLoop()
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

			wants, err := recipe.Instantiate(params)
			if err != nil {
				errors <- fmt.Errorf("instance %d: failed to instantiate: %v", idx, err)
				return
			}

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
		cb.mu.RLock()
		allAchieved := true
		primeSequenceCount := 0
		notAchievedWants := []string{}

		for _, want := range cb.Wants {
			if want.Metadata.Type == "prime sequence" {
				primeSequenceCount++
				prog, ok := cb.progress[want.Metadata.ID]
				if !ok || !prog.IsAchieved() {
					allAchieved = false
					notAchievedWants = append(notAchievedWants, want.Metadata.Name)

					// Print detailed state
					achievedState, hasAchieved := want.State["achieved"]
					totalProcessed, hasProcessed := want.State["total_processed"]
					log.Printf("⏳ Want %s not achieved - achieved=%v, total_processed=%v",
						want.Metadata.Name, achievedState, totalProcessed)

					// Check if it has achieved state but IsAchieved() returns false
					if hasAchieved && achievedState == true {
						log.Printf("⚠️  WARNING: Want %s has achieved=true in State but IsAchieved()=false!", want.Metadata.Name)
					}
				}
			}
		}
		cb.mu.RUnlock()

		elapsed := time.Since(startTime)
		if allAchieved {
			log.Printf("✅ All %d prime sequence wants achieved in %v!", primeSequenceCount, elapsed)

			// Print summary
			log.Println("\n=== Success Summary ===")
			cb.mu.RLock()
			for _, want := range cb.Wants {
				if want.Metadata.Type == "prime sequence" {
					primeCount := want.State["primeCount"]
					totalProcessed := want.State["total_processed"]
					log.Printf("  %s: found %v primes (processed %v numbers)",
						want.Metadata.Name, primeCount, totalProcessed)
				}
			}
			cb.mu.RUnlock()
			return
		}

		log.Printf("⏳ Progress [%v]: %d/%d achieved, not achieved: %v",
			elapsed.Round(time.Second), primeSequenceCount-len(notAchievedWants), primeSequenceCount, notAchievedWants)
	}

	// Timeout - print detailed diagnostics
	log.Println("\n❌ TIMEOUT - Detailed diagnostics:")
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	notAchievedCount := 0
	for _, want := range cb.Wants {
		if want.Metadata.Type == "prime sequence" {
			prog, _ := cb.progress[want.Metadata.ID]
			isAchieved := prog != nil && prog.IsAchieved()

			if !isAchieved {
				notAchievedCount++
				log.Printf("\n--- Want: %s (ID: %s) ---", want.Metadata.Name, want.Metadata.ID)
				log.Printf("  Type: %s", want.Metadata.Type)
				log.Printf("  IsAchieved: %v", isAchieved)
				log.Printf("  State: %+v", want.State)
				log.Printf("  Status: %+v", want.Status)
				log.Printf("  Using: %+v", want.Spec.Using)

				// Check input connections
				if prog != nil {
					if wantProg, ok := prog.(*Want); ok {
						log.Printf("  Input channels: %d", len(wantProg.InputChannels))
						for label, ch := range wantProg.InputChannels {
							log.Printf("    - %s: len=%d, cap=%d", label, len(ch), cap(ch))
						}
					}
				}
			}
		}
	}

	log.Printf("\n❌ %d prime sequence wants did not achieve after %v", notAchievedCount, maxWait)
	log.Fatal("Test failed")
}
