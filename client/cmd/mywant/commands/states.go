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

// StateCmd is the root command for cross-want state operations.
var StateCmd = &cobra.Command{
	Use:     "state",
	Aliases: []string{"st"},
	Short:   "Cross-want state CRUD",
	Long:    `List, search, get, set, and delete state across all wants and global state.`,
}

// ─── list ──────────────────────────────────────────────────────────────────

var stateListCmd = &cobra.Command{
	Use:   "list",
	Short: "List state for all wants",
	Long: `Display state snapshots for all running wants.

Examples:
  mywant state list
  mywant state list --no-global
  mywant state list --label current
  mywant state list --ancestor <want-id>`,
	Run: func(cmd *cobra.Command, args []string) {
		jsonFlag, _ := cmd.Flags().GetBool("json")
		noGlobal, _ := cmd.Flags().GetBool("no-global")
		label, _ := cmd.Flags().GetString("label")
		ancestor, _ := cmd.Flags().GetString("ancestor")

		c := client.NewClient(viper.GetString("server"))
		resp, err := c.ListStates(ancestor, label, !noGlobal)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		if jsonFlag {
			data, _ := json.MarshalIndent(resp, "", "  ")
			fmt.Println(string(data))
			return
		}

		fmt.Printf("Want States  (%d wants)\n", resp.Total)
		fmt.Println(strings.Repeat("─", 60))

		for _, snap := range resp.Wants {
			fmt.Printf("\n%s  (%s)\n", snap.WantName, snap.WantID)
			printHierarchicalState(snap.State)
		}

		if len(resp.GlobalState) > 0 {
			fmt.Printf("\nglobal\n")
			printFlatMap(resp.GlobalState, "  ")
		}
	},
}

// ─── search ────────────────────────────────────────────────────────────────

var stateSearchCmd = &cobra.Command{
	Use:   "search <field>",
	Short: "Search all wants by state field name",
	Long: `Find all wants (and optionally global state) that contain a given state field.

Examples:
  mywant state search achieving_percentage
  mywant state search hotel_name --no-global
  mywant state search status --ancestor <want-id>`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		field := args[0]
		jsonFlag, _ := cmd.Flags().GetBool("json")
		noGlobal, _ := cmd.Flags().GetBool("no-global")
		ancestor, _ := cmd.Flags().GetString("ancestor")

		c := client.NewClient(viper.GetString("server"))
		resp, err := c.SearchStates(field, ancestor, !noGlobal)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		if jsonFlag {
			data, _ := json.MarshalIndent(resp, "", "  ")
			fmt.Println(string(data))
			return
		}

		fmt.Printf("Search: %q  (%d results)\n", resp.Field, resp.Total)
		if resp.Total == 0 {
			fmt.Println("(no matches)")
			return
		}

		fmt.Println(strings.Repeat("─", 70))
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "WANT ID\tNAME\tFIELD\tVALUE\tLABEL\tSOURCE")
		for _, r := range resp.Results {
			fmt.Fprintf(w, "%s\t%s\t%s\t%v\t%s\t%s\n",
				r.WantID, r.WantName, r.Field, r.Value, r.Label, r.Source)
		}
		w.Flush()
	},
}

// ─── get ───────────────────────────────────────────────────────────────────

var stateGetCmd = &cobra.Command{
	Use:   "get <want-id>",
	Short: "Get full state for a specific want",
	Long: `Display the complete state snapshot (current/goal/plan) for a want.

Examples:
  mywant state get abc123
  mywant state get abc123 --json`,
	Args: cobra.ExactArgs(1),
	ValidArgsFunction: completeWantIDs,
	Run: func(cmd *cobra.Command, args []string) {
		wantID := args[0]
		jsonFlag, _ := cmd.Flags().GetBool("json")

		c := client.NewClient(viper.GetString("server"))
		snap, err := c.GetWantState(wantID)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		if jsonFlag {
			data, _ := json.MarshalIndent(snap, "", "  ")
			fmt.Println(string(data))
			return
		}

		fmt.Printf("%s  (%s)\n", snap.WantName, snap.WantID)
		fmt.Println(strings.Repeat("─", 50))
		printHierarchicalState(snap.State)
	},
}

