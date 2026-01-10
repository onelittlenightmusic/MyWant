package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Minimal types for the test
type WantMetadata struct {
	ID              string            `json:"id,omitempty"`
	Name            string            `json:"name"`
	Type            string            `json:"type"`
	OwnerReferences []OwnerReference  `json:"ownerReferences,omitempty"`
}

type OwnerReference struct {
	Name string `json:"name"`
	ID   string `json:"id"`
	Kind string `json:"kind"`
}

type WantSpec struct {
	Params Record `json:"params"`
}

type Record map[string]interface{};

type Want struct {
	Metadata WantMetadata `json:"metadata"`
	Spec     WantSpec     `json:"spec"`
	Status   string       `json:"status"`
}

type CreateWantResponse struct {
	ID      string   `json:"id"`
	WantIDs []string `json:"want_ids"`
}

type WantsListResponse struct {
	Wants []*Want `json:"wants"`
}

func main() {
	baseURL := "http://localhost:8080"
	fmt.Println("🚀 Starting Approval Workflow Test")

	// 0. Check server health
	fmt.Print("🔍 Checking server health... ")
	hResp, err := http.Get(baseURL + "/health")
	if err != nil {
		fmt.Printf("\n❌ Server not responding at %s. Make sure it's running.\n", baseURL)
		return
	}
	hResp.Body.Close()
	fmt.Println("OK")

	// 1. Deploy \"level 1 approval\" want
	wantName := fmt.Sprintf("test-approval-%d", time.Now().Unix())
	payload := Record{
		"wants": []Record{
			{
				"metadata": Record{
					"name": wantName,
					"type": "level 1 approval",
				},
				"spec": Record{
					"params": Record{
						"approval_id":      "test-id-123",
						"coordinator_type": "level1",
						"level2_authority": "senior_manager",
					},
				},
			},
		},
	}

	jsonBody, _ := json.Marshal(payload)
	fmt.Printf("📝 Deploying want: %s (type: level 1 approval)\n", wantName)
	resp, err := http.Post(baseURL+"/api/v1/wants", "application/json", bytes.NewBuffer(jsonBody))
	if err != nil {
		fmt.Printf("❌ Failed to deploy want: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("❌ Unexpected status code: %d, Response: %s\n", resp.StatusCode, string(body))
		return
	}

	var createResp CreateWantResponse
	json.NewDecoder(resp.Body).Decode(&createResp)
	
	// The parent want ID might be the first in want_ids or we can find it by name later
	fmt.Printf("✅ Want deployment queued. ID: %s\n", createResp.ID)

	// 2. Wait for 10 seconds
	fmt.Println("⏳ Waiting 10 seconds for child wants to be created and processed...")
	time.Sleep(10 * time.Second)

	// 3. Check if all child wants are achieved
	fmt.Println("🔍 Checking status of parent and child wants...")
	resp, err = http.Get(baseURL + "/api/v1/wants")
	if err != nil {
		fmt.Printf("❌ Failed to fetch wants: %v\n", err)
		return
	}
	defer resp.Body.Close()

	var listResp WantsListResponse
	json.NewDecoder(resp.Body).Decode(&listResp)

	var parentWant *Want
	var childWants []*Want

	// Find parent by name
	for _, w := range listResp.Wants {
		if w.Metadata.Name == wantName {
			parentWant = w
			break
		}
	}

	if parentWant == nil {
		fmt.Println("❌ Parent want not found in the list!")
		return
	}

	parentID := parentWant.Metadata.ID
	fmt.Printf("📍 Parent Want: %s (ID: %s, Status: %s)\n", wantName, parentID, parentWant.Status)

	// Find children by ownerReferences
	for _, w := range listResp.Wants {
		for _, ref := range w.Metadata.OwnerReferences {
			if ref.ID == parentID || (ref.Name == wantName && ref.Kind == "Want") {
				childWants = append(childWants, w)
				break
			}
		}
	}

	fmt.Printf("👶 Found %d child wants\n", len(childWants))
	
	allAchieved := true
	if parentWant.Status != "achieved" {
		allAchieved = false
		fmt.Printf("❌ Parent status is %s, expected achieved\n", parentWant.Status)
	}

	for _, child := range childWants {
		fmt.Printf("  - Child: %s (Type: %s, Status: %s)\n", child.Metadata.Name, child.Metadata.Type, child.Status)
		if child.Status != "achieved" {
			allAchieved = true // Changed my mind, level 2 approval might still be reaching
			// Wait, the requirement was "確認するテストを...追加して" (add a test to verify...)
			// Actually, "level 1 approval" has a child "level 2 approval" which might take more time or depend on others.
			// But the user asked to check if all child wants are achieved after 10s.
			allAchieved = false
		}
	}

	if allAchieved && len(childWants) > 0 {
		fmt.Println("🎉 PASS: All wants are achieved!")
	} else if len(childWants) == 0 {
		fmt.Println("⚠️  FAIL: No child wants found. Did the recipe instantiate correctly?")
	} else {
		fmt.Println("❌ FAIL: Some wants are not achieved yet.")
	}
}
