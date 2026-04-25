package commands

import (
	"fmt"
	"os"

	"mywant/client"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// GuiCmd is the top-level "gui" command.
var GuiCmd = &cobra.Command{
	Use:   "gui",
	Short: "Control the web GUI from the CLI",
	Long:  `Commands to inspect and control the MyWant web dashboard from the command line.`,
}

// guiShowCmd groups the show subcommands.
var guiShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Navigate the web GUI to a specific view",
}

// guiShowWantCmd opens the sidebar for a specific want.
var guiShowWantCmd = &cobra.Command{
	Use:   "want <ID>",
	Short: "Open the sidebar for a specific want",
	Long: `Open the want details sidebar in the web GUI.

Examples:
  mywant gui show want abc-123
  mywant gui show want abc-123 --tab logs
  mywant gui show want abc-123 --tab settings --filter reaching`,
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeWantIDs,
	Run: func(cmd *cobra.Command, args []string) {
		wantID := args[0]
		tab, _ := cmd.Flags().GetString("tab")
		filter, _ := cmd.Flags().GetString("filter")
		search, _ := cmd.Flags().GetString("search")

		c := client.NewClient(viper.GetString("server"))
		if err := c.ShowGUIWant(wantID, tab, filter, search); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("GUI: opened sidebar for want %s (tab: %s)\n", wantID, tab)
	},
}

// guiShowDashboardCmd navigates to the dashboard view.
var guiShowDashboardCmd = &cobra.Command{
	Use:   "dashboard",
	Short: "Navigate to the dashboard (close sidebar)",
	Long: `Navigate to the dashboard view in the web GUI, optionally applying filters.

Examples:
  mywant gui show dashboard
  mywant gui show dashboard --filter reaching
  mywant gui show dashboard --search "travel"
  mywant gui show dashboard --filter stopped --search "reminder"`,
	Run: func(cmd *cobra.Command, args []string) {
		filter, _ := cmd.Flags().GetString("filter")
		search, _ := cmd.Flags().GetString("search")

		c := client.NewClient(viper.GetString("server"))
		if err := c.ShowGUIDashboard(filter, search); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		msg := "GUI: navigated to dashboard"
		if filter != "" {
			msg += fmt.Sprintf(" (filter: %s)", filter)
		}
		if search != "" {
			msg += fmt.Sprintf(" (search: %q)", search)
		}
		fmt.Println(msg)
	},
}

// guiGetCmd shows the current GUI state.
var guiGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Show the current GUI state",
	Run: func(cmd *cobra.Command, args []string) {
		c := client.NewClient(viper.GetString("server"))
		snap, err := c.GetGUIState()
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		cur := snap.State.Current
		if cur == nil {
			cur = map[string]any{}
		}
		fmt.Printf("Source:                 %v\n", cur["source"])
		fmt.Printf("Dashboard status filter:%v\n", cur["dashboard_status_filter"])
		fmt.Printf("Dashboard search query: %v\n", cur["dashboard_search_query"])
		fmt.Printf("Sidebar open:           %v\n", cur["sidebar_open"])
		fmt.Printf("Sidebar want ID:        %v\n", cur["sidebar_want_id"])
		fmt.Printf("Sidebar active tab:     %v\n", cur["sidebar_active_tab"])
	},
}

func init() {
	// gui show want flags
	guiShowWantCmd.Flags().String("tab", "results", "Sidebar tab (settings|results|logs|agents|versions|chat)")
	guiShowWantCmd.Flags().String("filter", "", "Dashboard status filter (e.g. reaching, stopped)")
	guiShowWantCmd.Flags().String("search", "", "Dashboard search query")
	guiShowWantCmd.RegisterFlagCompletionFunc("tab", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return []string{"settings", "results", "logs", "agents", "versions", "chat"}, cobra.ShellCompDirectiveNoFileComp
	})

	// gui show dashboard flags
	guiShowDashboardCmd.Flags().String("filter", "", "Status filter (e.g. reaching, stopped, achieved)")
	guiShowDashboardCmd.Flags().String("search", "", "Search query")

	guiShowCmd.AddCommand(guiShowWantCmd)
	guiShowCmd.AddCommand(guiShowDashboardCmd)

	GuiCmd.AddCommand(guiShowCmd)
	GuiCmd.AddCommand(guiGetCmd)
}
