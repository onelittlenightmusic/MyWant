package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"mywant/client"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

var WantsCmd = &cobra.Command{
	Use:     "wants",
	Aliases: []string{"w"},
	Short:   "Manage want executions",
	Long:    `List, create, update, and delete want executions.`,
}

// completion helper for wants
func completeWantIDs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	c := client.NewClient(viper.GetString("server"))
	resp, err := c.ListWants("", []string{}, []string{}) // No filters for completion
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	var ids []string
	for _, want := range resp.Wants {
		ids = append(ids, fmt.Sprintf("%s\t%s (%s)", want.Metadata.ID, want.Metadata.Name, want.Metadata.Type))
	}
	return ids, cobra.ShellCompDirectiveNoFileComp
}

var listWantsCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"l"},
	Short:   "List all wants",
	Run: func(cmd *cobra.Command, args []string) {
		c := client.NewClient(viper.GetString("server"))
		wantType, _ := cmd.Flags().GetString("type")
		labels, _ := cmd.Flags().GetStringSlice("label")
		using, _ := cmd.Flags().GetStringSlice("using")
		resp, err := c.ListWants(wantType, labels, using)
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
	Use:               "get [id]",
	Aliases:           []string{"g"},
	Short:             "Get want details",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeWantIDs,
	Run: func(cmd *cobra.Command, args []string) {
		c := client.NewClient(viper.GetString("server"))
		showHidden, _ := cmd.Flags().GetBool("hidden")
		want, err := c.GetWant(args[0], showHidden)
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

		// Show hidden state if --hidden flag is set
		if showHidden {
			if len(want.HiddenState) > 0 {
				fmt.Println("\nHiddenState:")
				printMap(want.HiddenState)
			}
			if want.ConnectivityMetadata != nil {
				fmt.Println("\nConnectivityMetadata:")
				fmt.Printf("  RequiredInputs: %d\n", want.ConnectivityMetadata.RequiredInputs)
				fmt.Printf("  RequiredOutputs: %d\n", want.ConnectivityMetadata.RequiredOutputs)
				fmt.Printf("  MaxInputs: %d\n", want.ConnectivityMetadata.MaxInputs)
				fmt.Printf("  MaxOutputs: %d\n", want.ConnectivityMetadata.MaxOutputs)
				fmt.Printf("  WantType: %s\n", want.ConnectivityMetadata.WantType)
				fmt.Printf("  Description: %s\n", want.ConnectivityMetadata.Description)
			}
		}
	},
}

func init() {
	getWantCmd.Flags().BoolP("hidden", "H", false, "Show hidden state (for debugging)")
}

