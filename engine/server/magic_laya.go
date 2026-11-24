package server

// Which magic sentence a request means, as Laya reads it.
//
// A magic sentence matched only as written: 「国分寺から中野坂上」 made a
// route, 「国分寺から中野坂上に行きたい」 did not, 「10分後に国分寺駅から
// 中野坂上」 matched with the time inside a station, and 「東京から来た友達
// と話した」 would have been a route. Every request is asked of Laya, two
// questions at once: is it a request at all, or talk (magic_request) — and
// which of the sentences it is (magic_sentence: the types', the robot's own),
// with no "none" among them. "None" beside thirty types won whenever two of
// them were alike, their likelihood split between them (2026-11-24: 18 of 94
// asks read right). So the three likeliest are taken, and which of them it is
// is settled by what was said (magicResolve): how like each one's asks it is,
// and whether the model can read what each one needs. Two still alike, the
// person is asked which. With no Laya to ask, the sentences match as they
// are written, as before.
//
// What the request says fills the sentence's parameters is read by the model
// (magic_params.go), and a time by Laya again (time_reading.go).
//
// The options change as types and wants come and go; a head trained on one
// set of them is not used for another (the worker's head key holds the
// labels), so Laya's own reading is what answers until it is trained again.

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	mywant "mywant/engine/core"
)

// magicLayaTalkP is how sure Laya must be that something said is talk, not
// a request, for it to go to the robot as talk.
const magicLayaTalkP = 0.5

// magicLayaTop is how many of the likeliest sentences are weighed.
const magicLayaTop = 3

// The answers to whether something said is a request (magic_request).
const (
	magicLayaAsk  = "ask"
	magicLayaTalk = "talk"
)

// magicLayaReuse is how long Laya's answer to one request is used again: a
// message is matched to see whether it is a sentence, then again to make it.
const magicLayaReuse = 30 * time.Second

type magicSource struct {
	name, typ, tmpls string
	aliases          []string
	// tool: a robot tool's sentence (robotTool.Sentence), not a type's —
	// asked of the robot, not made.
	tool string
	// A type with asks and no sentence (askSources): what the model reads
	// for it (the parameters its asks name), and how it is told to Laya.
	slots []magicSlotDef
	words string
	// option: how it is named to Laya — the type's laya-option label (「電車の
	// 乗り換え経路と時刻を調べる」), a tool's Option.
	option string
}

// key is the source's option: a want's own sentence by its name, a type's by
// the type.
func (m magicSource) key() string {
	if m.tool != "" {
		return "tool:" + m.tool
	}
	if m.name != "" {
		return "want:" + m.name
	}
	return "type:" + m.typ
}

var (
	magicSlotWords = strings.NewReplacer("<alias>", "")
	magicLayaMu    sync.Mutex
	magicLayaSeen  = map[string]magicLayaAnswer{}
)

type magicLayaAnswer struct {
	at    time.Time
	match magicMatch
	ok    bool
}

// magicSentenceWords is a source's sentences as Laya reads them: each slot as
// what fills it — 「（station）から（station）」 — and its aliases first.
func magicSentenceWords(src magicSource) string {
	if src.words != "" {
		return src.words
	}
	var parts []string
	for _, tmpl := range strings.FieldsFunc(src.tmpls, func(r rune) bool { return r == '\n' || r == '|' || r == '｜' }) {
		if strings.Contains(tmpl, magicAliasSlot) {
			continue // the type's name said as a command: its aliases say it
		}
		t := magicSlot.ReplaceAllStringFunc(tmpl, func(m string) string {
			sm := magicSlot.FindStringSubmatch(m)
			if len(sm) > 2 && sm[2] != "" {
				return "（" + sm[2] + "）"
			}
			return "（" + sm[1] + "）"
		})
		parts = append(parts, strings.TrimSpace(magicSlotWords.Replace(t)))
	}
	words := strings.Join(parts, " / ")
	var names []string
	for _, a := range src.aliases {
		if a = strings.TrimSpace(a); a != "" {
			names = append(names, a)
		}
	}
	if len(names) > 0 {
		words = strings.Join(names, "・") + ": " + words
	}
	return words
}

