package commands

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"mywant/pkg/client"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var AgentsCmd = &cobra.Command{
	Use:   "agents",
	Short: "Manage agents",
	Long:  `List and manage agents.`,
}

var listAgentsCmd = &cobra.Command{
	Use:   "list",
	Short: "List all agents",
	Run: func(cmd *cobra.Command, args []string) {
		c := client.NewClient(viper.GetString("server"))
		resp, err := c.ListAgents()
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "Name\tType\tCapabilities")

		if agents, ok := resp["agents"]; ok {
			for _, agent := range agents {
				fmt.Fprintf(w, "%s\t%s\t%s\n",
					agent.Name,
					agent.Type,
					strings.Join(agent.Capabilities, ", "),
				)
			}
		}
		w.Flush()
	},
}

var CapabilitiesCmd = &cobra.Command{
	Use:   "capabilities",
	Short: "Manage capabilities",
	Long:  `List and manage capabilities.`,
}

var listCapabilitiesCmd = &cobra.Command{
	Use:   "list",
	Short: "List all capabilities",
	Run: func(cmd *cobra.Command, args []string) {
		c := client.NewClient(viper.GetString("server"))
		resp, err := c.ListCapabilities()
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "Name")

		if caps, ok := resp["capabilities"]; ok {
			for _, cap := range caps {
				fmt.Fprintf(w, "%s\n", cap.Name)
			}
		}
		w.Flush()
	},
}

var TypesCmd = &cobra.Command{
	Use:   "types",
	Short: "Manage want types",
	Long:  `List available want types.`,
}

var listTypesCmd = &cobra.Command{
	Use:   "list",
	Short: "List available want types",
	Run: func(cmd *cobra.Command, args []string) {
		c := client.NewClient(viper.GetString("server"))
		types, err := c.ListWantTypes()
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "Name\tCategory\tTitle")

		for _, t := range types {
			fmt.Fprintf(w, "%s\t%s\t%s\n", t.Name, t.Category, t.Title)
		}
		w.Flush()
	},
}

func init() {
	AgentsCmd.AddCommand(listAgentsCmd)
	CapabilitiesCmd.AddCommand(listCapabilitiesCmd)
	TypesCmd.AddCommand(listTypesCmd)
}
