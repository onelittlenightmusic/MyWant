package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// MagicCmd says a magic sentence (a want's or its type's label
// magic-sentence) and shows what came of it: `mywant magic transit 荻窪 中野坂上`
// makes the route — or finds the one already there — and prints its answer,
// as the robot's chat and a keyboard do with the same sentence.
var MagicCmd = &cobra.Command{
	Use:     "magic <words...>",
	Aliases: []string{"m"},
	Short:   "Say a magic sentence: make (or find) the want it names and print its result",
	Long: `Says the words as a magic sentence — a want type's (or a want's) label
magic-sentence, such as "<alias> <from:station> <to:station>" on transit_search
— and prints what came of it: the want made, or the one already there with the
same parameters, and its result fields once its answer is in.

The words are joined with spaces, so they need no quotes.`,
	Example: `  mywant magic transit 荻窪 中野坂上
  mywant m 荻窪から新宿まで
  mywant magic transit 荻窪 中野坂上 --wait 30 --json
  mywant m transit 荻窪 中野坂上 --field departure        # 16:49
  mywant m transit 荻窪 中野坂上 --field departure,fare
  mywant wants restart $(mywant m transit 荻窪 中野坂上 --id)`,
	Args:              cobra.MinimumNArgs(1),
	ValidArgsFunction: completeMagic,
	Run: func(cmd *cobra.Command, args []string) {
		wait, _ := cmd.Flags().GetInt("wait")
		asJSON, _ := cmd.Flags().GetBool("json")
		only, _ := cmd.Flags().GetStringSlice("field")
		idOnly, _ := cmd.Flags().GetBool("id")
		// --id: which want is all that is asked — its answer is not waited
		// for, unless --wait says so.
		if idOnly && !cmd.Flags().Changed("wait") {
			wait = -1
		}
		c := wantsClient()
		var res struct {
			Matched  bool              `json:"matched"`
			Answer   string            `json:"answer"`
			WantID   string            `json:"wantId"`
			WantName string            `json:"wantName"`
			WantType string            `json:"wantType"`
			Params   map[string]string `json:"params"`
			Settled  bool              `json:"settled"`
		}
		text := strings.Join(args, " ")
		if err := c.Request("POST", "/api/v1/magic", map[string]any{"text": text, "wait": wait}, &res); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if !res.Matched {
			fmt.Fprintf(os.Stderr, "No magic sentence matches %q (see a want type's label magic-sentence)\n", text)
			os.Exit(1)
		}
		// --id: the want's id alone, for another command to act on it:
		// mywant wants restart $(mywant m transit 荻窪 中野坂上 --id)
		if idOnly {
			if res.WantID == "" {
				fmt.Fprintf(os.Stderr, "Error: %s\n", res.Answer)
				os.Exit(1)
			}
			fmt.Println(res.WantID)
			return
		}
		fields := map[string]any{}
		if res.WantID != "" {
			if w, err := c.GetWant(res.WantID, false); err == nil {
				fields = magicFields(w.State)
			}
		}
		// --field: those fields alone — one asked prints its value bare (whole,
		// for a script to read), several print a line each.
		if len(only) > 0 {
			picked := map[string]any{}
			var missing []string
			for _, f := range only {
				if v, ok := fields[f]; ok {
					picked[f] = v
				} else {
					missing = append(missing, f)
				}
			}
			if len(missing) > 0 {
				names := make([]string, 0, len(fields))
				for k := range fields {
					names = append(names, k)
				}
				sort.Strings(names)
				fmt.Fprintf(os.Stderr, "Error: no field %s on %s (it has: %s)\n", strings.Join(missing, ", "), res.WantName, strings.Join(names, ", "))
				if !res.Settled {
					fmt.Fprintln(os.Stderr, "(its answer is not in yet — try a longer --wait)")
				}
				os.Exit(1)
			}
			if asJSON {
				out, _ := json.MarshalIndent(picked, "", "  ")
				fmt.Println(string(out))
				return
			}
			if len(only) == 1 {
				fmt.Println(magicWhole(picked[only[0]]))
				return
			}
			for _, f := range only {
				fmt.Printf("%s: %s\n", f, magicWhole(picked[f]))
			}
			return
		}
		if asJSON {
			out, _ := json.MarshalIndent(map[string]any{
				"answer": res.Answer, "want": map[string]string{"id": res.WantID, "name": res.WantName, "type": res.WantType},
				"params": res.Params, "settled": res.Settled, "fields": fields,
			}, "", "  ")
			fmt.Println(string(out))
			return
		}
		fmt.Println(res.Answer)
		if res.WantName != "" {
			fmt.Printf("%s (%s, %s)\n", res.WantName, res.WantType, res.WantID)
		}
		if !res.Settled {
			fmt.Println("(its answer is not in yet — run again, or see `mywant wants get " + res.WantID + "`)")
		}
		keys := make([]string, 0, len(fields))
		for k := range fields {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool {
			// The result first, then the rest by name.
			if (keys[i] == "final_result") != (keys[j] == "final_result") {
				return keys[i] == "final_result"
			}
			return keys[i] < keys[j]
		})
		for _, k := range keys {
			fmt.Printf("  %s: %s\n", k, magicValue(fields[k]))
		}
	},
}

