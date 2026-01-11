package commands

import (
	"encoding/json"
	"fmt"
	"mywant/pkg/client"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var AgentsCmd = &cobra.Command{
	Use:     "agents",
	Aliases: []string{"a"},
	Short:   "Manage agents",
	Long:    `List and manage agents.`,
}

// completion helper for agents
func completeAgentNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	c := client.NewClient(viper.GetString("server"))
	resp, err := c.ListAgents()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	var names []string
	if agents, ok := resp["agents"]; ok {
		for _, agent := range agents {
			names = append(names, fmt.Sprintf("%s\t%s", agent.Name, agent.Type))
		}
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

var listAgentsCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"l"},
	Short:   "List all agents",
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

var getAgentCmd = &cobra.Command{
	Use:               "get [name]",
	Aliases:           []string{"g"},
	Short:             "Get agent details",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeAgentNames,
	Run: func(cmd *cobra.Command, args []string) {
		c := client.NewClient(viper.GetString("server"))
		agent, err := c.GetAgent(args[0])
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Name: %s\n", agent.Name)
		fmt.Printf("Type: %s\n", agent.Type)
		fmt.Printf("Capabilities: %s\n", strings.Join(agent.Capabilities, ", "))
	},
}

var deleteAgentCmd = &cobra.Command{
	Use:               "delete [name]",
	Aliases:           []string{"d"},
	Short:             "Delete an agent",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeAgentNames,
	Run: func(cmd *cobra.Command, args []string) {
		c := client.NewClient(viper.GetString("server"))
		err := c.DeleteAgent(args[0])
		if err != nil {
			fmt.Printf("Error deleting agent: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Agent %s deleted\n", args[0])
	},
}

var CapabilitiesCmd = &cobra.Command{
	Use:     "capabilities",
	Aliases: []string{"c"},
	Short:   "Manage capabilities",
	Long:    `List and manage capabilities.`,
}

// completion helper for capabilities
func completeCapabilityNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	c := client.NewClient(viper.GetString("server"))
	resp, err := c.ListCapabilities()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	var names []string
	if caps, ok := resp["capabilities"]; ok {
		for _, cap := range caps {
			names = append(names, cap.Name)
		}
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

var listCapabilitiesCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"l"},
	Short:   "List all capabilities",
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

var getCapabilityCmd = &cobra.Command{
	Use:               "get [name]",
	Aliases:           []string{"g"},
	Short:             "Get capability details",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeCapabilityNames,
	Run: func(cmd *cobra.Command, args []string) {
		c := client.NewClient(viper.GetString("server"))
		cap, err := c.GetCapability(args[0])
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Name: %s\n", cap.Name)
	},
}

var deleteCapabilityCmd = &cobra.Command{
	Use:               "delete [name]",
	Aliases:           []string{"d"},
	Short:             "Delete a capability",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeCapabilityNames,
	Run: func(cmd *cobra.Command, args []string) {
		c := client.NewClient(viper.GetString("server"))
		err := c.DeleteCapability(args[0])
		if err != nil {
			fmt.Printf("Error deleting capability: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Capability %s deleted\n", args[0])
	},
}

var TypesCmd = &cobra.Command{
	Use:     "types",
	Aliases: []string{"t"},
	Short:   "Manage want types",
	Long:    `List available want types.`,
}

// completion helper for types
func completeTypeNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	c := client.NewClient(viper.GetString("server"))
	types, err := c.ListWantTypes()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	var names []string
	for _, t := range types {
		names = append(names, fmt.Sprintf("%s\t%s", t.Name, t.Category))
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

var listTypesCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"l"},
	Short:   "List available want types",
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

var getTypeCmd = &cobra.Command{
	Use:               "get [name]",
	Aliases:           []string{"g"},
	Short:             "Get want type details",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeTypeNames,
	Run: func(cmd *cobra.Command, args []string) {
		c := client.NewClient(viper.GetString("server"))
		tDef, err := c.GetWantType(args[0])
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		jsonData, _ := json.MarshalIndent(tDef, "", "  ")
		fmt.Println(string(jsonData))
	},
}

func init() {
	AgentsCmd.AddCommand(listAgentsCmd)
	AgentsCmd.AddCommand(getAgentCmd)
	AgentsCmd.AddCommand(deleteAgentCmd)

	CapabilitiesCmd.AddCommand(listCapabilitiesCmd)
	CapabilitiesCmd.AddCommand(getCapabilityCmd)
	CapabilitiesCmd.AddCommand(deleteCapabilityCmd)

	TypesCmd.AddCommand(listTypesCmd)
	TypesCmd.AddCommand(getTypeCmd)
}
