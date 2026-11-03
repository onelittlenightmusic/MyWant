package server

// plan.go — the robot's turn as a plan, not a loop of tools.
//
// What the person asks the robot for is information, and on the board
// information is a want: one already there that answers it, or one made for
// it. So the model is asked only what a model is needed for, and everything
// else is done here, the same way every time:
//
//	POST /plan/brief       what is on the board that may answer it, and the
//	                     types that could make one → the decision's prompt
//	                     and its choices (an existing want or thing by name,
//	                     a type to make, or none — not about the board)
//	POST /plan/params  a chosen type's parameters → the prompt to fill them
//	POST /plan/act         the decision carried out (shown, brought back from
//	                     the archive, or made — a want with the same
//	                     parameters shown instead of made twice), recorded as
//	                     the turn's steps → the prompt for the answer, from
//	                     what was done and found
//
// At most three generations a turn, none of them free to call a tool again:
// with tools of its own a small model went round describe_type and
// deploy_want five times after the want was made (fly, 2026-11-02: four
// minutes for a six-second search), and answered with the turn before's name.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	fmPlanCandidates = 10 // board names offered to the decision
	fmPlanTypes      = 8  // types offered with what they are
	fmPlanResultWait = 10 * time.Second
	// fmPlanNone is the choice that is no choice, in an enum.
	fmPlanNone = "-"
)

type fmBriefRequest struct {
	Question string `json:"question"`
	Locale   string `json:"locale"`
}

type fmBriefResponse struct {
	Prompt string   `json:"prompt"`
	Refs   []string `json:"refs"`
	Types  []string `json:"types"`
	// What may be asked of the want chosen as ref (tools_capability.go): the
	// standard capabilities and those of the candidates' types, by name; the
	// first, "-", is just to show it.
	Capabilities []string `json:"capabilities"`
}

type fmCandidate struct {
	name, line string
	score      int
	// A want (not a thing): it has the standard capabilities.
	want bool
}

// POST /api/v1/plan/brief
func (s *Server) handleFMBrief(w http.ResponseWriter, r *http.Request) {
	var req fmBriefRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}
	brief, err := s.fmBrief(req.Question, req.Locale)
	if err != nil {
		http.Error(w, "backend: "+err.Error(), http.StatusBadGateway)
		return
	}
	fmWriteJSON(w, brief)
}

