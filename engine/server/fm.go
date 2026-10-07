package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// The robot, for an on-device model elsewhere (Apple FoundationModels on an
// iPhone).
//
// The model runs on the device; who it is and everything it reaches for come
// from here. The app fetches /api/v1/fm/manifest — the robot's instructions
// and each tool's name, description and string arguments — builds its tools
// from that at runtime, and sends every call back to /api/v1/fm/call — each
// as a step of a turn (fm_turns.go): the question, what the robot did, and
// what it answered, kept here whole so the robot on the board has done it. So the
// app holds no knowledge of MyWant at all: the frame is this file, and
// changing the robot is a change here, not a new build on the phone.
//
// Ported from the Mac's own robot (fmtool/), less what only a Mac has — the filesystem, the CLI — and in the same spirit: a guide shows
// rather than recites, so "荻窪はどこ？" walks the robot to 荻窪.
//
// Answers are short on purpose. The model has 8k tokens for the instructions,
// the tool schemas and the whole talk; the board dumped raw would fill it.

type fmTool struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Arguments   []fmParam `json:"arguments"`
	run         func(s *Server, args map[string]string) (string, error)
	// replays: run again when a turn is played again (fm_turns.go) — the
	// steps that show something. Not the ones that make something, which a
	// second run would make twice, nor the ones that only read.
	replays bool
}

// fmParam is one argument. Every argument is a string: the device builds its
// schema from these, and one kind keeps that side trivial. A structured value
// (a want's params) travels as name=value pairs (fmParseParams).
type fmParam struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
	// Choices, when set, are the only values the device lets the model
	// produce: it becomes an enum in the generation schema, so a type that
	// does not exist cannot be written at all. Asking a model this small to
	// look a name up first does not work — it guesses ("sticky_note").
	Choices []string `json:"choices,omitempty"`
}

const fmInstructions = `You are the robot on the MyWant board, and a guide: a guide does not recite, a guide shows. The person talks to you from their phone; you stand on the board on their computer, and what you do appears there.
The board holds things (named values: stations, cities, places, albums) and wants (small tasks shown as tiles). Two kinds of question, two tools:
- WHERE something is (「荻窪はどこ？」): call point with the name alone ("荻窪", never the sentence). It walks you there so the person can see it; then say so, e.g. 「荻窪に来ました」.
- WHAT something says or how it is (「Nakanoの天気は？」, 「スマートゴルフの予約は？」, a timer's time left, a checklist): call look with the name (「Nakano」 finds NakanoのWeather, 「スマートゴルフ」 finds the SmartGolf want), and tell the person what it reports, e.g. 「Nakanoは曇り、23°Cです」 — the values that answer the question, read by what the type says they mean.
Pointing is showing, and showing is good: when the answer is on the board and seeing it helps — the want you just read, the place it is about — you may also point to it, so the person sees it while you say it. Point to the same name you looked at. Never point instead of answering: the words come from look.
Use board to see the names of what is on it — it only lists names, so after board, call look (or point) with the name you found; never answer from board alone.
To add a want, choose its type, read its parameters with describe_type, then deploy_want. A want already there with the same parameters is not made twice: deploy_want walks you to it instead — then say so, e.g. 「もうあります。ここです」.
An archived want is put away, off the board. When a tool says one is archived, make nothing new: ask the person whether to restore it, e.g. 「アーカイブした「新宿御苑前→銀座」があります。復元しますか？」, and if they say yes, call restore_want with its name.
Some types say how their questions are handled (the lines after these, and in describe_type): do as the type says.
Greetings and remarks about what was just said need no tool.
Say only what a tool told you or what you were told here; if a tool could not answer, say so, and never fill the gap from your own knowledge of the world. Answer in the language the person used — in Japanese when they write Japanese — in one or two short sentences.`

var fmTools = []fmTool{
	{
		Name:        "board",
		Description: "The names of everything on the board, things and wants, with their kinds. To say where one is, use point.",
		run:         (*Server).fmBoard,
	},
	{
		Name:        "point",
		Description: "For WHERE questions only: walks the robot onto a thing or want so the person sees where it is.",
		Arguments: []fmParam{
			{Name: "name", Description: "The name alone, e.g. 荻窪 or note-instance", Required: true},
		},
		run:     (*Server).fmPoint,
		replays: true,
	},
	{
		Name:        "look",
		Description: "For WHAT questions: what a want on the board says and how it is doing — the weather a weather want fetched, a timer's time left, its status.",
		Arguments: []fmParam{
			{Name: "name", Description: "The want's name or part of it, e.g. Nakano for NakanoのWeather", Required: true},
		},
		run: (*Server).fmLook,
	},
	{
		Name:        "describe_type",
		Description: "Describe one want type and the parameters it takes. Use before deploy_want.",
		Arguments: []fmParam{
			{Name: "type", Description: "The type name", Required: true},
		},
		run: (*Server).fmDescribeType,
	},
	{
		Name:        "deploy_want",
		Description: "Put a new want on the board, next to the person — or, if one of that type with the same parameters is already there, walk to it instead.",
		Arguments: []fmParam{
			{Name: "type", Description: "The type of the want", Required: true},
			{Name: "params", Description: "The parameters as name=value pairs separated by commas, e.g. from=新宿, to=渋谷 — no quotes, no braces. Empty when there are none.", Required: true},
			{Name: "name", Description: "A short name for the want, lowercase with hyphens", Required: false},
		},
		run: (*Server).fmDeployWant,
	},
	{
		Name:        "restore_want",
		Description: "Bring an archived want back onto the board — only after the person said yes to restoring it.",
		Arguments: []fmParam{
			{Name: "name", Description: "The archived want's name, as the tool that found it gave it", Required: true},
		},
		run: (*Server).fmRestoreWant,
	},
}

func (s *Server) handleFMManifest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// The type names are filled in per request, from the backend's list as it
	// is now: a type registered a minute ago is choosable without a rebuild.
	types, err := s.fmTypeNames()
	if err != nil {
		http.Error(w, "backend: "+err.Error(), http.StatusBadGateway)
		return
	}
	tools := make([]fmTool, len(fmTools))
	for i, t := range fmTools {
		t.Arguments = append([]fmParam(nil), t.Arguments...)
		for j, a := range t.Arguments {
			if a.Name == "type" && t.Name == "deploy_want" {
				a.Choices = types
				t.Arguments[j] = a
			}
		}
		tools[i] = t
	}
	fmWriteJSON(w, map[string]any{"instructions": fmInstructions + s.fmHintsText() + s.fmGlossaryText(), "tools": tools})
}

