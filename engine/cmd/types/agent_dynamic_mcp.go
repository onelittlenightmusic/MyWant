package types

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	mywant "mywant/engine/src"
)

// ============ Dynamic MCP Agents ============

// RegisterDynamicMCPAgents registers all specialized agents for dynamic wants
func RegisterDynamicMCPAgents(registry *mywant.AgentRegistry) {
	// 1. Discovery Agent
	registry.RegisterCapability(mywant.Capability{
		Name:  "mcp_dynamic_discovery",
		Gives: []string{"mcp_dynamic_discovery"},
	})
	registry.RegisterAgent(&mywant.DoAgent{
		BaseAgent: *mywant.NewBaseAgent("discovery_agent", []string{"mcp_dynamic_discovery"}, mywant.DoAgentType),
		Action:    executeDiscoveryAction,
	})

	// 2. Developer Agent
	registry.RegisterCapability(mywant.Capability{
		Name:  "mcp_dynamic_developer",
		Gives: []string{"mcp_dynamic_developer"},
	})
	registry.RegisterAgent(&mywant.DoAgent{
		BaseAgent: *mywant.NewBaseAgent("developer_agent", []string{"mcp_dynamic_developer"}, mywant.DoAgentType),
		Action:    executeDeveloperAction,
	})

	// 3. Compiler Agent
	registry.RegisterCapability(mywant.Capability{
		Name:  "mcp_dynamic_compiler",
		Gives: []string{"mcp_dynamic_compiler"},
	})
	registry.RegisterAgent(&mywant.DoAgent{
		BaseAgent: *mywant.NewBaseAgent("compiler_agent", []string{"mcp_dynamic_compiler"}, mywant.DoAgentType),
		Action:    executeCompilerAction,
	})

	// 4. Validator Agent
	registry.RegisterCapability(mywant.Capability{
		Name:  "mcp_dynamic_validator",
		Gives: []string{"mcp_dynamic_validator"},
	})
	registry.RegisterAgent(&mywant.DoAgent{
		BaseAgent: *mywant.NewBaseAgent("validator_agent", []string{"mcp_dynamic_validator"}, mywant.DoAgentType),
		Action:    executeValidatorAction,
	})
}

// --- Discovery Action ---
func executeDiscoveryAction(ctx context.Context, want *mywant.Want) error {
	prompt, _ := want.GetParameter("prompt")

	want.StoreLog("[DISCOVERY] Starting tool discovery for prompt: %s", prompt)

	goose, _ := GetGooseManager(ctx)

	discoveryPrompt := fmt.Sprintf(`Analyze the following request and find the appropriate Gmail MCP tool.
Request: "%v"

Steps:
1. Execute the tool via MCP
2. Record the EXACT JSON request sent to the MCP server
3. Record the EXACT JSON response received from the MCP server
4. Return ONLY a JSON object: {"request": {...}, "response": {...}}`, prompt)

	result, err := goose.ExecuteViaGoose(ctx, "mcp_discovery", map[string]interface{}{
		"provider": "gemini-cli",
		"prompt":   discoveryPrompt,
	})
	if err != nil {
		want.StoreLog("[DISCOVERY][ERROR] Goose execution failed: %v", err)
		return err
	}

	want.StoreLog("[DISCOVERY][SUCCESS] Tool discovery completed")
	want.StoreState("raw_samples", result)
	return nil
}