var createWantCmd = &cobra.Command{
	Use:     "create",
	Aliases: []string{"c"},
	Short:   "Create a new want",
	Run: func(cmd *cobra.Command, args []string) {
		file, _ := cmd.Flags().GetString("file")
		wantType, _ := cmd.Flags().GetString("type")
		useExample, _ := cmd.Flags().GetBool("example")

		if file == "" && wantType == "" {
			fmt.Println("Error: either --file or --type flag is required")
			os.Exit(1)
		}

		if useExample && wantType == "" {
			fmt.Println("Error: --example flag requires --type flag")
			os.Exit(1)
		}

		var config client.Config
		c := client.NewClient(viper.GetString("server"))

		if wantType != "" {
			if useExample {
				// Fetch example from server
				recipe, err := c.GetRecipe(wantType)
				if err != nil {
					recipes, rErr := c.ListRecipes()
					if rErr == nil {
						for _, r := range recipes {
							if r.Recipe.Metadata.CustomType == wantType {
								recipe = &r
								err = nil
								break
							}
						}
					}
				}

				if err == nil && recipe.Recipe.Example != nil {
					config = *recipe.Recipe.Example
				} else {
					exResp, err := c.GetWantTypeExamples(wantType)
					if err == nil {
						if examples, ok := (*exResp)["examples"].([]any); ok && len(examples) > 0 {
							exampleBytes, _ := json.Marshal(examples[0])
							var want client.Want
							if err := json.Unmarshal(exampleBytes, &want); err == nil {
								config = client.Config{Wants: []*client.Want{&want}}
							}
						}
					}
				}

				if len(config.Wants) == 0 {
					fmt.Printf("Error: No example found for type '%s'\n", wantType)
					os.Exit(1)
				}
			} else {
				config = client.Config{
					Wants: []*client.Want{
						{
							Metadata: client.Metadata{
								Name: "new-" + wantType,
								Type: wantType,
							},
							Spec: client.WantSpec{
								Params: make(map[string]any),
							},
						},
					},
				}
			}
		} else {
			data, err := os.ReadFile(file)
			if err != nil {
				fmt.Printf("Error reading file: %v\n", err)
				os.Exit(1)
			}

			if err := yaml.Unmarshal(data, &config); err != nil {
				if err := json.Unmarshal(data, &config); err != nil {
					var newWant client.Want
					if err2 := yaml.Unmarshal(data, &newWant); err2 == nil && newWant.Metadata.Type != "" {
						config = client.Config{Wants: []*client.Want{&newWant}}
					} else {
						fmt.Printf("Error parsing file: %v\n", err)
						os.Exit(1)
					}
				}
			}
		}

		if len(config.Wants) == 0 {
			fmt.Println("Error: No wants to create")
			os.Exit(1)
		}

		resp, err := c.CreateWant(config)
		if err != nil {
			fmt.Printf("Error creating want: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Successfully created want execution %s with %d wants\n", resp.ID, len(resp.WantIDs))
		for _, id := range resp.WantIDs {
			fmt.Printf("- %s\n", id)
		}
	},
}

var deleteWantCmd = &cobra.Command{
	Use:               "delete [id]",
	Aliases:           []string{"d"},
	Short:             "Delete a want",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeWantIDs,
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

	listWantsCmd.Flags().StringP("type", "t", "", "Filter wants by type (e.g., reminder, flight, queue)")
	listWantsCmd.Flags().StringSliceP("label", "l", []string{}, "Filter wants by labels (format: key=value, can be specified multiple times)")
	listWantsCmd.Flags().StringSliceP("using", "u", []string{}, "Filter wants by using selectors (format: key=value, can be specified multiple times)")
	createWantCmd.Flags().StringP("file", "f", "", "Path to YAML/JSON config file")
	createWantCmd.Flags().StringP("type", "t", "", "Create want of specific type")
	createWantCmd.Flags().BoolP("example", "e", false, "Use example parameters for the specified type (requires --type)")
	exportWantsCmd.Flags().StringP("output", "o", "", "Path to save exported YAML (stdout if not specified)")
	importWantsCmd.Flags().StringP("file", "f", "", "Path to YAML file to import")

	WantsCmd.AddCommand(suspendWantsCmd)
	WantsCmd.AddCommand(resumeWantsCmd)
	WantsCmd.AddCommand(stopWantsCmd)
	WantsCmd.AddCommand(startWantsCmd)
}

func printMap(m map[string]any) {
	for k, v := range m {
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
	Use:               "suspend [id]...",
	Aliases:           []string{"sus"},
	Short:             "Suspend want executions",
	Args:              cobra.MinimumNArgs(1),
	ValidArgsFunction: completeWantIDs,
	Run: func(cmd *cobra.Command, args []string) {
		runBatchOperation(args, "suspend", (*client.Client).SuspendWants)
	},
}

var resumeWantsCmd = &cobra.Command{
	Use:               "resume [id]...",
	Aliases:           []string{"res"},
	Short:             "Resume want executions",
	Args:              cobra.MinimumNArgs(1),
	ValidArgsFunction: completeWantIDs,
	Run: func(cmd *cobra.Command, args []string) {
		runBatchOperation(args, "resume", (*client.Client).ResumeWants)
	},
}

var stopWantsCmd = &cobra.Command{
	Use:               "stop [id]...",
	Aliases:           []string{"st"},
	Short:             "Stop want executions",
	Args:              cobra.MinimumNArgs(1),
	ValidArgsFunction: completeWantIDs,
	Run: func(cmd *cobra.Command, args []string) {
		runBatchOperation(args, "stop", (*client.Client).StopWants)
	},
}

var startWantsCmd = &cobra.Command{
	Use:               "start [id]...",
	Aliases:           []string{"sta"},
	Short:             "Start want executions",
	Args:              cobra.MinimumNArgs(1),
	ValidArgsFunction: completeWantIDs,
	Run: func(cmd *cobra.Command, args []string) {
		runBatchOperation(args, "start", (*client.Client).StartWants)
	},
}

var exportWantsCmd = &cobra.Command{
	Use:     "export",
	Aliases: []string{"e"},
	Short:   "Export all wants as YAML",
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
	Use:     "import",
	Aliases: []string{"i"},
	Short:   "Import wants from YAML file",
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
