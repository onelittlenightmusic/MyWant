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
				if want.Metadata.Type == "robot" && tryRunSlashCommand(want, text) {
					want.StoreLog("[CC_THINK] Handled as a slash command, not forwarding to LLM")
					want.SetCurrent("cc_webhook_processed", msgCount)
				} else {
					want.StoreLog("[CC_THINK] Webhook message received, overriding auto_request")
					want.SetCurrent("webhook_auto_request", text)
					want.SetCurrent("cc_webhook_processed", msgCount)
				}
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
		case "waiting", "complete":
			// One event, two names. The transcript records that the assistant
			// yielded the turn; whether that means "I have a question" or "I am
			// done" is not something it states, and guessing it from punctuation
			// is what used to make both of these wrong. Wants configured either
			// way keep firing where they always meant to.
			autonomousTriggered = GetCurrent(want, "current_session_state", "") == "waiting_for_input"
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
			want.StoreLog("[CC_THINK] Request %s already sent, skipping", requestID)
			// A message that has been delivered must not stay queued. Left in
			// the slot it re-triggers this same check on every tick forever:
			// the log fills with skips, the want looks busy, and — because the
			// slot holds exactly one message — nothing anyone says afterwards
			// is ever looked at. Now that the id is derived from the message
			// itself, reaching here means this exact message really was sent,
			// so dropping it loses nothing.
			if GetCurrent(want, "webhook_auto_request", "") != "" {
				want.SetCurrent("webhook_auto_request", "")
			}
			// Restore request_count from idempotency log
			sentCount := countClaudeSentLogs(sessionID)
			currentCount := GetCurrent(want, "request_count", 0)
			if sentCount > currentCount {
				want.SetCurrent("request_count", sentCount)
			}
			return nil
		}

		want.StoreLog("[CC_THINK] Trigger detected (%s), proposing send_request", triggerLabel)
		want.SetCurrent("pending_request_id", requestID)
		want.SetPlan("next_action", "send_request")

	case CCPhaseAwaitingResponse:
		// Check if MonitorAgent found a new response
		hasNew := GetCurrent(want, "has_new_response", false)
		if hasNew {
			want.StoreLog("[CC_THINK] Response received from Claude Code")
			want.SetPlan("next_action", "process_response")
			return nil
		}

		// Timeout check
		lastRequestAt := GetCurrent(want, "last_request_at", int64(0))
		timeoutSec := GetCurrent(want, "timeout_seconds", 300)
		if lastRequestAt > 0 && time.Now().Unix()-lastRequestAt > int64(timeoutSec) {
			want.StoreLog("[CC_THINK] Response timeout after %ds", timeoutSec)
			want.SetPlan("next_action", "handle_timeout")
		}

	case CCPhaseError:
		// Simple retry: wait a tick then resume
		want.StoreLog("[CC_THINK] Proposing retry from error state")
		want.SetPlan("next_action", "retry")
	}

	return nil
}

// ---------------------------------------------------------------------------
// DoAgent: Execute Claude Code CLI request
// ---------------------------------------------------------------------------

// sanitizedSubprocessEnv returns a copy of mywant's own process environment
// with vars stripped that indicate a nested Claude Code / API-key auth
// context (ANTHROPIC_API_KEY, CLAUDECODE, CLAUDE_CODE_*). Provider CLIs
// (claude, gemini) inherit whatever shell mywant itself happened to be
// launched from — if that shell is itself a live Claude Code session (or has
// ANTHROPIC_API_KEY set for unrelated reasons), the claude CLI detects it and
// disables claude.ai-connector auth ("claude.ai connectors are disabled
// because ANTHROPIC_API_KEY or another auth source is set"), breaking the
// `coding` want type regardless of how mywant was started.
func sanitizedSubprocessEnv() []string {
	out := make([]string, 0, len(os.Environ()))
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if name == "ANTHROPIC_API_KEY" || name == "CLAUDECODE" || strings.HasPrefix(name, "CLAUDE_CODE_") {
			continue
		}
		out = append(out, kv)
	}
	return out
}

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
	if model := GetGoal(want, "model", ""); model != "" {
		args = append(args, "--model", model)
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
	cmd.Env = sanitizedSubprocessEnv()
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
						RecordCCActivity(want, CCActivityNote, text)
					}
				case "tool_use":
					name, _ := cm["name"].(string)
					want.SetCurrent("cc_streaming_text", fmt.Sprintf("🔧 %s", name))
					input, _ := cm["input"].(map[string]any)
					RecordCCActivityDetail(want, CCActivityTool, name, toolInputDetail(input))
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
		RecordCCActivity(want, CCActivityError, errMsg)
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
		// The robot answering is the robot speaking, and it goes in the same
		// column as everybody else's words. Only the robot: a `coding` want is
		// somebody's agent on the board, not a character, and has no mouth.
		if want.Metadata.Type == "robot" {
			CharacterSpeaks("robot", finalResult, "agent")
		}
		// The closing text arrives twice — once as an assistant event (recorded
		// as a note above) and again as the result. Drop the note so the chat
		// does not show the answer immediately before itself.
		dropTrailingActivityNote(want, finalResult)
	}

	// Mark sent in idempotency log.
	if requestID != "" {
		writeClaudeRequestLog(sessionID, requestID, "sent")
	}

	want.StoreLog("[CC_DO] Request completed (result len=%d)", len(finalResult))
	return nil
}

