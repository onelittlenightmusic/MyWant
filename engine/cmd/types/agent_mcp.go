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
	"time"

	mywant "mywant/engine/src"
)

// ============ GooseManager ============

// GooseManager creates and executes Goose processes for MCP tool execution via LLM
// Uses per-request `goose run` processes rather than persistent sessions
type GooseManager struct {
	mu sync.Mutex
	// Currently unused - kept for API compatibility
	running bool
}

var (
	gooseManager *GooseManager
	gooseMutex   sync.Mutex
)

// NewGooseManager creates a GooseManager instance (mostly a no-op for per-request model)
func NewGooseManager(ctx context.Context) (*GooseManager, error) {
	fmt.Fprintf(os.Stderr, "[GOOSE-MANAGER] Initializing Goose manager (per-request mode with Gmail MCP)...\n")

	manager := &GooseManager{
		running: true,
	}

	fmt.Fprintf(os.Stderr, "[GOOSE-MANAGER] Goose manager ready for per-request execution\n")
	fmt.Fprintf(os.Stderr, "[GOOSE-MANAGER] Gmail MCP extension will be loaded from ~/.config/goose/config.yaml\n")
	return manager, nil
}

// ExecuteViaGoose executes an MCP operation via Goose with LLM decision-making
// Creates a new `goose run` process for each request
func (g *GooseManager) ExecuteViaGoose(ctx context.Context, operation string, params map[string]interface{}) (interface{}, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if !g.running {
		return nil, fmt.Errorf("Goose manager is not running")
	}

	// Build natural language prompt for Goose
	prompt := g.buildPrompt(operation, params)

	fmt.Fprintf(os.Stderr, "[GOOSE-MANAGER] Starting per-request Goose process for: %s\n", operation)

	// Determine preferred provider
	preferredProvider := ""
	if p, ok := params["provider"].(string); ok && p != "" {
		preferredProvider = p
	}
	if preferredProvider == "" {
		preferredProvider = os.Getenv("MYWANT_GOOSE_PROVIDER")
	}

	args := []string{"run", "-i", "-"}
	if preferredProvider != "" {
		args = append(args, "--provider", preferredProvider)
		fmt.Fprintf(os.Stderr, "[GOOSE-MANAGER] Using preferred provider: %s\n", preferredProvider)
	}

	// Try the first attempt
	startTime := time.Now()
	var fullOutput string
	var err error
	fullOutput, err = g.runGooseWithArgs(args, prompt)
	duration := time.Since(startTime)
	fmt.Fprintf(os.Stderr, "[GOOSE-MANAGER] Primary attempt took %v\n", duration)

	// Check if attempt failed
	failed := err != nil || strings.Contains(fullOutput, "Ran into this error") || strings.Contains(fullOutput, "Request failed")

	if failed {
		// Determine fallback provider
		fallbackProvider := "gemini-cli"
		if preferredProvider == "gemini-cli" {
			fallbackProvider = "claude-code"
		}

		fmt.Fprintf(os.Stderr, "[GOOSE-MANAGER] Primary attempt failed (provider: %s, err: %v), trying fallback to %s...\n", 
			preferredProvider, err, fallbackProvider)
		
		fallbackStartTime := time.Now()
		fallbackArgs := []string{"run", "-i", "-", "--provider", fallbackProvider}
		fallbackOutput, fallbackErr := g.runGooseWithArgs(fallbackArgs, prompt)
		fallbackDuration := time.Since(fallbackStartTime)
		fmt.Fprintf(os.Stderr, "[GOOSE-MANAGER] Fallback attempt took %v\n", fallbackDuration)
		
		fallbackFailed := fallbackErr != nil || strings.Contains(fallbackOutput, "Ran into this error") || strings.Contains(fallbackOutput, "Request failed")
		
		if !fallbackFailed {
			fullOutput = fallbackOutput
			err = nil
			fmt.Fprintf(os.Stderr, "[GOOSE-MANAGER] Fallback to %s succeeded!\n", fallbackProvider)
		} else {
			fmt.Fprintf(os.Stderr, "[GOOSE-MANAGER] Fallback to %s also failed: %v\n", fallbackProvider, fallbackErr)
			// If both failed, return the error from the primary attempt if it exists, or a general error
			if err == nil {
				err = fmt.Errorf("both primary and fallback providers failed to generate a response")
			}
		}
	}

	if err != nil {
		return nil, err
	}

	// Parse the response
	result, err := parseGooseResponse(fullOutput)
	if err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return result, nil
}

