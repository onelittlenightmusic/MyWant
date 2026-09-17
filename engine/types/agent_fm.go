package types

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	. "mywant/engine/core"
)

// The robot, answered by the Mac it is running on.
//
// `provider: fm` sends a request to fmtool — a small on-device agent built on
// Apple's FoundationModels framework, with tools of its own for the clock, the
// filesystem and this server (see github.com/onelittlenightmusic/fm-tools-proto).
// Nothing leaves the machine, nothing is billed, and an answer about the board
// ("is the server up", "what is running") comes back in a few seconds.
//
// It is deliberately the SIMPLEST of the three providers. Claude Code and
// Gemini are sessions the monitor agent reads back out of files; fmtool is one
// process, one question, one answer — there is no session to resume, and each
// request starts fresh. That is the trade for local and instant, and it is why
// this provider suits the header bubble's questions rather than a long piece of
// work.

const (
	// Where the answer comes from when nothing says otherwise.
	fmToolDefaultBinary = "fmtool"
	// Long enough for the on-device model to answer with a tool call or two
	// (measured: ~4s for a MyWant status question), short enough that a stuck
	// process does not hold the want's cycle.
	fmDefaultTimeoutSeconds = 120
)

// fmToolLine picks the tool fmtool reports on stderr: "[tool: mywant_status, native: true]".
var fmToolLine = regexp.MustCompile(`\[tool: ([a-z_]+), native: (true|false)\]`)

// fmToolPath finds the on-device agent, or reports that this machine has none.
//
// Three places, in order: MYWANT_FM_BIN for a build kept somewhere particular,
// then PATH, then ~/.local/bin, which is where `make install` puts the binaries
// in this project. macOS only — FoundationModels is Apple's, and a Linux server
// (fly.io) has no such model to ask, which is exactly when the caller falls
// back to Claude.
func fmToolPath() (string, bool) {
	if runtime.GOOS != "darwin" {
		return "", false
	}
	if custom := strings.TrimSpace(os.Getenv("MYWANT_FM_BIN")); custom != "" {
		if info, err := os.Stat(custom); err == nil && !info.IsDir() {
			return custom, true
		}
		return "", false
	}
	if found, err := exec.LookPath(fmToolDefaultBinary); err == nil {
		return found, true
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", false
	}
	candidate := filepath.Join(home, ".local", "bin", fmToolDefaultBinary)
	if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
		return candidate, true
	}
	return "", false
}

// fmSessionMonitor stands in for the session watcher the other two providers
// need, and reports the one fact the phase machine is waiting on.
//
// There is no session file to read here — the answer was recorded by
// fmRequester before the process exited (see agent_claude_code.go's phase
// machine: a want left in `awaiting_response` never asks anything again, which
// is what happened while this said nothing). So the answer already in hand is
// reported as the new response, and the want goes back to listening.
func fmSessionMonitor(want *Want) {
	want.SetCurrent("current_session_state", "on_device")
	want.SetCurrent("last_poll_at", time.Now().Unix())

	lastRequestAt := GetCurrent(want, "last_request_at", int64(0))
	if lastRequestAt == 0 {
		return
	}
	responses := GetCurrent(want, "cc_responses", []any{})
	if len(responses) == 0 {
		return
	}
	latest, ok := responses[len(responses)-1].(map[string]any)
	if !ok {
		return
	}
	stamp, _ := latest["timestamp"].(string)
	answeredAt, err := time.Parse(time.RFC3339, stamp)
	if err != nil || answeredAt.Unix() < lastRequestAt {
		return
	}
	text, _ := latest["text"].(string)
	want.SetCurrent("has_new_response", true)
	want.SetCurrent("latest_response_content", text)
	want.SetCurrent("latest_role", "assistant")
	want.SetCurrent("latest_timestamp", answeredAt.Unix())
}