// ---------------------------------------------------------------------------
// Activity log
// ---------------------------------------------------------------------------

// ccActivityCap bounds the activity ring buffer. Long enough to cover a whole
// exchange's worth of tool calls, short enough that the state field stays
// cheap to ship on every poll.
const ccActivityCap = 100

// Activity kinds. The chat tab styles each one differently, so they are a small
// closed set rather than free text.
const (
	CCActivityNote  = "note"  // the agent narrating between tool calls
	CCActivityTool  = "tool"  // a tool it reached for
	CCActivityPhase = "phase" // a state transition of the want itself
	CCActivityError = "error"
)

// RecordCCActivity appends one line to the running account of what the agent
// did between the question and the answer.
//
// cc_streaming_text holds only the newest line — each tool call overwrites the
// last, and the whole thing is cleared when the response lands — so without
// this the intermediate work is visible for a moment and then gone. This keeps
// it, so the chat can replay the work alongside the messages.
//
// AppendState (not SetCurrent) because the stream produces events faster than
// the reconcile interval, and a single slot silently drops the ones in between.
func RecordCCActivity(want *Want, kind, text string) {
	RecordCCActivityDetail(want, kind, text, "")
}

// ccDetailCap bounds one entry's detail. Long enough for a real shell command
// or a wall of tool arguments, short enough that a runaway input can't bloat
// the state field the GUI polls.
const ccDetailCap = 4000

// RecordCCActivityDetail is RecordCCActivity plus the full text behind the
// summary — the command a Bash call actually ran, the path a Read opened. The
// summary is what the chat shows inline; the detail is what it reveals when
// the line is expanded, so it can be long without making the log unreadable.
func RecordCCActivityDetail(want *Want, kind, text, detail string) {
	if text == "" {
		return
	}
	if len(detail) > ccDetailCap {
		detail = detail[:ccDetailCap] + "\n… (truncated)"
	}
	entry := map[string]any{
		"kind":      kind,
		"text":      text,
		"timestamp": time.Now().Format(time.RFC3339),
	}
	if detail != "" {
		entry["detail"] = detail
	}
	want.AppendState("cc_activity", entry)
	// Trim from the front once over cap. Appends outnumber trims heavily, so
	// paying the read-modify-write only when the buffer is actually full keeps
	// the hot path to a single locked append.
	if list := GetCurrent(want, "cc_activity", []any{}); len(list) > ccActivityCap {
		want.SetCurrent("cc_activity", list[len(list)-ccActivityCap:])
	}
}

// toolInputDetail renders a tool call's arguments as the text to show when the
// log line is expanded.
//
// The salient argument comes first on its own line — for Bash that is the
// command, which is the whole reason to expand a "Bash" line at all — followed
// by the rest of the arguments. Nothing is dropped: a tool this doesn't know
// still shows everything it was given.
func toolInputDetail(input map[string]any) string {
	if len(input) == 0 {
		return ""
	}
	// Ordered by how much each identifies what the call did. The first one
	// present wins; matching on argument names rather than tool names means a
	// tool this has never seen still gets a useful headline.
	var lead string
	for _, k := range []string{"command", "file_path", "path", "pattern", "url", "query", "prompt"} {
		if v, ok := input[k].(string); ok && v != "" {
			lead = v
			break
		}
	}

	rest, err := json.MarshalIndent(input, "", "  ")
	if err != nil {
		return lead
	}
	if lead == "" {
		return string(rest)
	}
	return lead + "\n\n" + string(rest)
}

// dropTrailingActivityNote removes the last activity entry when it is a note
// holding exactly the given text — the duplicate the closing assistant event
// leaves behind once the same text arrives as the result.
func dropTrailingActivityNote(want *Want, text string) {
	list := GetCurrent(want, "cc_activity", []any{})
	if len(list) == 0 {
		return
	}
	last, ok := list[len(list)-1].(map[string]any)
	if !ok {
		return
	}
	if last["kind"] != CCActivityNote {
		return
	}
	if s, _ := last["text"].(string); s != text {
		return
	}
	want.SetCurrent("cc_activity", list[:len(list)-1])
}

