package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// The CLI's own table of contents, in a form a program can read.
//
// Anything that wants to drive this CLI — the mywant-cli skill, the on-device
// agent's tool, a generated doc — needs to know what commands exist and what
// each one takes. Written by hand, that list is a copy of this binary that goes
// stale the day a command is added; every such copy has gone stale at least
// once. So the binary says it itself: the tree below IS the command tree, read
// out of Cobra at the moment of asking, and a command added anywhere appears
// here with no further work.
//
//	mywant commands           what can be run, one line each
//	mywant commands --json    the same, with flags and arguments, for a program
//	mywant commands --json --safe-only   only the ones that read

// CommandInfo is one runnable command, named by the whole path you would type.
type CommandInfo struct {
	// "wants list", "world export" — exactly what follows `mywant`.
	Path string `json:"path"`
	// One line, as the command itself describes it.
	Short string `json:"short,omitempty"`
	// The usage line, which is where the positional arguments are named:
	// "get <want-id>".
	Use string `json:"use,omitempty"`
	// Longer help, when the command has any worth reading.
	Long string `json:"long,omitempty"`
	// Examples the command carries, verbatim.
	Example string `json:"example,omitempty"`
	// Its own flags (not the global ones, which apply to everything).
	Flags []FlagInfo `json:"flags,omitempty"`
	// Whether running it only reads. See readOnlyCommand.
	ReadOnly bool `json:"readOnly"`
}

// FlagInfo is one flag of one command.
type FlagInfo struct {
	Name      string `json:"name"`
	Shorthand string `json:"shorthand,omitempty"`
	Usage     string `json:"usage,omitempty"`
	Default   string `json:"default,omitempty"`
	Type      string `json:"type,omitempty"`
}

// The verbs that only look. A caller handing this CLI to an agent wants to know
// which commands cannot change anything, and the honest answer is in the verb:
// this CLI names its commands after what they do, and every one of these reads.
//
// Matched on the LAST word of the path, so "wants list" and "world list" are
// both reads without listing either of them here. Anything not on this list is
// treated as a write — an unknown verb is not a safe one.
var readOnlyVerbs = map[string]bool{
	"list": true, "get": true, "show": true, "status": true, "ps": true,
	"logs": true, "export": true, "version": true, "help": true, "search": true,
	"stats": true, "events": true, "labels": true, "groups": true,
	"current-context": true, "get-contexts": true, "commands": true,
	"where": true, "board": true, "names": true, "relations": true, "connections": true,
	// Two that do change something, and are here anyway: what the robot is
	// saying, and where it is standing. Both are things the robot changes by
	// itself every minute as it wanders, neither is kept, and answering "where
	// is 新宿?" by going to stand on it is the whole point of being able to
	// ask. Nothing on the board is created, moved or deleted by either.
	"say": true, "point": true,
}

func readOnlyCommand(path string) bool {
	fields := strings.Fields(path)
	if len(fields) == 0 {
		return false
	}
	return readOnlyVerbs[fields[len(fields)-1]]
}

var (
	commandsJSON     bool
	commandsSafeOnly bool
)

// CommandsCmd prints this CLI's command tree.
var CommandsCmd = &cobra.Command{
	Use:   "commands",
	Short: "List every command this CLI has, for a person or a program",
	Long: `Walks this binary's own command tree and prints it.

Text (the default) is one line per command. --json adds each command's flags,
its usage line and whether it only reads, which is what a skill or an agent
needs to drive the CLI without a hand-written copy of this list going stale.`,
	Example: `  mywant commands
  mywant commands --json
  mywant commands --json --safe-only`,
	Run: func(cmd *cobra.Command, args []string) {
		infos := collectCommands(cmd.Root(), "")
		if commandsSafeOnly {
			kept := infos[:0]
			for _, info := range infos {
				if info.ReadOnly {
					kept = append(kept, info)
				}
			}
			infos = kept
		}
		sort.Slice(infos, func(i, j int) bool { return infos[i].Path < infos[j].Path })

		if commandsJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			if err := enc.Encode(infos); err != nil {
				fmt.Fprintf(os.Stderr, "Error encoding commands: %v\n", err)
				os.Exit(1)
			}
			return
		}
		for _, info := range infos {
			mark := " "
			if info.ReadOnly {
				mark = "r"
			}
			fmt.Printf("%s  %-28s %s\n", mark, info.Path, info.Short)
		}
	},
}

// collectCommands walks the tree, skipping what nobody can usefully run: the
// root itself, hidden commands, the shell-completion machinery, and any group
// that is only a heading for its children (`mywant thing` alone does nothing).
func collectCommands(cmd *cobra.Command, prefix string) []CommandInfo {
	var infos []CommandInfo
	for _, child := range cmd.Commands() {
		if child.Hidden || child.Name() == "completion" || child.Name() == "help" {
			continue
		}
		path := strings.TrimSpace(prefix + " " + child.Name())
		if child.Runnable() {
			infos = append(infos, CommandInfo{
				Path:     path,
				Short:    child.Short,
				Use:      child.Use,
				Long:     strings.TrimSpace(child.Long),
				Example:  strings.TrimSpace(child.Example),
				Flags:    collectFlags(child),
				ReadOnly: readOnlyCommand(path),
			})
		}
		infos = append(infos, collectCommands(child, path)...)
	}
	return infos
}

func collectFlags(cmd *cobra.Command) []FlagInfo {
	var flags []FlagInfo
	cmd.LocalNonPersistentFlags().VisitAll(func(f *pflag.Flag) {
		if f.Hidden {
			return
		}
		flags = append(flags, FlagInfo{
			Name:      f.Name,
			Shorthand: f.Shorthand,
			Usage:     f.Usage,
			Default:   f.DefValue,
			Type:      f.Value.Type(),
		})
	})
	return flags
}

func init() {
	CommandsCmd.Flags().BoolVar(&commandsJSON, "json", false, "Print the tree as JSON, with flags and arguments")
	CommandsCmd.Flags().BoolVar(&commandsSafeOnly, "safe-only", false, "Only the commands that read (see readOnly in the JSON)")
}
