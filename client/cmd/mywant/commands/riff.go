package commands

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"mywant/client"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// RiffCmd generates absurd wirings between the things you've named and the
// world's toys, and can deploy one straight from its structure.
var RiffCmd = &cobra.Command{
	Use:     "riff",
	Aliases: []string{"rf"},
	Short:   "Generate (and deploy) absurd wirings from your named world",
	Long: `Riff proposes random "XしたらX" wirings built from the things you have
named (aura definitions) and the effect toys the world offers.

  mywant riff            # show a handful of proposals
  mywant riff -n 8       # show more
  mywant riff --deploy 1 # deploy proposal #1's reaction want
  mywant riff -i         # pick one to deploy interactively`,
	Run: func(cmd *cobra.Command, args []string) {
		n, _ := cmd.Flags().GetInt("number")
		deployIdx, _ := cmd.Flags().GetInt("deploy")
		interactive, _ := cmd.Flags().GetBool("interactive")

		c := client.NewClient(viper.GetString("server"))
		proposals, err := c.GetRiffs(n)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		if len(proposals) == 0 {
			fmt.Println("まだriffが作れません。モノに名前をつけるか、effect wantを置いてください。")
			return
		}

		for i, p := range proposals {
			fmt.Printf("  %d. %s\n", i+1, p.Text)
		}

		pick := deployIdx
		if interactive && deployIdx == 0 {
			fmt.Print("\nデプロイする番号 (Enterでキャンセル): ")
			line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
			line = strings.TrimSpace(line)
			if line == "" {
				return
			}
			if v, convErr := strconv.Atoi(line); convErr == nil {
				pick = v
			}
		}
		if pick <= 0 {
			return
		}
		if pick > len(proposals) {
			fmt.Printf("番号は 1〜%d で指定してください\n", len(proposals))
			os.Exit(1)
		}

		chosen := proposals[pick-1]
		res, err := c.DeployRiff(chosen)
		if err != nil {
			fmt.Printf("Error deploying: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("\n✅ デプロイしました: %s\n   want: %v (%v)\n", chosen.Text, res["name"], res["wantId"])
	},
}

func init() {
	RiffCmd.Flags().IntP("number", "n", 5, "How many proposals to generate")
	RiffCmd.Flags().Int("deploy", 0, "Deploy proposal number N straight away")
	RiffCmd.Flags().BoolP("interactive", "i", false, "Pick a proposal to deploy interactively")
}
