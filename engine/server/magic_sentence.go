package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	mywant "mywant/engine/core"
)

// A want's magic sentence (its label magic-sentence), or its type's (the
// type's label of the same name — the default for every want of it, and said
// even with none of them on the board): what said to the robot makes a want
// of that type, with the parameters the sentence names —
// 「<from:station>から<to:station>」 on a route, and 「荻窪から中野坂上」 typed in
// the chat makes the route from 荻窪 to 中野坂上, with no model asked. Several
// sentences may be given, one a line (or split by |). <name:subtype>: the
// parameter, and what kind of thing fills it — a value that is such a thing
// is preferred where more than one sentence matches.
const magicSentenceLabel = "magic-sentence"

var magicSlot = regexp.MustCompile(`<([A-Za-z0-9_\-]+)(?::([A-Za-z0-9_\-]+))?>`)

// magicAliasSlot: <alias> in a sentence is the type's name said as a command —
// any of its aliases (the type's label aliases): 「<alias> <from:station>
// <to:station>」, and 「transit 荻窪 中野坂上」 makes the route.
const magicAliasSlot = "alias"

type magicSlotDef struct{ param, subtype string }

// compileMagic turns a sentence into a whole-question pattern and its slots.
func compileMagic(tmpl string, aliases ...string) (*regexp.Regexp, []magicSlotDef) {
	var b strings.Builder
	var slots []magicSlotDef
	b.WriteString(`^\s*`)
	last := 0
	for _, m := range magicSlot.FindAllStringSubmatchIndex(tmpl, -1) {
		raw := tmpl[last:m[0]]
		lit := strings.TrimSpace(raw)
		// Nothing but a space between two parts (「<alias> <from> <to>」):
		// the space is what tells them apart, so it must be there — else the
		// first slot took one character and the second the rest.
		if lit == "" && raw != "" && last > 0 {
			b.WriteString(`\s+`)
		}
		for i, part := range strings.Fields(lit) {
			if i > 0 {
				b.WriteString(`\s*`)
			}
			b.WriteString(regexp.QuoteMeta(part))
		}
		last = m[1]
		if tmpl[m[2]:m[3]] == magicAliasSlot {
			// The type's name said as a command: one of its aliases, no parameter.
			var alts []string
			for _, a := range aliases {
				if a = strings.TrimSpace(a); a != "" {
					alts = append(alts, regexp.QuoteMeta(a))
				}
			}
			if len(alts) == 0 {
				return nil, nil
			}
			b.WriteString(`\s*(?i:` + strings.Join(alts, "|") + `)\s*`)
			continue
		}
		b.WriteString(`\s*(.+?)\s*`)
		sub := ""
		if m[4] >= 0 {
			sub = tmpl[m[4]:m[5]]
		}
		slots = append(slots, magicSlotDef{param: tmpl[m[2]:m[3]], subtype: sub})
	}
	for i, part := range strings.Fields(strings.TrimSpace(tmpl[last:])) {
		if i > 0 {
			b.WriteString(`\s*`)
		}
		b.WriteString(regexp.QuoteMeta(part))
	}
	b.WriteString(`\s*$`)
	if len(slots) == 0 {
		return nil, nil
	}
	re, err := regexp.Compile(b.String())
	if err != nil {
		return nil, nil
	}
	return re, slots
}

type magicMatch struct {
	wantName, wantType string
	params             map[string]string
	slots              []magicSlotDef
	tool               string // a robot tool's sentence: the tool is run, nothing made
	byLaya             bool   // which sentence Laya said; the values are the model's
	known              int    // slot values that are things of their subtype
}