// runGooseWithArgs is a helper to run goose with specific arguments and input
func (g *GooseManager) runGooseWithArgs(args []string, prompt string) (string, error) {
	cmd := exec.CommandContext(context.Background(), "goose", args...)
	cmd.Env = os.Environ()

	// Set up pipes
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return "", fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	cmd.Stderr = os.Stderr

	// Start the process
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("failed to start Goose process: %w", err)
	}

	// Send the prompt to stdin
	if _, err := io.WriteString(stdin, prompt+"\n"); err != nil {
		cmd.Process.Kill()
		cmd.Wait()
		return "", fmt.Errorf("failed to send prompt to Goose: %w", err)
	}

	// Close stdin to signal EOF
	stdin.Close()

	// Read the response
	scanner := bufio.NewScanner(stdout)
	var outputLines []string
	for scanner.Scan() {
		line := scanner.Text()
		outputLines = append(outputLines, line)
	}

	if err := scanner.Err(); err != nil {
		cmd.Process.Kill()
		cmd.Wait()
		return "", fmt.Errorf("error reading Goose output: %w", err)
	}

	// Wait for the process to complete
	cmd.Wait()

	return strings.Join(outputLines, "\n"), nil
}

// buildPrompt constructs natural language instructions for Goose
func (g *GooseManager) buildPrompt(operation string, params map[string]interface{}) string {
	switch operation {
	case "gmail_search":
		query := params["query"].(string)
		maxResults := 10
		if mr, ok := params["maxResults"].(int); ok {
			maxResults = mr
		}
		return fmt.Sprintf(`You have access to the Gmail MCP server. Please use the search_emails tool to search for emails.

Search parameters:
- Query: "%s"
- Maximum results: %d

Steps:
1. Use the search_emails tool with the query and maxResults
2. Extract the email results
3. Format each email as a JSON object with these fields: id, from, subject, date, snippet
4. Return ONLY a JSON array of email objects, nothing else

Example output format:
[{"id":"msg123","from":"user@example.com","subject":"Example","date":"2025-01-01","snippet":"..."}]`, query, maxResults)

	case "gmail_read":
		messageID := params["messageID"].(string)
		return fmt.Sprintf(`You have access to the Gmail MCP server. Please use the read_email tool to read an email.

Message ID: "%s"

Steps:
1. Use the read_email tool with the message ID
2. Extract the email content
3. Return ONLY a JSON object with the email details, nothing else`, messageID)

	case "gmail_send":
		to := params["to"].(string)
		subject := params["subject"].(string)
		body := ""
		if b, ok := params["body"].(string); ok {
			body = b
		}
		return fmt.Sprintf(`You have access to the Gmail MCP server. Please use the send_email tool to send an email.

Email details:
- To: %s
- Subject: %s
- Body: %s

Steps:
1. Use the send_email tool with the email details
2. Extract the message ID from the response
3. Return ONLY a JSON object with: {"status":"sent","messageId":"..."}`, to, subject, body)

	case "google_search":
		query := params["query"].(string)
		return fmt.Sprintf(`Please use the available search tools (like Google Search MCP) to find the latest information on the following topic.

Query: "%s"

Steps:
1. Search for the query
2. Extract key findings, dates, and source URLs
3. Synthesize the findings into a list of "facts"
4. Return ONLY a JSON object with a "facts" array, where each fact has "text", "date", and "url"

Example output format:
{"facts": [{"text": "Fact description", "date": "2025-01-01", "url": "https://..."}]}
`, query)

	case "knowledge_synthesize":
		topic := params["topic"].(string)
		existingContent := params["existing_content"].(string)
		newFacts := params["new_facts"].(string)
		depth := params["depth"].(string)

		return fmt.Sprintf(`You are a knowledge synthesizer. Your task is to update a Markdown document with new information.

TOPIC: %s
DEPTH: %s

EXISTING CONTENT:
"""
%s
"""

NEW FACTS TO INTEGRATE:
"""
%s
"""

INSTRUCTIONS:
1. Review the existing content and new facts.
2. If the new facts are already covered, maintain the existing content but perhaps improve clarity.
3. If there are new facts, integrate them into the appropriate sections or create new sections.
4. Use professional Markdown formatting (headers, lists, etc.).
5. Include a "Sources" section at the end with URLs if provided.
6. The output should be the COMPLETE Markdown document.
7. Return ONLY the Markdown content, nothing else.`, topic, depth, existingContent, newFacts)

	case "interact_recommend":
		message := params["message"].(string)
		conversationHistory := ""
		if hist, ok := params["conversation_history"].(string); ok {
			conversationHistory = hist
		}
		return g.buildRecommendationPrompt(message, conversationHistory)

	default:
		return fmt.Sprintf("Execute MCP operation: %s with parameters: %v", operation, params)
	}
}

