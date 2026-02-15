package types

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	. "mywant/engine/core"
)

func init() {
	RegisterWantImplementation[GmailWant, GmailLocals]("gmail")
}

// GmailLocals holds type-specific local state for GmailWant
type GmailLocals struct {
	Prompt             string // User's natural language prompt
	GmailTokenPath     string
	GoogleClientID     string
	GoogleClientSecret string
}

// GmailWant represents a want that executes Gmail operations via Claude
type GmailWant struct {
	Want
}

func (g *GmailWant) GetLocals() *GmailLocals {
	return GetLocals[GmailLocals](&g.Want)
}

// Initialize prepares the Gmail want for execution
func (g *GmailWant) Initialize() {
	g.StoreLog("[GMAIL] Initializing Gmail want: %s", g.Metadata.Name)

	// Get or initialize locals
	locals := g.GetLocals()
	if locals == nil {
		locals = &GmailLocals{}
		g.Locals = locals
	}

	// Parse and validate required parameters using ConfigError pattern
	promptParam := g.GetStringParam("prompt", "")
	if promptParam == "" {
		g.SetConfigError("prompt", "Missing required parameter 'prompt'")
		g.Locals = locals
		return
	}
	locals.Prompt = promptParam

	// Optional auth parameters
	locals.GmailTokenPath = g.GetStringParam("gmail_token_path", "")
	locals.GoogleClientID = g.GetStringParam("google_client_id", "")
	locals.GoogleClientSecret = g.GetStringParam("google_client_secret", "")

	// --- Automatic discovery from ~/.gmail-mcp ---
	home, _ := os.UserHomeDir()
	if home != "" {
		mcpDir := filepath.Join(home, ".gmail-mcp")

		// 1. Discover Token Path
		if locals.GmailTokenPath == "" {
			defaultTokenPath := filepath.Join(mcpDir, "credentials.json")
			if _, err := os.Stat(defaultTokenPath); err == nil {
				locals.GmailTokenPath = defaultTokenPath
			}
		}

		// 2. Discover Client ID and Secret
		if locals.GoogleClientID == "" || locals.GoogleClientSecret == "" {
			keysPath := filepath.Join(mcpDir, "gcp-oauth.keys.json")
			if data, err := os.ReadFile(keysPath); err == nil {
				var config struct {
					Web struct {
						ClientID     string `json:"client_id"`
						ClientSecret string `json:"client_secret"`
					} `json:"web"`
				}
				if err := json.Unmarshal(data, &config); err == nil {
					if locals.GoogleClientID == "" {
						locals.GoogleClientID = config.Web.ClientID
					}
					if locals.GoogleClientSecret == "" {
						locals.GoogleClientSecret = config.Web.ClientSecret
					}
				}
			}
		}
	}

	// Check if claude command is available
	_, err := exec.LookPath("claude")
	if err != nil {
		g.StoreLog("Warning: claude command not found. Claude Code CLI features may be limited.")
	}

	g.Locals = locals
	g.StoreState("gmail_status", "initialized")
}

// IsAchieved returns true when the Gmail operation is complete or failed
func (g *GmailWant) IsAchieved() bool {
	status, _ := g.GetStateString("gmail_status", "pending")
	return status == "completed" || status == "failed"
}

// CalculateAchievingPercentage returns the progress percentage
func (g *GmailWant) CalculateAchievingPercentage() float64 {
	status, _ := g.GetStateString("gmail_status", "pending")
	switch status {
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
	locals := CheckLocalsInitialized[GmailLocals](&g.Want)

	// Check if already achieved
	if g.IsAchieved() {
		return
	}

	if locals.Prompt == "" {
		g.StoreState("gmail_status", "failed")
		g.StoreState("error", "No prompt available")
		return
	}

	status, _ := g.GetStateString("gmail_status", "pending")

	// State: pending or initialized -> Start execution
	if status == "pending" || status == "initialized" {
		g.StoreState("gmail_status", "executing")

		// Set up environment variables from parameters
		env := make(map[string]string)
		if locals.GmailTokenPath != "" {
			env["GMAIL_TOKEN_PATH"] = locals.GmailTokenPath
		}
		if locals.GoogleClientID != "" {
			env["GOOGLE_CLIENT_ID"] = locals.GoogleClientID
		}
		if locals.GoogleClientSecret != "" {
			env["GOOGLE_CLIENT_SECRET"] = locals.GoogleClientSecret
		}
		g.StoreState("mcp_env", env)
		g.StoreState("mcp_server_name", "gmail")
		g.StoreState("mcp_command", "npx")
		g.StoreState("mcp_args", []string{"-y", "@gongrzhe/server-gmail-autoauth-mcp"})
		g.StoreState("mcp_native", true)

		// Store the prompt as an MCP operation for the agent
		g.StoreState("mcp_operation", "gmail_search")
		g.StoreState("mcp_query", locals.Prompt)
		g.StoreState("mcp_max_results", 10)

		return // Yield and let agents run in their own turn
	}

	// State: executing -> Check if agents have finished
	if status == "executing" {
		// Execute Agents via agent framework (this triggers them if they haven't run)
		if err := g.ExecuteAgents(); err != nil {
			g.StoreLog("ERROR: Failed to execute MCP agent: %v", err)
			g.StoreState("gmail_status", "failed")
			g.StoreState("error", fmt.Sprintf("MCP agent failed: %v", err))
			return
		}

		// Check if result has arrived from MCPAgent
		agentResult, exists := g.GetState("agent_result")
		if !exists {
			// Result not ready yet, wait for next cycle
			return
		}

		// Convert agent result to map
		resultMap, ok := agentResult.(map[string]interface{})
		if !ok {
			g.StoreLog("ERROR: Agent result is not a map: %T", agentResult)
			g.StoreState("gmail_status", "failed")
			g.StoreState("error", "Invalid agent result format")
			return
		}

		// Flatten and parse emails from MCP content
		emails := []map[string]string{}
		if content := resultMap["content"]; content != nil {
			// Handle both []string and []interface{} for robustness
			if strSlice, ok := content.([]string); ok {
				for _, text := range strSlice {
					emails = append(emails, parseEmails(text)...)
				}
			} else if interSlice, ok := content.([]interface{}); ok {
				for _, c := range interSlice {
					if text, ok := c.(string); ok {
						emails = append(emails, parseEmails(text)...)
					}
				}
			}
		}

		g.StoreState("gmail_result", emails)
		g.StoreState("gmail_status", "completed")
		g.StoreLog("📦 Gmail search completed for '%s': found %d emails", locals.Prompt, len(emails))

		// Provide result to output channels
		g.Provide(emails)
		g.ProvideDone()
	}
}

// parseEmails parses the human-readable text from Gmail MCP into a slice of structured email maps
func parseEmails(text string) []map[string]string {
	var emails []map[string]string

	// Normalize line endings and split by "ID: " which marks the start of each email
	text = strings.ReplaceAll(text, "\r\n", "\n")
	parts := strings.Split(text, "ID: ")

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		email := make(map[string]string)
		// The first line of the part (without "ID: ") is the ID itself
		lines := strings.Split(part, "\n")
		email["id"] = strings.TrimSpace(lines[0])

		for _, line := range lines[1:] {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			kv := strings.SplitN(line, ": ", 2)
			if len(kv) == 2 {
				key := strings.ToLower(strings.TrimSpace(kv[0]))
				value := strings.TrimSpace(kv[1])
				email[key] = value
			}
		}

		if len(email) > 1 { // Should have more than just ID
			emails = append(emails, email)
		}
	}
	return emails
}
