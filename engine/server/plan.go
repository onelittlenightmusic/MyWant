package server

// plan.go — the robot's turn as a plan, not a loop of tools.
//
// What the person asks the robot for is information, and on the board
// information is a want: one already there that answers it, or one made for
// it. So a small model is asked only what a model is needed for — to choose —
// and everything else is done here, the same way every time:
//
//	POST /plans/brief   what is on the board that may answer it, the types
//	                    that could make one, what may be asked of a want (its
//	                    capabilities) → the decision's prompt and its choices
//	POST /plans/params  a chosen type's parameters → the prompt to fill them
//
// Both only help to plan: nothing is changed. What the model decided is
// kept as a Plan (plans.go) and carried out by an Act that names it
// (acts.go), which does it — shown, brought back from the
// archive, asked a capability, or made, a want with the same parameters shown
// instead of made twice — keeps the act to be read back, and hands back the
// prompt for the answer, from what was done and found.
//
// At most three generations a turn, none of them free to call a tool again:
// with tools of its own a small model went round describe_type and
// deploy_want five times after the want was made (fly, 2026-11-02: four
// minutes for a six-second search), and answered with the turn before's name.

import (
	"encoding/json"
	"fmt"
	ws "github.com/onelittlenightmusic/want-spec"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	planCandidates = 10 // board names offered to the decision
	planTypes      = 8  // types offered with what they are
	planResultWait = 10 * time.Second
	// planNone is the choice that is no choice, in an enum.
	planNone = "-"
)

type planBriefRequest struct {
	Question string `json:"question"`
	Locale   string `json:"locale"`
	// Turn is the turn being answered, when the client has opened one: what
	// was said before it is part of the brief (plan_earlier.go).
	Turn string `json:"turn,omitempty"`
}

type planBriefResponse struct {
	Prompt string   `json:"prompt"`
	Refs   []string `json:"refs"`
	Types  []string `json:"types"`
	// What may be asked of the want chosen as ref (tools_capability.go): the
	// standard capabilities and those of the candidates' types, by name; the
	// first, "-", is just to show it.
	Capabilities []string `json:"capabilities"`
	// Actions are the actions to choose from (want-spec's PlanActions):
	// existing, new, none. A client offers these rather than its own list,
	// so an action added here needs no new app.
	Actions []string `json:"actions"`
}

type planCandidate struct {
	name, line string
	score      int
	// A want (not a thing): it has the standard capabilities.
	want bool
}

// POST /api/v1/plans/brief
func (s *Server) handlePlanBrief(w http.ResponseWriter, r *http.Request) {
	var req planBriefRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}
	brief, err := s.planBrief(req.Question, req.Locale, req.Turn)
	if err != nil {
		http.Error(w, "backend: "+err.Error(), http.StatusBadGateway)
		return
	}
	robotWriteJSON(w, brief)
}