// magicFields: a want's current state as its fields — not the bookkeeping a
// want keeps while it works.
func magicFields(state map[string]any) map[string]any {
	cur := state
	if c, ok := state["current"].(map[string]any); ok {
		cur = c
	}
	skip := map[string]bool{"action_by_agent": true, "last_poll_at": true, "achieving_percentage": true,
		"mrs_raw_output": true, "status": true, "completed": true, "achieved": true, "current_session_state": true}
	out := map[string]any{}
	for k, v := range cur {
		if strings.HasPrefix(k, "_") || skip[k] || v == nil || v == "" {
			continue
		}
		out[k] = v
	}
	return out
}

// magicWhole: a field's value as it is — text as text, anything else as JSON.
func magicWhole(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case map[string]any, []any:
		b, _ := json.Marshal(x)
		return string(b)
	default:
		return fmt.Sprint(x)
	}
}

// magicValue: a field's value on one line, cut at 160 characters (not bytes:
// a cut in the middle of 荻窪 printed half a character).
func magicValue(v any) string {
	var s string
	switch x := v.(type) {
	case string:
		s = x
	case map[string]any, []any:
		b, _ := json.Marshal(x)
		s = string(b)
	default:
		s = fmt.Sprint(x)
	}
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > 160 {
		s = string(r[:160]) + "…"
	}
	return s
}

func init() {
	MagicCmd.Flags().Int("wait", 15, "Seconds to wait for the want's answer (at most 60)")
	MagicCmd.Flags().Bool("json", false, "Print the result as JSON")
	MagicCmd.Flags().StringSlice("field", nil, "Only these fields (repeat or comma-separate); one alone prints its bare value")
	_ = MagicCmd.RegisterFlagCompletionFunc("field", completeMagicField)
	MagicCmd.Flags().Bool("id", false, "Print only the want's id (no wait for its answer unless --wait is given)")
}

// magicCommand: a want type said as a command — one of its aliases, then the
// parameters of its magic sentence that starts with <alias>
// ("<alias> <from:station> <to:station>" on transit_search).
type magicCommand struct {
	Type, Title string
	Aliases     []string
	Slots       []magicCompSlot
}

type magicCompSlot struct{ Param, Subtype string }

// magicCommands: the want types that can be said as a command.
func magicCommands() ([]magicCommand, error) {
	var res struct {
		WantTypes []struct {
			Name   string            `json:"name"`
			Title  string            `json:"title"`
			Labels map[string]string `json:"labels"`
		} `json:"wantTypes"`
	}
	if err := wantsClient().Request("GET", "/api/v1/want-types", nil, &res); err != nil {
		return nil, err
	}
	var out []magicCommand
	for _, t := range res.WantTypes {
		aliases, sentences := t.Labels["aliases"], t.Labels["magic-sentence"]
		if aliases == "" || sentences == "" {
			continue
		}
		var names []string
		for _, a := range strings.Split(aliases, ",") {
			if a = strings.TrimSpace(a); a != "" {
				names = append(names, a)
			}
		}
		for _, s := range strings.FieldsFunc(sentences, func(r rune) bool { return r == '|' || r == '｜' || r == '\n' }) {
			parts := strings.Fields(s)
			if len(parts) < 2 || parts[0] != "<alias>" {
				continue
			}
			var slots []magicCompSlot
			for _, p := range parts[1:] {
				if !strings.HasPrefix(p, "<") || !strings.HasSuffix(p, ">") {
					slots = nil
					break
				}
				param, sub, _ := strings.Cut(p[1:len(p)-1], ":")
				slots = append(slots, magicCompSlot{param, sub})
			}
			if len(slots) > 0 && len(names) > 0 {
				title := t.Title
				if title == "" {
					title = t.Name
				}
				out = append(out, magicCommand{t.Name, title, names, slots})
				break
			}
		}
	}
	return out, nil
}