// buildRecommendationPrompt constructs a prompt for interactive want recommendation
func (g *GooseManager) buildRecommendationPrompt(message string, conversationHistory string) string {
	return fmt.Sprintf(`You are a MyWant system architect. Based on the user's request, generate 2-3 alternative solutions.

CONVERSATION HISTORY:
%s

LATEST USER REQUEST:
"%s"

AVAILABLE WANT TYPES:
- reminder: One-time or recurring reminders with optional user approval
- silencer: Auto-approval agent that approves pending reminders automatically
- gmail: Gmail operations via natural language
- knowledge: Knowledge freshness tracker for monitoring topics
- coordinator: Orchestrates multiple independent wants
- execution_result: Execute shell commands and capture results
- flight, hotel, restaurant, buffet: Travel-related wants
- queue, sink, collector: Queue system components
- numbers, prime_numbers, fibonacci_numbers: Mathematical generators

KEY RECIPES:
- "Silencer Example": Reminder with automatic approval (reminder + silencer pattern)
- "Travel Agent System": Restaurant + Hotel + Buffet (parallel independent wants)
- "Queue System": Generator → Queue → Sink (pipeline pattern)
- "Command Execution Example": Shell command execution with result capture

TASK:
1. Analyze the user's intent from their request
2. Generate 2-3 alternative approaches:
   - Approach 1: Recipe-based solution (if a suitable recipe exists)
   - Approach 2: Custom want composition (from individual want types)
   - Approach 3: Alternative variation (different parameters or structure)

For each recommendation, provide:
- ID: Use "rec-1", "rec-2", "rec-3"
- Title: A descriptive name in Japanese or English
- Approach: "recipe", "custom", or "hybrid"
- Description: Explain why this solution fits the user's needs (in Japanese if user message is Japanese)
- Config: Complete config structure with a "wants" array
- Metadata: want_count, recipes_used (if any), want_types_used, complexity, pros_cons

EXAMPLES:

Reminder request → Generate:
1. Simple one-time reminder (reminder want)
2. Auto-approved reminder (reminder + silencer, "Silencer Example" recipe)
3. Recurring reminder with schedule (reminder with "when" field)

Gmail request → Generate:
1. Simple gmail want with natural language task
2. Gmail + reminder combination for scheduled emails

Travel request → Generate:
1. Travel Agent System recipe (if matches)
2. Custom combination of flight/hotel/restaurant wants

IMPORTANT:
- Generate complete, valid want configurations
- Each want must have metadata (name, type, labels) and spec (params)
- Use realistic parameter values
- "using" field is OPTIONAL and ONLY needed for dependent wants (pipeline patterns)
- If using "using" field, it must be an ARRAY of objects: [{"key": "value"}]
- NEVER include empty "using" field - omit it completely if not needed
- Return ONLY valid JSON, no markdown, no extra text

Return ONLY this JSON structure:

Example 1 - Simple reminder (NO using field):
{
  "recommendations": [
    {
      "id": "rec-1",
      "title": "シンプルな1回限りリマインダ",
      "approach": "custom",
      "description": "最もシンプルなリマインダー構成。指定した時間にメッセージを表示...",
      "config": {
        "wants": [
          {
            "metadata": {
              "name": "my_reminder",
              "type": "reminder",
              "labels": {"role": "reminder"}
            },
            "spec": {
              "params": {
                "message": "重要なタスクの時間です",
                "duration_from_now": "30 minutes",
                "ahead": "5 minutes",
                "require_reaction": true,
                "reaction_type": "internal"
              }
            }
          }
        ]
      },
      "metadata": {
        "want_count": 1,
        "want_types_used": ["reminder"],
        "complexity": "low",
        "pros_cons": {
          "pros": ["最小構成で素早く動作", "設定が簡単"],
          "cons": ["単発のみ（繰り返しなし）", "手動承認が必要"]
        }
      }
    }
  ]
}

Example 2 - Auto-approved reminder with silencer (WITH using field):
{
  "recommendations": [
    {
      "id": "rec-2",
      "title": "自動承認リマインダ",
      "approach": "recipe",
      "description": "Silencerエージェントで自動承認されるリマインダー",
      "config": {
        "wants": [
          {
            "metadata": {"name": "auto_reminder", "type": "reminder"},
            "spec": {
              "params": {
                "message": "自動承認されるリマインダー",
                "duration_from_now": "15 seconds",
                "ahead": "10 seconds",
                "require_reaction": true
              }
            }
          },
          {
            "metadata": {"name": "auto_silencer", "type": "silencer"},
            "spec": {
              "params": {"policy": "all_true"},
              "using": [{"reaction_requests": "auto_reminder"}]
            }
          }
        ]
      },
      "metadata": {
        "want_count": 2,
        "recipes_used": ["Silencer Example"],
        "want_types_used": ["reminder", "silencer"],
        "complexity": "medium",
        "pros_cons": {
          "pros": ["完全自動化", "手動承認不要"],
          "cons": ["2つのwantが必要"]
        }
      }
    }
  ]
}`, conversationHistory, message)
}