// magicSentenceMatch: the want whose magic sentence the question is, and the
// parameters it names — the best of several (most values known as things).
func (s *Server) magicSentenceMatch(question string) (magicMatch, bool) {
	if s.globalBuilder == nil {
		return magicMatch{}, false
	}
	q := strings.TrimRight(strings.TrimSpace(question), "。．.!！?？ ")
	if q == "" {
		return magicMatch{}, false
	}
	var thingsBySub map[string]map[string]bool
	knownThing := func(sub, v string) bool {
		if sub == "" {
			return false
		}
		if thingsBySub == nil {
			thingsBySub = map[string]map[string]bool{}
			for _, t := range s.thingFulls() {
				if thingsBySub[t.Subtype] == nil {
					thingsBySub[t.Subtype] = map[string]bool{}
				}
				thingsBySub[t.Subtype][t.Value] = true
			}
		}
		return thingsBySub[sub][v]
	}
	sources, readsTime := s.magicSources()
	untimed := magicCutTime(q)
	var best magicMatch
	found := false
	for _, src := range sources {
		tmpls := src.tmpls
		for _, tmpl := range strings.FieldsFunc(tmpls, func(r rune) bool { return r == '\n' || r == '|' || r == '｜' }) {
			re, slots := compileMagic(tmpl, src.aliases...)
			if re == nil {
				continue
			}
			said := q
			if readsTime[src.typ] {
				said = untimed
			}
			m := re.FindStringSubmatch(said)
			if m == nil {
				continue
			}
			mm := magicMatch{wantName: src.name, wantType: src.typ, tool: src.tool, params: map[string]string{}, slots: slots}
			for i, sl := range slots {
				v := strings.TrimSpace(m[i+1])
				// 「国分寺駅」 is the station 国分寺 when that is the thing known.
				if t := strings.TrimSuffix(v, "駅"); t != v && t != "" && !knownThing(sl.subtype, v) && knownThing(sl.subtype, t) {
					v = t
				}
				mm.params[sl.param] = v
				if knownThing(sl.subtype, v) {
					mm.known++
				}
			}
			if !found || mm.known > best.known {
				best, found = mm, true
			}
		}
	}
	// Which sentence it is, if any, is Laya's to say (magic_laya.go), written
	// as it is or not: 「国分寺から中野坂上に行きたい」 is a route though no
	// sentence is said word for word, and 「東京から来た友達と話した」 is
	// none though it is 「<from>から<to>」 word for word. The sentence as
	// written decides only when there is no Laya to ask.
	if mm, ok, asked := s.magicLayaMatch(q, sources); asked {
		if ok && found && mm.wantType == best.wantType && mm.wantName == best.wantName {
			// Laya's sentence, as written: its reading is where the model has none.
			best.byLaya = true
			return best, true
		}
		return mm, ok
	}
	return best, found
}

// magicSentenceReply: the question was a want's magic sentence — a want of its
// type made with what it named (or the one already there shown), and what was
// done said back, for the brief to answer with (Answered).
func (s *Server) magicSentenceReply(question, turn, locale string) (string, bool) {
	answer, _, ok := s.magicSentenceDo(question, turn, locale)
	return answer, ok
}

// magicSentenceDo is magicSentenceReply with the want it made (or found) as
// its card, for a chat to show under the answer.
func (s *Server) magicSentenceDo(question, turn, locale string) (string, *robotCard, bool) {
	answer, card, _, ok := s.magicSentenceDoMatch(question, turn, locale)
	return answer, card, ok
}

