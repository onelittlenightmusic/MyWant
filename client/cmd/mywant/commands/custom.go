package commands

import (
	"fmt"
	"net/url"
	"os"
	"slices"
	"strings"
	"text/tabwriter"

	"mywant/client"
	mywant "mywant/engine/core"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Customs are installable packages (want types, canvas design plugins, recipes,
// icon styles) that live on the filesystem of the machine running the server.
// The install/link/registry logic itself is in engine/core/custom_store.go so
// that the server can run it too.
//
// Which machine a command acts on follows the active context: a remote backend
// is driven through /api/v1/customs, a local one is handled in-process so that
// installing works even when no server is running.

var (
	customNameFlag string
	customKindFlag string
	customForce    bool
	customNoReload bool
)

var CustomCmd = &cobra.Command{
	Use:     "custom",
	Aliases: []string{"customs"},
	Short:   "Install and uninstall MyWant customs",
	Long: `Manage customs: git repos or local directories that extend the running
MyWant system with want types, canvas design plugins, recipes or icon styles.
(To extend the CLI itself with "mywant-<name>" executables, see "mywant plugin".)

A custom is stored in ~/.mywant/customs/<name> and linked into the directory each
component kind is read from:

  custom-type  ~/.mywant/custom-types    wantType / agent YAML, scripts, card views
  design       ~/.mywant/design-plugin   canvas skins (plugin.jsx / tsx / js / ts)
  recipe       ~/.mywant/recipes         recipe YAML
  icon         ~/.mywant/icons           icon styles (not consumed by the server yet)

Installed customs are recorded in ~/.mywant/customs.yaml.

Customs live on the machine that runs the server, so these commands follow the
active context: with a remote context they act on that server through its API,
with a local one on this machine (which works even with no server running).
Switch machines the same way as every other command: --context local.

Sources accepted by "install":
  ./path/to/dir                      local directory (copied; local target only)
  https://github.com/owner/repo.git  git URL (cloned)
  owner/repo                         GitHub shorthand
  transit-plugin                     bare name -> https://github.com/` + mywant.DefaultCustomOwner + `/mywant-transit-plugin.git

Examples:
  mywant custom list
  mywant custom install transit-plugin
  mywant --context fly custom install onelittlenightmusic/mywant-transit-plugin
  mywant custom install ./my-skin --kind design --name neon
  mywant --context local custom list
  mywant custom uninstall mywant-transit-plugin`,
}

var customListCmd = &cobra.Command{
	Use:   "list",
	Short: "List installed customs",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		customs, untracked, err := listCustoms()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing customs: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Customs on %s\n\n", customTargetLabel())

		if len(customs) == 0 {
			fmt.Println("No customs installed. Use 'mywant custom install <source>'.")
		} else {
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tKINDS\tSOURCE\tPROVIDES\tSTATUS")
			for _, c := range customs {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
					c.Name, joinOrDash(c.Kinds()), shortPath(c.Source), joinOrDash(c.Provides()), c.Status)
			}
			w.Flush()
		}

		if len(untracked) > 0 {
			fmt.Println("\nNot managed by customs.yaml (installed by hand):")
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "  NAME\tKIND\tSOURCE\tPROVIDES")
			for _, u := range untracked {
				fmt.Fprintf(w, "  %s\t%s\t%s\t%s\n", u.Name, describeLocation(u), shortPath(u.Source),
					joinOrDash(u.Provides))
			}
			w.Flush()
		}
	},
}

var customInstallCmd = &cobra.Command{
	Use:   "install <source>",
	Short: "Install a custom and link its components",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if err := installCustomOnTarget(args[0]); err != nil {
			fmt.Fprintf(os.Stderr, "Error installing custom: %v\n", err)
			os.Exit(1)
		}
	},
}

var customUninstallCmd = &cobra.Command{
	Use:               "uninstall <name>",
	Aliases:           []string{"remove"},
	Short:             "Remove an installed custom and its links",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeCustomNames,
	Run: func(cmd *cobra.Command, args []string) {
		if err := uninstallCustomOnTarget(args[0]); err != nil {
			fmt.Fprintf(os.Stderr, "Error uninstalling custom: %v\n", err)
			os.Exit(1)
		}
	},
}

// ---------------------------------------------------------------------------
// target selection: remote server API vs the local filesystem
// ---------------------------------------------------------------------------

// customListEntry is a row of the untracked table, from either source.
type customListEntry struct {
	Name     string
	Kind     string
	Looks    []string
	Source   string
	Provides []string
}

