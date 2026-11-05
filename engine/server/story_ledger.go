package server

// story_ledger.go — every judged answer, kept in one place for good, and what
// it tells: for a kind of question, which way of answering it goes well.
//
//	~/.mywant/stories/turns.jsonl   a line a judged turn, every character's
//
// A row is written as its turn is judged (story_outcome.go) and never let
// go: the question, the plan carried out, what the answer showed, how it went
// — and two keys worked out from them:
//
//	pattern  the question with what the plan took from it put back as names:
//	         「中野坂上から銀座の乗り換え」, planned from=中野坂上 to=銀座, is
//	         「{from}から{to}の乗り換え」; numbers are {n} (「{n}時{n}分」).
//	shape    how it was answered, without the values: new transit_search(from,to),
//	         existing transit_search, existing smartgolf#rounds.
//
// Read back as patterns: the rows counted by pattern and shape, the outcomes
// of each. A question asked now finds its patterns by matching them — each
// pattern is a template, its names matching anything — so 「新宿から横浜の乗り換え」
// finds what 「中野坂上から銀座の乗り換え」 taught. None matching, the patterns
// most like it (storySimilar).
//
// Held in memory once read; a server meeting no ledger builds it from the
// turn.outcome events its stories still have.

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	ws "github.com/onelittlenightmusic/want-spec"
)

const storyLedgerFile = "turns.jsonl"

// storyLedgerRow is a judged turn, as the ledger keeps it.
type storyLedgerRow struct {
	ws.StoryTurn `yaml:",inline"`
	Character    string `yaml:"character" json:"character"`
	Pattern      string `yaml:"pattern" json:"pattern"`
	Shape        string `yaml:"shape" json:"shape"`
	// JudgedAt is when the outcome was decided.
	JudgedAt string `yaml:"judgedAt" json:"judgedAt"`
}

type storyLedger struct {
	mu      sync.Mutex
	loaded  bool
	rows    []storyLedgerRow
	seen    map[string]bool // turn ids
	pending []storyLedgerRow
	// matchers: each pattern compiled once.
	matchers map[string]*regexp.Regexp
}

// ledgerPath is the ledger's file.
func (r *storyRecorder) ledgerPath() string {
	return filepath.Join(r.root(), storyLedgerFile)
}

// ledgerLoad reads the ledger once — or, none there, builds it from the
// stories' turn.outcome events. Caller holds l.mu.
func (r *storyRecorder) ledgerLoad() {
	l := &r.ledger
	if l.loaded {
		return
	}
	l.loaded = true
	l.seen = map[string]bool{}
	l.matchers = map[string]*regexp.Regexp{}
	f, err := os.Open(r.ledgerPath())
	if err == nil {
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
		for sc.Scan() {
			var row storyLedgerRow
			if json.Unmarshal(sc.Bytes(), &row) == nil && !l.seen[row.Turn] {
				l.seen[row.Turn] = true
				l.rows = append(l.rows, row)
			}
		}
		f.Close()
		return
	}
	// No ledger yet: what the stories still say.
	characters, err := os.ReadDir(r.root())
	if err != nil {
		return
	}
	for _, c := range characters {
		if !c.IsDir() || !storyCharacterOK.MatchString(c.Name()) {
			continue
		}
		dir := filepath.Join(r.root(), c.Name())
		files, _ := os.ReadDir(dir)
		for _, file := range files {
			name := file.Name()
			if !strings.HasSuffix(name, ".jsonl") && !strings.HasSuffix(name, ".jsonl.gz") {
				continue
			}
			_ = readStoryFile(filepath.Join(dir, name), func(ev ws.StoryEvent) {
				if ev.Kind != ws.StoryKindTurnOutcome || l.seen[ev.Ref] {
					return
				}
				row := storyRowOf(ev)
				l.seen[row.Turn] = true
				l.rows = append(l.rows, row)
				l.pending = append(l.pending, row)
			})
		}
	}
	sort.SliceStable(l.pending, func(i, j int) bool { return l.pending[i].JudgedAt < l.pending[j].JudgedAt })
}

