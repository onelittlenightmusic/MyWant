package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"
	"text/tabwriter"

	"mywant/client"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Memo is the catalog of remembered input values, grouped by subtype
// (destination, hotel, command, ...). Wants record what the user typed, the
// server keeps provenance (events) and usage counts (stats), and values can be
// labelled - groups being a facade over a "group/<name>" label.
//
// Global state, which this command used to manage, now lives under
// "mywant state global".

var MemoCmd = &cobra.Command{
	Use:     "memo",
	Aliases: []string{"m"},
	Short:   "Manage the memo catalog (remembered input values)",
	Long: `Manage memo: the catalog of values the user has entered, keyed by subtype.

Examples:
  mywant memo list
  mywant memo get destination
  mywant memo add destination Kyoto Osaka
  mywant memo remove destination Osaka
  mywant memo events --catalog destination --value Kyoto
  mywant memo stats
  mywant memo label cities::Kyoto favourite true
  mywant memo groups list`,
}

// ─── catalog ───────────────────────────────────────────────────────────────

var memoListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"l"},
	Short:   "List every subtype and its recorded values",
	Args:    cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		memo, err := memoClient().GetMemo()
		if err != nil {
			exitErr("listing memo", err)
		}

		if jsonOut(cmd) {
			printJSON(memo)
			return
		}
		if len(memo) == 0 {
			fmt.Println("Memo is empty.")
			return
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "SUBTYPE\tCOUNT\tVALUES")
		for _, subtype := range sortedKeys(memo) {
			values := memo[subtype]
			fmt.Fprintf(w, "%s\t%d\t%s\n", subtype, len(values), truncateList(values, 5))
		}
		w.Flush()
	},
}

var memoGetCmd = &cobra.Command{
	Use:               "get <catalog|subtype>",
	Aliases:           []string{"g", "show"},
	Short:             "Show the values recorded under one catalog key, newest first",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeMemoSubtypes,
	Run: func(cmd *cobra.Command, args []string) {
		limit, _ := cmd.Flags().GetInt("limit")
		values, err := memoValues(args[0], limit)
		if err != nil {
			exitErr("reading memo", err)
		}

		if jsonOut(cmd) {
			printJSON(values)
			return
		}
		if len(values) == 0 {
			fmt.Printf("No values recorded for %q.\n", args[0])
			return
		}
		for _, v := range values {
			fmt.Println(v)
		}
	},
}

var memoAddCmd = &cobra.Command{
	Use:               "add <subtype> <value>...",
	Short:             "Add values to a subtype",
	Args:              cobra.MinimumNArgs(2),
	ValidArgsFunction: completeMemoSubtypes,
	Run: func(cmd *cobra.Command, args []string) {
		subtype, values := args[0], args[1:]
		c := memoClient()

		memo, err := c.GetMemo()
		if err != nil {
			exitErr("reading memo", err)
		}
		if memo == nil {
			memo = map[string][]string{}
		}

		added := 0
		for _, v := range values {
			if slices.Contains(memo[subtype], v) {
				continue
			}
			memo[subtype] = append(memo[subtype], v)
			added++
		}
		if added == 0 {
			fmt.Println("Nothing to add; all values are already recorded.")
			return
		}
		if err := c.PutMemo(memo); err != nil {
			exitErr("saving memo", err)
		}
		fmt.Printf("Added %d value(s) to %s.\n", added, subtype)
	},
}

var memoRemoveCmd = &cobra.Command{
	Use:               "remove <subtype> [value]...",
	Aliases:           []string{"rm", "delete"},
	Short:             "Remove values from a subtype, or the whole subtype",
	Args:              cobra.MinimumNArgs(1),
	ValidArgsFunction: completeMemoSubtypes,
	Run: func(cmd *cobra.Command, args []string) {
		subtype, values := args[0], args[1:]
		c := memoClient()

		memo, err := c.GetMemo()
		if err != nil {
			exitErr("reading memo", err)
		}
		if _, ok := memo[subtype]; !ok {
			fmt.Fprintf(os.Stderr, "Subtype %q is not in the memo.\n", subtype)
			os.Exit(1)
		}

		if len(values) == 0 {
			delete(memo, subtype)
			if err := c.PutMemo(memo); err != nil {
				exitErr("saving memo", err)
			}
			fmt.Printf("Removed subtype %s.\n", subtype)
			return
		}

		kept := make([]string, 0, len(memo[subtype]))
		removed := 0
		for _, v := range memo[subtype] {
			if slices.Contains(values, v) {
				removed++
				continue
			}
			kept = append(kept, v)
		}
		if removed == 0 {
			fmt.Println("Nothing to remove; no value matched.")
			return
		}
		memo[subtype] = kept
		if err := c.PutMemo(memo); err != nil {
			exitErr("saving memo", err)
		}
		fmt.Printf("Removed %d value(s) from %s.\n", removed, subtype)
	},
}

// ─── provenance ────────────────────────────────────────────────────────────

