package commands

// robot.go — `mywant plans` and `mywant acts`: the robot's Plans and Acts
// (want-spec's types), handled the kubectl way.

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	ws "github.com/onelittlenightmusic/want-spec"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var PlansCmd = &cobra.Command{
	Use:     "plans",
	Aliases: []string{"plan"},
	Short:   "Manage the robot's plans — what is to be done about a question",
	Long: `Manage the robot's plans (kind: Plan): what is to be done about a
question, declared and nothing done yet. An act carries one out.

  apiVersion: mywant.io/v1
  kind: Plan
  spec:
    question: 最新のゴルフのスコアは？
    action: existing          # existing | new | none | previous
    target: smartgolf-check-reserved-instance
    capability: rounds        # existing: what is asked of the want
    type: ""                  # new: the type to make
    params: {}                # new: its parameters

Examples:
  mywant plans list -l device=iphone
  mywant plans get plan-1762… -o yaml
  mywant plans create -f plan.yaml --label device=iphone
  mywant plans delete plan-1762…`,
}

var ActsCmd = &cobra.Command{
	Use:     "acts",
	Aliases: []string{"act"},
	Short:   "Manage the robot's acts — plans carried out, kept like Jobs",
	Long: `Manage the robot's acts (kind: Act): a plan carried out, kept like a
Kubernetes Job — Pending → Running → Succeeded | Failed, each tool run a step.

  apiVersion: mywant.io/v1
  kind: Act
  spec:
    plan: plan-1762…          # the plan to carry out
    locale: ja_JP

Examples:
  mywant acts list -l device=iphone
  mywant acts get act-1762… -o yaml
  mywant acts create -f act.yaml
  mywant acts wait act-1762…
  mywant acts run -f plan.yaml       # the plan made, carried out, waited for
  mywant acts delete act-1762…`,
}

// ── reading and writing documents ──

// readDocFile reads -f: a file, or "-" for stdin; YAML or JSON.
func readDocFile(path string, into any) error {
	if path == "" {
		return fmt.Errorf("-f is required")
	}
	var data []byte
	var err error
	if path == "-" {
		data, err = io.ReadAll(os.Stdin)
	} else {
		data, err = os.ReadFile(path)
	}
	if err != nil {
		return err
	}
	return yaml.Unmarshal(data, into)
}

// printDoc prints a document as -o says: yaml (the default) or json.
func printDoc(cmd *cobra.Command, v any) {
	switch out, _ := cmd.Flags().GetString("output"); out {
	case "json":
		data, _ := json.MarshalIndent(v, "", "  ")
		fmt.Println(string(data))
	case "", "yaml":
		data, err := yaml.Marshal(v)
		if err != nil {
			exitErr("printing", err)
		}
		fmt.Print(string(data))
	default:
		exitErr("printing", fmt.Errorf("-o must be yaml or json, not %q", out))
	}
}

// listOutput: true when -o asks for the documents rather than the table.
func listOutput(cmd *cobra.Command) bool {
	out, _ := cmd.Flags().GetString("output")
	return out != ""
}

// age is how long ago, as kubectl says it: 45s, 12m, 3h, 5d.
func age(createdAt string) string {
	t, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return "-"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func clip(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

// ── plans ──

var plansListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List plans, newest first",
	Args:    cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		limit, _ := cmd.Flags().GetInt("limit")
		selector, _ := cmd.Flags().GetString("selector")
		plans, err := memoClient().ListPlans(limit, selector)
		if err != nil {
			exitErr("listing plans", err)
		}
		if listOutput(cmd) {
			printDoc(cmd, ws.PlanList{Plans: plans})
			return
		}
		if len(plans) == 0 {
			fmt.Println("No plans.")
			return
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "NAME\tACTION\tTARGET\tQUESTION\tAGE\tLABELS")
		for _, p := range plans {
			target := p.Spec.Target
			if p.Spec.Action == ws.PlanActionNew {
				target = p.Spec.Type
			}
			if p.Spec.Capability != "" {
				target += "#" + p.Spec.Capability
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", p.Metadata.Name, p.Spec.Action, orDash(target), clip(p.Spec.Question, 40), age(p.Metadata.CreatedAt), labelsText(p.Metadata.Labels))
		}
		w.Flush()
	},
}

var plansGetCmd = &cobra.Command{
	Use:   "get <name>",
	Short: "Show a plan",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		p, err := memoClient().GetPlan(args[0])
		if err != nil {
			exitErr("getting plan", err)
		}
		printDoc(cmd, p)
	},
}