// magicSentenceDoMatch is magicSentenceDo with the match it made the want by
// (its parameters as they were read).
func (s *Server) magicSentenceDoMatch(question, turn, locale string) (string, *robotCard, magicMatch, bool) {
	mm, ok := s.magicSentenceMatch(question)
	if !ok {
		return "", nil, mm, false
	}
	// The sentence said which type; what fills it is read by the model from
	// what was said (magicModelParams), the sentence's own reading kept where
	// the model has none.
	s.magicModelParams(question, &mm)
	// A robot tool's sentence: the tool, not a want (magicToolDo).
	if mm.tool != "" {
		return s.magicToolDo(mm)
	}
	// Laya said which sentence, and the model could not read what fills it:
	// not that sentence after all — the robot is asked as it would be.
	// Every one of its slots, not some: 「国分寺はどこ？」 taken for a route
	// gave a from and no to, and the robot said it could not make one
	// (2026-11-24) instead of answering where 国分寺 is.
	if mm.byLaya {
		for _, sl := range mm.slots {
			if strings.TrimSpace(mm.params[sl.param]) == "" {
				return "", nil, mm, false
			}
		}
	}
	_, _, steps := s.doAct(actRequest{Turn: turn, Question: question, Locale: locale, Kind: "new", Type: mm.wantType, Params: mm.params}, nil)
	if len(steps) == 0 {
		return "", nil, mm, false
	}
	made := steps[0]
	name := ""
	if made.Card != nil {
		name = made.Card.Name
	}
	switch {
	case strings.HasPrefix(made.Output, "Error:"):
		return fmt.Sprintf("「%s」で作ろうとしましたが、できませんでした（%s）。", question, strings.TrimSpace(strings.TrimPrefix(made.Output, "Error:"))), nil, mm, true
	case strings.Contains(made.Output, "already on the board"):
		return fmt.Sprintf("もうあります。ここです（%s）。", name), made.Card, mm, true
	case strings.Contains(made.Output, "archived"):
		return fmt.Sprintf("同じものがアーカイブにあります（%s）。", name), made.Card, mm, true
	}
	if name == "" {
		name = mm.wantType
	}
	return fmt.Sprintf("「%s」を作りました。", name), made.Card, mm, true
}

// magicSentenceToRobot: a message to the robot want (its chat — the GUI's, a
// webhook) that is a magic sentence is done here, as the app's question is in
// its brief: the question and the answer in the robot's chat, with the want's
// card, and nothing handed to its model — which, given the sentence, made the
// same route by its own tools and then said something of its own about it.
// False: not one; the message goes to the robot as it is.
//
// Doing it takes the models a while (Laya, then Apple's), so it is done after
// the message is taken; one that turns out not to be a sentence done here —
// what a want says (look), a place not on the board, a value the model could
// not read — goes to the robot then, by notDone, as it would have at once.
func (s *Server) magicSentenceToRobot(text string, notDone func()) bool {
	q := strings.TrimSpace(text)
	if q == "" {
		return false
	}
	if _, ok := s.magicSentenceMatch(q); !ok {
		return false
	}
	go func() {
		answer, card, ok := s.magicSentenceDo(q, "", "")
		if !ok {
			notDone()
			return
		}
		s.robotAsked(q, "")
		var cards []robotCard
		if card != nil {
			cards = []robotCard{*card}
		}
		s.sayTurnAnswer(answer, cards, nil, "")
	}()
	return true
}

// magicRequest / magicResponse: POST /api/v1/magic — a magic sentence said by
// something that wants the result, not words: a keyboard that offers the
// route's answers to type once it has made it, the CLI's `mywant magic`.
type magicRequest struct {
	Text string `json:"text"`
	// How long to wait for the want's answer, in seconds (default 15, at most
	// 60; below 0, not at all — only which want is asked).
	Wait int `json:"wait,omitempty"`
}

type magicResponse struct {
	// Matched: the text is a magic sentence of some want or type.
	Matched bool `json:"matched"`
	// What the robot says of it: made, already there, or why not.
	Answer   string `json:"answer,omitempty"`
	WantID   string `json:"wantId,omitempty"`
	WantName string `json:"wantName,omitempty"`
	WantType string `json:"wantType,omitempty"`
	// The parameters the sentence named.
	Params map[string]string `json:"params,omitempty"`
	// Settled: its answer came within the wait (its fields are its result).
	Settled bool `json:"settled"`
}

