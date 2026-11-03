package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"mywant/client"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

// Memo is the catalog of remembered input values, grouped by subtype
// (destination, hotel, command, ...). Wants record what the user typed, the
// server keeps provenance (events) and usage counts (stats), and values can be
// labelled - constellations being a facade over a "constellation/<name>" label.
//
// Global state, which this command used to manage, now lives under
// "mywant state global".

var ThingCmd = &cobra.Command{
	Use: "thing",
	// "memo" is what this was called; it keeps working so a habit or a script
	// does not break on a rename.
	Aliases: []string{"things", "t", "memo", "m"},
	Short:   "Manage things (the values you have named)",
	Long: `Manage things: the values you have named, keyed by subtype.

A thing is not a note about something — it is the something: a station, a
city, an occasion. Wants point at them, and kata are about them.

Examples:
  mywant thing list                       # every kind, and how many
  mywant thing list --catalog stations    # one kind's values, newest first
  mywant thing get 中野坂上                # everything about one (a name, id or catalog::value)
  mywant thing get thg-1234 -o yaml       # its record
  mywant thing create stations 渋谷 新宿
  mywant thing update stations::新宿 --value 新宿三丁目
  mywant thing update 渋谷 --catalog places
  mywant thing delete stations::渋谷 thg-5678
  mywant thing delete --catalog stations  # every value of a kind
  mywant thing label 中野坂上 favourite=true note=駅 --remove old
  mywant thing export -o things.yaml
  mywant thing import -f things.yaml
  mywant thing events --catalog stations --value 渋谷
  mywant thing stats

Constellations are "mywant const".
`,
}

// ─── catalog ───────────────────────────────────────────────────────────────

var thingListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"l"},
	Short:   "List every kind and its values (--catalog: one kind's values)",
	Args:    cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		// One kind's values, newest first.
		if catalog, _ := cmd.Flags().GetString("catalog"); catalog != "" {
			limit, _ := cmd.Flags().GetInt("limit")
			values, err := memoValues(catalog, limit)
			if err != nil {
				exitErr("listing "+catalog, err)
			}
			if jsonOut(cmd) {
				printJSON(values)
				return
			}
			for _, v := range values {
				fmt.Println(v)
			}
			return
		}
		things, err := memoClient().GetThings()
		if err != nil {
			exitErr("listing memo", err)
		}

		// --json gets the whole thing — names, stats, naming wants and all —
		// since that is what the one GET now returns and what a script would
		// otherwise have to make four more calls to reassemble.
		if jsonOut(cmd) {
			printJSON(things)
			return
		}
		if len(things) == 0 {
			fmt.Println("Memo is empty.")
			return
		}

		byCatalog := map[string][]string{}
		named := map[string]int{}
		used := map[string]int{}
		for _, t := range things {
			byCatalog[t.Catalog] = append(byCatalog[t.Catalog], t.Value)
			if len(t.Definitions) > 0 {
				named[t.Catalog]++
			}
			used[t.Catalog] += len(t.WantIDs)
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "SUBTYPE\tCOUNT\tNAMED\tIN USE\tVALUES")
		for _, catalog := range sortedKeys(byCatalog) {
			values := byCatalog[catalog]
			fmt.Fprintf(w, "%s\t%d\t%d\t%d\t%s\n",
				catalog, len(values), named[catalog], used[catalog], truncateList(values, 5))
		}
		w.Flush()
		// The total, said rather than left to be added up. A reader counting
		// down a column of fourteen kinds to answer "how many things are
		// there?" gets it wrong often enough — and a small model reading this
		// output gets it wrong reliably.
		fmt.Printf("\nTotal: %d things in %d kinds\n", len(things), len(byCatalog))
	},
}