func (s *Server) planBrief(question, locale, turn string) (planBriefResponse, error) {
	cands, err := s.candidatesFor(question)
	if err != nil {
		return planBriefResponse{}, err
	}
	// What was asked just before (plan_earlier.go): its questions in the
	// brief, the wants it showed among the candidates, last.
	earlier := s.earlierInConversation(turn, question)
	cands = withEarlierWants(cands, earlier)
	allTypes, err := s.robotTypeNames()
	if err != nil {
		return planBriefResponse{}, err
	}
	typesText := s.planTypesText(question)

	var b strings.Builder
	fmt.Fprintf(&b, "The person asked: 「%s」\n\n", question)
	b.WriteString(earlierText(earlier))
	b.WriteString("You answer with the board: show a want or thing already there that answers it, or make a want that will. Decide which:\n")
	b.WriteString("- existing: one of the names below answers exactly this question — choose it as ref. A want about something else (other stations, another place or day) does not. When the question asks a want for something it can do or look up (below, and on its line), choose it as ref and that as capability; just to show it, capability -.\n")
	b.WriteString("- new: nothing below answers it, and a type can make a want that will — choose the type.\n")
	b.WriteString("- none: it is not a question about anything on or for the board (a greeting, thanks, a remark).\n\n")
	// A candidate's type's own capabilities, on its line.
	ownCaps := map[string][]string{}
	var capNames []string
	seenCap := map[string]bool{}
	isCand := map[string]bool{}
	for _, c := range cands {
		if c.want {
			isCand[c.name] = true
		}
	}
	for _, c := range s.boardCapabilities() {
		name := c.want.Metadata.Name
		if !isCand[name] {
			continue
		}
		ownCaps[name] = append(ownCaps[name], c.def.Name+" — "+firstLine(c.def.Description, 100))
		if !seenCap[c.def.Name] {
			seenCap[c.def.Name] = true
			capNames = append(capNames, c.def.Name)
		}
	}
	if len(cands) == 0 {
		b.WriteString("Nothing on the board seems to be about this question.\n")
	} else {
		b.WriteString("On the board, and put away (archived), what may be about it:\n")
		for _, c := range cands {
			line := c.line
			if own := ownCaps[c.name]; c.want && len(own) > 0 {
				line += "; it can also look up (capability): " + strings.Join(own, "; ")
			}
			b.WriteString(line + "\n")
		}
	}
	if typesText != "" {
		b.WriteString("\nTypes that could make a want for it:\n" + typesText)
	}
	// What may be asked of a want (tools_capability.go): the standard
	// capabilities once, for every want; a type's own on its wants' lines
	// (above). Not one line per want and capability: the decision's prompt is
	// read by a small model, and every want is not asked to run again.
	caps := []string{planNone}
	b.WriteString("\nWhat any want above can do (capability):\n")
	for _, c := range stdCaps {
		fmt.Fprintf(&b, "- %s: %s\n", c.Name, firstLine(c.Description, 160))
		caps = append(caps, c.Name)
	}
	for _, n := range capNames {
		caps = append(caps, n)
	}
	b.WriteString("\nSet the fields you do not use to " + planNone + ".")

	refs := []string{planNone}
	seen := map[string]bool{}
	for _, c := range cands {
		if !seen[c.name] {
			seen[c.name] = true
			refs = append(refs, c.name)
		}
	}
	return planBriefResponse{
		Prompt:       b.String() + robotLocaleText(locale),
		Refs:         refs,
		Types:        append([]string{planNone}, allTypes...),
		Capabilities: caps,
		Actions:      append([]string{}, ws.PlanActions...),
	}, nil
}

