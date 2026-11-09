package server

import (
	ws "github.com/onelittlenightmusic/want-spec"

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

	mywant "mywant/engine/core"
)

// The robot, for an on-device model elsewhere (Apple FoundationModels on an
// iPhone).
//
// The model runs on the device; who it is and everything it reaches for come
// from here. The app fetches /api/v1/tools — the robot's instructions
// and each tool's name, description and string arguments — builds its tools
// from that at runtime, and sends every call back to /api/v1/tools/call — each
// as a step of a turn (turns.go): the question, what the robot did, and
// what it answered, kept here whole so the robot on the board has done it. So the
// app holds no knowledge of MyWant at all: the frame is this file, and
// changing the robot is a change here, not a new build on the phone.
//
// Ported from the Mac's own robot (fmtool/), less what only a Mac has — the filesystem, the CLI — and in the same spirit: a guide shows
// rather than recites, so "荻窪はどこ？" walks the robot to 荻窪.
//
// Answers are short on purpose. The model has 8k tokens for the instructions,
// the tool schemas and the whole talk; the board dumped raw would fill it.

type robotTool struct {
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Arguments   []robotParam `json:"arguments"`
	// Writes: the call changes the board. A device that shows the board
	// (the iPhone app's web view) says "reflecting…" until it reads it back.
	Writes bool `json:"writes,omitempty"`
	run    func(s *Server, args map[string]string) (string, error)
	// replays: run again when a turn is played again (turns.go) — the
	// steps that show something. Not the ones that make something, which a
	// second run would make twice, nor the ones that only read.
	replays bool
}

// robotParam is one argument. Every argument is a string: the device builds its
// schema from these, and one kind keeps that side trivial. A structured value
// (a want's params) travels as name=value pairs (robotParseParams).
type robotParam struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
	// Choices, when set, are the only values the device lets the model
	// produce: it becomes an enum in the generation schema, so a type that
	// does not exist cannot be written at all. Asking a model this small to
	// look a name up first does not work — it guesses ("sticky_note").
	Choices []string `json:"choices,omitempty"`
}

const robotInstructions = `You are the robot on the MyWant board, and a guide: a guide does not recite, a guide shows. The person talks to you from their phone; you stand on the board on their computer, and what you do appears there.
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

var robotTools = []robotTool{
	{
		Name:        "board",
		Description: "The names of everything on the board, things and wants, with their kinds. To say where one is, use point.",
		run:         (*Server).robotBoard,
	},
	{
		Name:        "point",
		Description: "For WHERE questions only: walks the robot onto a thing or want so the person sees where it is.",
		Arguments: []robotParam{
			{Name: "name", Description: "The name alone, e.g. 荻窪 or note-instance", Required: true},
		},
		run:     (*Server).robotPoint,
		replays: true,
	},
	{
		Name:        "look",
		Description: "For WHAT questions: what a want on the board says and how it is doing — the weather a weather want fetched, a timer's time left, its status.",
		Arguments: []robotParam{
			{Name: "name", Description: "The want's name or part of it, e.g. Nakano for NakanoのWeather", Required: true},
		},
		run: (*Server).robotLook,
	},
	{
		Name:        "use_capability",
		Description: "Ask a want for one of its capabilities listed in your instructions: what every want can do (restart: run it again for what it finds now), or what one can look up beyond what it shows (e.g. the latest golf score from a golf want's photos).",
		Arguments: []robotParam{
			{Name: "capability", Description: "The capability, exactly as listed: restart, or want#capability", Required: true},
			{Name: "want", Description: "For one every want has (restart): the want's name, e.g. smartgolf. Empty for want#capability.", Required: false},
		},
		Writes: true,
		run:    (*Server).useCapability,
	},
	{
		Name:        "describe_type",
		Description: "Describe one want type and the parameters it takes. Use before deploy_want.",
		Arguments: []robotParam{
			{Name: "type", Description: "The type name", Required: true},
		},
		run: (*Server).robotDescribeType,
	},
	{
		Name:        "deploy_want",
		Description: "Put a new want on the board, next to the person — or, if one of that type with the same parameters is already there, walk to it instead.",
		Arguments: []robotParam{
			{Name: "type", Description: "The type of the want", Required: true},
			{Name: "params", Description: "The parameters as name=value pairs separated by commas, e.g. from=新宿, to=渋谷 — no quotes, no braces. Empty when there are none.", Required: true},
			{Name: "name", Description: "A short name for the want, lowercase with hyphens", Required: false},
		},
		run:    (*Server).robotDeployWant,
		Writes: true,
	},
	{
		Name:        "restore_want",
		Description: "Bring an archived want back onto the board — only after the person said yes to restoring it.",
		Arguments: []robotParam{
			{Name: "name", Description: "The archived want's name, as the tool that found it gave it", Required: true},
		},
		run:    (*Server).robotRestoreWant,
		Writes: true,
	},
}

func (s *Server) handleToolsList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// The type names are filled in per request, from the backend's list as it
	// is now: a type registered a minute ago is choosable without a rebuild.
	types, err := s.robotTypeNames()
	if err != nil {
		http.Error(w, "backend: "+err.Error(), http.StatusBadGateway)
		return
	}
	// The board's capabilities as they are now (tools_capability.go); with none,
	// there is nothing to use and the tool is not offered.
	caps := s.capabilityChoices()
	tools := make([]robotTool, 0, len(robotTools))
	for _, t := range robotTools {
		t.Arguments = append([]robotParam(nil), t.Arguments...)
		for j, a := range t.Arguments {
			if a.Name == "type" && t.Name == "deploy_want" {
				a.Choices = types
				t.Arguments[j] = a
			}
			if a.Name == "capability" && t.Name == "use_capability" {
				a.Choices = caps
				t.Arguments[j] = a
			}
		}
		tools = append(tools, t)
	}
	robotWriteJSON(w, map[string]any{"instructions": robotInstructions + s.robotHintsText() + s.capabilitiesText() + s.robotGlossaryText() + robotLocaleText(r.URL.Query().Get("locale")), "tools": tools})
}

// robotLanguages names the languages an on-device model may be set to, for the
// line that tells it which one to answer in.
var robotLanguages = map[string]string{
	"ja": "Japanese", "en": "English", "zh": "Chinese", "ko": "Korean",
	"fr": "French", "de": "German", "es": "Spanish", "it": "Italian",
	"pt": "Portuguese", "nl": "Dutch", "sv": "Swedish", "da": "Danish",
	"nb": "Norwegian", "tr": "Turkish", "vi": "Vietnamese",
}

// robotLocaleText is the line that says which language to answer in, for the
// device's locale (ja_JP, en-US) — sent only when the device's model speaks
// it. The instructions are in English, and a model set to Japanese read them
// and answered in English, "the language the person used" notwithstanding;
// naming the language outright is what keeps it. Nothing for no locale.
func robotLocaleText(locale string) string {
	if locale == "" {
		return ""
	}
	code := locale
	if i := strings.IndexAny(locale, "_-"); i >= 0 {
		code = locale[:i]
	}
	code = strings.ToLower(code)
	name, ok := robotLanguages[code]
	if !ok {
		name = "the language of the locale " + locale
	}
	return fmt.Sprintf("\n\nThe person's locale is %s: answer in %s, though these instructions are in English.", locale, name)
}

func (s *Server) handleToolsCall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Tool      string            `json:"tool"`
		Arguments map[string]string `json:"arguments"`
		// The turn this call is a step of (turns.go); recorded there and
		// shown in the robot's chat. Without one the call is only run.
		Turn string `json:"turn"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}
	if robotToolByName(req.Tool).Name == "" {
		http.Error(w, "unknown tool: "+req.Tool, http.StatusNotFound)
		return
	}
	if req.Arguments == nil {
		req.Arguments = map[string]string{}
	}
	if req.Turn != "" {
		if prev := s.robotRepeatedStep(req.Turn, req.Tool, req.Arguments); prev != nil {
			// The same call again in one turn: not run again, and the model
			// told so — what it got the first time, and to answer now. A small
			// model went round describe_type / deploy_want five times after
			// the want was made, ~40s a round on a phone (fly, 2026-11-02:
			// 新宿→国分寺 took four minutes for a six-second search).
			step := robotStep{Tool: req.Tool, Arguments: req.Arguments, Output: robotRepeatOutput(*prev), Card: prev.Card, Done: true, At: time.Now().Format(time.RFC3339)}
			_, _ = s.robotUpdateTurn(req.Turn, func(t *robotTurn) error { t.Steps = append(t.Steps, step); return nil })
			robotWriteJSON(w, map[string]any{"output": step.Output, "card": step.Card})
			return
		}
	}
	if req.Turn != "" && robotHearsQuestion(req.Tool) {
		// What the person asked, for look to see which of a want's answers
		// it is about (robotLookReferent) and for deploy_want to set each
		// parameter from the words that mark it (robotMarkedParams) — taken out
		// again before the step is kept.
		if q := s.turnQuestion(req.Turn); q != "" {
			req.Arguments[robotQuestionArg] = q
		}
	}
	step := robotStep{Tool: req.Tool, Arguments: req.Arguments}
	if req.Turn == "" {
		// Outside a turn it is only run: nobody asked the robot anything, so
		// its chat has nothing to show.
		out := s.robotRunTool(req.Tool, req.Arguments)
		robotWriteJSON(w, map[string]any{"output": out, "card": robotTakeCard(req.Arguments), "unread_photos": robotTakeUnread(req.Arguments)})
		return
	}
	s.robotRunStep(&step)
	step.Card = robotTakeCard(step.Arguments)
	unread := robotTakeUnread(step.Arguments)
	turn, err := s.robotUpdateTurn(req.Turn, func(t *robotTurn) error {
		t.Steps = append(t.Steps, step)
		return nil
	})
	if err != nil {
		log.Printf("[fm] step outside a known turn: %v", err)
	} else if !turn.Quiet {
		s.robotShowStep(step, "")
	}
	robotWriteJSON(w, map[string]any{"output": step.Output, "card": step.Card, "unread_photos": unread})
}

