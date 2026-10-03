package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"time"
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
// (a want's params) travels as JSON text.
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
- WHAT something says or how it is (「Nakanoの天気は？」, a timer's time left, a checklist): call look with the name (「Nakano」 finds NakanoのWeather), and tell the person what it reports, e.g. 「Nakanoは曇り、23°Cです」. Never point for these.
Use board to see the names of what is on it.
To add a want, choose its type, read its parameters with describe_type, then deploy_want. A want already there with the same parameters is not made twice: deploy_want walks you to it instead — then say so, e.g. 「もうあります。ここです」.
A route between two stations (「中野坂上から銀座の乗り換え」「AからBへの行き方」): deploy_want transit_search with params {"from":"中野坂上","to":"銀座"} — the station names alone.
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
			{Name: "params", Description: `The parameters as a JSON object, e.g. {"content":"Tea"}. Use {} when there are none.`, Required: true},
			{Name: "name", Description: "A short name for the want, lowercase with hyphens", Required: false},
		},
		run: (*Server).fmDeployWant,
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
	fmWriteJSON(w, map[string]any{"instructions": fmInstructions, "tools": tools})
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
		fmWriteJSON(w, map[string]any{"output": s.fmRunTool(req.Tool, req.Arguments)})
		return
	}
	s.fmRunStep(&step)
	turn, err := s.fmUpdateTurn(req.Turn, func(t *fmTurn) error {
		t.Steps = append(t.Steps, step)
		return nil
	})
	if err != nil {
		log.Printf("[fm] step outside a known turn: %v", err)
	} else if !turn.Quiet {
		s.fmShowStep(step, "")
	}
	fmWriteJSON(w, map[string]any{"output": step.Output})
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
	Name       string `json:"name"`
	Title      string `json:"title"`
	Category   string `json:"category"`
	SystemType bool   `json:"system_type"`
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
			Description string `json:"description"`
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
	if len(resp.Parameters) == 0 {
		b.WriteString("No parameters; deploy with {}.\n")
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
	params := map[string]any{}
	if raw := strings.TrimSpace(args["params"]); raw != "" {
		if err := json.Unmarshal([]byte(raw), &params); err != nil {
			return "", fmt.Errorf("params is not a JSON object: %v", err)
		}
	}
	// Already on the board — the same type asked the same thing — is not a
	// reason to make it twice: the robot goes to the one there is, and says so.
	// 「中野坂上から銀座の乗り換え」 asked again walks to the route already
	// found rather than finding it again beside it.
	if existing, err := s.fmFindWant(typ, params); err == nil && existing != "" {
		if _, err := s.fmPoint(map[string]string{"name": existing}); err == nil {
			return fmt.Sprintf("%q (%s) with these parameters is already on the board; the robot is standing on it. Nothing new was made.", existing, typ), nil
		}
		return fmt.Sprintf("%q (%s) with these parameters is already on the board. Nothing new was made.", existing, typ), nil
	}
	name := strings.TrimSpace(args["name"])
	if name == "" {
		name = fmt.Sprintf("%s-%d", strings.ReplaceAll(typ, " ", "-"), time.Now().Unix()%100000)
	}
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
	return fmt.Sprintf("Deployed %q (%s).", name, typ), nil
}

// fmFindWant names a want of this type whose parameters already say what
// these say (every non-empty one equal, as text), or "" when there is none.
func (s *Server) fmFindWant(typ string, params map[string]any) (string, error) {
	var wants struct {
		Wants []struct {
			Metadata struct {
				Name string `json:"name"`
				Type string `json:"type"`
			} `json:"metadata"`
			Spec struct {
				Params map[string]any `json:"params"`
			} `json:"spec"`
		} `json:"wants"`
	}
	if err := s.backend("GET", "/api/v1/wants", nil, &wants); err != nil {
		return "", err
	}
	text := func(v any) string { return strings.TrimSpace(fmt.Sprint(v)) }
	asked := 0
	for _, v := range params {
		if v != nil && text(v) != "" {
			asked++
		}
	}
	if asked == 0 {
		return "", nil
	}
	for _, w := range wants.Wants {
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
		if same {
			return w.Metadata.Name, nil
		}
	}
	return "", nil
}

