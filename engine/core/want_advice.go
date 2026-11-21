package mywant

import (
	"context"
	"encoding/json"
	"strings"
)

// Advice on a want's new answer: what its type knows the answer means for
// whoever acts on it — rain means an umbrella — given apart from its Progress
// by the type's advisor (a Go function, RegisterAdvisor, or <type>.advice.py)
// and told with the answer to the wants that hear it, which take up the kinds
// they act on (a checklist, bring). In set kinds, so any want can read any
// type's.

// The kinds of advice.
const (
	AdviceBring      = "bring"       // something to take along
	AdviceWear       = "wear"        // something to wear
	AdviceLeaveEarly = "leave_early" // set off earlier than planned
	AdviceAvoid      = "avoid"       // something better not done
)

var adviceKinds = map[string]bool{AdviceBring: true, AdviceWear: true, AdviceLeaveEarly: true, AdviceAvoid: true}

// Advice is one piece: its kind, what (an item, in words a person uses), and
// why (to say to the person).
type Advice struct {
	Kind string `json:"kind"`
	Item string `json:"item"`
	Why  string `json:"why,omitempty"`
}

// AdviceInput is what an advisor reads: the answer, what changed in it, and
// the want it is of.
type AdviceInput struct {
	WantID   string         `json:"want_id"`
	WantName string         `json:"want_name"`
	WantType string         `json:"want_type"`
	Timings  []string       `json:"timings,omitempty"`
	Result   any            `json:"result,omitempty"`
	Fields   map[string]any `json:"fields,omitempty"`
	Change   []Change       `json:"change,omitempty"`
}

// Advisor gives a type's advice on an answer.
type Advisor func(in AdviceInput) []Advice

var advisors = map[string]Advisor{}

// RegisterAdvisor gives a Go type its advisor (from its package's init).
func RegisterAdvisor(wantType string, a Advisor) { advisors[wantType] = a }

// AdviceOn is a want's advice on its answer: its type's advisor, then its
// advice script (script, "" for none) — only kinds known, each once.
func AdviceOn(ctx context.Context, w *Want, entry ResultHistoryEntry, script string) []Advice {
	if w == nil {
		return nil
	}
	in := AdviceInput{
		WantID: w.Metadata.ID, WantName: w.Metadata.Name, WantType: w.Metadata.Type,
		Timings: entry.Timings, Result: entry.Result, Fields: entry.Fields, Change: entry.Change,
	}
	var got []Advice
	if a := advisors[in.WantType]; a != nil {
		got = append(got, a(in)...)
	}
	if script != "" {
		got = append(got, scriptAdvice(ctx, w, script, in)...)
	}
	return validAdvice(got)
}

func scriptAdvice(ctx context.Context, w *Want, script string, in AdviceInput) []Advice {
	input, err := json.Marshal(in)
	if err != nil {
		return nil
	}
	out, err := runSideScript(ctx, script, input)
	if err != nil {
		ErrorLog("[ADVICE] %s (%s): %v", w.Metadata.Name, script, err)
		return nil
	}
	raw, err := json.Marshal(out["advice"])
	if err != nil {
		return nil
	}
	var list []Advice
	_ = json.Unmarshal(raw, &list)
	return list
}

// validAdvice keeps the known kinds with something to say, each once.
func validAdvice(list []Advice) []Advice {
	var out []Advice
	seen := map[string]bool{}
	for _, a := range list {
		a.Item = strings.TrimSpace(a.Item)
		key := a.Kind + "|" + a.Item
		if !adviceKinds[a.Kind] || a.Item == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, a)
	}
	return out
}
