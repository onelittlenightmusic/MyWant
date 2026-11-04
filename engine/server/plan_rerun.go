package server

// plan_rerun.go — a want run again with what the question changes.
//
// 「出発時刻を今にして再検索」 is about the route already on the board: run it
// again (existing, restart) — but from now, not from the 21:17 it was asked
// before. Run again as it was, it searched 21:17 again.
//
// So a plan that runs a want again carries, in its params, what the question
// changes. The model fills them as it fills a new want's (/plans/params, given
// the want as ref): told the want's parameters as they are and what time it
// is now, it fills only those the question changes. The act sets them on the
// want (PATCH, the rest left as they are) and then runs it again; the answer
// hears what was set.

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// rerunCapability is the standard capability a plan's params go with.
const rerunCapability = "restart"

// rerunWant is a want on the board a plan runs again.
type rerunWant struct {
	id, name, typ string
	params        map[string]any
}

// rerunWantNamed is the want a plan's ref names (robotWantNamed).
func (s *Server) rerunWantNamed(ref string) (*rerunWant, error) {
	w, err := s.robotWantNamed(ref)
	if err != nil {
		return nil, err
	}
	params := map[string]any{}
	if spec := w.GetSpec(); spec != nil {
		for k, v := range spec.Params {
			params[k] = v
		}
	}
	return &rerunWant{id: w.Metadata.ID, name: w.Metadata.Name, typ: w.Metadata.Type, params: params}, nil
}

// rerunParamsText tells the model what the want is asked now, and to fill
// only what the question changes.
func rerunParamsText(params map[string]any) string {
	var b strings.Builder
	if len(params) > 0 {
		b.WriteString("Its parameters now: " + robotParamsText(params) + ".\n")
	}
	b.WriteString("Fill only the parameters the person changes — the new value, from what they said. Leave every other parameter empty: it stays as it is. Never make a value up.\n")
	return b.String()
}

// planNowText is what time it is for the asker, so 「今」「30分後」 can be
// filled: their timezone when given and known, else the server's.
func planNowText(timezone string, now time.Time) string {
	if loc, err := time.LoadLocation(strings.TrimSpace(timezone)); timezone != "" && err == nil {
		now = now.In(loc)
	}
	return fmt.Sprintf("It is now %s (%s, %s).\n", now.Format("2006-01-02 15:04"), now.Weekday(), now.Format("MST"))
}

// rerunChange is what is set on a want: the params given that say something.
func rerunChange(params map[string]string) map[string]any {
	out := map[string]any{}
	for k, v := range params {
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if k == "" || v == "" || v == planNone {
			continue
		}
		out[k] = v
	}
	return out
}

// changeWantParams sets what the plan changes on the want ref names, the rest
// left as they are, and says what it set.
func (s *Server) changeWantParams(ref string, params map[string]string) (string, error) {
	change := rerunChange(params)
	if len(change) == 0 {
		return "", nil
	}
	w, err := s.rerunWantNamed(ref)
	if err != nil {
		return "", err
	}
	body := map[string]any{"spec": map[string]any{"params": change}}
	if err := s.backend("PATCH", "/api/v1/wants/"+w.id, body, nil); err != nil {
		return "", fmt.Errorf("could not change %q: %v", w.name, err)
	}
	keys := make([]string, 0, len(change))
	for k := range change {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = fmt.Sprintf("%s=%v", k, change[k])
	}
	return fmt.Sprintf("Set %s on %q.", strings.Join(parts, ", "), w.name), nil
}