// createPlanFrom reads a plan from -f, lays the --label ones on it, and posts it.
func createPlanFrom(path string, labels map[string]string) *ws.Plan {
	var p ws.Plan
	if err := readDocFile(path, &p); err != nil {
		exitErr("reading plan", err)
	}
	p.Metadata.Labels = withLabels(p.Metadata.Labels, labels)
	if err := p.Validate(); err != nil {
		exitErr("reading plan", err)
	}
	made, err := memoClient().CreatePlan(&p)
	if err != nil {
		exitErr("creating plan", err)
	}
	return made
}

var plansCreateCmd = &cobra.Command{
	Use:   "create -f <file|->",
	Short: "Declare a plan from a YAML or JSON file",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		file, _ := cmd.Flags().GetString("file")
		p := createPlanFrom(file, flagLabels(cmd))
		if listOutput(cmd) {
			printDoc(cmd, p)
			return
		}
		fmt.Printf("plan/%s created\n", p.Metadata.Name)
	},
}

var plansDeleteCmd = &cobra.Command{
	Use:   "delete <name>...",
	Short: "Let plans go",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		for _, name := range args {
			if err := memoClient().DeletePlan(name); err != nil {
				exitErr("deleting plan "+name, err)
			}
			fmt.Printf("plan/%s deleted\n", name)
		}
	},
}

// ── acts ──

var actsListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List acts, newest first",
	Args:    cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		limit, _ := cmd.Flags().GetInt("limit")
		selector, _ := cmd.Flags().GetString("selector")
		acts, err := memoClient().ListActs(limit, selector)
		if err != nil {
			exitErr("listing acts", err)
		}
		if listOutput(cmd) {
			printDoc(cmd, ws.ActList{Acts: acts})
			return
		}
		if len(acts) == 0 {
			fmt.Println("No acts.")
			return
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "NAME\tPHASE\tPLAN\tSTEPS\tAGE\tLABELS")
		for _, a := range acts {
			fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\t%s\n", a.Metadata.Name, a.Status.Phase, a.Spec.Plan, len(a.Status.Steps), age(a.Metadata.CreatedAt), labelsText(a.Metadata.Labels))
		}
		w.Flush()
	},
}

var actsGetCmd = &cobra.Command{
	Use:   "get <name>",
	Short: "Show an act, as far as it has got",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		a, err := memoClient().GetAct(args[0], 0)
		if err != nil {
			exitErr("getting act", err)
		}
		printDoc(cmd, a)
	},
}

var actsCreateCmd = &cobra.Command{
	Use:   "create -f <file|->",
	Short: "Carry out a plan: an act from a YAML or JSON file",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		file, _ := cmd.Flags().GetString("file")
		var a ws.Act
		if err := readDocFile(file, &a); err != nil {
			exitErr("reading act", err)
		}
		a.Metadata.Labels = withLabels(a.Metadata.Labels, flagLabels(cmd))
		if err := a.Validate(); err != nil {
			exitErr("reading act", err)
		}
		made, err := memoClient().CreateAct(&a)
		if err != nil {
			exitErr("creating act", err)
		}
		if listOutput(cmd) {
			printDoc(cmd, made)
			return
		}
		fmt.Printf("act/%s created\n", made.Metadata.Name)
	},
}

// waitAct asks again until the act is done or timeout is up, then prints
// it; exits 1 if it failed or did not finish.
func waitAct(cmd *cobra.Command, name string) {
	timeout, _ := cmd.Flags().GetDuration("timeout")
	deadline := time.Now().Add(timeout)
	for {
		secs := int(time.Until(deadline).Seconds())
		if secs > 20 {
			secs = 20 // under the client's own timeout
		}
		if secs < 1 {
			secs = 1
		}
		a, err := memoClient().GetAct(name, secs)
		if err != nil {
			exitErr("waiting for act", err)
		}
		if a.Finished() || !time.Now().Before(deadline) {
			printActResult(cmd, a)
			if a.Status.Phase != ws.ActSucceeded {
				os.Exit(1)
			}
			return
		}
	}
}

