package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"mywant/client"
	mywant "mywant/engine/core"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var versionJSON bool

// SetVersion hands main's build version (stamped from the git tag via -X, or
// "dev" for an unstamped build) to the engine, so a server started by this
// binary reports its real version on /health — and on the want types bundled
// into it — instead of a placeholder.
func SetVersion(v string) {
	mywant.SetVersion(v)
}

var VersionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show the version of this CLI and of the server it talks to",
	Long: `Print build versions.

Both come from the git tag the binary was built at, never from a value written
by hand, so they can be trusted to identify a build.

The two can differ, and that is the point of showing them together: with a
remote context the server is a different machine running its own build, and an
upgrade that only replaced one of them is worth seeing. The server line is
skipped when no server answers.`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		version, commit := mywant.BuildInfo()
		server := viper.GetString("server")

		health, err := client.NewClient(server).Health()

		if versionJSON {
			out := map[string]any{"client": map[string]string{"version": version, "commit": commit}}
			if err == nil {
				out["server"] = map[string]any{
					"version": health.Version,
					"commit":  health.Commit,
					"url":     server,
				}
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			_ = enc.Encode(out)
			return
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintf(w, "Client:\t%s\n", describeBuild(version, commit))
		if err != nil {
			fmt.Fprintf(w, "Server:\t(unreachable at %s)\n", server)
		} else {
			fmt.Fprintf(w, "Server:\t%s\t%s\n", describeBuild(health.Version, health.Commit), server)
		}
		w.Flush()
	},
}

func describeBuild(version, commit string) string {
	// A version derived from the content digest already identifies the build;
	// appending a commit to it would say the same thing twice.
	if commit == "" || version == commit {
		return version
	}
	return fmt.Sprintf("%s (%s)", version, commit)
}

func init() {
	VersionCmd.Flags().BoolVar(&versionJSON, "json", false, "output as JSON")
}
