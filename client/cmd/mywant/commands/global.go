package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"mywant/client"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

// ─── memo (global state) ───────────────────────────────────────────────────

var MemoCmd = &cobra.Command{
	Use:     "memo",
	Aliases: []string{"m"},
	Short:   "Manage global state (memo)",
	Long:    `View and clear global state persisted by wants via StoreGlobalState.`,
}

var memoGetCmd = &cobra.Command{
	Use:     "get",
	Aliases: []string{"g", "show"},
	Short:   "Display current global state",
	Run: func(cmd *cobra.Command, args []string) {
		jsonFlag, _ := cmd.Flags().GetBool("json")
		c := client.NewClient(viper.GetString("server"))
		resp, err := c.GetGlobalState()
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		if jsonFlag {
			data, _ := json.MarshalIndent(resp.State, "", "  ")
			fmt.Println(string(data))
			return
		}

		fmt.Printf("Global State  (updated: %s)\n", resp.Timestamp)
		fmt.Println(strings.Repeat("─", 50))
		if len(resp.State) == 0 {
			fmt.Println("(empty)")
			return
		}
		printFlatMap(resp.State, "")
	},
}

var memoClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear all global state",
	Run: func(cmd *cobra.Command, args []string) {
		yes, _ := cmd.Flags().GetBool("yes")
		if !yes {
			fmt.Print("Clear all global state? [y/N]: ")
			var answer string
			fmt.Scanln(&answer)
			if strings.ToLower(strings.TrimSpace(answer)) != "y" {
				fmt.Println("Cancelled.")
				return
			}
		}
		c := client.NewClient(viper.GetString("server"))
		if err := c.DeleteGlobalState(); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Global state cleared.")
	},
}

// ─── params (global parameters) ───────────────────────────────────────────

var ParamsCmd = &cobra.Command{
	Use:     "params",
	Aliases: []string{"pa"},
	Short:   "Manage global parameters",
	Long:    `View and edit global parameters stored in ~/.mywant/parameters.yaml.`,
}

var paramsGetCmd = &cobra.Command{
	Use:     "get",
	Aliases: []string{"g", "show", "list"},
	Short:   "Display all global parameters",
	Run: func(cmd *cobra.Command, args []string) {
		jsonFlag, _ := cmd.Flags().GetBool("json")
		c := client.NewClient(viper.GetString("server"))
		resp, err := c.GetGlobalParameters()
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		if jsonFlag {
			data, _ := json.MarshalIndent(resp.Parameters, "", "  ")
			fmt.Println(string(data))
			return
		}

		fmt.Printf("Global Parameters  (%d)\n", resp.Count)
		fmt.Println(strings.Repeat("─", 50))
		if resp.Count == 0 {
			fmt.Println("(empty)")
			return
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		keys := sortedKeys(resp.Parameters)
		for _, k := range keys {
			v := resp.Parameters[k]
			fmt.Fprintf(w, "%s\t%v\n", k, v)
		}
		w.Flush()
	},
}

var paramsSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a global parameter",
	Long: `Set a single global parameter. The value is parsed as JSON if possible,
otherwise treated as a plain string.

Examples:
  mywant params set llm_provider anthropic
  mywant params set opa_llm_use_llm true
  mywant params set opa_llm_planner_command /usr/local/bin/opa-llm-planner`,
	Args: cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		key, rawVal := args[0], args[1]
		c := client.NewClient(viper.GetString("server"))

		resp, err := c.GetGlobalParameters()
		if err != nil {
			fmt.Printf("Error fetching parameters: %v\n", err)
			os.Exit(1)
		}
		params := resp.Parameters
		if params == nil {
			params = make(map[string]any)
		}

		// Try JSON parse for structured types (bool, number, object, array)
		var parsed any
		if json.Unmarshal([]byte(rawVal), &parsed) == nil {
			params[key] = parsed
		} else {
			params[key] = rawVal
		}

		updated, err := c.UpdateGlobalParameters(params)
		if err != nil {
			fmt.Printf("Error saving parameters: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Set %s = %v  (%d parameters total)\n", key, params[key], updated.Count)
	},
}

var paramsDeleteCmd = &cobra.Command{
	Use:     "delete <key>",
	Aliases: []string{"del", "rm"},
	Short:   "Delete a global parameter",
	Args:    cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]
		c := client.NewClient(viper.GetString("server"))

		resp, err := c.GetGlobalParameters()
		if err != nil {
			fmt.Printf("Error fetching parameters: %v\n", err)
			os.Exit(1)
		}
		params := resp.Parameters
		if _, ok := params[key]; !ok {
			fmt.Printf("Key %q not found.\n", key)
			os.Exit(1)
		}
		delete(params, key)

		updated, err := c.UpdateGlobalParameters(params)
		if err != nil {
			fmt.Printf("Error saving parameters: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Deleted %s  (%d parameters remaining)\n", key, updated.Count)
	},
}

