package mywant

import (
	"strings"
	"time"

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
		consumer *Want
		// The parameter this lands in, for a param inlet — what a conversion
		// would have to be registered against.
		param string
	}
	byTarget := map[string][]need{}

	for _, rw := range cb.wants {
		w := rw.want
		if w == nil {
			continue
		}
		for key := range w.Spec.Imports {
			if target, state, ok := parseWireRef(key); ok {
				byTarget[target] = append(byTarget[target], need{state: state, key: key, consumer: w})
			}
		}
		for pname, v := range w.Spec.Params {
			ref, ok := v.(map[string]any)
			if !ok {
				continue
			}
			key, _ := ref["fromGlobalParam"].(string)
			if target, state, ok := parseWireRef(key); ok {
				byTarget[target] = append(byTarget[target], need{state: state, key: key, asParam: true, consumer: w, param: pname})
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
			DebugLog("[Wiring] %s now publishes %s for %s", provider.Metadata.Name, n.state, n.consumer.Metadata.Name)
		}
		// A wire can be right and still not usable. Both ends declare what kind
		// of value they deal in, and when those differ by writing rather than by
		// meaning the board converts rather than complains — see
		// ConvertSubTypeValue.
		for _, n := range needs {
			if !n.asParam || n.param == "" {
				continue
			}
			cb.registerParamConversion(n.consumer, n.param, provider, n.state)
			// And read it now. A want resolves its {fromGlobalParam} references
			// when its type definition is set, which is before anything has
			// published — and a want that declares where its value comes from
			// is precisely the case where the publisher did not exist yet. Left
			// alone it would wait for a restart to notice.
			n.consumer.ResolveGlobalParamRef(n.param)
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

// registerParamConversion tells the consumer what shape the value arriving in
// this parameter is written in, when it differs from the shape the parameter is
// read in.
//
// Both ends already say so: a state field and a parameter each declare a
// subType in their want type. Nothing was comparing them, so a departure
// ("22:47") landed in an event_time that only reads RFC3339, the reminder took
// it as malformed, and the wire — which was correct — did nothing. The types
// were there to be asked all along.
func (cb *ChainBuilder) registerParamConversion(consumer *Want, param string, provider *Want, state string) {
	if consumer == nil || provider == nil || cb.wantTypeDefinitions == nil {
		return
	}
	from := ""
	if def, ok := cb.wantTypeDefinitions[provider.Metadata.Type]; ok && def != nil {
		for _, sd := range def.State {
			if sd.Name == state {
				from = sd.SubType
				break
			}
		}
	}
	to := ""
	if def, ok := cb.wantTypeDefinitions[consumer.Metadata.Type]; ok && def != nil {
		for _, pd := range def.Parameters {
			if pd.Name == param {
				to = pd.SubType
				break
			}
		}
	}
	if from == "" || to == "" || from == to {
		return
	}
	// Only register what can actually be converted, so an unconvertible
	// mismatch stays visible rather than being quietly passed through a
	// conversion that never fires.
	if _, ok := ConvertSubTypeValue("00:00", from, to); !ok {
		if _, ok := ConvertSubTypeValue(time.Now().Format(time.RFC3339), from, to); !ok {
			return
		}
	}
	consumer.SetParamConversion(param, from, to)
	DebugLog("[Wiring] %s.%s reads %s", consumer.Metadata.Name, param,
		describeConversion(paramConversion{From: from, To: to}))
}
