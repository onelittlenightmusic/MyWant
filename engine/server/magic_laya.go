package server

// Which magic sentence a request means, as Laya reads it.
//
// A magic sentence matched only as written: 「国分寺から中野坂上」 made a
// route, 「国分寺から中野坂上に行きたい」 did not, 「10分後に国分寺駅から
// 中野坂上」 matched with the time inside a station, and 「東京から来た友達
// と話した」 would have been a route. Every request is asked of Laya — one
// question, a choice among the sentences on the board (each type's, each
// want's own) and "none": not a request to make anything. It is sure enough,
// or the request goes to the robot as it would have. With no Laya to ask,
// the sentences match as they are written, as before.
//
// What the request says fills the sentence's parameters is then read by the
// model (magic_params.go), and a time by Laya again (time_reading.go). A
// sentence Laya chose that the model can read nothing of is no match after
// all (magicSentenceDoMatch).
//
// The options change as types and wants come and go; a head trained on one
// set of them is not used for another (the worker's head key holds the
// labels), so Laya's own reading is what answers until it is trained again.

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// magicLayaMinP is how sure Laya must be that a request is a sentence's.
const magicLayaMinP = 0.6

// magicLayaNone is the option that the request is none of them.
const magicLayaNone = "none"

// magicLayaReuse is how long Laya's answer to one request is used again: a
// message is matched to see whether it is a sentence, then again to make it.
const magicLayaReuse = 30 * time.Second

type magicSource struct {
	name, typ, tmpls string
	aliases          []string
	// tool: a robot tool's sentence (robotTool.Sentence), not a type's —
	// asked of the robot, not made.
	tool string
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

// magicLayaMatch asks Laya which of sources the request means. The match it
// gives has the sentence's slots and no values: they are the model's to read.
// asked is false when Laya could not be asked or did not answer — the
// sentence as written decides then.
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

	crit, byKey := magicLayaOptions(sources)
	started := time.Now()
	answers, by, err := (&robotDecider{}).decide(q, map[string]decisionQuestion{magicLayaQuestionID: magicLayaQuestion(crit)})
	a := answers[magicLayaQuestionID]
	fields := map[string]any{"choice": a.Choice, "p": "", "by": by, "ms": time.Since(started).Milliseconds()}
	if p, ok := a.Probabilities[a.Choice]; ok {
		fields["p"] = fmt.Sprintf("%.2f", p)
	}
	switch {
	case err != nil:
		fields["error"] = err.Error()
		s.robotChatTrace("laya.magic", fields, []string{"choice", "p", "matched", "by", "ms", "error"})
		return magicMatch{}, false, false
	case a.Choice == magicLayaNone || a.Probabilities[a.Choice] < magicLayaMinP:
	default:
		src, known := byKey[a.Choice]
		if !known {
			break
		}
		// The slots of its first sentence that has any: what the model reads.
		for _, tmpl := range strings.FieldsFunc(src.tmpls, func(r rune) bool { return r == '\n' || r == '|' || r == '｜' }) {
			if _, slots := compileMagic(tmpl, src.aliases...); len(slots) > 0 {
				mm = magicMatch{wantName: src.name, wantType: src.typ, tool: src.tool, params: map[string]string{}, slots: slots, byLaya: true}
				ok = true
				break
			}
		}
	}
	fields["matched"] = ok
	s.robotChatTrace("laya.magic", fields, []string{"choice", "p", "matched", "by", "ms", "error"})
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

const magicLayaQuestionID = "magic_sentence"

func magicLayaQuestion(crit map[string]string) decisionQuestion {
	return decisionQuestion{Type: "choice", Instructions: "この依頼はどれ？", Criteria: crit}
}

// magicLayaOptions is the question's options for these sources: "none", and
// one for one sentence — a want made of a type carries the type's sentence
// as its own, and twenty routes on the board were twenty options saying the
// same thing, Laya's likelihood split twenty ways and none sure enough
// (2026-11-24). The type's own stands for them.
func magicLayaOptions(sources []magicSource) (map[string]string, map[string]magicSource) {
	crit := map[string]string{magicLayaNone: "どれでもない（何かを作る依頼ではない）"}
	byKey := map[string]magicSource{}
	seen := map[string]string{} // type + words → the option kept
	for _, src := range sources {
		w := magicSentenceWords(src)
		if w == "" {
			continue
		}
		// The same sentences of the same type are one option, whatever the
		// aliases: a want's own aliases include its name, and the routes on
		// the board were eight options told apart only by it.
		same := src.tool + "\x00" + src.typ + "\x00" + strings.Join(strings.Fields(src.tmpls), " ")
		if kept, ok := seen[same]; ok {
			if src.name != "" || kept == "type:"+src.typ {
				continue // the type's, or the first want's, already says it
			}
			delete(crit, kept)
			delete(byKey, kept)
		}
		seen[same] = src.key()
		crit[src.key()] = w
		byKey[src.key()] = src
	}
	return crit, byKey
}
