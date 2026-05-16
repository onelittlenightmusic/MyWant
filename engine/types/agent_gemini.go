package types

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	. "mywant/engine/core"
)

// ---------------------------------------------------------------------------
// MonitorAgent: Observe Gemini session files, write facts to Current
// ---------------------------------------------------------------------------

func geminiSessionMonitor(want *Want) (bool, error) {
	sessionID := GetGoal(want, "session_id", "")
	if sessionID == "" {
		want.SetCurrent("current_session_state", "waiting_for_first_message")
		return false, nil
	}

	phase := GetCurrent(want, "phase", CCPhaseMonitoring)
	if phase == CCPhaseAchieved {
		return true, nil
	}

	entries, err := readGeminiSessionEntries(sessionID)
	if err != nil {
		want.SetCurrent("session_read_error", err.Error())
		want.SetCurrent("current_session_state", "waiting_for_session")
		return false, nil
	}
	want.SetCurrent("session_read_error", "")

	sessionState := classifyClaudeSessionState(entries) // same logic as Claude
	want.SetCurrent("current_session_state", sessionState)

	if len(entries) > 0 {
		latest := entries[len(entries)-1]
		want.SetCurrent("latest_output", latest.Content)
		want.SetCurrent("latest_role", latest.Role)
		want.SetCurrent("latest_timestamp", latest.Timestamp)
	}

	const backfillPairs = 5
	existingMessages := GetCurrent(want, "cc_messages", []any{})
	if len(existingMessages) == 0 && len(entries) > 0 {
		backfillChatHistory(want, entries, backfillPairs)
	}

	lastRequestAt := GetCurrent(want, "last_request_at", int64(0))
	if lastRequestAt > 0 {
		hasNew := hasNewAssistantResponse(entries, lastRequestAt)
		want.SetCurrent("has_new_response", hasNew)
		if hasNew {
			want.SetCurrent("latest_response_content", getLatestAssistantContent(entries))
		}
	}

	want.SetCurrent("last_poll_at", time.Now().Unix())
	want.SetCurrent("session_entry_count", len(entries))

	return false, nil
}

// ---------------------------------------------------------------------------
// DoAgent: Execute Gemini CLI request
// ---------------------------------------------------------------------------

