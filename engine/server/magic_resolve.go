package server

// Which of Laya's three likeliest sentences a request is.
//
// Laya says which sentences it likeliest is (magic_laya.go); thirty types,
// some alike — a route and a transit search are both 「AからB」 — leave it
// often not sure of one. So each of the three is weighed again by what was
// said: how like that type's asks (its ways of being asked for) the request
// is, its numbers and places taken out, beside how likely Laya said it was;
// and whether the model can read what the type cannot be made without (its
// required parameters). The best that can be made is the one. Two that can,
// alike, are put to the person (「電車の乗り換えですか？歩く道順ですか？」),
// and the answer done — and kept, for Laya to learn from
// (~/.mywant/laya-clarified.jsonl, laya_readings_train.go).

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// resolveMargin is how far ahead the best must be of the next that can be
// made, else the person is asked which.
const resolveMargin = 0.1

// resolveAskFor is how long a question put to the person waits for an answer.
const resolveAskFor = 5 * time.Minute

var resolveNumber = regexp.MustCompile(`[0-9０-９]+`)

// sayShape is what was said with its numbers and the values given taken out:
// 「15分後に中野から渋谷に行きたい」 is 「○分後に○から○に行きたい」.
func sayShape(said string, values []string) string {
	sort.SliceStable(values, func(i, j int) bool { return len(values[i]) > len(values[j]) })
	for _, v := range values {
		if v = strings.TrimSpace(v); len([]rune(v)) >= 2 {
			said = strings.ReplaceAll(said, v, "○")
		}
	}
	return resolveNumber.ReplaceAllString(said, "○")
}

// shapeSimilarity is how alike two shapes are: the share of their two-letter
// pieces they have in common.
func shapeSimilarity(a, b string) float64 {
	grams := func(s string) map[string]bool {
		r := []rune(s)
		out := map[string]bool{}
		for i := 0; i+1 < len(r); i++ {
			out[string(r[i:i+2])] = true
		}
		if len(out) == 0 {
			out[s] = true
		}
		return out
	}
	A, B := grams(a), grams(b)
	both := 0
	for g := range A {
		if B[g] {
			both++
		}
	}
	all := len(A) + len(B) - both
	if all == 0 {
		return 0
	}
	return float64(both) / float64(all)
}

// candidateShapes are the ways a candidate is said, as shapes: a type's asks
// (each alias said), its sentences' slots taken out; a robot tool's sentences.
func (s *Server) candidateShapes(c magicMatch) []string {
	var out []string
	sentences := c.src.tmpls
	if c.tool != "" {
		sentences = robotToolByName(c.tool).Sentence
	}
	for _, tmpl := range strings.FieldsFunc(sentences, func(r rune) bool { return r == '\n' || r == '|' || r == '｜' }) {
		if strings.Contains(tmpl, magicAliasSlot) {
			continue
		}
		out = append(out, magicSlot.ReplaceAllString(tmpl, "○"))
	}
	if c.tool == "" {
		if def := s.typeDefinition(c.wantType); def != nil {
			aliases := cleanAliases(strings.Split(def.Metadata.Labels[robotAliasLabel], ","))
			for _, a := range expandAsks(def.Asks, aliases) {
				var values []string
				for _, v := range a.Params {
					values = append(values, fmt.Sprint(v))
				}
				out = append(out, sayShape(a.Say, values))
			}
		}
	}
	return out
}

// askSimilarity is how like a candidate's ways of being said the request is.
func (s *Server) askSimilarity(said string, c magicMatch, things []string) float64 {
	shape := sayShape(said, things)
	best := 0.0
	for _, sh := range s.candidateShapes(c) {
		best = max(best, shapeSimilarity(shape, sh))
	}
	return best
}

// boardWords are the things' names on the board: the places and names a
// request says, taken out of its shape.
func (s *Server) boardWords() []string {
	var out []string
	for _, t := range s.thingFulls() {
		out = append(out, t.Value)
	}
	return out
}

// candidateMadeOf: the model read what the candidate cannot be done without —
// a tool, every slot; a type, every required parameter it reads (where the
// person is fills its here-param, and a time is Laya's).
func (s *Server) candidateMadeOf(c magicMatch) bool {
	said := func(p string) bool { return strings.TrimSpace(c.params[p]) != "" }
	if c.tool != "" {
		for _, sl := range c.slots {
			if !said(sl.param) {
				return false
			}
		}
		return true
	}
	def := s.typeDefinition(c.wantType)
	if def == nil {
		return false
	}
	timeParam, sideParam, _ := timeReadingOf(def.Metadata.Labels)
	here := def.Metadata.Labels[robotHereLabel]
	for _, p := range def.Parameters {
		if !p.Required || p.Name == here || p.Name == timeParam || p.Name == sideParam {
			continue
		}
		if !said(p.Name) {
			return false
		}
	}
	return true
}

// Outcomes of magicResolve.
const (
	resolveMatched = "matched" // one, to be done
	resolveAsk     = "ask"     // two alike: the person is asked which
	resolveNone    = "none"    // none can be made of what was said
)