// remoteServer reports the active backend URL when it points somewhere other
// than this machine. Customs live on the server's filesystem, so that is what
// decides which machine a command acts on.
func remoteServer() string {
	u, err := url.Parse(viper.GetString("server"))
	if err != nil {
		return ""
	}
	switch u.Hostname() {
	case "", "localhost", "127.0.0.1", "::1":
		return ""
	}
	return strings.TrimSuffix(viper.GetString("server"), "/")
}

func customTargetLabel() string {
	if target := remoteServer(); target != "" {
		return target
	}
	return "this machine (~/.mywant)"
}

func listCustoms() ([]mywant.CustomRecord, []customListEntry, error) {
	if remoteServer() != "" {
		resp, err := client.NewClient(viper.GetString("server")).ListCustoms()
		if err != nil {
			return nil, nil, err
		}
		entries := make([]customListEntry, 0, len(resp.Untracked))
		for _, u := range resp.Untracked {
			entries = append(entries, customListEntry{Name: u.Name, Kind: u.Kind, Looks: u.Looks, Source: u.Source})
		}
		return resp.Customs, entries, nil
	}

	reg, err := mywant.LoadCustomRegistry()
	if err != nil {
		return nil, nil, err
	}
	customs := make([]mywant.CustomRecord, 0, len(reg.Customs))
	for _, rec := range reg.Customs {
		rec.Status = rec.DeriveStatus()
		customs = append(customs, rec)
	}

	var entries []customListEntry
	for _, u := range mywant.FindUntrackedCustoms(reg) {
		wantTypes, agents := mywant.ScanCustomYAML(u.Path)
		entries = append(entries, customListEntry{
			Name:     u.Name,
			Kind:     u.Kind,
			Looks:    u.Looks,
			Source:   u.Source,
			Provides: append(wantTypes, agents...),
		})
	}
	return customs, entries, nil
}

func installCustomOnTarget(source string) error {
	if target := remoteServer(); target != "" {
		if isLocalPathSource(source) {
			return fmt.Errorf("%q is a local directory, which %s cannot read; push it to git first, or use --context local to install on this machine", source, target)
		}
		result, err := client.NewClient(viper.GetString("server")).InstallCustom(source, customNameFlag, customKindFlag, customForce)
		if err != nil {
			return err
		}
		reportRemoteInstall(target, result)
		return nil
	}

	rec, err := mywant.InstallCustom(source, customNameFlag, customKindFlag, customForce)
	if err != nil {
		return err
	}
	fmt.Printf("Installed custom %s from %s\n", rec.Name, rec.Source)
	printCustomSummary(rec.Kinds(), rec.WantTypes, rec.Agents)

	if customNoReload {
		return nil
	}
	reloadLocalWantTypes(len(rec.Agents) > 0)
	return nil
}

func uninstallCustomOnTarget(name string) error {
	if target := remoteServer(); target != "" {
		result, err := client.NewClient(viper.GetString("server")).UninstallCustom(name, customForce)
		if err != nil {
			return err
		}
		fmt.Printf("Removed custom %s from %s\n", name, target)
		if removed, ok := result["removed"].([]any); ok {
			fmt.Printf("  removed:    %d path(s)\n", len(removed))
		}
		reportReload(result)
		return nil
	}

	removed, hadAgents, err := mywant.UninstallCustom(name, customForce)
	if err != nil {
		return err
	}
	fmt.Printf("Removed custom %s (%d path(s))\n", name, len(removed))

	if customNoReload {
		return nil
	}
	reloadLocalWantTypes(hadAgents)
	return nil
}

// isLocalPathSource reports whether a source refers to a directory on this
// machine, which only a local install can copy.
func isLocalPathSource(source string) bool {
	if strings.Contains(source, "://") || strings.HasPrefix(source, "git@") {
		return false
	}
	info, err := os.Stat(source)
	return err == nil && info.IsDir()
}

func reportRemoteInstall(target string, result map[string]any) {
	name, _ := result["message"].(string)
	fmt.Printf("%s (on %s)\n", name, target)

	if custom, ok := result["custom"].(map[string]any); ok {
		var kinds []string
		if components, ok := custom["components"].([]any); ok {
			for _, c := range components {
				if m, ok := c.(map[string]any); ok {
					if kind, _ := m["kind"].(string); kind != "" && !slices.Contains(kinds, kind) {
						kinds = append(kinds, kind)
					}
				}
			}
		}
		printCustomSummary(kinds, stringsFrom(custom["want_types"]), stringsFrom(custom["agents"]))
	}
	reportReload(result)
}