func geminiRequester(ctx context.Context, want *Want) error {
	sessionID := GetGoal(want, "session_id", "")
	requestID := GetCurrent(want, "pending_request_id", "")

	autoRequest := GetCurrent(want, "webhook_auto_request", "")
	if autoRequest != "" {
		want.SetCurrent("webhook_auto_request", "")
	} else {
		autoRequest = GetGoal(want, "auto_request", "")
	}

	if autoRequest == "" {
		want.StoreLog("[GEMINI_DO] No prompt configured, skipping")
		return nil
	}

	// Reuse the same idempotency mechanism as Claude Code
	if requestID != "" && isClaudeRequestSent(sessionID, requestID) {
		want.StoreLog("[GEMINI_DO] Request %s already sent (idempotency), skipping", requestID)
		want.SetCurrent("last_request_at", time.Now().Unix())
		return nil
	}
	if requestID != "" {
		writeClaudeRequestLog(sessionID, requestID, "pending")
	}

	args := []string{"-p", autoRequest, "--output-format", "json"}

	if sessionID != "" {
		// Find session index to resume by UUID
		if idx, err := findGeminiSessionIndex(sessionID, GetGoal(want, "working_dir", "")); err == nil && idx > 0 {
			args = append(args, "--resume", fmt.Sprintf("%d", idx))
		} else {
			want.StoreLog("[GEMINI_DO] Could not find session index for %s: %v", sessionID, err)
		}
	}

	want.StoreLog("[GEMINI_DO] Executing: gemini -p <prompt> --output-format json (session=%s)", sessionID)
	want.SetCurrent("last_request_at", time.Now().Unix())

	cmd := exec.CommandContext(ctx, "gemini", args...)
	cmd.Env = os.Environ()
	if workingDir := GetGoal(want, "working_dir", ""); workingDir != "" {
		cmd.Dir = workingDir
	}

	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		stderrStr := strings.TrimSpace(stderr.String())
		stdoutStr := strings.TrimSpace(string(out))
		errMsg := fmt.Sprintf("gemini CLI exit: stderr=%q stdout=%q err=%v", stderrStr, stdoutStr[:min(len(stdoutStr), 300)], err)
		want.StoreLog("[GEMINI_DO] ERROR: %s", errMsg)
		want.SetCurrent("last_error", errMsg)
		return fmt.Errorf("%s", errMsg)
	}

	// Parse JSON response — Gemini CLI output format may vary; try multiple field names
	var response map[string]any
	if err := json.Unmarshal(out, &response); err != nil {
		// Not JSON; store raw output and try to get session ID from files
		want.SetCurrent("last_response_raw", string(out))
		if sessionID == "" {
			if newID := findLatestGeminiSessionID(GetGoal(want, "working_dir", "")); newID != "" {
				want.SetGoal("session_id", newID)
			}
		}
	} else {
		want.SetCurrent("last_response_raw", response)

		// Extract session ID if present
		for _, field := range []string{"session_id", "sessionId", "conversationId"} {
			if sid, ok := response[field].(string); ok && sid != "" {
				want.SetGoal("session_id", sid)
				sessionID = sid
				break
			}
		}
		// Fall back: find from file system if session still unknown
		if sessionID == "" {
			if newID := findLatestGeminiSessionID(GetGoal(want, "working_dir", "")); newID != "" {
				want.SetGoal("session_id", newID)
				sessionID = newID
			}
		}

		// Extract response text from known field names
		var result string
		for _, field := range []string{"result", "response", "text", "content", "message"} {
			if v, ok := response[field].(string); ok && v != "" {
				result = v
				break
			}
		}
		if result != "" {
			responses := GetCurrent(want, "cc_responses", []any{})
			responses = append(responses, map[string]any{
				"text":      result,
				"timestamp": time.Now().Format(time.RFC3339),
				"subtype":   "success",
			})
			if len(responses) > 20 {
				responses = responses[len(responses)-20:]
			}
			want.SetCurrent("cc_responses", responses)
		}
	}

	if requestID != "" {
		writeClaudeRequestLog(sessionID, requestID, "sent")
	}

	want.StoreLog("[GEMINI_DO] Request sent successfully")
	return nil
}

// ---------------------------------------------------------------------------
// Gemini session file utilities
// ---------------------------------------------------------------------------

// geminiSessionLine is the top-level JSONL structure in Gemini session files.
type geminiSessionLine struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	Content   any    `json:"content"` // array for user, string for gemini
}

// readGeminiSessionEntries reads session entries from ~/.gemini/tmp/*/chats/.
func readGeminiSessionEntries(sessionID string) ([]sessionEntry, error) {
	geminiTmpDir := filepath.Join(os.Getenv("HOME"), ".gemini", "tmp")

	sessionFile, err := findGeminiSessionFile(geminiTmpDir, sessionID)
	if err != nil {
		return nil, fmt.Errorf("session file not found for %s: %w", sessionID, err)
	}

	f, err := os.Open(sessionFile)
	if err != nil {
		return nil, fmt.Errorf("cannot open session file: %w", err)
	}
	defer f.Close()

	// Track entries by ID to get the latest version of each (Gemini writes duplicates)
	type indexedEntry struct {
		idx   int
		entry sessionEntry
	}
	seen := map[string]indexedEntry{}
	var orderedIDs []string
	lineIdx := 0

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 2*1024*1024), 2*1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "{\"$set\"") {
			continue
		}

		var raw map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}

		msgType := strings.Trim(string(raw["type"]), `"`)
		if msgType != "user" && msgType != "gemini" {
			continue
		}

		var id string
		json.Unmarshal(raw["id"], &id) //nolint:errcheck

		var timestamp string
		json.Unmarshal(raw["timestamp"], &timestamp) //nolint:errcheck

		role := "user"
		if msgType == "gemini" {
			role = "assistant"
		}

		content := extractGeminiContent(raw["content"], msgType)
		// Skip gemini entries with no text (tool-only intermediate steps)
		if msgType == "gemini" && content == "" {
			continue
		}

		entry := sessionEntry{Role: role, Content: content, Timestamp: timestamp}

		if id == "" {
			// No ID — just append in order
			placeholder := fmt.Sprintf("__line_%d", lineIdx)
			seen[placeholder] = indexedEntry{idx: lineIdx, entry: entry}
			orderedIDs = append(orderedIDs, placeholder)
		} else if _, exists := seen[id]; !exists {
			seen[id] = indexedEntry{idx: lineIdx, entry: entry}
			orderedIDs = append(orderedIDs, id)
		} else {
			// Update existing entry (Gemini writes the same ID multiple times as it streams)
			seen[id] = indexedEntry{idx: seen[id].idx, entry: entry}
		}
		lineIdx++
	}

	entries := make([]sessionEntry, 0, len(orderedIDs))
	for _, id := range orderedIDs {
		entries = append(entries, seen[id].entry)
	}
	return entries, scanner.Err()
}

