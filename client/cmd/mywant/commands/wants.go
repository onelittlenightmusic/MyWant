package commands

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
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
	Long: `Create a new want.

Modes:
  -f file.yaml          Create from YAML/JSON file
  -t <type> [-e]        Create want of specific type, optionally with example parameters
  -i                    Interactive mode: prompts for all inputs`,
	Run: func(cmd *cobra.Command, args []string) {
		interactive, _ := cmd.Flags().GetBool("interactive")
		file, _ := cmd.Flags().GetString("file")
		wantType, _ := cmd.Flags().GetString("type")
		useExample, _ := cmd.Flags().GetBool("example")

		if interactive {
			runInteractiveCreateWant(cmd)
			return
		}

		if file == "" && wantType == "" {
			fmt.Println("Error: either --file, --type, or --interactive flag is required")
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
	createWantCmd.Flags().BoolP("interactive", "i", false, "Full interactive mode (prompts for all inputs)")
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

func runInteractiveCreateWant(cmd *cobra.Command) {
	fmt.Println("--- Create New Want (Interactive Mode) ---")

	c := client.NewClient(viper.GetString("server"))

	// 1. Get Want Type
	fmt.Println("\nFetching available Want Types...")
	wantTypes, err := c.ListWantTypes()
	if err != nil {
		fmt.Printf("Error fetching want types: %v\n", err)
		os.Exit(1)
	}
	if len(wantTypes) == 0 {
		fmt.Println("No want types found. Cannot create a want interactively.")
		os.Exit(1)
	}

	wantTypeLabels := make([]string, len(wantTypes))
	for i, wt := range wantTypes {
		wantTypeLabels[i] = fmt.Sprintf("%s (Category: %s, Version: %s)", wt.Name, wt.Category, wt.Version)
	}
	selectedTypeIdx := promptChoice("Select a Want Type", wantTypeLabels, 0)
	selectedWantType := wantTypes[selectedTypeIdx].Name
	fmt.Printf("Selected Want Type: %s\n", selectedWantType)

	// 2. Get Want Name
	wantName := promptInput("Enter Want Name", "new-"+selectedWantType)
	for wantName == "" {
		fmt.Print("Want Name cannot be empty. Enter Want Name: ")
		wantName = readLine()
	}

	// 3. Get Want Labels (Optional)
	labels := make(map[string]string)
	for {
		addLabel := promptInput("Add a label? (e.g., 'key=value') (press Enter to skip)", "")
		if addLabel == "" {
			break
		}
		parts := strings.SplitN(addLabel, "=", 2)
		if len(parts) == 2 {
			labels[parts[0]] = parts[1]
		} else {
			fmt.Println("Invalid label format. Please use 'key=value'.")
		}
	}

	// 4. Get Want Parameters (Spec.Params) - enhanced interactive input
	var params map[string]any

	paramSourceChoices := []string{
		"Start from an example",
		"Start from scratch (empty parameters)",
	}
	paramSourceIdx := promptChoice("How do you want to define parameters?", paramSourceChoices, 1) // Default to scratch

	if paramSourceIdx == 0 { // Start from an example
		fmt.Printf("\nFetching example parameters for '%s'...\n", selectedWantType)
		exResp, err := c.GetWantTypeExamples(selectedWantType)
		if err != nil {
			fmt.Printf("Error fetching example parameters: %v\n", err)
			fmt.Println("Proceeding with empty parameters.")
			params = make(map[string]any)
		} else {
			if examples, ok := (*exResp)["examples"].([]any); ok && len(examples) > 0 {
				exampleBytes, _ := json.Marshal(examples[0])
				var exampleWant client.Want
				if err := json.Unmarshal(exampleBytes, &exampleWant); err == nil {
					params = exampleWant.Spec.Params
					fmt.Println("Example parameters loaded.")
				} else {
					fmt.Printf("Warning: Could not unmarshal example parameters: %v\n", err)
					fmt.Println("Proceeding with empty parameters.")
					params = make(map[string]any)
				}
			} else {
				fmt.Println("No example parameters found for this Want Type. Proceeding with empty parameters.")
				params = make(map[string]any)
			}
		}
	} else { // Start from scratch
		params = make(map[string]any)
		fmt.Println("Starting with empty parameters.")
	}

	// Loop for parameter customization and validation
	for {
		fmt.Println("\n--- Current Parameters (YAML) ---")
		if len(params) > 0 {
			paramBytes, _ := yaml.Marshal(params)
			fmt.Println(string(paramBytes))
		} else {
			fmt.Println("(No parameters defined yet)")
		}

		paramActionChoices := []string{
			"Edit YAML directly",
			"Proceed with these parameters",
		}
		paramActionIdx := promptChoice("Parameters are defined. What do you want to do?", paramActionChoices, 1)

		if paramActionIdx == 0 { // Edit YAML
			fmt.Println("\n--- Paste new YAML content for parameters ---")
			fmt.Println("Press Ctrl+D (Unix) or Ctrl+Z (Windows) then Enter when done.")
			paramReader := bufio.NewReader(os.Stdin)
			var paramInput strings.Builder
			for {
				line, err := paramReader.ReadString('\n')
				if err != nil { // EOF or other error
					break
				}
				paramInput.WriteString(line)
			}

			newParamStr := strings.TrimSpace(paramInput.String())
			if newParamStr != "" {
				var tempParams map[string]any
				if err := yaml.Unmarshal([]byte(newParamStr), &tempParams); err != nil {
					fmt.Printf("Error: Could not parse YAML input: %v\n", err)
					fmt.Println("Current parameters are unchanged.")
				} else {
					params = tempParams
					fmt.Println("Parameters updated from input.")
				}
			} else {
				fmt.Println("No input received. Parameters remain unchanged.")
			}
		} else { // Proceed with these parameters
			// 5. Validation Step
			newWant := client.Want{
				Metadata: client.Metadata{
					Name:   wantName,
					Type:   selectedWantType,
					Labels: labels,
				},
				Spec: client.WantSpec{
					Params: params,
				},
			}
			configForValidation := client.Config{Wants: []*client.Want{&newWant}}

			fmt.Println("\n--- Validating Want Configuration ---")
			validationResult, valErr := c.ValidateWantConfig(configForValidation)
			if valErr != nil {
				fmt.Printf("Error during validation: %v\n", valErr)
				fmt.Println("Please review your parameters and try again.")
				continue // Loop back to allow editing
			}

			if validationResult.Valid {
				fmt.Println("Validation successful: Want configuration is valid.")
				if len(validationResult.Warnings) > 0 {
					fmt.Println("Warnings:")
					for _, warn := range validationResult.Warnings {
						fmt.Printf("  - [%s] %s: %s (Field: %s, Suggestion: %s)\n", warn.WarningType, warn.WantName, warn.Message, warn.Field, warn.Suggestion)
					}
					confirmProceed := promptInput("Proceed despite warnings? (y/N)", "n")
					if strings.ToLower(confirmProceed) != "y" {
						fmt.Println("Returning to parameter editing to address warnings.")
						continue // Loop back to allow editing
					}
				}
				break // Exit customization loop if valid and no fatal errors
			} else {
				fmt.Println("Validation failed!")
				if len(validationResult.FatalErrors) > 0 {
					fmt.Println("Fatal Errors:")
					for _, errItem := range validationResult.FatalErrors {
						fmt.Printf("  - [%s] %s: %s (Field: %s, Details: %s)\n", errItem.ErrorType, errItem.WantName, errItem.Message, errItem.Field, errItem.Details)
					}
				}
				fmt.Println("Please review your parameters and try again.")
				continue // Loop back to allow editing
			}
		}
	}

	// 6. Final Confirmation and Creation
	fmt.Println("\n--- Final Review Before Creation ---")
	fmt.Printf("Name: %s\n", wantName)
	fmt.Printf("Type: %s\n", selectedWantType)
	if len(labels) > 0 {
		fmt.Printf("Labels: %v\n", labels)
	}
	if len(params) > 0 {
		fmt.Println("Parameters:")
		paramBytes, _ := yaml.Marshal(params)
		fmt.Println(string(paramBytes))
	}

	confirm := promptInput("Create this Want? (y/N)", "n")
	if strings.ToLower(confirm) != "y" {
		fmt.Println("Want creation cancelled.")
		return
	}

	finalWant := client.Want{
		Metadata: client.Metadata{
			Name:   wantName,
			Type:   selectedWantType,
			Labels: labels,
		},
		Spec: client.WantSpec{
			Params: params,
		},
	}

	finalConfig := client.Config{Wants: []*client.Want{&finalWant}}

	resp, err := c.CreateWant(finalConfig)
	if err != nil {
		fmt.Printf("Error creating want: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\nSuccessfully created want execution %s with %d wants\n", resp.ID, len(resp.WantIDs))
	for _, id := range resp.WantIDs {
		fmt.Printf("- %s\n", id)
	}
}
