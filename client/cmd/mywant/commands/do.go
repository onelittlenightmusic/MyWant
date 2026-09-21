package commands

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"mywant/client"
)

// Asking for something in the words you would use.
//
//	mywant do "Parasomniaを(9,-9)に移動して"
//
// This makes a free_goal want and hands it the sentence. The want works out
// which commands carry it out — one short question to the on-device model at a
// time, each answer checked against this CLI's own command list — runs them,
// and keeps every step in its own state (see engine/types/agent_free_goal.go).
//
// A want rather than a function call, because a job that takes four commands
// has a shape somebody might want to look at: what it decided, what it ran,
// what came back, and where it stopped. A tile on the board can be looked at.
// A goroutine that finished cannot.
var DoCmd = &cobra.Command{
	Use:   "do <request>",
	Short: "Ask for something in plain words: a goal want works out the commands and runs them",
	Long: `Creates a free_goal want holding what you asked for, and waits for it.

The request is kept verbatim as the want's parameter, so what was asked and what
was done stay together. Commands that only read or that can be undone are run;
a command that cannot be undone stops the goal and asks you first.

Needs an on-device model (macOS). Without one the goal says so rather than
guessing.

--show-prompts prints each question put to the model and what it answered. A
goal is several small questions — which command, which want type, which
values — and a surprising answer is almost always a surprising question.`,
	Example: `  mywant do "荻窪はどの星座？"
  mywant do "Parasomniaを(9,-9)に移動して"
  mywant do "weatherの隣に新しいbuttonを置いて" --dry-run`,
	Args: cobra.MinimumNArgs(1),
	// Board work, whatever its group says. `do` is how anything gets built on
	// the canvas, and the on-device agent picks what it may write from this
	// label alone (see MyWantCLI.offered in fmtool) — without it, the robot
	// can describe a board and not add anything to it. The goal loop leaves it
	// out for itself, since a goal asking for a goal is a goal asking for a
	// goal (see freeGoalCatalogue).
	Annotations: map[string]string{AnnotationCanvas: "true"},
	Run: func(cmd *cobra.Command, args []string) {
		request := strings.Join(args, " ")
		api := wantsClient()

		at, _ := cmd.Flags().GetString("at")
		x, y, placing, err := parseCell(at)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		maxSteps, _ := cmd.Flags().GetInt("max-steps")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		showPrompts, _ := cmd.Flags().GetBool("show-prompts")

		want := &client.Want{
			Metadata: client.Metadata{
				Name:   "goal-" + strconv.FormatInt(time.Now().Unix(), 36),
				Type:   "free_goal",
				Labels: map[string]string{},
			},
			Spec: client.WantSpec{Params: map[string]any{
				"request":      request,
				"max_steps":    maxSteps,
				"dry_run":      dryRun,
				"show_prompts": showPrompts,
			}},
		}
		if placing {
			want.Metadata.Labels[canvasXLabel] = strconv.Itoa(x)
			want.Metadata.Labels[canvasYLabel] = strconv.Itoa(y)
		}

		resp, err := api.CreateWant(client.Config{Wants: []*client.Want{want}})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating the goal: %v\n", err)
			os.Exit(1)
		}
		if len(resp.WantIDs) == 0 {
			fmt.Fprintln(os.Stderr, "Error: the goal was not created")
			os.Exit(1)
		}
		id := resp.WantIDs[0]
		fmt.Printf("Goal %s: %s\n", shortID(id), request)

		if wait, _ := cmd.Flags().GetBool("wait"); !wait {
			fmt.Printf("Running. `mywant wants get %s` shows how far it gets.\n", id)
			return
		}
		watchGoal(api, id, cmd)
	},
}

