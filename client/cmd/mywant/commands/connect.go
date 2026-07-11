package commands

import (
	"fmt"

	"mywant/client"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var connectWantsCmd = &cobra.Command{
	Use:   "connect <name-or-id-A> <name-or-id-B>",
	Short: "Create an expose/import connection between two wants",
	Long: `Fetch field-match recommendations between two wants and apply the best one.
The provider/consumer role is determined automatically by the API.

Example:
  mywant wants connect workshop-eye workshop-brain`,
	Args:              cobra.ExactArgs(2),
	ValidArgsFunction: completeWantIDs,
	Run: func(cmd *cobra.Command, args []string) {
		c := client.NewClient(viper.GetString("server"))

		idA, err := c.ResolveWantID(args[0])
		if err != nil {
			fmt.Printf("Error: resolve %q: %v\n", args[0], err)
			return
		}
		idB, err := c.ResolveWantID(args[1])
		if err != nil {
			fmt.Printf("Error: resolve %q: %v\n", args[1], err)
			return
		}

		rec, err := c.ConnectWants(idA, idB)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			return
		}

		if rec.ExposeAction != nil && rec.ImportAction != nil {
			fmt.Printf("connected: %s → %s (key: %s)\n",
				rec.ExposeAction.WantName, rec.ImportAction.WantName, rec.ExposeAction.GlobalKey)
		} else {
			fmt.Printf("connected: %s ↔ %s\n", args[0], args[1])
		}
	},
}

var disconnectWantsCmd = &cobra.Command{
	Use:   "disconnect <name-or-id-A> <name-or-id-B>",
	Short: "Remove the expose/import connection between two wants",
	Long: `Find and delete the expose/import link between two wants.
The provider side is detected automatically.

Example:
  mywant wants disconnect workshop-hand workshop-eye`,
	Args:              cobra.ExactArgs(2),
	ValidArgsFunction: completeWantIDs,
	Run: func(cmd *cobra.Command, args []string) {
		c := client.NewClient(viper.GetString("server"))

		idA, err := c.ResolveWantID(args[0])
		if err != nil {
			fmt.Printf("Error: resolve %q: %v\n", args[0], err)
			return
		}
		idB, err := c.ResolveWantID(args[1])
		if err != nil {
			fmt.Printf("Error: resolve %q: %v\n", args[1], err)
			return
		}

		label, err := c.DisconnectWants(idA, idB)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			return
		}

		fmt.Printf("disconnected: %s ↔ %s (removed %s)\n", args[0], args[1], label)
	},
}

func init() {
	WantsCmd.AddCommand(connectWantsCmd)
	WantsCmd.AddCommand(disconnectWantsCmd)
}