// candidatesFor is what on the board (and in the archive) may be about the
// question, the likeliest first: named in it, called by a word of its line in
// the glossary, its type called by an alias, its parameters said in it.
func (s *Server) candidatesFor(question string) ([]planCandidate, error) {
	tiles, err := s.robotTiles()
	if err != nil {
		return nil, err
	}
	wants, err := s.robotWantList()
	if err != nil {
		return nil, err
	}
	paramsOf := map[string]map[string]any{}
	for _, w := range wants {
		paramsOf[w.Metadata.Name] = w.Spec.Params
	}
	said := robotLoose(question)
	glossary := s.robotGlossary()
	aliases := s.robotTypeAliases()

	var out []planCandidate
	// Its name or its parameters must be in what was said; its type called
	// by an alias only puts it higher — every route ever searched is not
	// about this one.
	score := func(name, typ string, params map[string]any) int {
		sc := 0
		if n := robotLoose(name); n != "" && (strings.Contains(said, n) || strings.Contains(n, said)) {
			sc += 3
		} else if robotNamePartSaid(name, said) {
			sc += 3 // 「Nakanoの天気」 for NakanoのWeather
		}
		asked := 0
		for _, v := range params {
			if robotEmptyParam(v) {
				continue // a checklist's items: [] was not asked of it
			}
			t := robotLoose(fmt.Sprint(v))
			if t == "" {
				continue
			}
			asked++
			if len([]rune(t)) >= 2 && strings.Contains(said, t) {
				sc += 2
			}
		}
		// A want asked nothing (「スマートゴルフの空き」, the one of its kind):
		// its type called by an alias is it.
		if asked == 0 && typ != "" && robotAliasScore(question, aliases[typ]) > 0 {
			sc += 3
		}
		return sc
	}
	// Ranked higher, never let in, by its type being called by an alias.
	rank := func(sc int, typ string) int {
		if typ != "" && robotAliasScore(question, aliases[typ]) > 0 {
			return sc + 2
		}
		return sc
	}
	names := make([]string, len(tiles))
	for i, t := range tiles {
		names[i] = t.name
	}
	byGlossary := map[int]bool{}
	for _, i := range robotByGlossary(question, names, glossary) {
		byGlossary[i] = true
	}
	for i, t := range tiles {
		sc := score(t.name, t.typ, paramsOf[t.name])
		if byGlossary[i] {
			sc += 2
		}
		// A want by its name, or by two of what it was asked (from and to);
		// one station said is not that station's every route.
		if sc == 0 || (t.cardKind == "want" && sc < 3) {
			continue
		}
		line := fmt.Sprintf("- %s: %s", t.name, t.kind)
		if p := paramsOf[t.name]; t.cardKind == "want" && len(p) > 0 {
			line += " (" + robotParamsText(p) + ")"
		}
		out = append(out, planCandidate{name: t.name, line: line, score: rank(sc, t.typ), want: t.cardKind == "want"})
	}
	for _, w := range wants {
		if !w.archived() || w.Metadata.Name == robotWantName {
			continue
		}
		sc := score(w.Metadata.Name, w.Metadata.Type, w.Spec.Params)
		if sc < 4 {
			continue // put away: only one that is plainly this question
		}
		line := fmt.Sprintf("- %s: want %s, put away", w.Metadata.Name, w.Metadata.Type)
		if len(w.Spec.Params) > 0 {
			line += " (" + robotParamsText(w.Spec.Params) + ")"
		}
		out = append(out, planCandidate{name: w.Metadata.Name, line: line, score: rank(sc, w.Metadata.Type) - 1})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].score > out[j].score })
	if len(out) > planCandidates {
		out = out[:planCandidates]
	}
	return out, nil
}

// robotNamePartSaid: a part of the name (split at の, spaces and dashes) of
// three letters or more is in what was said.
func robotNamePartSaid(name, said string) bool {
	parts := strings.FieldsFunc(strings.ToLower(name), func(r rune) bool {
		return r == 'の' || r == ' ' || r == '　' || r == '-' || r == '_' || r == '・'
	})
	if len(parts) < 2 {
		return false
	}
	for _, p := range parts {
		if len([]rune(p)) >= 3 && strings.Contains(said, robotLoose(p)) {
			return true
		}
	}
	return false
}

// planTypesText is the types the question may be for, with what they are
// and what they say about their questions: called by an alias or their title
// first, then those that say how their questions go.
func (s *Server) planTypesText(question string) string {
	types, err := s.robotWantTypes()
	if err != nil {
		return ""
	}
	aliases := s.robotTypeAliases()
	said := robotLoose(question)
	type ranked struct {
		t     robotWantType
		score int
	}
	var rs []ranked
	for _, t := range types {
		sc := robotAliasScore(question, aliases[t.Name]) * 2
		if title := robotLoose(t.Title); title != "" && strings.Contains(said, title) {
			sc += 2
		}
		if robotTypeHint(t.Labels) != "" {
			sc++
		}
		if sc > 0 {
			rs = append(rs, ranked{t, sc})
		}
	}
	sort.SliceStable(rs, func(i, j int) bool { return rs[i].score > rs[j].score })
	var b strings.Builder
	for i, r := range rs {
		if i == planTypes {
			break
		}
		fmt.Fprintf(&b, "- %s: %s\n", r.t.Name, firstLine(r.t.Title, 80))
		if h := planHint(robotTypeHint(r.t.Labels)); h != "" && b.Len()+len(h) < robotHintsMax {
			b.WriteString("  " + strings.ReplaceAll(h, "\n", "\n  ") + "\n")
		}
	}
	return b.String()
}