func (s *Server) handleFMCall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Tool      string            `json:"tool"`
		Arguments map[string]string `json:"arguments"`
		// The turn this call is a step of (fm_turns.go); recorded there and
		// shown in the robot's chat. Without one the call is only run.
		Turn string `json:"turn"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}
	if fmToolByName(req.Tool).Name == "" {
		http.Error(w, "unknown tool: "+req.Tool, http.StatusNotFound)
		return
	}
	if req.Arguments == nil {
		req.Arguments = map[string]string{}
	}
	step := fmStep{Tool: req.Tool, Arguments: req.Arguments}
	if req.Turn == "" {
		// Outside a turn it is only run: nobody asked the robot anything, so
		// its chat has nothing to show.
		out := s.fmRunTool(req.Tool, req.Arguments)
		fmWriteJSON(w, map[string]any{"output": out, "card": fmTakeCard(req.Arguments)})
		return
	}
	s.fmRunStep(&step)
	step.Card = fmTakeCard(step.Arguments)
	turn, err := s.fmUpdateTurn(req.Turn, func(t *fmTurn) error {
		t.Steps = append(t.Steps, step)
		return nil
	})
	if err != nil {
		log.Printf("[fm] step outside a known turn: %v", err)
	} else if !turn.Quiet {
		s.fmShowStep(step, "")
	}
	fmWriteJSON(w, map[string]any{"output": step.Output, "card": step.Card})
}

// fmTakeCard takes out of a step's arguments the want its tool brought the
// robot to (fmCardArg), if any.
func fmTakeCard(args map[string]string) *fmCard {
	c, ok := args[fmCardArg]
	if !ok {
		return nil
	}
	delete(args, fmCardArg)
	parts := strings.SplitN(c, "\x00", 3)
	card := &fmCard{Kind: "want", ID: parts[0]}
	if len(parts) > 1 {
		card.Name = parts[1]
	}
	if len(parts) > 2 && parts[2] != "" {
		card.Kind = parts[2]
	}
	return card
}

// fmCardValue is how a tool notes the want or thing it brought the robot to
// (fmCardArg): id, name and kind, for fmTakeCard to read back.
func fmCardValue(kind, id, name string) string {
	return id + "\x00" + name + "\x00" + kind
}

// fmToolByName is the tool of that name, or the zero tool.
func fmToolByName(name string) fmTool {
	for _, t := range fmTools {
		if t.Name == name {
			return t
		}
	}
	return fmTool{}
}

// fmRunTool runs a tool and hands back its output. A failure is an answer
// too: the model reads it and can try again, so it comes back as output.
func (s *Server) fmRunTool(name string, args map[string]string) string {
	t := fmToolByName(name)
	if t.run == nil {
		return "Error: unknown tool " + name
	}
	out, err := t.run(s, args)
	if err != nil {
		out = "Error: " + err.Error()
	}
	log.Printf("[fm] %s %v → %s", name, args, firstLine(out, 160))
	return out
}

func fmWriteJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(v)
}

// backend calls this server's own API in process, through the router: the
// tools then go through exactly the handlers every other client does —
// validation, the work log, the SSE that redraws the board — with nothing
// reimplemented here and no socket.
func (s *Server) backend(method, path string, body any, out any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, rd)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	s.router.ServeHTTP(rec, req)
	data := rec.Body.Bytes()
	if rec.Code >= 300 {
		return fmt.Errorf("%s %s: %d %s", method, path, rec.Code, firstLine(string(data), 200))
	}
	if out != nil {
		return json.Unmarshal(data, out)
	}
	return nil
}

type fmWantType struct {
	Name       string            `json:"name"`
	Title      string            `json:"title"`
	Category   string            `json:"category"`
	SystemType bool              `json:"system_type"`
	Labels     map[string]string `json:"labels"`
}

// fmAliasLabel is a want type's other names: what a person calls it rather
// than what it was named — "スマートゴルフ,ゴルフ" for smartgolf_check_reserved.
// A model this small cannot be relied on to turn スマートゴルフ into smartgolf,
// so the type says it.
const fmAliasLabel = "aliases"

// fmTypeAliases maps each type to its aliases (fmAliasLabel). Empty on a
// failure: aliases only ever help a lookup, never stop one.
func (s *Server) fmTypeAliases() map[string][]string {
	types, err := s.fmWantTypes()
	if err != nil {
		return nil
	}
	out := map[string][]string{}
	for _, t := range types {
		for _, a := range strings.Split(t.Labels[fmAliasLabel], ",") {
			if a = strings.TrimSpace(a); a != "" {
				out[t.Name] = append(out[t.Name], a)
			}
		}
	}
	return out
}

// fmHintLabel is a want type's word to the robot: how a question for it is
// handled, in the type's own YAML rather than in this file — transit_search's
// "a route: deploy at once, the stations need not be on the board, a time goes
// in as time=09:40, arrive_type=到着". The type knows its questions; the robot
// only reads what it says.
const fmHintLabel = "robot-hint"

// fmQALabel prefixes a want type's worked examples for the robot, one label
// each: the request after the slash, what should be done or said for it as the
// value — "robot-qa/9時40分までに戸塚に移動できる乗り換え": "deploy_want
// transit_search with params to=戸塚, time=09:40, arrive_type=到着". A small
// model follows an example more surely than a rule.
const fmQALabel = "robot-qa/"

// fmHintsMax keeps the hints, with the glossary, inside the phone's window.
const fmHintsMax = 2000

// fmTypeHint is what a type tells the robot, as the instructions and
// describe_type carry it: its hint, then its examples, each 「request」 → what
// to do. Empty when it says nothing.
func fmTypeHint(labels map[string]string) string {
	var b strings.Builder
	b.WriteString(strings.Join(strings.Fields(labels[fmHintLabel]), " "))
	// Labels are a map: in the requests' order, so the text is the same each time.
	var asks []string
	for k := range labels {
		if strings.HasPrefix(k, fmQALabel) {
			asks = append(asks, k)
		}
	}
	sort.Strings(asks)
	for _, k := range asks {
		q, a := strings.TrimSpace(strings.TrimPrefix(k, fmQALabel)), strings.Join(strings.Fields(labels[k]), " ")
		if q == "" || a == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n  ")
		}
		fmt.Fprintf(&b, "e.g. 「%s」 → %s", strings.Trim(q, "「」"), a)
	}
	return b.String()
}

// fmHintsText is every type's hint as the instructions carry it.
func (s *Server) fmHintsText() string {
	types, err := s.fmWantTypes()
	if err != nil {
		return ""
	}
	var b strings.Builder
	for _, t := range types {
		h := fmTypeHint(t.Labels)
		if h == "" {
			continue
		}
		row := fmt.Sprintf("- %s: %s\n", t.Name, h)
		if b.Len()+len(row) > fmHintsMax {
			break
		}
		b.WriteString(row)
	}
	if b.Len() == 0 {
		return ""
	}
	return "\n\nWhat some types say about their questions:\n" + b.String()
}

func fmLoose(v string) string {
	return strings.NewReplacer(" ", "", "　", "", "-", "", "_", "").Replace(strings.ToLower(strings.TrimSpace(v)))
}

// fmAliasScore is how well what the person said names a type by its aliases:
// the length of the longest alias it is or contains, 0 for none. Longest wins,
// so 「スマートゴルフの空き」 picks the type called that over the one called
// スマートゴルフ.
func fmAliasScore(said string, aliases []string) int {
	l, best := fmLoose(said), 0
	for _, a := range aliases {
		la := fmLoose(a)
		if la != "" && strings.Contains(l, la) && len([]rune(la)) > best {
			best = len([]rune(la))
		}
	}
	return best
}

// ── the glossary: the board's constellations as names that go together ──────
//
// A constellation is a person saying "these belong together": 中野 holds the
// city nakano and the station 中野坂上, 天気 holds the weather wants. Read as
// a glossary, every name in one is a way to say the others — which is what a
// small model cannot work out for itself (that 中野 is Nakano). Built from the
// constellations as they are each time it is asked for, never stored: a
// constellation made or changed a moment ago is in the next answer.

// fmGlossary is one line per constellation: its name, then its members' names.
func (s *Server) fmGlossary() [][]string {
	var groups struct {
		Groups []struct {
			Name    string   `json:"name"`
			Members []string `json:"members"`
		} `json:"groups"`
	}
	if err := s.backend("GET", "/api/v1/constellations", nil, &groups); err != nil {
		return nil
	}
	// Members are ids: a thing's (its own id, or the older catalog::value) or
	// a want's. Named the way the board names them.
	named := map[string]string{}
	var things struct {
		Things []struct {
			ID      string `json:"id"`
			Catalog string `json:"catalog"`
			Value   string `json:"value"`
		} `json:"things"`
	}
	if s.backend("GET", "/api/v1/things", nil, &things) == nil {
		for _, t := range things.Things {
			named[t.ID] = t.Value
			named[t.Catalog+"::"+t.Value] = t.Value
		}
	}
	var wants struct {
		Wants []struct {
			Metadata struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"metadata"`
		} `json:"wants"`
	}
	if s.backend("GET", "/api/v1/wants", nil, &wants) == nil {
		for _, w := range wants.Wants {
			named[w.Metadata.ID] = w.Metadata.Name
		}
	}
	var out [][]string
	for _, g := range groups.Groups {
		line := []string{g.Name}
		for _, m := range g.Members {
			n := named[m]
			if n == "" {
				if _, v, ok := strings.Cut(m, "::"); ok {
					n = v
				}
			}
			if n != "" && !containsString(line, n) {
				line = append(line, n)
			}
		}
		if len(line) > 1 {
			out = append(out, line)
		}
	}
	return out
}