var paramsImportCmd = &cobra.Command{
	Use:   "import",
	Short: "Import global parameters from a YAML or JSON file",
	Long: `Import (replace) all global parameters from a file.

Examples:
  mywant params import -f ~/.mywant/parameters.yaml
  mywant params import -f params.json`,
	Run: func(cmd *cobra.Command, args []string) {
		filePath, _ := cmd.Flags().GetString("file")
		mergeFlag, _ := cmd.Flags().GetBool("merge")

		if filePath == "" {
			fmt.Println("Error: --file (-f) is required")
			os.Exit(1)
		}

		data, err := os.ReadFile(filePath)
		if err != nil {
			fmt.Printf("Error reading file: %v\n", err)
			os.Exit(1)
		}

		var newParams map[string]any

		// Try YAML first (superset of JSON)
		if err := yaml.Unmarshal(data, &newParams); err != nil {
			fmt.Printf("Error parsing file: %v\n", err)
			os.Exit(1)
		}
		if newParams == nil {
			newParams = make(map[string]any)
		}

		c := client.NewClient(viper.GetString("server"))

		if mergeFlag {
			existing, err := c.GetGlobalParameters()
			if err != nil {
				fmt.Printf("Error fetching current parameters: %v\n", err)
				os.Exit(1)
			}
			merged := existing.Parameters
			if merged == nil {
				merged = make(map[string]any)
			}
			for k, v := range newParams {
				merged[k] = v
			}
			newParams = merged
		}

		updated, err := c.UpdateGlobalParameters(newParams)
		if err != nil {
			fmt.Printf("Error saving parameters: %v\n", err)
			os.Exit(1)
		}
		action := "Imported"
		if mergeFlag {
			action = "Merged"
		}
		fmt.Printf("%s %d parameters from %s\n", action, updated.Count, filePath)
	},
}

var paramsExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export global parameters to stdout or a file",
	Long: `Export global parameters as YAML.

Examples:
  mywant params export
  mywant params export -f backup.yaml`,
	Run: func(cmd *cobra.Command, args []string) {
		filePath, _ := cmd.Flags().GetString("file")
		c := client.NewClient(viper.GetString("server"))

		resp, err := c.GetGlobalParameters()
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		data, err := yaml.Marshal(resp.Parameters)
		if err != nil {
			fmt.Printf("Error marshalling parameters: %v\n", err)
			os.Exit(1)
		}

		if filePath != "" {
			if err := os.WriteFile(filePath, data, 0644); err != nil {
				fmt.Printf("Error writing file: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("Exported %d parameters to %s\n", resp.Count, filePath)
		} else {
			fmt.Print(string(data))
		}
	},
}

// ─── helpers ───────────────────────────────────────────────────────────────

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func printFlatMap(m map[string]any, prefix string) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	for _, k := range sortedKeys(m) {
		v := m[k]
		fullKey := k
		if prefix != "" {
			fullKey = prefix + "." + k
		}
		switch vt := v.(type) {
		case map[string]any:
			printFlatMap(vt, fullKey)
		default:
			fmt.Fprintf(w, "%s\t%v\n", fullKey, v)
		}
	}
	w.Flush()
}

func init() {
	// memo subcommands
	memoGetCmd.Flags().Bool("json", false, "Output as JSON")
	memoClearCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")
	MemoCmd.AddCommand(memoGetCmd)
	MemoCmd.AddCommand(memoClearCmd)

	// params subcommands
	paramsGetCmd.Flags().Bool("json", false, "Output as JSON")
	paramsImportCmd.Flags().StringP("file", "f", "", "Path to YAML or JSON file")
	paramsImportCmd.Flags().Bool("merge", false, "Merge with existing parameters instead of replacing")
	paramsExportCmd.Flags().StringP("file", "f", "", "Write output to file instead of stdout")
	ParamsCmd.AddCommand(paramsGetCmd)
	ParamsCmd.AddCommand(paramsSetCmd)
	ParamsCmd.AddCommand(paramsDeleteCmd)
	ParamsCmd.AddCommand(paramsImportCmd)
	ParamsCmd.AddCommand(paramsExportCmd)
}