// magicLayaMatch asks Laya whether the request is one, and which of sources
// it likeliest means. The match it gives is the likeliest, with the three
// likeliest as its candidates (each with its slots, no values — the model's
// to read, magicResolve). ok false: talk, not a request. asked is false when
// Laya could not be asked or did not answer — the sentence as written decides
// then.
func (s *Server) magicLayaMatch(q string, sources []magicSource) (mm magicMatch, ok, asked bool) {
	if len(sources) == 0 {
		return magicMatch{}, false, false
	}
	magicLayaMu.Lock()
	if a, seen := magicLayaSeen[q]; seen && time.Since(a.at) < magicLayaReuse {
		magicLayaMu.Unlock()
		return a.match, a.ok, true
	}
	magicLayaMu.Unlock()
	if !layaCan(modelOpDecide) {
		return magicMatch{}, false, false
	}

	crit, byKey, labelOf := magicLayaOptions(sources)
	started := time.Now()
	// Its numbers taken out (a time is read apart, time_reading.go): numbers
	// alone drew every request with a time to the timer (2026-11-24).
	said := magicLayaSaid(q)
	fits := s.thingFits(said, byKey)
	answers, by, err := (&robotDecider{}).decideCued(said, map[string]decisionQuestion{
		magicLayaQuestionID:    magicLayaQuestion(crit),
		magicRequestQuestionID: magicRequestQuestion(),
	}, map[string]map[string]float64{magicLayaQuestionID: keywordCues(said, byKey)})
	a, req := answers[magicLayaQuestionID], answers[magicRequestQuestionID]
	fields := map[string]any{"talk": "", "top": "", "by": by, "ms": time.Since(started).Milliseconds()}
	// What the things named fill, the best fits first (thingFits).
	var fitSaid []string
	for k, v := range fits {
		if v >= 0.5 {
			fitSaid = append(fitSaid, fmt.Sprintf("%s=%.2f", byKey[k].typ, v))
		}
	}
	sort.Strings(fitSaid)
	fields["fits"] = strings.Join(fitSaid, ",")
	if err != nil {
		fields["error"] = err.Error()
		s.robotChatTrace("laya.magic", fields, []string{"talk", "top", "fits", "by", "ms", "error"})
		return magicMatch{}, false, false
	}
	talk := req.Probabilities[magicLayaTalk]
	fields["talk"] = fmt.Sprintf("%.2f", talk)
	// The three likeliest, each as a match with its slots.
	keys := make([]string, 0, len(a.Probabilities))
	for k := range a.Probabilities {
		if _, known := byKey[k]; known {
			keys = append(keys, k)
		}
	}
	sort.SliceStable(keys, func(i, j int) bool { return a.Probabilities[keys[i]] > a.Probabilities[keys[j]] })
	var top []string
	for _, k := range keys {
		if len(mm.candidates) == magicLayaTop {
			break
		}
		if c, has := magicCandidate(byKey[k], a.Probabilities[k]); has {
			mm.candidates = append(mm.candidates, c)
			top = append(top, fmt.Sprintf("%s=%.2f", k, a.Probabilities[k]))
		}
	}
	// What the dictionary says the request names fills, not learnt: a type
	// every place of which is filled by the things named (「中野坂上から大森」:
	// two stations, a route's from and to) is a candidate whatever Laya made
	// of the words — weighed with the rest by its fit (magicResolve).
	for k, fit := range fits {
		if fit < 1 {
			continue
		}
		already := false
		for _, c := range mm.candidates {
			already = already || c.src.key() == byKey[k].key()
		}
		if !already {
			if c, has := magicCandidate(byKey[k], a.Probabilities[k]); has {
				mm.candidates = append(mm.candidates, c)
				top = append(top, fmt.Sprintf("%s=fit", k))
			}
		}
	}
	for i := range mm.candidates {
		mm.candidates[i].fit = fits[labelOf[mm.candidates[i].src.key()]]
	}
	fields["top"] = strings.Join(top, ",")
	// Every place of a type filled by what the dictionary says was named is
	// a request, whatever Laya made of the words as talk.
	filledAll := false
	for _, fit := range fits {
		filledAll = filledAll || fit >= 1
	}
	if (talk < magicLayaTalkP || filledAll) && len(mm.candidates) > 0 {
		cands := mm.candidates
		mm = cands[0]
		mm.candidates = cands
		ok = true
	}
	fields["request"] = ok
	s.robotChatTrace("laya.magic", fields, []string{"talk", "request", "top", "fits", "by", "ms", "error"})
	magicLayaMu.Lock()
	magicLayaSeen[q] = magicLayaAnswer{at: time.Now(), match: mm, ok: ok}
	for k, v := range magicLayaSeen {
		if time.Since(v.at) > magicLayaReuse {
			delete(magicLayaSeen, k)
		}
	}
	magicLayaMu.Unlock()
	return mm, ok, true
}