// parseGooseResponse extracts and processes Goose JSON output
func parseGooseResponse(output string) (interface{}, error) {
	// Debug: log raw input
	fmt.Fprintf(os.Stderr, "[GOOSE-PARSER] Raw input (%d chars, %d lines)\n", len(output), strings.Count(output, "\n")+1)
	fmt.Fprintf(os.Stderr, "[GOOSE-PARSER] First 500 chars: %s\n", output[:minInt(len(output), 500)])
	fmt.Fprintf(os.Stderr, "[GOOSE-PARSER] Last 500 chars: %s\n", output[maxInt(0, len(output)-500):])

	// Remove Goose session information lines
	lines := strings.Split(output, "\n")
	var cleanedLines []string
	for _, line := range lines {
		// Skip session info lines
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "starting session") ||
			strings.HasPrefix(trimmed, "session id:") ||
			strings.HasPrefix(trimmed, "working directory:") {
			continue
		}
		cleanedLines = append(cleanedLines, line)
	}
	output = strings.Join(cleanedLines, "\n")

	// Remove markdown code blocks if present (Goose may wrap JSON in ```json...```)
	output = strings.ReplaceAll(output, "```json", "")
	output = strings.ReplaceAll(output, "```", "")
	output = strings.TrimSpace(output)

	// Look for JSON object or array - check for both and use whichever comes first
	objIdx := strings.Index(output, "{")
	arrIdx := strings.Index(output, "[")

	var startIdx int
	var isArray bool

	if objIdx == -1 && arrIdx == -1 {
		// No JSON found at all, return the whole output as text
		return map[string]interface{}{
			"text": output,
		}, nil
	} else if arrIdx != -1 && (objIdx == -1 || arrIdx < objIdx) {
		// Array comes first (or object not found)
		startIdx = arrIdx
		isArray = true
	} else {
		// Object comes first (or array not found)
		startIdx = objIdx
		isArray = false
	}

	// Find the matching closing bracket
	var endIdx int
	if isArray {
		endIdx = strings.LastIndex(output, "]")
	} else {
		endIdx = strings.LastIndex(output, "}")
	}

	if endIdx == -1 || endIdx <= startIdx {
		return nil, fmt.Errorf("invalid JSON in response (startIdx=%d, endIdx=%d, isArray=%v)", startIdx, endIdx, isArray)
	}

	jsonStr := output[startIdx : endIdx+1]

	// Debug: log the extracted JSON string
	fmt.Fprintf(os.Stderr, "[GOOSE-PARSER] Extracted JSON (%d chars):\n%s\n", len(jsonStr), jsonStr[:minInt(len(jsonStr), 500)])

	// First, try to parse as Goose conversation format (with "messages" key)
	var gooseFormat map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &gooseFormat); err == nil {
		// Check if this is Goose conversation format
		if messages, ok := gooseFormat["messages"].([]interface{}); ok {
			// Extract the last assistant message content
			for i := len(messages) - 1; i >= 0; i-- {
				msg, ok := messages[i].(map[string]interface{})
				if !ok {
					continue
				}
				if role, ok := msg["role"].(string); ok && role == "assistant" {
					if contentArr, ok := msg["content"].([]interface{}); ok && len(contentArr) > 0 {
						if content, ok := contentArr[0].(map[string]interface{}); ok {
							if text, ok := content["text"].(string); ok {
								// Try to extract JSON from the assistant's text response
								result, err := extractJSONFromText(text)
								if err == nil {
									return result, nil
								}
								// If extraction failed, return text as is
								return map[string]interface{}{"text": text}, nil
							}
						}
					}
				}
			}
		}
	}

	// Try to unmarshal as array first (for email search results)
	var arrResult []map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &arrResult); err == nil {
		return arrResult, nil
	}

	// Try as object (for other operations)
	var objResult map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &objResult); err == nil {
		// Check for "emails" field (sometimes Goose wraps array in object)
		if emails, ok := objResult["emails"].([]interface{}); ok {
			var emailList []map[string]interface{}
			for _, email := range emails {
				if emailMap, ok := email.(map[string]interface{}); ok {
					emailList = append(emailList, emailMap)
				}
			}
			if len(emailList) > 0 {
				return emailList, nil
			}
		}

		// If it's not a Goose format message, return it as is
		if _, hasMessages := objResult["messages"]; !hasMessages {
			return objResult, nil
		}
	}

	// If all JSON parsing attempts failed, fall back to returning the whole output as text
	return map[string]interface{}{
		"text": output,
	}, nil
}

