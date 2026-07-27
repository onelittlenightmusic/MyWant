package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"mywant/client"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// A world is a named snapshot of every non-system want. Only one is open at a
// time; opening another saves the current one first, so switching never loses
// work.

var WorldCmd = &cobra.Command{
	Use:     "world",
	Aliases: []string{"worlds", "wo"},
	Short:   "Manage worlds (named snapshots of all wants)",
	Long: `Manage worlds: named snapshots of every non-system want, stored in ~/.mywant/worlds.

Opening a world auto-saves the world that is currently open, clears the canvas,
and loads the target - so switching worlds never loses the running setup.

Examples:
  mywant world list
  mywant world open travel-demo
  mywant world save travel-demo
  mywant world export travel-demo -o travel.yaml
  mywant world import travel-demo -f travel.yaml`,
}

var worldListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"l"},
	Short:   "List saved worlds",
	Args:    cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		jsonFlag, _ := cmd.Flags().GetBool("json")

		worlds, err := client.NewClient(viper.GetString("server")).ListWorlds()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing worlds: %v\n", err)
			os.Exit(1)
		}

		if jsonFlag {
			data, _ := json.MarshalIndent(worlds, "", "  ")
			fmt.Println(string(data))
			return
		}

		if len(worlds) == 0 {
			fmt.Println("No worlds saved yet. Use 'mywant world save <name>'.")
			return
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "CURRENT\tNAME\tWANTS\tMODIFIED")
		for _, world := range worlds {
			current := ""
			if world.Current {
				current = "*"
			}
			fmt.Fprintf(w, "%s\t%s\t%d\t%s\n", current, world.Name, world.WantCount, world.ModifiedAt)
		}
		w.Flush()
	},
}

var worldOpenCmd = &cobra.Command{
	Use:               "open <name>",
	Aliases:           []string{"switch", "use"},
	Short:             "Switch to a world (auto-saves the current one)",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeWorldNames,
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		result, err := client.NewClient(viper.GetString("server")).OpenWorld(name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening world %s: %v\n", name, err)
			os.Exit(1)
		}
		fmt.Printf("Opened world %s\n", name)
		if count, ok := result["want_count"].(float64); ok {
			fmt.Printf("  wants:   %d loaded (the previous world was saved first)\n", int(count))
		}
	},
}

var worldSaveCmd = &cobra.Command{
	Use:               "save <name>",
	Short:             "Snapshot the running wants into a world without switching",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeWorldNames,
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		if err := client.NewClient(viper.GetString("server")).SaveWorld(name); err != nil {
			fmt.Fprintf(os.Stderr, "Error saving world %s: %v\n", name, err)
			os.Exit(1)
		}
		fmt.Printf("Saved world %s\n", name)
	},
}

var worldExportCmd = &cobra.Command{
	Use:               "export <name>",
	Aliases:           []string{"e"},
	Short:             "Export a world snapshot as YAML",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeWorldNames,
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		output, _ := cmd.Flags().GetString("output")

		data, err := client.NewClient(viper.GetString("server")).ExportWorld(name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error exporting world %s: %v\n", name, err)
			os.Exit(1)
		}

		if output == "" {
			fmt.Print(string(data))
			return
		}
		if err := os.WriteFile(output, data, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", output, err)
			os.Exit(1)
		}
		fmt.Printf("Exported world %s to %s\n", name, output)
	},
}

var worldImportCmd = &cobra.Command{
	Use:   "import <name>",
	Short: "Import a wants YAML file as a world",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		file, _ := cmd.Flags().GetString("file")
		if file == "" {
			fmt.Fprintln(os.Stderr, "Error: --file is required")
			os.Exit(1)
		}

		data, err := os.ReadFile(file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading %s: %v\n", file, err)
			os.Exit(1)
		}

		result, err := client.NewClient(viper.GetString("server")).ImportWorld(name, data)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error importing world %s: %v\n", name, err)
			os.Exit(1)
		}
		fmt.Printf("Imported world %s from %s\n", name, file)
		if count, ok := result["want_count"].(float64); ok {
			fmt.Printf("  wants:   %d\n", int(count))
		}
		fmt.Printf("Run 'mywant world open %s' to switch to it.\n", name)
	},
}

func completeWorldNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	worlds, err := client.NewClient(viper.GetString("server")).ListWorlds()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	var names []string
	for _, world := range worlds {
		if strings.HasPrefix(world.Name, toComplete) {
			names = append(names, world.Name)
		}
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

func init() {
	worldListCmd.Flags().Bool("json", false, "Output as JSON")
	worldExportCmd.Flags().StringP("output", "o", "", "Write to a file instead of stdout")
	worldImportCmd.Flags().StringP("file", "f", "", "Wants YAML file to import (required)")

	WorldCmd.AddCommand(worldListCmd)
	WorldCmd.AddCommand(worldOpenCmd)
	WorldCmd.AddCommand(worldSaveCmd)
	WorldCmd.AddCommand(worldExportCmd)
	WorldCmd.AddCommand(worldImportCmd)
}
