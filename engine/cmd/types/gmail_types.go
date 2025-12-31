package types

import (
	"fmt"
	"os/exec"
	"strings"

	. "mywant/engine/src"
)

// GmailLocals holds type-specific local state for GmailWant
type GmailLocals struct {
	Prompt string // User's natural language prompt
}

// GmailWant represents a want that executes Gmail operations via Claude
type GmailWant struct {
	Want
}

// NewGmailWant creates a new GmailWant
func NewGmailWant(want *Want) *GmailWant {
	return &GmailWant{Want: *want}
}

// Initialize prepares the Gmail want for execution
func (g *GmailWant) Initialize() {
	InfoLog("[GMAIL] Initializing Gmail want: %s\n", g.Metadata.Name)

	// Initialize locals
	locals := &GmailLocals{}

	// Parse and validate parameters
	// prompt (required)
	promptParam, ok := g.Spec.Params["prompt"]
	if !ok || promptParam == "" {
		errorMsg := "Missing required parameter 'prompt'"
		g.StoreLog(fmt.Sprintf("ERROR: %s", errorMsg))
		g.StoreState("gmail_status", "failed")
		g.StoreState("error", errorMsg)
		g.Status = "failed"
		g.Locals = locals
		return
	}

	prompt := fmt.Sprintf("%v", promptParam)
	prompt = strings.TrimSpace(prompt)

	if prompt == "" {
		errorMsg := "Parameter 'prompt' cannot be empty"
		g.StoreLog(fmt.Sprintf("ERROR: %s", errorMsg))
		g.StoreState("gmail_status", "failed")
		g.StoreState("error", errorMsg)
		g.Status = "failed"
		g.Locals = locals
		return
	}

	locals.Prompt = prompt

	// Check if claude command is available
	_, err := exec.LookPath("claude")
	if err != nil {
		errorMsg := "claude command not found. Please install Claude Code CLI."
		g.StoreLog(fmt.Sprintf("ERROR: %s", errorMsg))
		g.StoreState("gmail_status", "failed")
		g.StoreState("error", errorMsg)
		g.Status = "failed"
		g.Locals = locals
		return
	}

	g.Locals = locals
	g.StoreState("gmail_status", "initialized")
	InfoLog("[GMAIL] Gmail want initialized with prompt: %s\n", prompt)
}

// IsAchieved returns true when the Gmail operation is complete or failed
func (g *GmailWant) IsAchieved() bool {
	status, exists := g.GetState("gmail_status")
	if !exists {
		return false
	}

	statusStr := fmt.Sprintf("%v", status)
	return statusStr == "completed" || statusStr == "failed"
}

// CalculateAchievingPercentage returns the progress percentage
func (g *GmailWant) CalculateAchievingPercentage() float64 {
	status, exists := g.GetState("gmail_status")
	if !exists {
		return 0
	}

	statusStr := fmt.Sprintf("%v", status)
	switch statusStr {
	case "pending", "initialized":
		return 10
	case "executing":
		return 50
	case "completed":
		return 100
	case "failed":
		return 100
	default:
		return 0
	}
}

// Progress executes the Gmail operation via MCP Agent
func (g *GmailWant) Progress() {
	// Check if already achieved
	if g.IsAchieved() {
		return
	}

	locals := g.getLocals()
	if locals == nil || locals.Prompt == "" {
		g.StoreState("gmail_status", "failed")
		g.StoreState("error", "No prompt available")
		return
	}

	// Mark as executing
	g.StoreState("gmail_status", "executing")

	// Store the prompt as an MCP operation for the agent
	g.StoreLog(fmt.Sprintf("Searching emails with query: %s", locals.Prompt))
	g.StoreState("mcp_operation", "gmail_search")
	g.StoreState("mcp_query", locals.Prompt)
	g.StoreState("mcp_max_results", 10)

	// Set agent requirement for MCP operations
	// This tells ExecuteAgents() to find agents that provide "mcp_gmail" capability
	g.Spec.Requires = []string{"mcp_gmail"}

	// Execute MCP Agent via agent framework
	if err := g.ExecuteAgents(); err != nil {
		g.StoreLog(fmt.Sprintf("ERROR: Failed to execute MCP agent: %v", err))
		g.StoreState("gmail_status", "failed")
		g.StoreState("error", fmt.Sprintf("MCP agent failed: %v", err))
		return
	}

	// Retrieve result from agent
	agentResult, exists := g.GetState("agent_result")
	if !exists {
		g.StoreLog("Warning: MCP agent did not return a result")
		g.StoreState("gmail_status", "failed")
		g.StoreState("error", "MCP agent returned no result")
		return
	}

	// Convert agent result to map
	resultMap, ok := agentResult.(map[string]interface{})
	if !ok {
		g.StoreLog(fmt.Sprintf("ERROR: Agent result is not a map: %T", agentResult))
		g.StoreState("gmail_status", "failed")
		g.StoreState("error", "Invalid agent result format")
		return
	}

	// Store final result with email list
	g.StoreState("final_result", resultMap)
	g.StoreState("gmail_status", "completed")
	g.StoreLog("Gmail search completed successfully")

	// Provide result to output channels
	g.Provide(resultMap)

	InfoLog("[GMAIL] Gmail search completed for want: %s\n", g.Metadata.Name)
}

// getLocals safely retrieves the locals struct
func (g *GmailWant) getLocals() *GmailLocals {
	if g.Locals == nil {
		return nil
	}

	locals, ok := g.Locals.(*GmailLocals)
	if !ok {
		return nil
	}

	return locals
}

// RegisterGmailWantType registers the Gmail want type with the builder
func RegisterGmailWantType(builder *ChainBuilder) {
	builder.RegisterWantType("gmail", func(metadata Metadata, spec WantSpec) Progressable {
		want := &Want{
			Metadata: metadata,
			Spec:     spec,
		}
		return NewGmailWant(want)
	})
}
