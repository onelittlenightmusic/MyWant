package commands

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"mywant/pkg/client"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

var InteractCmd = &cobra.Command{
	Use:     "interact",
	Aliases: []string{"i"},
	Short:   "Interactive want creation",
	Long:    `Create wants interactively using natural language conversations.`,
}

var startCmd = &cobra.Command{
	Use:     "start",
	Aliases: []string{"st"},
	Short:   "Start an interactive want creation session",
	Long:    `Creates a new interaction session and returns a session ID for subsequent operations.`,
	Run: func(cmd *cobra.Command, args []string) {
		c := client.NewClient(viper.GetString("server"))
		session, err := c.CreateSession()
		if err != nil {
			fmt.Printf("Error creating session: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Session created: %s\n", session.SessionID)
		fmt.Printf("Created at: %s\n", session.CreatedAt.Format(time.RFC3339))
		fmt.Printf("Expires at: %s\n", session.ExpiresAt.Format(time.RFC3339))
		fmt.Printf("\nUse this session ID to send messages:\n")
		fmt.Printf("  want-cli interact send %s \"I want to book a hotel in Tokyo\"\n", session.SessionID)
	},
}

var sendCmd = &cobra.Command{
	Use:     "send [session_id] [message]",
	Aliases: []string{"s"},
	Short:   "Send message to interactive session",
	Long:    `Sends a message to an existing session and receives recommendations.`,
	Args:    cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		sessionID := args[0]
		message := args[1]

		c := client.NewClient(viper.GetString("server"))

		// Get optional context flags
		preferRecipes, _ := cmd.Flags().GetBool("prefer-recipes")
		categories, _ := cmd.Flags().GetStringSlice("categories")

		var context *client.InteractContext
		if preferRecipes || len(categories) > 0 {
			context = &client.InteractContext{
				PreferRecipes: preferRecipes,
				Categories:    categories,
			}
		}

		req := client.InteractMessageRequest{
			Message: message,
			Context: context,
		}

		resp, err := c.SendMessage(sessionID, req)
		if err != nil {
			fmt.Printf("Error sending message: %v\n", err)
			os.Exit(1)
		}

		// Display recommendations
		fmt.Printf("\n=== Recommendations ===\n\n")
		for i, rec := range resp.Recommendations {
			fmt.Printf("[%d] %s (ID: %s)\n", i+1, rec.Title, rec.ID)
			fmt.Printf("    Approach: %s\n", rec.Approach)
			fmt.Printf("    Description: %s\n", rec.Description)
			fmt.Printf("    Complexity: %s\n", rec.Metadata.Complexity)
			fmt.Printf("    Want Count: %d\n", rec.Metadata.WantCount)

			if len(rec.Metadata.RecipesUsed) > 0 {
				fmt.Printf("    Recipes: %s\n", strings.Join(rec.Metadata.RecipesUsed, ", "))
			}

			if len(rec.Metadata.ProsCons.Pros) > 0 {
				fmt.Printf("    Pros:\n")
				for _, pro := range rec.Metadata.ProsCons.Pros {
					fmt.Printf("      + %s\n", pro)
				}
			}

			if len(rec.Metadata.ProsCons.Cons) > 0 {
				fmt.Printf("    Cons:\n")
				for _, con := range rec.Metadata.ProsCons.Cons {
					fmt.Printf("      - %s\n", con)
				}
			}

			fmt.Println()
		}

		fmt.Printf("To deploy a recommendation:\n")
		if len(resp.Recommendations) > 0 {
			fmt.Printf("  want-cli interact deploy %s %s\n", sessionID, resp.Recommendations[0].ID)
		}

		outputFormat, _ := cmd.Flags().GetString("output")
		if outputFormat == "json" {
			data, _ := json.MarshalIndent(resp, "", "  ")
			fmt.Printf("\n=== JSON Output ===\n%s\n", string(data))
		} else if outputFormat == "yaml" {
			data, _ := yaml.Marshal(resp)
			fmt.Printf("\n=== YAML Output ===\n%s\n", string(data))
		}
	},
}

var deployCmd = &cobra.Command{
	Use:     "deploy [session_id] [recommendation_id]",
	Aliases: []string{"d"},
	Short:   "Deploy a recommendation from session",
	Long:    `Deploys a selected recommendation, creating the actual wants.`,
	Args:    cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		sessionID := args[0]
		recID := args[1]

		c := client.NewClient(viper.GetString("server"))

		// Get optional modifications
		paramsOverride, _ := cmd.Flags().GetStringToString("params")
		disableWants, _ := cmd.Flags().GetStringSlice("disable")

		var mods *client.ConfigModifications
		if len(paramsOverride) > 0 || len(disableWants) > 0 {
			mods = &client.ConfigModifications{}
			if len(paramsOverride) > 0 {
				mods.ParameterOverrides = make(map[string]any)
				for k, v := range paramsOverride {
					mods.ParameterOverrides[k] = v
				}
			}
			if len(disableWants) > 0 {
				mods.DisableWants = disableWants
			}
		}

		req := client.InteractDeployRequest{
			RecommendationID: recID,
			Modifications:    mods,
		}

		resp, err := c.DeployRecommendation(sessionID, req)
		if err != nil {
			fmt.Printf("Error deploying recommendation: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Deployment successful!\n")
		fmt.Printf("Execution ID: %s\n", resp.ExecutionID)
		fmt.Printf("Status: %s\n", resp.Status)
		fmt.Printf("Message: %s\n", resp.Message)
		fmt.Printf("Want IDs created: %v\n", resp.WantIDs)

		fmt.Printf("\nTo view the deployed wants:\n")
		fmt.Printf("  want-cli wants list\n")
		if len(resp.WantIDs) > 0 {
			fmt.Printf("  want-cli wants get %s\n", resp.WantIDs[0])
		}
	},
}

