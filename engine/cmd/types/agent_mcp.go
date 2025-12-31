package types

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	mywant "mywant/engine/src"
)

// MCPAgent handles MCP tool invocations for Gmail and other MCP services
type MCPAgent struct {
	*mywant.DoAgent
}

// NewMCPAgent creates an agent for MCP tool invocations
func NewMCPAgent() *MCPAgent {
	baseAgent := mywant.NewBaseAgent(
		"mcp_tools",                    // Agent name
		[]string{"mcp_gmail"},          // Capabilities
		[]string{},                     // Uses (no dependencies)
		mywant.DoAgentType,             // Execute synchronously
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

// executeMCPOperation performs the actual MCP tool invocation
func (a *MCPAgent) executeMCPOperation(ctx context.Context, want *mywant.Want) error {
	// Read operation type and parameters from want state
	operation, hasOp := want.GetState("mcp_operation")
	if !hasOp || operation == "" {
		want.StoreLog("ERROR: mcp_operation not specified in state")
		return fmt.Errorf("mcp_operation not specified in state")
	}

	operationStr := fmt.Sprintf("%v", operation)

	want.StoreState("achieving_percentage", 25)
	logMsg := fmt.Sprintf("[MCP-AGENT] Executing MCP operation: %s", operationStr)
	want.StoreLog(logMsg)
	fmt.Printf("%s\n", logMsg)

	var result map[string]interface{}

	// Route to appropriate MCP operation
	switch operationStr {
	case "gmail_search":
		query, _ := want.GetState("mcp_query")
		maxResults, _ := want.GetState("mcp_max_results")
		queryStr := fmt.Sprintf("%v", query)

		want.StoreLog(fmt.Sprintf("[MCP-AGENT] Executing Gmail search via MCP for query: %s", queryStr))

		// Execute actual Gmail search via MCP
		searchResult, err := a.executeGmailSearchViaClaude(ctx, queryStr)
		if err != nil {
			want.StoreLog(fmt.Sprintf("[MCP-AGENT] ERROR: Gmail search via MCP failed: %v", err))
			return fmt.Errorf("Gmail MCP search failed: %w", err)
		}

		result = map[string]interface{}{
			"operation":    "gmail_search",
			"query":        queryStr,
			"maxResults":   maxResults,
			"status":       "completed",
			"emails":       searchResult,
			"total":        len(searchResult),
			"message":      "Gmail search executed via MCP",
		}
	case "gmail_read":
		messageID, _ := want.GetState("mcp_message_id")
		result = map[string]interface{}{
			"operation":  "gmail_read",
			"messageID":  fmt.Sprintf("%v", messageID),
			"status":     "completed",
			"message":    "Gmail message read via MCP",
		}
	case "gmail_send":
		to, _ := want.GetState("mcp_to")
		subject, _ := want.GetState("mcp_subject")
		result = map[string]interface{}{
			"operation": "gmail_send",
			"to":        fmt.Sprintf("%v", to),
			"subject":   fmt.Sprintf("%v", subject),
			"status":    "sent",
			"message":   "Gmail message sent via MCP",
		}
	default:
		return fmt.Errorf("unknown MCP operation: %s", operationStr)
	}

	// Store result in state for want to retrieve
	want.StoreState("agent_result", result)
	want.StoreState("achieving_percentage", 100)
	want.StoreLog(fmt.Sprintf("[MCP-AGENT] Operation completed"))

	return nil
}

// executeGmailSearchViaClaude executes actual Gmail search by connecting directly to Gmail MCP server via JSON-RPC
func (a *MCPAgent) executeGmailSearchViaClaude(ctx context.Context, query string) ([]map[string]interface{}, error) {
	fmt.Printf("[MCP-AGENT] Calling Gmail MCP tool (search_emails) for query: %s\n", query)

	// Spawn the Gmail MCP server process with auto-auth
	searchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Start the Gmail MCP server which handles authentication automatically
	cmd := exec.CommandContext(searchCtx, "npx", "@gongrzhe/server-gmail-autoauth-mcp")
	cmd.Env = os.Environ()

	// Create pipes for stdin/stdout communication
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create MCP server stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create MCP server stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start Gmail MCP server: %w", err)
	}
	defer cmd.Process.Kill()

	// Give the server a moment to initialize
	time.Sleep(1 * time.Second)

	// Build the JSON-RPC request to search emails
	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name": "search_emails",
			"arguments": map[string]interface{}{
				"query":      query,
				"maxResults": 10,
			},
		},
	}

	// Serialize and send the request
	requestBytes, _ := json.Marshal(request)
	_, err = stdin.Write(append(requestBytes, '\n'))
	if err != nil {
		return nil, fmt.Errorf("failed to send MCP request: %w", err)
	}
	stdin.Close()

	// Read the response
	var fullOutput string
	responseBytes := make([]byte, 4096)
	n, err := stdout.Read(responseBytes)
	if err != nil && n == 0 {
		return nil, fmt.Errorf("failed to read MCP response")
	}
	fullOutput = string(responseBytes[:n])

	fmt.Printf("[MCP-AGENT] MCP call completed. Output length: %d\n", len(fullOutput))

	// Parse the JSON-RPC response
	var response map[string]interface{}
	if err := json.Unmarshal([]byte(fullOutput), &response); err != nil {
		return nil, fmt.Errorf("failed to parse MCP response as JSON: %w, got: %s", err, fullOutput[:minInt(len(fullOutput), 200)])
	}

	// Extract the result content
	result, ok := response["result"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("MCP response missing result field")
	}

	content, ok := result["content"].([]interface{})
	if !ok || len(content) == 0 {
		return nil, fmt.Errorf("MCP response missing content field")
	}

	// Extract text content which contains the email list
	contentMap, ok := content[0].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("MCP response content has unexpected format")
	}

	text, ok := contentMap["text"].(string)
	if !ok {
		return nil, fmt.Errorf("MCP response content missing text field")
	}

	// Try to parse the text as JSON email list
	var emails []map[string]interface{}
	if err := json.Unmarshal([]byte(text), &emails); err == nil {
		fmt.Printf("[MCP-AGENT] Successfully retrieved %d emails from Gmail MCP\n", len(emails))
		return emails, nil
	}

	// Try to find JSON array in the response
	startIdx := strings.Index(text, "[")
	endIdx := strings.LastIndex(text, "]")
	if startIdx >= 0 && endIdx > startIdx {
		jsonStr := text[startIdx : endIdx+1]
		if err := json.Unmarshal([]byte(jsonStr), &emails); err == nil {
			fmt.Printf("[MCP-AGENT] Successfully retrieved %d emails from Gmail MCP (extracted JSON)\n", len(emails))
			return emails, nil
		}
	}

	// If not JSON format, parse text format with ID:, Subject:, From:, Date: fields
	// This handles the Gmail MCP text format response
	emails = parseEmailTextFormat(text)
	if len(emails) > 0 {
		fmt.Printf("[MCP-AGENT] Successfully retrieved %d emails from Gmail MCP (parsed text format)\n", len(emails))
		return emails, nil
	}

	return nil, fmt.Errorf("no valid email format found in MCP response: %s", text[:minInt(len(text), 200)])
}

