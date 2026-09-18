package types

import (
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

// Working out what to do, here, where the board is.
//
// The on-device agent was being asked to do the whole job: understand the
// sentence, know the CLI, choose the order, fill the arguments, notice what
// came back, and stop at the right moment. It is a model with an 8k window and
// a hundred commands to choose from, and it showed: asked to delete one want it
// called `gui tile set` forty-eight times and then offered to drop a character.
//
// The knowledge it has that nothing here has is language. Everything else — the
// command list, what each one costs, what came back, whether to go on — belongs
// to this side, so that is where it lives now. The loop below asks the model one
// short question at a time ("what next?"), and every answer is checked against
// the CLI's own catalogue before anything runs.
//
// Three rules the loop keeps, all of them learned the hard way:
//
//   - A command that is not in the catalogue does not run. The model invents
//     plausible ones.
//   - A command that destroys is never run by this loop. It is written down and
//     the want waits for a person (phase: waiting_confirmation).
//   - The loop stops. max_steps, and a step whose command repeats the one before
//     it with the same arguments, both end it — a model that is stuck says the
//     same thing forever, cheerfully.

const freeGoalAgentName = "agent_free_goal"

func init() {
	RegisterWithInit(func() {
		RegisterDoAgent(freeGoalAgentName, executeFreeGoal)
	})
}

// freeGoalStep is one command and what it printed.
type freeGoalStep struct {
	Command string `json:"command"`
	Args    string `json:"args,omitempty"`
	Why     string `json:"why,omitempty"`
	Output  string `json:"output,omitempty"`
	Failed  bool   `json:"failed,omitempty"`
}

// freeGoalOutputLimit is how much of a command's output is kept and shown to
// the model. A want list is tens of KB; the model's whole world is 8k tokens.
const freeGoalOutputLimit = 1200

// executeFreeGoal carries out one request, step by step.
func executeFreeGoal(ctx context.Context, want *Want) error {
	request := GetCurrent(want, "request", "")
	if request == "" {
		return nil
	}
	// Once only. A do agent can be asked to run again — a later cycle, a
	// restart, a want being re-executed — and a goal is not a query: running it
	// twice moved a tile twice and wrote two undo entries for one move, the
	// second of which "restored" the tile to where it already was. A goal that
	// has started, finished, or is waiting for a person has nothing more to do
	// here.
	if phase := GetCurrent(want, "phase", "planning"); phase != "planning" {
		want.StoreLog("[FREE_GOAL] already %s, not running again", phase)
		return nil
	}
	binary, ok := fmToolPath()
	if !ok {
		// No on-device model here (a Linux server, or nothing installed). The
		// want says so rather than sitting at "planning" forever.
		want.SetCurrent("phase", "failed")
		want.SetCurrent("error", "このマシンには on-device モデルがないので、手順を立てられません")
		return nil
	}
	catalogue, err := freeGoalCatalogue()
	if err != nil || len(catalogue) == 0 {
		want.SetCurrent("phase", "failed")
		want.SetCurrent("error", fmt.Sprintf("mywant のコマンド一覧を読めませんでした: %v", err))
		return nil
	}

	maxSteps := GetCurrent(want, "max_steps", 4)
	if maxSteps <= 0 {
		maxSteps = 4
	}
	dryRun := GetCurrent(want, "dry_run", false)
	agent := fmServerFor(binary)
	timeout := time.Duration(fmDefaultTimeoutSeconds) * time.Second

	want.SetCurrent("phase", "running")
	want.StoreLog("[FREE_GOAL] %s", request)

	var steps []freeGoalStep
	for len(steps) < maxSteps {
		prompt := freeGoalPrompt(want, request, catalogue, steps)
		reply, err := agent.askPlain(prompt, "", timeout)
		if err != nil {
			want.SetCurrent("phase", "failed")
			want.SetCurrent("error", fmt.Sprintf("モデルが答えませんでした: %v", err))
			freeGoalStore(want, steps)
			return nil
		}

		command, args, answer := freeGoalParse(reply.Text, catalogue)
		if command == "" {
			// Nothing runnable came back. If it said something, that is the
			// answer — the job may simply have been a question.
			freeGoalFinish(want, steps, firstNonEmpty(answer, reply.Text))
			return nil
		}

		entry := catalogue[command]
		step := freeGoalStep{Command: command, Args: args}
		if entry.Risk == "destroy" {
			// Written down, not run. What is waiting is a sentence a person can
			// read and agree to, which is the only form of consent worth having.
			sentence := strings.TrimSpace("mywant " + command + " " + args)
			want.SetCurrent("pending_command", sentence)
			want.SetCurrent("phase", "waiting_confirmation")
			want.SetCurrent("answer", fmt.Sprintf("`%s` を実行すると元に戻せません。よろしければ「はい」と答えてください。", sentence))
			freeGoalStore(want, steps)
			want.StoreLog("[FREE_GOAL] waiting for a yes: %s", sentence)
			return nil
		}
		if repeatsLastStep(steps, step) {
			// The same command with the same arguments twice running means the
			// model has stopped learning from the answers.
			freeGoalFinish(want, steps, firstNonEmpty(answer, freeGoalSummary(steps)))
			return nil
		}
		if dryRun {
			step.Why = "dry_run: 実行していません"
			steps = append(steps, step)
			freeGoalFinish(want, steps, "計画だけ立てました (dry_run)")
			return nil
		}

		out, failed := freeGoalRun(ctx, command, args)
		step.Output, step.Failed = out, failed
		steps = append(steps, step)
		freeGoalStore(want, steps)
		want.StoreLog("[FREE_GOAL] ran %s %s -> %s", command, args, truncateRunes(out, 120))
	}

	// Out of steps: say what was done rather than nothing at all.
	freeGoalFinish(want, steps, freeGoalSummary(steps))
	return nil
}

// freeGoalPrompt asks for one thing: the next command, or an answer.
//
// The catalogue rides in the first question only — the agent keeps the session,
// so repeating a hundred command names every step would fill the window with
// what it has already been told.
func freeGoalPrompt(want *Want, request string, catalogue map[string]freeGoalCommand, steps []freeGoalStep) string {
	var b strings.Builder
	if len(steps) == 0 {
		b.WriteString("You are working out how to carry out one request on a MyWant board, one command at a time.\n\n")
		b.WriteString("Commands you may use:\n")
		b.WriteString(freeGoalMenu(catalogue))
		// The first step is always a command, never an answer. Left free to
		// choose, the model answered "荻窪は丙座です" — a constellation that does
		// not exist, about a board it had not looked at. It has no knowledge of
		// this board at all; everything it can truthfully say has to be read
		// first, and reading is what these commands are for.
		b.WriteString("\nAnswer with ONE line and nothing else:\n")
		b.WriteString("  RUN <command path> | <arguments>\n")
		b.WriteString("Arguments are only what the command names — a name, or a name then numbers.\n")
		b.WriteString("You know nothing about this board yet, so you cannot answer yet: choose the command that\n")
		b.WriteString("finds out. `board` names everything on the canvas; `relations` says what one is connected to.\n\n")
		if context := GetCurrent(want, "goal_context", ""); context != "" {
			b.WriteString("Where the person is: " + context + "\n")
		}
		b.WriteString("Request: " + request + "\n")
		return b.String()
	}

	b.WriteString("What has been run so far:\n")
	for _, s := range steps {
		b.WriteString("- mywant " + strings.TrimSpace(s.Command+" "+s.Args) + "\n")
		b.WriteString("  -> " + truncateRunes(strings.TrimSpace(s.Output), 400) + "\n")
	}
	b.WriteString("\nRequest: " + request + "\n")
	b.WriteString("One line only, and nothing else:\n")
	b.WriteString("  RUN <command path> | <arguments>   — if something still has to be done or looked up\n")
	b.WriteString("  ANSWER <what to tell the person, in their language>   — if the request is carried out\n")
	b.WriteString("Say only what the output above actually shows. Do not invent names, and never answer with\n")
	b.WriteString("the raw output. If the request asks for something to be CHANGED, it is not carried out until\n")
	b.WriteString("you have RUN the command that changes it — looking is not doing.\n")
	return b.String()
}

// freeGoalParse reads the model's line: a command to run, or an answer.
//
// Lenient on purpose. The model wraps its line in politeness, backticks and
// occasionally a numbered list, and the useful part is still in there: the
// longest command path from the catalogue that the text actually contains.
func freeGoalParse(text string, catalogue map[string]freeGoalCommand) (command, args, answer string) {
	trimmed := strings.TrimSpace(text)
	for _, line := range strings.Split(trimmed, "\n") {
		line = strings.TrimSpace(strings.Trim(strings.TrimSpace(line), "`*-# "))
		upper := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(upper, "RUN "):
			rest := strings.TrimSpace(line[4:])
			path, rest := longestCommandIn(rest, catalogue)
			if path == "" {
				continue
			}
			return path, fitArgs(path, cleanArgs(rest), catalogue), ""
		case strings.HasPrefix(upper, "ANSWER"):
			return "", "", strings.TrimSpace(strings.TrimPrefix(line[6:], ":"))
		}
	}
	// No form at all: if a command path is in there somewhere, take it; the
	// model often answers "I will run wants list" and means it.
	if path, rest := longestCommandIn(trimmed, catalogue); path != "" {
		return path, fitArgs(path, cleanArgs(rest), catalogue), ""
	}
	return "", "", trimmed
}

