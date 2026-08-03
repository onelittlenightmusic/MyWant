package commands

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"mywant/client"

	"github.com/spf13/cobra"
)

// DefaultServerURL is the destination used when nothing else is configured.
const DefaultServerURL = "http://localhost:8080"

// ServerContext is one named backend the CLI can talk to, kubectl-style.
// Contexts live under `contexts:` in ~/.mywant/config.yaml and the active one
// is named by `current_context:`, so switching backends is a config edit
// (or `mywant config use-context <name>`) rather than a flag on every command.
type ServerContext struct {
	Server   string `yaml:"server"`             // full URL, e.g. https://osaki-mywant-gui.fly.dev
	Username string `yaml:"username,omitempty"` // Basic auth user
	Password string `yaml:"password,omitempty"` // Basic auth password (plaintext — prefer PasswordEnv)
	// PasswordEnv names an environment variable holding the password, so the
	// secret stays out of config.yaml. It wins over Password when both are set
	// and the variable is non-empty.
	PasswordEnv string `yaml:"password_env,omitempty"`
	Token       string `yaml:"token,omitempty"`     // Bearer token (wins over Basic auth)
	TokenEnv    string `yaml:"token_env,omitempty"` // env var holding the Bearer token
}

// Credentials resolves the context's auth, consulting the *_env indirections.
func (sc ServerContext) Credentials() client.Auth {
	auth := client.Auth{Username: sc.Username, Password: sc.Password, Token: sc.Token}
	if sc.PasswordEnv != "" {
		if v := os.Getenv(sc.PasswordEnv); v != "" {
			auth.Password = v
		}
	}
	if sc.TokenEnv != "" {
		if v := os.Getenv(sc.TokenEnv); v != "" {
			auth.Token = v
		}
	}
	return auth
}

// contextOverride is set from the --context persistent flag and selects a
// context for a single invocation without touching config.yaml.
var contextOverride string

// SetContextOverride records the --context flag value.
func SetContextOverride(name string) {
	contextOverride = name
}

// ResolveServer determines which backend to talk to and with what credentials.
//
// Precedence, highest first:
//  1. MYWANT_SERVER / MYWANT_TOKEN / MYWANT_USERNAME / MYWANT_PASSWORD env vars
//  2. the context named by --context, else by current_context
//  3. the legacy server_host + server_port pair
//  4. DefaultServerURL
//
// The --server flag sits above all of this, but it is handled by viper (a
// changed flag outranks the default this function feeds it).
func ResolveServer(config *MyWantConfig) (string, client.Auth, error) {
	url := ""
	auth := client.Auth{}

	if config != nil {
		name := contextOverride
		if name == "" {
			name = config.CurrentContext
		}
		if name != "" {
			ctx, ok := config.Contexts[name]
			if !ok {
				return "", auth, fmt.Errorf("context %q not found in %s (available: %s)",
					name, getConfigPath(), strings.Join(contextNames(config), ", "))
			}
			url = ctx.Server
			auth = ctx.Credentials()
		}

		// Fall back to the host/port pair the config has always carried. It
		// cannot express a scheme, so assume plain HTTP — a TLS endpoint has
		// to be configured as a context.
		if url == "" && config.ServerHost != "" && config.ServerPort > 0 {
			url = fmt.Sprintf("http://%s:%d", config.ServerHost, config.ServerPort)
		}
	}

	if url == "" {
		url = DefaultServerURL
	}

	// Env vars override whatever the config said.
	if v := os.Getenv("MYWANT_SERVER"); v != "" {
		url = v
	}
	if v := os.Getenv("MYWANT_TOKEN"); v != "" {
		auth.Token = v
	}
	if v := os.Getenv("MYWANT_USERNAME"); v != "" {
		auth.Username = v
	}
	if v := os.Getenv("MYWANT_PASSWORD"); v != "" {
		auth.Password = v
	}

	return strings.TrimSuffix(url, "/"), auth, nil
}