func (s *Server) fmBrief(question, locale string) (fmBriefResponse, error) {
	cands, err := s.fmCandidates(question)
	if err != nil {
		return fmBriefResponse{}, err
	}
	allTypes, err := s.fmTypeNames()
	if err != nil {
		return fmBriefResponse{}, err
	}
	typesText := s.fmPlanTypesText(question)

	var b strings.Builder
	fmt.Fprintf(&b, "The person asked: 「%s」\n\n", question)
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
	for _, c := range s.fmCapabilities() {
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
	caps := []string{fmPlanNone}
	b.WriteString("\nWhat any want above can do (capability):\n")
	for _, c := range fmStdCaps {
		fmt.Fprintf(&b, "- %s: %s\n", c.Name, firstLine(c.Description, 160))
		caps = append(caps, c.Name)
	}
	for _, n := range capNames {
		caps = append(caps, n)
	}
	b.WriteString("\nSet the fields you do not use to " + fmPlanNone + ".")

	refs := []string{fmPlanNone}
	seen := map[string]bool{}
	for _, c := range cands {
		if !seen[c.name] {
			seen[c.name] = true
			refs = append(refs, c.name)
		}
	}
	return fmBriefResponse{
		Prompt:       b.String() + fmLocaleText(locale),
		Refs:         refs,
		Types:        append([]string{fmPlanNone}, allTypes...),
		Capabilities: caps,
	}, nil
}

// fmCandidates is what on the board (and in the archive) may be about the
// question, the likeliest first: named in it, called by a word of its line in
// the glossary, its type called by an alias, its parameters said in it.
func (s *Server) fmCandidates(question string) ([]fmCandidate, error) {
	tiles, err := s.fmTiles()
	if err != nil {
		return nil, err
	}
	wants, err := s.fmWantList()
	if err != nil {
		return nil, err
	}
	paramsOf := map[string]map[string]any{}
	for _, w := range wants {
		paramsOf[w.Metadata.Name] = w.Spec.Params
	}
	said := fmLoose(question)
	glossary := s.fmGlossary()
	aliases := s.fmTypeAliases()

	var out []fmCandidate
	// Its name or its parameters must be in what was said; its type called
	// by an alias only puts it higher — every route ever searched is not
	// about this one.
	score := func(name, typ string, params map[string]any) int {
		sc := 0
		if n := fmLoose(name); n != "" && (strings.Contains(said, n) || strings.Contains(n, said)) {
			sc += 3
		} else if fmNamePartSaid(name, said) {
			sc += 3 // 「Nakanoの天気」 for NakanoのWeather
		}
		asked := 0
		for _, v := range params {
			if fmEmptyParam(v) {
				continue // a checklist's items: [] was not asked of it
			}
			t := fmLoose(fmt.Sprint(v))
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
		if asked == 0 && typ != "" && fmAliasScore(question, aliases[typ]) > 0 {
			sc += 3
		}
		return sc
	}
	// Ranked higher, never let in, by its type being called by an alias.
	rank := func(sc int, typ string) int {
		if typ != "" && fmAliasScore(question, aliases[typ]) > 0 {
			return sc + 2
		}
		return sc
	}
	names := make([]string, len(tiles))
	for i, t := range tiles {
		names[i] = t.name
	}
	byGlossary := map[int]bool{}
	for _, i := range fmByGlossary(question, names, glossary) {
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
			line += " (" + fmParamsText(p) + ")"
		}
		out = append(out, fmCandidate{name: t.name, line: line, score: rank(sc, t.typ), want: t.cardKind == "want"})
	}
	for _, w := range wants {
		if !w.archived() || w.Metadata.Name == fmRobotWant {
			continue
		}
		sc := score(w.Metadata.Name, w.Metadata.Type, w.Spec.Params)
		if sc < 4 {
			continue // put away: only one that is plainly this question
		}
		line := fmt.Sprintf("- %s: want %s, put away", w.Metadata.Name, w.Metadata.Type)
		if len(w.Spec.Params) > 0 {
			line += " (" + fmParamsText(w.Spec.Params) + ")"
		}
		out = append(out, fmCandidate{name: w.Metadata.Name, line: line, score: rank(sc, w.Metadata.Type) - 1})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].score > out[j].score })
	if len(out) > fmPlanCandidates {
		out = out[:fmPlanCandidates]
	}
	return out, nil
}

// fmNamePartSaid: a part of the name (split at の, spaces and dashes) of
// three letters or more is in what was said.
func fmNamePartSaid(name, said string) bool {
	parts := strings.FieldsFunc(strings.ToLower(name), func(r rune) bool {
		return r == 'の' || r == ' ' || r == '　' || r == '-' || r == '_' || r == '・'
	})
	if len(parts) < 2 {
		return false
	}
	for _, p := range parts {
		if len([]rune(p)) >= 3 && strings.Contains(said, fmLoose(p)) {
			return true
		}
	}
	return false
}

// fmPlanTypesText is the types the question may be for, with what they are
// and what they say about their questions: called by an alias or their title
// first, then those that say how their questions go.
func (s *Server) fmPlanTypesText(question string) string {
	types, err := s.fmWantTypes()
	if err != nil {
		return ""
	}
	aliases := s.fmTypeAliases()
	said := fmLoose(question)
	type ranked struct {
		t     fmWantType
		score int
	}
	var rs []ranked
	for _, t := range types {
		sc := fmAliasScore(question, aliases[t.Name]) * 2
		if title := fmLoose(t.Title); title != "" && strings.Contains(said, title) {
			sc += 2
		}
		if fmTypeHint(t.Labels) != "" {
			sc++
		}
		if sc > 0 {
			rs = append(rs, ranked{t, sc})
		}
	}
	sort.SliceStable(rs, func(i, j int) bool { return rs[i].score > rs[j].score })
	var b strings.Builder
	for i, r := range rs {
		if i == fmPlanTypes {
			break
		}
		fmt.Fprintf(&b, "- %s: %s\n", r.t.Name, firstLine(r.t.Title, 80))
		if h := fmPlanHint(fmTypeHint(r.t.Labels)); h != "" && b.Len()+len(h) < fmHintsMax {
			b.WriteString("  " + strings.ReplaceAll(h, "\n", "\n  ") + "\n")
		}
	}
	return b.String()
}

