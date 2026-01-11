package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

// MCP Server for MyWant API
// Exposes MyWant operations (want types, recipes, wants) as MCP tools for Goose

type MCPServer struct {
	apiBaseURL string
	httpClient *http.Client
}

// MCP JSON-RPC 2.0 message structures
type JSONRPCRequest struct {
	JSONRPC string                 `json:"jsonrpc"`
	ID      interface{}            `json:"id"`
	Method  string                 `json:"method"`
	Params  map[string]interface{} `json:"params,omitempty"`
}

type JSONRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// MCP Protocol messages
type InitializeResult struct {
	ProtocolVersion string       `json:"protocolVersion"`
	ServerInfo      ServerInfo   `json:"serverInfo"`
	Capabilities    Capabilities `json:"capabilities"`
}

type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type Capabilities struct {
	Tools map[string]bool `json:"tools"`
}

type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

func NewMCPServer(apiURL string) *MCPServer {
	return &MCPServer{
		apiBaseURL: apiURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (s *MCPServer) handleRequest(req JSONRPCRequest) JSONRPCResponse {
	log.Printf("[MCP] Handling method: %s", req.Method)

	switch req.Method {
	case "initialize":
		return s.handleInitialize(req)
	case "tools/list":
		return s.handleToolsList(req)
	case "tools/call":
		return s.handleToolCall(req)
	default:
		return JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &RPCError{
				Code:    -32601,
				Message: fmt.Sprintf("Method not found: %s", req.Method),
			},
		}
	}
}

func (s *MCPServer) handleInitialize(req JSONRPCRequest) JSONRPCResponse {
	result := InitializeResult{
		ProtocolVersion: "2024-11-05",
		ServerInfo: ServerInfo{
			Name:    "mywant-mcp-server",
			Version: "1.0.0",
		},
		Capabilities: Capabilities{
			Tools: map[string]bool{
				"enabled": true,
			},
		},
	}

	return JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  result,
	}
}

func (s *MCPServer) handleToolsList(req JSONRPCRequest) JSONRPCResponse {
	tools := []Tool{
		{
			Name:        "list_want_types",
			Description: "List all available want types with name, category, title, and pattern",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			Name:        "get_want_type",
			Description: "Get detailed want type definition including parameters, state, and agents",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Name of the want type",
					},
				},
				"required": []string{"name"},
			},
		},
		{
			Name:        "search_want_types",
			Description: "Search want types by category or pattern",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"category": map[string]interface{}{
						"type":        "string",
						"description": "Category to filter by (e.g., travel, data, communication)",
					},
					"pattern": map[string]interface{}{
						"type":        "string",
						"description": "Pattern to filter by (e.g., generator, processor, sink)",
					},
				},
			},
		},
		{
			Name:        "list_recipes",
			Description: "List all available recipes",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			Name:        "get_recipe",
			Description: "Get detailed recipe information including wants, parameters, and metadata",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"recipe_id": map[string]interface{}{
						"type":        "string",
						"description": "ID of the recipe",
					},
				},
				"required": []string{"recipe_id"},
			},
		},
	}

	return JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: map[string]interface{}{
			"tools": tools,
		},
	}
}

func (s *MCPServer) handleToolCall(req JSONRPCRequest) JSONRPCResponse {
	toolName, ok := req.Params["name"].(string)
	if !ok {
		return JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &RPCError{
				Code:    -32602,
				Message: "Invalid params: missing tool name",
			},
		}
	}

	arguments, _ := req.Params["arguments"].(map[string]interface{})

	log.Printf("[MCP] Tool call: %s with args: %v", toolName, arguments)

	var result interface{}
	var err error

	switch toolName {
	case "list_want_types":
		result, err = s.listWantTypes(context.Background())
	case "get_want_type":
		name, _ := arguments["name"].(string)
		result, err = s.getWantType(context.Background(), name)
	case "search_want_types":
		category, _ := arguments["category"].(string)
		pattern, _ := arguments["pattern"].(string)
		result, err = s.searchWantTypes(context.Background(), category, pattern)
	case "list_recipes":
		result, err = s.listRecipes(context.Background())
	case "get_recipe":
		recipeID, _ := arguments["recipe_id"].(string)
		result, err = s.getRecipe(context.Background(), recipeID)
	default:
		return JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &RPCError{
				Code:    -32601,
				Message: fmt.Sprintf("Unknown tool: %s", toolName),
			},
		}
	}

	if err != nil {
		return JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &RPCError{
				Code:    -32603,
				Message: fmt.Sprintf("Tool execution failed: %v", err),
			},
		}
	}

	return JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: map[string]interface{}{
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": formatToolResult(result),
				},
			},
		},
	}
}

func (s *MCPServer) listWantTypes(ctx context.Context) (interface{}, error) {
	url := fmt.Sprintf("%s/api/v1/want-types", s.apiBaseURL)
	resp, err := s.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch want types: %w", err)
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

func (s *MCPServer) getWantType(ctx context.Context, name string) (interface{}, error) {
	url := fmt.Sprintf("%s/api/v1/want-types/%s", s.apiBaseURL, name)
	resp, err := s.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch want type: %w", err)
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

func (s *MCPServer) searchWantTypes(ctx context.Context, category, pattern string) (interface{}, error) {
	// For now, fetch all types and filter client-side
	// TODO: Add query parameters to API if needed
	allTypes, err := s.listWantTypes(ctx)
	if err != nil {
		return nil, err
	}

	// If no filters, return all
	if category == "" && pattern == "" {
		return allTypes, nil
	}

	// Filter logic would go here
	// For now, return all types (Goose can filter if needed)
	return allTypes, nil
}

func (s *MCPServer) listRecipes(ctx context.Context) (interface{}, error) {
	url := fmt.Sprintf("%s/api/v1/recipes", s.apiBaseURL)
	resp, err := s.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch recipes: %w", err)
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

func (s *MCPServer) getRecipe(ctx context.Context, recipeID string) (interface{}, error) {
	url := fmt.Sprintf("%s/api/v1/recipes/%s", s.apiBaseURL, recipeID)
	resp, err := s.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch recipe: %w", err)
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

func main() {
	apiURL := flag.String("api-url", "http://localhost:8080", "MyWant API base URL")
	flag.Parse()

	log.SetOutput(os.Stderr) // Log to stderr, STDIO is for JSON-RPC
	log.Printf("[MCP] Starting MyWant MCP Server")
	log.Printf("[MCP] API URL: %s", *apiURL)

	server := NewMCPServer(*apiURL)

	scanner := bufio.NewScanner(os.Stdin)
	writer := bufio.NewWriter(os.Stdout)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		log.Printf("[MCP] Received: %s", line)

		var req JSONRPCRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			log.Printf("[MCP] Failed to parse request: %v", err)
			continue
		}

		resp := server.handleRequest(req)

		respBytes, err := json.Marshal(resp)
		if err != nil {
			log.Printf("[MCP] Failed to marshal response: %v", err)
			continue
		}

		log.Printf("[MCP] Sending: %s", string(respBytes))

		writer.Write(respBytes)
		writer.WriteString("\n")
		writer.Flush()
	}

	if err := scanner.Err(); err != nil {
		log.Printf("[MCP] Scanner error: %v", err)
	}

	log.Printf("[MCP] Server shutting down")
}
