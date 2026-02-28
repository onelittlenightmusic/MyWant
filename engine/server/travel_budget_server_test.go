package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	mywant "mywant/engine/core"
)

// TestTravelBudgetSystem_Integration verifies that deploying a travel budget system
// through the real server router correctly results in child want generation and 
// proper field-level correlation (stateAccess).
func TestTravelBudgetSystem_Integration(t *testing.T) {
	// Change working directory to project root so 'yaml/' paths work
	origWd, _ := os.Getwd()
	err := os.Chdir("../..")
	if err != nil {
		t.Fatalf("Failed to change directory to project root: %v", err)
	}
	defer os.Chdir(origWd)

	wd, _ := os.Getwd()
	t.Logf("Working directory: %s", wd)

	// Skip if we can't find the yaml directory
	if _, err := os.Stat("yaml"); os.IsNotExist(err) {
		t.Skip("Skipping integration test: 'yaml' directory not found.")
	}

	// Server config
	config := Config{
		Port:       8081, // Use different port
		Host:       "localhost",
		Debug:      true,
		ConfigPath: "", // Don't use real config
		MemoryPath: "", // Don't use real memory
	}

	// 1. Create Server instance (loads all YAMLs automatically)
	s := New(config)
	
	// Manually start the global builder reconcile loop for the test
	// Normally s.Start() would do this.
	go s.globalBuilder.ExecuteWithMode(true)
	defer s.globalBuilder.Shutdown()

	// 2. Register Monitor/Owner/Scheduler want types (normally done in s.Start())
	mywant.RegisterMonitorWantTypes(s.globalBuilder)
	mywant.RegisterOwnerWantTypes(s.globalBuilder)
	mywant.RegisterSchedulerWantTypes(s.globalBuilder)
	
	// Transfer loaded want type definitions (normally done in s.Start())
	if s.wantTypeLoader != nil {
		allDefs := s.wantTypeLoader.GetAll()
		for _, def := range allDefs {
			s.globalBuilder.StoreWantTypeDefinition(def)
		}
	}

	// 3. Deploy a Travel Budget System want
	deployment := mywant.Want{
		Metadata: mywant.Metadata{
			Name: "integration-test-planner",
			Type: "travel budget system",
		},
		Spec: mywant.WantSpec{
			Params: map[string]any{
				"prefix":          "int-test",
				"budget":          3000.0,
				"currency":        "USD",
				"restaurant_type": "fine dining",
				"hotel_type":      "luxury",
				"restaurant_cost": 350.0,
				"hotel_cost":      900.0,
			},
			Recipe: "travel-budget",
		},
	}

	body, _ := json.Marshal(deployment)
	req, _ := http.NewRequest("POST", "/api/v1/wants", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	
	// Setup routes
	s.setupRoutes()
	rr := httptest.NewRecorder()
	s.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("Failed to create want: HTTP %d - %s", rr.Code, rr.Body.String())
	}

	// 4. Wait for reconciliation (Recipe -> Children expansion and ID assignment)
	// We need multiple cycles for: Add Parent -> Expand Recipe -> Add Children -> Assign IDs -> Build Access Index -> Compute Correlation
	// Give it up to 2 seconds for all phases to complete reliably in test environment.
	time.Sleep(2 * time.Second)

	// 5. Verify results
	wants := s.globalBuilder.GetWants()
	t.Logf("Total wants found in system: %d", len(wants))

	var parentWant, budgetWant, hotelWant *mywant.Want
	
	for _, w := range wants {
		if w.Metadata.Name == "integration-test-planner" {
			parentWant = w
		}
		// Match child wants by their common prefix set in params
		if w.Metadata.Type == "budget" {
			budgetWant = w
		}
		if w.Metadata.Type == "hotel" {
			hotelWant = w
		}
	}

	if parentWant == nil {
		t.Fatal("Parent want 'integration-test-planner' not found")
	}
	if budgetWant == nil {
		t.Fatal("Child 'budget' want not found")
	}
	if hotelWant == nil {
		t.Fatal("Child 'hotel' want not found")
	}

	parentID := parentWant.Metadata.ID
	t.Logf("Parent ID: %s", parentID)
	t.Logf("Budget ID: %s", budgetWant.Metadata.ID)
	t.Logf("Hotel ID:  %s", hotelWant.Metadata.ID)

	// Expected label: stateAccess/<parentID>.costs
	expectedLabel := "stateAccess/" + parentID + ".costs"

	verifyCorrelation := func(t *testing.T, w *mywant.Want, peerID string, label string) {
		found := false
		for _, entry := range w.Metadata.Correlation {
			if entry.WantID == peerID {
				for _, l := range entry.Labels {
					if l == label {
						found = true
						break
					}
				}
			}
		}
		if !found {
			t.Errorf("Want %s (%s): Correlation with %s via %s not found. Current entries: %v", 
				w.Metadata.Name, w.Metadata.ID, peerID, label, w.Metadata.Correlation)
		} else {
			t.Logf("✅ Want %s has correlation with %s via %s", w.Metadata.Name, peerID, label)
		}
	}

	t.Run("SiblingCorrelation", func(t *testing.T) {
		verifyCorrelation(t, budgetWant, hotelWant.Metadata.ID, expectedLabel)
		verifyCorrelation(t, hotelWant, budgetWant.Metadata.ID, expectedLabel)
	})

	t.Run("ParentCorrelation", func(t *testing.T) {
		verifyCorrelation(t, budgetWant, parentID, expectedLabel)
		verifyCorrelation(t, hotelWant, parentID, expectedLabel)
	})
	
	if !t.Failed() {
		t.Log("✅ Integration test passed: travel-budget system successfully deployed with correct structural correlations.")
	}
}
