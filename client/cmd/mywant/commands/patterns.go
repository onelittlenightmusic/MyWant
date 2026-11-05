package commands

// patterns.go — `mywant patterns [question]`: the kinds of question asked
// and which way of answering each went well (the server's story_ledger.go).

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var PatternsCmd = &cobra.Command{
	Use:   "patterns [question]",
	Short: "Show the kinds of question asked and which way of answering each went well",
	Long: `Show the kinds of question the robot was asked — 「{from}から{to}の乗り換え」 —
and, for each, the ways it was answered (new transit_search(from,to), existing
transit_search, …) with how they went: success, corrected, reask, ignored.

A kind a want type names in a worked example (robot-qa/…) says which type;
one no type names is a worked example to add.

Given a question, the kinds it is one of (else the most like it).

Examples:
  mywant patterns
  mywant patterns 渋谷から品川の乗り換え
  mywant patterns --character Hero -o yaml`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		c := memoClient()
		question := ""
		if len(args) == 1 {
			question = args[0]
		}
		character, _ := cmd.Flags().GetString("character")
		if character != "" {
			character = resolveCharacter(c, character)
		}
		limit, _ := cmd.Flags().GetInt("limit")
		patterns, err := c.GetPatterns(question, character, limit)
		if err != nil {
			exitErr("reading the patterns", err)
		}
		if listOutput(cmd) {
			printDoc(cmd, map[string]any{"patterns": patterns})
			return
		}
		if len(patterns) == 0 {
			fmt.Println("No judged answers yet.")
			return
		}
		for i, p := range patterns {
			if i > 0 {
				fmt.Println()
			}
			about := "not in any type's worked examples"
			if p.Declared != nil {
				about = p.Declared.Type + ": 「" + p.Declared.Request + "」"
			}
			match := ""
			if p.Match == "similar" {
				match = " (similar)"
			}
			fmt.Printf("%s%s — %d asked · %s\n", p.Pattern, match, p.Count, about)
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "  ANSWERED AS\tCOUNT\tSUCCESS\tOUTCOMES\tLATEST SUCCESS")
			for _, a := range p.Answers {
				example := "-"
				if a.Example != nil {
					example = clip(a.Example.Question, 40)
				}
				fmt.Fprintf(w, "  %s\t%d\t%d%%\t%s\t%s\n", a.Shape, a.Count, int(a.SuccessRate*100+0.5), outcomeText(a.Outcomes), example)
			}
			w.Flush()
		}
	},
}

func outcomeText(o map[string]int) string {
	keys := make([]string, 0, len(o))
	for k := range o {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = fmt.Sprintf("%s=%d", k, o[k])
	}
	return strings.Join(parts, " ")
}

func init() {
	PatternsCmd.Flags().String("character", "", "Only this character's answers (id or name)")
	PatternsCmd.Flags().Int("limit", 0, "At most this many kinds")
	PatternsCmd.Flags().StringP("output", "o", "", "yaml or json: the patterns, not the table")
}