// magicResolve settles which of Laya's three likeliest the request is (or,
// matched as written, reads what fills it): the one, with what the model
// read for it — or, two alike, both, to ask which.
func (s *Server) magicResolve(said string, mm magicMatch) (magicMatch, string, []magicMatch) {
	if !mm.byLaya || len(mm.candidates) == 0 {
		s.magicModelParams(said, &mm)
		return mm, resolveMatched, nil
	}
	things := s.boardWords()
	cands := make([]magicMatch, len(mm.candidates))
	copy(cands, mm.candidates)
	for i := range cands {
		cands[i].params = map[string]string{}
		cands[i].score = (cands[i].p + s.askSimilarity(said, cands[i], things)) / 2
	}
	sort.SliceStable(cands, func(i, j int) bool { return cands[i].score > cands[j].score })
	var made []magicMatch
	for _, c := range cands {
		s.magicModelParams(said, &c)
		if s.candidateMadeOf(c) {
			made = append(made, c)
			if len(made) == 2 {
				break
			}
		}
	}
	var trace []string
	for _, c := range cands {
		trace = append(trace, fmt.Sprintf("%s=%.2f", c.src.key(), c.score))
	}
	fields := map[string]any{"weighed": strings.Join(trace, ","), "made_of": len(made)}
	outcome := resolveNone
	switch {
	case len(made) == 1, len(made) == 2 && made[0].score-made[1].score >= resolveMargin:
		outcome = resolveMatched
		fields["chose"] = made[0].src.key()
	case len(made) == 2:
		outcome = resolveAsk
	}
	fields["outcome"] = outcome
	s.robotChatTrace("laya.resolve", fields, []string{"outcome", "chose", "weighed", "made_of"})
	switch outcome {
	case resolveMatched:
		return made[0], outcome, nil
	case resolveAsk:
		return made[0], outcome, made
	}
	return mm, outcome, nil
}

// candidateLabel is how a candidate is named to the person.
func (s *Server) candidateLabel(c magicMatch) string {
	switch c.tool {
	case "point":
		return "場所を見せる"
	case "look":
		return "様子を聞く"
	case "":
	default:
		return c.tool
	}
	if def := s.typeDefinition(c.wantType); def != nil {
		for _, a := range cleanAliases(strings.Split(def.Metadata.Labels[robotAliasLabel], ",")) {
			if strings.IndexFunc(a, func(r rune) bool { return r > 0x7f }) >= 0 {
				return a
			}
		}
		if def.Metadata.Title != "" {
			return def.Metadata.Title
		}
	}
	return c.wantType
}

// A question put to the person: which of two a request was.
type resolvePending struct {
	said, turn, locale, who string
	cands                   []magicMatch
	at                      time.Time
}

var (
	resolveMu      sync.Mutex
	resolveWaiting = map[string]resolvePending{} // by who asked ("" unknown)
)

// resolveQuestion is the question put to the person, and kept to be answered.
func (s *Server) resolveQuestion(said, turn, locale, who string, cands []magicMatch) string {
	resolveMu.Lock()
	resolveWaiting[who] = resolvePending{said: said, turn: turn, locale: locale, who: who, cands: cands, at: time.Now()}
	resolveMu.Unlock()
	var names []string
	for _, c := range cands {
		names = append(names, s.candidateLabel(c))
	}
	return fmt.Sprintf("「%s」は、%sですか？それとも%sですか？", said, names[0], strings.Join(names[1:], "、"))
}

// resolveAnswered is the answer to a question put to who: the candidate the
// person named (its name, or 1 / 2), with what was asked. ok false: no
// question waiting, or this is not an answer to it — which leaves it.
func (s *Server) resolveAnswered(answer, who string) (resolvePending, magicMatch, bool) {
	resolveMu.Lock()
	defer resolveMu.Unlock()
	p, waiting := resolveWaiting[who]
	if !waiting || time.Since(p.at) > resolveAskFor {
		delete(resolveWaiting, who)
		return p, magicMatch{}, false
	}
	a := strings.TrimSpace(answer)
	pick := -1
	for i, c := range p.cands {
		names := []string{s.candidateLabel(c), c.wantType}
		if def := s.typeDefinition(c.wantType); def != nil && c.tool == "" {
			names = append(names, cleanAliases(strings.Split(def.Metadata.Labels[robotAliasLabel], ","))...)
		}
		for _, n := range names {
			if n != "" && strings.Contains(a, n) {
				pick = i
			}
		}
	}
	switch {
	case pick >= 0:
	case strings.Contains(a, "1") || strings.Contains(a, "最初") || strings.Contains(a, "前"):
		pick = 0
	case strings.Contains(a, "2") || strings.Contains(a, "後") || strings.Contains(a, "二"):
		pick = 1
	}
	delete(resolveWaiting, who)
	if pick < 0 || pick >= len(p.cands) {
		return p, magicMatch{}, false
	}
	noteClarified(p.said, p.cands[pick].src.key())
	return p, p.cands[pick], true
}

// noteClarified keeps what the person said a request was, for Laya to learn.
func noteClarified(said, key string) {
	line, err := json.Marshal(map[string]string{"at": time.Now().Format(time.RFC3339), "say": said, "key": key})
	if err != nil {
		return
	}
	f, err := os.OpenFile(initiativeHomeFile("laya-clarified.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	f.Write(append(line, '\n'))
}

// resolveWaitingFor: a question put to who waits for its answer.
func resolveWaitingFor(who string) bool {
	resolveMu.Lock()
	defer resolveMu.Unlock()
	p, ok := resolveWaiting[who]
	return ok && time.Since(p.at) <= resolveAskFor
}