// longestCommandIn finds the catalogue command a line names, and returns what
// follows it.
//
// From the front, not from anywhere in the line. Searching anywhere read
// "RUN board | i char set <characterId> <x> <y>" — a line meaning `gui i char
// set`, with the menu's own usage text copied in after it — as the command
// `board` with arguments "i char set …", and ran that. The command is the first
// thing on the line; the longest path it starts with is which one.
func longestCommandIn(line string, catalogue map[string]freeGoalCommand) (string, string) {
	rest := strings.TrimSpace(line)
	rest = strings.TrimPrefix(rest, "mywant ")
	rest = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(rest), "|"))

	best := ""
	for path := range catalogue {
		if !strings.HasPrefix(rest, path) {
			continue
		}
		// A prefix has to end where a word ends: "wants list" must not match a
		// line beginning "wants listen".
		after := rest[len(path):]
		if after != "" && !strings.ContainsAny(after[:1], " |\t") {
			continue
		}
		if len(path) > len(best) {
			best = path
		}
	}
	if best == "" {
		return "", ""
	}
	return best, rest[len(best):]
}

// cleanArgs takes the arguments out of whatever the line wrapped them in.
func cleanArgs(rest string) string {
	out := strings.TrimSpace(rest)
	out = strings.TrimPrefix(out, "|")
	out = strings.Trim(out, " `\"'")
	// Parentheses and commas are how a person writes a cell — "(9, -9)" — and
	// the CLI wants two words. Only the punctuation goes; the numbers stay.
	out = strings.NewReplacer("(", " ", ")", " ", "（", " ", "）", " ", ",", " ", "、", " ").Replace(out)
	return strings.Join(strings.Fields(out), " ")
}

