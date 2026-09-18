package commands

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"mywant/client"
)

// Where a want is on the board, and the robot walking over to show you.
//
// The thing side of this (thing_where.go) came first and the question turned
// out to be the same one: somebody looking at a board of forty tiles asks where
// one of them is, and a cell named in words is only a place once you have found
// it. A want keeps its position the way a thing does — mywant.io/canvas-x / -y
// on its own metadata — so the answer is the same shape.
//
//	mywant wants where weather-instance   the cell, in words
//	mywant wants point weather            the robot goes and stands beside it

// wantPlace is where one want stands, and how it is doing.
//
// The status rides along for the same reason a thing's constellations do (see
// thingPlace): the command that gets called is a guess, and an answer holding
// only a cell sends whatever asked back for a second call to learn the rest.
type wantPlace struct {
	id, name, wantType string
	status             string
	x, y               int
	placed             bool
}

// describe is the want in one clause: what type it is, and how it is doing.
func (p wantPlace) describe() string {
	out := fmt.Sprintf("%s (%s)", p.name, p.wantType)
	if p.status != "" {
		out += fmt.Sprintf(", status %s", p.status)
	}
	return out
}

// findWantPlace matches a want the way a person names one: its own name first,
// then its id, then its type — "weather" finds the weather want when there is
// only one, and says so when there are several.
func findWantPlace(c *client.Client, name string) (wantPlace, error) {
	resp, err := c.ListWants("", nil, nil, false, true)
	if err != nil {
		return wantPlace{}, err
	}
	needle := bareThingName(name)
	if needle == "" {
		return wantPlace{}, fmt.Errorf("no want named %q", strings.TrimSpace(name))
	}
	// Compared loosely — see looseName: "transit search", transit_search and
	// transit-search-instance are one name said three ways.
	looseNeedle := looseName(needle)

	place := func(w *client.Want) wantPlace {
		x, errX := strconv.Atoi(w.Metadata.Labels[canvasXLabel])
		y, errY := strconv.Atoi(w.Metadata.Labels[canvasYLabel])
		return wantPlace{
			id: w.Metadata.ID, name: w.Metadata.Name, wantType: w.Metadata.Type,
			status: w.Status, x: x, y: y, placed: errX == nil && errY == nil,
		}
	}

	var byType, partial []wantPlace
	for _, w := range resp.Wants {
		if idMatches(w.Metadata.ID, name) || idMatches(w.Metadata.ID, needle) || looseName(w.Metadata.Name) == looseNeedle {
			return place(w), nil
		}
		if looseName(w.Metadata.Type) == looseNeedle {
			byType = append(byType, place(w))
			continue
		}
		if strings.Contains(looseName(w.Metadata.Name), looseNeedle) {
			partial = append(partial, place(w))
		}
	}

	candidates := byType
	if len(candidates) == 0 {
		candidates = partial
	}
	switch len(candidates) {
	case 0:
		return wantPlace{}, fmt.Errorf("no want named %q", needle)
	case 1:
		return candidates[0], nil
	default:
		names := make([]string, 0, len(candidates))
		for _, c := range candidates {
			names = append(names, c.name)
		}
		return wantPlace{}, fmt.Errorf("%q matches several wants: %s", needle, strings.Join(names, ", "))
	}
}

var wantsWhereCmd = &cobra.Command{
	Use:     "where <want>",
	Short:   "Say where a want is, without moving the robot",
	Aliases: []string{"w"},
	Long: `Prints the cell a want's tile stands on, by name, id or type.

One line, in words, so anything answering questions can repeat it as it is.`,
	Example: `  mywant wants where weather-instance
  mywant wants where weather`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		c := client.NewClient(viper.GetString("server"))
		place, err := findWantPlace(c, args[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if !place.placed {
			fmt.Printf("%s has no place on the canvas yet. | id: %s\n", place.describe(), shortID(place.id))
			return
		}
		fmt.Printf("%s is on the canvas at (%d, %d). | id: %s\n", place.describe(), place.x, place.y, shortID(place.id))
	},
}

var wantsPointCmd = &cobra.Command{
	Use:     "point <want>",
	Short:   "Where a want is: say the cell AND send the robot to stand on it",
	Aliases: []string{"show-me"},
	Long: `Moves the robot's tile onto the want's cell and gives it a line to say.

Onto, not beside. Standing one cell over kept the want's tile visible, and made
the answer wrong in the only way that matters: the robot says (4, 4) while
standing at (5, 4), and what you follow is the robot, not the sentence. A
pointer that is off by one is pointing at the wrong thing.

Nothing is changed except where the robot is standing and what it is saying.`,
	Example: `  mywant wants point weather
  mywant wants point weather-instance --say "天気はこれです"`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		c := client.NewClient(viper.GetString("server"))
		place, err := findWantPlace(c, args[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if !place.placed {
			fmt.Printf("%s has no place on the canvas, so there is nowhere to point.\n", place.describe())
			return
		}
		if place.id == robotWantName || place.name == robotWantName {
			fmt.Println("That is the robot itself — it is already there.")
			return
		}

		for key, value := range map[string]string{
			canvasXLabel: strconv.Itoa(place.x),
			canvasYLabel: strconv.Itoa(place.y),
		} {
			if err := c.AddWantLabel(robotWantName, key, value); err != nil {
				fmt.Fprintf(os.Stderr, "Error moving the robot: %v\n", err)
				os.Exit(1)
			}
		}

		words, _ := cmd.Flags().GetString("say")
		if words == "" {
			words = fmt.Sprintf("「%s」はここです (%d, %d)", place.name, place.x, place.y)
		}
		if err := c.SetWantState(robotWantName, map[string]any{"say": words}); err != nil {
			fmt.Fprintf(os.Stderr, "Error: the robot got there but could not speak: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("The robot is standing on %s at (%d, %d) and saying: %s | id: %s\n",
			place.describe(), place.x, place.y, words, shortID(place.id))
	},
}

// wantsClient is the API client the want-side lookups use — the same one
// everything else here builds, named once so `point` can borrow it.
func wantsClient() *client.Client {
	return client.NewClient(viper.GetString("server"))
}

func init() {
	wantsPointCmd.Flags().String("say", "", "What the robot says when it gets there (default: the want and its cell)")
	WantsCmd.AddCommand(wantsWhereCmd)
	WantsCmd.AddCommand(wantsPointCmd)
}
