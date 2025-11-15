package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"google.golang.org/genai"
	"gopkg.in/yaml.v3"
)

// RecipeRequest represents the recipe structure for API submission
type RecipeRequest struct {
	Recipe struct {
		Metadata struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			CustomType  string `json:"custom_type"`
			Version     string `json:"version"`
		} `json:"metadata"`
		Parameters map[string]interface{} `json:"parameters"`
		Wants      []map[string]interface{} `json:"wants"`
		Example    map[string]interface{} `json:"example,omitempty"`
	} `json:"recipe"`
}

// RecipeYAML represents the recipe in YAML format
type RecipeYAML struct {
	Recipe struct {
		Metadata struct {
			Name        string `yaml:"name"`
			Description string `yaml:"description"`
			CustomType  string `yaml:"custom_type"`
			Version     string `yaml:"version"`
		} `yaml:"metadata"`
		Parameters map[string]interface{} `yaml:"parameters"`
		Wants      []map[string]interface{} `yaml:"wants"`
		Example    map[string]interface{} `yaml:"example,omitempty"`
	} `yaml:"recipe"`
}

func main() {
	// Define command-line flags
	userRequest := flag.String("request", "", "Natural language description of the recipe to create (e.g., 'Create a queue processing pipeline with 3 stages')")
	recipeName := flag.String("name", "", "Optional: Override recipe name from request")
	serverURL := flag.String("server", "http://localhost:8080", "Backend server URL")
	readmeFile := flag.String("readme", "../recipes/README.md", "Path to recipes/README.md for context")
	interactive := flag.Bool("interactive", false, "Interactive mode - prompts for user input")
	flag.Parse()

	// Get API key - try both GEMINI_API_KEY and GOOGLE_API_KEY
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("GOOGLE_API_KEY")
	}
	if apiKey == "" {
		log.Fatal("Neither GEMINI_API_KEY nor GOOGLE_API_KEY environment variable is set")
	}

	// Handle interactive mode
	if *interactive {
		runInteractiveMode(apiKey, *readmeFile, *serverURL)
		return
	}

	// Validate inputs
	if *userRequest == "" {
		fmt.Println("❌ Error: Please provide a recipe description using -request flag")
		fmt.Println("\nUsage: genai-runner -request \"Create a queue processing pipeline\" [-name custom-name] [-server http://localhost:8080] [-interactive]")
		fmt.Println("\nExample:")
		fmt.Println("  genai-runner -request \"Create a simple travel itinerary recipe with restaurant and hotel\" -name my-travel")
		fmt.Println("  genai-runner -request \"Create a fibonacci number generator pipeline\" -name fibonacci")
		fmt.Println("  genai-runner -interactive")
		os.Exit(1)
	}

	// Read README for context
	readmeContent, err := os.ReadFile(*readmeFile)
	if err != nil {
		fmt.Printf("⚠️  Warning: Could not read README at %s: %v\n", *readmeFile, err)
		readmeContent = []byte("")
	}

	// Create recipe using GenAI
	recipe, err := generateRecipe(apiKey, *userRequest, string(readmeContent), *recipeName)
	if err != nil {
		log.Fatalf("❌ Failed to generate recipe: %v", err)
	}

	// Display generated recipe
	fmt.Println("\n✅ Generated Recipe:")
	fmt.Println("════════════════════════════════════════════════════")
	recipeYAML, err := yaml.Marshal(recipe.Recipe)
	if err != nil {
		log.Fatalf("Failed to marshal recipe to YAML: %v", err)
	}
	fmt.Printf("recipe:\n%s", string(recipeYAML))

	// Register recipe with backend server
	fmt.Println("\n📤 Registering recipe with backend server...")
	recipeID, err := registerRecipeWithServer(*serverURL, recipe)
	if err != nil {
		log.Fatalf("❌ Failed to register recipe: %v", err)
	}

	fmt.Printf("\n✅ Recipe successfully registered!\n")
	fmt.Printf("   Recipe ID: %s\n", recipeID)
	fmt.Printf("   Server: %s\n", *serverURL)
	fmt.Printf("   Endpoint: %s/api/v1/recipes/%s\n", *serverURL, recipeID)
}

