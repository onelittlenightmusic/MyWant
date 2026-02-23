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

var RecipesCmd = &cobra.Command{
	Use:     "recipes",
	Aliases: []string{"r"},
	Short:   "Manage recipes",
	Long:    `List, create, view, and generate recipes from wants.`,
}

// completion helper for recipes
func completeRecipeIDs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	c := client.NewClient(viper.GetString("server"))
	recipes, err := c.ListRecipes()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	var ids []string
	for id, r := range recipes {
		ids = append(ids, fmt.Sprintf("%s\t%s", id, r.Recipe.Metadata.Name))
	}
	return ids, cobra.ShellCompDirectiveNoFileComp
}

var listRecipesCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"l"},
	Short:   "List all recipes",
	Run: func(cmd *cobra.Command, args []string) {
		c := client.NewClient(viper.GetString("server"))
		recipes, err := c.ListRecipes()
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "ID\tName\tVersion\tDescription")

		for id, r := range recipes {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
				id,
				r.Recipe.Metadata.Name,
				r.Recipe.Metadata.Version,
				truncate(r.Recipe.Metadata.Description, 50),
			)
		}
		w.Flush()
	},
}

var getRecipeCmd = &cobra.Command{
	Use:               "get [id]",
	Aliases:           []string{"g"},
	Short:             "Get recipe details",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeRecipeIDs,
	Run: func(cmd *cobra.Command, args []string) {
		c := client.NewClient(viper.GetString("server"))
		recipe, err := c.GetRecipe(args[0])
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		yamlData, _ := yaml.Marshal(recipe)
		fmt.Println(string(yamlData))
	},
}

var createRecipeCmd = &cobra.Command{
	Use:     "create",
	Aliases: []string{"c"},
	Short:   "Create a new recipe",
	Long: `Create a new recipe.

Modes:
  -f file.yaml          Create from YAML/JSON file (existing behavior)
  --from-want <ID>      Create from an existing want (non-interactive)
  -i                    Interactive mode: prompts for all inputs`,
	Run: func(cmd *cobra.Command, args []string) {
		interactive, _ := cmd.Flags().GetBool("interactive")
		fromWant, _ := cmd.Flags().GetString("from-want")
		file, _ := cmd.Flags().GetString("file")

		switch {
		case interactive:
			runInteractiveCreate(cmd)
		case fromWant != "":
			runFromWant(cmd, fromWant)
		case file != "":
			runCreateFromFile(cmd, file)
		default:
			fmt.Fprintln(os.Stderr, "Error: one of --file, --from-want, or --interactive (-i) is required")
			cmd.Usage()
			os.Exit(1)
		}
	},
}

