package types

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	. "mywant/engine/core"
)

const (
	ccMonitorAgentName        = "claude_code_session_monitor"
	ccWebhookMonitorAgentName = "monitor_cc_webhook"
	ccThinkAgentName          = "claude_code_watcher_think"
	ccDoAgentName             = "claude_code_requester"
)

func init() {
	RegisterMonitorAgentType(ccMonitorAgentName, []Capability{
		{Name: "claude_code_session_monitoring", Gives: []string{"claude_code_session_monitoring"}, Description: "Monitors Claude Code session files for state changes"},
	}, claudeCodeSessionMonitor)

	// Webhook MonitorAgent: reuses PollWebhook with cc-prefixed state keys.
	// Identical to monitor_teams_webhook / monitor_slack_webhook pattern.
	RegisterMonitorAgent(ccWebhookMonitorAgentName, func(ctx context.Context, want *Want) (bool, error) {
		return PollWebhook(ctx, want, ccWebhookConfig)
	})

	RegisterThinkAgentType(ccThinkAgentName, []Capability{
		{Name: "claude_code_watching", Gives: []string{"claude_code_watching"}, Description: "Decides when to trigger requests based on session observations"},
	}, claudeCodeWatcherThink)

	RegisterDoAgentType(ccDoAgentName, []Capability{
		{Name: "claude_code_requesting", Gives: []string{"claude_code_requesting"}, Description: "Sends requests to Claude Code via CLI"},
	}, claudeCodeRequester)
}

// ---------------------------------------------------------------------------
// MonitorAgent: Observe Claude Code session, write facts to Current
// ---------------------------------------------------------------------------

func claudeCodeSessionMonitor(_ context.Context, want *Want) (bool, error) {
	sessionID := GetCurrent(want, "session_id", "")
	if sessionID == "" {
		// Also try goal (set by Initialize)
		sessionID = GetGoal(want, "session_id", "")
	}
	if sessionID == "" || !isValidSessionID(sessionID) {
		// No valid session yet — DoAgent will create one on first trigger. Just wait.
		want.SetCurrent("current_session_state", "waiting_for_first_message")
		return false, nil
	}

	phase := GetCurrent(want, "phase", CCPhaseMonitoring)
	if phase == CCPhaseAchieved {
		return true, nil // stop monitoring
	}

	// Read session entries from Claude Code session directory
	entries, err := readClaudeSessionEntries(sessionID)
	if err != nil {
		// Session file not found yet (e.g. just created) — not a fatal error, retry next poll.
		want.SetCurrent("session_read_error", err.Error())
		want.SetCurrent("current_session_state", "waiting_for_session")
		return false, nil
	}
	want.SetCurrent("session_read_error", "")

	// Classify session state
	sessionState := classifyClaudeSessionState(entries)
	want.SetCurrent("current_session_state", sessionState)

	// Extract and store latest output
	if len(entries) > 0 {
		latest := entries[len(entries)-1]
		want.SetCurrent("latest_output", latest.Content)
		want.SetCurrent("latest_role", latest.Role)
		want.SetCurrent("latest_timestamp", latest.Timestamp)
	}

	// Pattern matching
	watchPattern := GetGoal(want, "watch_pattern", "")
	if watchPattern != "" {
		matched, content := matchClaudePattern(entries, watchPattern)
		want.SetCurrent("pattern_matched", matched)
		if matched {
			want.SetCurrent("matched_content", content)
		}
	}

	// Detect new assistant response (for awaiting_response phase)
	lastRequestAt := GetCurrent(want, "last_request_at", int64(0))
	if lastRequestAt > 0 {
		hasNew := hasNewAssistantResponse(entries, lastRequestAt)
		want.SetCurrent("has_new_response", hasNew)
		if hasNew {
			resp := getLatestAssistantContent(entries)
			want.SetCurrent("latest_response_content", resp)
		}
	}

	want.SetCurrent("last_poll_at", time.Now().Unix())
	want.SetCurrent("session_entry_count", len(entries))

	return false, nil
}

// ---------------------------------------------------------------------------
// ThinkAgent: Read Current, make decisions, write to Plan
// ---------------------------------------------------------------------------

