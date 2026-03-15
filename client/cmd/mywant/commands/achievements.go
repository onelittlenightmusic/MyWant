package commands

import (
	"encoding/json"
	"fmt"
	"mywant/client"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

var AchievementsCmd = &cobra.Command{
	Use:     "achievements",
	Aliases: []string{"ach"},
	Short:   "Manage agent achievements",
	Long:    `List, get, create, and delete agent achievements.`,
}

var listAchievementsCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"l"},
	Short:   "List all achievements",
	Run: func(cmd *cobra.Command, args []string) {
		c := client.NewClient(viper.GetString("server"))
		resp, err := c.ListAchievements()
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		agentFilter, _ := cmd.Flags().GetString("agent")

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "ID\tTitle\tAgent\tLevel\tCategory\tUnlocks\tEarnedAt")
		for _, a := range resp.Achievements {
			if agentFilter != "" && a.AgentName != agentFilter {
				continue
			}
			unlocks := a.UnlocksCapability
			if unlocks == "" {
				unlocks = "-"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\t%s\t%s\n",
				a.ID, a.Title, a.AgentName, a.Level, a.Category,
				unlocks, a.EarnedAt.Format(time.RFC3339),
			)
		}
		w.Flush()
		fmt.Printf("\nTotal: %d\n", resp.Count)
	},
}

var getAchievementCmd = &cobra.Command{
	Use:   "get [id]",
	Short: "Get achievement details",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		c := client.NewClient(viper.GetString("server"))
		a, err := c.GetAchievement(args[0])
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		data, _ := json.MarshalIndent(a, "", "  ")
		fmt.Println(string(data))
	},
}

var createAchievementCmd = &cobra.Command{
	Use:   "create",
	Short: "Create an achievement manually",
	Run: func(cmd *cobra.Command, args []string) {
		c := client.NewClient(viper.GetString("server"))

		file, _ := cmd.Flags().GetString("file")
		if file != "" {
			data, err := os.ReadFile(file)
			if err != nil {
				fmt.Printf("Error reading file: %v\n", err)
				os.Exit(1)
			}
			var a client.Achievement
			if err := yaml.Unmarshal(data, &a); err != nil {
				fmt.Printf("Error parsing file: %v\n", err)
				os.Exit(1)
			}
			created, err := c.CreateAchievement(a)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("Created achievement: %s (%s)\n", created.ID, created.Title)
			return
		}

		// Inline flags
		agentName, _ := cmd.Flags().GetString("agent")
		title, _ := cmd.Flags().GetString("title")
		description, _ := cmd.Flags().GetString("description")
		category, _ := cmd.Flags().GetString("category")
		level, _ := cmd.Flags().GetInt("level")
		unlocks, _ := cmd.Flags().GetString("unlocks")

		if agentName == "" || title == "" {
			fmt.Println("--agent and --title are required")
			os.Exit(1)
		}

		a := client.Achievement{
			AgentName:         agentName,
			Title:             title,
			Description:       description,
			Category:          category,
			Level:             level,
			UnlocksCapability: unlocks,
			AwardedBy:         "human",
		}
		created, err := c.CreateAchievement(a)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Created achievement: %s (%s) for agent %s\n", created.ID, created.Title, created.AgentName)
	},
}

var deleteAchievementCmd = &cobra.Command{
	Use:   "delete [id]",
	Short: "Delete an achievement",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		c := client.NewClient(viper.GetString("server"))
		if err := c.DeleteAchievement(args[0]); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Deleted achievement: %s\n", args[0])
	},
}

var lockAchievementCmd = &cobra.Command{
	Use:   "lock [id]",
	Short: "Deactivate an achievement's capability (set unlocked=false)",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		c := client.NewClient(viper.GetString("server"))
		a, err := c.LockAchievement(args[0])
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Locked achievement: %s (%s) for agent %s\n", a.ID, a.Title, a.AgentName)
		if a.UnlocksCapability != "" {
			fmt.Printf("  Capability now inactive: %s\n", a.UnlocksCapability)
		}
	},
}

var unlockAchievementCmd = &cobra.Command{
	Use:   "unlock [id]",
	Short: "Activate an achievement's capability (set unlocked=true)",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		c := client.NewClient(viper.GetString("server"))
		a, err := c.UnlockAchievement(args[0])
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Unlocked achievement: %s (%s) for agent %s\n", a.ID, a.Title, a.AgentName)
		if a.UnlocksCapability != "" {
			fmt.Printf("  Capability now active: %s\n", a.UnlocksCapability)
		}
	},
}

// ── Rules subcommand ──────────────────────────────────────────────────────────

var rulesCmd = &cobra.Command{
	Use:   "rules",
	Short: "Manage achievement rules",
}