// printCustomSummary reports what an install produced, without echoing paths on
// a machine the user may not even have a shell on.
func printCustomSummary(kinds, wantTypes, agents []string) {
	if len(kinds) > 0 {
		fmt.Printf("  kinds:      %s\n", strings.Join(kinds, ", "))
	}
	for _, kind := range kinds {
		if note := mywant.CustomKinds[kind].Note; note != "" {
			fmt.Printf("  note:       %s: %s\n", kind, note)
		}
	}
	if len(wantTypes) > 0 {
		fmt.Printf("  want types: %s\n", strings.Join(wantTypes, ", "))
	}
	if len(agents) > 0 {
		fmt.Printf("  agents:     %s\n", strings.Join(agents, ", "))
	}
}

// reportReload prints the want type reload the server performed for us.
func reportReload(result map[string]any) {
	if loaded, ok := result["reloaded"].(float64); ok {
		fmt.Printf("  reloaded:   %d want type(s)\n", int(loaded))
	}
	printWarnings(result["warnings"])
	if restart, _ := result["restart_needed"].(bool); restart {
		fmt.Println("Note: agent definitions are only loaded at startup - restart the server to apply them.")
	}
}

// reloadLocalWantTypes hot-reloads a locally running server, if there is one.
func reloadLocalWantTypes(hasAgents bool) {
	result, err := client.NewClient(viper.GetString("server")).ReloadWantTypes()
	if err != nil {
		fmt.Printf("Note: could not reload want types (%v). Restart the server to apply.\n", err)
		return
	}
	if msg, ok := result["message"].(string); ok {
		fmt.Printf("Reloaded want types: %s\n", msg)
	}
	printWarnings(result["warnings"])
	if hasAgents {
		fmt.Println("Note: agent definitions are only loaded at startup - run 'mywant stop && mywant start -D' to apply them.")
	}
}

func printWarnings(v any) {
	warnings, ok := v.([]any)
	if !ok || len(warnings) == 0 {
		return
	}
	fmt.Println("Warnings:")
	for _, w := range warnings {
		fmt.Printf("  - %v\n", w)
	}
}

func stringsFrom(v any) []string {
	items, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// presentation helpers
// ---------------------------------------------------------------------------

// describeLocation names the directory a custom sits in, and warns when its
// content says it belongs somewhere else (e.g. wantType YAML left in recipes/).
func describeLocation(u customListEntry) string {
	if len(u.Looks) == 0 {
		return u.Kind + " (no content?)"
	}
	if slices.Contains(u.Looks, u.Kind) {
		return u.Kind
	}
	return fmt.Sprintf("%s (content: %s)", u.Kind, strings.Join(u.Looks, ","))
}

// shortPath abbreviates this machine's home directory to "~" for display.
func shortPath(p string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" || !strings.HasPrefix(p, home) {
		return p
	}
	return "~" + strings.TrimPrefix(p, home)
}

func joinOrDash(items []string) string {
	if len(items) == 0 {
		return "-"
	}
	return strings.Join(items, ",")
}

func completeCustomNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	customs, untracked, err := listCustoms()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	var names []string
	for _, c := range customs {
		if strings.HasPrefix(c.Name, toComplete) {
			names = append(names, c.Name)
		}
	}
	for _, u := range untracked {
		if strings.HasPrefix(u.Name, toComplete) && !slices.Contains(names, u.Name) {
			names = append(names, u.Name)
		}
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

func init() {
	kinds := strings.Join(mywant.CustomKindOrder, "|")
	customInstallCmd.Flags().StringVar(&customNameFlag, "name", "", "install under this name (defaults to the repo/directory name)")
	customInstallCmd.Flags().StringVar(&customKindFlag, "kind", "", "component kinds to install, comma separated ("+kinds+"); default: auto-detect")
	customInstallCmd.Flags().BoolVar(&customForce, "force", false, "replace an existing custom or conflicting destination")
	customInstallCmd.Flags().BoolVar(&customNoReload, "no-reload", false, "do not ask a local server to reload want types")
	customUninstallCmd.Flags().BoolVar(&customForce, "force", false, "remove files that were not created by the custom")
	customUninstallCmd.Flags().BoolVar(&customNoReload, "no-reload", false, "do not ask a local server to reload want types")

	CustomCmd.AddCommand(customListCmd)
	CustomCmd.AddCommand(customInstallCmd)
	CustomCmd.AddCommand(customUninstallCmd)
}