// planHint is a type's hint as the plan reads it: written for the robot's
// old tools, whose names mean nothing here.
func planHint(h string) string {
	return strings.NewReplacer("deploy_want ", "make ", "deploy it", "make it").Replace(h)
}

// ── type-brief ──────────────────────────────────────────────────────────────

type planParamsRequest struct {
	Question string `json:"question"`
	Type     string `json:"type"`
	Locale   string `json:"locale"`
	// Turn is the turn being answered, as for the brief: what was said
	// before it may fill what this question leaves out (「同じルートで明日」).
	Turn string `json:"turn,omitempty"`
}

type planParam struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

// POST /api/v1/plans/params
func (s *Server) handlePlanParams(w http.ResponseWriter, r *http.Request) {
	var req planParamsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}
	name, err := s.robotResolveType(req.Type)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	var resp struct {
		Metadata struct {
			Description string            `json:"description"`
			Labels      map[string]string `json:"labels"`
		} `json:"metadata"`
		Parameters []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			Required    bool   `json:"required"`
			Example     any    `json:"example"`
		} `json:"parameters"`
	}
	if err := s.backend("GET", "/api/v1/want-types/"+name, nil, &resp); err != nil {
		http.Error(w, "backend: "+err.Error(), http.StatusBadGateway)
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "The person asked: 「%s」\n\nA new %s want is made for it (%s).\n", req.Question, name, firstLine(resp.Metadata.Description, 160))
	if h := planHint(robotTypeHint(resp.Metadata.Labels)); h != "" {
		b.WriteString(h + "\n")
	}
	b.WriteString(earlierText(s.earlierInConversation(req.Turn, req.Question)))
	b.WriteString("Fill its parameters from what the person said — and, when they refer to what was said earlier, from that. Leave a parameter empty when it was not said — never make a value up.\n")
	var params []planParam
	for i, p := range resp.Parameters {
		if i == 10 {
			break
		}
		desc := firstLine(p.Description, 120)
		if p.Example != nil {
			desc += fmt.Sprintf(" e.g. %v", p.Example)
		}
		params = append(params, planParam{Name: p.Name, Description: desc, Required: p.Required})
	}
	robotWriteJSON(w, map[string]any{"prompt": b.String() + robotLocaleText(req.Locale), "type": name, "params": params})
}

// ── act ─────────────────────────────────────────────────────────────────────

type actRequest struct {
	Turn     string            `json:"turn"`
	Question string            `json:"question"`
	Locale   string            `json:"locale"`
	Kind     string            `json:"kind"` // existing | new | none
	Ref      string            `json:"ref"`
	Type     string            `json:"type"`
	Params   map[string]string `json:"params"`
	// For existing: what is asked of the want (tools_capability.go), or "-".
	Capability string `json:"capability"`
}

