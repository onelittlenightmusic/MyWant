package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"mywant/client"
	"mywant/client/cmd/mywant/commands"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	cfgFile     string
	server      string
	contextName string
	version     = "dev"
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

	rootCmd.AddCommand(commands.RiffCmd)

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

	rootCmd.AddCommand(commands.CustomCmd)

	// kubectl-style plugin dispatch: if the first arg is not a known command,
	// look for mywant-<arg> in PATH and exec it.
	if len(os.Args) > 1 {
		firstArg := os.Args[1]
		if !strings.HasPrefix(firstArg, "-") && !isKnownCommand(rootCmd, firstArg) {
			pluginName := "mywant-" + firstArg
			if pluginPath, err := exec.LookPath(pluginName); err == nil {
				// Replace the current process with the plugin (kubectl/git style).
				// This keeps the same PID and process group, avoiding spurious
				// shell job notifications when the plugin spawns child processes.
				args := append([]string{pluginPath}, os.Args[2:]...)
				if err := syscall.Exec(pluginPath, args, os.Environ()); err != nil {
					fmt.Fprintf(os.Stderr, "Error: failed to exec plugin %s: %v\n", pluginName, err)
					os.Exit(1)
				}
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
	rootCmd.PersistentFlags().StringVar(&server, "server", commands.DefaultServerURL, "MyWant server URL (overrides the active context)")
	rootCmd.PersistentFlags().StringVar(&contextName, "context", "", "Use this context from config.yaml instead of current_context")

	viper.BindPFlag("server", rootCmd.PersistentFlags().Lookup("server"))
}

// preRunConfig is called before command execution to set custom config path
func preRunConfig(cmd *cobra.Command, args []string) {
	if cfgFile != "" {
		commands.SetConfigPath(cfgFile)
	}
}

// applyServerContext resolves the backend the CLI should talk to from the
// active context and installs it as viper's default for "server".
//
// It has to be a *default* rather than viper.Set: an explicit --server must
// still win, and viper only consults a bound flag's value ahead of defaults
// when the user actually changed it.
func applyServerContext() {
	if cfgFile != "" {
		commands.SetConfigPath(cfgFile)
	}
	commands.SetContextOverride(contextName)

	config, err := commands.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to load config: %v\n", err)
		return
	}

	url, auth, err := commands.ResolveServer(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	viper.SetDefault("server", url)
	client.SetDefaultAuth(auth)
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

	// Namespace env lookups so "server" reads MYWANT_SERVER rather than the
	// far too generic SERVER.
	viper.SetEnvPrefix("MYWANT")
	viper.AutomaticEnv()

	// Log the config path before reading
	if _, err := os.Stat(configPath); err == nil {
		fmt.Printf("Reading config from: %s\n", configPath)
	}

	if err := viper.ReadInConfig(); err == nil {
		// fmt.Println("Using config file:", viper.ConfigFileUsed())
	}

	applyServerContext()
}