var thingGetCmd = &cobra.Command{
	Use:               "get <name|id|catalog::value|catalog>",
	Aliases:           []string{"g", "show"},
	Short:             "Everything about one thing (-o yaml: its record); a kind's values, given a kind",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeMemoSubtypes,
	Run: func(cmd *cobra.Command, args []string) {
		// Its record, as YAML (GET /things/{id}): by id or catalog::value, or a name.
		if out, _ := cmd.Flags().GetString("output"); out == "yaml" {
			id, err := resolveThingRef(args[0])
			if err != nil {
				exitErr("finding the thing", err)
			}
			t, err := memoClient().GetThing(id)
			if err != nil {
				exitErr("reading the thing", err)
			}
			data, _ := yaml.Marshal(t)
			fmt.Print(string(data))
			return
		}
		// An id or catalog::value names one thing.
		if strings.HasPrefix(args[0], "thg-") || strings.Contains(args[0], "::") {
			if id, err := resolveThingRef(args[0]); err == nil {
				if t, err := memoClient().GetThing(id); err == nil && showThingDetail(t.Value, jsonOut(cmd)) {
					return
				}
			}
		}
		limit, _ := cmd.Flags().GetInt("limit")
		values, err := memoValues(args[0], limit)
		if err != nil {
			exitErr("reading memo", err)
		}

		if len(values) == 0 {
			// The argument was a catalog key and there is nothing under it —
			// or, far more often, it was the name of a thing. "get 中野坂上"
			// asked for everything known about a station and was answered "no
			// values recorded", which is true of the catalog and false of the
			// board: the station is standing on it. So a name falls through to
			// the thing itself.
			if showThingDetail(args[0], jsonOut(cmd)) {
				return
			}
		}
		if jsonOut(cmd) {
			printJSON(values)
			return
		}
		if len(values) == 0 {
			// Not a thing, but maybe a want. "Nothing on the board is named
			// that" was said of a picture want standing right there, and the
			// robot that asked took it at its word and gave up — so a want by
			// that name is named, with the command that reads it.
			if place, err := findWantPlace(wantsClient(), args[0]); err == nil {
				fmt.Printf("No thing is named %q — %q is a want (type %s). Read it with: mywant wants get %s\n",
					args[0], place.name, place.wantType, place.name)
				return
			}
			fmt.Printf("No values recorded for %q, and nothing on the board is named that.\n", args[0])
			return
		}
		for _, v := range values {
			fmt.Println(v)
		}
	},
}

// showThingDetail prints everything the board knows about one thing, and says
// whether it found one.
//
// Everything, because the cost of a second call is what makes questions go
// unanswered: whatever is asking has one small window, and a first answer that
// leaves out the constellation buys a second round trip to learn it.
func showThingDetail(name string, asJSON bool) bool {
	place, err := findThingPlace(memoClient(), name)
	if err != nil {
		return false
	}
	things, err := memoClient().GetThings()
	if err != nil {
		return false
	}
	var subject client.Thing
	for _, t := range things {
		if t.ID == place.id {
			subject = t
		}
	}

	var namedBy []string
	if wants, err := wantsClient().ListWants("", nil, nil, false, true); err == nil {
		byID := map[string]string{}
		for _, w := range wants.Wants {
			byID[w.Metadata.ID] = w.Metadata.Name
		}
		for _, id := range subject.WantIDs {
			if n, ok := byID[id]; ok {
				namedBy = append(namedBy, n)
			}
		}
		sort.Strings(namedBy)
	}
	constellations := constellationNames(subject)

	if asJSON {
		printJSON(map[string]any{
			"id": place.id, "value": place.value, "kind": place.kind,
			"catalog": subject.Catalog, "onCanvas": place.onCanvas,
			"x": place.x, "y": place.y,
			"constellations": constellations, "namedBy": namedBy,
		})
		return true
	}

	fmt.Printf("%s (thing %s) | id: %s\n", place.value, place.kind, shortID(place.id))
	if place.onCanvas {
		fmt.Printf("  on the canvas at (%d, %d)\n", place.x, place.y)
	} else {
		fmt.Println("  not on the canvas")
	}
	if len(constellations) > 0 {
		fmt.Printf("  constellation (星座): %s\n", strings.Join(constellations, ", "))
	}
	if len(namedBy) > 0 {
		fmt.Printf("  named by: %s\n", strings.Join(namedBy, ", "))
	}
	return true
}

var thingCreateCmd = &cobra.Command{
	Use:               "create <catalog|subtype> <value>...",
	Aliases:           []string{"add"},
	Short:             "Create things — one for each value, of that kind",
	Args:              cobra.MinimumNArgs(2),
	ValidArgsFunction: completeMemoSubtypes,
	Run: func(cmd *cobra.Command, args []string) {
		c := memoClient()
		for _, v := range args[1:] {
			e, err := c.CreateThing(args[0], v)
			if err != nil {
				exitErr("creating "+v, err)
			}
			fmt.Printf("Created %s::%s (%s)\n", e.Catalog, e.Value, shortID(e.ID))
		}
	},
}

