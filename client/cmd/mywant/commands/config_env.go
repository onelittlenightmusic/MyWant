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

	"github.com/spf13/cobra"
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
  mywant config env set TICKETMASTER_API_KEY <key>
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

The value can be given as an argument or read from stdin with --stdin, which
keeps a secret out of the shell history:

  mywant config env set TICKETMASTER_API_KEY --stdin < key.txt`,
	Args: cobra.RangeArgs(1, 2),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]
		if err := validateEnvKey(key); err != nil {
			fmt.Printf("❌ %v\n", err)
			os.Exit(1)
		}

		fromStdin, _ := cmd.Flags().GetBool("stdin")

		var value string
		switch {
		case fromStdin && len(args) == 2:
			fmt.Println("❌ Give the value as an argument or with --stdin, not both")
			os.Exit(1)
		case fromStdin:
			data, err := io.ReadAll(bufio.NewReader(os.Stdin))
			if err != nil {
				fmt.Printf("❌ Failed to read the value from stdin: %v\n", err)
				os.Exit(1)
			}
			// A key piped from a file or `echo` arrives with a trailing newline
			// that would be exported verbatim and break the header it lands in.
			value = strings.TrimRight(string(data), "\r\n")
		case len(args) == 2:
			value = args[1]
		default:
			fmt.Println("❌ No value given. Pass it as an argument or use --stdin")
			os.Exit(1)
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
		fmt.Println("   Restart the server to apply it: mywant stop && mywant start -D")
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
		fmt.Println("   Restart the server to apply it: mywant stop && mywant start -D")
	},
}

func init() {
	configEnvListCmd.Flags().Bool("show", false, "Print values in full instead of masking them")
	configEnvSetCmd.Flags().Bool("stdin", false, "Read the value from stdin instead of an argument")

	configEnvCmd.AddCommand(configEnvListCmd)
	configEnvCmd.AddCommand(configEnvSetCmd)
	configEnvCmd.AddCommand(configEnvUnsetCmd)

	ConfigCmd.AddCommand(configEnvCmd)
}
