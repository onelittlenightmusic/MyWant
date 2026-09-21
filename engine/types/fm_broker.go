package types

import (
	"context"
	"strings"

	. "mywant/engine/core"
)

// The one place a command from the robot is judged.
//
// The on-device agent used to run `mywant` itself and decide for itself what
// it was allowed to run, which meant the same three judgements existed twice,
// once here in Go and once in Swift: which commands are the board's, what each
// one costs, and what a person's "はい" looks like. They had already drifted —
// the two lists of yes-words disagreed, and the Swift one let
// "はい、でも先に天気を見せて" through as consent, which its own comment said was
// not consent.
//
// Now the agent asks and this answers (see Broker.swift, and readReply in
// fm_server.go). What arrives is a command path and its arguments; what goes
// back is what happened. Three outcomes:
//
//	not a command   refused, and told so — the model invents plausible ones.
//	read / change   run, and the output goes back.
//	destroy         NOT run. The sentence is written down where the person can
//	                see it and say yes, and that yes lands in one place
//	                (answerPending) whether it was typed or clicked.
//
// Nothing asks the model whether the person agreed. It used to: the delete
// tool carried a `confirmed` flag for it to fill in, which is the same model
// marking its own homework, and needed a second check behind it for exactly
// that reason. A question this side can answer is not a question to ask the
// model.

// fmEveryCommand is the whole CLI, destroying included, indexed by path.
//
// Not freeGoalCatalogue: that one is the goal loop's menu and leaves out what
// cannot be undone, because the loop is never allowed to choose it. Here the
// destructive commands have to be recognised — refusing them as unknown would
// tell the model to go and find another way to delete something.
var fmEveryCache map[string]freeGoalCommand

func fmEveryCommand() (map[string]freeGoalCommand, error) {
	if fmEveryCache != nil {
		return fmEveryCache, nil
	}
	all, err := freeGoalAllCommands()
	if err != nil {
		return nil, err
	}
	every := make(map[string]freeGoalCommand, len(all))
	for _, c := range all {
		if c.Path != "" {
			every[c.Path] = c
		}
	}
	fmEveryCache = every
	return every, nil
}

// fmBrokerRun carries out one command the agent asked for, or says why it did
// not. `request` is what the person actually said, which is what a misspelled
// name is checked against.
func fmBrokerRun(ctx context.Context, want *Want, request, command string, args []string) (ran, ok bool, output string) {
	catalogue, err := fmEveryCommand()
	if err != nil {
		return false, false, "NOT RUN — mywant could not read its own command list."
	}
	entry, known := catalogue[command]
	if !known {
		return false, false, "NOT RUN — mywant has no command '" + command + "'. " +
			"Choose one from the list you were given."
	}

	// The command's own name, handed back as its argument.
	//
	// The model writes the whole line into `args` often enough that
	// `mywant wants list wants list` is a normal sight. The CLI shrugs at the
	// extra words, so it went unnoticed — until a command that takes a name got
	// its own name as the name and looked for a want called "wants".
	args = fmDropEcho(command, args)

	// The board's own spelling, when the argument is a near miss for it.
	//
	// Asked to delete "NakanoのWeather" the model wrote "Nakanoのweather", and a
	// confirmation was offered for a want that does not exist: the person would
	// have said yes to nothing. Corrected rather than refused, since the
	// difference is usually a capital — and left alone when nothing matches,
	// because an id is a perfectly good thing to name and is on no such list.
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		if name := fmBoardName(args[0]); name != "" {
			args = append([]string{name}, args[1:]...)
		}
	}
	sentence := strings.TrimSpace("mywant " + command + " " + strings.Join(args, " "))

	// Nothing named, nothing to offer: asked to confirm "delete a want", the
	// person has no way to know which, and neither has this. Said before
	// anything is written down, so a confirmation for a command that names
	// nothing never reaches the screen.
	if entry.Risk == "destroy" && strings.ContainsAny(entry.Use, "<[") && len(args) == 0 {
		return false, false, "NOT RUN — '" + command + "' has to be told which one, and `args` was empty. " +
			"Look the name up with 'board' if you do not have it, then call '" + command + "' again with " +
			"`args` holding that name alone."
	}

	if entry.Risk == "destroy" {
		// Written down, not run. What waits is a sentence a person can read
		// and agree to, which is the only form of consent worth having.
		//
		// One key: what the board's confirmation overlay shows IS what a yes
		// carries out. There used to be two, because the agent kept a gate of
		// its own and would run its own pending command on its own reading of
		// a yes — so the sentence on the screen and the sentence that would
		// run had different authors and had to be kept apart. Neither is true
		// any more.
		want.SetCurrent("pending_command", sentence)
		RecordCCActivity(want, CCActivityNote, truncateRunes(sentence, 90))
		want.StoreLog("[FM_BROKER] waiting for a yes: %s", sentence)
		// Worded so it cannot be read as a failure. "It cannot be undone" came
		// back to the person as "the want cannot be deleted" — the model
		// paraphrased a warning into an impossibility, and the person believed
		// it and stopped.
		return false, false, "NOT RUN, and nothing failed. A person is being asked first. " +
			"Tell them, in their language, that `" + sentence + "` is waiting and that it is permanent. " +
			"Do not say it failed, do not say it is impossible, and do not ask them to repeat themselves — " +
			"their answer arrives by itself."
	}

	RecordCCActivity(want, CCActivityTool, truncateRunes(sentence, 90))
	text, failed := freeGoalRunArgv(ctx, command, args)
	want.StoreLog("[FM_BROKER] ran %s -> %s", sentence, truncateRunes(text, 120))
	return true, !failed, text
}

// fmBoardName is what the board calls something, when it calls anything that.
//
// Exactly first, then ignoring case — "Nakanoのweather" and "NakanoのWeather"
// are the same want and only one of them exists — then by containment, which
// catches a name given in part. Empty when nothing is close enough: the
// argument then stands as the model wrote it.
func fmBoardName(given string) string {
	wanted := strings.TrimSpace(given)
	if wanted == "" {
		return ""
	}
	names := freeGoalBoardNames()
	for _, name := range names {
		if name == wanted {
			return name
		}
	}
	for _, name := range names {
		if strings.EqualFold(name, wanted) {
			return name
		}
	}
	var near []string
	lower := strings.ToLower(wanted)
	for _, name := range names {
		if name == "" {
			continue
		}
		if strings.Contains(strings.ToLower(name), lower) || strings.Contains(lower, strings.ToLower(name)) {
			near = append(near, name)
		}
	}
	if len(near) == 1 {
		return near[0]
	}
	return ""
}

// fmDropEcho takes the command's own words back off the front of its arguments.
func fmDropEcho(command string, args []string) []string {
	if len(args) == 0 {
		return args
	}
	// The whole path written as one argument, or word by word.
	if len(args) == 1 && strings.TrimSpace(args[0]) == command {
		return nil
	}
	words := strings.Fields(command)
	if len(args) < len(words) {
		return args
	}
	for i, word := range words {
		if args[i] != word {
			return args
		}
	}
	return args[len(words):]
}
