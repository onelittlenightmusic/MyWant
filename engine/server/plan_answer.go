package server

// An answer worded before it is needed.
//
// A plan that makes a want used to be answered by asking the model again once
// the want was done (the act's prompt): on an iPhone that was three and a half
// seconds of a ten-second turn. The words of such an answer hardly change from
// one route to the next — only the values do — so the decision writes them
// once, with the result's fields in braces (spec.answerTemplate), and the
// server keeps them for the type and the asker's language. From then on an act
// that makes a want of that type is answered by filling the template from what
// the want found (status.answer), and no model is asked.
//
// A template whose fields the result does not fill (a field it does not have,
// a want not done yet) answers nothing: the act's prompt is still there to word
// one from. A template the decision wrote is kept only once it has answered.

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	ws "github.com/onelittlenightmusic/want-spec"
	"gopkg.in/yaml.v3"

	mywant "mywant/engine/core"
)

var answerTemplates struct {
	mu     sync.Mutex
	loaded bool
	// path is the file they are kept in; "" is ~/.mywant/answer-templates.yaml.
	path  string
	byKey map[string]string
}

// answerTemplateFields are at most this many of a type's result fields, told
// the decision to write its template with.
const answerTemplateFields = 12

// answerTemplateMax is the longest template kept, in bytes.
const answerTemplateMax = 600

var answerField = regexp.MustCompile(`\{([A-Za-z0-9_]+)\}`)

// answerLanguage is a locale's language (ja_JP, ja-JP → ja): one template for
// every place that speaks it.
func answerLanguage(locale string) string {
	l := strings.ToLower(strings.TrimSpace(locale))
	if i := strings.IndexAny(l, "_-"); i >= 0 {
		l = l[:i]
	}
	return l
}

func answerTemplateKey(typ, locale string) string { return typ + "|" + answerLanguage(locale) }

func answerTemplatesFile() string {
	if answerTemplates.path != "" {
		return answerTemplates.path
	}
	return filepath.Join(mywant.MyWantHomeDir(), "answer-templates.yaml")
}

// lockAnswerTemplates takes the lock, reading the file the first time.
func lockAnswerTemplates() {
	answerTemplates.mu.Lock()
	if answerTemplates.loaded {
		return
	}
	answerTemplates.loaded = true
	answerTemplates.byKey = map[string]string{}
	if data, err := os.ReadFile(answerTemplatesFile()); err == nil {
		_ = yaml.Unmarshal(data, &answerTemplates.byKey)
	}
}

// answerTemplateFor is the template kept for the type in the locale's
// language, or "".
func answerTemplateFor(typ, locale string) string {
	lockAnswerTemplates()
	defer answerTemplates.mu.Unlock()
	return answerTemplates.byKey[answerTemplateKey(typ, locale)]
}

// keepAnswerTemplate keeps a template that has answered.
func keepAnswerTemplate(typ, locale, template string) {
	lockAnswerTemplates()
	defer answerTemplates.mu.Unlock()
	answerTemplates.byKey[answerTemplateKey(typ, locale)] = template
	data, err := yaml.Marshal(answerTemplates.byKey)
	if err == nil {
		path := answerTemplatesFile()
		_ = os.MkdirAll(filepath.Dir(path), 0755)
		tmp := path + ".tmp"
		if err = os.WriteFile(tmp, data, 0644); err == nil {
			err = os.Rename(tmp, path)
		}
	}
	if err != nil {
		log.Printf("[PLAN] answer template for %s not kept: %v", typ, err)
	}
}

// planTypeDef is what the brief reads of a type: its parameters, and the
// fields of its result.
type planTypeDef struct {
	Parameters []struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Required    bool   `json:"required"`
		Example     any    `json:"example"`
		SubType     string `json:"subType"`
	} `json:"parameters"`
	State []struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	} `json:"state"`
}

// answerFields are the result fields a template may name, with what each
// means: those a sentence can say — not the raw output, not a JSON or a list.
func (d planTypeDef) answerFields() (names []string, meaning map[string]string) {
	meaning = map[string]string{}
	for _, st := range d.State {
		if st.Name == "" || st.Name == "error" || robotLookSkip(st.Name) {
			continue
		}
		desc := strings.ToLower(st.Description)
		if strings.Contains(desc, "json") || strings.Contains(desc, "array") || strings.Contains(desc, "raw ") {
			continue
		}
		names = append(names, st.Name)
		meaning[st.Name] = robotFirstSentence(st.Description, 70)
	}
	return names, meaning
}

