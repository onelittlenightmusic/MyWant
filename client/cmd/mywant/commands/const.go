package commands

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"mywant/client"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// Constellations (星座): things and wants named as one — an order, tied one
// after another and drawn as a chain, or a hub's capture, drawn from its tile
// out to each member. Their own command, with the same verbs as `mywant
// thing`; they were "mywant things constellations", though a constellation is
// not a kind of thing and holds wants as well.
var ConstCmd = &cobra.Command{
	Use:     "const",
	Aliases: []string{"constellation", "constellations"},
	Short:   "Manage constellations (星座) — things and wants named as one",
	Long: `Manage constellations (星座): things and wants named as one.

An order is tied one after another and drawn as a chain; a hub (--hub) is a
capture into one tile, drawn from it out to each member.

Examples:
  mywant const list
  mywant const get 中央線 -o yaml
  mywant const create 中央線 --member stations::新宿 --member stations::中野 --color '#fb7185'
  mywant const create スマートゴルフ --hub want-1234 --member thg-5678
  mywant const update 中央線 --name 中央快速線 --color '#f97316'
  mywant const update 中央線 --member stations::新宿 --member stations::荻窪
  mywant const delete 中央線 丸の内線
  mywant const export -o constellations.yaml
  mywant const import -f constellations.yaml`,
}

var constListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"l", "ls"},
	Short:   "List constellations, their kind and members",
	Args:    cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		kind, _ := cmd.Flags().GetString("kind")
		list, err := memoClient().GetConstellations(kind)
		if err != nil {
			exitErr("listing constellations", err)
		}
		if jsonOut(cmd) {
			printJSON(list)
			return
		}
		if len(list) == 0 {
			fmt.Println("No constellations defined.")
			return
		}
		// Members are kept as ids; the name comes first and the id after it,
		// shortened — the name is what a question is about, the id what a
		// follow-up command takes.
		names := memberNames()
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "NAME\tKIND\tHUB\tMEMBERS")
		for _, g := range list {
			hub := "-"
			if g.Capture != "" {
				hub = nameOrID(names, g.Capture)
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", g.Name, g.Kind, hub, truncateList(memberLabels(names, g.Members, g.Capture), 8))
		}
		w.Flush()
	},
}

var constGetCmd = &cobra.Command{
	Use:   "get <name>",
	Short: "One constellation: its hub, colour and members (-o yaml: its record)",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		g, err := memoClient().GetConstellation(args[0])
		if err != nil {
			exitErr("reading the constellation", err)
		}
		if out, _ := cmd.Flags().GetString("output"); out == "yaml" {
			data, _ := yaml.Marshal(g)
			fmt.Print(string(data))
			return
		}
		if jsonOut(cmd) {
			printJSON(g)
			return
		}
		names := memberNames()
		form := "order (a chain)"
		if g.Capture != "" {
			form = "hub: " + nameOrID(names, g.Capture)
		}
		fmt.Printf("%s (%s) | %s\n", g.Name, g.Kind, form)
		if g.Color != "" {
			fmt.Printf("  colour: %s\n", g.Color)
		}
		for _, m := range memberLabels(names, g.Members, g.Capture) {
			fmt.Printf("  - %s\n", m)
		}
	},
}

var constCreateCmd = &cobra.Command{
	Use:   "create <name> --member <ref>... [--hub <ref>] [--color #rrggbb]",
	Short: "Create a constellation (a hub with --hub, else an order)",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ch := constChange(cmd)
		if ch.Members == nil {
			exitErr("creating", fmt.Errorf("give its members with --member"))
		}
		if err := memoClient().CreateConstellation(args[0], ch); err != nil {
			exitErr("creating the constellation", err)
		}
		fmt.Printf("Created constellation %s\n", args[0])
	},
}

var constUpdateCmd = &cobra.Command{
	Use:   "update <name> [--name n] [--member <ref>...] [--hub <ref>|--order] [--color #rrggbb] [--label k=v...|--clear-labels]",
	Short: "Change what is given: its name, members (replaced), hub, colour or labels (replaced)",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ch := constChange(cmd)
		if newName, _ := cmd.Flags().GetString("name"); newName != "" {
			ch.Name = &newName
		}
		if order, _ := cmd.Flags().GetBool("order"); order {
			empty := ""
			ch.Capture = &empty
		}
		if ch.Name == nil && ch.Members == nil && ch.Color == nil && ch.Capture == nil && ch.Labels == nil {
			exitErr("updating", fmt.Errorf("say --name, --member, --hub, --order, --color, --label or --clear-labels"))
		}
		if err := memoClient().UpdateConstellation(args[0], ch); err != nil {
			exitErr("updating the constellation", err)
		}
		fmt.Printf("Updated constellation %s\n", args[0])
	},
}