// robotTakeCard takes out of a step's arguments the want its tool brought the
// robot to (robotCardArg), if any.
func robotTakeCard(args map[string]string) *robotCard {
	c, ok := args[robotCardArg]
	if !ok {
		return nil
	}
	delete(args, robotCardArg)
	parts := strings.SplitN(c, "\x00", 3)
	card := &robotCard{Kind: "want", ID: parts[0]}
	if len(parts) > 1 {
		card.Name = parts[1]
	}
	if len(parts) > 2 && parts[2] != "" {
		card.Kind = parts[2]
	}
	return card
}

// robotCardValue is how a tool notes the want or thing it brought the robot to
// (robotCardArg): id, name and kind, for robotTakeCard to read back.
func robotCardValue(kind, id, name string) string {
	return id + "\x00" + name + "\x00" + kind
}

// robotToolByName is the tool of that name, or the zero tool.
func robotToolByName(name string) robotTool {
	for _, t := range robotTools {
		if t.Name == name {
			return t
		}
	}
	return robotTool{}
}

// robotRunTool runs a tool and hands back its output. A failure is an answer
// too: the model reads it and can try again, so it comes back as output.
func (s *Server) robotRunTool(name string, args map[string]string) string {
	t := robotToolByName(name)
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

func robotWriteJSON(w http.ResponseWriter, v any) {
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

type robotWantType struct {
	Name       string            `json:"name"`
	Title      string            `json:"title"`
	Category   string            `json:"category"`
	SystemType bool              `json:"system_type"`
	Labels     map[string]string `json:"labels"`
}

// robotAliasLabel is a want type's other names: what a person calls it rather
// than what it was named — "スマートゴルフ,ゴルフ" for smartgolf_check_reserved.
// A model this small cannot be relied on to turn スマートゴルフ into smartgolf,
// so the type says it.
const robotAliasLabel = "aliases"

// robotTypeAliases maps each type to its aliases (robotAliasLabel). Empty on a
// failure: aliases only ever help a lookup, never stop one.
func (s *Server) robotTypeAliases() map[string][]string {
	types, err := s.robotWantTypes()
	if err != nil {
		return nil
	}
	out := map[string][]string{}
	for _, t := range types {
		for _, a := range strings.Split(t.Labels[robotAliasLabel], ",") {
			if a = strings.TrimSpace(a); a != "" {
				out[t.Name] = append(out[t.Name], a)
			}
		}
	}
	return out
}

// robotHintLabel is a want type's word to the robot: how a question for it is
// handled, in the type's own YAML rather than in this file — transit_search's
// "a route: deploy at once, the stations need not be on the board, a time goes
// in as time=09:40, arrive_type=到着". The type knows its questions; the robot
// only reads what it says.
const robotHintLabel = "robot-hint"

// robotQALabel prefixes a want type's worked examples for the robot, one label
// each: the request after the slash, what should be done or said for it as the
// value — "robot-qa/9時40分までに戸塚に移動できる乗り換え": "deploy_want
// transit_search with params to=戸塚, time=09:40, arrive_type=到着". A small
// model follows an example more surely than a rule.
const robotQALabel = "robot-qa/"

// robotHintsMax keeps the hints, with the glossary, inside the phone's window.
const robotHintsMax = 2000

// robotTypeHint is what a type tells the robot, as the instructions and
// describe_type carry it: its hint, then its examples, each 「request」 → what
// to do. Empty when it says nothing.
func robotTypeHint(labels map[string]string) string {
	var b strings.Builder
	b.WriteString(strings.Join(strings.Fields(labels[robotHintLabel]), " "))
	// Labels are a map: in the requests' order, so the text is the same each time.
	var asks []string
	for k := range labels {
		if strings.HasPrefix(k, robotQALabel) {
			asks = append(asks, k)
		}
	}
	sort.Strings(asks)
	for _, k := range asks {
		q, a := strings.TrimSpace(strings.TrimPrefix(k, robotQALabel)), strings.Join(strings.Fields(labels[k]), " ")
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

// robotHintsText is every type's hint as the instructions carry it.
func (s *Server) robotHintsText() string {
	types, err := s.robotWantTypes()
	if err != nil {
		return ""
	}
	var b strings.Builder
	for _, t := range types {
		h := robotTypeHint(t.Labels)
		if h == "" {
			continue
		}
		row := fmt.Sprintf("- %s: %s\n", t.Name, h)
		if b.Len()+len(row) > robotHintsMax {
			break
		}
		b.WriteString(row)
	}
	if b.Len() == 0 {
		return ""
	}
	return "\n\nWhat some types say about their questions:\n" + b.String()
}

func robotLoose(v string) string {
	return strings.NewReplacer(" ", "", "　", "", "-", "", "_", "").Replace(strings.ToLower(strings.TrimSpace(v)))
}

// robotAliasScore is how well what the person said names a type by its aliases:
// the length of the longest alias it is or contains, 0 for none. Longest wins,
// so 「スマートゴルフの空き」 picks the type called that over the one called
// スマートゴルフ.
func robotAliasScore(said string, aliases []string) int {
	l, best := robotLoose(said), 0
	for _, a := range aliases {
		la := robotLoose(a)
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

// robotGlossary is one line per constellation: its name, then its members' names.
func (s *Server) robotGlossary() [][]string {
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

// robotGlossaryMax keeps the glossary from crowding the window it is meant to
// help with (4,096 tokens on an iPhone): past it, the rest is left out.
const robotGlossaryMax = 800

// robotGlossaryText is the glossary as the instructions carry it.
func (s *Server) robotGlossaryText() string {
	lines := s.robotGlossary()
	if len(lines) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\nNames on this board that go together (its constellations), one group a line — a name in a line also means the others, so 「中野の天気」 is about a want named after any name in 中野's line:\n")
	for _, l := range lines {
		row := strings.Join(l, " = ") + "\n"
		if b.Len()+len(row) > robotGlossaryMax {
			break
		}
		b.WriteString(row)
	}
	return b.String()
}

// robotByGlossary picks the names a person meant through the glossary: every
// line holding a word of what they said, and of the names, the ones the most
// of those lines name. 「中野の天気」 reaches 中野's line (nakano) and 天気's
// (NakanoのWeather): the want both point at wins.
func robotByGlossary(said string, names []string, glossary [][]string) []int {
	l := robotLoose(said)
	var lines [][]string
	for _, line := range glossary {
		for _, word := range line {
			if w := robotLoose(word); w != "" && strings.Contains(l, w) {
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
		ln, score := robotLoose(n), 0
		for _, line := range lines {
			for _, word := range line {
				if w := robotLoose(word); w != "" && (ln == w || strings.Contains(ln, w)) {
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

// robotByAlias picks, from the types of the wants on the board, the ones the
// person named by an alias: all of those with the best score.
func robotByAlias(said string, typeOf []string, aliases map[string][]string) []int {
	best, out := 0, []int(nil)
	for i, t := range typeOf {
		sc := robotAliasScore(said, aliases[t])
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

func (s *Server) robotWantTypes() ([]robotWantType, error) {
	var resp struct {
		WantTypes []robotWantType `json:"wantTypes"`
	}
	if err := s.backend("GET", "/api/v1/want-types", nil, &resp); err != nil {
		return nil, err
	}
	var out []robotWantType
	for _, t := range resp.WantTypes {
		if !t.SystemType {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (s *Server) robotTypeNames() ([]string, error) {
	types, err := s.robotWantTypes()
	if err != nil {
		return nil, err
	}
	names := make([]string, len(types))
	for i, t := range types {
		names[i] = t.Name
	}
	return names, nil
}

// robotResolveType finds the type the model meant: exact, then ignoring case
// and the difference between spaces, hyphens and underscores, then by title.
// On a miss the error names the closest few, so the next try can succeed.
func (s *Server) robotResolveType(name string) (string, error) {
	norm := func(v string) string {
		return strings.NewReplacer(" ", "", "-", "", "_", "").Replace(strings.ToLower(strings.TrimSpace(v)))
	}
	types, err := s.robotWantTypes()
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

func (s *Server) robotDescribeType(args map[string]string) (string, error) {
	name, err := s.robotResolveType(args["type"])
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
	if h := robotTypeHint(resp.Metadata.Labels); h != "" {
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
	// The phone's model read this and then said 「検索します」 without
	// searching: the step after is said here, where it reads it last.
	fmt.Fprintf(&b, "Next: call deploy_want with type %s now. Do not tell the person you will — deploy it, then answer.\n", name)
	return b.String(), nil
}

func (s *Server) robotDeployWant(args map[string]string) (string, error) {
	question := args[robotQuestionArg]
	delete(args, robotQuestionArg)
	typ, err := s.robotResolveType(args["type"])
	if err != nil {
		return "", err
	}
	params, err := robotParseParams(args["params"])
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
	filled, err := s.robotFitParams(typ, params, question)
	if err != nil {
		return "", err
	}
	if missing, example := s.robotMissingParams(typ, params); len(missing) > 0 {
		return "", fmt.Errorf("%s needs %s in params, e.g. %s — nothing was made; call deploy_want again with them", typ, strings.Join(missing, " and "), example)
	}
	wants, err := s.robotWantList()
	if err != nil {
		return "", err
	}
	existing, archived := robotFindWant(wants, typ, params)
	// A question asked again — in the archive or still on the board — is
	// asked again: run now, not answered from when it was last asked (a
	// route an hour old gives an hour-old train).
	if existing != "" && s.robotTypeLabel(typ, robotAskAgainLabel) == "run" {
		// A question put away and asked again — 「中野坂上から国分寺への乗り換
		// え」 a second time — is asked again: the one in the archive is brought
		// back and run with what was asked now, not offered back to be agreed to.
		for _, w := range wants {
			if w.Metadata.Name == existing {
				return s.robotRunAgain(w, params, filled, args)
			}
		}
	}
	if existing != "" && archived {
		// Put away, not gone: asked again, it is offered back rather than made
		// a second time beside the one in the archive — its card under the
		// question, so the person sees what would come back (its status says
		// archived).
		for _, w := range wants {
			if w.Metadata.Name == existing {
				args[robotCardArg] = robotCardValue("want", w.Metadata.ID, w.Metadata.Name)
			}
		}
		return fmt.Sprintf("%q (%s) with these parameters exists but is archived — put away, off the board. Nothing new was made. %s", existing, typ, robotAskRestore(existing)), nil
	}
	if existing != "" {
		pointed := map[string]string{"name": existing}
		for _, w := range wants {
			if w.Metadata.Name == existing {
				pointed["id"] = w.Metadata.ID
			}
		}
		if _, err := s.robotPoint(pointed); err == nil {
			if c, ok := pointed[robotCardArg]; ok {
				args[robotCardArg] = c
			}
			return fmt.Sprintf("%q (%s, %s) with these parameters is already on the board; the robot is standing on it. Nothing new was made.", existing, typ, robotParamsText(params)), nil
		}
		return fmt.Sprintf("%q (%s, %s) with these parameters is already on the board. Nothing new was made.", existing, typ, robotParamsText(params)), nil
	}
	name := strings.TrimSpace(args["name"])
	// The type's own naming (robot-name, e.g. "{from}→{to}") over the model's:
	// a route named after where it goes (「戸塚」) is the station's name too.
	if named := robotNameFrom(s.robotTypeLabel(typ, robotNameLabel), params); named != "" {
		name = named
	}
	if name == "" {
		name = fmt.Sprintf("%s-%d", strings.ReplaceAll(typ, " ", "-"), time.Now().Unix()%100000)
	}
	// The name is the model's choice, and it often takes the type's own
	// ("transit_search") — which an archived want, out of sight, may hold. A
	// name taken is no reason to fail what was asked: another is found.
	name = robotFreeName(wants, name)
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
	var created struct {
		WantIDs []string `json:"want_ids"`
	}
	if err := s.backend("POST", "/api/v1/wants", want, &created); err != nil {
		return "", err
	}
	// Its card, from what the POST answered: shown with the answer whether or
	// not the robot gets there in time. Waited for, on fly the want was not on
	// the board within two seconds and 「武蔵境→中野坂上」 was answered with no
	// card (2026-11-09).
	if len(created.WantIDs) > 0 {
		args[robotCardArg] = robotCardValue("want", created.WantIDs[0], name)
	}
	// Walk there, as for one already on the board. Left where it stood, the
	// robot said 「ここに『荻窪→中野坂上』があります」 from the station it had
	// been asked about a question earlier. The new want is placed beside the
	// person when it is added (CanvasNearHook), a moment after the POST
	// returns — on fly more than two seconds later. The answer does not wait
	// for the walk: its card is already the POST's, and the robot goes there
	// once the want is on the board.
	go func() {
		for i := 0; i < robotDeployWalkWait; i++ {
			if s.robotWantOnBoard(name) {
				s.robotPoint(map[string]string{"name": name})
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	}()
	return fmt.Sprintf("Deployed %q (%s)%s, next to the person, and the robot is going to it: you may say it is here. %s", name, typ, filled, robotDeployedNote), nil
}

// robotDeployWalkWait: how long (in tenths of a second) the robot, after
// deploy_want, waits for the want made to be on the board to walk there —
// behind the answer, which does not wait. Two seconds was not enough on fly.
const robotDeployWalkWait = 60

// robotDeployedNote: a want just made has not finished yet. Read as nothing to
// report, the phone's model said 「検索に失敗しました」 of a search that was
// running.
const robotDeployedNote = "It is working now and its result will show on its tile: say in the person's language that it was made (e.g. 「中野坂上→国分寺を出しました」), not that it failed."

// robotWantOnBoard: a want of this exact name is listed with a cell.
func (s *Server) robotWantOnBoard(name string) bool {
	wants, err := s.robotWantList()
	if err != nil {
		return false
	}
	for _, w := range wants {
		if w.Metadata.Name == name {
			_, x := w.Metadata.Labels[robotCanvasX]
			_, y := w.Metadata.Labels[robotCanvasY]
			return x && y
		}
	}
	return false
}

// robotMissingParams names the type's required parameters these params leave out
// or empty, with the params written out as the type's examples would fill them.
func (s *Server) robotMissingParams(typ string, params map[string]any) (missing []string, example string) {
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
	return missing, robotPairs(ex)
}

// robotHereLabel names a want type's parameter that, left out, is where the
// person is — transit_search's from: 「戸塚への乗り換え」 starts here.
const robotHereLabel = "here-param"

// robotMarkerLabel prefixes a want type's words for one parameter: what follows
// a value in a request to say it is that parameter — transit_search's
// "param-marker/from": "から,発" and "param-marker/to": "まで,へ". A request
// says which end is which in those words; the model does not always keep it.
// On 2026-10-22 「銀座から中野坂上の乗り換え」 went to the phone's model,
// which deployed from=中野坂上, to=銀座 — the stations of the type's own worked
// example, in its order.
const robotMarkerLabel = "param-marker/"

// robotMarkedParams sets each marked parameter to the value the question marks
// for it: of the values sent, the one the question has just before one of
// the parameter's words (a character between allowed — 「銀座駅から」).
// Where that value was sent under another marked parameter, the two change
// places, so the ends the model swapped are put back rather than both made
// the same. Only marked parameters are read and moved: 「9時40分までに」 is a
// time, not where to.
// Values only, never words of its own: what the model sent is what is
// placed, by what the person said. Says what it put right, for the robot to
// tell; "" when nothing was.
func robotMarkedParams(labels map[string]string, params map[string]any, question string) string {
	if strings.TrimSpace(question) == "" {
		return ""
	}
	var names []string
	for k := range labels {
		if strings.HasPrefix(k, robotMarkerLabel) {
			names = append(names, strings.TrimPrefix(k, robotMarkerLabel))
		}
	}
	sort.Strings(names)
	var fixed []string
	for _, name := range names {
		var markers []string
		for _, m := range strings.Split(labels[robotMarkerLabel+name], ",") {
			if m = strings.TrimSpace(m); m != "" {
				markers = append(markers, m)
			}
		}
		marked := ""
		for _, other := range names {
			v, ok := params[other]
			if !ok {
				continue
			}
			sv := strings.TrimSpace(fmt.Sprint(v))
			if sv == "" || sv == marked || !robotMarks(question, sv, markers) {
				continue
			}
			if marked != "" {
				marked = "" // two values marked alike: the question does not say which
				break
			}
			marked = sv
		}
		if marked == "" || strings.TrimSpace(fmt.Sprint(params[name])) == marked {
			continue
		}
		old, had := params[name]
		for _, other := range names {
			if v, ok := params[other]; ok && other != name && strings.TrimSpace(fmt.Sprint(v)) == marked {
				if had {
					params[other] = old
				} else {
					delete(params, other)
				}
			}
		}
		params[name] = marked
		fixed = append(fixed, fmt.Sprintf("%s=%s", name, marked))
	}
	if len(fixed) == 0 {
		return ""
	}
	return fmt.Sprintf(" (%s, as the person said it: %s)", strings.Join(fixed, ", "), robotPairs(params))
}

// robotMarks: the question has v just before one of the markers, with at most
// one character between.
func robotMarks(question, v string, markers []string) bool {
	for i := 0; ; {
		at := strings.Index(question[i:], v)
		if at < 0 {
			return false
		}
		rest := question[i+at+len(v):]
		for _, m := range markers {
			if strings.HasPrefix(rest, m) {
				return true
			}
			if _, n := utf8.DecodeRuneInString(rest); n > 0 && strings.HasPrefix(rest[n:], m) {
				return true
			}
		}
		i += at + len(v)
	}
}

// robotFitParams makes params what the type takes, by what each parameter is
// rather than by the type: a time (subType time) as HH:MM however it was
// written, a value outside a parameter's choices sent back with them, and the
// type's here-param, left out, filled with where the person is. Says what it
// filled, for the robot to tell.
func (s *Server) robotFitParams(typ string, params map[string]any, question string) (string, error) {
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
	filled := robotMarkedParams(def.Metadata.Labels, params, question)
	for _, p := range def.Parameters {
		if !said(p.Name) {
			if p.Name == def.Metadata.Labels[robotHereLabel] {
				here, err := s.robotHere(p.SubType)
				if err != nil {
					return "", fmt.Errorf("%s was left out and where the person is is not known (%v): ask the person — nothing was made", p.Name, err)
				}
				params[p.Name] = here
				filled += fmt.Sprintf(" (%s=%s: where the person is, as none was said)", p.Name, here)
			}
			continue
		}
		if p.SubType == "time" {
			hhmm, err := robotClock(params[p.Name])
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
	robotClockColon = regexp.MustCompile(`^(\d{1,2})[:：.](\d{2})$`)
	robotClockJa    = regexp.MustCompile(`(\d{1,2})\s*時\s*(?:(\d{1,2})\s*分|(半))?`)
	robotClockPM    = regexp.MustCompile(`午後|夜|夕方|(?i:pm)`)
)

// robotClock reads a time of day as a person or a model writes it — 09:40, 9:40,
// 9時40分, 9時半, 午後3時, 0940 (which params read as the number 940) — as HH:MM.
func robotClock(v any) (string, error) {
	raw := strings.TrimSpace(fmt.Sprint(v))
	if f, ok := v.(float64); ok {
		raw = fmt.Sprintf("%04d", int(f))
	}
	h, m := -1, 0
	switch {
	case robotClockColon.MatchString(raw):
		p := robotClockColon.FindStringSubmatch(raw)
		h, _ = strconv.Atoi(p[1])
		m, _ = strconv.Atoi(p[2])
	case robotClockJa.MatchString(raw):
		p := robotClockJa.FindStringSubmatch(raw)
		h, _ = strconv.Atoi(p[1])
		if p[2] != "" {
			m, _ = strconv.Atoi(p[2])
		} else if p[3] != "" {
			m = 30
		}
		if robotClockPM.MatchString(raw) && h < 12 {
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

// robotNearestStationURL is HeartRails Express's nearest stations — what
// mywant-nearest-plugin reads for stations in Japan. A variable for the tests.
var robotNearestStationURL = "https://express.heartrails.com/api/json?method=getStations"

// robotHere is where the person is, as a parameter of this subType takes it: a
// station is the one nearest them, anything else their coordinates. Read from
// the location want that last heard from a device.
func (s *Server) robotHere(subType string) (string, error) {
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
		if w.Metadata.Type != "location" || w.Metadata.Labels[robotArchived] == "true" {
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
	return robotNearestStation(lat, lng)
}

func robotNearestStation(lat, lng float64) (string, error) {
	u := fmt.Sprintf("%s&x=%.6f&y=%.6f", robotNearestStationURL, lng, lat)
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

// robotPairs writes params the way deploy_want takes them: from=新宿, to=渋谷.
func robotPairs(params map[string]any) string {
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

// robotParseParams reads deploy_want's params: name=value pairs, separated by
// commas (or 、, ;, new lines), name and value split at the first = or :.
//
// Not JSON, though JSON is still read: every argument reaches the model as a
// string, and a model writing a JSON object inside one has to escape each of
// its quotes. The Mac's did not — its {"from":" ended the string there, and it
// sent {"from": three times over and gave up. Pairs need no quotes at all.
func robotParseParams(raw string) (map[string]any, error) {
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
	for _, part := range robotPairSep.Split(raw, -1) {
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
		params[name] = robotParamValue(value)
	}
	return params, nil
}

var robotPairSep = regexp.MustCompile(`[,、;\n]`)

// robotParamValue: a number or true/false as one, anything else as text.
func robotParamValue(v string) any {
	if f, err := strconv.ParseFloat(v, 64); err == nil {
		return f
	}
	if b, err := strconv.ParseBool(v); err == nil && (v == "true" || v == "false") {
		return b
	}
	return v
}

// robotWant is a want as the robot's tools see it.
type robotWant struct {
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

func (w robotWant) archived() bool { return w.Metadata.Labels[robotArchived] == "true" }

func (s *Server) robotWantList() ([]robotWant, error) {
	// From the builder, as /api/v1/wants lists them (no system wants, none
	// cancelled), without the list's JSON: on fly that is 2.6MB marshalled
	// and read back, four seconds each time, and a plan's brief asked three
	// times — the phone's 30 seconds were gone before it had its brief.
	if s.globalBuilder != nil {
		var out []robotWant
		for _, w := range s.globalBuilder.GetAllWantStates() {
			if shouldHideSystemWant(w, false) || w.GetStatus() == mywant.WantStatusCancelled {
				continue
			}
			var rw robotWant
			rw.Metadata.ID, rw.Metadata.Name, rw.Metadata.Type = w.Metadata.ID, w.Metadata.Name, w.Metadata.Type
			rw.Metadata.Labels = w.Metadata.Labels
			rw.Spec.Params = w.Spec.Params
			out = append(out, rw)
		}
		return out, nil
	}
	var resp struct {
		Wants []robotWant `json:"wants"`
	}
	if err := s.backend("GET", "/api/v1/wants", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Wants, nil
}

// robotFindWant names a want of this type whose parameters already say what
// these say (every non-empty one equal, as text), or "" when there is none —
// one on the board before one in the archive.
func robotFindWant(wants []robotWant, typ string, params map[string]any) (name string, archived bool) {
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
		if w.Metadata.Type != typ || w.Metadata.Name == robotWantName {
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

// robotFreeName is name, or name-2, name-3… — the first no want has.
func robotFreeName(wants []robotWant, name string) string {
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

// robotAskAgainLabel names what a want type does when deploy_want finds one of
// it with the same parameters in the archive. "run": a question, asked again —
// it is restored and run with the parameters asked now. Left out: the person
// is asked whether to restore it.
const robotAskAgainLabel = "ask-again"

// robotNameLabel: how a want of a type is named when the robot makes one —
// "{from}→{to}", its parameters in braces.
const robotNameLabel = "robot-name"

// robotNameFrom fills a robot-name template from params; "" when there is none
// or a parameter it names was not given.
func robotNameFrom(template string, params map[string]any) string {
	template = strings.TrimSpace(template)
	if template == "" {
		return ""
	}
	out, missing := template, false
	for _, m := range regexp.MustCompile(`\{([^}]+)\}`).FindAllStringSubmatch(template, -1) {
		v, ok := params[m[1]]
		text := ""
		if ok && v != nil {
			text = strings.TrimSpace(fmt.Sprint(v))
		}
		if text == "" {
			missing = true
			break
		}
		out = strings.ReplaceAll(out, m[0], text)
	}
	if missing {
		return ""
	}
	return out
}

// robotParamsText is a want's parameters as the model is told them: "from=中野坂上, to=戸塚".
func robotParamsText(params map[string]any) string {
	keys := make([]string, 0, len(params))
	for k, v := range params {
		if v != nil && strings.TrimSpace(fmt.Sprint(v)) != "" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k + "=" + strings.TrimSpace(fmt.Sprint(params[k]))
	}
	if len(parts) == 0 {
		return "no parameters"
	}
	return strings.Join(parts, ", ")
}

// robotTypeLabel is one of a want type's labels, "" when it has none or the type
// cannot be read.
func (s *Server) robotTypeLabel(typ, key string) string {
	var def struct {
		Metadata struct {
			Labels map[string]string `json:"labels"`
		} `json:"metadata"`
	}
	if err := s.backend("GET", "/api/v1/want-types/"+typ, nil, &def); err != nil {
		return ""
	}
	return def.Metadata.Labels[key]
}

// robotRunAgain brings an archived want back with the parameters asked now —
// those it had and was not asked for taken off, so a route last searched for
// 11:00 is searched for now — runs it, and walks the robot to it. A change of
// parameters runs it by itself; the same ones are run by a restart.
func (s *Server) robotRunAgain(w robotWant, params map[string]any, filled string, args map[string]string) (string, error) {
	text := func(v any) string { return strings.TrimSpace(fmt.Sprint(v)) }
	patchParams := map[string]any{}
	changed := false
	for k, v := range w.Spec.Params {
		if a, ok := params[k]; !ok || a == nil || text(a) == "" {
			if v != nil && text(v) != "" {
				patchParams[k] = nil
				changed = true
			}
		}
	}
	for k, v := range params {
		if v == nil || text(v) == "" {
			continue
		}
		patchParams[k] = v
		if got, ok := w.Spec.Params[k]; !ok || text(got) != text(v) {
			changed = true
		}
	}
	patch := map[string]any{
		"metadata": map[string]any{"labels": map[string]any{robotArchived: nil}},
		"spec":     map[string]any{"params": patchParams},
	}
	if err := s.backend("PATCH", "/api/v1/wants/"+w.Metadata.ID, patch, nil); err != nil {
		return "", fmt.Errorf("could not bring back %q: %v", w.Metadata.Name, err)
	}
	if !changed {
		if err := s.backend("POST", "/api/v1/wants/"+w.Metadata.ID+"/restart", nil, nil); err != nil {
			return "", fmt.Errorf("brought back %q but could not run it: %v", w.Metadata.Name, err)
		}
	}
	out := fmt.Sprintf("Brought %q (%s, %s)%s back onto the board and ran it again with these parameters — nothing to ask, it is running.", w.Metadata.Name, w.Metadata.Type, robotParamsText(params), filled)
	if !w.archived() {
		out = fmt.Sprintf("Ran %q (%s, %s)%s, already on the board, again with these parameters — nothing to ask, it is running.", w.Metadata.Name, w.Metadata.Type, robotParamsText(params), filled)
	}
	pointed := map[string]string{"name": w.Metadata.Name, "id": w.Metadata.ID}
	if walked, err := s.robotPoint(pointed); err == nil {
		if c, ok := pointed[robotCardArg]; ok {
			args[robotCardArg] = c
		}
		out += " " + walked
	} else {
		args[robotCardArg] = robotCardValue("want", w.Metadata.ID, w.Metadata.Name)
	}
	return out + " " + robotDeployedNote, nil
}

// robotAskRestore is what a tool that found an archived want tells the model.
func robotAskRestore(name string) string {
	return fmt.Sprintf("Ask the person whether to restore it; if they say yes, call restore_want with name %q.", name)
}

// robotArchivedNamed is the archived want called name (or loosely so), if any.
func (s *Server) robotArchivedNamed(name string) (robotWant, bool) {
	wants, err := s.robotWantList()
	if err != nil {
		return robotWant{}, false
	}
	for _, w := range wants {
		if w.archived() && (w.Metadata.Name == name || robotLooseName(w.Metadata.Name) == robotLooseName(name)) {
			return w, true
		}
	}
	return robotWant{}, false
}

func robotLooseName(v string) string {
	return strings.NewReplacer(" ", "", "-", "", "_", "").Replace(strings.ToLower(v))
}

// robotRestoreWant takes an archived want out of the archive — the label off,
// through PATCH as the GUI's restore does — and walks the robot to it.
func (s *Server) robotRestoreWant(args map[string]string) (string, error) {
	name := strings.TrimSpace(args["name"])
	if name == "" {
		return "", fmt.Errorf("name is required")
	}
	w, ok := s.robotArchivedNamed(name)
	if !ok {
		return "", fmt.Errorf("no archived want named %q", name)
	}
	patch := map[string]any{"metadata": map[string]any{"labels": map[string]any{robotArchived: nil}}}
	if err := s.backend("PATCH", "/api/v1/wants/"+w.Metadata.ID, patch, nil); err != nil {
		return "", fmt.Errorf("could not restore %q: %v", w.Metadata.Name, err)
	}
	out := fmt.Sprintf("Restored %q (%s); it is back on the board.", w.Metadata.Name, w.Metadata.Type)
	pointed := map[string]string{"name": w.Metadata.Name}
	if walked, err := s.robotPoint(pointed); err == nil {
		if c, ok := pointed[robotCardArg]; ok {
			args[robotCardArg] = c
		}
		out += " " + walked
	} else {
		args[robotCardArg] = robotCardValue("want", w.Metadata.ID, w.Metadata.Name)
	}
	return out, nil
}

const (
	robotWantName    = "robot"
	robotCanvasOn    = "mywant.io/canvas"
	robotCanvasX     = "mywant.io/canvas-x"
	robotCanvasY     = "mywant.io/canvas-y"
	robotArchived    = "mywant.io/archived"
	robotBoardLimit  = 50
	robotNearMatches = 6
)

// robotTile is one thing or want with a cell on the board.
type robotTile struct {
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

// robotCardArg is the key under which a tool notes, in its own arguments, the
// want it brought the robot to; handleToolsCall takes it out again and hands it
// back as the call's card — what the chat shows under the robot's words.
const robotCardArg = "_card"

// robotUnreadArg carries the photos look needed and could not read (a server
// with no recogniser — Fly); handleToolsCall hands them back as the call's
// unread_photos, for the asker to read on its own device and call again.
const robotUnreadArg = "_unread"

// robotUnreadPhoto is a photo to read: its thing, and the image as the server
// knows it — kept beside the reading (picture-read-from) to say what was read.
type robotUnreadPhoto struct {
	ID    string `json:"id"`
	Image string `json:"image"`
	// Read: its words are read already, and only reading them into fields
	// waits (photo_fields.go) — nothing to read off the image again.
	Read bool `json:"read,omitempty"`
}

// robotTakeUnread takes out of a step's arguments the photos look could not read.
func robotTakeUnread(args map[string]string) []robotUnreadPhoto {
	raw, ok := args[robotUnreadArg]
	if !ok {
		return nil
	}
	delete(args, robotUnreadArg)
	var list []robotUnreadPhoto
	_ = json.Unmarshal([]byte(raw), &list)
	return list
}

// robotQuestionArg carries what the person asked to look and deploy_want,
// within a turn (handleToolsCall); each takes it out before anything is kept.
const robotQuestionArg = "_question"

// robotShortArg asks look for its short form (the values and what they mean),
// never the whole type and want.
const robotShortArg = "_short"

// robotHearsQuestion: the tool is handed what the person asked (robotQuestionArg).
func robotHearsQuestion(tool string) bool {
	return tool == "look" || tool == "deploy_want" || tool == "use_capability"
}

// robotCard is a want a step brought the robot to — want-spec's RobotCard.
// What the card says (Type, Status, Summary) is filled when the answer is
// settled (robotAnswerCards): the same for every client that shows it.
type robotCard = ws.RobotCard

func (t robotTile) String() string {
	return fmt.Sprintf("%s (%s) at (%d, %d)", t.name, t.kind, t.x, t.y)
}

// named is the tile without its cell, as board lists it: a model handed the
// cells reads them out ("荻窪 is at (6, 0)") rather than showing the place, so
// where a tile is comes only from point, which walks the robot there.
func (t robotTile) named() string {
	if len(t.aliases) > 0 {
		return fmt.Sprintf("%s (%s; also called %s)", t.name, t.kind, strings.Join(t.aliases, ", "))
	}
	return fmt.Sprintf("%s (%s)", t.name, t.kind)
}

// robotTiles is the board: things first, then wants, each only when it has a cell.
func (s *Server) robotTiles() ([]robotTile, error) {
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
	list, err := s.robotWantList()
	if err != nil {
		return nil, err
	}
	wants := struct{ Wants []robotWant }{list}
	aliases := s.robotTypeAliases()
	var tiles []robotTile
	for _, t := range things.Things {
		if t.Labels[robotCanvasOn] != "true" {
			continue
		}
		kind := t.Subtype
		if kind == "" {
			kind = t.Catalog
		}
		if tile, ok := robotTileAt(t.Value, kind, t.Labels); ok {
			tile.id, tile.cardKind = t.ID, "thing"
			tiles = append(tiles, tile)
		}
	}
	for _, w := range wants.Wants {
		// Archived: put away, off the board — as the GUI draws it.
		if w.Metadata.Name == robotWantName || w.Metadata.Labels[robotArchived] == "true" {
			continue
		}
		if tile, ok := robotTileAt(w.Metadata.Name, "want "+w.Metadata.Type, w.Metadata.Labels); ok {
			tile.id, tile.cardKind = w.Metadata.ID, "want"
			tile.typ = w.Metadata.Type
			tile.aliases = aliases[w.Metadata.Type]
			tiles = append(tiles, tile)
		}
	}
	return tiles, nil
}

func robotTileAt(name, kind string, labels map[string]string) (robotTile, bool) {
	xs, okX := labels[robotCanvasX]
	ys, okY := labels[robotCanvasY]
	if !okX || !okY {
		return robotTile{}, false
	}
	var x, y int
	if _, err := fmt.Sscan(xs, &x); err != nil {
		return robotTile{}, false
	}
	if _, err := fmt.Sscan(ys, &y); err != nil {
		return robotTile{}, false
	}
	return robotTile{name: name, kind: kind, x: x, y: y}, true
}

func (s *Server) robotBoard(map[string]string) (string, error) {
	tiles, err := s.robotTiles()
	if err != nil {
		return "", err
	}
	if len(tiles) == 0 {
		return "The board is empty.", nil
	}
	var b strings.Builder
	for i, t := range tiles {
		if i == robotBoardLimit {
			fmt.Fprintf(&b, "...and %d more\n", len(tiles)-robotBoardLimit)
			break
		}
		b.WriteString(t.named() + "\n")
	}
	return b.String(), nil
}

// robotPoint walks the robot onto the named tile — the same answer as the
// coordinates, pointed at rather than spelled out — and has it say where.
func (s *Server) robotPoint(args map[string]string) (string, error) {
	name := strings.TrimSpace(args["name"])
	if name == "" {
		return "", fmt.Errorf("name is required")
	}
	tiles, err := s.robotTiles()
	if err != nil {
		return "", err
	}
	loose := func(v string) string {
		return strings.NewReplacer(" ", "", "-", "", "_", "").Replace(strings.ToLower(v))
	}
	var found *robotTile
	var near []string
	// By its id when the caller knows which one it means (a want just brought
	// back or found by its parameters): a route named 「戸塚」 and the station
	// 戸塚 share a name, and by name the robot walked to the station.
	if id := strings.TrimSpace(args["id"]); id != "" {
		for i, t := range tiles {
			if t.id == id {
				found = &tiles[i]
				break
			}
		}
	}
	for i, t := range tiles {
		if found != nil {
			break
		}
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
		// Not by its name — by what its type is also called (robotAliasLabel).
		typeOf := make([]string, len(tiles))
		for i, t := range tiles {
			typeOf[i] = t.typ
		}
		hits := robotByAlias(name, typeOf, s.robotTypeAliases())
		if len(hits) == 0 {
			// Nor by an alias — by a name its constellation puts beside it.
			names := make([]string, len(tiles))
			for i, t := range tiles {
				names[i] = t.name
			}
			hits = robotByGlossary(name, names, s.robotGlossary())
		}
		if len(hits) == 1 {
			found = &tiles[hits[0]]
		}
		for _, i := range hits {
			near = append(near, tiles[i].name)
		}
	}
	if found == nil {
		if w, ok := s.robotArchivedNamed(name); ok {
			args[robotCardArg] = robotCardValue("want", w.Metadata.ID, w.Metadata.Name)
			return fmt.Sprintf("%q (%s) is archived — put away, off the board, so there is nowhere to walk to. %s", w.Metadata.Name, w.Metadata.Type, robotAskRestore(w.Metadata.Name)), nil
		}
		if len(near) > robotNearMatches {
			near = near[:robotNearMatches]
		}
		if len(near) > 0 {
			return "", fmt.Errorf("%q is not on the board by that name; did you mean: %s", name, strings.Join(near, ", "))
		}
		// Not on the board is no answer by itself: 「戸塚への乗り換え」 stopped
		// here and read out another want instead.
		return "", fmt.Errorf("%q is not on the board — a want's parameter need not be: if it was asked for, deploy_want with it", name)
	}
	if err := s.robotWalkRobot(found.x, found.y); err != nil {
		return "", fmt.Errorf("could not walk the robot there: %v", err)
	}
	if found.id != "" {
		args[robotCardArg] = robotCardValue(found.cardKind, found.id, found.name)
	}
	words := fmt.Sprintf("「%s」はここです (%d, %d)", found.name, found.x, found.y)
	if err := s.robotSay(words); err != nil {
		return "", err
	}
	return fmt.Sprintf("The robot is standing on %s and saying: %s", found, words), nil
}

// robotWalkRobot stands the robot on a cell — how point shows where something is,
// and how an answer shows what it was read off (answer_evidence.go).
func (s *Server) robotWalkRobot(x, y int) error {
	// x and y in one step: two would leave the robot a moment at one of them.
	walk := map[string]any{"metadata": map[string]any{"labels": map[string]string{
		robotCanvasX: fmt.Sprint(x), robotCanvasY: fmt.Sprint(y),
	}}}
	return s.backend("PATCH", "/api/v1/wants/"+robotWantName, walk, nil)
}

// robotLook reads one want: its status and the state it has gathered, short.
//
// Only what says something — not the bookkeeping every want carries
// (achieving_percentage, action_by_agent…), not empty values, not the chat
// buffers, and never a value whose name says it is a secret: what this
// returns is read by a model and may be read out.
func (s *Server) robotLook(args map[string]string) (string, error) {
	question := args[robotQuestionArg]
	delete(args, robotQuestionArg)
	name := strings.TrimSpace(args["name"])
	if name == "" {
		return "", fmt.Errorf("name is required")
	}
	// Asked about a time of a want's answers (「今朝のゴルフのスコアは？」):
	// those answers, not the want as it stands (robotLookReferent).
	ref := s.resolveAnswerReferent(question, time.Now())
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
		return resp.Wants[i].Metadata.Labels[robotArchived] != "true" && resp.Wants[j].Metadata.Labels[robotArchived] == "true"
	})
	loose := robotLooseName
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
		// Not by its name — by what its type is also called (robotAliasLabel).
		typeOf := make([]string, len(resp.Wants))
		for i, w := range resp.Wants {
			typeOf[i] = w.Metadata.Type
		}
		hits := robotByAlias(name, typeOf, s.robotTypeAliases())
		if len(hits) == 0 {
			// Nor by an alias — by a name its constellation puts beside it.
			names := make([]string, len(resp.Wants))
			for i, w := range resp.Wants {
				names[i] = w.Metadata.Name
			}
			hits = robotByGlossary(name, names, s.robotGlossary())
		}
		if len(hits) == 1 {
			found = hits[0]
		}
		for _, i := range hits {
			near = append(near, resp.Wants[i].Metadata.Name)
		}
	}
	if ref != nil {
		// The want looked at is one the question named — or, not found by
		// that name alone, the name is one of theirs (several transit wants).
		hit := found >= 0 && ref.covers(resp.Wants[found].Metadata.ID)
		if found < 0 {
			for _, n := range near {
				for _, w := range ref.Wants {
					hit = hit || w.Metadata.Name == n
				}
			}
		}
		if hit {
			return s.robotLookReferent(ref, question, args), nil
		}
	}
	if found < 0 {
		if len(near) > robotNearMatches {
			near = near[:robotNearMatches]
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
		args[robotCardArg] = robotCardValue("want", w.Metadata.ID, w.Metadata.Name)
		// What the answer is read off: the robot stands on the want.
		if question != "" {
			s.walkRobotToEvidence(w.Metadata.ID)
		}
	}
	note := ""
	if w.Metadata.Labels[robotArchived] == "true" {
		note = fmt.Sprintf("%q is archived — put away, off the board. %s\n", w.Metadata.Name, robotAskRestore(w.Metadata.Name))
	}

	// Everything there is about it — the whole type definition (what it is,
	// what each of its values means, how it is made) and the whole want — cut
	// to what the smallest model can take (robotLookFull). A want that cannot be
	// cut to fit falls back to the short form below.
	// The plan's look (plan.go) asks for the short form only: what it
	// reads goes into a prompt with the rest of the turn, cut at its length,
	// and the type's definition first left the values themselves past the cut.
	if args[robotShortArg] != "true" {
		if full := s.robotLookFull(w.Metadata.Name, w.Metadata.Type); full != "" {
			return note + full, nil
		}
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
		meaning[st.Name] = robotFirstSentence(st.Description, 90)
		order = append(order, st.Name)
	}

	var b strings.Builder
	b.WriteString(note)
	fmt.Fprintf(&b, "%s (%s): %s\n", w.Metadata.Name, w.Metadata.Type, w.Status)
	if about := robotFirstSentence(def.Metadata.Description, 160); about != "" {
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
		if robotLookSkip(k) {
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

// robotLookTokens is what look may hand the model, in tokens.
//
// Sized for the smallest window that asks: an iPhone's model has 4,096 tokens
// (a Mac's 8,192) for the instructions and the tool schemas — about 2,000 of
// them together — the question, this answer and the reply. A smartgolf want
// and its type handed over whole came to 5,327 tokens there and the turn
// failed outright.
const robotLookTokens = 1500

// robotLookFull is one want with its type's whole definition, as JSON, cut to
// robotLookTokens by the same token budget every GET can ask for (token_budget.go):
// its history first, then bookkeeping, display hints, the framework's shared
// fields, how the type is made, and long values — what it is and what its
// values mean go last. "" when it cannot be read or cut to fit.
//
// Secrets stay out here too: a state or label whose name says it is one is
// dropped, as robotLookSkip drops it from the short form.
func (s *Server) robotLookFull(name, typ string) string {
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
	robotDropSecrets(want)
	robotDropSecrets(typeDef)
	cut, res := budgetFit(map[string]any{"type": typeDef, "want": want}, robotLookTokens)
	if !res.Fits {
		return ""
	}
	doc := cut.(map[string]any)
	td, _ := json.Marshal(doc["type"])
	wj, _ := json.Marshal(doc["want"])
	return "Type definition:\n" + string(td) + "\n\nWant:\n" + string(wj)
}

// robotDropSecrets removes, at any depth, every map entry whose key names a
// secret (see robotLookSecret) — or, in a state definition's list, every entry
// whose "name" does.
func robotDropSecrets(v any) {
	switch x := v.(type) {
	case map[string]any:
		for k, child := range x {
			if robotLookSecret(k) {
				delete(x, k)
				continue
			}
			robotDropSecrets(child)
		}
	case []any:
		for i, child := range x {
			if m, ok := child.(map[string]any); ok {
				if n, _ := m["name"].(string); n != "" && robotLookSecret(n) {
					x[i] = map[string]any{"name": n, "omitted": "secret"}
					continue
				}
			}
			robotDropSecrets(child)
		}
	}
}

// robotLookSecret: a key whose name says it holds a secret.
func robotLookSecret(key string) bool {
	lower := strings.ToLower(key)
	for _, secret := range []string{"key", "token", "secret", "password", "credential", "auth"} {
		if strings.Contains(lower, secret) {
			return true
		}
	}
	return false
}

// robotLookSkip: the state every want keeps for its own running, and secrets.
func robotLookSkip(key string) bool {
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
	return robotLookSecret(lower)
}

// robotSay puts words in the robot's speech bubble on the board — said in
// the room, as every character's words are (recordSpeech): the bubble over
// its head and a line in the conversation the board's column keeps. Written
// only to the robot want's `say`, they reached neither.
func (s *Server) robotSay(words string) error {
	return s.robotSayAs(words, "")
}

// robotSayAs is robotSay with what kind of line it is (SpeechEntry.Kind).
func (s *Server) robotSayAs(words, kind string) error {
	words = strings.TrimSpace(words)
	if words == "" {
		return nil
	}
	recordSpeechAs(robotCharacterID, words, "say", kind)
	return s.backend("PUT", "/api/v1/states/"+robotWantName, map[string]any{"say": words}, nil)
}

// robotFirstSentence is a description cut to its first sentence, on one line —
// a type's YAML wraps its descriptions across lines and goes on to say more
// than a model needs to read a value by.
func robotFirstSentence(s string, max int) string {
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