func claudeCodeWatcherThink(ctx context.Context, want *Want) error {
	phase := GetCurrent(want, "phase", CCPhaseMonitoring)
	sessionID := GetGoal(want, "session_id", "")

	// Check for incoming webhook messages (dynamic prompt override).
	// The webhook MonitorAgent (PollWebhook) + HTTP handler write to cc_latest_message.
	// If a new message arrived, use its text as the auto_request for the next send.
	if latestMsg := GetCurrent(want, "cc_latest_message", map[string]any{}); len(latestMsg) > 0 {
		lastProcessedCount := GetCurrent(want, "cc_webhook_processed", 0)
		msgCount := GetCurrent(want, "cc_message_count", 0)
		if msgCount > lastProcessedCount {
			if text, ok := latestMsg["text"].(string); ok && text != "" {
				want.DirectLog("[CC_THINK] Webhook message received, overriding auto_request")
				want.SetCurrent("webhook_auto_request", text)
				want.SetCurrent("cc_webhook_processed", msgCount)
			}
		}
	}

	switch phase {
	case CCPhaseMonitoring:
		// Webhook messages always trigger immediately, regardless of trigger_on setting.
		// This allows manual requests to coexist with autonomous pattern/waiting triggers.
		webhookTriggered := GetCurrent(want, "webhook_auto_request", "") != ""

		// Check autonomous trigger conditions from MonitorAgent's observations
		triggerOn := GetGoal(want, "trigger_on", "pattern")
		autonomousTriggered := false
		switch triggerOn {
		case "pattern":
			autonomousTriggered = GetCurrent(want, "pattern_matched", false)
		case "waiting":
			autonomousTriggered = GetCurrent(want, "current_session_state", "") == "waiting_for_input"
		case "complete":
			autonomousTriggered = GetCurrent(want, "current_session_state", "") == "task_complete"
		case "idle":
			autonomousTriggered = GetCurrent(want, "current_session_state", "") == "idle"
		case "webhook":
			// webhook-only mode: autonomous trigger is same as webhook trigger
			autonomousTriggered = webhookTriggered
		}

		triggered := webhookTriggered || autonomousTriggered
		triggerLabel := triggerOn
		if webhookTriggered {
			triggerLabel = "webhook"
		}

		if !triggered {
			return nil
		}

		// Check idempotency: is this trigger already handled?
		requestID := deriveClaudeRequestID(want)
		if isClaudeRequestSent(sessionID, requestID) {
			want.DirectLog("[CC_THINK] Request %s already sent, skipping", requestID)
			// Restore request_count from idempotency log
			sentCount := countClaudeSentLogs(sessionID)
			currentCount := GetCurrent(want, "request_count", 0)
			if sentCount > currentCount {
				want.SetCurrent("request_count", sentCount)
			}
			return nil
		}

		want.DirectLog("[CC_THINK] Trigger detected (%s), proposing send_request", triggerLabel)
		want.SetCurrent("pending_request_id", requestID)
		want.SetPlan("next_action", "send_request")

	case CCPhaseAwaitingResponse:
		// Check if MonitorAgent found a new response
		hasNew := GetCurrent(want, "has_new_response", false)
		if hasNew {
			want.DirectLog("[CC_THINK] Response received from Claude Code")
			want.SetPlan("next_action", "process_response")
			return nil
		}

		// Timeout check
		lastRequestAt := GetCurrent(want, "last_request_at", int64(0))
		timeoutSec := GetCurrent(want, "timeout_seconds", 300)
		if lastRequestAt > 0 && time.Now().Unix()-lastRequestAt > int64(timeoutSec) {
			want.DirectLog("[CC_THINK] Response timeout after %ds", timeoutSec)
			want.SetPlan("next_action", "handle_timeout")
		}

	case CCPhaseError:
		// Simple retry: wait a tick then resume
		want.DirectLog("[CC_THINK] Proposing retry from error state")
		want.SetPlan("next_action", "retry")
	}

	return nil
}

// ---------------------------------------------------------------------------
// DoAgent: Execute Claude Code CLI request
// ---------------------------------------------------------------------------