var constDeleteCmd = &cobra.Command{
	Use:     "delete <name>...",
	Aliases: []string{"rm", "remove"},
	Short:   "Delete constellations — several at once (their members are kept)",
	Args:    cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		out, err := memoClient().DeleteConstellations(args)
		if err != nil {
			exitErr("deleting", err)
		}
		fmt.Printf("Deleted %d constellation(s)", len(out.Deleted))
		if len(out.Missing) > 0 {
			fmt.Printf("; not found: %s", strings.Join(out.Missing, ", "))
		}
		fmt.Println()
	},
}

var constExportCmd = &cobra.Command{
	Use:   "export [name]... [-o file]",
	Short: "Write constellations out as YAML — every one, or those named",
	Run: func(cmd *cobra.Command, args []string) {
		data, err := memoClient().ExportConstellations(args)
		if err != nil {
			exitErr("exporting", err)
		}
		writeOut(cmd, data)
	},
}

var constImportCmd = &cobra.Command{
	Use:   "import -f <file.yaml>",
	Short: "Read constellations in from YAML — made or joined by name, with colour, art and hub",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		names, err := memoClient().ImportConstellations(readIn(cmd))
		if err != nil {
			exitErr("importing", err)
		}
		fmt.Printf("Imported %d constellation(s): %s\n", len(names), strings.Join(names, ", "))
	},
}

// constChange is what --member, --hub and --color say, members and the hub
// read as a thing's name, id or catalog::value — or a want's id.
func constChange(cmd *cobra.Command) client.ConstellationChange {
	var ch client.ConstellationChange
	if refs, _ := cmd.Flags().GetStringArray("member"); len(refs) > 0 {
		members := make([]string, 0, len(refs))
		for _, r := range refs {
			members = append(members, memberRef(r))
		}
		ch.Members = &members
	}
	if hub, _ := cmd.Flags().GetString("hub"); hub != "" {
		id := memberRef(hub)
		ch.Capture = &id
		// The hub is a member too, as a tile caught into is.
		if ch.Members != nil && !containsString(*ch.Members, id) {
			m := append(*ch.Members, id)
			ch.Members = &m
		}
	}
	if color, _ := cmd.Flags().GetString("color"); color != "" {
		ch.Color = &color
	}
	if labels := flagLabels(cmd); labels != nil {
		ch.Labels = &labels
	}
	if clear, _ := cmd.Flags().GetBool("clear-labels"); clear {
		none := map[string]string{}
		ch.Labels = &none
	}
	return ch
}

// memberRef is a member's id: a want's id as it is, a thing's resolved.
func memberRef(ref string) string {
	if strings.HasPrefix(ref, "want-") || ref == "robot" {
		return ref
	}
	id, err := resolveThingRef(ref)
	if err != nil {
		exitErr("finding "+ref, err)
	}
	return id
}

func containsString(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// memberNames is every thing's and want's name by id.
func memberNames() map[string]string {
	names := map[string]string{}
	if things, err := memoClient().GetThings(); err == nil {
		for _, t := range things {
			names[t.ID] = t.Value
		}
	}
	if wants, err := wantsClient().ListWants("", nil, nil, false, true); err == nil {
		for _, w := range wants.Wants {
			names[w.Metadata.ID] = w.Metadata.Name
		}
	}
	return names
}

func nameOrID(names map[string]string, id string) string {
	if n, ok := names[id]; ok {
		return fmt.Sprintf("%s (%s)", n, shortID(id))
	}
	return shortID(id)
}

// memberLabels is the members as a person reads them, the hub left out.
func memberLabels(names map[string]string, members []string, hub string) []string {
	out := []string{}
	for _, m := range members {
		if m != hub {
			out = append(out, nameOrID(names, m))
		}
	}
	sort.Strings(out)
	return out
}

func init() {
	constListCmd.Flags().Bool("json", false, "Output as JSON")
	constListCmd.Flags().String("kind", "", "Only thing or want constellations")
	constGetCmd.Flags().Bool("json", false, "Output as JSON")
	constGetCmd.Flags().StringP("output", "o", "", "yaml: its record")
	for _, c := range []*cobra.Command{constCreateCmd, constUpdateCmd} {
		c.Flags().StringArray("member", nil, "Member: a thing's name, id or catalog::value, or a want's id (repeatable)")
		c.Flags().String("hub", "", "Its hub: the tile it is a capture into (a thing or a want)")
		c.Flags().String("color", "", "Its colour, #rrggbb")
		c.Flags().StringArray("label", nil, "Its own label k=v (repeatable; on update, they replace its labels)")
	}
	constUpdateCmd.Flags().Bool("clear-labels", false, "Clear its own labels")
	constUpdateCmd.Flags().String("name", "", "Its new name")
	constUpdateCmd.Flags().Bool("order", false, "No hub: an order, drawn as a chain")
	constExportCmd.Flags().StringP("output", "o", "", "File to write (default stdout)")
	constImportCmd.Flags().StringP("file", "f", "", "YAML file (- for stdin)")
	ConstCmd.AddCommand(constListCmd, constGetCmd, constCreateCmd, constUpdateCmd, constDeleteCmd, constExportCmd, constImportCmd)
}
