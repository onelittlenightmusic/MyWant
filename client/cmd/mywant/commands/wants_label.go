package commands

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"mywant/client"
)

// wantsLabelCmd sets and removes a want's labels, as `things label` does a
// thing's: aliases=golf (what the robot also finds it by), magic-sentence=…
var wantsLabelCmd = &cobra.Command{
	Use:   "label <name-or-id> [key=value]... [--remove key]...",
	Short: "Set and remove a want's labels (aliases=golf, magic-sentence=…)",
	Example: `  mywant wants label smartgolf-available aliases=golf
  mywant wants label smartgolf-available --remove aliases
  mywant --context fly wants label smartgolf-available aliases=golf`,
	Args:              cobra.MinimumNArgs(1),
	ValidArgsFunction: completeWantIDs,
	Run: func(cmd *cobra.Command, args []string) {
		remove, _ := cmd.Flags().GetStringArray("remove")
		set := map[string]string{}
		for _, kv := range args[1:] {
			k, v, ok := strings.Cut(kv, "=")
			if !ok || k == "" {
				fmt.Fprintf(os.Stderr, "Error: write key=value (got %q)\n", kv)
				os.Exit(1)
			}
			set[k] = v
		}
		if len(set) == 0 && len(remove) == 0 {
			fmt.Fprintln(os.Stderr, "Error: say key=value or --remove key")
			os.Exit(1)
		}
		c := client.NewClient(viper.GetString("server"))
		id := resolveRef(c, args[0])
		keys := make([]string, 0, len(set))
		for k := range set {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if err := c.AddWantLabel(id, k, set[k]); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("%s=%s\n", k, set[k])
		}
		for _, k := range remove {
			if err := c.Request("DELETE", "/api/v1/wants/"+id+"/labels/"+k, nil, nil); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("-%s\n", k)
		}
	},
}

func init() {
	wantsLabelCmd.Flags().StringArray("remove", nil, "A label to take off (repeatable)")
	WantsCmd.AddCommand(wantsLabelCmd)
}