func claudeCodeRequester(ctx context.Context, want *Want) error {
	sessionID := GetGoal(want, "session_id", "")
	requestID := GetCurrent(want, "pending_request_id", "")

	// Webhook message takes priority over static auto_request
	autoRequest := GetCurrent(want, "webhook_auto_request", "")
	if autoRequest != "" {
		// Consume the webhook message so it's not reused
		want.SetCurrent("webhook_auto_request", "")
	} else {
		autoRequest = GetGoal(want, "auto_request", "")
	}

	if autoRequest == "" {
		want.StoreLog("[CC_DO] No auto_request configured, skipping")
		return nil
	}

	// Idempotency check: already sent?
	if requestID != "" && isClaudeRequestSent(sessionID, requestID) {
		want.StoreLog("[CC_DO] Request %s already sent (idempotency), skipping", requestID)
		want.SetCurrent("last_request_at", time.Now().Unix())
		return nil
	}

	// Write pending log before sending (crash recovery)
	if requestID != "" {
		writeClaudeRequestLog(sessionID, requestID, "pending")
	}

	// Build and execute Claude CLI command
	args := []string{"--print", "--output-format", "json"}
	if isValidSessionID(sessionID) {
		args = append(args, "--resume", sessionID)
	}
	args = append(args, autoRequest)

	want.StoreLog("[CC_DO] Executing: claude %s", strings.Join(args[:len(args)-1], " "))

	// Set last_request_at before executing so MonitorAgent can detect responses
	// that arrive during execution (timestamps will be >= this value).
	want.SetCurrent("last_request_at", time.Now().Unix())

	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Env = os.Environ()
	if workingDir := GetGoal(want, "working_dir", ""); workingDir != "" {
		cmd.Dir = workingDir
	}

	out, err := cmd.Output()
	if err != nil {
		errMsg := fmt.Sprintf("claude CLI failed: %v", err)
		if exitErr, ok := err.(*exec.ExitError); ok {
			errMsg = fmt.Sprintf("claude CLI exit %d: %s", exitErr.ExitCode(), string(exitErr.Stderr))
		}
		want.StoreLog("[CC_DO] ERROR: %s", errMsg)
		want.SetCurrent("last_error", errMsg)
		return fmt.Errorf("%s", errMsg)
	}

	// Parse JSON response
	var response map[string]any
	if err := json.Unmarshal(out, &response); err != nil {
		// Not JSON — store raw output
		want.SetCurrent("last_response_raw", string(out))
	} else {
		want.SetCurrent("last_response_raw", response)
		// Extract session_id from response if available for subsequent requests
		if sid, ok := response["session_id"].(string); ok && sid != "" {
			want.SetGoal("session_id", sid)
		}
	}

	// Mark sent in idempotency log
	if requestID != "" {
		writeClaudeRequestLog(sessionID, requestID, "sent")
	}

	want.StoreLog("[CC_DO] Request sent successfully")
	return nil
}

// ---------------------------------------------------------------------------
// Session file reading utilities
// ---------------------------------------------------------------------------

// sessionEntry represents a single message in a Claude Code session.
type sessionEntry struct {
	Role      string // "user" or "assistant"
	Content   string // extracted text content
	Timestamp string // ISO 8601 timestamp from the outer envelope
}

// rawSessionLine is the top-level JSONL structure in Claude Code session files.
// Lines have type: "user", "assistant", or "queue-operation" (skipped).
type rawSessionLine struct {
	Type      string          `json:"type"`
	Timestamp string          `json:"timestamp"`
	Message   json.RawMessage `json:"message"`
}

// rawMessage is the nested message inside a session line.
type rawMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"` // string or []contentBlock
}

// contentBlock is one element in a Claude content array.
type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// readClaudeSessionEntries reads session entries from the Claude Code projects directory.
// Claude Code stores conversations in ~/.claude/projects/<project-hash>/<session-id>.jsonl.
func readClaudeSessionEntries(sessionID string) ([]sessionEntry, error) {
	claudeDir := filepath.Join(os.Getenv("HOME"), ".claude", "projects")

	sessionFile, err := findSessionFile(claudeDir, sessionID)
	if err != nil {
		return nil, fmt.Errorf("session file not found for %s: %w", sessionID, err)
	}

	f, err := os.Open(sessionFile)
	if err != nil {
		return nil, fmt.Errorf("cannot open session file: %w", err)
	}
	defer f.Close()

	var entries []sessionEntry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 2*1024*1024), 2*1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var raw rawSessionLine
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}
		// Only process user/assistant message lines
		if raw.Type != "user" && raw.Type != "assistant" {
			continue
		}
		if raw.Message == nil {
			continue
		}

		text := extractMessageText(raw.Message)
		entries = append(entries, sessionEntry{
			Role:      raw.Type,
			Content:   text,
			Timestamp: raw.Timestamp,
		})
	}

	return entries, scanner.Err()
}

// extractMessageText extracts readable text from a Claude Code message.
// message.content can be a string or an array of content blocks [{type, text}, ...].
func extractMessageText(msgRaw json.RawMessage) string {
	var msg struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(msgRaw, &msg); err != nil {
		return ""
	}
	if msg.Content == nil {
		return ""
	}

	// Try as string first
	var s string
	if err := json.Unmarshal(msg.Content, &s); err == nil {
		return s
	}

	// Try as array of content blocks
	var blocks []contentBlock
	if err := json.Unmarshal(msg.Content, &blocks); err == nil {
		var texts []string
		for _, b := range blocks {
			if b.Type == "text" && b.Text != "" {
				texts = append(texts, b.Text)
			}
		}
		return strings.Join(texts, "\n")
	}

	return string(msg.Content)
}

