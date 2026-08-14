package server

import (
	mywant "mywant/engine/core"
	want_spec "github.com/onelittlenightmusic/want-spec"
)

// Resolving the names people give things, once, on the server.
//
// A parameter that accepts a catalog kind takes a NAME: `from: 自宅` on a route,
// `place: ジム` on a place_arrival. What the name stands for — the coordinate,
// the URL, whatever was named — lives in the thing ledger, and every consumer
// that wanted it went and got it for itself. place_arrival walks the aura
// catalog in Go; the route card walked the definitions again in TypeScript;
// each knew a slightly different list of kinds to try, and a third consumer
// would have written a third version.
//
// So the want carries the answer. Each parameter that names something gets a
// companion state field with the value that name stands for, filled in here and
// refreshed whenever the want is written. A card reads a field like any other
// field, in any language, without knowing that a ledger exists.

// resolvedSuffix names the companion field: `from` → `from_resolved`.
//
// A separate field rather than a rewrite of the parameter, because the name is
// the part a person meant. Replacing 自宅 with a pair of numbers would answer
// the question and lose it: the card could no longer say where it was going,
// and re-editing the want would show coordinates nobody typed.
const resolvedSuffix = "_resolved"

// resolveThingName returns what a name stands for in any of the given catalog
// kinds, and which kind answered.
//
// Definitions come from deriveThingDefinitions, which merges the ledger with
// what characters carry — the same list the Thing page and the map read, so a
// name resolves here exactly as it displays there. Newest definition wins, by
// its own timestamp, which is the rule everywhere else a shared name is
// resolved.
func (s *Server) resolveThingName(name string, kinds []string) (value any, kind string, ok bool) {
	if name == "" || len(kinds) == 0 {
		return nil, "", false
	}
	wanted := map[string]bool{}
	for _, k := range kinds {
		wanted[k] = true
	}
	bestAt := ""
	for _, d := range s.deriveThingDefinitions() {
		if d.Name != name || !wanted[d.Subtype] || d.Value == nil {
			continue
		}
		if value != nil && d.At <= bestAt {
			continue
		}
		value, kind, bestAt = d.Value, d.Subtype, d.At
	}
	return value, kind, value != nil
}

// resolveThingValues is what each of this want's named parameters stands for:
// `from: 自宅` yields `from_resolved` → the coordinate named 自宅.
//
// Answered on every read rather than stored on the want. A stored copy has to
// be kept fresh against three separate events — the want being edited, the name
// being defined, the name being taken back — and the first attempt here did
// exactly that, with a creation hook, an update call and a refresh from the
// naming endpoints. It still came out wrong on creation, because a want's state
// is built after the hooks run and the answer was written to a want that was
// about to be replaced. Derived on read, there is nothing to keep in sync and
// nothing that can go stale: the answer is worked out from the ledger as it
// stands at the moment somebody asks.
func (s *Server) resolveThingValues(want *mywant.Want) map[string]any {
	if want == nil || s.globalBuilder == nil {
		return nil
	}
	def := s.globalBuilder.GetWantTypeDefinition(want.Metadata.Type)
	if def == nil {
		return nil
	}
	var out map[string]any
	for _, p := range def.Parameters {
		kinds := catalogKinds(p)
		if len(kinds) == 0 {
			continue
		}
		name, _ := want.Spec.Params[p.Name].(string)
		value, _, ok := s.resolveThingName(name, kinds)
		if !ok {
			continue
		}
		if out == nil {
			out = map[string]any{}
		}
		out[p.Name+resolvedSuffix] = value
	}
	return out
}

// catalogKinds is the subtypes of a parameter that name a catalog entry, i.e.
// the ones a value here could be the NAME of rather than the thing itself.
//
// Every accepted subtype is considered, not just the declared one: a route
// records what is typed as a station and also takes a named place, and it is
// the place that needs resolving.
func catalogKinds(p want_spec.ParameterDef) []string {
	var out []string
	for _, st := range p.AcceptedSubTypes() {
		if st != "" {
			out = append(out, st)
		}
	}
	return out
}