func runCreateFromFile(cmd *cobra.Command, file string) {
	data, err := os.ReadFile(file)
	if err != nil {
		fmt.Printf("Error reading file: %v\n", err)
		os.Exit(1)
	}

	var recipe client.GenericRecipe
	if err := yaml.Unmarshal(data, &recipe); err != nil {
		if err := json.Unmarshal(data, &recipe); err != nil {
			fmt.Printf("Error parsing file: %v\n", err)
			os.Exit(1)
		}
	}

	c := client.NewClient(viper.GetString("server"))
	err = c.CreateRecipe(recipe)
	if err != nil {
		fmt.Printf("Error creating recipe: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Recipe created successfully\n")
}

func runFromWant(cmd *cobra.Command, wantID string) {
	name, _ := cmd.Flags().GetString("name")
	if name == "" {
		fmt.Println("Error: --name is required with --from-want")
		os.Exit(1)
	}
	desc, _ := cmd.Flags().GetString("description")
	ver, _ := cmd.Flags().GetString("version")
	category, _ := cmd.Flags().GetString("category")
	customType, _ := cmd.Flags().GetString("custom-type")

	req := client.SaveRecipeFromWantRequest{
		WantID: wantID,
		Metadata: client.RecipeMetadata{
			Name:        name,
			Description: desc,
			Version:     ver,
			Category:    category,
			CustomType:  customType,
		},
	}

	c := client.NewClient(viper.GetString("server"))
	resp, err := c.SaveRecipeFromWant(req)
	if err != nil {
		fmt.Printf("Error creating recipe from want: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Recipe '%s' created from want '%s'\n", resp.ID, wantID)
	fmt.Printf("Saved to: %s\n", resp.File)
	fmt.Printf("Contains %d wants\n", resp.Wants)
}

func runInteractiveCreate(cmd *cobra.Command) {
	fmt.Println("--- Create Recipe ---")

	choices := []string{
		"From an existing Want",
		"Start from scratch",
	}
	sourceIdx := promptChoice("Source", choices, 0)

	c := client.NewClient(viper.GetString("server"))

	if sourceIdx == 0 {
		// --- From existing Want ---
		fmt.Println("\nFetching wants...")
		resp, err := c.ListWants("", []string{}, []string{})
		if err != nil {
			fmt.Printf("Error fetching wants: %v\n", err)
			os.Exit(1)
		}
		if len(resp.Wants) == 0 {
			fmt.Println("No wants found.")
			os.Exit(1)
		}

		wantLabels := make([]string, len(resp.Wants))
		for i, w := range resp.Wants {
			wantLabels[i] = fmt.Sprintf("%s  %s  (%s)", w.Metadata.ID, w.Metadata.Name, w.Metadata.Type)
		}
		wantIdx := promptChoice("Select a want", wantLabels, 0)
		selectedWant := resp.Wants[wantIdx]
		wantID := selectedWant.Metadata.ID

		fmt.Printf("\nAnalyzing want %s...\n", wantID)
		analysis, err := c.AnalyzeWantForRecipe(wantID)
		if err != nil {
			fmt.Printf("Error analyzing want: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Found %d child want(s).\n", analysis.ChildCount)

		var includedState []client.StateDef
		if len(analysis.RecommendedState) > 0 {
			fmt.Println("Detected state fields:")
			for _, sd := range analysis.RecommendedState {
				typeStr := sd.Type
				if typeStr == "" {
					typeStr = "any"
				}
				fmt.Printf("  %-20s (%s)  %s\n", sd.Name, typeStr, sd.Description)
			}
			includedState = analysis.RecommendedState
		}

		// Metadata
		fmt.Println("\n--- Recipe Metadata ---")
		defaultName := analysis.SuggestedMetadata.Name
		name := promptInput("Name", defaultName)
		if name == "" {
			name = defaultName
		}
		desc := promptInput("Description", "")
		ver := promptInput("Version", "1.0.0")
		if ver == "" {
			ver = "1.0.0"
		}
		category := promptInput("Category (general/approval/travel/mathematics/queue)", "general")
		if category == "" {
			category = "general"
		}
		customType := promptInput("Custom Type", "")

		fmt.Printf("\nSave recipe '%s'? (y/N): ", name)
		confirm := readLine()
		if strings.ToLower(strings.TrimSpace(confirm)) != "y" {
			fmt.Println("Cancelled.")
			return
		}

		req := client.SaveRecipeFromWantRequest{
			WantID: wantID,
			Metadata: client.RecipeMetadata{
				Name:        name,
				Description: desc,
				Version:     ver,
				Category:    category,
				CustomType:  customType,
			},
			State: includedState,
		}

		saveResp, err := c.SaveRecipeFromWant(req)
		if err != nil {
			fmt.Printf("Error saving recipe: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("\nRecipe '%s' saved.\n", saveResp.ID)
		fmt.Printf("File: %s\n", saveResp.File)
		fmt.Printf("Contains %d wants\n", saveResp.Wants)

	} else {
		// --- Scratch ---
		fmt.Println("\n--- Recipe Metadata ---")
		name := promptInput("Name", "")
		for name == "" {
			fmt.Print("Name cannot be empty. Name: ")
			name = readLine()
		}
		desc := promptInput("Description", "")
		ver := promptInput("Version", "1.0.0")
		if ver == "" {
			ver = "1.0.0"
		}
		category := promptInput("Category (general/approval/travel/mathematics/queue)", "general")
		if category == "" {
			category = "general"
		}
		customType := promptInput("Custom Type", "")

		recipe := client.GenericRecipe{
			Recipe: client.RecipeContent{
				Metadata: client.RecipeMetadata{
					Name:        name,
					Description: desc,
					Version:     ver,
					Category:    category,
					CustomType:  customType,
				},
				Wants: []any{},
			},
		}

		if err := c.CreateRecipe(recipe); err != nil {
			fmt.Printf("Error creating recipe: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("\nRecipe '%s' created.\n", name)
	}
}

var deleteRecipeCmd = &cobra.Command{
	Use:               "delete [id]",
	Aliases:           []string{"d"},
	Short:             "Delete a recipe",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeRecipeIDs,
	Run: func(cmd *cobra.Command, args []string) {
		c := client.NewClient(viper.GetString("server"))
		err := c.DeleteRecipe(args[0])
		if err != nil {
			fmt.Printf("Error deleting recipe: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Recipe %s deleted\n", args[0])
	},
}

// fromWantCmd is deprecated; kept for backward compatibility with a migration message.
var fromWantCmd = &cobra.Command{
	Use:        "from-want [want-id]",
	Aliases:    []string{"fw"},
	Short:      "Deprecated: use 'recipes create --from-want'",
	Deprecated: "use 'mywant recipes create --from-want <ID> --name <NAME>' instead",
	Args:       cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name, _ := cmd.Flags().GetString("name")
		fmt.Println("'recipes from-want' is deprecated.")
		fmt.Printf("Use: mywant recipes create --from-want %s --name %s\n", args[0], name)
	},
}

func init() {
	RecipesCmd.AddCommand(listRecipesCmd)
	RecipesCmd.AddCommand(getRecipeCmd)
	RecipesCmd.AddCommand(createRecipeCmd)
	RecipesCmd.AddCommand(deleteRecipeCmd)
	RecipesCmd.AddCommand(fromWantCmd)

	// createRecipeCmd flags
	createRecipeCmd.Flags().StringP("file", "f", "", "Path to recipe YAML/JSON file")
	createRecipeCmd.Flags().String("from-want", "", "Create from existing want (want ID)")
	createRecipeCmd.Flags().StringP("name", "n", "", "Name of the recipe")
	createRecipeCmd.Flags().StringP("description", "d", "", "Description of the recipe")
	createRecipeCmd.Flags().StringP("version", "v", "1.0.0", "Version of the recipe")
	createRecipeCmd.Flags().StringP("category", "c", "", "Category (general/approval/travel/mathematics/queue)")
	createRecipeCmd.Flags().String("custom-type", "", "Custom type identifier")
	createRecipeCmd.Flags().BoolP("interactive", "i", false, "Full interactive mode (prompts for all inputs)")

	// fromWantCmd flags (kept for deprecation message)
	fromWantCmd.Flags().StringP("name", "n", "", "Name of the new recipe")
	fromWantCmd.Flags().StringP("description", "d", "", "Description of the recipe")
	fromWantCmd.Flags().StringP("version", "v", "1.0.0", "Version of the recipe")
}

// promptInput prints a prompt with an optional default value and returns user input.
// If the user presses Enter without typing anything the default is returned.
func promptInput(prompt, defaultVal string) string {
	if defaultVal != "" {
		fmt.Printf("%s [%s]: ", prompt, defaultVal)
	} else {
		fmt.Printf("%s: ", prompt)
	}
	line := readLine()
	if line == "" {
		return defaultVal
	}
	return line
}

// promptChoice displays a numbered list and returns the 0-based index of the selection.
// defaultIdx is shown as the default when the user presses Enter.
func promptChoice(prompt string, choices []string, defaultIdx int) int {
	fmt.Printf("\n%s:\n", prompt)
	for i, c := range choices {
		marker := " "
		if i == defaultIdx {
			marker = "*"
		}
		fmt.Printf("  %s %d. %s\n", marker, i+1, c)
	}
	for {
		fmt.Printf("Choice [%d]: ", defaultIdx+1)
		line := strings.TrimSpace(readLine())
		if line == "" {
			return defaultIdx
		}
		var n int
		if _, err := fmt.Sscanf(line, "%d", &n); err == nil && n >= 1 && n <= len(choices) {
			return n - 1
		}
		fmt.Printf("Please enter a number between 1 and %d.\n", len(choices))
	}
}

var stdinReader *bufio.Reader

func readLine() string {
	if stdinReader == nil {
		stdinReader = bufio.NewReader(os.Stdin)
	}
	line, _ := stdinReader.ReadString('\n')
	return strings.TrimRight(line, "\r\n")
}

func truncate(s string, max int) string {
	if len(s) > max {
		return s[:max] + "..."
	}
	return s
}