// fmPlanHint is a type's hint as the plan reads it: written for the robot's
// old tools, whose names mean nothing here.
func fmPlanHint(h string) string {
	return strings.NewReplacer("deploy_want ", "make ", "deploy it", "make it").Replace(h)
}

// ── type-brief ──────────────────────────────────────────────────────────────

type fmTypeBriefRequest struct {
	Question string `json:"question"`
	Type     string `json:"type"`
	Locale   string `json:"locale"`
}

type fmPlanParam struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

// POST /api/v1/plan/params
func (s *Server) handleFMTypeBrief(w http.ResponseWriter, r *http.Request) {
	var req fmTypeBriefRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}
	name, err := s.fmResolveType(req.Type)
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
	if h := fmPlanHint(fmTypeHint(resp.Metadata.Labels)); h != "" {
		b.WriteString(h + "\n")
	}
	b.WriteString("Fill its parameters from what the person said. Leave a parameter empty when they did not say it — never make a value up.\n")
	var params []fmPlanParam
	for i, p := range resp.Parameters {
		if i == 10 {
			break
		}
		desc := firstLine(p.Description, 120)
		if p.Example != nil {
			desc += fmt.Sprintf(" e.g. %v", p.Example)
		}
		params = append(params, fmPlanParam{Name: p.Name, Description: desc, Required: p.Required})
	}
	fmWriteJSON(w, map[string]any{"prompt": b.String() + fmLocaleText(req.Locale), "type": name, "params": params})
}

// ── act ─────────────────────────────────────────────────────────────────────

