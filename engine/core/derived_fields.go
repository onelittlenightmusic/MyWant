package mywant

import (
	"encoding/json"
	"fmt"
	"strings"
)

// DerivedFieldsLabel is the want label under which the GUI persists user-defined
// computed fields (JSON). Each entry is { key, expr }, where expr is either a
// "text" concatenation of field refs + literals (A + "+" + B) or a "json" object
// whose values are themselves exprs ({A: a, B: b}). Stored as a label (rather
// than a want-spec field) so it persists per-instance and the GUI can patch it
// through the existing label-patch path — no want_spec change needed.
const DerivedFieldsLabel = "mywant.io/derived-fields"

// evaluateDerivedFields recomputes every derived field into current state. Called
// from the reconcile cycle, which runs on state changes (not a fixed tick), so
// e.g. C = A + B refreshes whenever A or B changes and stays put otherwise.
func (n *Want) evaluateDerivedFields() {
	n.metadataMutex.RLock()
	raw := ""
	if n.Metadata.Labels != nil {
		raw = n.Metadata.Labels[DerivedFieldsLabel]
	}
	n.metadataMutex.RUnlock()
	if raw == "" {
		return
	}
	var defs []struct {
		Key  string         `json:"key"`
		Expr map[string]any `json:"expr"`
	}
	if err := json.Unmarshal([]byte(raw), &defs); err != nil {
		return
	}
	get := func(field string) (any, bool) { return n.getState(field) }
	for _, d := range defs {
		if d.Key == "" || d.Expr == nil {
			continue
		}
		// Register in ProvidedStateFields so the GUI-added key counts as explicit
		// state (buildWantAPIResponse's "current" bucket) instead of hidden_state,
		// same as recipe/type-declared state fields (see owner_types.go).
		if !Contains(n.ProvidedStateFields, d.Key) {
			n.ProvidedStateFields = append(n.ProvidedStateFields, d.Key)
		}
		n.storeState(d.Key, evalDerivedExpr(d.Expr, get))
	}
}

// evalDerivedExpr evaluates one expr node against the want's current state.
func evalDerivedExpr(expr map[string]any, get func(string) (any, bool)) any {
	kind, _ := expr["kind"].(string)
	if kind == "json" {
		entries, _ := expr["entries"].([]any)
		out := map[string]any{}
		for _, e := range entries {
			em, ok := e.(map[string]any)
			if !ok {
				continue
			}
			k, _ := em["key"].(string)
			ve, _ := em["value"].(map[string]any)
			if k == "" || ve == nil {
				continue
			}
			out[k] = evalDerivedExpr(ve, get)
		}
		return out
	}

	// "text" (default): concatenate token values. A lone bare field token keeps
	// the field's raw value/type (so a JSON entry {A: a} carries a's number, not
	// its string form); any literal or multi-token expr becomes a string.
	tokens, _ := expr["tokens"].([]any)
	if len(tokens) == 1 {
		if tm, ok := tokens[0].(map[string]any); ok {
			if f, ok := tm["field"].(string); ok {
				if v, present := get(f); present {
					return v
				}
				return nil
			}
		}
	}
	var sb strings.Builder
	for _, t := range tokens {
		tm, ok := t.(map[string]any)
		if !ok {
			continue
		}
		if f, ok := tm["field"].(string); ok {
			if v, present := get(f); present {
				sb.WriteString(derivedToString(v))
			}
		} else if l, ok := tm["literal"].(string); ok {
			sb.WriteString(l)
		}
	}
	return sb.String()
}

func derivedToString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}
