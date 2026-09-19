package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

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
// The plugins on PATH are part of that tree, even though they are separate
// binaries reached by exec: each is asked for its own commands and answers
// under the name you would type (`gui tile set`). See commands_plugins.go — a
// verb nothing could find is a verb nobody has.
//
//	mywant commands           what can be run, one line each
//	mywant commands --json    the same, with flags and arguments, for a program
//	mywant commands --json --safe-only   only the ones that read
//	mywant commands --core-only          leave the plugins out

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
	// What running it costs if it was the wrong command: "read", "change" or
	// "destroy". See commandRisk.
	Risk string `json:"risk"`
	// What it is for: "observe" (it tells you something) or "control" (it
	// changes something). See commandKind.
	Kind string `json:"kind"`
	// Whether it concerns what is drawn on the canvas — the wants, things,
	// tiles and constellations somebody is looking at — as opposed to the
	// server behind it. See commandCanvas.
	Canvas bool `json:"canvas"`
}

// Annotations a command can set to say what it is, when the defaults below
// would get it wrong.
//
//	Annotations: map[string]string{AnnotationKind: "observe", AnnotationCanvas: "true"}
const (
	AnnotationKind   = "mywant.io/kind"
	AnnotationCanvas = "mywant.io/canvas"
)

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
	// Asking for something in words is not a read — the goal it makes may run
	// anything — but it is not a write either: `do` creates a want that decides,
	// and that want stops at whatever cannot be undone. Left as a change.
}

func readOnlyCommand(path string) bool {
	fields := strings.Fields(path)
	if len(fields) == 0 {
		return false
	}
	return readOnlyVerbs[fields[len(fields)-1]]
}

// The verbs that take something away.
//
// "Only reads" and "changes something" is the wrong place to stop when the
// caller is an agent acting on what somebody said out loud. Moving a tile and
// deleting a want are both writes, and they are nothing alike: one is a
// gesture you can take back (see `mywant undo`), the other is a want that is
// gone, along with everything it was keeping. A misheard name costs a shrug in
// the first case and the afternoon's work in the second.
//
// So the answer has three parts, and the third one is what anything driving
// this CLI should stop and ask about. Matched on the last word of the path, as
// readOnlyVerbs is.
var destructiveVerbs = map[string]bool{
	"delete": true, "remove": true, "uninstall": true, "clear": true,
	"reset": true, "stop": true, "suspend": true, "disconnect": true,
	// Switching or replacing the world puts every want on the board back to
	// what a snapshot says, which is a deletion of everything since.
	"open": true, "import": true,
}

// commandRisk says what running a command costs if it was the wrong one:
// "read" (nothing), "change" (something, reversibly) or "destroy" (something
// that is not coming back).
// kindOf is commandKind with the command's own say, when it has one.
func kindOf(cmd *cobra.Command, path string) string {
	if cmd != nil {
		if value, ok := cmd.Annotations[AnnotationKind]; ok && (value == "observe" || value == "control") {
			return value
		}
	}
	return commandKind(path)
}

// shortSummary keeps a description to one readable column.
func shortSummary(text string) string {
	text = strings.TrimSpace(text)
	if runes := []rune(text); len(runes) > 60 {
		return string(runes[:60]) + "…"
	}
	return text
}

func commandRisk(path string) string {
	fields := strings.Fields(path)
	if len(fields) == 0 {
		return "change"
	}
	verb := fields[len(fields)-1]
	switch {
	case readOnlyVerbs[verb]:
		return "read"
	case destructiveVerbs[verb]:
		return "destroy"
	default:
		return "change"
	}
}

// The groups whose commands are about what is on the canvas.
//
// A caller driving this CLI on somebody's behalf is nearly always working on
// the board — the tiles, the things, where they stand, what they hold — and
// nearly never on the server's own administration. Both are here, and saying
// which is which lets a caller start with the half it means.
//
// By group, because that is how this CLI is arranged, with per-command
// annotations (AnnotationCanvas) for the exceptions: `gui tile set` moves a
// tile and belongs with the board, while the rest of `gui` drives a screen.
var canvasGroups = map[string]bool{
	"board": true, "relations": true, "point": true, "undo": true,
	"thing": true, "wants": true, "world": true, "types": true,
}