var endCmd = &cobra.Command{
	Use:     "end [session_id]",
	Aliases: []string{"e"},
	Short:   "End interactive session",
	Long:    `Terminates an interaction session and removes it from the cache.`,
	Args:    cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		sessionID := args[0]

		c := client.NewClient(viper.GetString("server"))
		err := c.DeleteSession(sessionID)
		if err != nil {
			fmt.Printf("Error ending session: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Session %s ended successfully\n", sessionID)
	},
}

var shellCmd = &cobra.Command{
	Use:     "shell [session_id]",
	Aliases: []string{"sh"},
	Short:   "Enter interactive shell for want creation",
	Long:    `Starts an interactive shell session where you can have a conversation to create wants.`,
	Args:    cobra.MinimumNArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		c := client.NewClient(viper.GetString("server"))

		var sessionID string

		// Create new session if not provided
		if len(args) == 0 {
			session, err := c.CreateSession()
			if err != nil {
				fmt.Printf("Error creating session: %v\n", err)
				os.Exit(1)
			}
			sessionID = session.SessionID
			fmt.Printf("Created new session: %s\n", sessionID)
			fmt.Printf("Expires at: %s\n\n", session.ExpiresAt.Format(time.RFC3339))
		} else {
			sessionID = args[0]
			fmt.Printf("Using session: %s\n\n", sessionID)
		}

		fmt.Println("Interactive Want Creation Shell")
		fmt.Println("===============================")
		fmt.Println("Commands:")
		fmt.Println("  Type your message to get recommendations")
		fmt.Println("  'deploy <rec-id>' to deploy a recommendation")
		fmt.Println("  'exit' or 'quit' to exit")
		fmt.Println()

		scanner := bufio.NewScanner(os.Stdin)

		for {
			fmt.Print("> ")
			if !scanner.Scan() {
				break
			}

			input := strings.TrimSpace(scanner.Text())
			if input == "" {
				continue
			}

			// Handle exit commands
			if input == "exit" || input == "quit" {
				fmt.Println("Ending session...")
			c.DeleteSession(sessionID)
				break
			}

			// Handle deploy command
			if strings.HasPrefix(input, "deploy ") {
				recID := strings.TrimSpace(strings.TrimPrefix(input, "deploy "))
				req := client.InteractDeployRequest{
					RecommendationID: recID,
				}

				resp, err := c.DeployRecommendation(sessionID, req)
				if err != nil {
					fmt.Printf("Error: %v\n\n", err)
					continue
				}

				fmt.Printf("\nDeployment successful!\n")
				fmt.Printf("Execution ID: %s\n", resp.ExecutionID)
				fmt.Printf("Want IDs: %v\n\n", resp.WantIDs)
				continue
			}

			// Send message
			req := client.InteractMessageRequest{
				Message: input,
			}

			resp, err := c.SendMessage(sessionID, req)
			if err != nil {
				fmt.Printf("Error: %v\n\n", err)
				continue
			}

			// Display recommendations
			fmt.Printf("\n--- Recommendations ---\n\n")
			for i, rec := range resp.Recommendations {
				fmt.Printf("[%d] %s (ID: %s)\n", i+1, rec.Title, rec.ID)
				fmt.Printf("    %s\n", rec.Description)
				fmt.Printf("    Complexity: %s | Wants: %d\n", rec.Metadata.Complexity, rec.Metadata.WantCount)
				if len(rec.Metadata.RecipesUsed) > 0 {
					fmt.Printf("    Recipes: %s\n", strings.Join(rec.Metadata.RecipesUsed, ", "))
				}
				fmt.Println()
			}

			if len(resp.Recommendations) > 0 {
				fmt.Printf("To deploy: deploy %s\n", resp.Recommendations[0].ID)
			}
			fmt.Println()
		}

		if err := scanner.Err(); err != nil {
			fmt.Printf("Error reading input: %v\n", err)
		}
	},
}

func init() {
	InteractCmd.AddCommand(startCmd)
	InteractCmd.AddCommand(sendCmd)
	InteractCmd.AddCommand(deployCmd)
	InteractCmd.AddCommand(endCmd)
	InteractCmd.AddCommand(shellCmd)

	// Flags for send command
	sendCmd.Flags().BoolP("prefer-recipes", "r", false, "Prefer recipe-based recommendations")
	sendCmd.Flags().StringSliceP("categories", "c", nil, "Filter by want type categories")
	sendCmd.Flags().StringP("output", "o", "", "Output format: json, yaml")

	// Flags for deploy command
	deployCmd.Flags().StringToStringP("params", "p", nil, "Override parameters (key=value)")
	deployCmd.Flags().StringSliceP("disable", "d", nil, "Disable specific wants by name")
}
