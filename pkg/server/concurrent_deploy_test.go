package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	_ "mywant/engine/cmd/types"
	mywant "mywant/engine/src"

	"github.com/stretchr/testify/assert"
)

// TestConcurrentPrimeSieveDeployment tests deploying 10 prime sieve recipes in parallel
func TestConcurrentPrimeSieveDeployment(t *testing.T) {
	// Setup test server
	config := Config{Port: 0, Host: "localhost", Debug: true}
	os.MkdirAll("yaml/want_types", 0755)
	os.MkdirAll("yaml/recipes", 0755)

	server := New(config)
	server.setupRoutes()

	// Start the reconcile loop (critical for processing wants)
	go server.globalBuilder.ExecuteWithMode(true)
	defer server.globalBuilder.Stop()

	// Give reconcile loop time to start
	time.Sleep(200 * time.Millisecond)

	t.Log("=== Starting Concurrent Prime Sieve Deployment Test ===")
	t.Log("Deploying 10 prime sieve instances in parallel...")

	// Deploy 10 prime sieve instances concurrently
	var wg sync.WaitGroup
	deploymentErrors := make(chan error, 10)
	deployedIDs := make(chan []string, 10)

	deployStart := time.Now()
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			// Load and instantiate recipe with unique parameters
			params := map[string]interface{}{
				"start_number": 2,
				"end_number":   100,
				"prefix":       fmt.Sprintf("prime-%d", idx),
			}

			recipeConfig, err := mywant.LoadRecipe("../../yaml/recipes/prime-sieve.yaml", params)
			if err != nil {
				deploymentErrors <- fmt.Errorf("instance %d: failed to load recipe: %v", idx, err)
				return
			}

			wants := recipeConfig.Config.Wants
			if len(wants) == 0 {
				deploymentErrors <- fmt.Errorf("instance %d: recipe produced no wants", idx)
				return
			}

			// Assign IDs
			var ids []string
			for _, want := range wants {
				if want.Metadata.ID == "" {
					want.Metadata.ID = generateWantID()
				}
				ids = append(ids, want.Metadata.ID)
			}

			// Create payload
			configPayload := mywant.Config{Wants: wants}
			body, err := json.Marshal(configPayload)
			if err != nil {
				deploymentErrors <- fmt.Errorf("instance %d: failed to marshal: %v", idx, err)
				return
			}

			// POST to API
			req, _ := http.NewRequest("POST", "/api/v1/wants", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			server.router.ServeHTTP(w, req)

			if w.Code != http.StatusCreated {
				deploymentErrors <- fmt.Errorf("instance %d: deployment failed with status %d: %s",
					idx, w.Code, w.Body.String())
				return
			}

			deployedIDs <- ids
			t.Logf("✅ Instance %d: deployed successfully (wants: %v)", idx, ids)
		}(i)
	}

	// Wait for all deployments to complete
	wg.Wait()
	close(deploymentErrors)
	close(deployedIDs)

	// Check for deployment errors
	errorCount := 0
	for err := range deploymentErrors {
		t.Logf("❌ Deployment error: %v", err)
		errorCount++
	}

	// Collect all deployed IDs
	allDeployedIDs := make([]string, 0)
	for ids := range deployedIDs {
		allDeployedIDs = append(allDeployedIDs, ids...)
	}

	if errorCount > 0 {
		t.Fatalf("Failed to deploy %d instances", errorCount)
	}

	t.Logf("✅ All 10 instances deployed in %v", time.Since(deployStart))
	t.Logf("Total wants deployed: %d", len(allDeployedIDs))

	// Wait for all wants to be added to the system
	time.Sleep(500 * time.Millisecond)

	// Monitor execution until all prime sequence wants achieve
	t.Log("\nWaiting for all prime sequence wants to achieve...")
	maxWait := 30 * time.Second
	checkInterval := 1 * time.Second
	startTime := time.Now()

	var lastNotAchievedWants []string

	for time.Since(startTime) < maxWait {
		time.Sleep(checkInterval)

		// Get all want states
		allStates := server.globalBuilder.GetAllWantStates()

		primeSequenceCount := 0
		notAchievedCount := 0
		notAchievedWants := []string{}

		for _, want := range allStates {
			if want.Metadata.Type == "prime sequence" {
				primeSequenceCount++

				// Check achieved state
				achievedVal, hasAchieved := want.State["achieved"]
				achieved, _ := achievedVal.(bool)

				if !hasAchieved || !achieved {
					notAchievedCount++
					notAchievedWants = append(notAchievedWants, want.Metadata.Name)

					// Log detailed state for debugging
					totalProcessed, _ := want.State["total_processed"]
					primeCount, _ := want.State["primeCount"]
					t.Logf("⏳ Want %s - achieved=%v, processed=%v, primes=%v",
						want.Metadata.Name, achieved, totalProcessed, primeCount)
				}
			}
		}

		elapsed := time.Since(startTime)
		achievedCount := primeSequenceCount - notAchievedCount

		if primeSequenceCount == 10 && notAchievedCount == 0 {
			t.Logf("✅ All %d prime sequence wants achieved in %v!", primeSequenceCount, elapsed)

			// Print summary
			t.Log("\n=== Success Summary ===")
			for _, want := range allStates {
				if want.Metadata.Type == "prime sequence" {
					primeCount := want.State["primeCount"]
					totalProcessed := want.State["total_processed"]
					t.Logf("  %s: found %v primes (processed %v numbers)",
						want.Metadata.Name, primeCount, totalProcessed)
				}
			}
			return
		}

		lastNotAchievedWants = notAchievedWants
		t.Logf("⏳ Progress [%v]: %d/%d achieved, not achieved: %v",
			elapsed.Round(time.Second), achievedCount, primeSequenceCount, notAchievedWants)
	}

	// Timeout - print detailed diagnostics
	t.Log("\n❌ TIMEOUT - Detailed diagnostics:")

	allStates := server.globalBuilder.GetAllWantStates()
	notAchievedCount := 0

	// First, check prime numbers (generators) state
	t.Log("\n=== Prime Numbers (Generators) State ===")
	for _, want := range allStates {
		if want.Metadata.Type == "prime numbers" {
			currentNumber, _ := want.State["current_number"]
			completed, _ := want.State["completed"]
			achieved, _ := want.State["achieved"]
			t.Logf("Want: %s - current=%v, completed=%v, achieved=%v, status=%v",
				want.Metadata.Name, currentNumber, completed, achieved, want.Status)
		}
	}

	// Then, check prime sequence (processors) state
	t.Log("\n=== Prime Sequence (Processors) State ===")
	for _, want := range allStates {
		if want.Metadata.Type == "prime sequence" {
			achievedVal, _ := want.State["achieved"]
			achieved, _ := achievedVal.(bool)

			if !achieved {
				notAchievedCount++
				t.Logf("\n--- Want: %s (ID: %s) ---", want.Metadata.Name, want.Metadata.ID)
				t.Logf("  Type: %s", want.Metadata.Type)
				t.Logf("  State: %+v", want.State)
				t.Logf("  Status: %+v", want.Status)
				t.Logf("  Using: %+v", want.Spec.Using)
			}
		}
	}

	assert.Equal(t, 0, notAchievedCount,
		fmt.Sprintf("%d prime sequence wants did not achieve after %v: %v",
			notAchievedCount, maxWait, lastNotAchievedWants))
}