// --- Developer Action ---
func executeDeveloperAction(ctx context.Context, want *mywant.Want) error {
	samples, _ := want.GetState("raw_samples")
	feedback, _ := want.GetStateString("error_feedback", "")

	if feedback != "" {
		want.StoreLog("[DEVELOPER] Re-generating code with error feedback: %s", feedback)
	} else {
		want.StoreLog("[DEVELOPER] Generating code from samples")
	}

	goose, _ := GetGooseManager(ctx)

	devPrompt := fmt.Sprintf(`You are a code generation engine. Your output is piped directly into a compiler.

TASK:
Implement two Go functions for a WASM-based MCP tool adapter.

AVAILABLE PACKAGES:
- "encoding/json"
- "strings"
- "fmt"

INTERFACES TO IMPLEMENT:
1. func TransformRequest(params map[string]any) map[string]any
   - Maps MyWant parameters to the MCP tool's expected arguments.
2. func ParseResponse(rawResponse map[string]any) map[string]any
   - Maps the raw MCP tool output (from the "content" field) to a flat map for MyWant's state.

STRICT CONSTRAINTS:
- DO NOT output any conversational text, explanations, or thoughts.
- DO NOT output anything before or after the Go code block.
- DO NOT use any packages other than those listed above.
- Ensure all imports used in your code are listed in an "import" block.
- Handle potential nil maps or missing keys safely.

CONTEXT & PREVIOUS ERRORS:
%s

SAMPLES:
%v

FEEDBACK FROM PREVIOUS ATTEMPT:
%s

REQUIREMENTS:
1. Implement func TransformRequest(params map[string]any) map[string]any
2. Implement func ParseResponse(rawResponse map[string]any) map[string]any
3. Return ONLY the Go source code.

OUTPUT FORMAT:
` + "```go" + `
package main
import (...)
func TransformRequest(...) ...
func ParseResponse(...) ...
` + "```", "Implement Go functions to transform parameters and parse responses for a WASM plugin.", samples, feedback)

	result, err := goose.ExecuteViaGoose(ctx, "mcp_developer", map[string]interface{}{
		"provider": "gemini-cli",
		"prompt":   devPrompt,
	})
	if err != nil {
		want.StoreLog("[DEVELOPER][ERROR] Goose execution failed: %v", err)
		return err
	}

	code := ""
	if resMap, ok := result.(map[string]interface{}); ok {
		if text, ok := resMap["text"].(string); ok {
			code = text
		}
	} else if text, ok := result.(string); ok {
		code = text
	}

	want.StoreLog("[DEVELOPER][SUCCESS] Code generated (%d characters)", len(code))
	want.StoreState("source_code", code)
	return nil
}

// --- Compiler Action ---
func executeCompilerAction(ctx context.Context, want *mywant.Want) error {
	code, _ := want.GetStateString("source_code", "")
	if code == "" {
		return fmt.Errorf("no source code found in state")
	}

	tmpDir := os.TempDir()
	sourcePath := filepath.Join(tmpDir, fmt.Sprintf("plugin_%s.go", want.Metadata.ID))
	wasmPath := filepath.Join(tmpDir, fmt.Sprintf("plugin_%s.wasm", want.Metadata.ID))

	// Precise Code Extraction: Look for the first Go code block
	if strings.Contains(code, "```go") {
		parts := strings.Split(code, "```go")
		if len(parts) > 1 {
			code = strings.Split(parts[1], "```")[0]
		}
	} else if strings.Contains(code, "```") {
		parts := strings.Split(code, "```")
		if len(parts) > 1 {
			code = strings.Split(parts[1], "```")[0]
		}
	}

	// Just use the extracted code as-is
	fullCode := strings.TrimSpace(code)

	if err := os.WriteFile(sourcePath, []byte(fullCode), 0644); err != nil {
		return err
	}

	// Debugging: Print the full code being compiled and the paths
	fmt.Printf("[WASM Compiler Debug] Full Go source code to be compiled (ID: %s):\n%s\n", want.Metadata.ID, fullCode)
	fmt.Printf("[WASM Compiler Debug] Source file: %s\n", sourcePath)
	fmt.Printf("[WASM Compiler Debug] WASM output file: %s\n", wasmPath)

	// Run tinygo build
	cmd := exec.CommandContext(ctx, "tinygo", "build", "-target=wasi", "-o", wasmPath, sourcePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		errMsg := string(output)
		errorFeedback := fmt.Sprintf("Compilation error: %s", errMsg)
		want.StoreState("error_feedback", errorFeedback)
		want.StoreLog("[COMPILER][ERROR] %s", errorFeedback)
		
		// PRINT TO STDOUT FOR IMMEDIATE VISIBILITY
		fmt.Printf("[WASM-COMPILER-FAILURE] Want %s failed to compile Go to WASM.\nOutput: %s\n", want.Metadata.ID, errMsg)
		
		return fmt.Errorf("compilation failed: %w", err)
	}

	fmt.Printf("[WASM-COMPILER-SUCCESS] Want %s compiled successfully to: %s\n", want.Metadata.ID, wasmPath)
	want.StoreLog("[COMPILER][SUCCESS] WASM compiled successfully: %s", wasmPath)
	want.StoreState("wasm_path", wasmPath)
	want.StoreState("error_feedback", "") // Clear any previous error feedback
	return nil
}