var memoEventsCmd = &cobra.Command{
	Use:   "events",
	Short: "Show where memo values came from, newest first",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		limit, _ := cmd.Flags().GetInt("limit")
		catalog, _ := cmd.Flags().GetString("catalog")
		value, _ := cmd.Flags().GetString("value")

		events, err := memoClient().GetMemoEvents(catalog, value, limit)
		if err != nil {
			exitErr("reading memo events", err)
		}

		if jsonOut(cmd) {
			printJSON(events)
			return
		}
		if len(events) == 0 {
			fmt.Println("No memo events recorded.")
			return
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "AT\tCATALOG\tVALUE\tSOURCE\tWANT")
		for _, e := range events {
			want := e.WantType
			if e.WantID != "" {
				want = strings.TrimSpace(want + " " + e.WantID)
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", e.At, e.Catalog, e.Value, dashIfEmpty(e.Source), dashIfEmpty(want))
		}
		w.Flush()
	},
}

var memoStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show how often each memo value is used",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		stats, err := memoClient().GetMemoStats()
		if err != nil {
			exitErr("reading memo stats", err)
		}

		if jsonOut(cmd) {
			printJSON(stats)
			return
		}
		if len(stats) == 0 {
			fmt.Println("No memo usage recorded.")
			return
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "CATALOG\tVALUE\tUSED\tLAST USED")
		for _, catalog := range sortedKeys(stats) {
			values := stats[catalog]
			names := sortedKeys(values)
			sort.SliceStable(names, func(i, j int) bool { return values[names[i]].Count > values[names[j]].Count })
			for _, value := range names {
				fmt.Fprintf(w, "%s\t%s\t%d\t%s\n", catalog, value, values[value].Count, values[value].LastUsed)
			}
		}
		w.Flush()
	},
}

// ─── labels ────────────────────────────────────────────────────────────────

var memoLabelsCmd = &cobra.Command{
	Use:     "labels",
	Short:   "List labels attached to memo values",
	Args:    cobra.NoArgs,
	Aliases: []string{"label-list"},
	Run: func(cmd *cobra.Command, args []string) {
		labels, err := memoClient().GetMemoLabels()
		if err != nil {
			exitErr("reading memo labels", err)
		}

		if jsonOut(cmd) {
			printJSON(labels)
			return
		}
		if len(labels) == 0 {
			fmt.Println("No memo labels set.")
			return
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "VALUE\tLABELS")
		for _, valueID := range sortedKeys(labels) {
			pairs := make([]string, 0, len(labels[valueID]))
			for _, key := range sortedKeys(labels[valueID]) {
				pairs = append(pairs, key+"="+labels[valueID][key])
			}
			fmt.Fprintf(w, "%s\t%s\n", valueID, strings.Join(pairs, " "))
		}
		w.Flush()
	},
}

var memoLabelCmd = &cobra.Command{
	Use:   "label <catalog::value> <key> [value]",
	Short: "Attach a label to a memo value (omit the label value to remove it)",
	Args:  cobra.RangeArgs(2, 3),
	Run: func(cmd *cobra.Command, args []string) {
		valueID, key := args[0], args[1]
		c := memoClient()

		if len(args) == 2 {
			if err := c.RemoveMemoLabel(valueID, key); err != nil {
				exitErr("removing memo label", err)
			}
			fmt.Printf("Removed label %s from %s.\n", key, valueID)
			return
		}
		if err := c.SetMemoLabel(valueID, key, args[2]); err != nil {
			exitErr("setting memo label", err)
		}
		fmt.Printf("Set %s=%s on %s.\n", key, args[2], valueID)
	},
}

// ─── groups ────────────────────────────────────────────────────────────────

var memoGroupsCmd = &cobra.Command{
	Use:   "groups",
	Short: "Manage groups of memo values or wants",
	Long: `Groups are named sets stored as "group/<name>" labels on memo values or wants.

Examples:
  mywant memo groups list
  mywant memo groups create favourites --kind memo --member cities::Kyoto
  mywant memo groups delete favourites --kind memo`,
}

var memoGroupsListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"l"},
	Short:   "List groups",
	Args:    cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		kind, _ := cmd.Flags().GetString("kind")
		groups, err := memoClient().GetGroups(kind)
		if err != nil {
			exitErr("listing groups", err)
		}

		if jsonOut(cmd) {
			printJSON(groups)
			return
		}
		if len(groups) == 0 {
			fmt.Println("No groups defined.")
			return
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "NAME\tKIND\tMEMBERS")
		for _, g := range groups {
			fmt.Fprintf(w, "%s\t%s\t%s\n", g.Name, g.Kind, truncateList(g.Members, 5))
		}
		w.Flush()
	},
}

var memoGroupsCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a group",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		kind, _ := cmd.Flags().GetString("kind")
		members, _ := cmd.Flags().GetStringArray("member")
		if err := memoClient().CreateGroup(args[0], kind, members); err != nil {
			exitErr("creating group", err)
		}
		fmt.Printf("Created %s group %s with %d member(s).\n", kind, args[0], len(members))
	},
}

