package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MCPServer for MyWant API
type MCPServer struct {
	apiBaseURL string
	httpClient *http.Client
}

func NewMCPServer(apiURL string) *MCPServer {
	return &MCPServer{
		apiBaseURL: apiURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (s *MCPServer) listWantTypes(ctx context.Context) (interface{}, error) {
	url := fmt.Sprintf("%s/api/v1/want-types", s.apiBaseURL)
	return s.doGet(url)
}

func (s *MCPServer) getWantType(ctx context.Context, name string) (interface{}, error) {
	url := fmt.Sprintf("%s/api/v1/want-types/%s", s.apiBaseURL, name)
	return s.doGet(url)
}

func (s *MCPServer) listRecipes(ctx context.Context) (interface{}, error) {
	url := fmt.Sprintf("%s/api/v1/recipes", s.apiBaseURL)
	return s.doGet(url)
}

func (s *MCPServer) getRecipe(ctx context.Context, recipeID string) (interface{}, error) {
	url := fmt.Sprintf("%s/api/v1/recipes/%s", s.apiBaseURL, recipeID)
	return s.doGet(url)
}

func (s *MCPServer) doGet(url string) (interface{}, error) {
	resp, err := s.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	var result interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result, nil
}

func formatToolResult(result interface{}) string {
	bytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", result)
	}
	return string(bytes)
}

// ErrorResult is a helper to return a CallToolResult with an error message
func ErrorResult(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{
			&mcp.TextContent{Text: msg},
		},
	}
}

// SuccessResult is a helper to return a CallToolResult with text content
func SuccessResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: text},
		},
	}
}

func main() {
	apiURL := flag.String("api-url", "http://localhost:8080", "MyWant API base URL")
	flag.Parse()

	// Log to stderr as MCP uses stdout for JSON-RPC
	log.SetOutput(os.Stderr)
	log.Printf("[MCP] Starting MyWant MCP Server with Go SDK")
	log.Printf("[MCP] API URL: %s", *apiURL)

	myWantServer := NewMCPServer(*apiURL)

	// Create a new MCP server
	server := mcp.NewServer(
		&mcp.Implementation{
			Name:    "mywant-mcp-server",
			Version: "1.1.0",
		},
		nil,
	)

	// Register list_want_types tool
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_want_types",
		Description: "List all available want types with name, category, title, and pattern",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input struct{}) (*mcp.CallToolResult, struct{}, error) {
		result, err := myWantServer.listWantTypes(ctx)
		if err != nil {
			return ErrorResult(err.Error()), struct{}{}, nil
		}
		return SuccessResult(formatToolResult(result)), struct{}{}, nil
	})

	// Register get_want_type tool
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_want_type",
		Description: "Get detailed want type definition including parameters, state, and agents",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input struct {
		Name string `json:"name" jsonschema:"Name of the want type"`
	}) (*mcp.CallToolResult, struct{}, error) {
		result, err := myWantServer.getWantType(ctx, input.Name)
		if err != nil {
			return ErrorResult(err.Error()), struct{}{}, nil
		}
		return SuccessResult(formatToolResult(result)), struct{}{}, nil
	})

	// Register search_want_types tool
	mcp.AddTool(server, &mcp.Tool{
		Name:        "search_want_types",
		Description: "Search want types by category or pattern",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input struct {
		Category string `json:"category,omitempty" jsonschema:"Category to filter by (e.g., travel, data, communication)"`
		Pattern  string `json:"pattern,omitempty" jsonschema:"Pattern to filter by (e.g., generator, processor, sink)"`
	}) (*mcp.CallToolResult, struct{}, error) {
		// Just reuse listWantTypes for now as in the original implementation
		result, err := myWantServer.listWantTypes(ctx)
		if err != nil {
			return ErrorResult(err.Error()), struct{}{}, nil
		}
		return SuccessResult(formatToolResult(result)), struct{}{}, nil
	})

	// Register list_recipes tool
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_recipes",
		Description: "List all available recipes",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input struct{}) (*mcp.CallToolResult, struct{}, error) {
		result, err := myWantServer.listRecipes(ctx)
		if err != nil {
			return ErrorResult(err.Error()), struct{}{}, nil
		}
		return SuccessResult(formatToolResult(result)), struct{}{}, nil
	})

	// Register get_recipe tool
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_recipe",
		Description: "Get detailed recipe information including wants, parameters, and metadata",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input struct {
		RecipeID string `json:"recipe_id" jsonschema:"ID of the recipe"`
	}) (*mcp.CallToolResult, struct{}, error) {
		result, err := myWantServer.getRecipe(ctx, input.RecipeID)
		if err != nil {
			return ErrorResult(err.Error()), struct{}{}, nil
		}
		return SuccessResult(formatToolResult(result)), struct{}{}, nil
	})

	// Start the server using stdio transport
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