// SetCCPhase stores the want's phase and records the transition in the activity
// log, so the chat shows how the want moved, not just where it ended up.
// Re-setting the phase it is already in records nothing.
func SetCCPhase(want *Want, phase string) {
	if GetCurrent(want, "phase", "") == phase {
		want.SetCurrent("phase", phase)
		return
	}
	want.SetCurrent("phase", phase)
	RecordCCActivity(want, CCActivityPhase, phase)
}

// ---------------------------------------------------------------------------
// Session file reading utilities
// ---------------------------------------------------------------------------

// sessionEntry represents a single message in a Claude Code session.
type sessionEntry struct {
	Role      string // "user" or "assistant"
	Content   string // extracted text content
	Timestamp string // ISO 8601 timestamp from the outer envelope
	// StopReason is why the assistant stopped: "end_turn" (yielded to the user),
	// "tool_use" (paused to run a tool, still working), "stop_sequence", or ""
	// on user lines. This is how the transcript says whether a turn is over —
	// the text alone cannot, and reading punctuation instead of this field is
	// what made the session state wrong.
	StopReason string
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
	Role       string `json:"role"`
	Content    any    `json:"content"` // string or []contentBlock
	StopReason string `json:"stop_reason"`
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
			Role:       raw.Type,
			Content:    text,
			Timestamp:  raw.Timestamp,
			StopReason: extractStopReason(raw.Message),
		})
	}

	return entries, scanner.Err()
}

// extractStopReason reads message.stop_reason, which is absent on user lines.
func extractStopReason(msgRaw json.RawMessage) string {
	var msg struct {
		StopReason string `json:"stop_reason"`
	}
	if err := json.Unmarshal(msgRaw, &msg); err != nil {
		return ""
	}
	return msg.StopReason
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
//
// The rule is about whose turn it is, not about what was said. When the
// assistant yields the turn the ball is with the user, and that is
// waiting_for_input whether the last sentence was a question, a summary or a
// diff. This used to be decided by whether the text ended in "?" — which meant
// an ASCII question mark, so a Japanese session ending in "？" (or in a question
// followed by a parenthetical) was read as "task complete" and never triggered
// a trigger_on: waiting want.
//
// stop_reason is the transcript's own answer to the same question, and it also
// distinguishes the case the punctuation rule could not see at all: an assistant
// message that stopped to call a tool has not yielded anything, and reporting it
// as finished said the task was done while the work was still running.
func classifyClaudeSessionState(entries []sessionEntry) string {
	if len(entries) == 0 {
		return "idle"
	}
	last := entries[len(entries)-1]

	switch last.Role {
	case "user":
		return "waiting_for_response"
	case "assistant":
		if last.StopReason == "tool_use" {
			return "waiting_for_response" // paused to run a tool; still working
		}
		return "waiting_for_input"
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
	want.StoreLog("[CC_MONITOR] Backfilled %d exchange pair(s) from session history", len(pairs))
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
// deriveClaudeRequestID names the request a trigger is asking for, so that the
// same one is never sent twice.
//
// A message from a person is identified by the MESSAGE. It used to be
// identified by a counter — session:auto_request:request_count:matched — which
// contains nothing the person typed, so what the id actually said was "the Nth
// request of this session", and two entirely different messages could hash the
// same. Then the counter could not advance: it only moves when a send
// completes, and the send was being skipped as a duplicate. A message that hit
// that state was dropped in silence, permanently, and so was every message
// after it — the only way out was deleting files by hand.
//
// What makes one message different from another is what it says, where it sits
// in the conversation, and when it was said, so that is what is hashed. Two
// different messages cannot collide. The same message after a server restart
// still hashes the same and is still not sent twice, which is the one thing the
// idempotency log is actually for.
//
// Autonomous triggers (pattern / waiting) keep the old derivation: there is no
// message there, and what identifies those is the matched content and the
// session's own state.
func deriveClaudeRequestID(want *Want) string {
	sessionID := GetGoal(want, "session_id", "")

	if msg := GetCurrent(want, "webhook_auto_request", ""); msg != "" {
		stamp := ""
		if latest := GetCurrent(want, "cc_latest_message", map[string]any{}); len(latest) > 0 {
			if ts, ok := latest["timestamp"].(string); ok {
				stamp = ts
			}
		}
		// The count distinguishes the same words said twice; the timestamp
		// distinguishes them across a want that was recreated and started
		// counting again.
		count := GetCurrent(want, "cc_message_count", 0)
		return hashClaudeRequestID(fmt.Sprintf("%s:webhook:%d:%s:%s", sessionID, count, stamp, msg))
	}

	autoRequest := GetGoal(want, "auto_request", "")
	reqCount := GetCurrent(want, "request_count", 0)
	matchedContent := GetCurrent(want, "matched_content", "")
	return hashClaudeRequestID(fmt.Sprintf("%s:%s:%d:%s", sessionID, autoRequest, reqCount, matchedContent))
}

func hashClaudeRequestID(input string) string {
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