// POST /api/v1/magic
func (s *Server) handleMagic(w http.ResponseWriter, r *http.Request) {
	var req magicRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}
	text := strings.TrimSpace(req.Text)
	mm, ok := s.magicSentenceMatch(text)
	if !ok {
		// A want's own alias said alone ("golf"): that want, as it is — its
		// answer shown, nothing made.
		if want := s.wantByAlias(text); want != nil {
			out := magicResponse{Matched: true, Answer: fmt.Sprintf("「%s」です。", want.Metadata.Name),
				WantID: want.Metadata.ID, WantName: want.Metadata.Name, WantType: want.Metadata.Type}
			if req.Wait >= 0 && s.magicSettles(want.Metadata.Type) {
				wait := req.Wait
				if wait == 0 {
					wait = 15
				}
				if wait > 60 {
					wait = 60
				}
				out.Settled = s.robotWaitSettled(want.Metadata.ID, time.Duration(wait)*time.Second)
			}
			robotWriteJSON(w, out)
			return
		}
		robotWriteJSON(w, magicResponse{Matched: false})
		return
	}
	answer, card, read, done := s.magicSentenceDoMatch(text, "", "")
	if !done {
		// Read as a sentence and not one done here (look, a place not on
		// the board): not a match, for the keyboard as for the robot.
		robotWriteJSON(w, magicResponse{Matched: false})
		return
	}
	if read.params != nil {
		mm = read // the parameters as the model read them
	}
	out := magicResponse{Matched: true, Answer: answer, WantType: mm.wantType, Params: mm.params}
	if card != nil && (req.Wait < 0 || !s.magicSettles(mm.wantType)) {
		out.WantID, out.WantName = card.ID, card.Name
	} else if card != nil {
		out.WantID, out.WantName = card.ID, card.Name
		wait := req.Wait
		if wait <= 0 {
			wait = 15
		}
		if wait > 60 {
			wait = 60
		}
		out.Settled = s.robotWaitSettled(card.ID, time.Duration(wait)*time.Second)
	}
	robotWriteJSON(w, out)
}

// magicSettles: a want of this type is worth waiting for — it runs and is
// done. One a person works (user-control: a checklist, a note, a timer) is
// never done by itself: waited for, a checklist said on the keyboard held its
// fields back for the whole wait, twenty seconds each time (2026-11-16).
func (s *Server) magicSettles(wantType string) bool {
	return s.robotTypeLabel(wantType, "user-control") != "true"
}

// wantByAlias: the want on the board (not archived) that `said` names by an
// alias — its own (its label aliases: "golf") first, else its type's — nil
// when none does. Of several of one type, the one created last.
func (s *Server) wantByAlias(said string) *mywant.Want {
	var live []*mywant.Want
	for _, w := range s.globalBuilder.GetWants() {
		if w.Metadata.Labels[robotArchived] != "true" && w.Metadata.Name != robotWantName {
			live = append(live, w)
		}
	}
	for _, w := range live {
		if robotOwnAlias(strings.Split(w.Metadata.Labels[robotAliasLabel], ","), said) {
			return w
		}
	}
	types, _ := s.robotWantTypes()
	for _, t := range types {
		if !robotOwnAlias(strings.Split(t.Labels[robotAliasLabel], ","), said) {
			continue
		}
		var last *mywant.Want
		for _, w := range live {
			if w.Metadata.Type == t.Name && (last == nil || w.Metadata.Labels["mywant.io/created-at"] > last.Metadata.Labels["mywant.io/created-at"]) {
				last = w
			}
		}
		if last != nil {
			return last
		}
	}
	return nil
}

// A time said in a request, with the words around it that only say it is a
// time: 明日の朝9時40分までに, 10分後に, 20:15発で, 十分後に.
var magicTimePhrase = regexp.MustCompile(
	`(?:(?:明日|今日|あした|きょう|あす)の?)?(?:(?:朝|昼|夕方|夜|午前|午後)の?)?` +
		`(?:[0-9０-９一二三四五六七八九十]+\s*時間(?:\s*[0-9０-９一二三四五六七八九十]+\s*分|半)?\s*後` +
		`|[0-9０-９一二三四五六七八九十]+\s*分\s*後` +
		`|[0-9０-９]{1,2}\s*[:：]\s*[0-9０-９]{2}` +
		`|[0-9０-９一二三四五六七八九十]+\s*時(?:\s*[0-9０-９一二三四五六七八九十]+\s*分|半)?)` +
		`\s*(?:ごろ|頃)?\s*(?:出発|到着|発|着)?\s*(?:までに|まで|に|で)?`)

