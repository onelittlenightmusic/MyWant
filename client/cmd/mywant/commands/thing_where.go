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

// Where a thing is on the board, and the robot walking over to show you.
//
// A thing's place is kept the way every tile's place is kept: as labels on the
// thing itself (mywant.io/canvas-x / -y, and mywant.io/canvas for whether it is
// out there at all). Reading that took either the HTTP API or a squint at
// `thing labels`, which is fine for a person with a browser open and no use at
// all to anything answering the question in words — "新宿というthingの場所は？"
// has an answer, and until now nothing could say it.
//
//	mywant thing where 新宿     the cell, in words
//	mywant thing point 新宿     the robot goes and stands there, and says so

const (
	thingCanvasLabel  = "mywant.io/canvas"
	thingCanvasXLabel = "mywant.io/canvas-x"
	thingCanvasYLabel = "mywant.io/canvas-y"
	// The want the robot is. Its tile is what moves when it points at
	// something; see robot_types.go, where the same labels carry its position.
	robotWantName = "robot"
)

// thingPlace is where one thing stands, as far as the board is concerned, and
// what else the board knows about it.
//
// More than a cell, because which command gets called is a guess made by
// whatever is answering: asked "荻窪はどの星座？" it reaches for `point` about
// as often as for `relations`, and an answer that is only coordinates is then
// the wrong answer to a question the board could have answered. Every lookup
// carrying the constellations costs a line of output and makes both guesses
// right.
type thingPlace struct {
	id             string
	value          string
	kind           string
	onCanvas       bool
	x, y           int
	constellations []string
}

// describe is the thing in one clause: what it is, and which constellations it
// is in.
func (p thingPlace) describe() string {
	out := fmt.Sprintf("%s (%s)", p.value, p.kind)
	if len(p.constellations) > 0 {
		out += fmt.Sprintf(" in constellation (星座) %s", strings.Join(p.constellations, ", "))
	}
	return out
}

// thingNameDecorations are what a name arrives wrapped in when it was quoted in
// a sentence rather than typed as an argument: brackets, quotes, and the
// particles and words that ride along with it.
//
// This matters because the asker is often not a person at a prompt. An agent
// relaying "「新宿」というthingの場所は？" passes 「新宿」 or 新宿は, and a
// lookup that insists on the bare name answers "no thing named that" about a
// thing sitting on the board. The name is what is left once the wrapping comes
// off, and taking it off here is cheaper than teaching every caller not to put
// it on.
var thingNameDecorations = []string{
	"「", "」", "『", "』", `"`, "'", "“", "”", "the ", "The ",
}

// thingNameSuffixes come off the end only: the words a name is followed by when
// it was said in a sentence rather than typed as an argument.
//
// "spotify-instanceの場所" and "新宿はどこ" are how the name arrives when an
// agent passes along what it was asked, and a lookup that insists on the bare
// name answers "no such thing" about something sitting on the board. Longest
// first, so 場所 comes off before は does.
var thingNameSuffixes = []string{
	"ですか", "でしょうか", "という", "どこ", "場所", "位置", "です", "thing", "Thing", "want", "Want",
	"は", "が", "の", "を", "って", "とは", "?", "？",
}

// bareThingName strips that wrapping. It never touches the middle of a name, so
// a thing genuinely called 中野坂上 or "Web Player (Chrome)" is unchanged.
func bareThingName(name string) string {
	out := strings.TrimSpace(name)
	for _, d := range thingNameDecorations {
		out = strings.ReplaceAll(out, d, "")
	}
	out = strings.TrimSpace(out)
	// Suffixes come off one at a time, and only while something is left:
	// "新宿というthing" loses both, "は" alone stays a name. The list is written
	// longest-first so 場所 is taken before は.
	for changed := true; changed && len([]rune(out)) > 1; {
		changed = false
		for _, suffix := range thingNameSuffixes {
			if trimmed := strings.TrimSuffix(out, suffix); trimmed != out && strings.TrimSpace(trimmed) != "" {
				out = strings.TrimSpace(trimmed)
				changed = true
			}
		}
	}
	return out
}

