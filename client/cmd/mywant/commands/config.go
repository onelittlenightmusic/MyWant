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

// GoalThinkerSettings holds goal_thinker configuration from config.yaml.
type GoalThinkerSettings struct {
	UseStub bool `yaml:"use_stub"` // If true, call stub Python script instead of LLM
}

// MyWantConfig represents the CLI configuration
type MyWantConfig struct {
	AgentMode        string              `yaml:"agent_mode"`         // local, webhook, grpc
	ServerHost       string              `yaml:"server_host"`        // Main server host
	ServerPort       int                 `yaml:"server_port"`        // Main server port
	GUIHost          string              `yaml:"gui_host"`           // mywant-gui host
	GUIPort          int                 `yaml:"gui_port"`           // mywant-gui port
	AgentServiceHost string              `yaml:"agent_service_host"` // Agent service host (for webhook mode)
	AgentServicePort int                 `yaml:"agent_service_port"` // Agent service port (for webhook mode)
	MockFlightPort   int                 `yaml:"mock_flight_port"`   // Mock flight server port
	HeaderPosition   string              `yaml:"header_position"`    // top or bottom
	ColorMode        string              `yaml:"color_mode"`         // light, dark, system
	Environments     map[string]string   `yaml:"environments"`       // arbitrary env vars applied at startup
	OTELEndpoint     string              `yaml:"otel_endpoint"`      // OTLP/gRPC endpoint (e.g. "localhost:4317"). Falls back to OTEL_EXPORTER_OTLP_ENDPOINT env var.
	GoalThinker      GoalThinkerSettings `yaml:"goal_thinker"`       // Goal thinker settings
}

// ApplyEnvironments sets entries from the environments section as environment variables.
// Existing env vars (e.g. set in shell) always take precedence.
func (c *MyWantConfig) ApplyEnvironments() {
	for k, v := range c.Environments {
		if v != "" && os.Getenv(k) == "" {
			os.Setenv(k, v)
		}
	}
}

// DefaultConfig returns the default configuration
func DefaultConfig() *MyWantConfig {
	return &MyWantConfig{
		AgentMode:        "local",
		ServerHost:       "localhost",
		ServerPort:       8080,
		GUIHost:          "localhost",
		GUIPort:          8081,
		AgentServiceHost: "localhost",
		AgentServicePort: 8082,
		MockFlightPort:   8090,
		HeaderPosition:   "top",
		ColorMode:        "system",
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
	config := DefaultConfig()

	// If config doesn't exist, return default
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return config, nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	// Unmarshal into the default config to preserve defaults for missing fields
	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return config, nil
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
	Use:     "set [key value]",
	Aliases: []string{"s"},
	Short:   "Set configuration value (or interactively if no args given)",
	Long: `Set a single config key directly:
  mywant config set otel_endpoint localhost:4317
  mywant config set server_port 9090

Or run without arguments for interactive setup.

Valid keys: agent_mode, server_host, server_port, agent_service_host, agent_service_port, mock_flight_port, header_position, color_mode, otel_endpoint`,
	Run: func(cmd *cobra.Command, args []string) {
		// Non-interactive: config set <key> <value>
		if len(args) == 2 {
			config, err := LoadConfig()
			if err != nil {
				fmt.Printf("Warning: Failed to load config, using defaults: %v\n", err)
				config = DefaultConfig()
			}
			key, value := args[0], args[1]
			if err := applyConfigKey(config, key, value); err != nil {
				fmt.Printf("Error: %v\n", err)
				os.Exit(1)
			}
			if err := config.Save(); err != nil {
				fmt.Printf("Error saving config: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("✅ %s = %s\n", key, value)
			return
		}

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

		// 6. Header Position
		fmt.Printf("Header Position [%s]:\n", config.HeaderPosition)
		fmt.Println("  1) top    - Display header at the top (default)")
		fmt.Println("  2) bottom - Display header at the bottom (better for mobile)")
		fmt.Print("Select (1-2) or press Enter to keep current: ")

		posInput, _ := reader.ReadString('\n')
		posInput = strings.TrimSpace(posInput)
		if posInput != "" {
			switch posInput {
			case "1":
				config.HeaderPosition = "top"
			case "2":
				config.HeaderPosition = "bottom"
			default:
				fmt.Println("Invalid selection, keeping current value")
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
	fmt.Printf("Header Position:    %s\n", config.HeaderPosition)

	if len(config.Environments) > 0 {
		fmt.Println()
		fmt.Println("Environments:")
		for k, v := range config.Environments {
			masked := maskSecret(v)
			fmt.Printf("  %s = %s\n", k, masked)
		}
	}

	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("Config file: %s\n", getConfigPath())
}

// applyConfigKey sets a single config field by name.
func applyConfigKey(config *MyWantConfig, key, value string) error {
	switch key {
	case "agent_mode":
		config.AgentMode = value
	case "server_host":
		config.ServerHost = value
	case "server_port":
		port, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("server_port must be an integer")
		}
		config.ServerPort = port
	case "agent_service_host":
		config.AgentServiceHost = value
	case "agent_service_port":
		port, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("agent_service_port must be an integer")
		}
		config.AgentServicePort = port
	case "mock_flight_port":
		port, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("mock_flight_port must be an integer")
		}
		config.MockFlightPort = port
	case "header_position":
		config.HeaderPosition = value
	case "color_mode":
		config.ColorMode = value
	case "otel_endpoint":
		config.OTELEndpoint = value
	default:
		return fmt.Errorf("unknown config key %q. Valid keys: agent_mode, server_host, server_port, agent_service_host, agent_service_port, mock_flight_port, header_position, color_mode, otel_endpoint", key)
	}
	return nil
}

// maskSecret masks all but the first 4 characters of a secret value
func maskSecret(v string) string {
	if len(v) <= 4 {
		return "****"
	}
	return v[:4] + strings.Repeat("*", len(v)-4)
}

func init() {
	ConfigCmd.AddCommand(configSetCmd)
	ConfigCmd.AddCommand(configGetCmd)
	ConfigCmd.AddCommand(configResetCmd)
	ConfigCmd.AddCommand(configEditCmd)
}