// fmGlossaryMax keeps the glossary from crowding the window it is meant to
// help with (4,096 tokens on an iPhone): past it, the rest is left out.
const fmGlossaryMax = 800

// fmGlossaryText is the glossary as the instructions carry it.
func (s *Server) fmGlossaryText() string {
	lines := s.fmGlossary()
	if len(lines) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\nNames on this board that go together (its constellations), one group a line — a name in a line also means the others, so 「中野の天気」 is about a want named after any name in 中野's line:\n")
	for _, l := range lines {
		row := strings.Join(l, " = ") + "\n"
		if b.Len()+len(row) > fmGlossaryMax {
			break
		}
		b.WriteString(row)
	}
	return b.String()
}

// fmByGlossary picks the names a person meant through the glossary: every
// line holding a word of what they said, and of the names, the ones the most
// of those lines name. 「中野の天気」 reaches 中野's line (nakano) and 天気's
// (NakanoのWeather): the want both point at wins.
func fmByGlossary(said string, names []string, glossary [][]string) []int {
	l := fmLoose(said)
	var lines [][]string
	for _, line := range glossary {
		for _, word := range line {
			if w := fmLoose(word); w != "" && strings.Contains(l, w) {
				lines = append(lines, line)
				break
			}
		}
	}
	if len(lines) == 0 {
		return nil
	}
	best, out := 0, []int(nil)
	for i, n := range names {
		ln, score := fmLoose(n), 0
		for _, line := range lines {
			for _, word := range line {
				if w := fmLoose(word); w != "" && (ln == w || strings.Contains(ln, w)) {
					score++
					break
				}
			}
		}
		if score == 0 || score < best {
			continue
		}
		if score > best {
			best, out = score, nil
		}
		out = append(out, i)
	}
	return out
}

// fmByAlias picks, from the types of the wants on the board, the ones the
// person named by an alias: all of those with the best score.
func fmByAlias(said string, typeOf []string, aliases map[string][]string) []int {
	best, out := 0, []int(nil)
	for i, t := range typeOf {
		sc := fmAliasScore(said, aliases[t])
		if sc == 0 || sc < best {
			continue
		}
		if sc > best {
			best, out = sc, nil
		}
		out = append(out, i)
	}
	return out
}