type fmActRequest struct {
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

// POST /api/v1/plan/act
func (s *Server) handleFMAct(w http.ResponseWriter, r *http.Request) {
	var req fmActRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Question == "" && req.Turn != "" {
		req.Question = s.fmTurnQuestion(req.Turn)
	}
	steps := s.fmAct(req)
	if req.Kind == "new" && len(steps) == 0 {
		// Nothing of a want was said (「こんにちは」 taken for a route): it
		// was not about the board after all.
		req.Kind = "none"
	}
	var cards []*fmCard
	shown := map[string]bool{}
	facts := make([]string, 0, len(steps))
	for _, st := range steps {
		if st.Card != nil && !shown[st.Card.ID] {
			shown[st.Card.ID] = true
			cards = append(cards, st.Card)
		}
		facts = append(facts, fmPlanFact(st))
	}
	if req.Turn != "" && len(steps) > 0 {
		if _, err := s.fmUpdateTurn(req.Turn, func(t *fmTurn) error { t.Steps = append(t.Steps, steps...); return nil }); err == nil {
			for _, st := range steps {
				s.fmShowStep(st, "")
			}
		}
	}
	fmWriteJSON(w, map[string]any{"prompt": fmAnswerPrompt(req, facts) + fmLocaleText(req.Locale), "cards": cards, "steps": steps})
}

// fmAct carries the decision out, as the steps the old tools would have
// taken — the same tools, so the board, the turn and its cards are as before.
func (s *Server) fmAct(req fmActRequest) []fmStep {
	run := func(tool string, args map[string]string) fmStep {
		if fmHearsQuestion(tool) && req.Question != "" {
			args[fmQuestionArg] = req.Question
		}
		st := fmStep{Tool: tool, Arguments: args}
		s.fmRunStep(&st)
		delete(st.Arguments, fmQuestionArg)
		delete(st.Arguments, fmShortArg)
		st.Card = fmTakeCard(st.Arguments)
		return st
	}
	switch req.Kind {
	case "existing":
		name := strings.TrimSpace(req.Ref)
		if name == "" || name == fmPlanNone {
			return nil
		}
		if c := strings.TrimSpace(req.Capability); c != "" && c != fmPlanNone {
			// A capability of the want (tools_capability.go): done here, what it
			// found handed to the answer.
			return []fmStep{run("use_capability", map[string]string{"capability": c, "want": name})}
		}
		if strings.Contains(name, fmCapSep) {
			// One chosen as want#capability (the tool loop's spelling).
			return []fmStep{run("use_capability", map[string]string{"capability": name})}
		}
		var steps []fmStep
		if _, archived := s.fmArchivedNamed(name); archived {
			// Asked for by name: brought back, not asked about again.
			steps = append(steps, run("restore_want", map[string]string{"name": name}))
		}
		steps = append(steps, run("point", map[string]string{"name": name}))
		if strings.HasPrefix(steps[len(steps)-1].Output, "Error:") {
			return steps
		}
		return append(steps, run("look", map[string]string{"name": name, fmShortArg: "true"}))
	case "new":
		typ := strings.TrimSpace(req.Type)
		if typ == "" || typ == fmPlanNone {
			return nil
		}
		// Only what the person said: a small model fills a greeting's
		// 「こんにちは」 with the type's example (中野坂上→銀座). A want with
		// parameters, none of them said, is not asked for — nothing is made.
		var pairs []string
		for k, v := range req.Params {
			if v = strings.TrimSpace(v); v != "" && v != fmPlanNone && fmSaidIn(v, req.Question) {
				pairs = append(pairs, k+"="+v)
			}
		}
		if len(pairs) == 0 && len(req.Params) > 0 {
			return nil
		}
		sort.Strings(pairs)
		made := run("deploy_want", map[string]string{"type": typ, "params": strings.Join(pairs, ", ")})
		steps := []fmStep{made}
		if made.Card == nil || strings.HasPrefix(made.Output, "Error:") {
			return steps
		}
		// What it found, when it finds it quickly (a route search: seconds);
		// otherwise the answer says it was made and the tile shows the rest.
		if s.fmWaitSettled(made.Card.ID, fmPlanResultWait) {
			steps = append(steps, run("look", map[string]string{"name": made.Card.Name, fmShortArg: "true"}))
		}
		return steps
	}
	return nil
}

// fmSaidIn: the value is in what was asked — as it is, or, for a time of
// day (10:00), its hour (「10時に」).
func fmSaidIn(v, question string) bool {
	q := fmLoose(question)
	if strings.Contains(q, fmLoose(v)) {
		return true
	}
	if m := fmClockColon.FindStringSubmatch(v); m != nil {
		return strings.Contains(question, strings.TrimLeft(m[1], "0")+"時") ||
			strings.Contains(question, strings.TrimLeft(m[1], "0")+":")
	}
	return false
}

// fmWaitSettled waits for a want to finish its first run, up to max.
func (s *Server) fmWaitSettled(id string, max time.Duration) bool {
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

// fmPlanCell is a tile's cell as the tools say it, which the answer must not
// read out: where it is, the robot walking there shows.
var fmPlanCell = regexp.MustCompile(`\s*(?:at\s*)?\(\d+,\s*\d+\)`)

// fmPlanFact is a step as the answer's prompt reads it.
func fmPlanFact(st fmStep) string {
	if st.Tool == "point" && st.Card != nil && !strings.HasPrefix(st.Output, "Error:") {
		// Not the robot's own line (「…はここです」), which the answer took
		// for what the want says: only that it was there, and is shown.
		return fmt.Sprintf("[shown] %q, already on the board — the robot is standing on it.", st.Card.Name)
	}
	out := fmPlanCell.ReplaceAllString(st.Output, "")
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

// fmAnswerPrompt asks for the words of the answer, from what was done.
func fmAnswerPrompt(req fmActRequest, facts []string) string {
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
		b.WriteString("\nAnswer the person from these lines only: what was shown or made, and — when the lines say it — the values that answer the question (times, prices, the weather), read by what they mean. If a want is still working, say it was made and its result will appear on its tile. Never add what the lines do not say.\n")
	}
	b.WriteString("One or two short sentences, spoken to the person.")
	return b.String()
}

// fmEmptyParam: a parameter left empty — nothing, "", an empty list or map.
func fmEmptyParam(v any) bool {
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