const (
	fmRobotWant   = "robot"
	fmCanvasOn    = "mywant.io/canvas"
	fmCanvasX     = "mywant.io/canvas-x"
	fmCanvasY     = "mywant.io/canvas-y"
	fmBoardLimit  = 50
	fmNearMatches = 6
)

// fmTile is one thing or want with a cell on the board.
type fmTile struct {
	name, kind string
	x, y       int
}

func (t fmTile) String() string { return fmt.Sprintf("%s (%s) at (%d, %d)", t.name, t.kind, t.x, t.y) }

// named is the tile without its cell, as board lists it: a model handed the
// cells reads them out ("荻窪 is at (6, 0)") rather than showing the place, so
// where a tile is comes only from point, which walks the robot there.
func (t fmTile) named() string { return fmt.Sprintf("%s (%s)", t.name, t.kind) }

// fmTiles is the board: things first, then wants, each only when it has a cell.
func (s *Server) fmTiles() ([]fmTile, error) {
	var things struct {
		Things []struct {
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
				Name   string            `json:"name"`
				Type   string            `json:"type"`
				Labels map[string]string `json:"labels"`
			} `json:"metadata"`
		} `json:"wants"`
	}
	if err := s.backend("GET", "/api/v1/wants", nil, &wants); err != nil {
		return nil, err
	}
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
			tiles = append(tiles, tile)
		}
	}
	for _, w := range wants.Wants {
		if w.Metadata.Name == fmRobotWant {
			continue
		}
		if tile, ok := fmTileAt(w.Metadata.Name, "want "+w.Metadata.Type, w.Metadata.Labels); ok {
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
	if found == nil {
		if len(near) > fmNearMatches {
			near = near[:fmNearMatches]
		}
		if len(near) > 0 {
			return "", fmt.Errorf("%q is not on the board by that name; did you mean: %s", name, strings.Join(near, ", "))
		}
		return "", fmt.Errorf("%q is not on the board", name)
	}
	for key, v := range map[string]int{fmCanvasX: found.x, fmCanvasY: found.y} {
		body := map[string]string{"key": key, "value": fmt.Sprint(v)}
		if err := s.backend("POST", "/api/v1/wants/"+fmRobotWant+"/labels", body, nil); err != nil {
			return "", fmt.Errorf("could not walk the robot there: %v", err)
		}
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
				Name string `json:"name"`
				Type string `json:"type"`
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
	loose := func(v string) string {
		return strings.NewReplacer(" ", "", "-", "", "_", "").Replace(strings.ToLower(v))
	}
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
	var b strings.Builder
	fmt.Fprintf(&b, "%s (%s): %s\n", w.Metadata.Name, w.Metadata.Type, w.Status)
	keys := make([]string, 0, len(w.State.Current))
	for k := range w.State.Current {
		keys = append(keys, k)
	}
	sort.Strings(keys)
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
		fmt.Fprintf(&b, "- %s: %s\n", k, firstLine(text, 160))
		if b.Len() > 900 {
			b.WriteString("…\n")
			break
		}
	}
	if fr := fmt.Sprint(w.State.FinalResult); w.State.FinalResult != nil && fr != "" && fr != "map[]" {
		raw, _ := json.Marshal(w.State.FinalResult)
		fmt.Fprintf(&b, "- final result: %s\n", firstLine(string(raw), 200))
	}
	return b.String(), nil
}

// fmLookSkip: the state every want keeps for its own running, and secrets.
func fmLookSkip(key string) bool {
	switch key {
	case "achieved", "achieving_percentage", "action_by_agent", "completed", "say":
		return true
	}
	lower := strings.ToLower(key)
	if strings.HasPrefix(lower, "cc_") || strings.HasPrefix(lower, "webhook_") || strings.HasPrefix(lower, "fm_") {
		return true
	}
	for _, secret := range []string{"key", "token", "secret", "password", "credential", "auth"} {
		if strings.Contains(lower, secret) {
			return true
		}
	}
	return false
}

// fmSay puts words in the robot's speech bubble on the board.
func (s *Server) fmSay(words string) error {
	words = strings.TrimSpace(words)
	if words == "" {
		return nil
	}
	return s.backend("PUT", "/api/v1/states/"+fmRobotWant, map[string]any{"say": words}, nil)
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
