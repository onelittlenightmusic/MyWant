package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
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

type Record map[string]interface{}

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

	// 2. Poll for completion
	fmt.Println("⏳ Waiting for child wants to be created and processed (max 30s)...")
	startWait := time.Now()
	timeout := 30 * time.Second
	var parentWant *Want
	var childWants []*Want
	allAchieved := false

	for time.Since(startWait) < timeout {
		resp, err = http.Get(baseURL + "/api/v1/wants")
		if err != nil {
			fmt.Printf("❌ Failed to fetch wants: %v\n", err)
			return
		}
		
		var listResp WantsListResponse
		json.NewDecoder(resp.Body).Decode(&listResp)
		resp.Body.Close()

		parentWant = nil
		childWants = []*Want{}

		// Find parent by name
		for _, w := range listResp.Wants {
			if w.Metadata.Name == wantName {
				parentWant = w
				break
			}
		}

		if parentWant != nil {
			parentID := parentWant.Metadata.ID
			
			// Find children by ownerReferences
			for _, w := range listResp.Wants {
				for _, ref := range w.Metadata.OwnerReferences {
					if ref.ID == parentID || (ref.Name == wantName && ref.Kind == "Want") {
						childWants = append(childWants, w)
						break
					}
				}
			}

			// Check if all are achieved
			if parentWant.Status == "achieved" && len(childWants) >= 4 {
				allChildAchieved := true
				for _, child := range childWants {
					if child.Status != "achieved" {
						allChildAchieved = false
						break
					}
				}
				if allChildAchieved {
					allAchieved = true
					break
				}
			}
		}
		
		time.Sleep(2 * time.Second)
	}

	// 3. Final Result Report
	fmt.Println("🔍 Final Status Check:")
	if parentWant == nil {
		fmt.Println("❌ Parent want not found!")
		os.Exit(1)
	}

	fmt.Printf("📍 Parent Want: %s (ID: %s, Status: %s)\n", wantName, parentWant.Metadata.ID, parentWant.Status)
	fmt.Printf("👶 Found %d child wants\n", len(childWants))
	for _, child := range childWants {
		fmt.Printf("  - Child: %s (Type: %s, Status: %s)\n", child.Metadata.Name, child.Metadata.Type, child.Status)
	}

	if allAchieved {
		fmt.Printf("🎉 PASS: All wants achieved in %v!\n", time.Since(startWait).Round(time.Second))
	} else {
		fmt.Println("❌ FAIL: Timeout reached or some wants are not achieved.")
		os.Exit(1)
	}
}