// magicCandidate is a source as a match Laya gave p for: its slots (a type
// known by its asks: the parameters they name; else its first sentence's
// that has any), no values yet.
func magicCandidate(src magicSource, p float64) (magicMatch, bool) {
	base := magicMatch{wantName: src.name, wantType: src.typ, tool: src.tool, params: map[string]string{}, byLaya: true, p: p, src: src}
	if src.tmpls == "" {
		base.slots = src.slots
		return base, true
	}
	for _, tmpl := range strings.FieldsFunc(src.tmpls, func(r rune) bool { return r == '\n' || r == '|' || r == '｜' }) {
		if _, slots := compileMagic(tmpl, src.aliases...); len(slots) > 0 {
			base.slots = slots
			return base, true
		}
	}
	return base, false
}

const magicLayaQuestionID = "magic_sentence"

func magicLayaQuestion(crit map[string]string) decisionQuestion {
	return decisionQuestion{Type: "choice", Instructions: "この依頼はどれ？", Criteria: crit}
}

// magicRequestQuestionID asks whether something said is a request at all —
// to make, show or look at something — or talk.
const magicRequestQuestionID = "magic_request"

func magicRequestQuestion() decisionQuestion {
	return decisionQuestion{Type: "choice", Instructions: "これは何かを頼む文？それとも会話？",
		Criteria: map[string]string{magicLayaAsk: "何かを作る・見せる・調べる依頼", magicLayaTalk: "ただの会話や感想（何も頼んでいない）"}}
}

// layaOptionLabel names a type to Laya in a few words of its own (a want
// type's label laya-option): what Laya reads most of an option is its name —
// named 「type:execution_result」, a command took every request with a time.
const layaOptionLabel = "laya-option"

// magicLayaSaid is what was said as Laya is asked which sentence it is: its
// numbers taken out.
func magicLayaSaid(q string) string {
	return resolveNumber.ReplaceAllString(q, "○")
}

// magicLayaOptions is the question's options for these sources, one for one
// sentence — a want made of a type carries the type's sentence as its own,
// and twenty routes on the board were twenty options saying the same thing,
// Laya's likelihood split twenty ways (2026-11-24); the type's own stands for
// them. Each is named by its option words (laya-option; else its first
// alias in Japanese; else its key), told by its aliases. byKey is by that
// name; labelOf names each source's key.
func magicLayaOptions(sources []magicSource) (crit map[string]string, byKey map[string]magicSource, labelOf map[string]string) {
	crit, byKey, labelOf = map[string]string{}, map[string]magicSource{}, map[string]string{}
	seen := map[string]string{} // type + sentences → the source key kept
	kept := map[string]magicSource{}
	var order []string
	for _, src := range sources {
		// One option for one type, whatever its wants' own sentences and
		// aliases (a want's own aliases include its name): what is made is a
		// want of the type. Two for one type put 「乗り換えですか？それとも
		// 乗り換えですか？」 to the person (2026-11-25).
		same := src.tool + "\x00" + src.typ
		if k, ok := seen[same]; ok {
			if src.name != "" || k == "type:"+src.typ {
				continue // the type's, or the first want's, already says it
			}
			delete(kept, k)
		} else {
			order = append(order, same)
		}
		seen[same] = src.key()
		kept[src.key()] = src
	}
	used := map[string]bool{}
	for _, same := range order {
		src, ok := kept[seen[same]]
		if !ok {
			continue
		}
		name := strings.TrimSpace(src.option)
		if name == "" {
			for _, a := range cleanAliases(src.aliases) {
				if strings.IndexFunc(a, func(r rune) bool { return r > 0x7f }) >= 0 {
					name = a
					break
				}
			}
		}
		if name == "" {
			name = src.key()
		}
		if used[name] {
			name += "（" + src.typ + src.tool + "）"
		}
		used[name] = true
		words := strings.Join(cleanAliases(src.aliases), "・")
		if words == "" {
			words = "-"
		}
		crit[name] = words
		byKey[name] = src
		labelOf[src.key()] = name
	}
	return crit, byKey, labelOf
}

