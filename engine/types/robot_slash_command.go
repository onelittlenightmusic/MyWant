package types

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	. "mywant/engine/core"
)

// tryRunSlashCommand inspects an incoming robot-want chat message. If it
// starts with "/", the rest is tokenized and run as `mywant <tokens...>` —
// reusing the mywant CLI's own existing command tree (built-in subcommands
// like "agents"/"config"/"wants", and kubectl-style plugin dispatch for
// "gui" -> mywant-gui) directly, with no new subcommand or plugin of our
// own. The result is appended to cc_responses exactly like a normal LLM
// reply, and this returns true — meaning the caller must NOT forward the
// message to the LLM. Any other text returns false and is left untouched
// for the normal LLM flow.
func tryRunSlashCommand(want *Want, text string) bool {
	trimmed := strings.TrimSpace(text)
	if !strings.HasPrefix(trimmed, "/") {
		return false
	}
	rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "/"))

	args, err := tokenizeSlashCommand(rest)
	if err != nil {
		appendSlashCommandResponse(want, fmt.Sprintf("コマンド解析エラー: %v", err))
		return true
	}
	if len(args) == 0 {
		appendSlashCommandResponse(want, `使い方: /<command> [args...]（例: /gui robot say "hello"、/agents list）`)
		return true
	}

	// Async: this runs synchronously inside the robot want's own Think()
	// cycle (see agent_claude_code.go) — a command that mutates the robot
	// want itself would deadlock if exec'd inline here, since its own HTTP
	// call back into this same want would block on whatever this Think()
	// call is still holding until it returns. Firing it in a goroutine lets
	// Think() return immediately; the response still lands in cc_responses
	// whenever the command finishes.
	go runSlashCommand(want, args)
	return true
}

// runSlashCommand shells out to `mywant <args...>` directly.
func runSlashCommand(want *Want, args []string) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "mywant", args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()

	result := strings.TrimSpace(out.String())
	label := "/" + strings.Join(args, " ")
	if err != nil {
		if result != "" {
			appendSlashCommandResponse(want, fmt.Sprintf("❌ %s 失敗: %v\n%s", label, err, result))
		} else {
			appendSlashCommandResponse(want, fmt.Sprintf("❌ %s 失敗: %v", label, err))
		}
		return
	}
	if result == "" {
		result = "(no output)"
	}
	appendSlashCommandResponse(want, fmt.Sprintf("✅ %s\n%s", label, result))
}

// appendSlashCommandResponse writes to cc_responses using the same
// ring-buffer shape (max 20 entries) that claudeCodeRequester uses for LLM
// replies, so the chat sidebar / interact bubble render it identically.
func appendSlashCommandResponse(want *Want, text string) {
	responses := GetCurrent(want, "cc_responses", []any{})
	responses = append(responses, map[string]any{
		"text":      text,
		"timestamp": time.Now().Format(time.RFC3339),
		"subtype":   "slash_command",
	})
	if len(responses) > 20 {
		responses = responses[len(responses)-20:]
	}
	want.SetCurrent("cc_responses", responses)
}

// tokenizeSlashCommand splits a command string on whitespace, honoring
// double-quoted segments (e.g. `robot say "hello there"` -> ["robot", "say",
// "hello there"]). Single quotes are treated as literal characters.
func tokenizeSlashCommand(s string) ([]string, error) {
	var args []string
	var cur strings.Builder
	inQuotes := false
	hasCur := false

	for _, r := range s {
		switch {
		case r == '"':
			inQuotes = !inQuotes
			hasCur = true
		case r == ' ' || r == '\t':
			if inQuotes {
				cur.WriteRune(r)
			} else if hasCur {
				args = append(args, cur.String())
				cur.Reset()
				hasCur = false
			}
		default:
			cur.WriteRune(r)
			hasCur = true
		}
	}
	if inQuotes {
		return nil, fmt.Errorf("unterminated quote")
	}
	if hasCur {
		args = append(args, cur.String())
	}
	return args, nil
}
