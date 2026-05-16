package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"mywant/client/cmd/mywant/commands"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	cfgFile string
	server  string
	version = "dev"
)

var rootCmd = &cobra.Command{
	Use:     "mywant",
	Version: version,
	Short:   "MyWant CLI - Declarative Chain Programming System",
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

	rootCmd.AddCommand(commands.AchievementsCmd)

	rootCmd.AddCommand(commands.CapabilitiesCmd)

	rootCmd.AddCommand(commands.TypesCmd)

	rootCmd.AddCommand(commands.InteractCmd)

	rootCmd.AddCommand(commands.LogsCmd)

	rootCmd.AddCommand(commands.StartCmd)

	rootCmd.AddCommand(commands.StopCmd)

	rootCmd.AddCommand(commands.PsCmd)

	rootCmd.AddCommand(commands.ConfigCmd)

	rootCmd.AddCommand(commands.MemoCmd)

	rootCmd.AddCommand(commands.ParamsCmd)

	rootCmd.AddCommand(commands.StateCmd)

	rootCmd.AddCommand(commands.PluginCmd)

	rootCmd.AddCommand(commands.SkillsCmd)

	// kubectl-style plugin dispatch: if the first arg is not a known command,
	// look for mywant-<arg> in PATH and exec it.
	if len(os.Args) > 1 {
		firstArg := os.Args[1]
		if !strings.HasPrefix(firstArg, "-") && !isKnownCommand(rootCmd, firstArg) {
			pluginName := "mywant-" + firstArg
			if pluginPath, err := exec.LookPath(pluginName); err == nil {
				pluginCmd := exec.Command(pluginPath, os.Args[2:]...)
				pluginCmd.Stdin = os.Stdin
				pluginCmd.Stdout = os.Stdout
				pluginCmd.Stderr = os.Stderr
				if err := pluginCmd.Run(); err != nil {
					if exitErr, ok := err.(*exec.ExitError); ok {
						os.Exit(exitErr.ExitCode())
					}
					fmt.Fprintln(os.Stderr, err)
					os.Exit(1)
				}
				os.Exit(0)
			}
		}
	}

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func isKnownCommand(root *cobra.Command, name string) bool {
	for _, cmd := range root.Commands() {
		if cmd.Name() == name {
			return true
		}
		for _, alias := range cmd.Aliases {
			if alias == name {
				return true
			}
		}
	}
	return false
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