// doAct carries a plan out (carryOutPlan) and says what was done: the steps, the
// cards they showed, and the prompt to word the answer from. The act's own
// record is acts.go's.
// onStep hears each step as it is done, for an act's progress (acts.go).
func (s *Server) doAct(req actRequest, onStep func(robotStep)) (prompt string, cards []*robotCard, steps []robotStep) {
	if req.Question == "" && req.Turn != "" {
		req.Question = s.turnQuestion(req.Turn)
	}
	steps = s.carryOutPlan(req, onStep)
	if req.Kind == "new" && len(steps) == 0 {
		// Nothing of a want was said (「こんにちは」 taken for a route): it
		// was not about the board after all.
		req.Kind = "none"
	}
	shown := map[string]bool{}
	facts := make([]string, 0, len(steps))
	for _, st := range steps {
		if st.Card != nil && !shown[st.Card.ID] {
			shown[st.Card.ID] = true
			cards = append(cards, st.Card)
		}
		facts = append(facts, planFact(st))
	}
	if req.Turn != "" && len(steps) > 0 {
		if _, err := s.robotUpdateTurn(req.Turn, func(t *robotTurn) error { t.Steps = append(t.Steps, steps...); return nil }); err == nil {
			for _, st := range steps {
				s.robotShowStep(st, "")
			}
		}
	}
	return robotAnswerPrompt(req, facts) + robotLocaleText(req.Locale), cards, steps
}

// carryOutPlan carries the decision out, as the steps the old tools would have
// taken — the same tools, so the board, the turn and its cards are as before.
func (s *Server) carryOutPlan(req actRequest, onStep func(robotStep)) []robotStep {
	run := func(tool string, args map[string]string) robotStep {
		if robotHearsQuestion(tool) && req.Question != "" {
			args[robotQuestionArg] = req.Question
		}
		st := robotStep{Tool: tool, Arguments: args}
		s.robotRunStep(&st)
		delete(st.Arguments, robotQuestionArg)
		delete(st.Arguments, robotShortArg)
		st.Card = robotTakeCard(st.Arguments)
		st.Done = true
		st.At = time.Now().Format(time.RFC3339)
		if onStep != nil {
			onStep(st)
		}
		return st
	}
	switch req.Kind {
	case "existing":
		name := strings.TrimSpace(req.Ref)
		if name == "" || name == planNone {
			return nil
		}
		if c := strings.TrimSpace(req.Capability); c != "" && c != planNone {
			// A capability of the want (tools_capability.go): done here, what it
			// found handed to the answer.
			return []robotStep{run("use_capability", map[string]string{"capability": c, "want": name})}
		}
		if strings.Contains(name, capSep) {
			// One chosen as want#capability (the tool loop's spelling).
			return []robotStep{run("use_capability", map[string]string{"capability": name})}
		}
		var steps []robotStep
		if _, archived := s.robotArchivedNamed(name); archived {
			// Asked for by name: brought back, not asked about again.
			steps = append(steps, run("restore_want", map[string]string{"name": name}))
		}
		steps = append(steps, run("point", map[string]string{"name": name}))
		if strings.HasPrefix(steps[len(steps)-1].Output, "Error:") {
			return steps
		}
		return append(steps, run("look", map[string]string{"name": name, robotShortArg: "true"}))
	case "new":
		typ := strings.TrimSpace(req.Type)
		if typ == "" || typ == planNone {
			return nil
		}
		// Only what the person said: a small model fills a greeting's
		// 「こんにちは」 with the type's example (中野坂上→銀座). A want with
		// parameters, none of them said, is not asked for — nothing is made.
		var pairs []string
		for k, v := range req.Params {
			if v = strings.TrimSpace(v); v != "" && v != planNone && robotSaidIn(v, req.Question) {
				pairs = append(pairs, k+"="+v)
			}
		}
		if len(pairs) == 0 && len(req.Params) > 0 {
			return nil
		}
		sort.Strings(pairs)
		made := run("deploy_want", map[string]string{"type": typ, "params": strings.Join(pairs, ", ")})
		steps := []robotStep{made}
		if made.Card == nil || strings.HasPrefix(made.Output, "Error:") {
			return steps
		}
		// What it found, when it finds it quickly (a route search: seconds);
		// otherwise the answer says it was made and the tile shows the rest.
		if s.robotWaitSettled(made.Card.ID, planResultWait) {
			steps = append(steps, run("look", map[string]string{"name": made.Card.Name, robotShortArg: "true"}))
		}
		return steps
	}
	return nil
}