// fmRequester asks the on-device model, and records the answer exactly the way
// the other providers record theirs — the chat, the bubble and the history know
// nothing about which one answered.
func fmRequester(ctx context.Context, want *Want, binary string) error {
	requestID := GetCurrent(want, "pending_request_id", "")

	// Webhook message first, static auto_request second — the same order the
	// other two providers read them in.
	request := GetCurrent(want, "webhook_auto_request", "")
	if request != "" {
		want.SetCurrent("webhook_auto_request", "")
	} else {
		request = GetGoal(want, "auto_request", "")
	}
	if request == "" {
		want.StoreLog("[FM_DO] No prompt configured, skipping")
		return nil
	}

	// The same idempotency log the other providers write. There is no session
	// id here, so the log is keyed by the request alone.
	if requestID != "" && isClaudeRequestSent("", requestID) {
		want.StoreLog("[FM_DO] Request %s already sent (idempotency), skipping", requestID)
		want.SetCurrent("last_request_at", time.Now().Unix())
		return nil
	}
	if requestID != "" {
		writeClaudeRequestLog("", requestID, "pending")
	}

	// What the agent may read: the want's working dir, else the home directory.
	// fmtool sandboxes its file tools to this root.
	root := GetGoal(want, "working_dir", "")
	if root == "" {
		if home, err := os.UserHomeDir(); err == nil {
			root = home
		}
	}

	timeout := time.Duration(GetGoal(want, "timeout_seconds", fmDefaultTimeoutSeconds)) * time.Second
	if timeout <= 0 {
		timeout = fmDefaultTimeoutSeconds * time.Second
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	want.StoreLog("[FM_DO] Asking the on-device model: %s", binary)
	want.SetCurrent("last_request_at", time.Now().Unix())
	want.SetCurrent("cc_streaming_text", "考えています…")

	args := []string{}
	if root != "" {
		args = append(args, "--root", root)
	}
	args = append(args, request)

	cmd := exec.CommandContext(runCtx, binary, args...)
	// See sanitizedSubprocessEnv (agent_claude_code.go): the same stripping,
	// for the same reason — a nested agent context is not this one's.
	cmd.Env = sanitizedSubprocessEnv()
	if root != "" {
		cmd.Dir = root
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	answer := strings.TrimSpace(string(out))
	notes := strings.TrimSpace(stderr.String())

	// Which tool it reached for, if any — fmtool says so on stderr, and the
	// chat shows it the same way it shows Claude Code's tool calls.
	if m := fmToolLine.FindStringSubmatch(notes); m != nil {
		RecordCCActivity(want, CCActivityTool, m[1])
	}

	if err != nil {
		want.SetCurrent("cc_streaming_text", "")
		errMsg := fmt.Sprintf("fmtool failed: %v", err)
		if notes != "" {
			errMsg = fmt.Sprintf("fmtool: %s", notes)
		}
		want.StoreLog("[FM_DO] ERROR: %s", errMsg)
		want.SetCurrent("last_error", errMsg)
		RecordCCActivity(want, CCActivityError, errMsg)
		return fmt.Errorf("%s", errMsg)
	}

	want.SetCurrent("cc_streaming_text", "")
	want.SetCurrent("last_error", "")

	if answer == "" {
		want.StoreLog("[FM_DO] Answered with nothing")
		return nil
	}

	responses := GetCurrent(want, "cc_responses", []any{})
	responses = append(responses, map[string]any{
		"text":      answer,
		"timestamp": time.Now().Format(time.RFC3339),
		"subtype":   "fm",
	})
	if len(responses) > 20 {
		responses = responses[len(responses)-20:]
	}
	want.SetCurrent("cc_responses", responses)
	want.SetCurrent("last_response_raw", answer)
	// The robot answering is the robot speaking — see the same call in
	// claudeCodeRequester for why only the robot has a mouth.
	if want.Metadata.Type == "robot" {
		CharacterSpeaks("robot", answer, "agent")
	}

	if requestID != "" {
		writeClaudeRequestLog("", requestID, "sent")
	}
	want.StoreLog("[FM_DO] Answered (len=%d)", len(answer))
	return nil
}