// extractJSONFromText tries to extract JSON data from text content
func extractJSONFromText(text string) (interface{}, error) {
	// Look for JSON array in text
	startIdx := strings.Index(text, "[")
	if startIdx == -1 {
		startIdx = strings.Index(text, "{")
	}

	if startIdx == -1 {
		// No JSON found, return the text as is
		return map[string]interface{}{
			"text": text,
		}, nil
	}

	// Find matching closing bracket
	var endIdx int
	if text[startIdx] == '[' {
		endIdx = strings.LastIndex(text, "]")
	} else {
		endIdx = strings.LastIndex(text, "}")
	}

	if endIdx <= startIdx {
		// Invalid JSON, return text
		return map[string]interface{}{
			"text": text,
		}, nil
	}

	jsonStr := text[startIdx : endIdx+1]

	// Try to unmarshal as array
	var arrResult []map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &arrResult); err == nil {
		return arrResult, nil
	}

	// Try as object
	var objResult map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &objResult); err == nil {
		return objResult, nil
	}

	// Return raw text if JSON parsing fails
	return map[string]interface{}{
		"text": text,
	}, nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Close is a no-op for per-request model
// (processes are created and destroyed for each request)
func (g *GooseManager) Close() error {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.running = false
	fmt.Fprintf(os.Stderr, "[GOOSE-MANAGER] GooseManager closed (no persistent process)\n")
	return nil
}