// looseName is a name with the differences that are not differences taken out:
// case, and which separator somebody used between words.
//
// A want called transit-search-instance is of type transit_search and gets
// asked about as "transit search" — three spellings of one name, and a matcher
// that compares them literally finds nothing while the tile sits on the board.
// Only separators and case; nothing else is touched, so two genuinely different
// names never collapse into one.
func looseName(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch r {
		case ' ', '-', '_', '\u3000':
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// findThingPlace matches a thing by its name — exactly first, then by
// containing the words, so "新宿" finds 新宿 rather than 新宿駅 when both exist
// and the exact one was asked for. The name is taken as said: see
// bareThingName for the wrapping that comes off first.
func findThingPlace(c *client.Client, name string) (thingPlace, error) {
	things, err := c.GetThings()
	if err != nil {
		return thingPlace{}, err
	}
	needle := bareThingName(name)
	if needle == "" {
		return thingPlace{}, fmt.Errorf("no thing named %q", strings.TrimSpace(name))
	}
	var partial []thingPlace

	for _, t := range things {
		place := thingPlace{id: t.ID, value: t.Value, kind: t.Subtype, constellations: constellationNames(t)}
		if place.kind == "" {
			place.kind = t.Catalog
		}
		labels := t.Labels
		place.onCanvas = labels[thingCanvasLabel] == "true"
		place.x, _ = strconv.Atoi(labels[thingCanvasXLabel])
		place.y, _ = strconv.Atoi(labels[thingCanvasYLabel])

		// An id is a name too. `mywant board` prints one for every tile, so
		// whatever read it can hand back the exact tile rather than the words it
		// was asked with — which is the point of printing the id at all. The
		// board prints it short, so the short form has to be enough here.
		if idMatches(t.ID, name) || idMatches(t.ID, needle) {
			return place, nil
		}
		if t.Value == needle || looseName(t.Value) == looseName(needle) {
			return place, nil
		}
		if strings.Contains(looseName(t.Value), looseName(needle)) {
			partial = append(partial, place)
		}
	}

	switch len(partial) {
	case 0:
		return thingPlace{}, fmt.Errorf("no thing named %q", needle)
	case 1:
		return partial[0], nil
	default:
		names := make([]string, 0, len(partial))
		for _, p := range partial {
			names = append(names, p.value)
		}
		return thingPlace{}, fmt.Errorf("%q matches several things: %s", needle, strings.Join(names, ", "))
	}
}

var thingWhereCmd = &cobra.Command{
	Use:     "where <thing>",
	Aliases: []string{"w"},
	Short:   "Say where a thing is, without moving the robot",
	Long: `Prints the cell a thing stands on, or says that it is not on the board.

The answer is one line, in words, so anything that answers questions — the
robot, an agent, a person reading a terminal — can repeat it as it is.`,
	Example: `  mywant thing where 新宿
  mywant thing where Kokubunji`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		c := memoClient()
		place, err := findThingPlace(c, args[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if jsonOut(cmd) {
			printJSON(map[string]any{
				"id": place.id, "value": place.value, "kind": place.kind,
				"onCanvas": place.onCanvas, "x": place.x, "y": place.y,
				"constellations": place.constellations,
			})
			return
		}
		if !place.onCanvas {
			fmt.Printf("%s is not on the canvas. Pin it to put it there. | id: %s\n", place.describe(), shortID(place.id))
			return
		}
		fmt.Printf("%s is on the canvas at (%d, %d). | id: %s\n", place.describe(), place.x, place.y, shortID(place.id))
	},
}

var thingPointCmd = &cobra.Command{
	Use:     "point <thing>",
	Short:   "Where a thing is: say the cell AND send the robot to stand on it",
	Aliases: []string{"show-me"},
	Long: `Moves the robot's tile onto the thing's cell and gives it a line to say.

"Where is 新宿?" answered in coordinates is an answer nobody can use while
looking at the board: the cell is only a place once you have found it. So the
robot goes and stands on it, which is the same answer pointed at rather than
spelled out.

Nothing is changed except where the robot is standing and what it is saying —
both of which it changes on its own, wandering, every minute or so.`,
	Example: `  mywant thing point 新宿
  mywant thing point 新宿 --say "ここが新宿です"`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		c := memoClient()
		place, err := findThingPlace(c, args[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if !place.onCanvas {
			fmt.Printf("%s is not on the canvas, so there is nowhere to point.\n", place.describe())
			return
		}

		api := client.NewClient(viper.GetString("server"))
		for key, value := range map[string]string{
			thingCanvasXLabel: strconv.Itoa(place.x),
			thingCanvasYLabel: strconv.Itoa(place.y),
		} {
			if err := api.AddWantLabel(robotWantName, key, value); err != nil {
				fmt.Fprintf(os.Stderr, "Error moving the robot: %v\n", err)
				os.Exit(1)
			}
		}

		words, _ := cmd.Flags().GetString("say")
		if words == "" {
			words = fmt.Sprintf("「%s」はここです (%d, %d)", place.value, place.x, place.y)
		}
		if err := api.SetWantState(robotWantName, map[string]any{"say": words}); err != nil {
			fmt.Fprintf(os.Stderr, "Error: the robot got there but could not speak: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("The robot is standing on %s at (%d, %d) and saying: %s | id: %s\n",
			place.describe(), place.x, place.y, words, shortID(place.id))
	},
}

func init() {
	thingWhereCmd.Flags().Bool("json", false, "Output as JSON")
	thingPointCmd.Flags().String("say", "", "What the robot says when it gets there (default: the thing and its cell)")
	ThingCmd.AddCommand(thingWhereCmd)
	ThingCmd.AddCommand(thingPointCmd)
}