// askSources are the types that say how they are asked for (asks) and have no
// sentence of their own: a choice for Laya like a sentence's, told by their
// aliases and a few of their asks; what fills one is the parameters its asks
// name, read by the model — all but a time Laya reads (time-reading).
func (s *Server) askSources(types []robotWantType, aliasesOf map[string][]string) []magicSource {
	var out []magicSource
	for _, t := range types {
		if t.Labels[magicSentenceLabel] != "" {
			continue
		}
		def := s.typeDefinition(t.Name)
		if def == nil || len(def.Asks) == 0 {
			continue
		}
		timeParam, sideParam, _ := timeReadingOf(def.Metadata.Labels)
		named := map[string]bool{}
		for _, a := range def.Asks {
			for k := range a.Params {
				named[k] = true
			}
		}
		var slots []magicSlotDef
		for _, p := range def.Parameters {
			if named[p.Name] && p.Name != timeParam && p.Name != sideParam {
				slots = append(slots, magicSlotDef{param: p.Name, subtype: p.SubType})
			}
		}
		aliases := cleanAliases(aliasesOf[t.Name])
		words := strings.Join(askSamples(def.Asks, aliases, magicWordsAsks), " / ")
		if len(aliases) > 0 {
			words = strings.Join(aliases, "・") + ": " + words
		}
		out = append(out, magicSource{typ: t.Name, aliases: aliases, slots: slots, words: words, option: t.Labels[layaOptionLabel]})
	}
	return out
}

// cleanAliases are a type's aliases, each once and none empty.
func cleanAliases(aliases []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, a := range aliases {
		if a = strings.TrimSpace(a); a != "" && !seen[a] {
			seen[a] = true
			out = append(out, a)
		}
	}
	return out
}

// expandAlias is an ask's words with <alias> said as each of the type's
// aliases: 「<alias>を出して」 is 「天気を出して」 and 「weatherを出して」.
// Words with no <alias> are themselves; with no aliases, none.
func expandAlias(say string, aliases []string) []string {
	slot := "<" + magicAliasSlot + ">"
	if !strings.Contains(say, slot) {
		return []string{say}
	}
	var out []string
	for _, a := range aliases {
		out = append(out, strings.ReplaceAll(say, slot, a))
	}
	return out
}

// magicWordsAsks is how many of a type's asks it is told to Laya by.
const magicWordsAsks = 3

// askSamples are n of a type's asks to tell it by, as unlike each other as
// can be (a route without a time, and with one), their numbers taken out:
// numbers alone made a type — the timer, 「5分ごと」「1時間ごと」 — what any
// request with a time looked like (2026-11-24: 「30分後に荻窪を出て銀座へ」
// read as a timer, a route told only as 「（station）から（station）」).
func askSamples(asks []mywant.AskDef, aliases []string, n int) []string {
	var all []string
	for _, a := range expandAsks(asks, aliases) {
		all = append(all, resolveNumber.ReplaceAllString(a.Say, "○"))
	}
	var picked []string
	for len(picked) < n && len(picked) < len(all) {
		best, bestSim := -1, 2.0
		for i, a := range all {
			taken := false
			for _, p := range picked {
				taken = taken || p == a
			}
			if taken {
				continue
			}
			sim := 0.0
			for _, p := range picked {
				sim = max(sim, shapeSimilarity(a, p))
			}
			if sim < bestSim {
				best, bestSim = i, sim
			}
		}
		if best < 0 {
			break
		}
		picked = append(picked, all[best])
	}
	return picked
}

// keywordCues are the options one of whose keywords — its aliases, a tool's
// words — is in what was said: 「札幌の天気」 has 天気's. Given to Laya
// beside the words (decisionItem.Cues); how much a keyword counts is learnt
// (LayaHead.keyword), not set here.
func keywordCues(said string, byKey map[string]magicSource) map[string]float64 {
	lower := strings.ToLower(said)
	out := map[string]float64{}
	for label, src := range byKey {
		for _, a := range src.aliases {
			for _, w := range strings.Split(a, "・") {
				w = strings.ToLower(strings.TrimSpace(w))
				if len([]rune(w)) >= 2 && strings.Contains(lower, w) {
					out[label] = 1
				}
			}
		}
	}
	return out
}