// robotSaidIn: the value is in what was asked — as it is, or, for a time of
// day (10:00), its hour (「10時に」).
func robotSaidIn(v, question string) bool {
	q := robotLoose(question)
	if strings.Contains(q, robotLoose(v)) {
		return true
	}
	if m := robotClockColon.FindStringSubmatch(v); m != nil {
		return strings.Contains(question, strings.TrimLeft(m[1], "0")+"時") ||
			strings.Contains(question, strings.TrimLeft(m[1], "0")+":")
	}
	return false
}

// robotWaitSettled waits for a want to finish its first run, up to max.
func (s *Server) robotWaitSettled(id string, max time.Duration) bool {
	deadline := time.Now().Add(max)
	// A want run again reads as it was until its restart is taken up.
	time.Sleep(700 * time.Millisecond)
	for {
		var resp struct {
			Status string `json:"status"`
		}
		if err := s.backend("GET", "/api/v1/wants/"+id, nil, &resp); err == nil {
			switch resp.Status {
			case "achieved", "completed", "failed", "terminated", "cancelled":
				return true
			}
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// planCell is a tile's cell as the tools say it, which the answer must not
// read out: where it is, the robot walking there shows.
var planCell = regexp.MustCompile(`\s*(?:at\s*)?\(\d+,\s*\d+\)`)

// planFact is a step as the answer's prompt reads it.
func planFact(st robotStep) string {
	if st.Tool == "point" && st.Card != nil && !strings.HasPrefix(st.Output, "Error:") {
		// Not the robot's own line (「…はここです」), which the answer took
		// for what the want says: only that it was there, and is shown.
		return fmt.Sprintf("[shown] %q, already on the board — the robot is standing on it.", st.Card.Name)
	}
	out := planCell.ReplaceAllString(st.Output, "")
	// A capability's data is the answer's whole source: more of it is kept.
	max := 1200
	if st.Tool == "use_capability" {
		max = 4000
	}
	if len([]rune(out)) > max {
		out = string([]rune(out)[:max]) + " …"
	}
	return fmt.Sprintf("[%s] %s", st.Tool, out)
}

// robotAnswerPrompt asks for the words of the answer, from what was done.
func robotAnswerPrompt(req actRequest, facts []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "The person asked: 「%s」\n\n", req.Question)
	if req.Kind == "none" {
		b.WriteString("Reply to what they said, briefly and kindly, as a person would. Only if they ask what you can do: you show what is on the board and make wants that find things out.\n")
		// Nothing follows this reply: a promise to look ("ちょっと待って、すぐ
		// 見つけます") is left hanging, and the person waits for nothing.
		b.WriteString("Nothing was looked up or done for it, and nothing will be after this reply: never say you will look, check, find or get back to them, or ask them to wait. If they asked about something on the board, say you did not find it on the board.\n")
	} else if len(facts) == 0 {
		b.WriteString("Nothing on the board was found or made for it: say so, and that you could not find or make one for it.\n")
	} else {
		b.WriteString("What was done on the board for it, and what it says:\n")
		for _, f := range facts {
			b.WriteString(f + "\n")
		}
		b.WriteString("\nAnswer the person from these lines only: what was shown or made, and — when the lines say it — the values that answer the question (times, prices, the weather), read by what they mean. If a want was made for it ([deploy_want]) and is still working, say it was made and its result will appear on its tile; a want shown that was already there was not made — never say it was. Never add what the lines do not say.\n")
	}
	b.WriteString("One or two short sentences, spoken to the person.")
	return b.String()
}

// robotEmptyParam: a parameter left empty — nothing, "", an empty list or map.
func robotEmptyParam(v any) bool {
	switch t := v.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(t) == ""
	case []any:
		return len(t) == 0
	case map[string]any:
		return len(t) == 0
	}
	return false
}