// magicCutTime is the request without the time it says, for a sentence of a
// type whose time is read apart (readsTime above).
func magicCutTime(q string) string {
	return strings.Join(strings.Fields(magicTimePhrase.ReplaceAllString(q, " ")), " ")
}

// magicSources is where sentences are said: each want's own, then each
// type's default (on a tie the first wins, so a want's own sentence before
// its type's) — and which types read their time apart.
func (s *Server) magicSources() ([]magicSource, map[string]bool) {
	var sources []magicSource
	aliasesOf := map[string][]string{}
	// A type whose time is read (time-reading/<param>, time_reading.go) has
	// its sentence matched without the time said: 「10分後に国分寺駅から
	// 中野坂上」 is its 「<from>から<to>」, the time read by Laya from the
	// whole question as the want is made — not a station named 「10分後に
	// 国分寺駅」 (2026-11-24, a route searched from there for now).
	readsTime := map[string]bool{}
	types, _ := s.robotWantTypes()
	for _, t := range types {
		aliasesOf[t.Name] = strings.Split(t.Labels[robotAliasLabel], ",")
		for k := range t.Labels {
			if strings.HasPrefix(k, timeReadingLabel) {
				readsTime[t.Name] = true
			}
		}
	}
	if s.globalBuilder != nil {
		for _, w := range s.globalBuilder.GetWants() {
			if t := w.Metadata.Labels[magicSentenceLabel]; t != "" && w.Metadata.Labels[robotArchived] != "true" {
				// A want's <alias> is any of its own aliases, or its type's.
				own := robotWantAliases(w.Metadata.Labels, w.Metadata.Type, nil)
				sources = append(sources, magicSource{name: w.Metadata.Name, typ: w.Metadata.Type, tmpls: t, aliases: append(own, aliasesOf[w.Metadata.Type]...)})
			}
		}
	}
	for _, t := range types {
		if tmpl := t.Labels[magicSentenceLabel]; tmpl != "" {
			sources = append(sources, magicSource{typ: t.Name, tmpls: tmpl, aliases: aliasesOf[t.Name]})
		}
	}
	// What is asked of the robot itself: where a thing is (point), how a
	// want is (look) — choices beside the types', so 「国分寺はどこ？」 is
	// read as where 国分寺 is, not taken for a route.
	for _, t := range robotTools {
		if t.Sentence != "" {
			sources = append(sources, magicSource{tool: t.Name, tmpls: t.Sentence, aliases: []string{t.Words}})
		}
	}
	return sources, readsTime
}

// magicToolDo is a robot tool's sentence done: where a thing is (point) —
// the robot walks there, and says so. Anything else the tool's sentence was
// (look: what a want says, which takes words of the robot's own to answer
// with), or a point that found nothing to walk to, is the robot's to
// answer, as it would have been: not a match.
func (s *Server) magicToolDo(mm magicMatch) (string, *robotCard, magicMatch, bool) {
	for _, sl := range mm.slots {
		if strings.TrimSpace(mm.params[sl.param]) == "" {
			return "", nil, mm, false
		}
	}
	if mm.tool != "point" {
		return "", nil, mm, false
	}
	out, err := robotToolByName("point").run(s, mm.params)
	if err != nil || !strings.HasPrefix(out, "The robot is standing on") {
		return "", nil, mm, false // not there, or archived: the robot says so
	}
	return fmt.Sprintf("%sに来ました。", mm.params["name"]), nil, mm, true
}