// watchGoal follows one goal until it finishes, stops to ask, or runs out of
// patience, printing each step as it happens.
//
// Each step as it happens, because a goal that takes four commands and twenty
// seconds is otherwise a cursor blinking at nothing: the interesting part is
// which command it chose, and that is known long before the answer is.
func watchGoal(api *client.Client, id string, cmd *cobra.Command) {
	timeout, _ := cmd.Flags().GetDuration("timeout")
	showPrompts, _ := cmd.Flags().GetBool("show-prompts")
	deadline := time.Now().Add(timeout)
	shown, shownPrompts := 0, 0

	for time.Now().Before(deadline) {
		want, err := api.GetWant(id, false)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading the goal: %v\n", err)
			os.Exit(1)
		}
		// A want's state comes back under "current" — the label the state keys
		// are declared with — not flat. Read flat, every phase looked empty and
		// the goal appeared to run forever while it had long since finished.
		state, _ := want.State["current"].(map[string]any)
		if state == nil {
			state = want.State
		}
		// What was asked of the model, before what came of it: the questions
		// are the part nobody can see otherwise, and they are where a strange
		// answer usually comes from.
		if showPrompts {
			prompts, _ := state["prompts"].([]any)
			for ; shownPrompts < len(prompts); shownPrompts++ {
				entry, _ := prompts[shownPrompts].(map[string]any)
				fmt.Printf("\n┌─ asked (%v) ─────────────────────────────\n", entry["asked"])
				for _, line := range strings.Split(strings.TrimRight(fmt.Sprint(entry["prompt"]), "\n"), "\n") {
					fmt.Println("│ " + line)
				}
				fmt.Println("├─ answered ──────────────────────────────")
				for _, line := range strings.Split(strings.TrimRight(fmt.Sprint(entry["answer"]), "\n"), "\n") {
					fmt.Println("│ " + line)
				}
				fmt.Println("└─────────────────────────────────────────")
			}
		}

		steps, _ := state["steps"].([]any)
		for ; shown < len(steps); shown++ {
			step, _ := steps[shown].(map[string]any)
			mark := "·"
			if failed, _ := step["failed"].(bool); failed {
				mark = "×"
			}
			fmt.Printf("  %s mywant %v\n", mark, step["command"])
			if out, _ := step["output"].(string); out != "" {
				fmt.Printf("    %s\n", firstLine(out))
			}
		}

		switch phase, _ := state["phase"].(string); phase {
		case "done":
			if answer, _ := state["answer"].(string); answer != "" {
				fmt.Printf("\n%s\n", answer)
			}
			return
		case "failed":
			if reason, _ := state["error"].(string); reason != "" {
				fmt.Fprintf(os.Stderr, "\n%s\n", reason)
			}
			os.Exit(1)
		case "waiting_confirmation":
			pending, _ := state["pending_command"].(string)
			reason, _ := state["pending_reason"].(string)
			askToRun(api, id, pending, reason, cmd)
			return
		}
		time.Sleep(700 * time.Millisecond)
	}
	fmt.Printf("\nStill running after %s. `mywant wants get %s` shows where it got to.\n", timeout, id)
}

// askToRun puts the goal's one pending command to the person, and runs it if
// they say yes.
//
// A goal stops for two reasons and they deserve different words: a command
// that cannot be undone is a warning, and a request that nothing on the board
// answers yet is an offer. Asked to know the weather somewhere with no want
// for it, "there is no such want" is true and unhelpful — "shall I make one?"
// is the same fact with the next move in it.
//
// Only asked when somebody is there to answer. Piped or scripted, the command
// is printed and nothing runs: a goal that quietly created something because
// nobody was watching is the failure this whole mechanism exists to prevent.
func askToRun(api *client.Client, id, pending, reason string, cmd *cobra.Command) {
	if pending == "" {
		fmt.Println("\nStopped without finishing, and with nothing to suggest.")
		return
	}
	if reason == "destroy" {
		fmt.Printf("\nThis cannot be undone:\n  %s\n", pending)
	} else {
		fmt.Printf("\nNothing on the board answers that yet. This would do it:\n  %s\n", pending)
	}

	yes, _ := cmd.Flags().GetBool("yes")
	if !yes {
		if !isTerminal(os.Stdin) {
			fmt.Println("Run it yourself if you want it, or pass --yes next time.")
			return
		}
		fmt.Print("Run it? [y/N] ")
		reader := bufio.NewReader(os.Stdin)
		line, _ := reader.ReadString('\n')
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "y", "yes", "はい":
		default:
			fmt.Println("Left alone.")
			return
		}
	}

	self, err := os.Executable()
	if err != nil {
		self = os.Args[0]
	}
	argv := strings.Fields(strings.TrimPrefix(pending, "mywant "))
	out, err := exec.Command(self, argv...).CombinedOutput()
	fmt.Print(string(out))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	// The goal records what became of it, so its card does not sit forever at
	// "waiting" for a question that was answered.
	_ = api.SetWantState(id, map[string]any{
		"phase":           "done",
		"pending_command": "",
		"pending_reason":  "",
		"answer":          strings.TrimSpace(firstLine(string(out))),
	})
}

// isTerminal reports whether somebody is there to be asked.
func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func firstLine(text string) string {
	line := strings.TrimSpace(strings.SplitN(strings.TrimSpace(text), "\n", 2)[0])
	if len([]rune(line)) > 120 {
		return string([]rune(line)[:120]) + "…"
	}
	return line
}

func init() {
	DoCmd.Flags().String("at", "", "Canvas cell for the goal's own tile, as x,y")
	DoCmd.Flags().Int("max-steps", 4, "How many commands the goal may run")
	DoCmd.Flags().Bool("dry-run", false, "Work out the first step and run none of it")
	DoCmd.Flags().Bool("wait", true, "Follow the goal until it finishes")
	DoCmd.Flags().Duration("timeout", 3*time.Minute, "How long to follow it before leaving it running")
	DoCmd.Flags().Bool("yes", false, "Answer yes to the goal's question, if it has to stop and ask one")
	DoCmd.Flags().Bool("show-prompts", false, "Print every question put to the model, and its answer")
}