// completeMagic: the aliases first ("tra" → transit, with what it makes and
// its parameters), then each parameter's things of its kind (stations for
// <from:station>), as the keyboards offer them.
func completeMagic(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	cmds, err := magicCommands()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	if len(args) == 0 {
		var out []string
		low := strings.ToLower(toComplete)
		for _, c := range cmds {
			rest := make([]string, len(c.Slots))
			for i, s := range c.Slots {
				rest[i] = "<" + s.Param + ">"
			}
			for _, a := range c.Aliases {
				if strings.HasPrefix(strings.ToLower(a), low) {
					out = append(out, fmt.Sprintf("%s\t%s %s", a, c.Title, strings.Join(rest, " ")))
				}
			}
		}
		return out, cobra.ShellCompDirectiveNoFileComp
	}
	var found *magicCommand
	for i, c := range cmds {
		for _, a := range c.Aliases {
			if strings.EqualFold(a, args[0]) {
				found = &cmds[i]
			}
		}
	}
	slot := len(args) - 1
	if found == nil || slot >= len(found.Slots) {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	s := found.Slots[slot]
	var things struct {
		Things []struct {
			Subtype string            `json:"subtype"`
			Value   string            `json:"value"`
			Labels  map[string]string `json:"labels"`
		} `json:"things"`
	}
	if err := wantsClient().Request("GET", "/api/v1/things", nil, &things); err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	seen := map[string]bool{}
	var out []string
	byReading := false
	typed := strings.ToLower(toComplete)
	for _, t := range things.Things {
		if (s.Subtype != "" && t.Subtype != s.Subtype) || t.Value == "" || seen[t.Value] {
			continue
		}
		// By its name, or — letters typed for a name in Japanese — by its
		// reading (the label reading: "ogi" → 荻窪, reading ogikubo).
		reading := foldReading(t.Labels[ThingReadingLabel])
		switch {
		case strings.HasPrefix(t.Value, toComplete):
		case typed != "" && isASCII(typed) && reading != "" && strings.HasPrefix(reading, foldReading(typed)):
			byReading = true
		default:
			continue
		}
		seen[t.Value] = true
		out = append(out, fmt.Sprintf("%s\t<%s> %s", t.Value, s.Param, t.Subtype))
	}
	d := cobra.ShellCompDirectiveNoFileComp
	if byReading {
		// What was typed is not the start of these: the shell is to offer them
		// anyway (CompDirectiveAsChosen, completion_patch.go).
		d |= CompDirectiveAsChosen
	}
	return out, d
}

// ThingReadingLabel: a thing's name in Latin letters, as it is read
// (荻窪 → ogikubo) — what letters typed on a keyboard or in a terminal are
// compared with. The apps fill it in from the system's own reading of
// Japanese (PaletteFetch.fillReadings); a person may set it.
const ThingReadingLabel = "reading"

// foldReading: a reading as it is typed, more than one way — long vowels
// short (toukyou, tokyo; ootemachi, otemachi; yuurakuchou), and the
// spellings of one sound one (shi/si, chi/ti, tsu/tu, fu/hu, ji/zi, mb/nb).
// Both what is typed and the label are folded alike.
func foldReading(s string) string {
	s = strings.ToLower(s)
	for _, r := range [][2]string{
		{"shi", "si"}, {"chi", "ti"}, {"tsu", "tu"}, {"fu", "hu"}, {"ji", "zi"},
		{"sh", "sy"}, {"ch", "ty"}, {"mb", "nb"}, {"mp", "np"}, {"mm", "nm"},
		{"ou", "o"}, {"oo", "o"}, {"uu", "u"}, {"ei", "e"}, {"ii", "i"}, {"aa", "a"},
	} {
		s = strings.ReplaceAll(s, r[0], r[1])
	}
	return s
}

func isASCII(s string) bool {
	for _, r := range s {
		if r > 0x7f {
			return false
		}
	}
	return true
}

// completeMagicField: --field's names — the state fields the command's want
// type declares (its current ones, not what it keeps hidden), with what each
// is. Known once the alias is typed; before it, nothing.
func completeMagicField(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) == 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	cmds, err := magicCommands()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	typ := ""
	for _, c := range cmds {
		for _, a := range c.Aliases {
			if strings.EqualFold(a, args[0]) {
				typ = c.Type
			}
		}
	}
	if typ == "" {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	var def struct {
		State []struct {
			Name        string `json:"name"`
			Label       string `json:"label"`
			Description string `json:"description"`
		} `json:"state"`
	}
	if err := wantsClient().Request("GET", "/api/v1/want-types/"+typ, nil, &def); err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	// Comma-separated: what is already named stays, the last is completed.
	done, last := "", toComplete
	if i := strings.LastIndex(toComplete, ","); i >= 0 {
		done, last = toComplete[:i+1], toComplete[i+1:]
	}
	var out []string
	for _, f := range def.State {
		if f.Label != "current" || strings.HasPrefix(f.Name, "_") || !strings.HasPrefix(f.Name, last) {
			continue
		}
		desc := f.Description
		if r := []rune(desc); len(r) > 60 {
			desc = string(r[:60]) + "…"
		}
		out = append(out, done+f.Name+"\t"+desc)
	}
	return out, cobra.ShellCompDirectiveNoFileComp | cobra.ShellCompDirectiveNoSpace
}