// ─── set ───────────────────────────────────────────────────────────────────

var stateSetCmd = &cobra.Command{
	Use:   "set <want-id> <key> <value>",
	Short: "Set a state key on a want",
	Long: `Store a value under a state key on a specific want.
The value is parsed as JSON if valid, otherwise stored as a plain string.

Examples:
  mywant state set abc123 status "in_progress"
  mywant state set abc123 price 45000
  mywant state set abc123 approved true`,
	Args: cobra.ExactArgs(3),
	ValidArgsFunction: completeWantIDs,
	Run: func(cmd *cobra.Command, args []string) {
		wantID, key, rawVal := args[0], args[1], args[2]

		var value any
		if err := json.Unmarshal([]byte(rawVal), &value); err != nil {
			value = rawVal
		}

		c := client.NewClient(viper.GetString("server"))
		if err := c.SetWantStateKey(wantID, key, value); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Set %s = %v\n", key, value)
	},
}

// ─── delete ────────────────────────────────────────────────────────────────

var stateDeleteCmd = &cobra.Command{
	Use:     "delete <want-id> <key>",
	Aliases: []string{"del", "rm"},
	Short:   "Delete a state key from a want",
	Long: `Remove a specific state key from a want.

Examples:
  mywant state delete abc123 tmp_key
  mywant state del abc123 debug_info`,
	Args: cobra.ExactArgs(2),
	ValidArgsFunction: completeWantIDs,
	Run: func(cmd *cobra.Command, args []string) {
		wantID, key := args[0], args[1]

		c := client.NewClient(viper.GetString("server"))
		if err := c.DeleteWantStateKey(wantID, key); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Deleted state key %q from want %s\n", key, wantID)
	},
}

// ─── helpers ───────────────────────────────────────────────────────────────

func printHierarchicalState(s client.HierarchicalState) {
	if s.FinalResult != nil {
		fmt.Printf("  final_result:  %v\n", s.FinalResult)
	}
	if len(s.Current) > 0 {
		fmt.Println("  current:")
		printFlatMap(s.Current, "    ")
	}
	if len(s.Goal) > 0 {
		fmt.Println("  goal:")
		printFlatMap(s.Goal, "    ")
	}
	if len(s.Plan) > 0 {
		fmt.Println("  plan:")
		printFlatMap(s.Plan, "    ")
	}
	if s.FinalResult == nil && len(s.Current) == 0 && len(s.Goal) == 0 && len(s.Plan) == 0 {
		fmt.Println("  (empty)")
	}
}

// ─── init ──────────────────────────────────────────────────────────────────

func init() {
	// list flags
	stateListCmd.Flags().Bool("json", false, "Output as JSON")
	stateListCmd.Flags().Bool("no-global", false, "Exclude global state")
	stateListCmd.Flags().String("label", "", "Filter by state label: current, goal, plan")
	stateListCmd.Flags().String("ancestor", "", "Scope to descendants of this want ID")

	// search flags
	stateSearchCmd.Flags().Bool("json", false, "Output as JSON")
	stateSearchCmd.Flags().Bool("no-global", false, "Exclude global state from search")
	stateSearchCmd.Flags().String("ancestor", "", "Scope search to descendants of this want ID")

	// get flags
	stateGetCmd.Flags().Bool("json", false, "Output as JSON")

	StateCmd.AddCommand(stateListCmd)
	StateCmd.AddCommand(stateSearchCmd)
	StateCmd.AddCommand(stateGetCmd)
	StateCmd.AddCommand(stateSetCmd)
	StateCmd.AddCommand(stateDeleteCmd)
}
