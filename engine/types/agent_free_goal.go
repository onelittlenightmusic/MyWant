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

	// No plan is asked for, and that is a measurement rather than a
	// preference.
	//
	// Asked to lay out the steps first, this model plans confidently and
	// wrongly, and then follows its own plan: told to move a tile it planned
	// around `gui i set` and spent every step there, where the same model with
	// no plan picks `thing pin` and is done in one. Same for handing it the
	// board's contents up front — it planned around the nearest familiar name
	// rather than the one in the request. An 8k on-device model does better
	// answering the small question in front of it than the large one about
	// what it is going to do.
	//
	// What that costs is the multi-step goal: "know X" where X does not exist
	// yet needs a look, a create and a read, and it will do the look and stop.
	// Ask for the create in so many words and it does that in one step. The
	// trade is deliberate — see the commit that removed the planning call.

	var steps []freeGoalStep
	emptyTurns := 0
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
		if command == "" && strings.TrimSpace(answer) == "" {
			answer = strings.TrimSpace(reply.Text)
		}
		// An answer before anything has been looked at is not an answer.
		//
		// Asked "荻窪はどの星座？" this model will happily reply "荻窪は北辰座
		// です" — a constellation it made up — without running a command,
		// because it is a language model and the question sounds answerable.
		// Nothing here says which command to run; it says that a first turn
		// has to run one. What is on this board is not in the model.
		if command == "" && len(steps) == 0 {
			answer = ""
		}
		if command == "" && strings.TrimSpace(answer) == "" {
			// Nothing usable came back. Ask again — twice — rather than
			// ending a request on an unreadable reply.
			emptyTurns++
			if emptyTurns < 3 {
				continue
			}
			if command, args := freeGoalPropose(agent, request, catalogue, steps, timeout); command != "" {
				freeGoalAsk(want, steps, command, args, "missing")
				return nil
			}
			freeGoalFinish(want, steps, freeGoalSummary(steps))
			return nil
		}
		emptyTurns = 0
		if command == "" {
			// It said something instead of running something. If anything has
			// worked, that sentence is the answer and the job is done.
			//
			// If nothing has, the sentence is an apology — "値は未知的です" —
			// and an apology is the wrong last word when there is a command
			// that would have worked. Offer that instead and wait for a
			// person: see freeGoalPropose, and freeGoalAsk for the waiting.
			if !anyStepWorked(steps) {
				if next, nextArgs := freeGoalPropose(agent, request, catalogue, steps, timeout); next != "" {
					freeGoalAsk(want, steps, next, nextArgs, "missing")
					return nil
				}
			}
			freeGoalFinish(want, steps, answer)
			return nil
		}

		entry := catalogue[command]
		// A want type is a hundred-odd names this model has never seen, and it
		// guesses: asked for a weather want it wrote `--type aura`, which is a
		// real type and the wrong one, and the board got an aura called
		// "NakanoのWeather". The names are knowable, so they are looked up and
		// the choice is put as its own small question rather than left to
		// memory. See freeGoalFixType.
		if strings.HasPrefix(command, "wants ") && flagNeedsWantType(entry) {
			args = freeGoalFixType(agent, request, args, timeout)
			// A want that is made without its parameters is made for nowhere:
			// a weather want with no `at` reads Tokyo, whatever was asked for.
			// The type declares what it takes, so the values are the only part
			// worth asking about — one question, one line per parameter.
			args = freeGoalFillParams(agent, request, args, timeout)
		}
		step := freeGoalStep{Command: command, Args: args}
		if entry.Risk == "destroy" {
			// Written down, not run. What is waiting is a sentence a person
			// can read and agree to, which is the only form of consent worth
			// having.
			freeGoalAsk(want, steps, command, args, "destroy")
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
		// A want that was just made is not finished being made. `wants create`
		// answers the moment the want exists, and the thing it was created for
		// — the weather, the unread count — arrives seconds later. Asked to
		// KNOW something, a goal that stopped here would report having made a
		// want and never say what it found out.
		if !failed && command == "wants create" {
			if result := freeGoalAwaitResult(ctx, args); result != "" {
				out += "\n" + result
			}
		}
		step.Output, step.Failed = out, failed
		steps = append(steps, step)
		freeGoalStore(want, steps)
		want.StoreLog("[FREE_GOAL] ran %s %s -> %s", command, args, truncateRunes(out, 120))
	}

	// Out of steps. If none of them worked, the useful last word is not a
	// list of failures but the command that would have done it — put to the
	// person, who can say yes.
	if !anyStepWorked(steps) {
		if command, args := freeGoalPropose(agent, request, catalogue, steps, timeout); command != "" {
			freeGoalAsk(want, steps, command, args, "missing")
			return nil
		}
	}
	freeGoalFinish(want, steps, freeGoalSummary(steps))
	return nil
}

