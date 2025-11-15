package main

import (
	"context"
	"fmt"
	"os"

	"google.golang.org/genai"
)

func main() {
	apiKey := os.Getenv("GOOGLE_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("GEMINI_API_KEY")
	}
	if apiKey == "" {
		fmt.Println("API key not set")
		return
	}

	ctx := context.Background()
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Println("Listing available models...")
	resp, err := client.Models.ListModels(ctx)
	if err != nil {
		fmt.Printf("Error listing models: %v\n", err)
		return
	}

	for _, model := range resp.Models {
		fmt.Printf("- %s\n", model.Name)
	}
}