// Kinds of thing that are not where or what a request names: a time is read
// apart (time_reading.go), a secret is never said.
var thingFitSkip = map[string]bool{"time": true, "date": true, "datetime": true, "date_time": true, "secret": true}

// namedThing is a thing of the dictionary named in what was said, with every
// kind it is known as (中野坂上: a station, and a city).
type namedThing struct {
	value string
	kinds map[string]bool
}

// thingsNamed are the things of the dictionary (every value named and kept,
// thingFulls) said in what was said — the longest first, each place in the
// words taken once (中野坂上駅 before 中野坂上).
func (s *Server) thingsNamed(said string) []namedThing {
	kinds := map[string]map[string]bool{}
	for _, t := range s.thingFulls() {
		v := strings.TrimSpace(t.Value)
		if len([]rune(v)) < 2 || thingFitSkip[t.Subtype] {
			continue
		}
		if kinds[v] == nil {
			kinds[v] = map[string]bool{}
		}
		kinds[v][t.Subtype] = true
	}
	values := make([]string, 0, len(kinds))
	for v := range kinds {
		values = append(values, v)
	}
	sort.SliceStable(values, func(i, j int) bool { return len([]rune(values[i])) > len([]rune(values[j])) })
	rest := strings.ToLower(said)
	var out []namedThing
	for _, v := range values {
		lv := strings.ToLower(v)
		if strings.Contains(rest, lv) {
			out = append(out, namedThing{value: v, kinds: kinds[v]})
			rest = strings.Replace(rest, lv, "○", 1)
		}
	}
	return out
}

// thingFits is, for each option, how well the things named in what was said
// fill the places it takes (its parameters' subTypes, of the kinds the
// dictionary knows): each thing given to one place of its kind, and of the
// places and the things, the more counts — 「中野坂上から大森」 (two
// stations) fills a route's from and to (1), a weather's at with one of its
// two (中野坂上 is a city too: 0.5) — not to Laya, nothing learnt: what the dictionary says is so. A type every
// place of which is filled is a candidate (magicLayaMatch), and its fit adds
// to its score (magicResolve).
func (s *Server) thingFits(said string, byKey map[string]magicSource) map[string]float64 {
	named := s.thingsNamed(said)
	if len(named) == 0 {
		return nil
	}
	out := map[string]float64{}
	for label, src := range byKey {
		if src.tool != "" {
			continue
		}
		def := s.typeDefinition(src.typ)
		if def == nil {
			continue
		}
		// The places it cannot be made without: its required ones — with or
		// without where the person is (its here-param: a transit search's
		// from, when not said, is where they are), whichever the things named
		// fill better; a type that requires none, every place it takes. A
		// route's via, never required, left a route below a transit search
		// for two stations. 「中野坂上から大森」 fills a transit search's from
		// and to; 「大森まで」 its to, all it needs.
		here := def.Metadata.Labels[robotHereLabel]
		var withHere, withoutHere, all []string
		for _, p := range def.Parameters {
			if sub := strings.TrimSpace(p.SubType); sub != "" && !thingFitSkip[sub] {
				all = append(all, sub)
				if p.Required {
					withHere = append(withHere, sub)
					if p.Name != here {
						withoutHere = append(withoutHere, sub)
					}
				}
			}
		}
		if len(withHere) == 0 {
			withHere, withoutHere = all, all
		}
		if fit := max(placeFit(withHere, named), placeFit(withoutHere, named)); fit > 0 {
			out[label] = fit
		}
	}
	return out
}

// placeFit is how well the things named fill these places: each thing given
// to one place of its kind (the thing known as fewest kinds first — a station
// only before one that is a city too), and of the places and the things, the
// more counts: what else was named and fills none of it counts against it (a
// weather of one of two places named).
func placeFit(places []string, named []namedThing) float64 {
	if len(places) == 0 {
		return 0
	}
	used := make([]bool, len(named))
	filled := 0
	for _, kind := range places {
		best := -1
		for i, n := range named {
			if !used[i] && n.kinds[kind] && (best < 0 || len(n.kinds) < len(named[best].kinds)) {
				best = i
			}
		}
		if best >= 0 {
			used[best] = true
			filled++
		}
	}
	return float64(filled) / float64(max(len(places), len(named)))
}