// runInteractiveMode prompts the user for recipe requests
func runInteractiveMode(apiKey, readmeFile, serverURL string) {
	fmt.Println("🧠 GenAI Recipe Creator - Interactive Mode")
	fmt.Println("════════════════════════════════════════════════════")
	fmt.Println("\nThis tool creates recipe definitions from natural language descriptions.")
	fmt.Println("Recipes are automatically registered with the MyWant backend server.\n")

	// Read README for context
	readmeContent, err := os.ReadFile(readmeFile)
	if err != nil {
		fmt.Printf("⚠️  Warning: Could not read README at %s\n", readmeFile)
		readmeContent = []byte("")
	}

	for {
		fmt.Println("\n" + strings.Repeat("─", 60))
		fmt.Print("📝 Describe the recipe you want to create (or 'quit' to exit):\n> ")

		// Read user input
		inputBytes := make([]byte, 1024)
		n, _ := os.Stdin.Read(inputBytes)
		userRequest := strings.TrimSpace(string(inputBytes[:n]))

		if strings.ToLower(userRequest) == "quit" || strings.ToLower(userRequest) == "exit" {
			fmt.Println("\n👋 Goodbye!")
			break
		}

		if userRequest == "" {
			fmt.Println("❌ Empty input. Please describe a recipe.")
			continue
		}

		// Generate recipe
		fmt.Println("\n🤖 Generating recipe from your request...")
		recipe, err := generateRecipe(apiKey, userRequest, string(readmeContent), "")
		if err != nil {
			fmt.Printf("❌ Error: %v\n", err)
			continue
		}

		// Display recipe
		fmt.Println("\n✅ Generated Recipe:")
		fmt.Println(strings.Repeat("─", 60))
		recipeYAML, _ := yaml.Marshal(recipe.Recipe)
		fmt.Printf("recipe:\n%s", string(recipeYAML))

		// Ask to register
		fmt.Print("\n📤 Register this recipe? (yes/no): ")
		confirmBytes := make([]byte, 10)
		nConfirm, _ := os.Stdin.Read(confirmBytes)
		confirm := strings.ToLower(strings.TrimSpace(string(confirmBytes[:nConfirm])))

		if confirm == "yes" || confirm == "y" {
			recipeID, err := registerRecipeWithServer(serverURL, recipe)
			if err != nil {
				fmt.Printf("❌ Failed to register: %v\n", err)
			} else {
				fmt.Printf("✅ Recipe registered! ID: %s\n", recipeID)
			}
		} else {
			fmt.Println("⏭️  Skipped registration.")
		}
	}
}

