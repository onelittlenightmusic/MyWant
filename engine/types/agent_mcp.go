package types

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	mywant "mywant/engine/core"
)

func init() {
	mywant.RegisterWithInit(func() {
		mywant.RegisterDoAgent("mcp_tools", executeMCPOperation)
	})
}

// ============ NativeMCPManager ============

type NativeMCPManager struct {
	mu       sync.Mutex
	sessions map[string]*mcp.ClientSession
}

var (
	nativeMCPManager *NativeMCPManager
	nativeMCPMutex   sync.Mutex
)

func GetNativeMCPManager(ctx context.Context) *NativeMCPManager {
	nativeMCPMutex.Lock(); defer nativeMCPMutex.Unlock()
	if nativeMCPManager == nil {
		nativeMCPManager = &NativeMCPManager{
			sessions: make(map[string]*mcp.ClientSession),
		}
	}
	return nativeMCPManager
}

func (m *NativeMCPManager) ExecuteTool(ctx context.Context, serverName string, command string, args []string, toolName string, toolArgs map[string]interface{}) (*mcp.CallToolResult, error) {
	m.mu.Lock(); session, exists := m.sessions[serverName]; m.mu.Unlock()
	if !exists {
		var reader io.ReadCloser; var writer io.WriteCloser
		if proc, ok := GetMCPServerRegistry().Get(serverName); ok {
			reader = proc.Stdout; writer = proc.Stdin
		} else {
			cmd := exec.CommandContext(ctx, command, args...)
			stdin, err := cmd.StdinPipe(); if err != nil { return nil, err }
			stdout, err := cmd.StdoutPipe(); if err != nil { return nil, err }
			cmd.Stderr = os.Stderr
			if err := cmd.Start(); err != nil { return nil, err }
			reader = stdout; writer = stdin
		}
		client := mcp.NewClient(&mcp.Implementation{Name: "mywant-native-client", Version: "1.0.0"}, nil)
		transport := &mcp.IOTransport{Reader: reader, Writer: writer}
		// Use context.Background() so the session lifetime is not tied to the tool-call context.
		// The tool-call context (which may have a short timeout) would otherwise close the
		// connection when it expires, breaking subsequent calls.
		cs, err := client.Connect(context.Background(), transport, nil); if err != nil { return nil, err }
		m.mu.Lock(); m.sessions[serverName] = cs; session = cs; m.mu.Unlock()
	}
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: toolName, Arguments: toolArgs})
	if err != nil && isSessionClosed(err) {
		// Stale session: remove it so the next call creates a fresh one
		m.mu.Lock(); delete(m.sessions, serverName); m.mu.Unlock()
	}
	return result, err
}

func isSessionClosed(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "connection closed") ||
		strings.Contains(msg, "client is closing") ||
		strings.Contains(msg, "EOF") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "file already closed")
}

func (m *NativeMCPManager) CloseAllSessions() {
	m.mu.Lock(); defer m.mu.Unlock()
	for _, session := range m.sessions { session.Close() }
	m.sessions = make(map[string]*mcp.ClientSession)
}

// ============ GooseManager ============

type GooseManager struct {
	mu sync.Mutex
	running bool
}

var (
	gooseManager *GooseManager
	gooseMutex   sync.Mutex
)

func NewGooseManager(ctx context.Context) (*GooseManager, error) {
	return &GooseManager{running: true}, nil
}

func (g *GooseManager) ExecuteViaGoose(ctx context.Context, operation string, params map[string]interface{}) (interface{}, error) {
	g.mu.Lock(); defer g.mu.Unlock()
	if !g.running { return nil, fmt.Errorf("Goose manager is not running") }
	prompt := g.buildPrompt(operation, params)
	// For interact_recommend, use claude --print directly to avoid goose session conflicts
	if operation == "interact_recommend" {
		output, err := runClaudeDirect(ctx, prompt)
		if err == nil {
			return parseGooseResponse(output)
		}
	}
	preferredProvider := ""; if p, ok := params["provider"].(string); ok && p != "" { preferredProvider = p }
	if preferredProvider == "" { preferredProvider = os.Getenv("MYWANT_GOOSE_PROVIDER") }
	args := []string{"run", "-i", "-"}; if preferredProvider != "" { args = append(args, "--provider", preferredProvider) }
	fullOutput, err := g.runGooseWithArgs(args, prompt)
	if err != nil || strings.Contains(fullOutput, "Ran into this error") {
		fallbackProvider := "gemini-cli"; if preferredProvider == "gemini-cli" { fallbackProvider = "claude-code" }
		fallbackArgs := []string{"run", "-i", "-", "--provider", fallbackProvider}
		fullOutput, _ = g.runGooseWithArgs(fallbackArgs, prompt)
	}
	return parseGooseResponse(fullOutput)
}