var thingUpdateCmd = &cobra.Command{
	Use:   "update <name|id|catalog::value> [--value v] [--catalog c]",
	Short: "Rename a thing or file it under another kind — the same thing, kept",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		value, _ := cmd.Flags().GetString("value")
		catalog, _ := cmd.Flags().GetString("catalog")
		if value == "" && catalog == "" {
			exitErr("updating", fmt.Errorf("say --value or --catalog"))
		}
		id, err := resolveThingRef(args[0])
		if err != nil {
			exitErr("finding the thing", err)
		}
		e, err := memoClient().PatchThing(id, catalog, value)
		if err != nil {
			exitErr("updating", err)
		}
		fmt.Printf("Updated %s: now %s::%s\n", shortID(e.ID), e.Catalog, e.Value)
	},
}

var thingDeleteCmd = &cobra.Command{
	Use:     "delete <name|id|catalog::value>... | --catalog <c>",
	Aliases: []string{"rm", "remove"},
	Short:   "Delete things — several at once, or every value of a kind",
	Run: func(cmd *cobra.Command, args []string) {
		c := memoClient()
		catalog, _ := cmd.Flags().GetString("catalog")
		var ids []string
		if catalog != "" {
			things, err := c.GetThings()
			if err != nil {
				exitErr("listing things", err)
			}
			for _, t := range things {
				if t.Catalog == catalog || t.Subtype == catalog {
					ids = append(ids, t.ID)
				}
			}
		}
		for _, ref := range args {
			id, err := resolveThingRef(ref)
			if err != nil {
				exitErr("finding "+ref, err)
			}
			ids = append(ids, id)
		}
		if len(ids) == 0 {
			exitErr("deleting", fmt.Errorf("nothing to delete"))
		}
		out, err := c.DeleteThings(ids)
		if err != nil {
			exitErr("deleting", err)
		}
		fmt.Printf("Deleted %d thing(s)", len(out.Deleted))
		if len(out.Missing) > 0 {
			fmt.Printf("; not found: %s", strings.Join(out.Missing, ", "))
		}
		fmt.Println()
	},
}

// resolveThingRef is the id of the thing ref names: an id ("thg-…"),
// "<catalog>::<value>", or its name as the board shows it.
func resolveThingRef(ref string) (string, error) {
	c := memoClient()
	if strings.HasPrefix(ref, "thg-") || strings.Contains(ref, "::") {
		return c.ResolveThingID(ref)
	}
	place, err := findThingPlace(c, ref)
	if err != nil {
		return "", err
	}
	return place.id, nil
}

// ─── provenance ────────────────────────────────────────────────────────────

var thingEventsCmd = &cobra.Command{
	Use:   "events",
	Short: "Show where memo values came from, newest first",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		limit, _ := cmd.Flags().GetInt("limit")
		catalog, _ := cmd.Flags().GetString("catalog")
		value, _ := cmd.Flags().GetString("value")

		events, err := memoClient().GetThingEvents(catalog, value, limit)
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

var thingStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show how often each memo value is used",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		stats, err := memoClient().GetThingStats()
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

var thingLabelCmd = &cobra.Command{
	Use:   "label <name|id|catalog::value> [key=value]... [--remove key]...",
	Short: "Set and remove a thing's labels — all in one request (see them with get -o yaml)",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		remove, _ := cmd.Flags().GetStringArray("remove")
		set := map[string]string{}
		for _, kv := range args[1:] {
			k, v, ok := strings.Cut(kv, "=")
			if !ok || k == "" {
				exitErr("reading "+kv, fmt.Errorf("write key=value"))
			}
			set[k] = v
		}
		if len(set) == 0 && len(remove) == 0 {
			exitErr("labelling", fmt.Errorf("say key=value or --remove key"))
		}
		id, err := resolveThingRef(args[0])
		if err != nil {
			exitErr("finding the thing", err)
		}
		labels, err := memoClient().PatchThingLabels(id, set, remove)
		if err != nil {
			exitErr("labelling", err)
		}
		if jsonOut(cmd) {
			printJSON(labels)
			return
		}
		keys := make([]string, 0, len(labels))
		for k := range labels {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Printf("%s=%s\n", k, labels[k])
		}
	},
}

