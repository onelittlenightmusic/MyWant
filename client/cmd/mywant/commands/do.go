package commands

import (
	"fmt"
	"os"
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
guessing.`,
	Example: `  mywant do "荻窪はどの星座？"
  mywant do "Parasomniaを(9,-9)に移動して"
  mywant do "weatherの隣に新しいbuttonを置いて" --dry-run`,
	Args: cobra.MinimumNArgs(1),
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

		want := &client.Want{
			Metadata: client.Metadata{
				Name:   "goal-" + strconv.FormatInt(time.Now().Unix(), 36),
				Type:   "free_goal",
				Labels: map[string]string{},
			},
			Spec: client.WantSpec{Params: map[string]any{
				"request":   request,
				"max_steps": maxSteps,
				"dry_run":   dryRun,
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
	deadline := time.Now().Add(timeout)
	shown := 0

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
			fmt.Printf("\nStopped before something that cannot be undone:\n  %s\n", pending)
			fmt.Printf("Run it yourself if you want it, or `mywant wants delete %s` to drop the goal.\n", shortID(id))
			return
		}
		time.Sleep(700 * time.Millisecond)
	}
	fmt.Printf("\nStill running after %s. `mywant wants get %s` shows where it got to.\n", timeout, id)
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
}