// findSessionFile locates the JSONL session file for a given session ID.
// Searches recursively inside ~/.claude/projects/<project-hash>/ directories.
func findSessionFile(claudeDir, sessionID string) (string, error) {
	// Try direct: <claudeDir>/<sessionID>.jsonl
	direct := filepath.Join(claudeDir, sessionID+".jsonl")
	if _, err := os.Stat(direct); err == nil {
		return direct, nil
	}

	// Walk one level of project directories
	projectDirs, err := os.ReadDir(claudeDir)
	if err != nil {
		return "", err
	}
	for _, d := range projectDirs {
		if !d.IsDir() {
			continue
		}
		candidate := filepath.Join(claudeDir, d.Name(), sessionID+".jsonl")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("no .jsonl file found for session %q", sessionID)
}

// classifyClaudeSessionState determines the current state of a Claude Code session.
func classifyClaudeSessionState(entries []sessionEntry) string {
	if len(entries) == 0 {
		return "idle"
	}
	last := entries[len(entries)-1]

	switch last.Role {
	case "user":
		return "waiting_for_response"
	case "assistant":
		content := strings.ToLower(last.Content)
		if strings.HasSuffix(strings.TrimSpace(content), "?") {
			return "waiting_for_input"
		}
		return "task_complete"
	default:
		return "unknown"
	}
}

// matchClaudePattern checks entries against a regex pattern.
func matchClaudePattern(entries []sessionEntry, pattern string) (bool, string) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return false, ""
	}

	for i := len(entries) - 1; i >= 0; i-- {
		if match := re.FindString(entries[i].Content); match != "" {
			return true, match
		}
	}
	return false, ""
}

// parseTimestamp tries RFC3339Nano then RFC3339 for Claude Code timestamps.
func parseTimestamp(s string) (time.Time, bool) {
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, true
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, true
	}
	return time.Time{}, false
}

// hasNewAssistantResponse checks if there's an assistant message after the given timestamp.
func hasNewAssistantResponse(entries []sessionEntry, afterTimestamp int64) bool {
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].Role == "assistant" && entries[i].Timestamp != "" {
			if t, ok := parseTimestamp(entries[i].Timestamp); ok && t.Unix() > afterTimestamp {
				return true
			}
		}
	}
	return false
}

// isValidSessionID returns true if s looks like a UUID (8-4-4-4-12 hex).
// Claude Code requires session IDs to be in UUID format for --resume.
var uuidRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func isValidSessionID(s string) bool {
	return uuidRe.MatchString(strings.ToLower(s))
}

// getLatestAssistantContent returns the content of the most recent assistant message.
func getLatestAssistantContent(entries []sessionEntry) string {
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].Role == "assistant" {
			return entries[i].Content
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// Idempotency log utilities
// ---------------------------------------------------------------------------

const idempotencyBaseDir = ".mywant/claude_code_requests"

func idempotencyDir(sessionID string) string {
	return filepath.Join(os.Getenv("HOME"), idempotencyBaseDir, sessionID)
}

func idempotencyLogPath(sessionID, requestID string) string {
	return filepath.Join(idempotencyDir(sessionID), requestID+".json")
}

// deriveClaudeRequestID generates a deterministic request ID from current state.
func deriveClaudeRequestID(want *Want) string {
	sessionID := GetGoal(want, "session_id", "")
	autoRequest := GetGoal(want, "auto_request", "")
	reqCount := GetCurrent(want, "request_count", 0)
	matchedContent := GetCurrent(want, "matched_content", "")

	input := fmt.Sprintf("%s:%s:%d:%s", sessionID, autoRequest, reqCount, matchedContent)
	hash := sha256.Sum256([]byte(input))
	return fmt.Sprintf("%x", hash[:8])
}

func writeClaudeRequestLog(sessionID, requestID, status string) {
	dir := idempotencyDir(sessionID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return
	}

	logEntry := map[string]any{
		"request_id": requestID,
		"status":     status,
		"timestamp":  time.Now().Unix(),
	}
	data, _ := json.Marshal(logEntry)
	_ = os.WriteFile(idempotencyLogPath(sessionID, requestID), data, 0644)
}

func isClaudeRequestSent(sessionID, requestID string) bool {
	data, err := os.ReadFile(idempotencyLogPath(sessionID, requestID))
	if err != nil {
		return false
	}
	var entry map[string]any
	if err := json.Unmarshal(data, &entry); err != nil {
		return false
	}
	return entry["status"] == "sent"
}

func countClaudeSentLogs(sessionID string) int {
	dir := idempotencyDir(sessionID)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	count := 0
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			data, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				continue
			}
			var entry map[string]any
			if err := json.Unmarshal(data, &entry); err != nil {
				continue
			}
			if entry["status"] == "sent" {
				count++
			}
		}
	}
	return count
}
