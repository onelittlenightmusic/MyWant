package integration_tests

import (
	"bytes"
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// Helper functions for HTTP requests
func sendGetRequest(t *testing.T, url string) (*http.Response, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func sendPostRequest(t *testing.T, url string, contentType string, body []byte) (*http.Response, error) {
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	client := &http.Client{}
	return client.Do(req)
}

func readJSONResponse(resp *http.Response, target any) error {
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, target)
}

func containsAny(s string, subs []string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

type ItineraryEvent struct {
	Type string `json:"type"`
	Name string `json:"name"`
}

func TestDynamicTravelChangeIntegration(t *testing.T) {
	// --- 0. Pre-test Cleanup ---
	t.Log("Cleaning up existing processes...")
	cleanupCmd := exec.Command("./want-cli", "stop")
	cleanupCmd.Dir = ".."
	cleanupCmd.Run()
	
	exec.Command("pkill", "-9", "-f", "bin/mywant").Run()
	exec.Command("pkill", "-9", "-f", "bin/flight-server").Run()
	exec.Command("pkill", "-9", "-f", "want-cli").Run()
	rmCmd := exec.Command("rm", "-f", "engine/memory/*.yaml")
	rmCmd.Dir = ".."
	rmCmd.Run()
	time.Sleep(5 * time.Second)

	// --- 1. Build binaries ---
	t.Log("Building binaries...")
	buildSrv := exec.Command("make", "build-server")
	buildSrv.Dir = ".."
	buildSrv.Run()

	buildCLICmd := exec.Command("make", "build-cli")
	buildCLICmd.Dir = ".."
	buildCLICmd.Run()

	buildMock := exec.Command("make", "build-mock")
	buildMock.Dir = ".."
	buildMock.Run()

	// --- 2. Start Mock Flight Server ---
	t.Log("Starting mock flight server...")
	mockCmd := exec.Command("./bin/flight-server")
	mockCmd.Dir = ".."
	mockOut, _ := os.Create("mock_server.log")
	defer mockOut.Close()
	mockCmd.Stdout = mockOut
	mockCmd.Stderr = mockOut
	if err := mockCmd.Start(); err != nil {
		t.Fatalf("Failed to start mock server: %v", err)
	}
	defer mockCmd.Process.Kill()
	time.Sleep(2 * time.Second)

	// --- 3. Start MyWant Server via want-cli ---
	t.Log("Starting MyWant server via want-cli...")
	startCmd := exec.Command("./want-cli", "start", "-D", "--port", "8080")
	startCmd.Dir = ".."
	if err := startCmd.Run(); err != nil {
		t.Fatalf("Failed to start MyWant server: %v", err)
	}
	
	defer func() {
		t.Log("Stopping MyWant server via want-cli...")
		stopServerCmd := exec.Command("./want-cli", "stop")
		stopServerCmd.Dir = ".."
		stopServerCmd.Run()
	}()
	
	time.Sleep(5 * time.Second)

	// --- 4. Deploy Recipe ---
	t.Log("Deploying dynamic travel change recipe...")
	configPath := "../config/config-dynamic-travel-change.yaml"
	content, err := ioutil.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read config file: %v", err)
	}
	resp, err := sendPostRequest(t, "http://localhost:8080/api/v1/wants", "application/yaml", content)
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("Failed to create want: %v", err)
	}

	var createResp struct {
		WantIDs []string `json:"want_ids"`
	}
	readJSONResponse(resp, &createResp)
	topLevelID := createResp.WantIDs[0]
	t.Logf("Scenario deployed. Top-level Target ID: %s", topLevelID)

	// --- 5. Monitor Coordinator State ---
	t.Log("Monitoring Coordinator for dynamic updates...")
	// Timeout allows for: initial collection (10s) + delay trigger (40s) + rebooking (10s) + buffer
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	var coordinatorID string

	for {
		select {
		case <-ctx.Done():
			// Final attempt to log state before failure
			if coordinatorID != "" {
				detailResp, _ := sendGetRequest(t, "http://localhost:8080/api/v1/wants/"+coordinatorID)
				if detailResp != nil {
					body, _ := ioutil.ReadAll(detailResp.Body)
					t.Errorf("TIMEOUT DUMP: Coordinator State: %s", string(body))
				}
			}
			t.Fatal("Test timed out before rebooking was confirmed")
		case <-ticker.C:
			// 5.1. Find the coordinator ID if not yet known
			if coordinatorID == "" {
				listResp, err := sendGetRequest(t, "http://localhost:8080/api/v1/wants")
				if err != nil {
					continue
				}
				var listData struct {
					Wants []struct {
						Metadata struct {
							ID   string `json:"id"`
							Type string `json:"type"`
							OwnerReferences []struct {
								ID string `json:"id"`
							} `json:"ownerReferences"`
						} `json:"metadata"`
					} `json:"wants"`
				}
				if err := readJSONResponse(listResp, &listData); err != nil {
					continue
				}

				for _, w := range listData.Wants {
					for _, ref := range w.Metadata.OwnerReferences {
						if ref.ID == topLevelID && w.Metadata.Type == "coordinator" {
							coordinatorID = w.Metadata.ID
							t.Logf("Found Coordinator ID: %s", coordinatorID)
							break
						}
					}
				}
				if coordinatorID == "" {
					continue
				}
			}

			// 5.2. Inspect Coordinator details
			detailResp, err := sendGetRequest(t, "http://localhost:8080/api/v1/wants/"+coordinatorID)
			if err != nil {
				continue
			}
			body, _ := ioutil.ReadAll(detailResp.Body)
			detailResp.Body.Close()
			
			var fullData map[string]any
			if err := json.Unmarshal(body, &fullData); err != nil {
				continue
			}

			// Get hidden_state where final_itinerary usually resides
			hiddenState, _ := fullData["hidden_state"].(map[string]any)
			state, _ := fullData["state"].(map[string]any)

			// Try to find itinerary in either state or hidden_state
			itineraryRaw, ok := hiddenState["final_itinerary"]
			if !ok {
				itineraryRaw, ok = state["final_itinerary"]
			}
			
			if !ok || itineraryRaw == nil {
				continue
			}

			// Marshal and unmarshal to get strongly typed events
			itineraryBytes, _ := json.Marshal(itineraryRaw)
			var events []ItineraryEvent
			json.Unmarshal(itineraryBytes, &events)

			// Map types to verify coverage
			foundTypes := make(map[string]string)
			for _, e := range events {
				foundTypes[e.Type] = e.Name
			}

			hasRestaurant := foundTypes["restaurant"] != ""
			hasHotel := foundTypes["hotel"] != ""
			hasBuffet := foundTypes["buffet"] != ""
			flightName := foundTypes["flight"]
			hasFlight := flightName != ""

			t.Logf("[MONITOR] Itinerary Events: %d | R:%v H:%v B:%v F:%v (%s)", 
				len(foundTypes), hasRestaurant, hasHotel, hasBuffet, hasFlight, flightName)

			// 終了条件: すべてのスケジュール(4種類)が揃っており、かつフライトが再予約済み(AA100A-E)であること
			if hasRestaurant && hasHotel && hasBuffet && hasFlight &&
			   containsAny(flightName, []string{"AA100A", "AA100B", "AA100C", "AA100D", "AA100E"}) {
				t.Logf("✅ SUCCESS: Dynamic rebooking verified. Final flight: %s. All 4 schedules intact.", flightName)
				return // テスト成功
			}
		}
	}
}