var listRulesCmd = &cobra.Command{
	Use:   "list",
	Short: "List all achievement rules",
	Run: func(cmd *cobra.Command, args []string) {
		c := client.NewClient(viper.GetString("server"))
		resp, err := c.ListAchievementRules()
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "ID\tActive\tCondition\tAward\tLevel\tUnlocks")
		for _, r := range resp.Rules {
			cond := fmt.Sprintf("%s >= %d", r.Condition.AgentCapability, r.Condition.CompletedCount)
			if r.Condition.AgentCapability == "" {
				cond = fmt.Sprintf("any >= %d", r.Condition.CompletedCount)
			}
			unlocks := r.Award.UnlocksCapability
			if unlocks == "" {
				unlocks = "-"
			}
			fmt.Fprintf(w, "%s\t%v\t%s\t%s\t%d\t%s\n",
				r.ID, r.Active, cond, r.Award.Title, r.Award.Level, unlocks)
		}
		w.Flush()
	},
}

var createRuleCmd = &cobra.Command{
	Use:   "create",
	Short: "Create an achievement rule",
	Run: func(cmd *cobra.Command, args []string) {
		c := client.NewClient(viper.GetString("server"))

		file, _ := cmd.Flags().GetString("file")
		if file != "" {
			data, err := os.ReadFile(file)
			if err != nil {
				fmt.Printf("Error reading file: %v\n", err)
				os.Exit(1)
			}
			var r client.AchievementRule
			if err := yaml.Unmarshal(data, &r); err != nil {
				fmt.Printf("Error parsing file: %v\n", err)
				os.Exit(1)
			}
			created, err := c.CreateAchievementRule(r)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("Created rule: %s (%s)\n", created.ID, created.Award.Title)
			return
		}

		capability, _ := cmd.Flags().GetString("capability")
		count, _ := cmd.Flags().GetInt("count")
		title, _ := cmd.Flags().GetString("title")
		description, _ := cmd.Flags().GetString("description")
		level, _ := cmd.Flags().GetInt("level")
		category, _ := cmd.Flags().GetString("category")
		unlocks, _ := cmd.Flags().GetString("unlocks")

		if title == "" || count == 0 {
			fmt.Println("--title and --count are required")
			os.Exit(1)
		}

		r := client.AchievementRule{
			Active: true,
			Condition: client.AchievementCondition{
				AgentCapability: capability,
				CompletedCount:  count,
			},
			Award: client.AchievementAward{
				Title:             title,
				Description:       description,
				Level:             level,
				Category:          category,
				UnlocksCapability: unlocks,
			},
		}
		created, err := c.CreateAchievementRule(r)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Created rule: %s (%s)\n", created.ID, created.Award.Title)
	},
}

var deleteRuleCmd = &cobra.Command{
	Use:   "delete [id]",
	Short: "Delete an achievement rule",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		c := client.NewClient(viper.GetString("server"))
		if err := c.DeleteAchievementRule(args[0]); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Deleted rule: %s\n", args[0])
	},
}

func init() {
	listAchievementsCmd.Flags().StringP("agent", "a", "", "Filter by agent name")

	createAchievementCmd.Flags().StringP("file", "f", "", "YAML file containing achievement definition")
	createAchievementCmd.Flags().String("agent", "", "Agent name")
	createAchievementCmd.Flags().String("title", "", "Achievement title")
	createAchievementCmd.Flags().String("description", "", "Achievement description")
	createAchievementCmd.Flags().String("category", "execution", "Category (execution/quality/specialization)")
	createAchievementCmd.Flags().Int("level", 1, "Level: 1=bronze 2=silver 3=gold")
	createAchievementCmd.Flags().String("unlocks", "", "Capability unlocked by this achievement")

	createRuleCmd.Flags().StringP("file", "f", "", "YAML file containing rule definition")
	createRuleCmd.Flags().String("capability", "", "Agent capability to count (empty = any)")
	createRuleCmd.Flags().Int("count", 0, "Minimum completions to trigger")
	createRuleCmd.Flags().String("title", "", "Achievement title to award")
	createRuleCmd.Flags().String("description", "", "Achievement description")
	createRuleCmd.Flags().Int("level", 1, "Level: 1=bronze 2=silver 3=gold")
	createRuleCmd.Flags().String("category", "execution", "Category")
	createRuleCmd.Flags().String("unlocks", "", "Capability to unlock")

	rulesCmd.AddCommand(listRulesCmd)
	rulesCmd.AddCommand(createRuleCmd)
	rulesCmd.AddCommand(deleteRuleCmd)

	AchievementsCmd.AddCommand(listAchievementsCmd)
	AchievementsCmd.AddCommand(getAchievementCmd)
	AchievementsCmd.AddCommand(createAchievementCmd)
	AchievementsCmd.AddCommand(deleteAchievementCmd)
	AchievementsCmd.AddCommand(lockAchievementCmd)
	AchievementsCmd.AddCommand(unlockAchievementCmd)
	AchievementsCmd.AddCommand(rulesCmd)
}
