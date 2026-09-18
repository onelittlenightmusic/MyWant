package commands

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"mywant/client"
)

// Everything standing on the board, with what is known about it.
//
// The list exists for the step before pointing. Asked "where is transit
// search?", anything answering has the words of the question and not the name
// of the tile — which is transit-search-instance, of type transit_search, and
// neither is what was said. Matching the two loosely inside the lookup gets
// some of those right and guesses at the rest.
//
// Reading the board first turns a guess into a choice. But a name alone is a
// thin choice: "荻窪はどの星座？" and "荻窪はどこ？" are different questions
// about the same tile, and a list of names answers neither without a second
// call. So each line carries what the board already knows about the tile — its
// id, what it is, where it stands, the constellations it is in, what names it —
// and most questions about the board are then answerable from one read.
var BoardCmd = &cobra.Command{
	Use:     "board",
	Short:   "Everything standing on the canvas: name, id, kind, cell, constellation (星座) — read this first",
	Aliases: []string{"names"},
	Long: `Lists every thing and want on the canvas with what is known about each one.

Each line is one tile: its name, its id, what it is, the cell it stands on,
and — for a thing — which constellations (星座) it belongs to and which wants
name it.

This is the list to read before ` + "`mywant point <name>`" + `: it names each tile
exactly as ` + "`point`" + ` expects to be given it, and an id works there too. It also
answers "which constellation is this in?" without a second call.`,
	Example: `  mywant board
  mywant board --json`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		type entry struct {
			Name           string   `json:"name"`
			ID             string   `json:"id"`
			Kind           string   `json:"kind"`
			What           string   `json:"what"`
			X              int      `json:"x"`
			Y              int      `json:"y"`
			Constellations []string `json:"constellations,omitempty"`
			NamedBy        []string `json:"namedBy,omitempty"`
			Status         string   `json:"status,omitempty"`
		}
		var entries []entry

		things, err := memoClient().GetThings()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing things: %v\n", err)
			os.Exit(1)
		}
		resp, err := wantsClient().ListWants("", nil, nil, false, true)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing wants: %v\n", err)
			os.Exit(1)
		}
		wantNames := map[string]string{}
		for _, w := range resp.Wants {
			wantNames[w.Metadata.ID] = w.Metadata.Name
		}

		for _, t := range things {
			if t.Labels[canvasOnLabel] != "true" {
				continue
			}
			x, _ := strconv.Atoi(t.Labels[canvasXLabel])
			y, _ := strconv.Atoi(t.Labels[canvasYLabel])
			what := t.Subtype
			if what == "" {
				what = t.Catalog
			}
			var namedBy []string
			for _, id := range t.WantIDs {
				if name, ok := wantNames[id]; ok {
					namedBy = append(namedBy, name)
				}
			}
			sort.Strings(namedBy)
			entries = append(entries, entry{
				Name: t.Value, ID: t.ID, Kind: "thing", What: what, X: x, Y: y,
				Constellations: constellationNames(t), NamedBy: namedBy,
			})
		}

		for _, w := range resp.Wants {
			x, errX := strconv.Atoi(w.Metadata.Labels[canvasXLabel])
			y, errY := strconv.Atoi(w.Metadata.Labels[canvasYLabel])
			if errX != nil || errY != nil {
				continue
			}
			entries = append(entries, entry{
				Name: w.Metadata.Name, ID: w.Metadata.ID, Kind: "want", What: w.Metadata.Type,
				X: x, Y: y, Status: w.Status,
			})
		}

		sort.Slice(entries, func(i, j int) bool {
			if entries[i].Kind != entries[j].Kind {
				return entries[i].Kind < entries[j].Kind
			}
			return entries[i].Name < entries[j].Name
		})

		if jsonOut(cmd) {
			printJSON(entries)
			return
		}
		for _, e := range entries {
			line := fmt.Sprintf("%s (%s %s) at (%d, %d)", e.Name, e.Kind, e.What, e.X, e.Y)
			if len(e.Constellations) > 0 {
				line += fmt.Sprintf(" | constellation (星座): %s", strings.Join(e.Constellations, ", "))
			}
			if len(e.NamedBy) > 0 {
				line += fmt.Sprintf(" | named by: %s", strings.Join(e.NamedBy, ", "))
			}
			if e.Status != "" {
				line += fmt.Sprintf(" | status: %s", e.Status)
			}
			fmt.Printf("%s | id: %s\n", line, shortID(e.ID))
		}
		fmt.Printf("\nTotal: %d on the board\n", len(entries))
	},
}

func init() {
	BoardCmd.Flags().Bool("json", false, "Output as JSON")
}

// shortID is an id with the rest of the UUID left off.
//
// thg-a7732139-264f-4863-b46d-e331365a1e99 is forty characters of which the
// first twelve already pick out one tile among forty; printed whole, forty of
// them cost more of a small context window than everything else on the board
// put together. So the board prints the short form and the lookups take it —
// see idMatches, which accepts a prefix as long as it names one tile. The full
// id is still there in --json, for anything that wants to keep it.
func shortID(id string) string {
	parts := strings.Split(id, "-")
	if len(parts) <= 3 {
		return id
	}
	return strings.Join(parts[:2], "-")
}

// idMatches reports whether a name is this id, or the start of it.
//
// A prefix counts only when it is long enough to have been an id rather than a
// name: "新宿" and "weather" must never be read as the beginning of some UUID.
func idMatches(id, name string) bool {
	name = strings.TrimSpace(name)
	if id == name {
		return true
	}
	if len(name) < 8 || !strings.Contains(name, "-") {
		return false
	}
	return strings.HasPrefix(id, name)
}

// constellationNames are the constellations one thing belongs to.
//
// Membership is a label — constellation/<name> — so the names are the labels
// with the prefix taken off, sorted so two reads of the board agree.
func constellationNames(t client.Thing) []string {
	var names []string
	for key := range t.Labels {
		if name, ok := strings.CutPrefix(key, "constellation/"); ok && name != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}
