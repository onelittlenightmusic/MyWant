package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"mywant/pkg/client"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

var WantsCmd = &cobra.Command{
	Use:   "wants",
	Short: "Manage want executions",
	Long:  `List, create, update, and delete want executions.`,
}

var listWantsCmd = &cobra.Command{
	Use:   "list",
	Short: "List all wants",
	Run: func(cmd *cobra.Command, args []string) {
		c := client.NewClient(viper.GetString("server"))
		resp, err := c.ListWants()
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "ID\tName\tType\tStatus")

		for _, want := range resp.Wants {
			status := want.Status
			if status == "" {
				status = "unknown"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
				want.Metadata.ID,
				want.Metadata.Name,
				want.Metadata.Type,
				status,
			)
		}
		w.Flush()
	},
}

var getWantCmd = &cobra.Command{
	Use:   "get [id]",
	Short: "Get want details",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		c := client.NewClient(viper.GetString("server"))
		want, err := c.GetWant(args[0])
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		// Basic info
		fmt.Printf("ID: %s\n", want.Metadata.ID)
		fmt.Printf("Name: %s\n", want.Metadata.Name)
		fmt.Printf("Type: %s\n", want.Metadata.Type)
		fmt.Printf("Status: %s\n", want.Status)
		
		fmt.Println("\nParams:")
		printMap(want.Spec.Params)

		if len(want.State) > 0 {
			fmt.Println("\nState:")
			printMap(want.State)
		}
	},
}

var createWantCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new want from file",
	Run: func(cmd *cobra.Command, args []string) {
		file, _ := cmd.Flags().GetString("file")
		if file == "" {
			fmt.Println("Error: --file flag is required")
			os.Exit(1)
		}

		data, err := os.ReadFile(file)
		if err != nil {
			fmt.Printf("Error reading file: %v\n", err)
			os.Exit(1)
		}

		var config client.Config
		// Try YAML first, then JSON
		if err := yaml.Unmarshal(data, &config); err != nil {
			if err := json.Unmarshal(data, &config); err != nil {
				fmt.Printf("Error parsing file: %v\n", err)
				os.Exit(1)
			}
		}

		c := client.NewClient(viper.GetString("server"))
		resp, err := c.CreateWant(config)
		if err != nil {
			fmt.Printf("Error creating want: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Successfully created want execution %s with %d wants\n", resp.ID, resp.Wants)
		for _, id := range resp.WantIDs {
			fmt.Printf("- %s\n", id)
		}
	},
}

var deleteWantCmd = &cobra.Command{
	Use:   "delete [id]",
	Short: "Delete a want",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		c := client.NewClient(viper.GetString("server"))
		err := c.DeleteWant(args[0])
		if err != nil {
			fmt.Printf("Error deleting want: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Want %s deleted\n", args[0])
	},
}

func init() {
	WantsCmd.AddCommand(listWantsCmd)
	WantsCmd.AddCommand(getWantCmd)
	WantsCmd.AddCommand(createWantCmd)
	WantsCmd.AddCommand(deleteWantCmd)
	WantsCmd.AddCommand(exportWantsCmd)
	WantsCmd.AddCommand(importWantsCmd)

	createWantCmd.Flags().StringP("file", "f", "", "Path to YAML/JSON config file")
	exportWantsCmd.Flags().StringP("output", "o", "", "Path to save exported YAML (stdout if not specified)")
	importWantsCmd.Flags().StringP("file", "f", "", "Path to YAML file to import")

	WantsCmd.AddCommand(suspendWantsCmd)
	WantsCmd.AddCommand(resumeWantsCmd)
	WantsCmd.AddCommand(stopWantsCmd)
	WantsCmd.AddCommand(startWantsCmd)
}

var exportWantsCmd = &cobra.Command{
	Use:   "export",
	Short: "Export all wants as YAML",
	Run: func(cmd *cobra.Command, args []string) {
		output, _ := cmd.Flags().GetString("output")
		c := client.NewClient(viper.GetString("server"))
		data, err := c.ExportWants()
		if err != nil {
			fmt.Printf("Error exporting wants: %v\n", err)
			os.Exit(1)
		}

		if output != "" {
			if err := os.WriteFile(output, data, 0644); err != nil {
				fmt.Printf("Error writing to file: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("Successfully exported wants to %s\n", output)
		} else {
			fmt.Println(string(data))
		}
	},
}

var importWantsCmd = &cobra.Command{
	Use:   "import",
	Short: "Import wants from YAML file",
	Run: func(cmd *cobra.Command, args []string) {
		file, _ := cmd.Flags().GetString("file")
		if file == "" {
			fmt.Println("Error: --file flag is required")
			os.Exit(1)
		}

		data, err := os.ReadFile(file)
		if err != nil {
			fmt.Printf("Error reading file: %v\n", err)
			os.Exit(1)
		}

		c := client.NewClient(viper.GetString("server"))
		resp, err := c.ImportWants(data)
		if err != nil {
			fmt.Printf("Error importing wants: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Successfully imported execution %s with %d wants\n", resp.ID, resp.Wants)
		fmt.Println(resp.Message)
	},
}

func printMap(m map[string]any) {	for k, v := range m {
		fmt.Printf("  %s: %v\n", k, v)
	}
}

func runBatchOperation(args []string, opName string, opFunc func(*client.Client, []string) error) {
	c := client.NewClient(viper.GetString("server"))
	if err := opFunc(c, args); err != nil {
		fmt.Printf("Error during %s: %v\n", opName, err)
		os.Exit(1)
	}
	fmt.Printf("Successfully queued %s for %d wants\n", opName, len(args))
}

var suspendWantsCmd = &cobra.Command{
	Use:   "suspend [id]...",
	Short: "Suspend want executions",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runBatchOperation(args, "suspend", (*client.Client).SuspendWants)
	},
}

var resumeWantsCmd = &cobra.Command{
	Use:   "resume [id]...",
	Short: "Resume want executions",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runBatchOperation(args, "resume", (*client.Client).ResumeWants)
	},
}

var stopWantsCmd = &cobra.Command{
	Use:   "stop [id]...",
	Short: "Stop want executions",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runBatchOperation(args, "stop", (*client.Client).StopWants)
	},
}

var startWantsCmd = &cobra.Command{
	Use:   "start [id]...",
	Short: "Start want executions",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runBatchOperation(args, "start", (*client.Client).StartWants)
	},
}
