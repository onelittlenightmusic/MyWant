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

var RecipesCmd = &cobra.Command{
	Use:   "recipes",
	Short: "Manage recipes",
	Long:  `List, create, view, and generate recipes from wants.`,
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
	Use:   "list",
	Short: "List all recipes",
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
	Use:   "create",
	Short: "Create a new recipe from file",
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
	},
}

var deleteRecipeCmd = &cobra.Command{
	Use:               "delete [id]",
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

var fromWantCmd = &cobra.Command{
	Use:   "from-want [want-id]",
	Short: "Create recipe from existing want",
	Args:  cobra.ExactArgs(1),
	// Complete want IDs from wants command helper
	ValidArgsFunction: completeWantIDs,
	Run: func(cmd *cobra.Command, args []string) {
		name, _ := cmd.Flags().GetString("name")
		desc, _ := cmd.Flags().GetString("description")
		ver, _ := cmd.Flags().GetString("version")

		if name == "" {
			fmt.Println("Error: --name flag is required")
			os.Exit(1)
		}

		req := client.SaveRecipeFromWantRequest{
			WantID: args[0],
			Metadata: client.RecipeMetadata{
				Name:        name,
				Description: desc,
				Version:     ver,
			},
		}

		c := client.NewClient(viper.GetString("server"))
		resp, err := c.SaveRecipeFromWant(req)
		if err != nil {
			fmt.Printf("Error creating recipe from want: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Recipe '%s' created from want '%s'\n", resp.ID, args[0])
		fmt.Printf("Saved to: %s\n", resp.File)
		fmt.Printf("Contains %d wants\n", resp.Wants)
	},
}

func init() {
	RecipesCmd.AddCommand(listRecipesCmd)
	RecipesCmd.AddCommand(getRecipeCmd)
	RecipesCmd.AddCommand(createRecipeCmd)
	RecipesCmd.AddCommand(deleteRecipeCmd)
	RecipesCmd.AddCommand(fromWantCmd)

	createRecipeCmd.Flags().StringP("file", "f", "", "Path to recipe YAML/JSON file")

	fromWantCmd.Flags().StringP("name", "n", "", "Name of the new recipe")
	fromWantCmd.Flags().StringP("description", "d", "", "Description of the recipe")
	fromWantCmd.Flags().StringP("version", "v", "1.0.0", "Version of the recipe")
}

func truncate(s string, max int) string {
	if len(s) > max {
		return s[:max] + "..."
	}
	return s
}