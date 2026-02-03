package commands

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// MyWantConfig represents the CLI configuration
type MyWantConfig struct {
	AgentMode        string `yaml:"agent_mode"`         // local, webhook, grpc
	ServerHost       string `yaml:"server_host"`        // Main server host
	ServerPort       int    `yaml:"server_port"`        // Main server port
	AgentServiceHost string `yaml:"agent_service_host"` // Agent service host (for webhook mode)
	AgentServicePort int    `yaml:"agent_service_port"` // Agent service port (for webhook mode)
	MockFlightPort   int    `yaml:"mock_flight_port"`   // Mock flight server port
}

// DefaultConfig returns the default configuration
func DefaultConfig() *MyWantConfig {
	return &MyWantConfig{
		AgentMode:        "local",
		ServerHost:       "localhost",
		ServerPort:       8080,
		AgentServiceHost: "localhost",
		AgentServicePort: 8081,
		MockFlightPort:   8090,
	}
}

// configFilePath holds the custom config file path if specified
var configFilePath string

// SetConfigPath sets a custom config file path
func SetConfigPath(path string) {
	configFilePath = path
}

// getConfigPath returns the path to the config file
func getConfigPath() string {
	if configFilePath != "" {
		return configFilePath
	}
	return filepath.Join(getMyWantDir(), "config.yaml")
}

// LoadConfig loads configuration from file or returns default
func LoadConfig() (*MyWantConfig, error) {
	configPath := getConfigPath()

	// If config doesn't exist, return default
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return DefaultConfig(), nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	var config MyWantConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return &config, nil
}

