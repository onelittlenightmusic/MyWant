package mywant

import (
	"strings"

	ws "github.com/onelittlenightmusic/want-spec"
)

// wireRefPrefix marks a reference to another want's state, written where a
// global key would otherwise go.
//
//	imports: { "want:<id or name>/<state>": <my state key> }
//	params:  { <p>: { fromGlobalParam: "want:<id or name>/<state>" } }
//
// The point of the shape is that it is a DECLARATION rather than a lookup. A
// plain global key says "whatever happens to be published here"; this says
// "that want's departure", and if nothing is publishing it yet then the world
// is not in the state the want asked for and the reconciler puts it right —
// see wirePhase.
const wireRefPrefix = "want:"

// parseWireRef splits "want:<target>/<state>" into its two halves.
func parseWireRef(key string) (target, state string, ok bool) {
	if !strings.HasPrefix(key, wireRefPrefix) {
		return "", "", false
	}
	rest := strings.TrimPrefix(key, wireRefPrefix)
	slash := strings.LastIndex(rest, "/")
	if slash <= 0 || slash == len(rest)-1 {
		return "", "", false
	}
	return rest[:slash], rest[slash+1:], true
}

// wirePhase makes the board true to what its wants declared.
//
// A want that says its parameter comes from another want's state has declared
// something about the OTHER want too: that it publishes that state. Nothing
// says a human has to go and arrange that. So this walks the declarations, and
// wherever a provider is not yet publishing what somebody is reading, it adds
// the expose that makes it so.
//
// Which expose depends on the inlet, because the board has two and they are
// not interchangeable: a state field is fed by a global state key (`as`), a
// parameter by a global parameter (`asGlobalParam`), and a want reads its
// parameters from a different place than its state.
//
// Idempotent by construction — an expose that is already there is left alone —
// so this runs on every reconcile and settles.
func (cb *ChainBuilder) wirePhase() {
	type need struct {
		state    string
		key      string
		asParam  bool
		consumer string
	}
	byTarget := map[string][]need{}

	for _, rw := range cb.wants {
		w := rw.want
		if w == nil {
			continue
		}
		for key := range w.Spec.Imports {
			if target, state, ok := parseWireRef(key); ok {
				byTarget[target] = append(byTarget[target], need{state: state, key: key, consumer: w.Metadata.Name})
			}
		}
		for _, v := range w.Spec.Params {
			ref, ok := v.(map[string]any)
			if !ok {
				continue
			}
			key, _ := ref["fromGlobalParam"].(string)
			if target, state, ok := parseWireRef(key); ok {
				byTarget[target] = append(byTarget[target], need{state: state, key: key, asParam: true, consumer: w.Metadata.Name})
			}
		}
	}
	if len(byTarget) == 0 {
		return
	}

	for target, needs := range byTarget {
		provider := cb.findWireTarget(target)
		if provider == nil {
			// Named a want that is not here. Left alone rather than guessed at:
			// the reference stays unresolved, which is the honest state, and it
			// resolves by itself if that want arrives later.
			continue
		}
		changed := false
		for _, n := range needs {
			if hasExposeFor(provider, n.state, n.key, n.asParam) {
				continue
			}
			entry := ws.ExposeEntry{CurrentState: n.state}
			if n.asParam {
				entry.AsGlobalParam = n.key
			} else {
				entry.As = n.key
			}
			provider.Spec.Exposes = append(provider.Spec.Exposes, entry)
			changed = true
			DebugLog("[Wiring] %s now publishes %s for %s", provider.Metadata.Name, n.state, n.consumer)
		}
		if changed {
			// Re-register so the new expose gets its subscription — and so the
			// value the field already holds is published now rather than on its
			// next change, which for a finished want may be never.
			RegisterWant(provider)
		}
	}
}

// findWireTarget resolves a reference by id first and by name second. An id is
// what a machine writes and a name is what a person writes; both name the same
// want, and refusing one of them would only make the notation harder to use.
func (cb *ChainBuilder) findWireTarget(target string) *Want {
	for _, rw := range cb.wants {
		if rw.want != nil && rw.want.Metadata.ID == target {
			return rw.want
		}
	}
	for _, rw := range cb.wants {
		if rw.want != nil && rw.want.Metadata.Name == target {
			return rw.want
		}
	}
	return nil
}

// hasExposeFor reports whether this want already publishes that state under
// that key, in the flavour the inlet needs.
func hasExposeFor(w *Want, state, key string, asParam bool) bool {
	for _, e := range w.Spec.Exposes {
		if e.CurrentState != state {
			continue
		}
		if asParam && e.AsGlobalParam == key {
			return true
		}
		if !asParam && e.As == key {
			return true
		}
	}
	return false
}
