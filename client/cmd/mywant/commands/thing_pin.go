package commands

import (
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/cobra"
)

// Putting a thing on the canvas, and taking it off again.
//
// A thing stands on the board because three labels say so: mywant.io/canvas is
// "true", and canvas-x / canvas-y are the cell. That is how the sidebar pins
// one, and until now it was the only way — from a terminal you would write the
// three labels yourself, by hand, through `thing label 駅::荻窪 mywant.io/canvas-x 5`,
// three times, in the catalog::value form nobody says out loud.
//
// So "荻窪を (5, 0) に置いて" was not a sentence this CLI could hear, which
// means it was not a sentence the robot could act on either. One verb per
// gesture, taking a name the way every other lookup takes one:
//
//	mywant thing pin 荻窪 5 0
//	mywant thing unpin 荻窪
var thingPinCmd = &cobra.Command{
	Use:   "pin <thing> <x> <y>",
	Short: "Put a thing on the canvas at a cell (or move one that is already there)",
	Long: `Places a thing's tile on the canvas at the given grid cell.

The thing is found the way every other command finds one — by name, loosely, or
by the id the board prints. A thing already on the canvas is moved.`,
	Example: `  mywant thing pin 荻窪 5 0
  mywant thing pin thg-5f4162d0 5 0`,
	Args: cobra.ExactArgs(3),
	Run: func(cmd *cobra.Command, args []string) {
		x, err := strconv.Atoi(args[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: x must be a whole number, got %q\n", args[1])
			os.Exit(1)
		}
		y, err := strconv.Atoi(args[2])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: y must be a whole number, got %q\n", args[2])
			os.Exit(1)
		}

		c := memoClient()
		place, err := findThingPlace(c, args[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		for key, value := range map[string]string{
			canvasOnLabel: "true",
			canvasXLabel:  strconv.Itoa(x),
			canvasYLabel:  strconv.Itoa(y),
		} {
			if err := c.SetThingLabel(place.id, key, value); err != nil {
				fmt.Fprintf(os.Stderr, "Error pinning %s: %v\n", place.value, err)
				os.Exit(1)
			}
		}
		// Where it was standing, while that is still known: after the write,
		// nobody can say where it came from, which is exactly what undoing it
		// needs. A thing that was not on the board goes back to not being on it.
		if place.onCanvas {
			recordUndo(fmt.Sprintf("moved %s to (%d, %d)", place.value, x, y),
				"thing", "pin", place.id, strconv.Itoa(place.x), strconv.Itoa(place.y))
		} else {
			recordUndo(fmt.Sprintf("put %s on the canvas at (%d, %d)", place.value, x, y),
				"thing", "unpin", place.id)
		}

		moved := "is now on the canvas at"
		if place.onCanvas {
			moved = "moved to"
		}
		fmt.Printf("%s %s (%d, %d). | id: %s\n", place.describe(), moved, x, y, shortID(place.id))
	},
}

var thingUnpinCmd = &cobra.Command{
	Use:     "unpin <thing>",
	Short:   "Take a thing off the canvas (the thing itself is kept)",
	Aliases: []string{"unplace"},
	Long: `Removes a thing's tile from the canvas.

Nothing is forgotten: the thing, its labels, its constellations and the wants
that name it are all untouched. Only its place on the board goes, and
` + "`thing pin`" + ` puts it back.`,
	Example: `  mywant thing unpin 荻窪`,
	Args:    cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		c := memoClient()
		place, err := findThingPlace(c, args[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if !place.onCanvas {
			fmt.Printf("%s is not on the canvas.\n", place.describe())
			return
		}
		for _, key := range []string{canvasOnLabel, canvasXLabel, canvasYLabel} {
			if err := c.RemoveThingLabel(place.id, key); err != nil {
				fmt.Fprintf(os.Stderr, "Error unpinning %s: %v\n", place.value, err)
				os.Exit(1)
			}
		}
		recordUndo(fmt.Sprintf("took %s off the canvas", place.value),
			"thing", "pin", place.id, strconv.Itoa(place.x), strconv.Itoa(place.y))
		fmt.Printf("%s is off the canvas. It is still remembered; `thing pin` puts it back. | id: %s\n",
			place.describe(), shortID(place.id))
	},
}

func init() {
	// The board has cells left of and above the origin — Kokubunji stands at
	// (-3, 0) — so a coordinate can begin with a minus, and a parser that reads
	// flags anywhere sees `-3` as a bundle of short flags and refuses: "unknown
	// shorthand flag: '3' in -3". Stopping at the first positional argument
	// makes everything after the thing's name what it plainly is. Flags still
	// work before it (`mywant --context fly thing pin …`).
	thingPinCmd.Flags().SetInterspersed(false)
	ThingCmd.AddCommand(thingPinCmd)
	ThingCmd.AddCommand(thingUnpinCmd)
}
