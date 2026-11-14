package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"mywant/client"
)

// wantsSearchCmd finds wants by what they are: their name, type, status, id,
// a parameter, a label or a field of their state — each a flag, all of them
// to hold. With -q, their ids alone, for another command to act on them:
//
//	mywant wants restart $(mywant wants search --type transit_search --param from=荻窪 -q)
var wantsSearchCmd = &cobra.Command{
	Use:     "search [flags]",
	Aliases: []string{"s", "find"},
	Short:   "Find wants by name, type, status, id, parameter, label or state field",
	Long: `Finds the wants that every flag given holds for.

A value with * ? or [...] is a shell-style pattern over the whole value
(transit* , *新宿). Without one it is looked for inside the value, letter case
aside ("新宿" finds 荻窪→新宿) — except --type, --status and --id, which are
the value itself.

--name, --type, --status and --id may be given more than once: any of them.
--param, --label and --state are key=value and may be given more than once:
all of them. A key alone (--param from) asks only that it is set.

Archived wants are left out unless --all is given (or --status archived).
Quote a pattern, or the shell expands it against the files here first.`,
	Example: `  mywant wants search --name 'transit*'
  mywant wants search --type transit_search --param from=荻窪
  mywant wants search --param 'to=*新宿*' --status achieved
  mywant wants search --label favourite=true
  mywant wants search --state fare=178
  mywant wants restart $(mywant wants search --type transit_search -q)`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		f := cmd.Flags()
		all, _ := f.GetBool("all")
		quiet, _ := f.GetBool("quiet")
		names, _ := f.GetStringSlice("name")
		types, _ := f.GetStringSlice("type")
		statuses, _ := f.GetStringSlice("status")
		ids, _ := f.GetStringSlice("id")
		params, _ := f.GetStringArray("param")
		labels, _ := f.GetStringArray("label")
		states, _ := f.GetStringArray("state")
		if len(names)+len(types)+len(statuses)+len(ids)+len(params)+len(labels)+len(states) == 0 {
			fmt.Fprintln(os.Stderr, "Error: say what to search by: --name, --type, --status, --id, --param, --label or --state (see --help)")
			os.Exit(1)
		}
		c := client.NewClient(viper.GetString("server"))
		resp, err := c.ListWants("", nil, nil, false, false)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		archivedAsked := false
		for _, st := range statuses {
			if searchMatch(st, "archived", true) {
				archivedAsked = true
			}
		}
		var found []*client.Want
		for _, w := range resp.Wants {
			archived := w.Status == "archived" || w.Metadata.Labels["mywant.io/archived"] == "true"
			if archived && !all && !archivedAsked {
				continue
			}
			if !anyMatch(names, w.Metadata.Name, false) || !anyMatch(types, w.Metadata.Type, true) ||
				!anyMatch(statuses, w.Status, true) || !anyMatch(ids, w.Metadata.ID, true) {
				continue
			}
			if !allKV(params, func(k string) (any, bool) { v, ok := w.Spec.Params[k]; return v, ok }) ||
				!allKV(labels, func(k string) (any, bool) { v, ok := w.Metadata.Labels[k]; return v, ok }) ||
				!allKV(states, func(k string) (any, bool) { return stateField(w.State, k) }) {
				continue
			}
			found = append(found, w)
		}
		if len(found) == 0 {
			fmt.Fprintln(os.Stderr, "No want matches")
			os.Exit(1)
		}
		if quiet {
			for _, w := range found {
				fmt.Println(w.Metadata.ID)
			}
			return
		}
		tw := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(tw, "ID\tName\tType\tStatus")
		for _, w := range found {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", w.Metadata.ID, w.Metadata.Name, w.Metadata.Type, w.Status)
		}
		tw.Flush()
		fmt.Printf("\nFound: %d wants\n", len(found))
	},
}

// searchMatch: a pattern (with * ? [...]) over the whole value; otherwise the
// value itself when exact, else the words inside it, letter case aside.
func searchMatch(pattern, value string, exact bool) bool {
	if strings.ContainsAny(pattern, "*?[") {
		ok, err := path.Match(pattern, value)
		return err == nil && ok
	}
	if exact {
		return value == pattern
	}
	return strings.Contains(strings.ToLower(value), strings.ToLower(pattern))
}

// anyMatch: none asked, or one of them matches.
func anyMatch(patterns []string, value string, exact bool) bool {
	if len(patterns) == 0 {
		return true
	}
	for _, p := range patterns {
		if searchMatch(p, value, exact) {
			return true
		}
	}
	return false
}

// allKV: every key=pattern holds — the key is set and its value matches (a
// key alone: only that it is set).
func allKV(pairs []string, get func(string) (any, bool)) bool {
	for _, kv := range pairs {
		k, pat, hasValue := strings.Cut(kv, "=")
		v, ok := get(strings.TrimSpace(k))
		if !ok || v == nil {
			return false
		}
		if hasValue && !searchMatch(pat, searchText(v), false) {
			return false
		}
	}
	return true
}

// stateField: a field of a want's state — its current values first, then the
// rest (final_result).
func stateField(state map[string]any, k string) (any, bool) {
	if cur, ok := state["current"].(map[string]any); ok {
		if v, ok := cur[k]; ok {
			return v, true
		}
	}
	v, ok := state[k]
	return v, ok
}

func searchText(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case map[string]any, []any:
		b, _ := json.Marshal(x)
		return string(b)
	default:
		return fmt.Sprint(x)
	}
}

func init() {
	f := wantsSearchCmd.Flags()
	f.StringSliceP("name", "n", nil, "Name (pattern, or words inside it); repeat for any of several")
	f.StringSliceP("type", "t", nil, "Want type (pattern or exact); repeat for any of several")
	f.StringSlice("status", nil, "Status: achieved, reaching, failed, suspended, archived… (pattern or exact)")
	f.StringSlice("id", nil, "Want id (pattern or exact)")
	f.StringArrayP("param", "p", nil, "A parameter, key=value (pattern, or words inside it); repeat: all must hold")
	f.StringArrayP("label", "l", nil, "A label, key=value; repeat: all must hold")
	f.StringArray("state", nil, "A state field, key=value (fare=178, departure=17*); repeat: all must hold")
	f.BoolP("all", "A", false, "Include archived wants")
	f.BoolP("quiet", "q", false, "Print only the ids (one a line), for another command")
	WantsCmd.AddCommand(wantsSearchCmd)
}