// generateRecipe uses Gemini to create a recipe from natural language
func generateRecipe(apiKey, userRequest, readmeContent, overrideName string) (*RecipeRequest, error) {
	ctx := context.Background()
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Gemini client: %v", err)
	}

	// Construct prompt with recipe documentation
	prompt := fmt.Sprintf(`You are a recipe generator for the MyWant functional chain programming system.

READ THIS RECIPE FORMAT DOCUMENTATION CAREFULLY:
%s

USER REQUEST: %s

Based on the request, generate a recipe in JSON format (not YAML, JSON only) that:
1. Uses realistic want types (queue, sink, sequence, restaurant, hotel, buffet, travel_coordinator, etc.)
2. Includes meaningful parameters that match the use case
3. Uses appropriate labels for connectivity
4. Follows the format shown in the documentation
5. IMPORTANT: Include an "example" field with one-click deployment configuration

Return ONLY valid JSON in this structure (no markdown, no code blocks):
{
  "recipe": {
    "metadata": {
      "name": "recipe-name",
      "description": "description",
      "custom_type": "type",
      "version": "1.0.0"
    },
    "parameters": {
      "param1": "value1"
    },
    "wants": [
      {
        "metadata": {
          "type": "want-type",
          "labels": {"role": "label"}
        },
        "spec": {
          "params": {"key": "value"}
        }
      }
    ],
    "example": {
      "wants": [
        {
          "metadata": {
            "name": "recipe-name-demo",
            "type": "owner",
            "labels": {
              "recipe": "recipe-name"
            }
          },
          "spec": {
            "params": {
              "param1": "value1"
            }
          }
        }
      ]
    }
  }
}`, readmeContent, userRequest)

	// Try different model names - gemini-2.0-flash is the newest/best
	modelNames := []string{
		"gemini-2.0-flash",
		"gemini-1.5-pro",
		"gemini-1.5-flash",
	}

	var resp *genai.GenerateContentResponse
	var lastErr error

	for _, modelName := range modelNames {
		fmt.Printf("🔄 Trying model: %s\n", modelName)
		contents := genai.Text(prompt)
		resp, lastErr = client.Models.GenerateContent(ctx, modelName, contents, nil)
		if lastErr == nil {
			fmt.Printf("✅ Model %s succeeded\n", modelName)
			break
		}
		// Check for rate limit - if we hit it, the model works but we need to wait
		if lastErr.Error() != "" && (contains(lastErr.Error(), "429") || contains(lastErr.Error(), "RESOURCE_EXHAUSTED")) {
			fmt.Printf("⚠️  Model %s hit rate limit. Please try again in a moment.\n", modelName)
			return nil, fmt.Errorf("rate limit exceeded. Please try again later")
		}
		fmt.Printf("⚠️  Model %s failed: %v\n", modelName, lastErr)
	}

	if resp == nil {
		return nil, fmt.Errorf("all models failed. Last error: %v", lastErr)
	}

	if len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
		return nil, fmt.Errorf("no content in response")
	}

	// Extract JSON from response
	var responseText string
	for _, part := range resp.Candidates[0].Content.Parts {
		responseText += part.Text
	}

	recipe, err := parseRecipeJSON(responseText)
	if err != nil {
		return nil, fmt.Errorf("failed to parse recipe: %v", err)
	}

	// Override name if provided
	if overrideName != "" {
		recipe.Recipe.Metadata.Name = overrideName
	}

	return recipe, nil
}

// parseRecipeJSON extracts and parses JSON from the response
func parseRecipeJSON(response string) (*RecipeRequest, error) {
	// Try to find JSON in the response
	startIdx := strings.Index(response, "{")
	endIdx := strings.LastIndex(response, "}")

	if startIdx == -1 || endIdx == -1 || startIdx >= endIdx {
		return nil, fmt.Errorf("no JSON found in response: %s", response)
	}

	jsonStr := response[startIdx : endIdx+1]

	// Parse the JSON
	var recipe RecipeRequest
	err := json.Unmarshal([]byte(jsonStr), &recipe)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %v", err)
	}

	// Validate recipe
	if recipe.Recipe.Metadata.Name == "" {
		recipe.Recipe.Metadata.Name = "generated-recipe"
	}
	if recipe.Recipe.Metadata.CustomType == "" {
		recipe.Recipe.Metadata.CustomType = "generated"
	}
	if recipe.Recipe.Metadata.Version == "" {
		recipe.Recipe.Metadata.Version = "1.0.0"
	}

	return &recipe, nil
}

// registerRecipeWithServer sends the recipe to the backend API
func registerRecipeWithServer(serverURL string, recipe *RecipeRequest) (string, error) {
	// Prepare the request body
	payload, err := json.Marshal(recipe)
	if err != nil {
		return "", fmt.Errorf("failed to marshal recipe: %v", err)
	}

	// Create the HTTP request
	url := fmt.Sprintf("%s/api/v1/recipes", serverURL)
	resp, err := http.Post(url, "application/json", bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("failed to connect to server at %s: %v", url, err)
	}
	defer resp.Body.Close()

	// Read response body
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %v", err)
	}

	// Check status code
	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// Parse response to get recipe ID
	var responseData map[string]interface{}
	err = json.Unmarshal(bodyBytes, &responseData)
	if err != nil {
		return "", fmt.Errorf("failed to parse server response: %v", err)
	}

	recipeID, ok := responseData["id"].(string)
	if !ok {
		return "", fmt.Errorf("recipe ID not found in response")
	}

	return recipeID, nil
}

// contains checks if a string contains a substring
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