// --- Validator Action ---
func executeValidatorAction(ctx context.Context, want *mywant.Want) error {
	wasmPath, _ := want.GetStateString("wasm_path", "")
	prompt, _ := want.GetParameter("prompt")

	want.StoreLog("[VALIDATOR] Starting validation with WASM: %s", wasmPath)

	manager := mywant.NewWasmManager(ctx)
	defer manager.Close(ctx)

	// 1. Transform Request
	inputJSON, _ := json.Marshal(map[string]any{"prompt": prompt})
	mcpReqRaw, err := manager.ExecuteFunction(ctx, wasmPath, "transform_request", inputJSON)
	if err != nil {
		errorFeedback := fmt.Sprintf("WASM TransformRequest error: %v", err)
		want.StoreState("error_feedback", errorFeedback)
		want.StoreLog("[VALIDATOR][ERROR] %s", errorFeedback)
		return err
	}

	var mcpReq map[string]any
	json.Unmarshal(mcpReqRaw, &mcpReq)
	want.StoreLog("[VALIDATOR] TransformRequest output: %v", mcpReq)

	// 2. Call MCP Directly
	native := GetNativeMCPManager(ctx)
	// We need to know which tool to call - should be part of WASM result or state
	toolName, _ := want.GetStateString("mcp_tool_name", "search_emails")

	want.StoreLog("[VALIDATOR] Calling MCP tool: %s with args: %v", toolName, mcpReq)
	toolResult, err := native.ExecuteTool(ctx, "gmail", "npx", []string{"-y", "@gongrzhe/server-gmail-autoauth-mcp"}, toolName, mcpReq)
	if err != nil {
		errorFeedback := fmt.Sprintf("MCP execution error: %v", err)
		want.StoreState("error_feedback", errorFeedback)
		want.StoreLog("[VALIDATOR][ERROR] %s", errorFeedback)
		return err
	}

	// 3. Parse Response
	respRaw, _ := json.Marshal(toolResult)
	finalResRaw, err := manager.ExecuteFunction(ctx, wasmPath, "parse_response", respRaw)
	if err != nil {
		errorFeedback := fmt.Sprintf("WASM ParseResponse error: %v", err)
		want.StoreState("error_feedback", errorFeedback)
		want.StoreLog("[VALIDATOR][ERROR] %s", errorFeedback)
		return err
	}

	var finalRes any
	json.Unmarshal(finalResRaw, &finalRes)

	want.StoreLog("[VALIDATOR][SUCCESS] Validation passed. Final result: %v", finalRes)
	want.StoreState("final_result", finalRes)
	want.StoreState("validation_success", true)
	want.StoreState("error_feedback", "") // Clear any previous error feedback
	return nil
}

// RegisterGmailDynamicWant registers the Gmail dynamic want type factory
func RegisterGmailDynamicWant(builder *mywant.ChainBuilder) {
	builder.RegisterWantType("gmail_dynamic", NewGmailDynamicWant)
}
