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

	// A yes to what a goal proposed is carried out here.
	//
	// A goal that found nothing on the board to answer with does not guess and
	// does not act: it writes down the command that would do it and asks (see
	// freeGoalAsk). The question reaches the person as a sentence in the chat
	// and as the board's own two-button overlay, and both answer it by saying
	// "はい" to the robot — so this is where the yes lands.
	//
	// Not left to the model. The goal is over by the time the question is
	// asked, it ran outside the conversation, and the model was never told
	// what was proposed: asked "はい" it has nothing to say yes to and says
	// something agreeable instead. The sentence is right here, and running it
	// is not a judgement call.
	if done, reply := answerGoalPending(ctx, want, request); done {
		recordFMAnswer(want, reply)
		want.SetCurrent("last_request_at", time.Now().Unix())
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
	// The context is the want's cycle: it carries the commands the broker runs
	// on the agent's behalf. The wait for an answer is the timeout below, which
	// the served agent is given directly.

	want.StoreLog("[FM_DO] Asking the on-device model: %s", binary)
	want.SetCurrent("last_request_at", time.Now().Unix())
	want.SetCurrent("cc_streaming_text", "考えています…")
	RecordCCActivity(want, "note", truncateRunes(request, 80))

	// What it is doing, as it does it. The agent says on its stderr which
	// command it is running and which tool it reached for; those lines used to
	// go to the server log, where the person waiting cannot see them, and all
	// the board showed for the whole thirty seconds was "考えています".
	server := fmServerFor(binary)
	server.watch(func(kind, text string) {
		RecordCCActivity(want, kind, truncateRunes(text, 90))
	})
	// Every command the agent wants run comes back through here first. It runs
	// none itself any more: what is safe, what is the board's and what needs a
	// person are all this side's to say, and saying them twice in two languages
	// is how the two answers came apart. See fm_broker.go.
	server.broker(func(command string, args []string) (bool, bool, string) {
		return fmBrokerRun(ctx, want, request, command, args)
	})
	reply, err := server.ask(request, root, timeout)
	server.watch(nil)
	server.broker(nil)
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

	// The agent handed the request back instead of answering it: making or
	// finding something takes several commands, and which ones depends on what
	// the earlier ones found. That is worked out here, step by step, with the
	// model asked one short question at a time — the same loop `mywant do`
	// runs, run quietly against a want that is never put on the board. What
	// comes back is the robot's answer, because it is the answer.
	if goal := strings.TrimSpace(reply.Goal); goal != "" {
		want.StoreLog("[FM_DO] Working out: %s", goal)
		want.SetCurrent("cc_streaming_text", "手順を考えています…")
		RecordCCActivity(want, CCActivityNote, "手順を考えています…")
		goalAnswer, goalPending := runGoalInline(ctx, goal, func(line string) {
			RecordCCActivity(want, CCActivityTool, truncateRunes(line, 90))
		})
		want.SetCurrent("cc_streaming_text", "")
		if goalAnswer != "" {
			answer = goalAnswer
		}
		// What a yes would carry out, kept where the next turn can find it.
		if goalPending != "" {
			want.SetCurrent("goal_pending", goalPending)
			want.SetCurrent("pending_command", goalPending)
		}
	}

	if answer == "" {
		want.StoreLog("[FM_DO] Answered with nothing")
		return nil
	}

	recordFMAnswer(want, answer)
	want.SetCurrent("last_response_raw", answer)
	// pending_command is not set here: whatever is waiting was written the
	// moment it came up — by the broker when the agent reached for something
	// destructive, or by the goal loop when it had a proposal — and a blanket
	// write at the end of the turn would clear it again. Where a screen can
	// see it, either way: see the confirmation overlay in the dashboard.

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

// The ways a person says yes to the robot, and the ways they say no.
//
// One list, in one language's worth of code, because there used to be two: one
// here and one in the on-device agent's own gate, and they had stopped agreeing
// about どうぞ, いいよ, そう and 実行. Both are read the same way now — this is
// the only place a yes is recognised at all (see fm_broker.go for why the agent
// no longer has an opinion about it).
//
// Both spellings of the question end up here: typed into the chat, or clicked
// on the board's confirmation overlay, which says 「はい」 as the person.
var goalYesWords = []string{
	"はい", "うん", "ええ", "そう", "そうして", "どうぞ", "いいよ", "お願い", "おねがい",
	"やって", "実行", "消して", "削除して", "ok", "okay", "yes", "y", "sure",
	"go ahead", "do it", "please",
}

// goalNoWords end the offer without running it. Read before the yes list, so
// "いいえ" is not answered by the "い" in it.
var goalNoWords = []string{"いいえ", "no", "n", "やめ", "キャンセル", "cancel", "しない", "結構", "だめ"}

// goalPoliteTails are what a yes may be wearing and still be only a yes.
var goalPoliteTails = []string{"します", "してください", "して", "ください", "よ", "ね", "です", "ます", "!", "！"}

// isOnlyConsent reports whether a message is agreement and nothing else.
//
// The distinction that matters: 「はい」 is consent, and
// 「はい、でも先に天気を見せて」 is a new request that happens to start with one.
// The agent's old gate claimed to make it and did not — it accepted any short
// message beginning with a yes, and that example is thirteen characters — so a
// delete could be confirmed by a sentence that was asking for something else.
//
// The rule: cut the message at its punctuation, and every piece that is left
// has to be a yes. A piece that is anything else means the person moved on, and
// what was waiting lapses rather than fires.
func isOnlyConsent(said string) bool {
	pieces := strings.FieldsFunc(said, func(r rune) bool {
		return strings.ContainsRune("、。，．,.!！?？\n", r)
	})
	found := false
	for _, piece := range pieces {
		piece = strings.TrimSpace(piece)
		if piece == "" {
			continue
		}
		if !isYesWord(piece) {
			return false
		}
		found = true
	}
	return found
}

// isYesWord reports whether one piece is a yes, politeness and all.
//
// The piece has to come apart into nothing but yes: a yes word, a yes wearing a
// tail ("はいです"), or two of them run together ("はいお願いします"). Anything
// left over that is not one of those makes it not a yes — which is what keeps
// "はいでも天気を見せて" out, punctuation or no punctuation.
//
// The length cap is a stop on the recursion below, not the rule: a yes is
// short, and the rule that decides is the decomposition, not the size. (The
// gate this replaces had the cap AS the rule, and a thirteen-character request
// walked straight through it.)
func isYesWord(piece string) bool {
	piece = strings.TrimSpace(piece)
	if piece == "" || len(piece) > 64 {
		return false
	}
	for _, yes := range goalYesWords {
		if piece == yes {
			return true
		}
	}
	for _, tail := range goalPoliteTails {
		if len(piece) > len(tail) && strings.HasSuffix(piece, tail) {
			if isYesWord(strings.TrimSuffix(piece, tail)) {
				return true
			}
		}
	}
	for _, yes := range goalYesWords {
		if len(piece) > len(yes) && strings.HasPrefix(piece, yes) {
			if isYesWord(piece[len(yes):]) {
				return true
			}
		}
	}
	return false
}

// answerGoalPending carries out whatever is waiting, when this message is the
// yes it was waiting for.
//
// The one place a yes is acted on. Two things put something here — a goal that
// found nothing and proposed a command (freeGoalAsk), and the agent reaching
// for something that cannot be undone (fmBrokerRun) — and both write the same
// sentence to the same key, so there is nothing to tell apart here.
//
// Not left to the model, in either case. The work is over by the time the
// question is asked and the agent was never told what was proposed: asked
// "はい" it has nothing to say yes to and says something agreeable instead.
//
// Returns whether the message was an answer to the offer at all. A message
// that is neither yes nor no is a new subject, and the offer lapses with it:
// leaving it armed meant a "はい" three questions later ran something the
// person had long stopped thinking about.
func answerGoalPending(ctx context.Context, want *Want, message string) (bool, string) {
	pending := strings.TrimSpace(GetCurrent(want, "goal_pending", ""))
	if pending == "" {
		return false, ""
	}
	// What was said, and only that: the server appends the asker's position
	// after a blank line, which is for the model and not part of the answer.
	said := strings.ToLower(strings.TrimSpace(message))
	if first, _, found := strings.Cut(said, "\n\n"); found {
		said = strings.TrimSpace(first)
	}
	// Addressed to the robot, so the name is not part of the answer.
	said = strings.TrimSpace(strings.TrimPrefix(said, "@robot"))
	clear := func() {
		want.SetCurrent("goal_pending", "")
		want.SetCurrent("pending_command", "")
	}
	for _, no := range goalNoWords {
		if strings.HasPrefix(said, no) {
			clear()
			return true, "やめておきます。"
		}
	}
	if !isOnlyConsent(said) {
		// A new subject. The offer is dropped and the message goes on to the
		// model as it would have anyway.
		clear()
		return false, ""
	}

	command, args := splitPendingCommand(pending)
	if command == "" {
		clear()
		return true, "何を実行するのか分からなくなりました。もう一度お願いします。"
	}
	clear()
	want.StoreLog("[FM_DO] Yes — running %s", pending)
	RecordCCActivity(want, CCActivityTool, truncateRunes("mywant "+command+" "+args, 90))
	out, failed := freeGoalRun(ctx, command, args)
	if failed {
		return true, "うまくいきませんでした: " + truncateRunes(strings.TrimSpace(firstLine(out)), 160)
	}
	done := strings.TrimSpace(out)
	if done == "" {
		done = "やりました。"
	}
	return true, truncateRunes(done, 300)
}

// splitPendingCommand takes the sentence that was written down — "mywant wants
// create --type weather …" — back apart into the command path the catalogue
// knows and the arguments after it.
//
// Against the WHOLE catalogue, destroying included. The goal loop's own menu
// leaves those out, because the loop may never choose one — but what is waiting
// for a yes is very often exactly one of them, and read back against that menu
// a deletion came apart into nothing: the person said yes and was told
// "何を実行するのか分からなくなりました".
func splitPendingCommand(sentence string) (command, args string) {
	rest := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(sentence), "mywant "))
	if rest == "" {
		return "", ""
	}
	catalogue, err := fmEveryCommand()
	if err != nil || len(catalogue) == 0 {
		return "", ""
	}
	// The longest path that this sentence starts with: "wants create" before
	// "wants", so the subcommand is not read as the first argument.
	for path := range catalogue {
		if !strings.HasPrefix(rest, path) {
			continue
		}
		after := strings.TrimSpace(strings.TrimPrefix(rest, path))
		if len(path) > len(command) {
			command, args = path, after
		}
	}
	return command, args
}
