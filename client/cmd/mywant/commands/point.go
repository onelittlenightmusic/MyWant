package commands

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Point at anything on the board, by name.
//
// `thing point` and `wants point` each need the asker to know which of the two
// they are naming, and that is knowledge about how the board is built rather
// than about what is on it. Somebody looking at it — a person, or something
// answering for one — says "spotify-instance" or "新宿" and means "that, there".
// Asked with the wrong family the answer was "no such thing", about something
// plainly on the board.
//
// So this looks in both: things first (a named value is what most names are),
// then wants. Each of the two keeps its own command for when the family is
// known and the distinction matters.
var PointCmd = &cobra.Command{
	Use:     "point <name>",
	Short:   "Where anything on the board is: say the cell AND send the robot there",
	Aliases: []string{"show-me", "where-is"},
	Long: `Finds a thing or a want by name and points the robot at it.

Looks for a thing first, then a want. Either way the robot stands on the cell
it is naming: a pointer that is one cell over is pointing at the wrong thing.

Nothing is changed except where the robot is standing and what it is saying.`,
	Example: `  mywant point 新宿
  mywant point weather-instance
  mywant point weather --say "天気はこれです"`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		// Run the family commands themselves rather than reimplementing them:
		// one place decides what "point at a thing" means, and this is not it.
		if _, err := findThingPlace(memoClient(), args[0]); err == nil {
			thingPointCmd.Run(cmd, args)
			return
		}
		if _, err := findWantPlace(wantsClient(), args[0]); err == nil {
			wantsPointCmd.Run(cmd, args)
			return
		}
		fmt.Fprintf(os.Stderr, "Error: nothing on the board is named %q — neither a thing nor a want\n", args[0])
		os.Exit(1)
	},
}

func init() {
	PointCmd.Flags().String("say", "", "What the robot says when it gets there (default: the name and its cell)")
}