// ledgerAdd keeps a judged turn (its turn.outcome event).
func (r *storyRecorder) ledgerAdd(ev ws.StoryEvent) {
	l := &r.ledger
	l.mu.Lock()
	defer l.mu.Unlock()
	r.ledgerLoad()
	if l.seen[ev.Ref] {
		return
	}
	row := storyRowOf(ev)
	l.seen[row.Turn] = true
	l.rows = append(l.rows, row)
	l.pending = append(l.pending, row)
}

// ledgerFlush writes the rows not yet written.
func (r *storyRecorder) ledgerFlush() error {
	l := &r.ledger
	l.mu.Lock()
	pending := l.pending
	l.pending = nil
	l.mu.Unlock()
	if len(pending) == 0 {
		return nil
	}
	if err := os.MkdirAll(r.root(), 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(r.ledgerPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(f)
	enc := json.NewEncoder(w)
	for _, row := range pending {
		if err := enc.Encode(row); err != nil {
			f.Close()
			return err
		}
	}
	if err := w.Flush(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// storyRowOf is a turn.outcome event as a ledger row.
func storyRowOf(ev ws.StoryEvent) storyLedgerRow {
	d := ev.Detail
	row := storyLedgerRow{Character: ev.Character, JudgedAt: ev.At}
	row.Turn, row.At, row.Question, row.Outcome = ev.Ref, d["asked"], d["question"], d["outcome"]
	if row.At == "" {
		row.At = ev.At
	}
	if s := d["shown"]; s != "" {
		for _, id := range strings.Split(s, ",") {
			row.Shown = append(row.Shown, "want:"+id)
		}
	}
	if row.Outcome == ws.TurnOutcomeCorrected && ev.To != nil {
		p := storyPlaceOnly(ev.To)
		row.CorrectedTo = &p
	}
	if n, err := time.ParseDuration(d["seconds"] + "s"); err == nil {
		row.SecondsToAct = int(n / time.Second)
	}
	if d["plan.action"] != "" {
		p := &ws.PlanSpec{Question: row.Question, Action: d["plan.action"], Target: d["plan.target"],
			Capability: d["plan.capability"], Type: d["plan.type"]}
		for k, v := range d {
			if name, ok := strings.CutPrefix(k, "plan.param."); ok {
				if p.Params == nil {
					p.Params = map[string]string{}
				}
				p.Params[name] = v
			}
		}
		row.Plan = p
	}
	row.Pattern = storyPattern(row.Question, row.Plan)
	row.Shape = storyShape(row.Plan, d["plan.targetType"])
	return row
}

// ── patterns ───────────────────────────────────────────────────────────────

// storyNormal is a question as patterns are made and matched: no spaces, no
// punctuation, letters lowered.
func storyNormal(s string) string {
	var b strings.Builder
	for _, c := range strings.ToLower(s) {
		if unicode.IsSpace(c) || unicode.IsPunct(c) || unicode.IsSymbol(c) {
			continue
		}
		b.WriteRune(c)
	}
	return b.String()
}

// storyPattern is a question with what its plan took from it put back as
// names ({from}, {to}, {target}), and its numbers as {n}.
func storyPattern(question string, plan *ws.PlanSpec) string {
	q := storyNormal(question)
	type value struct{ name, text string }
	var values []value
	if plan != nil {
		for k, v := range plan.Params {
			if v = storyNormal(v); v != "" {
				values = append(values, value{k, v})
			}
		}
		if t := storyNormal(plan.Target); t != "" {
			values = append(values, value{"target", t})
		}
	}
	// The longest first: 「中野坂上」 before a 「中野」 inside it.
	sort.SliceStable(values, func(i, j int) bool { return len(values[i].text) > len(values[j].text) })
	// Names held as \x00name\x00 while values are put back, so a later value
	// is not found inside one.
	for _, v := range values {
		if strings.Contains(q, v.text) {
			q = strings.ReplaceAll(q, v.text, "\x00"+v.name+"\x00")
		}
	}
	var b strings.Builder
	parts := strings.Split(q, "\x00")
	for i, p := range parts {
		if i%2 == 1 {
			b.WriteString("{" + p + "}")
			continue
		}
		inNumber := false
		for _, c := range p {
			if unicode.IsDigit(c) {
				if !inNumber {
					b.WriteString("{n}")
				}
				inNumber = true
				continue
			}
			inNumber = false
			b.WriteRune(c)
		}
	}
	return b.String()
}

// storyShape is how a turn was answered, without the values.
func storyShape(plan *ws.PlanSpec, targetType string) string {
	if plan == nil || plan.Action == "" {
		return "-"
	}
	switch plan.Action {
	case "new":
		keys := make([]string, 0, len(plan.Params))
		for k, v := range plan.Params {
			if v != "" {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		return "new " + plan.Type + "(" + strings.Join(keys, ",") + ")"
	case "existing":
		s := "existing"
		if targetType != "" {
			s += " " + targetType
		}
		if plan.Capability != "" {
			s += "#" + plan.Capability
		}
		return s
	}
	return plan.Action
}

var storyPatternName = regexp.MustCompile(`\{[^{}]+\}`)

// storyMatcher is a pattern as a template: its names match anything, {n} a
// number.
func storyMatcher(pattern string) *regexp.Regexp {
	var b strings.Builder
	b.WriteString("^")
	last := 0
	for _, m := range storyPatternName.FindAllStringIndex(pattern, -1) {
		b.WriteString(regexp.QuoteMeta(pattern[last:m[0]]))
		if pattern[m[0]:m[1]] == "{n}" {
			b.WriteString(`\d+`)
		} else {
			b.WriteString(`(.+?)`)
		}
		last = m[1]
	}
	b.WriteString(regexp.QuoteMeta(pattern[last:]))
	b.WriteString("$")
	return regexp.MustCompile(b.String())
}

// ── read back ──────────────────────────────────────────────────────────────

// storyAnswerStat is one way of answering a pattern, and how it went.
type storyAnswerStat struct {
	Shape    string         `yaml:"shape" json:"shape"`
	Count    int            `yaml:"count" json:"count"`
	Outcomes map[string]int `yaml:"outcomes" json:"outcomes"`
	// SuccessRate is success / count.
	SuccessRate float64 `yaml:"successRate" json:"successRate"`
	// Example is its latest success: the question and the plan carried out.
	Example *ws.PlanSpec `yaml:"example,omitempty" json:"example,omitempty"`
}

// storyPatternStat is a kind of question, and the ways it was answered.
type storyPatternStat struct {
	Pattern string `yaml:"pattern" json:"pattern"`
	// Declared is the want type's worked example this kind of question is
	// (robot-qa/…); nil, a kind no type names yet — a worked example to add.
	Declared *storyQA `yaml:"declared,omitempty" json:"declared,omitempty"`
	// Match is how it was found for the question asked: "pattern" (the
	// question fits it), "similar", or "" when none was asked.
	Match   string            `yaml:"match,omitempty" json:"match,omitempty"`
	Count   int               `yaml:"count" json:"count"`
	Answers []storyAnswerStat `yaml:"answers" json:"answers"`
	LastAt  string            `yaml:"lastAt" json:"lastAt"`
}

// storyPatternQuery is what a patterns read asks for.
type storyPatternQuery struct {
	question  string // the patterns it fits (else the most like it); "" all
	character string // only this character's rows; "" everyone's
	limit     int
	// qas are the want types' worked examples (robot-qa/…): the kinds of
	// question the types say they answer, matched before a row's own pattern.
	qas []storyQA
}

// storyQA is a want type's worked example — "robot-qa/中野坂上から銀座の乗り換え":
// "deploy_want transit_search with params from=中野坂上, to=銀座" — as a kind
// of question: its request with its own values put back as names.
type storyQA struct {
	Type     string `yaml:"type" json:"type"`
	Request  string `yaml:"request" json:"request"`
	Answer   string `yaml:"answer" json:"answer"`
	Template string `yaml:"template" json:"template"`
	matcher  *regexp.Regexp
}

var storyQAParams = regexp.MustCompile(`([A-Za-z_][A-Za-z0-9_]*)=([^,]+)`)

// newStoryQA is a worked example as a kind of question.
func newStoryQA(typ, request, answer string) storyQA {
	plan := &ws.PlanSpec{Params: map[string]string{}}
	if i := strings.Index(answer, "with params"); i >= 0 {
		for _, m := range storyQAParams.FindAllStringSubmatch(answer[i+len("with params"):], -1) {
			plan.Params[m[1]] = strings.TrimSpace(m[2])
		}
	}
	tmpl := storyPattern(request, plan)
	return storyQA{Type: typ, Request: request, Answer: answer, Template: tmpl, matcher: storyMatcher(tmpl)}
}

// declared is the worked example a question is one of, or nil.
func storyDeclared(qas []storyQA, question, pattern string) *storyQA {
	asked := storyNormal(question)
	for i := range qas {
		if qas[i].Template == pattern || qas[i].matcher.MatchString(asked) {
			return &qas[i]
		}
	}
	return nil
}

// patterns is the ledger counted by pattern and shape.
func (r *storyRecorder) patterns(q storyPatternQuery) []storyPatternStat {
	l := &r.ledger
	l.mu.Lock()
	defer l.mu.Unlock()
	r.ledgerLoad()

	// Each row under the worked example it is one of, else its own pattern.
	byPattern := map[string][]storyLedgerRow{}
	declared := map[string]*storyQA{}
	for _, row := range l.rows {
		if q.character != "" && row.Character != q.character {
			continue
		}
		key := row.Pattern
		if qa := storyDeclared(q.qas, row.Question, row.Pattern); qa != nil {
			key = qa.Template
			declared[key] = qa
		}
		byPattern[key] = append(byPattern[key], row)
	}
	match := map[string]string{}
	if q.question != "" {
		asked := storyNormal(q.question)
		for p := range byPattern {
			m := l.matchers[p]
			if m == nil {
				m = storyMatcher(p)
				l.matchers[p] = m
			}
			if m.MatchString(asked) {
				match[p] = "pattern"
			}
		}
		if len(match) == 0 {
			for p := range byPattern {
				if storySimilar(storyPatternName.ReplaceAllString(p, ""), asked) >= 0.3 {
					match[p] = "similar"
				}
			}
		}
	}

	var out []storyPatternStat
	for p, rows := range byPattern {
		if q.question != "" && match[p] == "" {
			continue
		}
		ps := storyPatternStat{Pattern: p, Declared: declared[p], Match: match[p], Count: len(rows)}
		shapes := map[string]*storyAnswerStat{}
		for _, row := range rows {
			s := shapes[row.Shape]
			if s == nil {
				s = &storyAnswerStat{Shape: row.Shape, Outcomes: map[string]int{}}
				shapes[row.Shape] = s
			}
			s.Count++
			s.Outcomes[row.Outcome]++
			if row.Outcome == ws.TurnOutcomeSuccess && row.Plan != nil {
				s.Example = row.Plan // rows are oldest first: the latest stays
			}
			if row.JudgedAt > ps.LastAt {
				ps.LastAt = row.JudgedAt
			}
		}
		for _, s := range shapes {
			s.SuccessRate = float64(s.Outcomes[ws.TurnOutcomeSuccess]) / float64(s.Count)
			ps.Answers = append(ps.Answers, *s)
		}
		sort.Slice(ps.Answers, func(i, j int) bool {
			a, b := ps.Answers[i], ps.Answers[j]
			if a.SuccessRate != b.SuccessRate {
				return a.SuccessRate > b.SuccessRate
			}
			return a.Count > b.Count
		})
		out = append(out, ps)
	}
	sort.Slice(out, func(i, j int) bool {
		if (out[i].Match == "pattern") != (out[j].Match == "pattern") {
			return out[i].Match == "pattern"
		}
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Pattern < out[j].Pattern
	})
	if q.limit > 0 && len(out) > q.limit {
		out = out[:q.limit]
	}
	return out
}