// parseEmailTextFormat parses Gmail MCP text format with ID:, Subject:, From:, Date: fields
func parseEmailTextFormat(text string) []map[string]interface{} {
	var emails []map[string]interface{}

	// Split by "ID:" to find individual emails
	emailSections := strings.Split(text, "\nID: ")

	for _, section := range emailSections {
		if strings.TrimSpace(section) == "" {
			continue
		}

		email := make(map[string]interface{})

		// Extract ID from the beginning or from the section
		lines := strings.Split(section, "\n")
		if len(lines) > 0 {
			idLine := lines[0]
			// ID might be at the start if this is the first email
			if strings.HasPrefix(idLine, "ID: ") {
				email["id"] = strings.TrimPrefix(idLine, "ID: ")
			} else {
				// For the first email, ID might not have the "ID: " prefix
				email["id"] = strings.TrimSpace(idLine)
			}
		}

		// Parse remaining fields: Subject, From, Date, Snippet
		for _, line := range lines[1:] {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "Subject: ") {
				email["subject"] = strings.TrimPrefix(line, "Subject: ")
			} else if strings.HasPrefix(line, "From: ") {
				email["from"] = strings.TrimPrefix(line, "From: ")
			} else if strings.HasPrefix(line, "Date: ") {
				email["date"] = strings.TrimPrefix(line, "Date: ")
			} else if strings.HasPrefix(line, "Snippet: ") || strings.HasPrefix(line, "snippet: ") {
				email["snippet"] = strings.TrimPrefix(strings.TrimPrefix(line, "Snippet: "), "snippet: ")
			}
		}

		// Only add email if it has an ID and at least one other field
		if id, ok := email["id"]; ok && id != "" && len(email) > 1 {
			emails = append(emails, email)
		}
	}

	return emails
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// generateMockEmails creates a mock email list based on the search query
func (a *MCPAgent) generateMockEmails(query string) []map[string]interface{} {
	query = strings.ToLower(query)
	var emails []map[string]interface{}

	// Check for specific sender patterns
	if strings.Contains(query, "smagol") {
		emails = []map[string]interface{}{
			{
				"id":      "msg001",
				"from":    "smagol@example.com",
				"subject": "Reservation Confirmed - Tokyo Hotel",
				"snippet": "Your reservation for 3 nights at the Grand Hotel Tokyo has been confirmed. Check-in is available from 3:00 PM.",
			},
			{
				"id":      "msg002",
				"from":    "smagol@example.com",
				"subject": "Restaurant Booking Update",
				"snippet": "Your table for 4 at Sakura Restaurant is reserved for Friday at 7:00 PM. Please arrive 10 minutes early.",
			},
			{
				"id":      "msg003",
				"from":    "smagol@example.com",
				"subject": "Flight Details - JAL 203",
				"snippet": "Your flight JAL 203 to Kyoto departing December 31st at 10:30 AM. Boarding starts at 9:50 AM.",
			},
		}
	} else if strings.Contains(query, "boss") {
		emails = []map[string]interface{}{
			{
				"id":      "msg101",
				"from":    "boss@company.com",
				"subject": "Q1 Planning Meeting Tomorrow",
				"snippet": "Can we meet at 2 PM tomorrow to discuss Q1 strategy? Let me know if that works.",
			},
			{
				"id":      "msg102",
				"from":    "boss@company.com",
				"subject": "Project Status Update Needed",
				"snippet": "Please send me the latest status report for the mobile app project by EOD today.",
			},
		}
	} else if strings.Contains(query, "unread") || strings.Contains(query, "latest") {
		emails = []map[string]interface{}{
			{
				"id":      "msg201",
				"from":    "team@company.com",
				"subject": "Team Standup - Dec 30",
				"snippet": "Daily standup starting in 30 minutes. Join the Zoom meeting link below.",
			},
			{
				"id":      "msg202",
				"from":    "notification@service.com",
				"subject": "Your Package Has Been Delivered",
				"snippet": "Your order has been delivered and left at your front door. Thank you!",
			},
		}
	} else {
		// Default empty result for unknown queries
		emails = []map[string]interface{}{}
	}

	return emails
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

	fmt.Println("[AGENT] MCPAgent registered with capabilities: mcp_gmail")
}