// SaveConfig saves configuration to file
func (c *MyWantConfig) Save() error {
	configPath := getConfigPath()

	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

var ConfigCmd = &cobra.Command{
	Use:     "config",
	Aliases: []string{"cfg"},
	Short:   "Manage MyWant CLI configuration",
	Long:    `Configure agent execution mode, server addresses, ports, and other settings.`,
}

var configSetCmd = &cobra.Command{
	Use:     "set",
	Aliases: []string{"s"},
	Short:   "Set configuration interactively",
	Run: func(cmd *cobra.Command, args []string) {
		reader := bufio.NewReader(os.Stdin)

		// Load current config or use default
		config, err := LoadConfig()
		if err != nil {
			fmt.Printf("Warning: Failed to load config, using defaults: %v\n", err)
			config = DefaultConfig()
		}

		fmt.Println("🔧 MyWant Configuration")
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Println()

		// 1. Agent Mode
		fmt.Printf("Agent Execution Mode [%s]:\n", config.AgentMode)
		fmt.Println("  1) local   - In-process execution (fastest, default)")
		fmt.Println("  2) webhook - External HTTP service (language-agnostic)")
		fmt.Println("  3) grpc    - gRPC service (high-performance)")
		fmt.Print("Select (1-3) or press Enter to keep current: ")

		modeInput, _ := reader.ReadString('\n')
		modeInput = strings.TrimSpace(modeInput)
		if modeInput != "" {
			switch modeInput {
			case "1":
				config.AgentMode = "local"
			case "2":
				config.AgentMode = "webhook"
			case "3":
				config.AgentMode = "grpc"
			default:
				fmt.Println("Invalid selection, keeping current value")
			}
		}
		fmt.Println()

		// 2. Server Host
		fmt.Printf("Main Server Host [%s]: ", config.ServerHost)
		hostInput, _ := reader.ReadString('\n')
		hostInput = strings.TrimSpace(hostInput)
		if hostInput != "" {
			config.ServerHost = hostInput
		}

		// 3. Server Port
		fmt.Printf("Main Server Port [%d]: ", config.ServerPort)
		portInput, _ := reader.ReadString('\n')
		portInput = strings.TrimSpace(portInput)
		if portInput != "" {
			if port, err := strconv.Atoi(portInput); err == nil {
				config.ServerPort = port
			} else {
				fmt.Println("Invalid port, keeping current value")
			}
		}
		fmt.Println()

		// 4. Agent Service settings (only for webhook/grpc mode)
		if config.AgentMode == "webhook" || config.AgentMode == "grpc" {
			fmt.Println("Agent Service Settings (for webhook/grpc mode):")

			fmt.Printf("Agent Service Host [%s]: ", config.AgentServiceHost)
			agentHostInput, _ := reader.ReadString('\n')
			agentHostInput = strings.TrimSpace(agentHostInput)
			if agentHostInput != "" {
				config.AgentServiceHost = agentHostInput
			}

			fmt.Printf("Agent Service Port [%d]: ", config.AgentServicePort)
			agentPortInput, _ := reader.ReadString('\n')
			agentPortInput = strings.TrimSpace(agentPortInput)
			if agentPortInput != "" {
				if port, err := strconv.Atoi(agentPortInput); err == nil {
					config.AgentServicePort = port
				} else {
					fmt.Println("Invalid port, keeping current value")
				}
			}
			fmt.Println()
		}

		// 5. Mock Flight Port
		fmt.Printf("Mock Flight Server Port [%d]: ", config.MockFlightPort)
		mockPortInput, _ := reader.ReadString('\n')
		mockPortInput = strings.TrimSpace(mockPortInput)
		if mockPortInput != "" {
			if port, err := strconv.Atoi(mockPortInput); err == nil {
				config.MockFlightPort = port
			} else {
				fmt.Println("Invalid port, keeping current value")
			}
		}
		fmt.Println()

		// Save configuration
		if err := config.Save(); err != nil {
			fmt.Printf("❌ Failed to save config: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("✅ Configuration saved to", getConfigPath())
		fmt.Println()
		displayConfig(config)
	},
}

var configGetCmd = &cobra.Command{
	Use:     "get",
	Aliases: []string{"g", "show"},
	Short:   "Display current configuration",
	Run: func(cmd *cobra.Command, args []string) {
		config, err := LoadConfig()
		if err != nil {
			fmt.Printf("Error loading config: %v\n", err)
			os.Exit(1)
		}

		displayConfig(config)
	},
}

var configResetCmd = &cobra.Command{
	Use:     "reset",
	Aliases: []string{"r"},
	Short:   "Reset configuration to defaults",
	Run: func(cmd *cobra.Command, args []string) {
		config := DefaultConfig()

		if err := config.Save(); err != nil {
			fmt.Printf("❌ Failed to save config: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("✅ Configuration reset to defaults")
		fmt.Println()
		displayConfig(config)
	},
}

var configEditCmd = &cobra.Command{
	Use:     "edit",
	Aliases: []string{"e"},
	Short:   "Edit configuration file directly",
	Run: func(cmd *cobra.Command, args []string) {
		configPath := getConfigPath()

		// Ensure config exists
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			config := DefaultConfig()
			if err := config.Save(); err != nil {
				fmt.Printf("❌ Failed to create config: %v\n", err)
				os.Exit(1)
			}
		}

		editor := os.Getenv("EDITOR")
		if editor == "" {
			editor = "vim"
		}

		fmt.Printf("Opening %s with %s...\n", configPath, editor)
		// Note: actual editor launching would need exec.Command
		fmt.Printf("Please edit manually: %s\n", configPath)
	},
}

func displayConfig(config *MyWantConfig) {
	fmt.Println("📋 Current Configuration")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("Agent Mode:         %s\n", config.AgentMode)
	fmt.Printf("Server:             %s:%d\n", config.ServerHost, config.ServerPort)

	if config.AgentMode == "webhook" || config.AgentMode == "grpc" {
		fmt.Printf("Agent Service:      %s:%d\n", config.AgentServiceHost, config.AgentServicePort)
	}

	fmt.Printf("Mock Flight Port:   %d\n", config.MockFlightPort)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("Config file: %s\n", getConfigPath())
}

func init() {
	ConfigCmd.AddCommand(configSetCmd)
	ConfigCmd.AddCommand(configGetCmd)
	ConfigCmd.AddCommand(configResetCmd)
	ConfigCmd.AddCommand(configEditCmd)
}
