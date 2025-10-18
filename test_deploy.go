package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

func main() {
	// Read the config file
	configContent, err := os.ReadFile("config/config-dynamic-travel-change.yaml")
	if err != nil {
		fmt.Printf("Error reading config: %v\n", err)
		return
	}

	// Deploy to server
	req, err := http.NewRequest("POST", "http://localhost:8080/api/v1/wants",
		io.NopCloser(bytes.NewReader(configContent)))
	if err != nil {
		fmt.Printf("Error creating request: %v\n", err)
		return
	}
	req.Header.Set("Content-Type", "application/yaml")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Error deploying: %v\n", err)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("Deployment Status: %d\n", resp.StatusCode)
	fmt.Printf("Response: %s\n", string(body))

	// Wait for status progression
	fmt.Println("\nWaiting 50 seconds for flight status progression...")
	time.Sleep(50 * time.Second)

	// Retrieve flight state
	resp2, err := http.Get("http://localhost:8080/api/v1/wants")
	if err != nil {
		fmt.Printf("Error retrieving wants: %v\n", err)
		return
	}
	defer resp2.Body.Close()

	body2, _ := io.ReadAll(resp2.Body)
	fmt.Printf("\nWants list:\n%s\n", string(body2))
}
