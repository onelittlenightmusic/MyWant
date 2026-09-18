package commands

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"time"
)

// The commands that are not in this binary.
//
// `mywant gui tile set <want> <x> <y>` moves a tile on the canvas. It has
// existed for months, and nothing driving this CLI has ever been able to find
// it: plugins are dispatched by exec (see main.go), so they are not in the
// command tree `mywant commands` walks. An agent reading the tree therefore
// learns everything about the board except how to arrange it, and answers "I
// cannot move tiles" about a verb sitting on the same PATH.
//
// So the tree is asked for, not just walked. Every mywant-<name> on PATH is
// asked for its own `commands --json`, and what comes back is merged in under
// the name you would type — "gui tile set". A plugin that does not answer is
// skipped in silence: not supporting it is not an error, it is just a plugin
// with nothing to say.

// pluginTimeout is how long one plugin gets to describe itself. Generous for
// printing JSON, short enough that a hung binary does not hang this command.
const pluginTimeout = 5 * time.Second

// pluginCommandInfo is CommandInfo as it arrives from a plugin. ReadOnly is a
// pointer so "said false" and "said nothing" stay different: a plugin that
// classifies its own commands is believed, and one that does not falls back to
// the verb heuristic here, which knows nothing about that plugin's verbs.
type pluginCommandInfo struct {
	Path     string     `json:"path"`
	Risk     string     `json:"risk"`
	Short    string     `json:"short"`
	Use      string     `json:"use"`
	Long     string     `json:"long"`
	Example  string     `json:"example"`
	Flags    []FlagInfo `json:"flags"`
	ReadOnly *bool      `json:"readOnly"`
}

// pluginCommands asks every plugin on PATH what it can do. The plugins are the
// ones `mywant plugin list` shows — one discovery, so the two never disagree.
func pluginCommands() []CommandInfo {
	var infos []CommandInfo
	for _, p := range discoverPlugins() {
		infos = append(infos, commandsOfPlugin(p.name, p.path)...)
	}
	return infos
}

// commandsOfPlugin asks one plugin for its tree, and returns nothing if it has
// no answer to give.
func commandsOfPlugin(name, path string) []CommandInfo {
	ctx, cancel := context.WithTimeout(context.Background(), pluginTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "commands", "--json").Output()
	if err != nil {
		return nil
	}
	var raw []pluginCommandInfo
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil
	}

	infos := make([]CommandInfo, 0, len(raw))
	for _, r := range raw {
		if strings.TrimSpace(r.Path) == "" || r.Path == "commands" {
			continue
		}
		// Under the name you would type it: the plugin says "tile set", and
		// what runs it is `mywant gui tile set`.
		full := name + " " + r.Path
		readOnly := readOnlyCommand(full)
		if r.ReadOnly != nil {
			readOnly = *r.ReadOnly
		}
		// The plugin's own reckoning of what its verbs cost, when it has one:
		// `gui i char delete` removes a cursor and `gui show` only navigates,
		// and this binary's verb list knows neither.
		risk := r.Risk
		if risk != "read" && risk != "change" && risk != "destroy" {
			risk = commandRisk(full)
			if readOnly {
				risk = "read"
			}
		}
		infos = append(infos, CommandInfo{
			Path:     full,
			Short:    r.Short,
			Use:      r.Use,
			Long:     strings.TrimSpace(r.Long),
			Example:  strings.TrimSpace(r.Example),
			Flags:    r.Flags,
			ReadOnly: readOnly,
			Risk:     risk,
		})
	}
	return infos
}
