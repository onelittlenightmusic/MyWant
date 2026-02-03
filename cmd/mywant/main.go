package main

import (
	"fmt"
	"os"

	"mywant/cmd/mywant/commands"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	cfgFile string
	server  string
)

var rootCmd = &cobra.Command{
	Use:   "mywant",
	Short: "MyWant CLI - Declarative Chain Programming System",
	Long: `MyWant is a declarative chain programming system where you express
"what you want" through YAML configuration. Autonomous agents collaborate
to solve your wants based on their capabilities.`,
}

func main() {

	// Register commands

	rootCmd.AddCommand(commands.WantsCmd)

	rootCmd.AddCommand(commands.RecipesCmd)

	rootCmd.AddCommand(commands.AgentsCmd)

	rootCmd.AddCommand(commands.CapabilitiesCmd)

	rootCmd.AddCommand(commands.TypesCmd)

	rootCmd.AddCommand(commands.InteractCmd)

	rootCmd.AddCommand(commands.LogsCmd)

	rootCmd.AddCommand(commands.StartCmd)

	rootCmd.AddCommand(commands.StopCmd)

	rootCmd.AddCommand(commands.PsCmd)

	rootCmd.AddCommand(commands.ConfigCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.mywant.yaml)")
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
		viper.SetConfigName(".mywant")
	}

	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err == nil {
		// fmt.Println("Using config file:", viper.ConfigFileUsed())
	}
}
