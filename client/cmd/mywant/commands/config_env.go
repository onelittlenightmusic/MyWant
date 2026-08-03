package commands

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"
	"text/tabwriter"

	"mywant/client"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/term"
)

// The `environments:` block holds arbitrary env vars that `mywant start` exports
// into the server's process (see MyWantConfig.ApplyEnvironments) — API keys for
// custom types live here. `config set` deliberately takes a fixed key list, so
// these get their own verbs: the names are arbitrary, and the values are usually
// secrets that must never be echoed back in full.

// envKeyPattern is the portable shape of an environment variable name. Anything
// looser (a lowercase name, a dash, a stray `=`) survives the YAML write but
// fails to export, so it is rejected here rather than at startup.
var envKeyPattern = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)

// promptSecret asks for a value at the terminal without echoing it. The value
// is usually an API key, and it would otherwise sit in the scrollback of a
// shared screen long after the command finished.
func promptSecret(key string) (string, error) {
	fmt.Printf("Value for %s (hidden): ", key)
	entered, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(entered)), nil
}

func validateEnvKey(key string) error {
	if key == "" {
		return fmt.Errorf("environment variable name is empty")
	}
	if !envKeyPattern.MatchString(key) {
		return fmt.Errorf("%q is not a usable environment variable name "+
			"(upper case letters, digits and underscore; not starting with a digit)", key)
	}
	return nil
}

var configEnvCmd = &cobra.Command{
	Use:   "env",
	Short: "Manage the environment variables applied to the server at startup",
	Long: `Read and write the ` + "`environments:`" + ` block of ~/.mywant/config.yaml.

These are exported into the server's process by 'mywant start', which is how
custom types reach their API keys (e.g. TICKETMASTER_API_KEY).

  mywant config env list
  mywant config env set TICKETMASTER_API_KEY          # asks for the value, hidden
  mywant config env set TICKETMASTER_API_KEY --stdin < key.txt
  mywant config env unset TICKETMASTER_API_KEY`,
}

var configEnvListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls", "get"},
	Short:   "List the configured environment variables (values masked)",
	Args:    cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		config, err := LoadConfig()
		if err != nil {
			fmt.Printf("❌ Error loading config: %v\n", err)
			os.Exit(1)
		}

		if len(config.Environments) == 0 {
			fmt.Println("No environment variables configured.")
			fmt.Println("Add one with: mywant config env set NAME value")
			return
		}

		show, _ := cmd.Flags().GetBool("show")

		keys := make([]string, 0, len(config.Environments))
		for k := range config.Environments {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "NAME\tVALUE")
		for _, k := range keys {
			value := config.Environments[k]
			if !show {
				value = maskSecret(value)
			}
			fmt.Fprintf(w, "%s\t%s\n", k, value)
		}
		_ = w.Flush()
	},
}

var configEnvSetCmd = &cobra.Command{
	Use:   "set NAME [VALUE]",
	Short: "Set an environment variable applied at server startup",
	Long: `Set one entry of the environments: block.

Leave the value off and it is asked for, hidden, at the prompt — Enter ends it.
That is the way to enter a secret: an argument lands in the shell history.

  mywant config env set TICKETMASTER_API_KEY            # prompts
  mywant config env set TICKETMASTER_API_KEY --stdin < key.txt
  mywant config env set MYWANT_SERVER http://localhost:8080`,
	Args: cobra.RangeArgs(1, 2),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]
		if err := validateEnvKey(key); err != nil {
			fmt.Printf("❌ %v\n", err)
			os.Exit(1)
		}

		fromStdin, _ := cmd.Flags().GetBool("stdin")
		if fromStdin && len(args) == 2 {
			fmt.Println("❌ Give the value as an argument or on stdin, not both")
			os.Exit(1)
		}

		var value string
		switch {
		case len(args) == 2:
			value = args[1]
		case term.IsTerminal(int(os.Stdin.Fd())):
			// At a terminal there is no EOF to wait for — the user presses Enter.
			// Reading to EOF here is what makes a paste look like a hang.
			var err error
			value, err = promptSecret(key)
			if err != nil {
				fmt.Printf("❌ Failed to read the value: %v\n", err)
				os.Exit(1)
			}
		default:
			data, err := io.ReadAll(bufio.NewReader(os.Stdin))
			if err != nil {
				fmt.Printf("❌ Failed to read the value from stdin: %v\n", err)
				os.Exit(1)
			}
			// A key piped from a file or `echo` arrives with a trailing newline
			// that would be exported verbatim and break the header it lands in.
			value = strings.TrimRight(string(data), "\r\n")
			if value == "" {
				fmt.Println("❌ Nothing arrived on stdin. Redirect a file into it, " +
					"or run the command at a terminal to be prompted")
				os.Exit(1)
			}
		}

		if value == "" {
			fmt.Println("❌ The value is empty. Use 'mywant config env unset' to remove an entry")
			os.Exit(1)
		}

		config, err := LoadConfig()
		if err != nil {
			fmt.Printf("❌ Error loading config: %v\n", err)
			os.Exit(1)
		}
		if config.Environments == nil {
			config.Environments = map[string]string{}
		}

		_, existed := config.Environments[key]
		config.Environments[key] = value

		if err := config.Save(); err != nil {
			fmt.Printf("❌ Failed to save config: %v\n", err)
			os.Exit(1)
		}

		verb := "Set"
		if existed {
			verb = "Updated"
		}
		fmt.Printf("✅ %s %s = %s\n", verb, key, maskSecret(value))
		reloadServerEnv()
	},
}

var configEnvUnsetCmd = &cobra.Command{
	Use:     "unset NAME",
	Aliases: []string{"delete", "rm"},
	Short:   "Remove an environment variable",
	Args:    cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]

		config, err := LoadConfig()
		if err != nil {
			fmt.Printf("❌ Error loading config: %v\n", err)
			os.Exit(1)
		}

		if _, ok := config.Environments[key]; !ok {
			fmt.Printf("%s is not set — nothing to do\n", key)
			return
		}

		delete(config.Environments, key)

		if err := config.Save(); err != nil {
			fmt.Printf("❌ Failed to save config: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("✅ Removed %s\n", key)
		reloadServerEnv()
	},
}

// reloadServerEnv hands the change to a running server, the way `custom
// install` hands over a newly installed type. Writing the file is only half the
// job: the point of `config env set` is to change what the server runs with.
//
// A server that is not running needs nothing done — it reads the file at
// startup — so an unreachable one is a note, not an error.
func reloadServerEnv() {
	result, err := client.NewClient(viper.GetString("server")).ReloadConfigEnv()
	if err != nil {
		fmt.Println("   No running server reached — it will be picked up at the next start.")
		return
	}
	if msg, ok := result["message"].(string); ok {
		fmt.Printf("   Server reloaded: %s\n", msg)
	}
	fmt.Println("   Agents pick it up on their next run.")
}

func init() {
	configEnvListCmd.Flags().Bool("show", false, "Print values in full instead of masking them")
	configEnvSetCmd.Flags().Bool("stdin", false, "Read the value from a pipe or file on stdin (at a terminal you are prompted anyway)")

	configEnvCmd.AddCommand(configEnvListCmd)
	configEnvCmd.AddCommand(configEnvSetCmd)
	configEnvCmd.AddCommand(configEnvUnsetCmd)

	ConfigCmd.AddCommand(configEnvCmd)
}
