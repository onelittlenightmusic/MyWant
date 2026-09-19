package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"mywant/client"
)

// What a want or a thing is connected to.
//
// The board is not a set of tiles standing apart: a want reads a thing (the
// weather want reads Kokubunji), and a want passes a field to another want (the
// claude-info want's session percentage feeds the gauge). Both are drawn as
// roads on the canvas and both were readable only by assembling them yourself —
// one API for the field relations, another for which wants name a thing, and a
// want's own params for what it named.
//
// "What is weather connected to?" is one question, so it is one command.
//
//	mywant relations weather          what the weather want reads and feeds
//	mywant relations 新宿             the wants that name this thing
type relationLine struct {
	Direction string `json:"direction"` // "reads" | "feeds" | "named by"
	Other     string `json:"other"`
	Kind      string `json:"kind"`  // "want" | "thing"
	Field     string `json:"field"` // the field carried, for want→want
}

var RelationsCmd = &cobra.Command{
	Use:     "relations <name>",
	Short:   "What a want or thing is connected to — what it reads, feeds, is named by, and which constellation (星座/group) it is in",
	Aliases: []string{"connections", "rel"},
	Long: `Lists the connections of one want or thing, in both directions.

For a want: the things it names (its parameters that are named values), the
wants whose fields it consumes, and the wants that consume its fields.
For a thing: the wants that name it, and the constellations it belongs to —
which is the answer to "which constellation is this in?" as well as to "what is
this connected to?", since a constellation is a line drawn between things.

Connections are what makes the board a board rather than a pile of tiles; this
is how to read them without opening a card.`,
	Example: `  mywant relations weather
  mywant relations 新宿
  mywant relations gauge-instance --json`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		api := client.NewClient(viper.GetString("server"))
		name := args[0]
		var lines []relationLine
		subject := ""

		// A thing first, as everywhere else: a named value is what most names
		// are. A thing's connections are the wants that named it.
		things, err := memoClient().GetThings()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		wantsByID := map[string]*client.Want{}
		resp, err := api.ListWants("", nil, nil, false, true)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		for _, w := range resp.Wants {
			wantsByID[w.Metadata.ID] = w
		}

		if place, err := findThingPlace(memoClient(), name); err == nil {
			subject = fmt.Sprintf("%s (thing %s, id: %s)", place.value, place.kind, shortID(place.id))
			for _, t := range things {
				if t.ID != place.id {
					continue
				}
				for _, id := range t.WantIDs {
					other := id
					kind := "want"
					if w, ok := wantsByID[id]; ok {
						other = w.Metadata.Name
					}
					lines = append(lines, relationLine{Direction: "named by", Other: other, Kind: kind})
				}
				// The other way things are joined: a constellation, which is
				// the user saying "these are one thing" — 荻窪 and 新宿 are both
				// on 中央線. It is drawn as a line between them on the canvas,
				// so leaving it out of "what is this connected to" answered
				// "nothing" about a thing with two lines running through it.
				lines = append(lines, constellationLines(t, things)...)
			}
		} else if place, err := findWantPlace(api, name); err == nil {
			subject = fmt.Sprintf("%s (want %s, id: %s)", place.name, place.wantType, shortID(place.id))
			want := wantsByID[place.id]

			// The things this want names: its parameters, matched against the
			// values the board remembers. A parameter holding a remembered
			// value IS the connection — it is what the canvas draws a road for.
			if want != nil {
				var paramValues []string
				for _, v := range want.Spec.Params {
					if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
						paramValues = append(paramValues, s)
					}
				}
				for _, t := range things {
					for _, v := range paramValues {
						if t.Value != v {
							continue
						}
						// Named by its catalog, because one word can be two
						// things: 荻窪 is filed under both stations and
						// cities, and a want that names it reads both. Two
						// identical lines read as a bug; two lines that say
						// which is which read as the board.
						kind := "thing"
						if t.Subtype != "" {
							kind = "thing " + t.Subtype
						} else if t.Catalog != "" {
							kind = "thing " + t.Catalog
						}
						lines = append(lines, relationLine{Direction: "reads", Other: t.Value, Kind: kind})
					}
				}
			}

			// The fields flowing between wants, in both directions.
			var all struct {
				Relations []struct {
					ProviderID   string `json:"provider_id"`
					ProviderName string `json:"provider_name"`
					ConsumerID   string `json:"consumer_id"`
					ConsumerName string `json:"consumer_name"`
					FieldName    string `json:"field_name"`
				} `json:"relations"`
			}
			if err := api.Request("GET", "/api/v1/relations", nil, &all); err == nil {
				for _, r := range all.Relations {
					switch place.id {
					case r.ConsumerID:
						lines = append(lines, relationLine{Direction: "reads", Other: r.ProviderName, Kind: "want", Field: r.FieldName})
					case r.ProviderID:
						lines = append(lines, relationLine{Direction: "feeds", Other: r.ConsumerName, Kind: "want", Field: r.FieldName})
					}
				}
			}
		} else {
			fmt.Fprintf(os.Stderr, "Error: nothing on the board is named %q\n", name)
			os.Exit(1)
		}

		sort.Slice(lines, func(i, j int) bool {
			if lines[i].Direction != lines[j].Direction {
				return lines[i].Direction < lines[j].Direction
			}
			return lines[i].Other < lines[j].Other
		})

		if jsonOut(cmd) {
			out, _ := json.Marshal(map[string]any{"subject": subject, "relations": lines})
			fmt.Println(string(out))
			return
		}
		if len(lines) == 0 {
			fmt.Printf("%s is connected to nothing.\n", subject)
			return
		}
		fmt.Printf("%s is connected to:\n", subject)
		for _, l := range lines {
			switch {
			case l.Kind == "constellation" && l.Field != "":
				fmt.Printf("  %-10s %s (%s) with %s\n", l.Direction, l.Other, l.Kind, l.Field)
			case l.Field != "":
				fmt.Printf("  %-10s %s (%s) via %s\n", l.Direction, l.Other, l.Kind, l.Field)
			default:
				fmt.Printf("  %-10s %s (%s)\n", l.Direction, l.Other, l.Kind)
			}
		}
	},
}

func init() {
	RelationsCmd.Flags().Bool("json", false, "Output as JSON")
}

// constellationLines reports the constellations a thing belongs to, and who
// else is in them.
//
// Membership is a label on the thing — constellation/<name>, whose value is the
// order it sits in — so the members of one are every thing carrying that label.
func constellationLines(subject client.Thing, things []client.Thing) []relationLine {
	var lines []relationLine
	for key := range subject.Labels {
		name, ok := strings.CutPrefix(key, "constellation/")
		if !ok || name == "" {
			continue
		}
		var members []string
		for _, other := range things {
			if other.ID == subject.ID {
				continue
			}
			if _, in := other.Labels[key]; in {
				members = append(members, other.Value)
			}
		}
		sort.Strings(members)
		lines = append(lines, relationLine{
			Direction: "grouped in",
			Other:     name,
			Kind:      "constellation",
			Field:     strings.Join(members, ", "),
		})
	}
	return lines
}