var memoGroupsUpdateCmd = &cobra.Command{
	Use:   "update <name>",
	Short: "Rename a group or replace its members",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		kind, _ := cmd.Flags().GetString("kind")
		var newName *string
		if cmd.Flags().Changed("name") {
			v, _ := cmd.Flags().GetString("name")
			newName = &v
		}
		var members *[]string
		if cmd.Flags().Changed("member") {
			v, _ := cmd.Flags().GetStringArray("member")
			members = &v
		}
		if newName == nil && members == nil {
			fmt.Fprintln(os.Stderr, "Error: pass --name and/or --member")
			os.Exit(1)
		}
		if err := memoClient().UpdateGroup(args[0], kind, newName, members); err != nil {
			exitErr("updating group", err)
		}
		fmt.Printf("Updated group %s.\n", args[0])
	},
}

var memoGroupsDeleteCmd = &cobra.Command{
	Use:     "delete <name>",
	Aliases: []string{"rm", "remove"},
	Short:   "Delete a group (its members are kept)",
	Args:    cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		kind, _ := cmd.Flags().GetString("kind")
		if err := memoClient().DeleteGroup(args[0], kind); err != nil {
			exitErr("deleting group", err)
		}
		fmt.Printf("Deleted %s group %s.\n", kind, args[0])
	},
}

// ─── helpers ───────────────────────────────────────────────────────────────

// memoValues reads one catalog. "memo list" prints catalog keys (stations),
// while the suggestions API is keyed by data subtype (station), so accept both.
func memoValues(name string, limit int) ([]string, error) {
	memo, err := memoClient().GetMemo()
	if err != nil {
		return nil, err
	}
	values, ok := memo[name]
	if !ok {
		return memoClient().GetMemoSuggestions(name, limit)
	}
	if limit > 0 && len(values) > limit {
		values = values[:limit]
	}
	return values, nil
}

func memoClient() *client.Client {
	return client.NewClient(viper.GetString("server"))
}

func completeMemoSubtypes(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	memo, err := memoClient().GetMemo()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	var names []string
	for subtype := range memo {
		if strings.HasPrefix(subtype, toComplete) {
			names = append(names, subtype)
		}
	}
	sort.Strings(names)
	return names, cobra.ShellCompDirectiveNoFileComp
}

func exitErr(action string, err error) {
	fmt.Fprintf(os.Stderr, "Error %s: %v\n", action, err)
	os.Exit(1)
}

func jsonOut(cmd *cobra.Command) bool {
	v, _ := cmd.Flags().GetBool("json")
	return v
}

func printJSON(v any) {
	data, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(data))
}

func truncateList(items []string, max int) string {
	if len(items) == 0 {
		return "-"
	}
	if len(items) <= max {
		return strings.Join(items, ", ")
	}
	return fmt.Sprintf("%s, ... (+%d)", strings.Join(items[:max], ", "), len(items)-max)
}

func dashIfEmpty(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func init() {
	memoListCmd.Flags().Bool("json", false, "Output as JSON")
	memoGetCmd.Flags().Bool("json", false, "Output as JSON")
	memoGetCmd.Flags().Int("limit", 20, "Maximum number of values")
	memoEventsCmd.Flags().Bool("json", false, "Output as JSON")
	memoEventsCmd.Flags().Int("limit", 200, "Maximum number of events")
	memoEventsCmd.Flags().String("catalog", "", "Filter to one subtype (requires --value)")
	memoEventsCmd.Flags().String("value", "", "Filter to one value (requires --catalog)")
	memoStatsCmd.Flags().Bool("json", false, "Output as JSON")
	memoLabelsCmd.Flags().Bool("json", false, "Output as JSON")

	memoGroupsListCmd.Flags().Bool("json", false, "Output as JSON")
	memoGroupsListCmd.Flags().String("kind", "", "Only memo or want groups")
	memoGroupsCreateCmd.Flags().String("kind", "memo", "Group kind: memo or want")
	memoGroupsCreateCmd.Flags().StringArray("member", nil, "Member id, repeatable")
	memoGroupsUpdateCmd.Flags().String("kind", "memo", "Group kind: memo or want")
	memoGroupsUpdateCmd.Flags().String("name", "", "New group name")
	memoGroupsUpdateCmd.Flags().StringArray("member", nil, "Replacement member id, repeatable")
	memoGroupsDeleteCmd.Flags().String("kind", "memo", "Group kind: memo or want")

	memoGroupsCmd.AddCommand(memoGroupsListCmd)
	memoGroupsCmd.AddCommand(memoGroupsCreateCmd)
	memoGroupsCmd.AddCommand(memoGroupsUpdateCmd)
	memoGroupsCmd.AddCommand(memoGroupsDeleteCmd)

	MemoCmd.AddCommand(memoListCmd)
	MemoCmd.AddCommand(memoGetCmd)
	MemoCmd.AddCommand(memoAddCmd)
	MemoCmd.AddCommand(memoRemoveCmd)
	MemoCmd.AddCommand(memoEventsCmd)
	MemoCmd.AddCommand(memoStatsCmd)
	MemoCmd.AddCommand(memoLabelsCmd)
	MemoCmd.AddCommand(memoLabelCmd)
	MemoCmd.AddCommand(memoGroupsCmd)
}