func (s *Server) fmWantTypes() ([]fmWantType, error) {
	var resp struct {
		WantTypes []fmWantType `json:"wantTypes"`
	}
	if err := s.backend("GET", "/api/v1/want-types", nil, &resp); err != nil {
		return nil, err
	}
	var out []fmWantType
	for _, t := range resp.WantTypes {
		if !t.SystemType {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (s *Server) fmTypeNames() ([]string, error) {
	types, err := s.fmWantTypes()
	if err != nil {
		return nil, err
	}
	names := make([]string, len(types))
	for i, t := range types {
		names[i] = t.Name
	}
	return names, nil
}

// fmResolveType finds the type the model meant: exact, then ignoring case
// and the difference between spaces, hyphens and underscores, then by title.
// On a miss the error names the closest few, so the next try can succeed.
func (s *Server) fmResolveType(name string) (string, error) {
	norm := func(v string) string {
		return strings.NewReplacer(" ", "", "-", "", "_", "").Replace(strings.ToLower(strings.TrimSpace(v)))
	}
	types, err := s.fmWantTypes()
	if err != nil {
		return "", err
	}
	want := norm(name)
	for _, t := range types {
		if t.Name == name {
			return t.Name, nil
		}
	}
	for _, t := range types {
		if norm(t.Name) == want || norm(t.Title) == want {
			return t.Name, nil
		}
	}
	var near []string
	for _, t := range types {
		n := norm(t.Name)
		if want != "" && (strings.Contains(n, want) || strings.Contains(want, n) || strings.Contains(norm(t.Title), want)) {
			near = append(near, t.Name)
		}
	}
	if len(near) > 8 {
		near = near[:8]
	}
	if len(near) == 0 {
		return "", fmt.Errorf("there is no want type %q; choose one of the types deploy_want offers", name)
	}
	return "", fmt.Errorf("there is no want type %q; did you mean: %s", name, strings.Join(near, ", "))
}

func (s *Server) fmDescribeType(args map[string]string) (string, error) {
	name, err := s.fmResolveType(args["type"])
	if err != nil {
		return "", err
	}
	var resp struct {
		Metadata struct {
			Description string            `json:"description"`
			Labels      map[string]string `json:"labels"`
		} `json:"metadata"`
		Parameters []struct {
			Name        string `json:"name"`
			Type        string `json:"type"`
			Description string `json:"description"`
			Required    bool   `json:"required"`
			Example     any    `json:"example"`
		} `json:"parameters"`
	}
	if err := s.backend("GET", "/api/v1/want-types/"+name, nil, &resp); err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s: %s\n", name, firstLine(resp.Metadata.Description, 200))
	if h := fmTypeHint(resp.Metadata.Labels); h != "" {
		fmt.Fprintf(&b, "How: %s\n", h)
	}
	if len(resp.Parameters) == 0 {
		b.WriteString("No parameters; deploy with empty params.\n")
	}
	for i, p := range resp.Parameters {
		if i == 10 {
			b.WriteString("...more optional parameters omitted\n")
			break
		}
		req := "optional"
		if p.Required {
			req = "required"
		}
		fmt.Fprintf(&b, "- %s (%s, %s): %s", p.Name, p.Type, req, firstLine(p.Description, 120))
		if p.Example != nil {
			fmt.Fprintf(&b, " e.g. %v", p.Example)
		}
		b.WriteString("\n")
	}
	return b.String(), nil
}

func (s *Server) fmDeployWant(args map[string]string) (string, error) {
	typ, err := s.fmResolveType(args["type"])
	if err != nil {
		return "", err
	}
	params, err := fmParseParams(args["params"])
	if err != nil {
		return "", err
	}
	// Already on the board — the same type asked the same thing — is not a
	// reason to make it twice: the robot goes to the one there is, and says so.
	// 「中野坂上から銀座の乗り換え」 asked again walks to the route already
	// found rather than finding it again beside it.
	// What the type cannot run without, asked for before anything is made: a
	// small model often puts the question in the name (「新宿→横浜」) and
	// sends {} — which made a route with neither end, and an answer that said
	// it had searched.
	filled, err := s.fmFitParams(typ, params)
	if err != nil {
		return "", err
	}
	if missing, example := s.fmMissingParams(typ, params); len(missing) > 0 {
		return "", fmt.Errorf("%s needs %s in params, e.g. %s — nothing was made; call deploy_want again with them", typ, strings.Join(missing, " and "), example)
	}
	wants, err := s.fmWantList()
	if err != nil {
		return "", err
	}
	existing, archived := fmFindWant(wants, typ, params)
	if existing != "" && archived {
		// Put away, not gone: asked again, it is offered back rather than made
		// a second time beside the one in the archive — its card under the
		// question, so the person sees what would come back (its status says
		// archived).
		for _, w := range wants {
			if w.Metadata.Name == existing {
				args[fmCardArg] = fmCardValue("want", w.Metadata.ID, w.Metadata.Name)
			}
		}
		return fmt.Sprintf("%q (%s) with these parameters exists but is archived — put away, off the board. Nothing new was made. %s", existing, typ, fmAskRestore(existing)), nil
	}
	if existing != "" {
		pointed := map[string]string{"name": existing}
		if _, err := s.fmPoint(pointed); err == nil {
			if c, ok := pointed[fmCardArg]; ok {
				args[fmCardArg] = c
			}
			return fmt.Sprintf("%q (%s) with these parameters is already on the board; the robot is standing on it. Nothing new was made.", existing, typ), nil
		}
		return fmt.Sprintf("%q (%s) with these parameters is already on the board. Nothing new was made.", existing, typ), nil
	}
	name := strings.TrimSpace(args["name"])
	if name == "" {
		name = fmt.Sprintf("%s-%d", strings.ReplaceAll(typ, " ", "-"), time.Now().Unix()%100000)
	}
	// The name is the model's choice, and it often takes the type's own
	// ("transit_search") — which an archived want, out of sight, may hold. A
	// name taken is no reason to fail what was asked: another is found.
	name = fmFreeName(wants, name)
	want := map[string]any{
		"metadata": map[string]any{
			"name": name,
			"type": typ,
			// Next to whoever is at the controls, not in the next free cell
			// somewhere off screen.
			"labels": map[string]string{"mywant.io/canvas-near": "cursor"},
		},
		"spec": map[string]any{"params": params},
	}
	if err := s.backend("POST", "/api/v1/wants", want, nil); err != nil {
		return "", err
	}
	// Walk there, as for one already on the board, and show its card. Left
	// where it stood, the robot said 「ここに『荻窪→中野坂上』があります」 from
	// the station it had been asked about a question earlier. The new want is
	// placed beside the person when it is added (CanvasNearHook), which is a
	// moment after the POST returns: wait for it to be there with its cell.
	for i := 0; i < 20; i++ {
		if s.fmWantOnBoard(name) {
			pointed := map[string]string{"name": name}
			if walked, err := s.fmPoint(pointed); err == nil {
				if c, ok := pointed[fmCardArg]; ok {
					args[fmCardArg] = c
				}
				return fmt.Sprintf("Deployed %q (%s)%s, next to the person. %s", name, typ, filled, walked), nil
			}
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Sprintf("Deployed %q (%s)%s, next to the person. The robot has not walked to it: do not say it is here.", name, typ, filled), nil
}

// fmWantOnBoard: a want of this exact name is listed with a cell.
func (s *Server) fmWantOnBoard(name string) bool {
	wants, err := s.fmWantList()
	if err != nil {
		return false
	}
	for _, w := range wants {
		if w.Metadata.Name == name {
			_, x := w.Metadata.Labels[fmCanvasX]
			_, y := w.Metadata.Labels[fmCanvasY]
			return x && y
		}
	}
	return false
}

// fmMissingParams names the type's required parameters these params leave out
// or empty, with the params written out as the type's examples would fill them.
func (s *Server) fmMissingParams(typ string, params map[string]any) (missing []string, example string) {
	var def struct {
		Parameters []struct {
			Name     string `json:"name"`
			Required bool   `json:"required"`
			Default  any    `json:"default"`
			Example  any    `json:"example"`
		} `json:"parameters"`
	}
	if err := s.backend("GET", "/api/v1/want-types/"+typ, nil, &def); err != nil {
		return nil, ""
	}
	ex := map[string]any{}
	for _, p := range def.Parameters {
		if !p.Required {
			continue
		}
		if p.Example != nil {
			ex[p.Name] = p.Example
		}
		if v, ok := params[p.Name]; ok && v != nil && strings.TrimSpace(fmt.Sprint(v)) != "" {
			continue
		}
		if p.Default != nil && fmt.Sprint(p.Default) != "" {
			continue // the type fills it
		}
		missing = append(missing, p.Name)
	}
	return missing, fmPairs(ex)
}

// fmHereLabel names a want type's parameter that, left out, is where the
// person is — transit_search's from: 「戸塚への乗り換え」 starts here.
const fmHereLabel = "here-param"

// fmFitParams makes params what the type takes, by what each parameter is
// rather than by the type: a time (subType time) as HH:MM however it was
// written, a value outside a parameter's choices sent back with them, and the
// type's here-param, left out, filled with where the person is. Says what it
// filled, for the robot to tell.
func (s *Server) fmFitParams(typ string, params map[string]any) (string, error) {
	var def struct {
		Metadata struct {
			Labels map[string]string `json:"labels"`
		} `json:"metadata"`
		Parameters []struct {
			Name       string `json:"name"`
			SubType    string `json:"subType"`
			Validation struct {
				Enum []any `json:"enum"`
			} `json:"validation"`
		} `json:"parameters"`
	}
	if err := s.backend("GET", "/api/v1/want-types/"+typ, nil, &def); err != nil {
		return "", nil
	}
	said := func(name string) bool {
		v, ok := params[name]
		return ok && v != nil && strings.TrimSpace(fmt.Sprint(v)) != ""
	}
	filled := ""
	for _, p := range def.Parameters {
		if !said(p.Name) {
			if p.Name == def.Metadata.Labels[fmHereLabel] {
				here, err := s.fmHere(p.SubType)
				if err != nil {
					return "", fmt.Errorf("%s was left out and where the person is is not known (%v): ask the person — nothing was made", p.Name, err)
				}
				params[p.Name] = here
				filled = fmt.Sprintf(" (%s=%s: where the person is, as none was said)", p.Name, here)
			}
			continue
		}
		if p.SubType == "time" {
			hhmm, err := fmClock(params[p.Name])
			if err != nil {
				return "", fmt.Errorf("%s %v — nothing was made", p.Name, err)
			}
			params[p.Name] = hhmm
		}
		if len(p.Validation.Enum) > 0 {
			v, ok := fmt.Sprint(params[p.Name]), false
			var choices []string
			for _, c := range p.Validation.Enum {
				choices = append(choices, fmt.Sprint(c))
				ok = ok || fmt.Sprint(c) == v
			}
			if !ok {
				return "", fmt.Errorf("%s=%s is not one of %s — nothing was made; call deploy_want again with one of them", p.Name, v, strings.Join(choices, ", "))
			}
		}
	}
	return filled, nil
}

var (
	fmClockColon = regexp.MustCompile(`^(\d{1,2})[:：.](\d{2})$`)
	fmClockJa    = regexp.MustCompile(`(\d{1,2})\s*時\s*(?:(\d{1,2})\s*分|(半))?`)
	fmClockPM    = regexp.MustCompile(`午後|夜|夕方|(?i:pm)`)
)

// fmClock reads a time of day as a person or a model writes it — 09:40, 9:40,
// 9時40分, 9時半, 午後3時, 0940 (which params read as the number 940) — as HH:MM.
func fmClock(v any) (string, error) {
	raw := strings.TrimSpace(fmt.Sprint(v))
	if f, ok := v.(float64); ok {
		raw = fmt.Sprintf("%04d", int(f))
	}
	h, m := -1, 0
	switch {
	case fmClockColon.MatchString(raw):
		p := fmClockColon.FindStringSubmatch(raw)
		h, _ = strconv.Atoi(p[1])
		m, _ = strconv.Atoi(p[2])
	case fmClockJa.MatchString(raw):
		p := fmClockJa.FindStringSubmatch(raw)
		h, _ = strconv.Atoi(p[1])
		if p[2] != "" {
			m, _ = strconv.Atoi(p[2])
		} else if p[3] != "" {
			m = 30
		}
		if fmClockPM.MatchString(raw) && h < 12 {
			h += 12
		}
	case len(raw) == 4:
		if n, err := strconv.Atoi(raw); err == nil {
			h, m = n/100, n%100
		}
	}
	if h < 0 || h > 23 || m > 59 {
		return "", fmt.Errorf("%q is not a time of day; write it as HH:MM, e.g. 09:40", raw)
	}
	return fmt.Sprintf("%02d:%02d", h, m), nil
}

// fmNearestStationURL is HeartRails Express's nearest stations — what
// mywant-nearest-plugin reads for stations in Japan. A variable for the tests.
var fmNearestStationURL = "https://express.heartrails.com/api/json?method=getStations"

// fmHere is where the person is, as a parameter of this subType takes it: a
// station is the one nearest them, anything else their coordinates. Read from
// the location want that last heard from a device.
func (s *Server) fmHere(subType string) (string, error) {
	var resp struct {
		Wants []struct {
			Metadata struct {
				Type   string            `json:"type"`
				Labels map[string]string `json:"labels"`
			} `json:"metadata"`
			State struct {
				Current map[string]any `json:"current"`
			} `json:"state"`
		} `json:"wants"`
	}
	if err := s.backend("GET", "/api/v1/wants", nil, &resp); err != nil {
		return "", err
	}
	var lat, lng float64
	latest, found := "", false
	for _, w := range resp.Wants {
		if w.Metadata.Type != "location" || w.Metadata.Labels[fmArchived] == "true" {
			continue
		}
		la, ok1 := w.State.Current["lat"].(float64)
		ln, ok2 := w.State.Current["lng"].(float64)
		// RFC 3339 in UTC: the later sorts after.
		at, _ := w.State.Current["webhook_received_at"].(string)
		if ok1 && ok2 && (!found || at > latest) {
			lat, lng, latest, found = la, ln, at, true
		}
	}
	if !found {
		return "", fmt.Errorf("no location want knows where the person is")
	}
	if subType != "station" {
		return fmt.Sprintf("%.6f,%.6f", lat, lng), nil
	}
	return fmNearestStation(lat, lng)
}

func fmNearestStation(lat, lng float64) (string, error) {
	u := fmt.Sprintf("%s&x=%.6f&y=%.6f", fmNearestStationURL, lng, lat)
	res, err := (&http.Client{Timeout: 5 * time.Second}).Get(u)
	if err != nil {
		return "", fmt.Errorf("nearest station: %w", err)
	}
	defer res.Body.Close()
	var body struct {
		Response struct {
			Station []struct {
				Name string `json:"name"`
			} `json:"station"`
		} `json:"response"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("nearest station: %w", err)
	}
	// Nearest first, as HeartRails sorts them.
	if len(body.Response.Station) == 0 || body.Response.Station[0].Name == "" {
		return "", fmt.Errorf("no station near %.4f,%.4f", lat, lng)
	}
	return body.Response.Station[0].Name, nil
}

// fmPairs writes params the way deploy_want takes them: from=新宿, to=渋谷.
func fmPairs(params map[string]any) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = fmt.Sprintf("%s=%v", k, params[k])
	}
	return strings.Join(parts, ", ")
}

// fmParseParams reads deploy_want's params: name=value pairs, separated by
// commas (or 、, ;, new lines), name and value split at the first = or :.
//
// Not JSON, though JSON is still read: every argument reaches the model as a
// string, and a model writing a JSON object inside one has to escape each of
// its quotes. The Mac's did not — its {"from":" ended the string there, and it
// sent {"from": three times over and gave up. Pairs need no quotes at all.
func fmParseParams(raw string) (map[string]any, error) {
	raw = strings.TrimSpace(raw)
	params := map[string]any{}
	if raw == "" || raw == "{}" {
		return params, nil
	}
	if strings.HasPrefix(raw, "{") {
		if err := json.Unmarshal([]byte(raw), &params); err == nil {
			return params, nil
		}
		// Cut off, as above: the pairs say the same thing without quotes.
		inner := strings.Trim(raw, "{} ")
		if !strings.ContainsAny(inner, "=:") || strings.HasSuffix(strings.TrimSpace(inner), ":") {
			return nil, fmt.Errorf("params %q is cut off; write name=value pairs instead, e.g. from=新宿, to=渋谷", raw)
		}
		raw = inner
	}
	for _, part := range fmPairSep.Split(raw, -1) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		i := strings.IndexAny(part, "=:＝：")
		if i <= 0 {
			return nil, fmt.Errorf("params %q: %q is not name=value; write e.g. from=新宿, to=渋谷", raw, part)
		}
		_, size := utf8.DecodeRuneInString(part[i:])
		name := strings.Trim(strings.TrimSpace(part[:i]), `"' `)
		value := strings.Trim(strings.TrimSpace(part[i+size:]), `"' 「」`)
		if name == "" {
			return nil, fmt.Errorf("params %q: %q has no name", raw, part)
		}
		params[name] = fmParamValue(value)
	}
	return params, nil
}

var fmPairSep = regexp.MustCompile(`[,、;\n]`)

// fmParamValue: a number or true/false as one, anything else as text.
func fmParamValue(v string) any {
	if f, err := strconv.ParseFloat(v, 64); err == nil {
		return f
	}
	if b, err := strconv.ParseBool(v); err == nil && (v == "true" || v == "false") {
		return b
	}
	return v
}

// fmWant is a want as the robot's tools see it.
type fmWant struct {
	Metadata struct {
		ID     string            `json:"id"`
		Name   string            `json:"name"`
		Type   string            `json:"type"`
		Labels map[string]string `json:"labels"`
	} `json:"metadata"`
	Spec struct {
		Params map[string]any `json:"params"`
	} `json:"spec"`
}

func (w fmWant) archived() bool { return w.Metadata.Labels[fmArchived] == "true" }

func (s *Server) fmWantList() ([]fmWant, error) {
	var resp struct {
		Wants []fmWant `json:"wants"`
	}
	if err := s.backend("GET", "/api/v1/wants", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Wants, nil
}

// fmFindWant names a want of this type whose parameters already say what
// these say (every non-empty one equal, as text), or "" when there is none —
// one on the board before one in the archive.
func fmFindWant(wants []fmWant, typ string, params map[string]any) (name string, archived bool) {
	text := func(v any) string { return strings.TrimSpace(fmt.Sprint(v)) }
	asked := 0
	for _, v := range params {
		if v != nil && text(v) != "" {
			asked++
		}
	}
	if asked == 0 {
		return "", false
	}
	for _, w := range wants {
		if w.Metadata.Type != typ || w.Metadata.Name == fmRobotWant {
			continue
		}
		same := true
		for k, v := range params {
			if v == nil || text(v) == "" {
				continue
			}
			if got, ok := w.Spec.Params[k]; !ok || text(got) != text(v) {
				same = false
				break
			}
		}
		if !same {
			continue
		}
		if !w.archived() {
			return w.Metadata.Name, false
		}
		if name == "" {
			name, archived = w.Metadata.Name, true
		}
	}
	return name, archived
}

// fmFreeName is name, or name-2, name-3… — the first no want has.
func fmFreeName(wants []fmWant, name string) string {
	taken := map[string]bool{}
	for _, w := range wants {
		taken[w.Metadata.Name] = true
	}
	if !taken[name] {
		return name
	}
	for i := 2; ; i++ {
		if n := fmt.Sprintf("%s-%d", name, i); !taken[n] {
			return n
		}
	}
}

// fmAskRestore is what a tool that found an archived want tells the model.
func fmAskRestore(name string) string {
	return fmt.Sprintf("Ask the person whether to restore it; if they say yes, call restore_want with name %q.", name)
}

// fmArchivedNamed is the archived want called name (or loosely so), if any.
func (s *Server) fmArchivedNamed(name string) (fmWant, bool) {
	wants, err := s.fmWantList()
	if err != nil {
		return fmWant{}, false
	}
	for _, w := range wants {
		if w.archived() && (w.Metadata.Name == name || fmLooseName(w.Metadata.Name) == fmLooseName(name)) {
			return w, true
		}
	}
	return fmWant{}, false
}

func fmLooseName(v string) string {
	return strings.NewReplacer(" ", "", "-", "", "_", "").Replace(strings.ToLower(v))
}

// fmRestoreWant takes an archived want out of the archive — the label off,
// through PATCH as the GUI's restore does — and walks the robot to it.
func (s *Server) fmRestoreWant(args map[string]string) (string, error) {
	name := strings.TrimSpace(args["name"])
	if name == "" {
		return "", fmt.Errorf("name is required")
	}
	w, ok := s.fmArchivedNamed(name)
	if !ok {
		return "", fmt.Errorf("no archived want named %q", name)
	}
	patch := map[string]any{"metadata": map[string]any{"labels": map[string]any{fmArchived: nil}}}
	if err := s.backend("PATCH", "/api/v1/wants/"+w.Metadata.ID, patch, nil); err != nil {
		return "", fmt.Errorf("could not restore %q: %v", w.Metadata.Name, err)
	}
	out := fmt.Sprintf("Restored %q (%s); it is back on the board.", w.Metadata.Name, w.Metadata.Type)
	pointed := map[string]string{"name": w.Metadata.Name}
	if walked, err := s.fmPoint(pointed); err == nil {
		if c, ok := pointed[fmCardArg]; ok {
			args[fmCardArg] = c
		}
		out += " " + walked
	} else {
		args[fmCardArg] = fmCardValue("want", w.Metadata.ID, w.Metadata.Name)
	}
	return out, nil
}

const (
	fmRobotWant   = "robot"
	fmCanvasOn    = "mywant.io/canvas"
	fmCanvasX     = "mywant.io/canvas-x"
	fmCanvasY     = "mywant.io/canvas-y"
	fmArchived    = "mywant.io/archived"
	fmBoardLimit  = 50
	fmNearMatches = 6
)

// fmTile is one thing or want with a cell on the board.
type fmTile struct {
	name, kind string
	x, y       int
	// typ: a want's type ("" for a thing); aliases: what its type is also called.
	typ     string
	aliases []string
	// id and cardKind: what a chat shows its card by — a want's id or a
	// thing's, and which of the two it is.
	id       string
	cardKind string
}

// fmCardArg is the key under which a tool notes, in its own arguments, the
// want it brought the robot to; handleFMCall takes it out again and hands it
// back as the call's card — what the chat shows under the robot's words.
const fmCardArg = "_card"

// fmCard is a want a step brought the robot to.
type fmCard struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
	Name string `json:"name"`
	// What the card says, filled when the answer is settled (fmAnswerCards):
	// the same for every client that shows it — the GUI's chat, guiex, the
	// phone's chat, answer window and Live Activity — which each draw it their
	// own way.
	Type    string `json:"type,omitempty"`
	Status  string `json:"status,omitempty"`
	Summary string `json:"summary,omitempty"`
}

func (t fmTile) String() string { return fmt.Sprintf("%s (%s) at (%d, %d)", t.name, t.kind, t.x, t.y) }

// named is the tile without its cell, as board lists it: a model handed the
// cells reads them out ("荻窪 is at (6, 0)") rather than showing the place, so
// where a tile is comes only from point, which walks the robot there.
func (t fmTile) named() string {
	if len(t.aliases) > 0 {
		return fmt.Sprintf("%s (%s; also called %s)", t.name, t.kind, strings.Join(t.aliases, ", "))
	}
	return fmt.Sprintf("%s (%s)", t.name, t.kind)
}

// fmTiles is the board: things first, then wants, each only when it has a cell.
func (s *Server) fmTiles() ([]fmTile, error) {
	var things struct {
		Things []struct {
			ID      string            `json:"id"`
			Value   string            `json:"value"`
			Subtype string            `json:"subtype"`
			Catalog string            `json:"catalog"`
			Labels  map[string]string `json:"labels"`
		} `json:"things"`
	}
	if err := s.backend("GET", "/api/v1/things", nil, &things); err != nil {
		return nil, err
	}
	var wants struct {
		Wants []struct {
			Metadata struct {
				ID     string            `json:"id"`
				Name   string            `json:"name"`
				Type   string            `json:"type"`
				Labels map[string]string `json:"labels"`
			} `json:"metadata"`
		} `json:"wants"`
	}
	if err := s.backend("GET", "/api/v1/wants", nil, &wants); err != nil {
		return nil, err
	}
	aliases := s.fmTypeAliases()
	var tiles []fmTile
	for _, t := range things.Things {
		if t.Labels[fmCanvasOn] != "true" {
			continue
		}
		kind := t.Subtype
		if kind == "" {
			kind = t.Catalog
		}
		if tile, ok := fmTileAt(t.Value, kind, t.Labels); ok {
			tile.id, tile.cardKind = t.ID, "thing"
			tiles = append(tiles, tile)
		}
	}
	for _, w := range wants.Wants {
		// Archived: put away, off the board — as the GUI draws it.
		if w.Metadata.Name == fmRobotWant || w.Metadata.Labels[fmArchived] == "true" {
			continue
		}
		if tile, ok := fmTileAt(w.Metadata.Name, "want "+w.Metadata.Type, w.Metadata.Labels); ok {
			tile.id, tile.cardKind = w.Metadata.ID, "want"
			tile.typ = w.Metadata.Type
			tile.aliases = aliases[w.Metadata.Type]
			tiles = append(tiles, tile)
		}
	}
	return tiles, nil
}

func fmTileAt(name, kind string, labels map[string]string) (fmTile, bool) {
	xs, okX := labels[fmCanvasX]
	ys, okY := labels[fmCanvasY]
	if !okX || !okY {
		return fmTile{}, false
	}
	var x, y int
	if _, err := fmt.Sscan(xs, &x); err != nil {
		return fmTile{}, false
	}
	if _, err := fmt.Sscan(ys, &y); err != nil {
		return fmTile{}, false
	}
	return fmTile{name: name, kind: kind, x: x, y: y}, true
}

func (s *Server) fmBoard(map[string]string) (string, error) {
	tiles, err := s.fmTiles()
	if err != nil {
		return "", err
	}
	if len(tiles) == 0 {
		return "The board is empty.", nil
	}
	var b strings.Builder
	for i, t := range tiles {
		if i == fmBoardLimit {
			fmt.Fprintf(&b, "...and %d more\n", len(tiles)-fmBoardLimit)
			break
		}
		b.WriteString(t.named() + "\n")
	}
	return b.String(), nil
}

// fmPoint walks the robot onto the named tile — the same answer as the
// coordinates, pointed at rather than spelled out — and has it say where.
func (s *Server) fmPoint(args map[string]string) (string, error) {
	name := strings.TrimSpace(args["name"])
	if name == "" {
		return "", fmt.Errorf("name is required")
	}
	tiles, err := s.fmTiles()
	if err != nil {
		return "", err
	}
	loose := func(v string) string {
		return strings.NewReplacer(" ", "", "-", "", "_", "").Replace(strings.ToLower(v))
	}
	var found *fmTile
	var near []string
	for i, t := range tiles {
		if t.name == name || loose(t.name) == loose(name) {
			found = &tiles[i]
			break
		}
		if strings.Contains(loose(t.name), loose(name)) {
			near = append(near, t.name)
		}
	}
	if found == nil && len(near) == 1 {
		for i, t := range tiles {
			if t.name == near[0] {
				found = &tiles[i]
			}
		}
	}
	if found == nil && len(near) == 0 {
		// Not by its name — by what its type is also called (fmAliasLabel).
		typeOf := make([]string, len(tiles))
		for i, t := range tiles {
			typeOf[i] = t.typ
		}
		hits := fmByAlias(name, typeOf, s.fmTypeAliases())
		if len(hits) == 0 {
			// Nor by an alias — by a name its constellation puts beside it.
			names := make([]string, len(tiles))
			for i, t := range tiles {
				names[i] = t.name
			}
			hits = fmByGlossary(name, names, s.fmGlossary())
		}
		if len(hits) == 1 {
			found = &tiles[hits[0]]
		}
		for _, i := range hits {
			near = append(near, tiles[i].name)
		}
	}
	if found == nil {
		if w, ok := s.fmArchivedNamed(name); ok {
			args[fmCardArg] = fmCardValue("want", w.Metadata.ID, w.Metadata.Name)
			return fmt.Sprintf("%q (%s) is archived — put away, off the board, so there is nowhere to walk to. %s", w.Metadata.Name, w.Metadata.Type, fmAskRestore(w.Metadata.Name)), nil
		}
		if len(near) > fmNearMatches {
			near = near[:fmNearMatches]
		}
		if len(near) > 0 {
			return "", fmt.Errorf("%q is not on the board by that name; did you mean: %s", name, strings.Join(near, ", "))
		}
		// Not on the board is no answer by itself: 「戸塚への乗り換え」 stopped
		// here and read out another want instead.
		return "", fmt.Errorf("%q is not on the board — a want's parameter need not be: if it was asked for, deploy_want with it", name)
	}
	// x and y in one step: two would leave the robot a moment at one of them.
	walk := map[string]any{"metadata": map[string]any{"labels": map[string]string{
		fmCanvasX: fmt.Sprint(found.x), fmCanvasY: fmt.Sprint(found.y),
	}}}
	if err := s.backend("PATCH", "/api/v1/wants/"+fmRobotWant, walk, nil); err != nil {
		return "", fmt.Errorf("could not walk the robot there: %v", err)
	}
	if found.id != "" {
		args[fmCardArg] = fmCardValue(found.cardKind, found.id, found.name)
	}
	words := fmt.Sprintf("「%s」はここです (%d, %d)", found.name, found.x, found.y)
	if err := s.fmSay(words); err != nil {
		return "", err
	}
	return fmt.Sprintf("The robot is standing on %s and saying: %s", found, words), nil
}

// fmLook reads one want: its status and the state it has gathered, short.
//
// Only what says something — not the bookkeeping every want carries
// (achieving_percentage, action_by_agent…), not empty values, not the chat
// buffers, and never a value whose name says it is a secret: what this
// returns is read by a model and may be read out.
func (s *Server) fmLook(args map[string]string) (string, error) {
	name := strings.TrimSpace(args["name"])
	if name == "" {
		return "", fmt.Errorf("name is required")
	}
	var resp struct {
		Wants []struct {
			Metadata struct {
				ID     string            `json:"id"`
				Name   string            `json:"name"`
				Type   string            `json:"type"`
				Labels map[string]string `json:"labels"`
			} `json:"metadata"`
			Status string `json:"status"`
			State  struct {
				Current     map[string]any `json:"current"`
				FinalResult any            `json:"final_result"`
			} `json:"state"`
		} `json:"wants"`
	}
	if err := s.backend("GET", "/api/v1/wants", nil, &resp); err != nil {
		return "", err
	}
	// The wants on the board before the archived ones: a name both have is the
	// one in sight.
	sort.SliceStable(resp.Wants, func(i, j int) bool {
		return resp.Wants[i].Metadata.Labels[fmArchived] != "true" && resp.Wants[j].Metadata.Labels[fmArchived] == "true"
	})
	loose := fmLooseName
	found, near := -1, []string{}
	for i, w := range resp.Wants {
		if w.Metadata.Name == name || loose(w.Metadata.Name) == loose(name) {
			found = i
			break
		}
		if strings.Contains(loose(w.Metadata.Name), loose(name)) {
			near = append(near, w.Metadata.Name)
		}
	}
	if found < 0 && len(near) == 1 {
		for i, w := range resp.Wants {
			if w.Metadata.Name == near[0] {
				found = i
			}
		}
	}
	if found < 0 && len(near) == 0 {
		// Not by its name — by what its type is also called (fmAliasLabel).
		typeOf := make([]string, len(resp.Wants))
		for i, w := range resp.Wants {
			typeOf[i] = w.Metadata.Type
		}
		hits := fmByAlias(name, typeOf, s.fmTypeAliases())
		if len(hits) == 0 {
			// Nor by an alias — by a name its constellation puts beside it.
			names := make([]string, len(resp.Wants))
			for i, w := range resp.Wants {
				names[i] = w.Metadata.Name
			}
			hits = fmByGlossary(name, names, s.fmGlossary())
		}
		if len(hits) == 1 {
			found = hits[0]
		}
		for _, i := range hits {
			near = append(near, resp.Wants[i].Metadata.Name)
		}
	}
	if found < 0 {
		if len(near) > fmNearMatches {
			near = near[:fmNearMatches]
		}
		if len(near) > 0 {
			return "", fmt.Errorf("no want named %q; did you mean: %s", name, strings.Join(near, ", "))
		}
		return "", fmt.Errorf("no want named %q; use board to see the names", name)
	}
	w := resp.Wants[found]
	// The want it read, shown as its card under the answer: what the answer
	// is about, at a glance — a person who asked about 国分寺 sees the card
	// says Nakano. Archived, it still has its card (its status says so), what
	// it holds is still read, and the person is asked whether to bring it back.
	if w.Metadata.ID != "" {
		args[fmCardArg] = fmCardValue("want", w.Metadata.ID, w.Metadata.Name)
	}
	note := ""
	if w.Metadata.Labels[fmArchived] == "true" {
		note = fmt.Sprintf("%q is archived — put away, off the board. %s\n", w.Metadata.Name, fmAskRestore(w.Metadata.Name))
	}

	// Everything there is about it — the whole type definition (what it is,
	// what each of its values means, how it is made) and the whole want — cut
	// to what the smallest model can take (fmLookFull). A want that cannot be
	// cut to fit falls back to the short form below.
	if full := s.fmLookFull(w.Metadata.Name, w.Metadata.Type); full != "" {
		return note + full, nil
	}
	// What the type says it is and what each of its values means — the want
	// carries only the values, and a value's key ("next_store") is all a model
	// had to go on. Written once in the type's YAML (metadata.description, each
	// state's description), read here; best effort, the values stand alone.
	var def struct {
		Metadata struct {
			Description string `json:"description"`
		} `json:"metadata"`
		State []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"state"`
	}
	_ = s.backend("GET", "/api/v1/want-types/"+w.Metadata.Type, nil, &def)
	meaning := map[string]string{}
	var order []string
	for _, st := range def.State {
		meaning[st.Name] = fmFirstSentence(st.Description, 90)
		order = append(order, st.Name)
	}

	var b strings.Builder
	b.WriteString(note)
	fmt.Fprintf(&b, "%s (%s): %s\n", w.Metadata.Name, w.Metadata.Type, w.Status)
	if about := fmFirstSentence(def.Metadata.Description, 160); about != "" {
		fmt.Fprintf(&b, "What it is: %s\n", about)
	}
	// In the order the type lists them — the author's order, not the alphabet's
	// — then anything the type does not declare.
	keys := make([]string, 0, len(w.State.Current))
	seen := map[string]bool{}
	for _, k := range order {
		if _, ok := w.State.Current[k]; ok && !seen[k] {
			keys, seen[k] = append(keys, k), true
		}
	}
	var rest []string
	for k := range w.State.Current {
		if !seen[k] {
			rest = append(rest, k)
		}
	}
	sort.Strings(rest)
	keys = append(keys, rest...)
	// Each value once. A want commonly keeps the same answer under several
	// keys (the reservation as current_reservation, as reservations[0], in the
	// raw output, as the final result), and a small model handed it four times
	// over loses the thread before it reaches the end.
	said := map[string]bool{}
	for _, k := range keys {
		if fmLookSkip(k) {
			continue
		}
		v := w.State.Current[k]
		text := fmt.Sprint(v)
		if _, isText := v.(string); !isText {
			raw, _ := json.Marshal(v)
			text = string(raw)
		}
		if text == "" || text == "[]" || text == "{}" || text == "null" || text == "0" || text == "false" {
			continue
		}
		if said[text] || said["["+text+"]"] {
			continue
		}
		said[text] = true
		if m := meaning[k]; m != "" {
			fmt.Fprintf(&b, "- %s: %s  (%s)\n", k, firstLine(text, 160), m)
		} else {
			fmt.Fprintf(&b, "- %s: %s\n", k, firstLine(text, 160))
		}
		if b.Len() > 1500 {
			b.WriteString("…\n")
			break
		}
	}
	if fr := fmt.Sprint(w.State.FinalResult); w.State.FinalResult != nil && fr != "" && fr != "map[]" {
		raw, _ := json.Marshal(w.State.FinalResult)
		if !said[string(raw)] && !said["["+string(raw)+"]"] {
			fmt.Fprintf(&b, "- final result: %s\n", firstLine(string(raw), 200))
		}
	}
	return b.String(), nil
}

// fmLookTokens is what look may hand the model, in tokens.
//
// Sized for the smallest window that asks: an iPhone's model has 4,096 tokens
// (a Mac's 8,192) for the instructions and the tool schemas — about 2,000 of
// them together — the question, this answer and the reply. A smartgolf want
// and its type handed over whole came to 5,327 tokens there and the turn
// failed outright.
const fmLookTokens = 1500

// fmLookFull is one want with its type's whole definition, as JSON, cut to
// fmLookTokens by the same token budget every GET can ask for (token_budget.go):
// its history first, then bookkeeping, display hints, the framework's shared
// fields, how the type is made, and long values — what it is and what its
// values mean go last. "" when it cannot be read or cut to fit.
//
// Secrets stay out here too: a state or label whose name says it is one is
// dropped, as fmLookSkip drops it from the short form.
func (s *Server) fmLookFull(name, typ string) string {
	var typeDef map[string]any
	if err := s.backend("GET", "/api/v1/want-types/"+typ, nil, &typeDef); err != nil {
		return ""
	}
	var all struct {
		Wants []map[string]any `json:"wants"`
	}
	if err := s.backend("GET", "/api/v1/wants", nil, &all); err != nil {
		return ""
	}
	var want map[string]any
	for _, x := range all.Wants {
		if md, _ := x["metadata"].(map[string]any); md != nil && md["name"] == name {
			want = x
		}
	}
	if want == nil {
		return ""
	}
	fmDropSecrets(want)
	fmDropSecrets(typeDef)
	cut, res := budgetFit(map[string]any{"type": typeDef, "want": want}, fmLookTokens)
	if !res.Fits {
		return ""
	}
	doc := cut.(map[string]any)
	td, _ := json.Marshal(doc["type"])
	wj, _ := json.Marshal(doc["want"])
	return "Type definition:\n" + string(td) + "\n\nWant:\n" + string(wj)
}

// fmDropSecrets removes, at any depth, every map entry whose key names a
// secret (see fmLookSecret) — or, in a state definition's list, every entry
// whose "name" does.
func fmDropSecrets(v any) {
	switch x := v.(type) {
	case map[string]any:
		for k, child := range x {
			if fmLookSecret(k) {
				delete(x, k)
				continue
			}
			fmDropSecrets(child)
		}
	case []any:
		for i, child := range x {
			if m, ok := child.(map[string]any); ok {
				if n, _ := m["name"].(string); n != "" && fmLookSecret(n) {
					x[i] = map[string]any{"name": n, "omitted": "secret"}
					continue
				}
			}
			fmDropSecrets(child)
		}
	}
}

// fmLookSecret: a key whose name says it holds a secret.
func fmLookSecret(key string) bool {
	lower := strings.ToLower(key)
	for _, secret := range []string{"key", "token", "secret", "password", "credential", "auth"} {
		if strings.Contains(lower, secret) {
			return true
		}
	}
	return false
}

// fmLookSkip: the state every want keeps for its own running, and secrets.
func fmLookSkip(key string) bool {
	switch key {
	case "achieved", "achieving_percentage", "action_by_agent", "completed", "say", "skill_path":
		return true
	}
	lower := strings.ToLower(key)
	// The same moment twice (a machine form beside the readable one), and the
	// script's raw output beside the fields made from it.
	if strings.HasSuffix(lower, "_rfc3339") || strings.Contains(lower, "raw_output") {
		return true
	}
	if strings.HasPrefix(lower, "cc_") || strings.HasPrefix(lower, "webhook_") || strings.HasPrefix(lower, "fm_") {
		return true
	}
	return fmLookSecret(lower)
}

// fmSay puts words in the robot's speech bubble on the board.
func (s *Server) fmSay(words string) error {
	words = strings.TrimSpace(words)
	if words == "" {
		return nil
	}
	return s.backend("PUT", "/api/v1/states/"+fmRobotWant, map[string]any{"say": words}, nil)
}

// fmFirstSentence is a description cut to its first sentence, on one line —
// a type's YAML wraps its descriptions across lines and goes on to say more
// than a model needs to read a value by.
func fmFirstSentence(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	// "e.g. 北新宿店" is not the end of a sentence; its example is the useful half.
	guard := strings.NewReplacer("e.g. ", "e.g.\x00", "i.e. ", "i.e.\x00")
	s = guard.Replace(s)
	for _, end := range []string{". ", "。"} {
		if i := strings.Index(s, end); i >= 0 {
			s = s[:i+len(strings.TrimSpace(end))]
		}
	}
	s = strings.ReplaceAll(s, "\x00", " ")
	if r := []rune(s); len(r) > max {
		s = string(r[:max]) + "…"
	}
	return s
}

func firstLine(s string, max int) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if r := []rune(s); len(r) > max {
		s = string(r[:max]) + "…"
	}
	return s
}