// fitArgs keeps only the arguments the command actually takes.
//
// The menu lists each command with its usage, and the model copies from it:
// asked to move something it answered "RUN board | i char set <characterId> <x>
// <y>" — the command it chose, followed by a different command's usage — and
// `board` was run with four words it has no use for. The usage line says how
// many values a command names, so anything past that, and anything still
// wearing its angle brackets, is not an argument.
func fitArgs(command, args string, catalogue map[string]freeGoalCommand) string {
	if args == "" {
		return ""
	}
	placeholders := strings.Count(catalogue[command].Use, "<") + strings.Count(catalogue[command].Use, "[")
	var kept []string
	values := 0
	for _, token := range strings.Fields(args) {
		if strings.ContainsAny(token, "<>") {
			continue // usage text, not a value
		}
		if strings.HasPrefix(token, "--") {
			kept = append(kept, token)
			continue
		}
		if values >= placeholders {
			continue
		}
		kept = append(kept, token)
		values++
	}
	return strings.Join(kept, " ")
}

// freeGoalRun runs one command through the CLI, and reports whether it failed.
func freeGoalRun(ctx context.Context, command, args string) (string, bool) {
	binary, err := mywantBinaryPath()
	if err != nil {
		return fmt.Sprintf("mywant が見つかりません: %v", err), true
	}
	argv := strings.Fields(command)
	if args != "" {
		argv = append(argv, strings.Fields(args)...)
	}
	runCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	out, err := exec.CommandContext(runCtx, binary, argv...).CombinedOutput()
	text := strings.TrimSpace(string(out))
	if len(text) > freeGoalOutputLimit {
		text = text[:freeGoalOutputLimit] + "\n...(truncated)"
	}
	if err != nil {
		if text == "" {
			text = err.Error()
		}
		return text, true
	}
	return text, false
}

// freeGoalCommand is one command as the CLI describes it.
type freeGoalCommand struct {
	Path  string `json:"path"`
	Short string `json:"short"`
	Use   string `json:"use"`
	Risk  string `json:"risk"`
}

// The groups a goal on this board has business in. Reads are all allowed;
// writing is confined to what the board is made of, because the same CLI also
// installs plugins and rewrites config, and neither is canvas work.
var freeGoalWriteGroups = map[string]bool{
	"wants": true, "thing": true, "world": true, "state": true, "gui": true, "undo": true,
}

// freeGoalCatalogue is what the CLI says it can do, asked once per process.
//
// From the binary, not from a list kept here: a command added to the CLI is
// usable by a goal the next time this server starts, with nothing to change.
var freeGoalCatalogueCache map[string]freeGoalCommand

