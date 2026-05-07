package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var PluginCmd = &cobra.Command{
	Use:   "plugin",
	Short: "Manage mywant plugins",
	Long: `Manage mywant plugins.

Plugins are executables named "mywant-<name>" found in PATH.
They are invoked transparently: "mywant <name> [args]" runs "mywant-<name> [args]".`,
}

var pluginListCmd = &cobra.Command{
	Use:   "list",
	Short: "List installed plugins",
	Run: func(cmd *cobra.Command, args []string) {
		plugins := discoverPlugins()
		if len(plugins) == 0 {
			fmt.Println("No plugins found. Install a plugin by placing a mywant-<name> executable in your PATH.")
			return
		}
		fmt.Printf("%-20s %s\n", "NAME", "PATH")
		fmt.Println(strings.Repeat("-", 60))
		for _, p := range plugins {
			fmt.Printf("%-20s %s\n", p.name, p.path)
		}
	},
}

type pluginInfo struct {
	name string
	path string
}

func discoverPlugins() []pluginInfo {
	var plugins []pluginInfo
	seen := map[string]bool{}

	pathDirs := filepath.SplitList(os.Getenv("PATH"))
	for _, dir := range pathDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if !strings.HasPrefix(name, "mywant-") {
				continue
			}
			pluginName := strings.TrimPrefix(name, "mywant-")
			if seen[pluginName] {
				continue
			}

			fullPath := filepath.Join(dir, name)
			info, err := e.Info()
			if err != nil {
				continue
			}
			if info.Mode()&0111 == 0 {
				continue
			}

			seen[pluginName] = true
			plugins = append(plugins, pluginInfo{name: pluginName, path: fullPath})
		}
	}
	return plugins
}

func init() {
	PluginCmd.AddCommand(pluginListCmd)
}