// ─── export / import ───────────────────────────────────────────────────────

var thingExportCmd = &cobra.Command{
	Use:   "export [name|id|catalog::value]... [-o file]",
	Short: "Write things out as YAML — every one, or those named",
	Run: func(cmd *cobra.Command, args []string) {
		var ids []string
		for _, ref := range args {
			id, err := resolveThingRef(ref)
			if err != nil {
				exitErr("finding "+ref, err)
			}
			ids = append(ids, id)
		}
		data, err := memoClient().ExportThings(ids)
		if err != nil {
			exitErr("exporting", err)
		}
		writeOut(cmd, data)
	},
}

var thingImportCmd = &cobra.Command{
	Use:   "import -f <file.yaml>",
	Short: "Read things in from YAML — matched by kind and value, their labels laid on",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		ids, err := memoClient().ImportThings(readIn(cmd))
		if err != nil {
			exitErr("importing", err)
		}
		fmt.Printf("Imported %d thing(s)\n", len(ids))
	},
}

// writeOut writes data to -o, or to stdout.
func writeOut(cmd *cobra.Command, data []byte) {
	if out, _ := cmd.Flags().GetString("output"); out != "" && out != "-" {
		if err := os.WriteFile(out, data, 0o644); err != nil {
			exitErr("writing "+out, err)
		}
		fmt.Printf("Wrote %s\n", out)
		return
	}
	fmt.Print(string(data))
}

// readIn reads -f, or stdin with "-".
func readIn(cmd *cobra.Command) []byte {
	file, _ := cmd.Flags().GetString("file")
	if file == "" {
		exitErr("reading", fmt.Errorf("--file (-f) is required"))
	}
	var data []byte
	var err error
	if file == "-" {
		data, err = io.ReadAll(os.Stdin)
	} else {
		data, err = os.ReadFile(file)
	}
	if err != nil {
		exitErr("reading "+file, err)
	}
	return data
}

// ─── helpers ───────────────────────────────────────────────────────────────

// memoValues reads one catalog. "memo list" prints catalog keys (stations),
// while the suggestions API is keyed by data subtype (station), so accept both.
func memoValues(name string, limit int) ([]string, error) {
	memo, err := memoClient().GetThingsByCatalog()
	if err != nil {
		return nil, err
	}
	values, ok := memo[name]
	if !ok {
		return memoClient().GetThingSuggestions(name, limit)
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
	memo, err := memoClient().GetThingsByCatalog()
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
	thingListCmd.Flags().Bool("json", false, "Output as JSON")
	thingListCmd.Flags().String("catalog", "", "Only this kind's values, newest first")
	thingListCmd.Flags().Int("limit", 0, "With --catalog: at most this many values")
	thingGetCmd.Flags().Bool("json", false, "Output as JSON")
	thingGetCmd.Flags().StringP("output", "o", "", "yaml: the thing's record")
	thingGetCmd.Flags().Int("limit", 20, "Maximum number of values")
	thingUpdateCmd.Flags().String("value", "", "Its new name")
	thingUpdateCmd.Flags().String("catalog", "", "The kind to file it under")
	thingDeleteCmd.Flags().String("catalog", "", "Delete every value of this kind")
	thingLabelCmd.Flags().StringArray("remove", nil, "Label key to remove, repeatable")
	thingLabelCmd.Flags().Bool("json", false, "Output as JSON")
	thingExportCmd.Flags().StringP("output", "o", "", "File to write (default stdout)")
	thingImportCmd.Flags().StringP("file", "f", "", "YAML file (- for stdin)")
	thingEventsCmd.Flags().Bool("json", false, "Output as JSON")
	thingEventsCmd.Flags().Int("limit", 200, "Maximum number of events")
	thingEventsCmd.Flags().String("catalog", "", "Filter to one subtype (requires --value)")
	thingEventsCmd.Flags().String("value", "", "Filter to one value (requires --catalog)")
	thingStatsCmd.Flags().Bool("json", false, "Output as JSON")

	ThingCmd.AddCommand(thingListCmd, thingGetCmd, thingCreateCmd, thingUpdateCmd, thingDeleteCmd,
		thingLabelCmd, thingExportCmd, thingImportCmd, thingEventsCmd, thingStatsCmd)
}