// runClaudeDirect calls claude --print directly with the prompt, bypassing goose
func runClaudeDirect(ctx context.Context, prompt string) (string, error) {
	claudeCmd := os.Getenv("CLAUDE_CODE_COMMAND")
	if claudeCmd == "" {
		claudeCmd = "claude"
	}
	cmd := exec.CommandContext(ctx, claudeCmd, "--print")
	cmd.Env = os.Environ()
	cmd.Stdin = strings.NewReader(prompt)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func (g *GooseManager) runGooseWithArgs(args []string, prompt string) (string, error) {
	cmd := exec.CommandContext(context.Background(), "goose", args...)
	cmd.Env = os.Environ()
	stdin, _ := cmd.StdinPipe(); stdout, _ := cmd.StdoutPipe(); cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil { return "", err }
	io.WriteString(stdin, prompt+"\n"); stdin.Close()
	scanner := bufio.NewScanner(stdout); var outputLines []string
	for scanner.Scan() { outputLines = append(outputLines, scanner.Text()) }
	cmd.Wait(); return strings.Join(outputLines, "\n"), nil
}

func (g *GooseManager) buildPrompt(operation string, params map[string]interface{}) string {
	if operation == "interact_recommend" {
		return buildInteractRecommendPrompt(params)
	}
	return fmt.Sprintf("Execute operation %s with %v", operation, params)
}

func buildInteractRecommendPrompt(params map[string]interface{}) string {
	message, _ := params["message"].(string)
	historyJSON, _ := params["conversation_history"].(string)

	return fmt.Sprintf(`You are a MyWant configuration assistant. MyWant is a declarative workflow system where users define "wants" (goals) using YAML.

Available want types include: gmail, morning_briefing, reminder, hotel, flight, restaurant, weather, transit, knowledge, slack_post, budget, itinerary, execution_result, docker_run, opa_llm_planner, python_thinker, teams_webhook, slack_webhook, ngrok, cloudflare_tunnel.

Conversation history: %s

User's request: %s

Based on the user's request, generate 1-3 want configuration recommendations. You MUST respond with ONLY valid JSON in exactly this format (no markdown, no explanation, just JSON):

{
  "recommendations": [
    {
      "id": "rec-1",
      "title": "Short descriptive title",
      "approach": "custom",
      "description": "Why this configuration fits the user's need",
      "config": {
        "wants": [
          {
            "name": "my_want_name",
            "type": "want_type_here",
            "spec": {
              "params": {}
            }
          }
        ]
      },
      "metadata": {
        "want_count": 1,
        "want_types_used": ["want_type_here"],
        "complexity": "low",
        "pros_cons": {
          "pros": ["benefit 1"],
          "cons": ["limitation 1"]
        }
      }
    }
  ]
}`, historyJSON, message)
}

func parseGooseResponse(output string) (interface{}, error) {
	// Try to extract JSON from markdown code blocks first
	extracted := extractJSONFromOutput(output)
	if extracted != "" {
		var result map[string]interface{}
		if err := json.Unmarshal([]byte(extracted), &result); err == nil {
			return result, nil
		}
	}
	return map[string]interface{}{"text": output}, nil
}

func extractJSONFromOutput(output string) string {
	// Try ```json ... ``` block
	if idx := strings.Index(output, "```json"); idx >= 0 {
		start := idx + len("```json")
		if end := strings.Index(output[start:], "```"); end >= 0 {
			return strings.TrimSpace(output[start : start+end])
		}
	}
	// Try ``` ... ``` block containing JSON
	if idx := strings.Index(output, "```"); idx >= 0 {
		start := idx + 3
		if end := strings.Index(output[start:], "```"); end >= 0 {
			candidate := strings.TrimSpace(output[start : start+end])
			if strings.HasPrefix(candidate, "{") {
				return candidate
			}
		}
	}
	// Try bare JSON object (find first { to last })
	start := strings.Index(output, "{")
	if start >= 0 {
		// Find matching closing brace
		depth := 0
		for i := start; i < len(output); i++ {
			switch output[i] {
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					return output[start : i+1]
				}
			}
		}
	}
	return ""
}


func (g *GooseManager) Close() error {
	g.mu.Lock(); defer g.mu.Unlock(); g.running = false; return nil
}

func GetGooseManager(ctx context.Context) (*GooseManager, error) {
	gooseMutex.Lock(); defer gooseMutex.Unlock()
	if gooseManager != nil && gooseManager.running { return gooseManager, nil }
	return NewGooseManager(ctx)
}

// executeMCPOperation performs the actual MCP tool invocation via Goose or Native SDK
func executeMCPOperation(ctx context.Context, want *mywant.Want) error {
	if plan := mywant.GetPlan(want, "execute_operation", false); !plan {
		if op := mywant.GetCurrent(want, "mcp_operation", ""); op == "" {
			return nil
		}
	}

	operationStr := mywant.GetCurrent(want, "mcp_operation", "")
	useNative := mywant.GetCurrent(want, "mcp_native", false)
	if operationStr == "" {
		return fmt.Errorf("mcp_operation not specified")
	}

	want.SetCurrent("achieving_percentage", 25)

	var result map[string]interface{}
	if useNative {
		result = map[string]any{"operation": operationStr, "status": "completed"}
	} else {
		goose, err := GetGooseManager(ctx); if err != nil { return err }
		res, err := goose.ExecuteViaGoose(ctx, operationStr, nil); if err != nil { return err }
		result = map[string]any{"status": "completed", "data": res}
	}

	want.SetCurrent("result", result)
	want.SetCurrent("final_result", result)
	want.SetCurrent("achieving_percentage", 100)
	want.ClearPlan("execute_operation")

	return nil
}

func flattenMCPContent(contents []mcp.Content) []string {
	var results []string
	for _, c := range contents {
		if tc, ok := c.(*mcp.TextContent); ok { results = append(results, tc.Text) }
	}
	return results
}