// freeGoalPrompt asks for one thing: the next command, or an answer.
//
// The whole question every time — menu, what has been run, the request. The
// planning call is answered by the model alone, on a session made and dropped
// for that one question (see askPlain), so nothing carries over between steps:
// the first version sent the command list only in the first prompt, and from
// the second step the model was choosing from a list it could no longer see. It
// did the only thing it could and repeated the command it had just run.
func freeGoalPrompt(want *Want, request string, catalogue map[string]freeGoalCommand, steps []freeGoalStep) string {
	var b strings.Builder
	b.WriteString("You are carrying out one request on a MyWant board, one command at a time.\n\n")
	b.WriteString("Commands you may use:\n")
	b.WriteString(freeGoalMenu(catalogue))

	if len(steps) > 0 {
		b.WriteString("\nWhat has been run so far:\n")
		for _, s := range steps {
			b.WriteString("- mywant " + strings.TrimSpace(s.Command+" "+s.Args) + "\n")
			// Said in a word, because the model reads a failure as a result
			// otherwise: it ran a create that the CLI refused for a missing
			// flag, was shown the refusal, and reported the want as made.
			mark := "  -> "
			if s.Failed {
				mark = "  -> FAILED: "
			}
			b.WriteString(mark + truncateRunes(strings.TrimSpace(s.Output), 400) + "\n")
		}
	}

	if context := GetCurrent(want, "goal_context", ""); context != "" {
		b.WriteString("\nWhere the person is: " + context + "\n")
	}
	b.WriteString("\nRequest: " + request + "\n\n")

	// What this board is, said plainly, because it is the part a model cannot
	// infer from a command list: MyWant answers questions by HAVING wants. A
	// weather want for a place holds that place's weather and keeps it
	// current. So "I want to know X" and "I want to make a thing that knows X"
	// are the same request here, and the only difference is whether the want
	// already exists.
	b.WriteString("How this board works: a want holds an answer and keeps it up to date — a weather want\n")
	b.WriteString("for a place holds that place's weather. So wanting to KNOW something and wanting to MAKE\n")
	b.WriteString("the want that knows it are the same request: look for a want that already provides it\n")
	b.WriteString("('board' / 'wants list'), read its value with 'wants get' if there is one, and create it\n")
	b.WriteString("with 'wants create' if there is not. Answer with the value, not with what you ran.\n")
	b.WriteString("'wants get <name>' is how you read what a want holds. 'point' only shows WHERE something\n")
	b.WriteString("stands — it never tells you what it knows, so it is no use for a question about a value.\n\n")

	if len(steps) == 0 {
		// The first step is always a command, never an answer. Left free to
		// choose, the model answered "荻窪は丙座です" — a constellation that does
		// not exist, about a board it had not looked at. It knows nothing about
		// this board; everything it can truthfully say has to be read first.
		b.WriteString("Answer with ONE line and nothing else:\n")
		b.WriteString("  RUN <command path> | <arguments>\n")
		b.WriteString("You know nothing about this board yet, so you cannot answer yet: choose the command\n")
		b.WriteString("that finds out, or the one that does what was asked.\n")
	} else {
		b.WriteString("Answer with ONE line and nothing else:\n")
		b.WriteString("  RUN <command path> | <arguments>   — if anything still has to be done or looked up\n")
		b.WriteString("  ANSWER <what to tell the person, in their language>   — only once it is done\n")
		// Two kinds of failure, and they call for opposite things. The first
		// version said only "run it again, corrected", which told a goal that
		// had just learned a want does not exist to go and look for it a
		// second time — instead of creating it, which was the whole request.
		b.WriteString("A command that FAILED because its arguments were wrong (usage, unknown flag, missing\n")
		b.WriteString("value) should be run again, corrected — not repeated unchanged, not abandoned.\n")
		// No sentence in quotes here, ever. This model reads a quoted example
		// as a thing to say: given one, it answered "it is not there" to a
		// question about constellations, to a request to move a tile, and to
		// a want whose weather it had already read. Rules, not phrasings.
		b.WriteString("A command that FAILED because what it looked for does not exist has answered you:\n")
		b.WriteString("do not look for it again.\n")
		b.WriteString("Otherwise never repeat a command that has already been run above.\n")
		b.WriteString("ANSWER reports what has been DONE. If the request asked for something to be made,\n")
		b.WriteString("moved, connected or removed and no command above has done it, you have not finished:\n")
		b.WriteString("RUN the command that does it, using the exact names the output above showed.\n")
	}
	b.WriteString("Arguments are only what the command needs — a name, numbers, or its flags.\n")
	b.WriteString("A flag is written --name value, one you actually need; never copy a command's whole\n")
	b.WriteString("flag list from the menu.\n")
	b.WriteString("Say only what the output above actually shows; do not invent names. The names of things\n")
	b.WriteString("and wants on the board come from 'board'; the names of want TYPES come from 'types list'.\n")
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
	entry := catalogue[command]
	placeholders := strings.Count(entry.Use, "<") + strings.Count(entry.Use, "[")
	flags := entry.flagsByName()

	var kept []string
	values := 0
	expectFlagValue := false
	for _, token := range strings.Fields(args) {
		if strings.ContainsAny(token, "<>") {
			continue // usage text, not a value
		}
		// Words out of the prompt rather than out of the request: the step
		// list marks a failure with "FAILED:", and that word came back as an
		// argument the very next turn.
		if strings.Contains(token, "FAILED") || strings.Trim(token, "-—–>|:") == "" {
			continue
		}
		// The word after a flag that takes one belongs to it: "--param
		// at=Nakano" is a flag and its value, and counting that value against
		// the command's positional arguments threw it away — `wants create`
		// names none, so the parameter went missing and a weather want was
		// created for nowhere.
		if expectFlagValue {
			kept = append(kept, token)
			expectFlagValue = false
			continue
		}
		if strings.HasPrefix(token, "--") {
			name := strings.TrimPrefix(strings.SplitN(token, "=", 2)[0], "--")
			f, known := flags[name]
			// A flag the command does not have is not an argument, it is
			// something the model read somewhere else: "--type]" came out of
			// the menu's own punctuation and the CLI refused the whole line
			// for it.
			if !known {
				continue
			}
			kept = append(kept, token)
			if f.Type != "bool" && !strings.Contains(token, "=") {
				expectFlagValue = true
			}
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

// flagNeedsWantType reports whether this command takes a --type that means a
// want type.
func flagNeedsWantType(entry freeGoalCommand) bool {
	_, ok := entry.flagsByName()["type"]
	return ok
}

// freeGoalFixType makes sure a --type is one the server actually has.
//
// Asked as its own question, with the whole list in front of the model and
// nothing else in the prompt: choosing one name from a list is the kind of
// thing a small model is good at, and composing a command line while
// remembering a hundred type names is not.
func freeGoalFixType(agent *fmServer, request, args string, timeout time.Duration) string {
	types := freeGoalWantTypes()
	if len(types) == 0 {
		return args
	}
	known := map[string]bool{}
	for _, t := range types {
		known[t] = true
	}

	fields := strings.Fields(args)
	current, at := "", -1
	for i, token := range fields {
		if token == "--type" && i+1 < len(fields) {
			current, at = fields[i+1], i+1
		}
	}
	if current != "" && known[current] {
		return args
	}

	prompt := "Which want type does this request need? Answer with ONE name from this list and nothing else.\n\n" +
		strings.Join(types, ", ") + "\n\nRequest: " + request + "\n"
	reply, err := agent.askPlain(prompt, "", timeout)
	if err != nil {
		return args
	}
	chosen := ""
	for _, word := range strings.Fields(strings.ToLower(reply.Text)) {
		word = strings.Trim(word, "`\"'.,:;()[]")
		if known[word] {
			chosen = word
			break
		}
	}
	if chosen == "" {
		return args
	}
	if at >= 0 {
		fields[at] = chosen
		return strings.Join(fields, " ")
	}
	return strings.TrimSpace(args + " --type " + chosen)
}

// freeGoalParam is one parameter a want type declares.
type freeGoalParam struct {
	Name        string `json:"name"`
	SubType     string `json:"subType"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

// freeGoalFillParams gives a new want the values it is being made for.
//
// The type says what it takes; the request says what it is for. Neither is
// something to guess at, so the question put to the model is small and
// closed: here are the parameters this type declares, here is what was asked
// for, what is each one? Secrets are never asked about — they come from the
// environment, and a model inventing an API key would be worse than an empty
// one.
func freeGoalFillParams(agent *fmServer, request, args string, timeout time.Duration) string {
	fields := strings.Fields(args)
	wantType := ""
	for i, token := range fields {
		if token == "--type" && i+1 < len(fields) {
			wantType = fields[i+1]
		}
	}
	if wantType == "" || strings.Contains(args, "--param") {
		return args
	}
	params := freeGoalTypeParams(wantType)
	var askable []freeGoalParam
	for _, p := range params {
		if p.SubType == "secret" || p.Name == "" {
			continue
		}
		askable = append(askable, p)
	}
	if len(askable) == 0 {
		return args
	}

	var b strings.Builder
	// The form is shown with the real names filled in, because "answer as
	// name=value" was read as literally that: the model replied
	// "name=荻窪のWeather", the line was discarded, and the want was created
	// with no parameters and read Tokyo's weather under the name 荻窪のWeather.
	b.WriteString("A want of type \"" + wantType + "\" is being created for this request:\n")
	b.WriteString(request + "\n\nFill in the lines below, one per line, and write nothing else.\n")
	b.WriteString("Use only what the request actually says; delete any line it says nothing about.\n\n")
	for _, p := range askable {
		b.WriteString("  " + p.Name + "=       (" + truncateRunes(strings.TrimSpace(p.Description), 80) + ")\n")
	}
	declared := map[string]bool{}
	for _, p := range askable {
		declared[p.Name] = true
	}

	// Twice, if the first answer parses to nothing. A want is created once and
	// then the name is taken: the first attempt at this came back as prose,
	// the want was made for Tokyo, and the correction could only fail with
	// "already exists". A second question costs three seconds and is asked
	// before anything exists.
	for attempt := 0; attempt < 2; attempt++ {
		reply, err := agent.askPlain(b.String(), "", timeout)
		if err != nil {
			return args
		}
		out := args
		for _, line := range strings.Split(reply.Text, "\n") {
			line = strings.TrimSpace(strings.Trim(strings.TrimSpace(line), "`*-• "))
			// "at=荻窪" was asked for and "at: 荻窪" came back, which was
			// dropped for want of an equals sign — so the want was created
			// with no parameters at all and read Tokyo's weather under the
			// name 荻窪のWeather. Either separator; the answer is the same.
			name, value, found := strings.Cut(line, "=")
			if !found {
				name, value, found = strings.Cut(line, ":")
			}
			// A bare value, when there is only one thing it could be. Shown
			// the line "at=" to fill in, the model answers "荻窪" — which is
			// the whole of what was asked for, and was thrown away for not
			// repeating the name back.
			if !found && len(askable) == 1 && line != "" && len([]rune(line)) <= 60 {
				name, value, found = askable[0].Name, line, true
			}
			name, value = strings.TrimSpace(name), strings.Trim(strings.TrimSpace(value), "\"'")
			if !found || !declared[name] || value == "" || strings.ContainsAny(value, " <>") {
				continue
			}
			out += " --param " + name + "=" + freeGoalKnownThing(value)
		}
		if out != args {
			return out
		}
	}
	return args
}

// freeGoalKnownThing answers with the board's own spelling of a value.
//
// "NakanoのWeather" gives "Nakano"; the board has been calling that place
// "nakano" since somebody typed it that way. Same word, two spellings, and the
// canvas draws a road between a want and a thing only when they match — so a
// new want would stand next to the place it is about, unconnected.
func freeGoalKnownThing(value string) string {
	binary, err := mywantBinaryPath()
	if err != nil {
		return value
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, binary, "board", "--json").Output()
	if err != nil {
		return value
	}
	var entries []struct {
		Name string `json:"name"`
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(out, &entries); err != nil {
		return value
	}
	for _, e := range entries {
		if e.Kind == "thing" && strings.EqualFold(e.Name, value) {
			return e.Name
		}
	}
	return value
}

// freeGoalTypeParams is what one want type declares it takes.
func freeGoalTypeParams(wantType string) []freeGoalParam {
	builder := GetGlobalChainBuilder()
	if builder == nil {
		return nil
	}
	def := builder.GetWantTypeDefinition(wantType)
	if def == nil {
		return nil
	}
	var params []freeGoalParam
	for _, p := range def.Parameters {
		params = append(params, freeGoalParam{
			Name:        p.Name,
			SubType:     p.SubType,
			Description: p.Description,
			Required:    p.Required,
		})
	}
	return params
}

// freeGoalWantTypes is every want type the server knows.
//
// From the running builder, not from a subprocess: this agent runs inside the
// server that loaded them.
func freeGoalWantTypes() []string {
	builder := GetGlobalChainBuilder()
	if builder == nil {
		return nil
	}
	var names []string
	for name := range builder.AllWantTypeDefinitions() {
		names = append(names, name)
	}
	sortStrings(names)
	return names
}

// freeGoalAwaitResult waits a little for a newly created want to have
// something to say, and reports it.
//
// A little: long enough for the things a want does by itself (a weather fetch
// took five seconds), short enough that a goal does not sit on a want that
// waits for a person.
func freeGoalAwaitResult(ctx context.Context, args string) string {
	name := ""
	fields := strings.Fields(args)
	for i, token := range fields {
		if token == "--name" && i+1 < len(fields) {
			name = fields[i+1]
		}
	}
	if name == "" {
		return ""
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ""
		case <-time.After(2 * time.Second):
		}
		builder := GetGlobalChainBuilder()
		if builder == nil {
			return ""
		}
		for _, want := range builder.GetAllWantStates() {
			if want == nil || want.Metadata.Name != name {
				continue
			}
			// final_result is where a want puts what it is for — see
			// FinalResultField in the want type.
			if result := strings.TrimSpace(fmt.Sprint(want.GetAllState()["final_result"])); result != "" && result != "<nil>" {
				return "The want " + name + " now holds: " + truncateRunes(result, 200)
			}
		}
	}
	return ""
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
	Path  string         `json:"path"`
	Short string         `json:"short"`
	Use   string         `json:"use"`
	Risk  string         `json:"risk"`
	Flags []freeGoalFlag `json:"flags"`
}

// freeGoalFlag is one option a command takes. The type matters: a boolean flag
// stands alone, and every other kind swallows the word after it.
type freeGoalFlag struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// flagsByName indexes a command's flags for the argument fitting below.
func (c freeGoalCommand) flagsByName() map[string]freeGoalFlag {
	out := make(map[string]freeGoalFlag, len(c.Flags))
	for _, f := range c.Flags {
		out[f.Name] = f
	}
	return out
}

// The groups a goal on this board has business in. Reads are all allowed;
// writing is confined to what the board is made of, because the same CLI also
// installs plugins and rewrites config, and neither is canvas work.
var freeGoalWriteGroups = map[string]bool{
	"wants": true, "thing": true, "world": true, "state": true, "undo": true,
}

// freeGoalWritePaths are single commands allowed in from a group that is
// otherwise not a goal's business.
//
// `gui tile set` moves a want's tile, which is board work. The rest of `gui`
// drives the viewer's screen — where the cursor is, which page is open — and
// offering it to a goal asked to MOVE something cost every attempt: `gui i
// set --x 9 --y -9` reads exactly like the answer, moves the cursor, and
// leaves the tile where it was.
var freeGoalWritePaths = map[string]bool{
	"gui tile set": true,
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
			if freeGoalWriteGroups[group] || freeGoalWritePaths[c.Path] {
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
		// The flags, because for some commands they are the whole of the
		// instruction: `wants create` takes no positional argument at all, and
		// a menu that showed only its name said nothing about --type or
		// --param — so a goal asking for "a weather want for Nakano" could not
		// have expressed it. --json is left out; it changes the shape of the
		// answer and never the work.
		// Only where the flags ARE the command. `wants create` names no
		// positional argument and everything it needs is a flag, so without
		// them it cannot be used at all. Everywhere else they doubled the
		// length of the menu, and a longer menu measurably costs picks: with
		// flags on every line, "荻窪はどの星座？" stopped running `relations`
		// and started inventing constellations.
		var options []string
		if strings.ContainsAny(c.Use, "<[") {
			b.WriteString("  " + path + takes)
			if short := truncateRunes(strings.TrimSpace(c.Short), 64); short != "" {
				b.WriteString("  — " + short)
			}
			b.WriteString("\n")
			continue
		}
		for _, f := range c.Flags {
			// Eight, not five: flags are listed alphabetically, and five cut
			// `wants create` off at --name — losing --param and --type, which
			// are the entire instruction for creating anything. A menu that
			// stops before the useful flag is worse than no menu.
			if f.Name == "json" || f.Name == "help" || len(options) >= 8 {
				continue
			}
			options = append(options, f.Name)
		}
		// Named, not written out as a command line. Printed as "[--type
		// --param …]" the model copied the whole bracket into its arguments
		// and ran `wants create --example --file --interactive --name --param
		// --type]`. A list of names cannot be pasted as a line.
		if len(options) > 0 {
			b.WriteString(" (flags: " + strings.Join(options, ", ") + ")")
		}
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

// freeGoalAsk stops the goal and puts one command to the person.
//
// Two things end a goal without finishing it, and both are answered the same
// way: a command that cannot be undone, and a request that cannot be carried
// out with what is on the board. Neither is a decision for a model or for this
// code — "「荻窪のWeather」はまだありません" is a fact, and "作りますか？" is a
// question — so the command is written down, the goal waits, and a person says.
func freeGoalAsk(want *Want, steps []freeGoalStep, command, args, why string) {
	sentence := strings.TrimSpace("mywant " + command + " " + args)
	want.SetCurrent("pending_command", sentence)
	want.SetCurrent("pending_reason", why)
	want.SetCurrent("phase", "waiting_confirmation")
	if why == "destroy" {
		want.SetCurrent("answer", fmt.Sprintf("`%s` は元に戻せません。実行しますか？", sentence))
	} else {
		want.SetCurrent("answer", fmt.Sprintf("まだありません。`%s` で作れます。実行しますか？", sentence))
	}
	freeGoalStore(want, steps)
	want.StoreLog("[FREE_GOAL] waiting for a yes (%s): %s", why, sentence)
}

// freeGoalPropose asks what would carry the request out, when nothing has.
//
// One question, asked only at the end: the goal has looked and come up empty,
// and the useful thing to say is not "no" but "this would do it — shall I?".
// The model names the command; it is checked against the catalogue and filled
// in like any other, and then it waits. Nothing is run.
func freeGoalPropose(agent *fmServer, request string, catalogue map[string]freeGoalCommand, steps []freeGoalStep, timeout time.Duration) (string, string) {
	var b strings.Builder
	b.WriteString("A request on a MyWant board could not be carried out with what is already there.\n\n")
	b.WriteString("Request: " + request + "\n")
	if len(steps) > 0 {
		b.WriteString("\nWhat was tried:\n")
		for _, s := range steps {
			b.WriteString("- mywant " + strings.TrimSpace(s.Command+" "+s.Args) + "\n")
			b.WriteString("  -> " + truncateRunes(strings.TrimSpace(s.Output), 200) + "\n")
		}
	}
	b.WriteString("\nCommands:\n")
	b.WriteString(freeGoalMenu(catalogue))
	b.WriteString("\nName the ONE command that would carry the request out, as:\n")
	b.WriteString("  RUN <command path> | <arguments>\n")
	b.WriteString("Nothing will be run: it will be put to the person, who decides. If no command would\n")
	b.WriteString("help, write NONE.\n")

	// Twice at most, and never a command that has already been tried. Asked
	// what would carry the request out, the model's first answer was the
	// lookup that had just failed — an offer to do the thing that did not
	// work, which is worse than no offer.
	tried := map[string]bool{}
	for _, s := range steps {
		tried[strings.TrimSpace(s.Command+" "+s.Args)] = true
	}
	question := b.String()
	var command, args string
	for attempt := 0; attempt < 2; attempt++ {
		reply, err := agent.askPlain(question, "", timeout)
		if err != nil {
			return "", ""
		}
		command, args, _ = freeGoalParse(reply.Text, catalogue)
		fmt.Fprintf(os.Stderr, "[FREE_GOAL] proposal: %q -> %s %s\n", truncateRunes(reply.Text, 120), command, args)
		if command == "" {
			return "", ""
		}
		if !tried[strings.TrimSpace(command+" "+args)] {
			break
		}
		question = b.String() + "\n'" + command + "' has already been tried and did not work.\n" +
			"Name a different command — the one that would make what is missing.\n"
		command = ""
	}
	if command == "" {
		// Nothing usable from the model. The situation itself says what is
		// missing: a lookup by name came back not-found, and what was asked
		// for is a want of that name. Offering it is not deciding to make it
		// — the offer is the point, and a person answers.
		if command, args = freeGoalMissingWant(steps, catalogue); command == "" {
			return "", ""
		}
	}
	entry := catalogue[command]
	if strings.HasPrefix(command, "wants ") && flagNeedsWantType(entry) {
		args = freeGoalFixType(agent, request, args, timeout)
		args = freeGoalFillParams(agent, request, args, timeout)
	}
	return command, args
}

// freeGoalMissingWant reads an offer out of a failed lookup.
//
// `wants get 中野坂上のWeather` → "want not found" is the whole situation: the
// thing the request is about does not exist, and it has a name. What TYPE it
// should be and what it should read are still the model's to say (see
// freeGoalFixType, freeGoalFillParams) — this only supplies the shape of the
// offer, and the offer is put to a person before anything happens.
func freeGoalMissingWant(steps []freeGoalStep, catalogue map[string]freeGoalCommand) (string, string) {
	if _, ok := catalogue["wants create"]; !ok {
		return "", ""
	}
	for i := len(steps) - 1; i >= 0; i-- {
		s := steps[i]
		if !s.Failed || s.Command != "wants get" || strings.TrimSpace(s.Args) == "" {
			continue
		}
		if !strings.Contains(strings.ToLower(s.Output), "not found") {
			continue
		}
		return "wants create", "--name " + strings.Fields(s.Args)[0]
	}
	return "", ""
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
// Out loud when the goal says who asked for it: whoever set `asked_by` is a
// person standing on the board, and the answer belongs where they are looking.
// A goal from a terminal names nobody, and its answer is printed there instead.
// (Talking to the robot on the canvas is a conversation, not a goal — that goes
// to the robot's own want; see forwardToRobotIfAddressed.)
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

func anyStepWorked(steps []freeGoalStep) bool {
	for _, s := range steps {
		if !s.Failed {
			return true
		}
	}
	return false
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