// extractGeminiContent extracts displayable text from a Gemini content field.
// User content is [{text:"..."}], model content is a plain string.
func extractGeminiContent(raw json.RawMessage, msgType string) string {
	if raw == nil {
		return ""
	}
	if msgType == "user" {
		var parts []struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(raw, &parts); err == nil {
			var sb strings.Builder
			for _, p := range parts {
				sb.WriteString(p.Text)
			}
			return sb.String()
		}
	}
	// model: string content
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return ""
}

// findGeminiSessionFile searches ~/.gemini/tmp/ for a session file by UUID.
// Gemini names files as session-<timestamp>-<first8charsOfUUID>.jsonl.
func findGeminiSessionFile(geminiTmpDir, sessionID string) (string, error) {
	shortID := sessionID
	if len(sessionID) > 8 {
		shortID = sessionID[:8]
	}

	var found string
	_ = filepath.Walk(geminiTmpDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		base := filepath.Base(path)
		if strings.HasSuffix(base, "-"+shortID+".jsonl") {
			found = path
			return filepath.SkipAll
		}
		return nil
	})

	if found == "" {
		return "", fmt.Errorf("no session file found for UUID %s", sessionID)
	}
	return found, nil
}

// findGeminiSessionIndex returns the 1-based index for --resume by parsing
// the output of `gemini --list-sessions`. Returns 0 on failure.
func findGeminiSessionIndex(sessionID, workingDir string) (int, error) {
	cmd := exec.Command("gemini", "--list-sessions")
	if workingDir != "" {
		cmd.Dir = workingDir
	}
	out, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("gemini --list-sessions failed: %w", err)
	}

	// Output format (from largest index to smallest, newest first):
	//   1. Title (6 days ago) [uuid-full]
	shortID := sessionID
	if len(sessionID) > 8 {
		shortID = sessionID[:8]
	}

	for _, line := range strings.Split(string(out), "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.Contains(trimmed, sessionID) && !strings.Contains(trimmed, shortID) {
			continue
		}
		// Extract the number before the first "."
		dotIdx := strings.Index(trimmed, ".")
		if dotIdx <= 0 {
			continue
		}
		var idx int
		if _, err := fmt.Sscanf(trimmed[:dotIdx], "%d", &idx); err == nil && idx > 0 {
			return idx, nil
		}
	}
	return 0, fmt.Errorf("session %s not found in --list-sessions output", sessionID)
}

// findLatestGeminiSessionID returns the UUID of the most recently created Gemini session.
// Used when the DoAgent cannot extract session_id from the JSON response.
func findLatestGeminiSessionID(workingDir string) string {
	cmd := exec.Command("gemini", "--list-sessions")
	if workingDir != "" {
		cmd.Dir = workingDir
	}
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	// The first listed session (index 1) is the most recent
	for _, line := range strings.Split(string(out), "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "1.") {
			continue
		}
		start := strings.LastIndex(trimmed, "[")
		end := strings.LastIndex(trimmed, "]")
		if start >= 0 && end > start {
			return trimmed[start+1 : end]
		}
	}
	return ""
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