// printActResult: the act's status (or, with -o, the whole act).
func printActResult(cmd *cobra.Command, a *ws.Act) {
	if listOutput(cmd) {
		printDoc(cmd, a)
		return
	}
	fmt.Printf("act/%s %s\n", a.Metadata.Name, a.Status.Phase)
	if !a.Finished() {
		fmt.Fprintln(os.Stderr, "timed out before it was done")
	}
	data, _ := yaml.Marshal(map[string]any{"status": a.Status})
	fmt.Print(string(data))
}

var actsWaitCmd = &cobra.Command{
	Use:   "wait <name>",
	Short: "Wait until an act is done, then show its status",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		waitAct(cmd, args[0])
	},
}

var actsRunCmd = &cobra.Command{
	Use:   "run -f <plan file|->",
	Short: "Declare a plan, carry it out, and wait until it is done",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		file, _ := cmd.Flags().GetString("file")
		locale, _ := cmd.Flags().GetString("locale")
		turn, _ := cmd.Flags().GetString("turn")
		labels := flagLabels(cmd)
		p := createPlanFrom(file, labels)
		fmt.Fprintf(os.Stderr, "plan/%s created\n", p.Metadata.Name)
		a, err := memoClient().CreateAct(&ws.Act{
			Metadata: ws.ResourceMeta{Labels: labels},
			Spec:     ws.ActSpec{Plan: p.Metadata.Name, Locale: locale, Turn: turn},
		})
		if err != nil {
			exitErr("creating act", err)
		}
		fmt.Fprintf(os.Stderr, "act/%s created\n", a.Metadata.Name)
		waitAct(cmd, a.Metadata.Name)
	},
}

var actsDeleteCmd = &cobra.Command{
	Use:   "delete <name>...",
	Short: "Let acts' records go (what they did stays done)",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		for _, name := range args {
			if err := memoClient().DeleteAct(name); err != nil {
				exitErr("deleting act "+name, err)
			}
			fmt.Printf("act/%s deleted\n", name)
		}
	},
}

func init() {
	for _, c := range []*cobra.Command{plansListCmd, actsListCmd} {
		c.Flags().Int("limit", 0, "At most this many (default: all the server keeps)")
		c.Flags().StringP("selector", "l", "", "Label selector: k=v,k2!=v2,k3")
		c.Flags().StringP("output", "o", "", "yaml or json: the documents, not the table")
	}
	for _, c := range []*cobra.Command{plansGetCmd, actsGetCmd} {
		c.Flags().StringP("output", "o", "yaml", "yaml or json")
	}
	for _, c := range []*cobra.Command{plansCreateCmd, actsCreateCmd, actsRunCmd} {
		c.Flags().StringP("file", "f", "", "YAML or JSON file, - for stdin")
		c.Flags().StringArray("label", nil, "A label k=v to put on it (repeatable; on run, on the plan and the act)")
		c.Flags().StringP("output", "o", "", "yaml or json: print the document made")
	}
	actsWaitCmd.Flags().StringP("output", "o", "", "yaml or json: print the whole act")
	for _, c := range []*cobra.Command{actsWaitCmd, actsRunCmd} {
		c.Flags().Duration("timeout", 5*time.Minute, "How long to wait")
	}
	actsRunCmd.Flags().String("locale", "", "The act's locale (ja_JP, en_US …)")
	actsRunCmd.Flags().String("turn", "", "The robot's turn it belongs to")

	PlansCmd.AddCommand(plansListCmd, plansGetCmd, plansCreateCmd, plansDeleteCmd)
	ActsCmd.AddCommand(actsListCmd, actsGetCmd, actsCreateCmd, actsWaitCmd, actsRunCmd, actsDeleteCmd)
}

// flagLabels is --label k=v …; a bad one ends the command.
func flagLabels(cmd *cobra.Command) map[string]string {
	pairs, _ := cmd.Flags().GetStringArray("label")
	labels, err := ws.ParseLabels(pairs)
	if err != nil {
		exitErr("reading --label", err)
	}
	return labels
}

// withLabels lays extra over labels (extra wins).
func withLabels(labels, extra map[string]string) map[string]string {
	if len(extra) == 0 {
		return labels
	}
	out := make(map[string]string, len(labels)+len(extra))
	for k, v := range labels {
		out[k] = v
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}

// labelsText is labels as kubectl --show-labels writes them: k=v,k2=v2.
func labelsText(labels map[string]string) string {
	if len(labels) == 0 {
		return "-"
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k + "=" + labels[k]
	}
	return strings.Join(parts, ",")
}
