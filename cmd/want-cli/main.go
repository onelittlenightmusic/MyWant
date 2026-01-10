package main

import (
	"fmt"
	"os"

	"mywant/cmd/want-cli/commands"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	cfgFile string
	server  string
)

var rootCmd = &cobra.Command{
	Use:   "want-cli",
	Short: "CLI for MyWant API",
	Long: `A command line interface for managing MyWant executions,
recipes, agents, and more.`,
}

func main() {
	// Register commands
	rootCmd.AddCommand(commands.WantsCmd)
	rootCmd.AddCommand(commands.RecipesCmd)
	rootCmd.AddCommand(commands.AgentsCmd)
	rootCmd.AddCommand(commands.CapabilitiesCmd)
	rootCmd.AddCommand(commands.TypesCmd)
	rootCmd.AddCommand(commands.LlmCmd)
	rootCmd.AddCommand(commands.LogsCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.want-cli.yaml)")
	rootCmd.PersistentFlags().StringVar(&server, "server", "http://localhost:8080", "MyWant server URL")

	viper.BindPFlag("server", rootCmd.PersistentFlags().Lookup("server"))
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		viper.AddConfigPath(home)
		viper.SetConfigName(".want-cli")
	}

	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err == nil {
		// fmt.Println("Using config file:", viper.ConfigFileUsed())
	}
}