// GetGooseManager returns or creates a GooseManager instance
// In per-request mode, the manager creates new `goose run` processes for each request
// This avoids issues with interactive session management
func GetGooseManager(ctx context.Context) (*GooseManager, error) {
	gooseMutex.Lock()
	defer gooseMutex.Unlock()

	// Check if we already have a running manager
	if gooseManager != nil && gooseManager.running {
		fmt.Fprintf(os.Stderr, "[GOOSE-MANAGER] Reusing existing GooseManager\n")
		return gooseManager, nil
	}

	// Create a new GooseManager instance
	fmt.Fprintf(os.Stderr, "[GOOSE-MANAGER] Creating new GooseManager\n")
	manager, err := NewGooseManager(ctx)
	if err != nil {
		return nil, err
	}

	// Store the manager for reuse
	gooseManager = manager
	return manager, nil
}

// ============ MCPAgent ============

// MCPAgent handles MCP tool invocations for Gmail and other MCP services
type MCPAgent struct {
	*mywant.DoAgent
}

// NewMCPAgent creates an agent for MCP tool invocations
func NewMCPAgent() *MCPAgent {
	baseAgent := mywant.NewBaseAgent(
		"mcp_tools",           // Agent name
		[]string{"mcp_gmail"}, // Capabilities
		mywant.DoAgentType,    // Execute synchronously
	)

	agent := &MCPAgent{
		DoAgent: &mywant.DoAgent{
			BaseAgent: *baseAgent,
			Action:    nil, // Set below
		},
	}

	// Define MCP execution logic
	agent.DoAgent.Action = func(ctx context.Context, want *mywant.Want) error {
		return agent.executeMCPOperation(ctx, want)
	}

	return agent
}