func freeGoalCatalogue() (map[string]freeGoalCommand, error) {
	if freeGoalCatalogueCache != nil {
		return freeGoalCatalogueCache, nil
	}
	binary, err := mywantBinaryPath()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, binary, "commands", "--json").Output()
	if err != nil {
		return nil, err
	}
	var all []freeGoalCommand
	if err := json.Unmarshal(out, &all); err != nil {
		return nil, err
	}
	catalogue := map[string]freeGoalCommand{}
	for _, c := range all {
		if c.Path == "commands" || c.Path == "" {
			continue
		}
		group := strings.Fields(c.Path)[0]
		switch c.Risk {
		case "read":
			catalogue[c.Path] = c
		case "change", "destroy":
			if freeGoalWriteGroups[group] && c.Path != "gui start" && c.Path != "gui stop" {
				catalogue[c.Path] = c
			}
		}
	}
	freeGoalCatalogueCache = catalogue
	return catalogue, nil
}

// freeGoalMenu is the catalogue as the model sees it: the path, what it takes,
// and what it is for. Sorted, so two runs read the same.
//
// With the descriptions, because without them it is a list of words: asked to
// move a thing, a model given only paths read `thing pin` as unrelated and
// replied that an album "is not a location that can be physically moved". The
// descriptions cost about a quarter of the window in a question that is asked
// once and answered without tools — the room is there, and this is what it is
// for.
func freeGoalMenu(catalogue map[string]freeGoalCommand) string {
	paths := make([]string, 0, len(catalogue))
	for path := range catalogue {
		paths = append(paths, path)
	}
	sortStrings(paths)
	var b strings.Builder
	for _, path := range paths {
		c := catalogue[path]
		takes := ""
		if fields := strings.Fields(c.Use); len(fields) > 1 {
			takes = " " + strings.Join(fields[1:], " ")
		}
		b.WriteString("  " + path + takes)
		if short := truncateRunes(strings.TrimSpace(c.Short), 64); short != "" {
			b.WriteString("  — " + short)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// mywantBinaryPath finds this CLI: the one running the server, then PATH, then
// where `make install` puts it.
func mywantBinaryPath() (string, error) {
	if custom := strings.TrimSpace(os.Getenv("MYWANT_BIN")); custom != "" {
		if info, err := os.Stat(custom); err == nil && !info.IsDir() {
			return custom, nil
		}
	}
	if found, err := exec.LookPath("mywant"); err == nil {
		return found, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	candidate := filepath.Join(home, ".local", "bin", "mywant")
	if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
		return candidate, nil
	}
	return "", fmt.Errorf("mywant not found on PATH")
}

// freeGoalStore keeps the steps where anybody can see them: the want's own
// state, which is what the card shows.
func freeGoalStore(want *Want, steps []freeGoalStep) {
	stored := make([]any, 0, len(steps))
	for _, s := range steps {
		stored = append(stored, map[string]any{
			"command": strings.TrimSpace(s.Command + " " + s.Args),
			"output":  truncateRunes(s.Output, 400),
			"failed":  s.Failed,
		})
	}
	want.SetCurrent("steps", stored)
}

// freeGoalFinish records the answer, says it out loud, and marks the goal done.
//
// Out loud because somebody asked: a goal started by a person talking to the
// robot is answered where they are looking, over the robot's own tile. A goal
// started from a terminal has nobody standing on the board to tell.
func freeGoalFinish(want *Want, steps []freeGoalStep, answer string) {
	freeGoalStore(want, steps)
	answer = strings.TrimSpace(answer)
	want.SetCurrent("answer", answer)
	want.SetCurrent("phase", "done")
	want.StoreLog("[FREE_GOAL] done: %s", truncateRunes(answer, 160))
	if GetCurrent(want, "asked_by", "") != "" && answer != "" {
		freeGoalSay(answer)
	}
}

// freeGoalSay puts the answer over the robot's tile, through the same command
// anybody else would use.
func freeGoalSay(answer string) {
	binary, err := mywantBinaryPath()
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_ = exec.CommandContext(ctx, binary, "wants", "say", "robot", truncateRunes(answer, 300)).Run()
}

// freeGoalSummary is the fallback answer: what was run, when the model never
// got around to saying it.
func freeGoalSummary(steps []freeGoalStep) string {
	if len(steps) == 0 {
		return "何もしませんでした"
	}
	var parts []string
	for _, s := range steps {
		parts = append(parts, strings.TrimSpace(s.Command+" "+s.Args))
	}
	return "実行しました: " + strings.Join(parts, " / ")
}

func repeatsLastStep(steps []freeGoalStep, step freeGoalStep) bool {
	if len(steps) == 0 {
		return false
	}
	last := steps[len(steps)-1]
	return last.Command == step.Command && last.Args == step.Args
}

func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}