// ActiveContextName reports the context in effect, or "" when none is set.
func ActiveContextName(config *MyWantConfig) string {
	if contextOverride != "" {
		return contextOverride
	}
	if config == nil {
		return ""
	}
	return config.CurrentContext
}

// IsContextName reports whether name is a context in the config file at the
// *default* path, ignoring any --config override. It exists so that a
// `--config fly` mix-up can be answered with "did you mean --context fly?".
func IsContextName(name string) bool {
	saved := configFilePath
	configFilePath = ""
	defer func() { configFilePath = saved }()

	config, err := LoadConfig()
	if err != nil || config == nil {
		return false
	}
	_, ok := config.Contexts[name]
	return ok
}

func contextNames(config *MyWantConfig) []string {
	names := make([]string, 0, len(config.Contexts))
	for name := range config.Contexts {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

var configGetContextsCmd = &cobra.Command{
	Use:     "get-contexts",
	Aliases: []string{"contexts"},
	Short:   "List the configured backends",
	Run: func(cmd *cobra.Command, args []string) {
		config, err := LoadConfig()
		if err != nil {
			fmt.Printf("Error loading config: %v\n", err)
			os.Exit(1)
		}

		if len(config.Contexts) == 0 {
			fmt.Printf("No contexts defined in %s\n", getConfigPath())
			fmt.Println()
			fmt.Println("Add one with:")
			fmt.Println("  mywant config set-context fly --server https://example.fly.dev --username admin --password-env MYWANT_AUTH_PASSWORD")
			return
		}

		active := ActiveContextName(config)
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "CURRENT\tNAME\tSERVER\tAUTH")
		for _, name := range contextNames(config) {
			ctx := config.Contexts[name]
			marker := ""
			if name == active {
				marker = "*"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", marker, name, ctx.Server, describeAuth(ctx))
		}
		w.Flush()
	},
}

// describeAuth summarises a context's credentials without printing secrets.
func describeAuth(ctx ServerContext) string {
	switch {
	case ctx.TokenEnv != "":
		return "bearer ($" + ctx.TokenEnv + ")"
	case ctx.Token != "":
		return "bearer"
	case ctx.PasswordEnv != "":
		return fmt.Sprintf("basic %s ($%s)", ctx.Username, ctx.PasswordEnv)
	case ctx.Username != "" || ctx.Password != "":
		return "basic " + ctx.Username
	default:
		return "none"
	}
}

var configCurrentContextCmd = &cobra.Command{
	Use:   "current-context",
	Short: "Show the context currently in use",
	Run: func(cmd *cobra.Command, args []string) {
		config, err := LoadConfig()
		if err != nil {
			fmt.Printf("Error loading config: %v\n", err)
			os.Exit(1)
		}

		name := ActiveContextName(config)
		url, _, err := ResolveServer(config)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		if name == "" {
			fmt.Printf("(no context set) → %s\n", url)
			return
		}
		fmt.Printf("%s → %s\n", name, url)
	},
}

var configUseContextCmd = &cobra.Command{
	Use:               "use-context <name>",
	Short:             "Switch the CLI to a different backend",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeContextNames,
	Run: func(cmd *cobra.Command, args []string) {
		config, err := LoadConfig()
		if err != nil {
			fmt.Printf("Error loading config: %v\n", err)
			os.Exit(1)
		}

		name := args[0]
		ctx, ok := config.Contexts[name]
		if !ok {
			fmt.Printf("Error: context %q not found (available: %s)\n", name, strings.Join(contextNames(config), ", "))
			os.Exit(1)
		}

		config.CurrentContext = name
		if err := config.Save(); err != nil {
			fmt.Printf("Error saving config: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("✅ switched to context %q → %s\n", name, ctx.Server)
	},
}

var configSetContextCmd = &cobra.Command{
	Use:   "set-context <name>",
	Short: "Create or update a backend context",
	Long: `Create or update a named backend in config.yaml.

Only the flags you pass are modified; the rest of an existing context is kept.

Examples:
  mywant config set-context local --server http://localhost:8080
  mywant config set-context fly --server https://osaki-mywant-gui.fly.dev \
      --username admin --password-env MYWANT_AUTH_PASSWORD
  mywant config set-context fly --use`,
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeContextNames,
	Run: func(cmd *cobra.Command, args []string) {
		config, err := LoadConfig()
		if err != nil {
			fmt.Printf("Error loading config: %v\n", err)
			os.Exit(1)
		}

		name := args[0]
		if config.Contexts == nil {
			config.Contexts = map[string]ServerContext{}
		}
		ctx := config.Contexts[name]

		if cmd.Flags().Changed("server") {
			// --server is a persistent flag on the root command, so read it
			// from there rather than from this command's own flag set.
			v, _ := cmd.Flags().GetString("server")
			ctx.Server = strings.TrimSuffix(v, "/")
		}
		if cmd.Flags().Changed("username") {
			ctx.Username, _ = cmd.Flags().GetString("username")
		}
		if cmd.Flags().Changed("password") {
			ctx.Password, _ = cmd.Flags().GetString("password")
			ctx.PasswordEnv = ""
		}
		if cmd.Flags().Changed("password-env") {
			ctx.PasswordEnv, _ = cmd.Flags().GetString("password-env")
			ctx.Password = ""
		}
		if cmd.Flags().Changed("token") {
			ctx.Token, _ = cmd.Flags().GetString("token")
			ctx.TokenEnv = ""
		}
		if cmd.Flags().Changed("token-env") {
			ctx.TokenEnv, _ = cmd.Flags().GetString("token-env")
			ctx.Token = ""
		}

		if ctx.Server == "" {
			fmt.Printf("Error: context %q has no server URL; pass --server\n", name)
			os.Exit(1)
		}

		config.Contexts[name] = ctx

		use, _ := cmd.Flags().GetBool("use")
		if use || config.CurrentContext == "" {
			config.CurrentContext = name
		}

		if err := config.Save(); err != nil {
			fmt.Printf("Error saving config: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("✅ context %q → %s (%s)\n", name, ctx.Server, describeAuth(ctx))
		if config.CurrentContext == name {
			fmt.Println("   now the current context")
		}
	},
}

var configDeleteContextCmd = &cobra.Command{
	Use:               "delete-context <name>",
	Short:             "Remove a backend context",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeContextNames,
	Run: func(cmd *cobra.Command, args []string) {
		config, err := LoadConfig()
		if err != nil {
			fmt.Printf("Error loading config: %v\n", err)
			os.Exit(1)
		}

		name := args[0]
		if _, ok := config.Contexts[name]; !ok {
			fmt.Printf("Error: context %q not found\n", name)
			os.Exit(1)
		}

		delete(config.Contexts, name)
		if config.CurrentContext == name {
			config.CurrentContext = ""
			fmt.Printf("⚠️  %q was the current context; falling back to server_host/server_port\n", name)
		}

		if err := config.Save(); err != nil {
			fmt.Printf("Error saving config: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("✅ deleted context %q\n", name)
	},
}

// completeContextNames provides shell completion for context arguments.
func completeContextNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	config, err := LoadConfig()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return contextNames(config), cobra.ShellCompDirectiveNoFileComp
}

func init() {
	configSetContextCmd.Flags().String("username", "", "Basic auth username")
	configSetContextCmd.Flags().String("password", "", "Basic auth password (stored in plaintext; prefer --password-env)")
	configSetContextCmd.Flags().String("password-env", "", "Name of an env var holding the Basic auth password")
	configSetContextCmd.Flags().String("token", "", "Bearer token (stored in plaintext; prefer --token-env)")
	configSetContextCmd.Flags().String("token-env", "", "Name of an env var holding the Bearer token")
	configSetContextCmd.Flags().Bool("use", false, "Also switch to this context")

	ConfigCmd.AddCommand(configGetContextsCmd)
	ConfigCmd.AddCommand(configCurrentContextCmd)
	ConfigCmd.AddCommand(configUseContextCmd)
	ConfigCmd.AddCommand(configSetContextCmd)
	ConfigCmd.AddCommand(configDeleteContextCmd)
}
