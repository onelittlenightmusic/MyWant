package commands

// story.go — `mywant story <character>`: what the person playing a character
// did on the board (the server's story.go).

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"mywant/client"

	ws "github.com/onelittlenightmusic/want-spec"
	"github.com/spf13/cobra"
)

var StoryCmd = &cobra.Command{
	Use:   "story <character>",
	Short: "Show a character's story — where it went, what it opened, asked and was answered",
	Long: `Show a character's story: what the person playing it did on the board,
kept by the server from the cursor's moves, the GUI's state, the turns and acts.

The character is its id or its name.

Examples:
  mywant story chr-abc
  mywant story ロボット --since 2h
  mywant story chr-abc --kind want.arrive,chat.ask,chat.answer
  mywant story chr-abc --since 2026-11-04T09:00:00+09:00 -o yaml
  mywant story Hero --digest --since 30d     # a line a day: how the answers went`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		c := memoClient()
		id := resolveCharacter(c, args[0])
		since, _ := cmd.Flags().GetString("since")
		until, _ := cmd.Flags().GetString("until")
		if digest, _ := cmd.Flags().GetBool("digest"); digest {
			showDigests(cmd, c, id, since, until)
			return
		}
		kinds, _ := cmd.Flags().GetString("kind")
		limit, _ := cmd.Flags().GetInt("limit")
		events, err := c.GetStory(id, client.StoryQuery{Since: since, Until: until, Kinds: kinds, Limit: limit})
		if err != nil {
			exitErr("reading the story", err)
		}
		if listOutput(cmd) {
			printDoc(cmd, ws.StoryList{Events: events})
			return
		}
		if len(events) == 0 {
			fmt.Println("Nothing in the story for that time.")
			return
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "TIME\tKIND\tFROM\tTO\tREF\tDETAIL")
		for _, ev := range events {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", storyTime(ev.At), ev.Kind, storyPlaceText(ev.From),
				storyPlaceText(ev.To), orDash(ev.Ref), clip(storyDetailText(ev.Detail), 60))
		}
		w.Flush()
	},
}

// resolveCharacter is a character's id, given its id or its name.
func resolveCharacter(c *client.Client, ref string) string {
	chars, err := c.ListCharacterRefs()
	if err != nil {
		return ref // let the server say
	}
	for _, ch := range chars {
		if ch.ID == ref {
			return ref
		}
	}
	for _, ch := range chars {
		if ch.Name == ref {
			return ch.ID
		}
	}
	return ref
}

func storyTime(at string) string {
	t, err := time.Parse(time.RFC3339, at)
	if err != nil {
		return at
	}
	return t.Local().Format("01-02 15:04:05")
}

func storyPlaceText(p *ws.StoryPlace) string {
	if p == nil {
		return "-"
	}
	switch {
	case p.Name != "":
		return p.Name
	case p.ID != "":
		return p.ID
	case p.X != nil && p.Y != nil:
		return fmt.Sprintf("(%d,%d)", *p.X, *p.Y)
	}
	return p.Kind
}

func storyDetailText(d map[string]string) string {
	if len(d) == 0 {
		return "-"
	}
	keys := make([]string, 0, len(d))
	for k := range d {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k + "=" + d[k]
	}
	return strings.Join(parts, " ")
}

// showDigests is --digest: a line a day, the answers counted by how they
// went; -o yaml / json, the digests whole (each question with its plan).
func showDigests(cmd *cobra.Command, c *client.Client, id, since, until string) {
	digests, err := c.GetDigests(id, since, until)
	if err != nil {
		exitErr("reading the digests", err)
	}
	if listOutput(cmd) {
		printDoc(cmd, map[string]any{"digests": digests})
		return
	}
	if len(digests) == 0 {
		fmt.Println("No days in the story for that time.")
		return
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "DATE\tQUESTIONS\tSUCCESS\tCORRECTED\tREASK\tIGNORED\tSUCCESS RATE")
	total := map[string]int{}
	questions := 0
	line := func(date string, q int, o map[string]int) {
		judged := o[ws.TurnOutcomeSuccess] + o[ws.TurnOutcomeCorrected] + o[ws.TurnOutcomeReask] + o[ws.TurnOutcomeIgnored]
		rate := "-"
		if judged > 0 {
			rate = fmt.Sprintf("%d%% (%d/%d)", o[ws.TurnOutcomeSuccess]*100/judged, o[ws.TurnOutcomeSuccess], judged)
		}
		fmt.Fprintf(w, "%s\t%d\t%d\t%d\t%d\t%d\t%s\n", date, q, o[ws.TurnOutcomeSuccess], o[ws.TurnOutcomeCorrected],
			o[ws.TurnOutcomeReask], o[ws.TurnOutcomeIgnored], rate)
	}
	for _, d := range digests {
		line(d.Date, len(d.Turns), d.Outcomes)
		questions += len(d.Turns)
		for k, v := range d.Outcomes {
			total[k] += v
		}
	}
	if len(digests) > 1 {
		line("TOTAL", questions, total)
	}
	w.Flush()
}

func init() {
	StoryCmd.Flags().String("since", "", "From when: RFC 3339, or a duration back from now (default 24h); with --digest, YYYY-MM-DD or days back (7d)")
	StoryCmd.Flags().String("until", "", "Until when: RFC 3339 (default now); with --digest, YYYY-MM-DD")
	StoryCmd.Flags().Bool("digest", false, "A line a day: the questions and how their answers went (success, corrected, reask, ignored)")
	StoryCmd.Flags().String("kind", "", "Only these kinds, comma-separated (want.arrive,chat.ask,…)")
	StoryCmd.Flags().Int("limit", 0, "The last this many (default 500)")
	StoryCmd.Flags().StringP("output", "o", "", "yaml or json: the events, not the table")
}