// commandKind says what a command is for: telling you something, or changing
// something. The verb already says it — this only gives it a name.
func commandKind(path string) string {
	if readOnlyCommand(path) {
		return "observe"
	}
	return "control"
}

// commandCanvas reports whether a command is about what is drawn.
func commandCanvas(cmd *cobra.Command, path string) bool {
	if cmd != nil {
		if value, ok := cmd.Annotations[AnnotationCanvas]; ok {
			return value == "true"
		}
	}
	fields := strings.Fields(path)
	if len(fields) == 0 {
		return false
	}
	return canvasGroups[fields[0]]
}

var (
	commandsJSON     bool
	commandsSafeOnly bool
	commandsCoreOnly bool
	commandsRisk     string
	commandsKind     string
	commandsCanvas   bool
)

// CommandsCmd prints this CLI's command tree.
var CommandsCmd = &cobra.Command{
	Use:   "commands",
	Short: "List every command this CLI has, for a person or a program",
	Long: `Walks this binary's own command tree and prints it.

Text (the default) is one line per command with its labels: what it is for
(observe / control), whether it is about what is drawn on the canvas, and what
it costs if it was the wrong one (read / change / destroy).

--json adds each command's flags and its usage line, which is what a skill or
an agent needs to drive the CLI without a hand-written copy of this list going
stale. The on-device robot builds its own menu from exactly these labels: the
canvas commands first, the rest of the CLI only when it asks.`,
	Example: `  mywant commands
  mywant commands --canvas --kind control
  mywant commands --json
  mywant commands --json --safe-only`,
	Run: func(cmd *cobra.Command, args []string) {
		infos := collectCommands(cmd.Root(), "")
		if !commandsCoreOnly {
			infos = append(infos, pluginCommands()...)
		}
		if commandsRisk != "" {
			kept := infos[:0]
			for _, info := range infos {
				if info.Risk == commandsRisk {
					kept = append(kept, info)
				}
			}
			infos = kept
		}
		if commandsKind != "" {
			kept := infos[:0]
			for _, info := range infos {
				if info.Kind == commandsKind {
					kept = append(kept, info)
				}
			}
			infos = kept
		}
		if commandsCanvas {
			kept := infos[:0]
			for _, info := range infos {
				if info.Canvas {
					kept = append(kept, info)
				}
			}
			infos = kept
		}
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
		// The labels, in columns, because they are the reason to read this
		// list: what a command is for, whether it touches what is drawn, and
		// what it costs if it was the wrong one. Anything driving this CLI
		// sorts by them, and a person deciding what to hand over should be
		// able to see the same thing without parsing JSON.
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "KIND\tWHERE\tRISK\tCOMMAND\tWHAT IT DOES")
		for _, info := range infos {
			where := "—"
			if info.Canvas {
				where = "canvas"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				info.Kind, where, info.Risk, info.Path, shortSummary(info.Short))
		}
		w.Flush()
		fmt.Printf("\nTotal: %d commands. observe = it tells you something, control = it changes something;\n", len(infos))
		fmt.Println("canvas = it is about what is drawn on the board; destroy = it cannot be undone.")
		fmt.Println("Narrow it with --kind observe|control, --canvas, --risk read|change|destroy.")
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
				Risk:     commandRisk(path),
				Kind:     kindOf(child, path),
				Canvas:   commandCanvas(child, path),
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
	CommandsCmd.Flags().BoolVar(&commandsCoreOnly, "core-only", false, "Leave out the plugins on PATH, which are asked for their own commands")
	CommandsCmd.Flags().StringVar(&commandsRisk, "risk", "", "Only commands of this risk: read, change or destroy (see risk in the JSON)")
	CommandsCmd.Flags().StringVar(&commandsKind, "kind", "", "Only commands of this kind: observe (they tell you something) or control (they change something)")
	CommandsCmd.Flags().BoolVar(&commandsCanvas, "canvas", false, "Only the commands about what is drawn on the canvas")
}
