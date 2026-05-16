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
	RegisterWithInit(func() {
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
	})
}

// ---------------------------------------------------------------------------
// MonitorAgent: Observe Claude Code session, write facts to Current
// ---------------------------------------------------------------------------

func claudeCodeSessionMonitor(_ context.Context, want *Want) (bool, error) {
	if GetGoal(want, "provider", "claude_code") == "gemini" {
		return geminiSessionMonitor(want)
	}

	sessionID := GetCurrent(want, "session_id", "")
	if sessionID == "" {
		// Also try goal (set by Initialize)
		sessionID = GetGoal(want, "session_id", "")
	}
	if sessionID == "" {
		// No session yet — DoAgent will create one on first trigger. Just wait.
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

	// Backfill chat history after restart: if cc_messages is empty but session has
	// entries, populate cc_messages + cc_responses from the last N exchange pairs.
	const backfillPairs = 5
	existingMessages := GetCurrent(want, "cc_messages", []any{})
	if len(existingMessages) == 0 && len(entries) > 0 {
		backfillChatHistory(want, entries, backfillPairs)
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
		// Skip when sessionID is empty — the shared idempotency dir would mix logs
		// from different want instances that haven't established a session yet.
		requestID := deriveClaudeRequestID(want)
		if sessionID != "" && isClaudeRequestSent(sessionID, requestID) {
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
	if GetGoal(want, "provider", "claude_code") == "gemini" {
		return geminiRequester(ctx, want)
	}

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
	// Skip when sessionID is empty — shared dir would match logs from other want instances.
	if sessionID != "" && requestID != "" && isClaudeRequestSent(sessionID, requestID) {
		want.StoreLog("[CC_DO] Request %s already sent (idempotency), skipping", requestID)
		want.SetCurrent("last_request_at", time.Now().Unix())
		return nil
	}

	// Write pending log before sending (crash recovery)
	if requestID != "" {
		writeClaudeRequestLog(sessionID, requestID, "pending")
	}

	// Build and execute Claude CLI command (stream-json for real-time progress)
	args := []string{"--print", "--output-format", "stream-json", "--verbose"}
	if sessionID != "" {
		args = append(args, "--resume", sessionID)
	}
	if permMode := GetGoal(want, "permission_mode", ""); permMode != "" && permMode != "default" {
		args = append(args, "--permission-mode", permMode)
	}
	if allowedTools := GetGoal(want, "allowed_tools", ""); allowedTools != "" {
		args = append(args, "--allowedTools", allowedTools)
	}
	args = append(args, autoRequest)

	want.StoreLog("[CC_DO] Executing: claude %s", strings.Join(args[:len(args)-1], " "))

	// Set last_request_at before executing so MonitorAgent can detect responses.
	want.SetCurrent("last_request_at", time.Now().Unix())

	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Env = os.Environ()
	if workingDir := GetGoal(want, "working_dir", ""); workingDir != "" {
		cmd.Dir = workingDir
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("claude stdout pipe: %v", err)
	}
	var stderrBuf strings.Builder
	cmd.Stderr = &stderrBuf

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("claude start: %v", err)
	}

	// Stream JSONL events and update state in real-time.
	var (
		finalResult    string
		finalSessionID string
		finalSubtype   string
	)

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 4*1024*1024), 4*1024*1024) // 4MB for large tool outputs
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		var ev map[string]any
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue
		}
		switch ev["type"] {
		case "system":
			if ev["subtype"] == "init" {
				if sid, ok := ev["session_id"].(string); ok && sid != "" {
					finalSessionID = sid
				}
			}

		case "assistant":
			msg, _ := ev["message"].(map[string]any)
			if msg == nil {
				continue
			}
			contents, _ := msg["content"].([]any)
			for _, c := range contents {
				cm, ok := c.(map[string]any)
				if !ok {
					continue
				}
				switch cm["type"] {
				case "text":
					if text, ok := cm["text"].(string); ok && text != "" {
						want.SetCurrent("cc_streaming_text", text)
					}
				case "tool_use":
					name, _ := cm["name"].(string)
					want.SetCurrent("cc_streaming_text", fmt.Sprintf("🔧 %s", name))
				}
			}

		case "result":
			finalSubtype, _ = ev["subtype"].(string)
			finalResult, _ = ev["result"].(string)
			if sid, ok := ev["session_id"].(string); ok && sid != "" {
				finalSessionID = sid
			}
		}
	}

	if err := cmd.Wait(); err != nil {
		want.SetCurrent("cc_streaming_text", "")
		errMsg := fmt.Sprintf("claude CLI failed: %v", err)
		if stderrBuf.Len() > 0 {
			errMsg = fmt.Sprintf("claude CLI: %s", stderrBuf.String())
		}
		want.StoreLog("[CC_DO] ERROR: %s", errMsg)
		want.SetCurrent("last_error", errMsg)
		return fmt.Errorf("%s", errMsg)
	}

	// Clear streaming indicator now that the response is complete.
	want.SetCurrent("cc_streaming_text", "")

	// Persist session_id for subsequent requests.
	if finalSessionID != "" {
		want.SetGoal("session_id", finalSessionID)
	}

	// Append final response to cc_responses ring buffer (FIFO, max 20).
	if finalResult != "" {
		responses := GetCurrent(want, "cc_responses", []any{})
		responses = append(responses, map[string]any{
			"text":      finalResult,
			"timestamp": time.Now().Format(time.RFC3339),
			"subtype":   finalSubtype,
		})
		if len(responses) > 20 {
			responses = responses[len(responses)-20:]
		}
		want.SetCurrent("cc_responses", responses)
		want.SetCurrent("last_response_raw", finalResult)
	}

	// Mark sent in idempotency log.
	if requestID != "" {
		writeClaudeRequestLog(sessionID, requestID, "sent")
	}

	want.StoreLog("[CC_DO] Request completed (result len=%d)", len(finalResult))
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

// backfillChatHistory populates cc_messages and cc_responses from the last N
// user/assistant exchange pairs in the session file. Called once after restart
// when cc_messages is empty but the session already has history.
func backfillChatHistory(want *Want, entries []sessionEntry, maxPairs int) {
	type pair struct {
		user      sessionEntry
		assistant sessionEntry
	}
	var pairs []pair
	i := len(entries) - 1
	for i >= 0 && len(pairs) < maxPairs {
		for i >= 0 && entries[i].Role != "assistant" {
			i--
		}
		if i < 0 {
			break
		}
		asst := entries[i]
		i--
		for i >= 0 && entries[i].Role != "user" {
			i--
		}
		if i < 0 {
			break
		}
		pairs = append(pairs, pair{user: entries[i], assistant: asst})
		i--
	}
	if len(pairs) == 0 {
		return
	}
	// Reverse so oldest pair is first
	for l, r := 0, len(pairs)-1; l < r; l, r = l+1, r-1 {
		pairs[l], pairs[r] = pairs[r], pairs[l]
	}
	msgs := make([]any, 0, len(pairs))
	resps := make([]any, 0, len(pairs))
	for _, p := range pairs {
		msgs = append(msgs, map[string]any{
			"sender":    "user",
			"text":      p.user.Content,
			"timestamp": p.user.Timestamp,
		})
		resps = append(resps, map[string]any{
			"text":      p.assistant.Content,
			"timestamp": p.assistant.Timestamp,
			"subtype":   "success",
		})
	}
	want.SetCurrent("cc_messages", msgs)
	want.SetCurrent("cc_responses", resps)
	want.DirectLog("[CC_MONITOR] Backfilled %d exchange pair(s) from session history", len(pairs))
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
