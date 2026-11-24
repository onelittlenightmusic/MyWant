package server

// What fills a magic sentence's parameters, read by the model.
//
// A magic sentence says which type a request is — 「<from:station>から
// <to:station>」 is a route — and that it is cheap to know. What it is poor at
// is the values: a slot takes everything between its words, and
// 「10分後に国分寺駅から中野坂上」 made a route from 「10分後に国分寺駅」
// (2026-11-24). So the sentence picks the type, and Apple's model (a device's,
// else this Mac's fmtool: generateText) reads each of its parameters from what
// was said — the place itself, without the time or the words around it.
//
// The model writes nothing of its own: a value is kept only as words the
// person said. Where it gives none (no model — Fly with no device — or an
// answer that is not the person's words), the sentence's own reading stays.
// A time is not the model's to read either: it is Laya's, from the whole
// question, as the want is made (time_reading.go).

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// magicModelWait is how long the model is waited for before the sentence's
// own reading is used.
const magicModelWait = 20 * time.Second

// magicModelParams sets mm's parameters to what the model reads for each of
// its slots from question, where it reads one that was said.
func (s *Server) magicModelParams(question string, mm *magicMatch) {
	if len(mm.slots) == 0 || strings.TrimSpace(question) == "" {
		return
	}
	var def struct {
		Parameters []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"parameters"`
	}
	desc := map[string]string{}
	what := "a " + mm.wantType
	if mm.tool != "" {
		// A robot tool's: its arguments say what each is.
		t := robotToolByName(mm.tool)
		what = "the robot's " + t.Name + " (" + firstLine(t.Description, 100) + ")"
		for _, a := range t.Arguments {
			desc[a.Name] = firstLine(a.Description, 100)
		}
	} else {
		_ = s.backend("GET", "/api/v1/want-types/"+mm.wantType, nil, &def)
		for _, p := range def.Parameters {
			desc[p.Name] = firstLine(p.Description, 100)
		}
	}
	prompt := magicParamsPrompt(question, what, mm.slots, desc)

	started := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), magicModelWait)
	defer cancel()
	text, by, err := generateText(ctx, prompt, false, magicModelWait)
	fields := map[string]any{"want": mm.wantType + mm.tool, "by": string(by), "ms": time.Since(started).Milliseconds()}
	if err != nil {
		fields["error"] = err.Error()
		s.robotChatTrace("fm.params", fields, []string{"want", "by", "ms", "error"})
		return
	}
	read := magicParamsRead(text, question, mm.slots, desc)
	order := []string{"want"}
	for _, sl := range mm.slots {
		v, ok := read[sl.param]
		if !ok {
			fields[sl.param] = "(kept " + mm.params[sl.param] + ")"
		} else {
			// 「国分寺駅」 is the station 国分寺 when that is the thing known,
			// as the sentence's own reading has it.
			if t := strings.TrimSuffix(v, "駅"); t != v && t != "" && s.magicThingKnown(sl.subtype, t) && !s.magicThingKnown(sl.subtype, v) {
				v = t
			}
			mm.params[sl.param] = v
			fields[sl.param] = v
		}
		order = append(order, sl.param)
	}
	// What it wrote, as it wrote it: a value thrown away is seen here.
	fields["answer"] = strings.Join(strings.Fields(firstLine(strings.ReplaceAll(text, "\n", " / "), 120)), " ")
	s.robotChatTrace("fm.params", fields, append(order, "by", "ms", "answer"))
}

// magicParamsPrompt asks for each slot, one a line.
func magicParamsPrompt(question, what string, slots []magicSlotDef, desc map[string]string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "The person said: 「%s」\n", question)
	fmt.Fprintf(&b, "This asks for %s. Take each of these from what they said:\n", what)
	for _, sl := range slots {
		kind := sl.subtype
		if kind == "" {
			kind = "text"
		}
		fmt.Fprintf(&b, "- %s (%s)", sl.param, kind)
		if d := desc[sl.param]; d != "" {
			fmt.Fprintf(&b, ": %s", d)
		}
		b.WriteString("\n")
	}
	b.WriteString("The value is the words as they said them — only the place or thing itself: " +
		"no time (10分後, 9時), no particle (から, まで, へ, に), no other words. " +
		"Leave it empty when it was not said. Fill in these lines and write nothing else:\n")
	for _, sl := range slots {
		fmt.Fprintf(&b, "%s=\n", sl.param)
	}
	return b.String()
}

// magicParamsRead reads the model's lines: each slot's value, kept only when
// it is words the person said.
func magicParamsRead(text, question string, slots []magicSlotDef, desc map[string]string) map[string]string {
	// A line may name its parameter, or say it as it was described
	// (「Departure station: 国分寺駅」 — the phone's and the Mac's model both
	// wrote that, 2026-11-24).
	nameOf := map[string]string{}
	for _, sl := range slots {
		nameOf[strings.ToLower(sl.param)] = sl.param
		if d := strings.ToLower(strings.TrimSpace(desc[sl.param])); d != "" {
			nameOf[d] = sl.param
		}
	}
	out := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "-*・ "))
		i := strings.IndexAny(line, "=:：")
		if i < 0 {
			continue
		}
		name := nameOf[strings.ToLower(strings.TrimSpace(line[:i]))]
		_, size := utf8.DecodeRuneInString(line[i:])
		v := strings.Trim(strings.TrimSpace(line[i+size:]), "\"'「」`")
		if name == "" || v == "" || !strings.Contains(question, v) {
			continue
		}
		out[name] = v
	}
	return out
}

// magicThingKnown: a thing of this subtype is named v on the board.
func (s *Server) magicThingKnown(sub, v string) bool {
	if sub == "" {
		return false
	}
	for _, t := range s.thingFulls() {
		if t.Subtype == sub && t.Value == v {
			return true
		}
	}
	return false
}