// planBriefAlso is what the decision fills besides its choice: the offered
// types' parameters (for new), and an answer template for each that has none
// kept — the lines told it, and what the brief hands the client.
func (s *Server) planBriefAlso(question, locale, timezone string) (map[string][]planParam, []string, string) {
	types := s.planTypesFor(question)
	if len(types) == 0 {
		return nil, nil, ""
	}
	now := planNow(timezone, time.Now())
	params := map[string][]planParam{}
	var needs []string
	var pb, tb strings.Builder
	for _, t := range types {
		var def planTypeDef
		if err := s.backend("GET", "/api/v1/want-types/"+t.Name, nil, &def); err != nil {
			continue
		}
		var ps []planParam
		var said []string
		for i, p := range def.Parameters {
			if i == 10 {
				break
			}
			desc := planParamDescription(p.Description, p.Example, p.SubType, now)
			ps = append(ps, planParam{Name: p.Name, Description: desc, Required: p.Required})
			said = append(said, p.Name+" ("+desc+")")
		}
		params[t.Name] = ps
		if len(said) > 0 {
			fmt.Fprintf(&pb, "- %s: %s\n", t.Name, strings.Join(said, "; "))
		}
		if answerTemplateFor(t.Name, locale) != "" {
			continue
		}
		names, meaning := def.answerFields()
		if len(names) == 0 {
			continue
		}
		if len(names) > answerTemplateFields {
			names = names[:answerTemplateFields]
		}
		needs = append(needs, t.Name)
		var fs []string
		for _, n := range names {
			if m := meaning[n]; m != "" {
				fs = append(fs, "{"+n+"} "+m)
			} else {
				fs = append(fs, "{"+n+"}")
			}
		}
		fmt.Fprintf(&tb, "- %s: %s\n", t.Name, strings.Join(fs, "; "))
	}
	var b strings.Builder
	if pb.Len() > 0 {
		b.WriteString("\nFor new, also fill the chosen type's parameters (params) from what the person said — and, when they refer to what was said earlier, from that. Leave a parameter empty when it was not said — never make a value up.\n")
		b.WriteString(planNowText(timezone, time.Now()))
		b.WriteString(pb.String())
	}
	if tb.Len() > 0 {
		b.WriteString("\nFor new, also write answer_template: what you will say once the want is made and done, one or two short sentences spoken to the person, in their language — its values not yet known, written as the fields below in braces, and {name} for the want's name. Only these fields:\n")
		b.WriteString(tb.String())
	}
	return params, needs, b.String()
}

// answerTemplateUsable is a template that names only the type's fields (and
// {name}), and not too long to be an answer.
func answerTemplateUsable(template string, fields []string) bool {
	template = strings.TrimSpace(template)
	if template == "" || template == planNone || len(template) > answerTemplateMax {
		return false
	}
	ok := map[string]bool{"name": true}
	for _, f := range fields {
		ok[f] = true
	}
	for _, m := range answerField.FindAllStringSubmatch(template, -1) {
		if !ok[m[1]] {
			return false
		}
	}
	return true
}

// answerValue is a result value as a sentence says it, or "" for none.
func answerValue(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(x)
	case float64:
		if x == math.Trunc(x) && math.Abs(x) < 1e15 {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'f', -1, 64)
	case float32, int, int32, int64, uint, uint32, uint64, bool:
		return fmt.Sprint(x)
	case json.Number:
		return x.String()
	}
	return "" // a list or a map is not said in a sentence
}

// fillAnswer is the template with every field filled from values, or false
// when one is not there.
func fillAnswer(template, name string, values func(string) any) (string, bool) {
	ok := true
	out := answerField.ReplaceAllStringFunc(template, func(m string) string {
		key := m[1 : len(m)-1]
		if key == "name" {
			return name
		}
		v := answerValue(values(key))
		if v == "" {
			ok = false
		}
		return v
	})
	return strings.TrimSpace(out), ok
}

// actAnswer is a new want's answer, worded from the type's template (kept,
// or the one the plan brings): "" when there is none or the want's result
// does not fill it. The plan's template is kept once it has answered.
func (s *Server) actAnswer(plan ws.PlanSpec, locale string, cards []*ws.RobotCard) string {
	if plan.Action != ws.PlanActionNew || plan.Type == "" {
		return ""
	}
	var card *ws.RobotCard
	for _, c := range cards {
		if c != nil && c.Kind == "want" {
			card = c
			break
		}
	}
	if card == nil {
		return ""
	}
	w := s.findWantByIDOrName(card.ID)
	if w == nil || w.Metadata.Type != plan.Type || w.GetStatus() != mywant.WantStatusAchieved {
		return ""
	}
	values := func(k string) any { return mywant.GetCurrent[any](w, k, nil) }
	if kept := answerTemplateFor(plan.Type, locale); kept != "" {
		if answer, ok := fillAnswer(kept, w.Metadata.Name, values); ok {
			return answer
		}
		return ""
	}
	proposed := strings.TrimSpace(plan.AnswerTemplate)
	if proposed == "" {
		return ""
	}
	var def planTypeDef
	if err := s.backend("GET", "/api/v1/want-types/"+plan.Type, nil, &def); err != nil {
		return ""
	}
	fields, _ := def.answerFields()
	if !answerTemplateUsable(proposed, fields) {
		log.Printf("[PLAN] answer template for %s not used (a field it does not have): %q", plan.Type, proposed)
		return ""
	}
	answer, ok := fillAnswer(proposed, w.Metadata.Name, values)
	if !ok {
		return ""
	}
	keepAnswerTemplate(plan.Type, locale, proposed)
	return answer
}
