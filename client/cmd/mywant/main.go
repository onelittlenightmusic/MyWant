package main

import (
	"fmt"
	"os"
	"path/filepath"

	"mywant/client/cmd/mywant/commands"

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
	// Set up persistent pre-run to handle config file
	rootCmd.PersistentPreRun = preRunConfig

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

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.mywant/config.yaml)")
	rootCmd.PersistentFlags().StringVar(&server, "server", "http://localhost:8080", "MyWant server URL")

	viper.BindPFlag("server", rootCmd.PersistentFlags().Lookup("server"))
}

// preRunConfig is called before command execution to set custom config path
func preRunConfig(cmd *cobra.Command, args []string) {
	if cfgFile != "" {
		commands.SetConfigPath(cfgFile)
	}
}

func initConfig() {
	configPath := ""
	if cfgFile != "" {
		configPath = cfgFile
		viper.SetConfigFile(cfgFile)
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		// Use ~/.mywant/config.yaml
		mywantDir := filepath.Join(home, ".mywant")
		configPath = filepath.Join(mywantDir, "config.yaml")
		viper.AddConfigPath(mywantDir)
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
	}

	viper.AutomaticEnv()

	// Log the config path before reading
	if _, err := os.Stat(configPath); err == nil {
		fmt.Printf("Reading config from: %s\n", configPath)
	}

	if err := viper.ReadInConfig(); err == nil {
		// fmt.Println("Using config file:", viper.ConfigFileUsed())
	}
}
