package types

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
// The conversation is kept. fmtool runs as a live process (`--serve`, see
// fm_server.go) holding one session, so "新宿はどこ？" and then "その隣は？"
// are two turns of one talk rather than two strangers. The session is the only
// memory — this model has 8k tokens for all of it — so the agent drops the
// transcript and starts again when it fills, and says so when it did.

const (
	// Where the answer comes from when nothing says otherwise.
	fmToolDefaultBinary = "fmtool"
	// Long enough for the on-device model to answer with a tool call or two
	// (measured: ~4s for a MyWant status question), short enough that a stuck
	// process does not hold the want's cycle.
	fmDefaultTimeoutSeconds = 120
)

// fmToolPath finds the on-device agent, or reports that this machine has none.
//
// Four places, in order: MYWANT_FM_BIN for a build kept somewhere particular,
// then PATH, then beside the binary that is running — `make fmtool` puts it in
// this project's bin/ next to mywant itself — then ~/.local/bin, where
// `make install-fmtool` puts it. macOS only: FoundationModels is Apple's, and a
// Linux server (fly.io) has no such model to ask, which is exactly when the
// caller falls back to Claude.
//
// The agent's source lives in this repository (fmtool/). It was a second repo
// for as long as it was an experiment; a want type that depends on it is not an
// experiment, and two repositories that have to be in step are one repository
// with a gap in it.
// HasOnDeviceModel reports whether this machine can answer with its own model.
//
// Asked by the server so a person choosing who answers is not offered a choice
// that silently turns into another one: `provider: fm` on a Linux box falls
// back to Claude, which is the right behaviour and a confusing thing to pick.
func HasOnDeviceModel() bool {
	_, ok := fmToolPath()
	return ok
}

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
	if self, err := os.Executable(); err == nil {
		beside := filepath.Join(filepath.Dir(self), fmToolDefaultBinary)
		if info, err := os.Stat(beside); err == nil && !info.IsDir() {
			return beside, true
		}
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
	// The context is the want's cycle; the wait for an answer is the timeout
	// below, which the served agent is given directly.
	_ = ctx

	want.StoreLog("[FM_DO] Asking the on-device model: %s", binary)
	want.SetCurrent("last_request_at", time.Now().Unix())
	want.SetCurrent("cc_streaming_text", "考えています…")

	reply, err := fmServerFor(binary).ask(request, root, timeout)
	answer := strings.TrimSpace(reply.Text)
	notes := reply.Error

	// Which tool it reached for, if any — the chat shows it the same way it
	// shows Claude Code's tool calls.
	if reply.Tool != "" {
		RecordCCActivity(want, CCActivityTool, reply.Tool)
	}
	// The conversation was trimmed to make room — the recent turns carried, the
	// older talk and its tool output dropped. Said out loud, because what the
	// next question can refer back to has just got shorter.
	if reply.Trimmed {
		want.StoreLog("[FM_DO] The conversation was trimmed to its recent turns")
		RecordCCActivity(want, CCActivityNote, "（会話が長くなったので、古いやり取りを整理しました）")
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
		// Say so, rather than going quiet.
		//
		// A failure used to end here: the error went to the log and the want's
		// last_error, and the person who asked got nothing at all — the same
		// silence as not having been heard. Being told "I could not answer" is
		// a different thing from being ignored, and the one thing the asker
		// needs to know before asking again.
		// firstLine + truncateRunes: the reason fmtool printed first, kept short
		// enough for a speech bubble (both live in web_inspector_naming.go).
		reason := strings.TrimSpace(firstLine(notes))
		if reason == "" {
			reason = "理由は分かりません"
		}
		recordFMAnswer(want, "答えられませんでした: "+truncateRunes(reason, 160))
		return fmt.Errorf("%s", errMsg)
	}

	want.SetCurrent("cc_streaming_text", "")
	want.SetCurrent("last_error", "")

	if answer == "" {
		want.StoreLog("[FM_DO] Answered with nothing")
		return nil
	}

	recordFMAnswer(want, answer)
	want.SetCurrent("last_response_raw", answer)

	if requestID != "" {
		writeClaudeRequestLog("", requestID, "sent")
	}
	want.StoreLog("[FM_DO] Answered (len=%d)", len(answer))
	return nil
}

// recordFMAnswer puts one answer where every provider's answers go: the chat's
// ring buffer, and the robot's own mouth.
func recordFMAnswer(want *Want, answer string) {
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
	// The robot answering is the robot speaking — see the same call in
	// claudeCodeRequester for why only the robot has a mouth.
	if want.Metadata.Type == "robot" {
		CharacterSpeaks("robot", answer, "agent")
	}
}