// executeMCPOperation performs the actual MCP tool invocation via Goose
func (a *MCPAgent) executeMCPOperation(ctx context.Context, want *mywant.Want) error {
	// Read operation type and parameters from want state
	operation, hasOp := want.GetState("mcp_operation")
	if !hasOp || operation == "" {
		want.StoreLog("ERROR: mcp_operation not specified in state")
		return fmt.Errorf("mcp_operation not specified in state")
	}

	operationStr := fmt.Sprintf("%v", operation)

	want.StoreState("achieving_percentage", 25)
	logMsg := fmt.Sprintf("[MCP-AGENT] Executing MCP operation via Goose: %s", operationStr)
	want.StoreLog(logMsg)

	// Get the Goose session server (singleton, persistent)
	goose, err := GetGooseManager(ctx)
	if err != nil {
		errMsg := fmt.Sprintf("[MCP-AGENT] ERROR: Goose initialization failed: %v", err)
		want.StoreLog(errMsg)
		return fmt.Errorf("Goose initialization failed: %w", err)
	}

	// Note: We don't close Goose here because it's a singleton session server
	// that should remain running to handle multiple requests

	var result map[string]interface{}

	// Route to appropriate MCP operation via Goose
	switch operationStr {
	case "gmail_search":
		query, _ := want.GetState("mcp_query")
		maxResults, _ := want.GetState("mcp_max_results")
		queryStr := fmt.Sprintf("%v", query)
		maxResultsInt := 10
		if maxResults != nil {
			if mr, ok := maxResults.(float64); ok {
				maxResultsInt = int(mr)
			}
		}

		want.StoreLog(fmt.Sprintf("[MCP-AGENT] Executing Gmail search via Goose for query: %s", queryStr))

		// Execute Gmail search via Goose
		searchResult, err := goose.ExecuteViaGoose(ctx, "gmail_search", map[string]interface{}{
			"query":      queryStr,
			"maxResults": maxResultsInt,
		})
		if err != nil {
			errMsg := fmt.Sprintf("[MCP-AGENT] ERROR: Gmail search via Goose failed: %v", err)
			want.StoreLog(errMsg)
			return fmt.Errorf("Gmail Goose search failed: %w", err)
		}

		// Convert result to email list
		var emails []map[string]interface{}
		if resultData, ok := searchResult.([]map[string]interface{}); ok {
			emails = resultData
		} else if resultData, ok := searchResult.([]interface{}); ok {
			for _, item := range resultData {
				if emailMap, ok := item.(map[string]interface{}); ok {
					emails = append(emails, emailMap)
				}
			}
		}

		result = map[string]interface{}{
			"operation":  "gmail_search",
			"query":      queryStr,
			"maxResults": maxResultsInt,
			"status":     "completed",
			"emails":     emails,
			"total":      len(emails),
			"message":    "Gmail search executed via Goose + MCP",
		}

	case "gmail_read":
		messageID, _ := want.GetState("mcp_message_id")
		messageIDStr := fmt.Sprintf("%v", messageID)

		want.StoreLog(fmt.Sprintf("[MCP-AGENT] Executing Gmail read via Goose for messageID: %s", messageIDStr))

		// Execute via Goose
		readResult, err := goose.ExecuteViaGoose(ctx, "gmail_read", map[string]interface{}{
			"messageID": messageIDStr,
		})
		if err != nil {
			errMsg := fmt.Sprintf("[MCP-AGENT] ERROR: Gmail read via Goose failed: %v", err)
			want.StoreLog(errMsg)
			return fmt.Errorf("Gmail Goose read failed: %w", err)
		}

		result = map[string]interface{}{
			"operation": "gmail_read",
			"messageID": messageIDStr,
			"status":    "completed",
			"data":      readResult,
			"message":   "Gmail message read via Goose + MCP",
		}

	case "gmail_send":
		to, _ := want.GetState("mcp_to")
		subject, _ := want.GetState("mcp_subject")
		body, _ := want.GetState("mcp_body")
		toStr := fmt.Sprintf("%v", to)
		subjectStr := fmt.Sprintf("%v", subject)
		bodyStr := fmt.Sprintf("%v", body)

		want.StoreLog(fmt.Sprintf("[MCP-AGENT] Executing Gmail send via Goose to: %s", toStr))

		// Execute via Goose
		sendResult, err := goose.ExecuteViaGoose(ctx, "gmail_send", map[string]interface{}{
			"to":      toStr,
			"subject": subjectStr,
			"body":    bodyStr,
		})
		if err != nil {
			errMsg := fmt.Sprintf("[MCP-AGENT] ERROR: Gmail send via Goose failed: %v", err)
			want.StoreLog(errMsg)
			return fmt.Errorf("Gmail Goose send failed: %w", err)
		}

		result = map[string]interface{}{
			"operation": "gmail_send",
			"to":        toStr,
			"subject":   subjectStr,
			"status":    "sent",
			"data":      sendResult,
			"message":   "Gmail message sent via Goose + MCP",
		}

	default:
		return fmt.Errorf("unknown MCP operation: %s", operationStr)
	}

	// Store result in state for want to retrieve
	want.StoreState("agent_result", result)
	want.StoreState("achieving_percentage", 100)
	want.StoreLog(fmt.Sprintf("[MCP-AGENT] Operation completed via Goose"))

	return nil
}

// RegisterMCPAgent registers the MCP agent with the agent registry
func RegisterMCPAgent(registry *mywant.AgentRegistry) {
	if registry == nil {
		fmt.Println("Warning: No agent registry found, skipping MCPAgent registration")
		return
	}

	// CRITICAL: Register capability BEFORE registering agent
	// This is required for ExecuteAgents() to find the agent later
	registry.RegisterCapability(mywant.Capability{
		Name:  "mcp_gmail",
		Gives: []string{"mcp_gmail_search", "mcp_gmail_read", "mcp_gmail_send"},
	})

	// Now register the agent that provides these capabilities
	agent := NewMCPAgent()
	registry.RegisterAgent(agent)

	fmt.Fprintf(os.Stderr, "[AGENT] MCPAgent registered with capabilities: mcp_gmail\n")
	fmt.Println("[AGENT] MCPAgent registered with capabilities: mcp_gmail")
}
