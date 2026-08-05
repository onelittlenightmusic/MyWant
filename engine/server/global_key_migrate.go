package server

import (
	"log"
	"strings"

	mywant "mywant/engine/core"
)

// A global key names the slot a want publishes a field into. The server builds
// it from the want's name and the field's name, and it used to join the two
// with an underscore — which was unreadable, because both halves contain
// underscores of their own. `globalKeyFor` now joins with a dot.
//
// Keys already written into deployed wants are not left behind. This runs once
// over the wants restored at startup and renames them in place, on both sides:
// the expose that publishes the key and everything that reads it back.
//
// Only keys the server itself generated are touched. A key is recognised by
// reconstructing it from the publishing want's own name and field and checking
// that it matches exactly; a key the user wrote by hand looks nothing like that
// and is left alone. Nothing is renamed on the strength of containing an
// underscore.
//
// The global state is persisted too (global_state.yaml), so the values already
// published under the old keys are moved across as well. Left behind they would
// be reloaded on every start and sit beside the new ones forever — two entries
// holding the same value under two names, one of which nothing reads.

// globalKeyRenames returns old→new for every server-generated key the given
// wants publish. Built from the publishing side only — a key nobody exposes is
// not one this code wrote.
func globalKeyRenames(wants []*mywant.Want) map[string]string {
	renames := map[string]string{}
	for _, w := range wants {
		if w == nil {
			continue
		}
		slug := slugifyWantKey(w.Metadata.Name)
		if slug == "" {
			continue
		}
		for _, e := range w.Spec.Exposes {
			field := e.CurrentState
			if field == "" {
				field = e.Param
			}
			if field == "" {
				continue
			}
			old := slug + "_" + field
			for _, published := range []string{e.As, e.AsGoal, e.AsPlan, e.AsGlobalParam} {
				if published == old {
					renames[old] = slug + "." + field
				}
			}
		}
	}
	return renames
}

// renameParamRef rewrites a {fromGlobalParam: key} value in place, whatever map
// shape persistence brought it back in. Returns the new value and whether it
// changed.
func renameParamRef(v any, renames map[string]string) (any, bool) {
	switch m := v.(type) {
	case map[string]any:
		if k, ok := m["fromGlobalParam"].(string); ok {
			if neu, found := renames[k]; found {
				out := make(map[string]any, len(m))
				for mk, mv := range m {
					out[mk] = mv
				}
				out["fromGlobalParam"] = neu
				return out, true
			}
		}
	case map[string]string:
		if neu, found := renames[m["fromGlobalParam"]]; found {
			out := make(map[string]string, len(m))
			for mk, mv := range m {
				out[mk] = mv
			}
			out["fromGlobalParam"] = neu
			return out, true
		}
	case map[any]any:
		for mk, mv := range m {
			ks, ok := mk.(string)
			if !ok || ks != "fromGlobalParam" {
				continue
			}
			k, _ := mv.(string)
			if neu, found := renames[k]; found {
				out := make(map[any]any, len(m))
				for k2, v2 := range m {
					out[k2] = v2
				}
				out["fromGlobalParam"] = neu
				return out, true
			}
		}
	}
	return v, false
}

// migrateGlobalKeysAndState renames the keys across the wants and then moves the
// values the old keys already hold in the persisted global state.
//
// Order matters only in one direction: the wants are the record of what a key
// is called, so they are read first and the state follows them.
func migrateGlobalKeysAndState(wants []*mywant.Want, builder *mywant.ChainBuilder) int {
	renames := globalKeyRenames(wants)
	touched := migrateGlobalKeys(wants, renames)
	if builder != nil && len(renames) > 0 {
		for old, neu := range renames {
			if v, ok := builder.GetGlobalStateValue(old); ok {
				// Do not overwrite a value already living under the new name: a
				// want that has run since the rename knows better than a
				// snapshot taken before it.
				if _, exists := builder.GetGlobalStateValue(neu); !exists {
					builder.StoreGlobalState(neu, v)
				}
				builder.DeleteGlobalStateKey(old)
			}
			// A structured value is also flattened into sub-keys
			// ("<key>.lat"), and those carry the old name in their prefix.
			// They are moved whether or not the bare key holds anything —
			// for a value that only ever existed flattened, they are all
			// there is.
			for k, sub := range builder.GetGlobalStateAll() {
				if !strings.HasPrefix(k, old+".") {
					continue
				}
				builder.StoreGlobalState(neu+strings.TrimPrefix(k, old), sub)
				builder.DeleteGlobalStateKey(k)
			}
		}
	}
	return touched
}

// migrateGlobalKeys rewrites underscore-joined global keys to dot-joined ones
// across every want that publishes or reads them. Returns how many wants it
// touched, for the startup log.
func migrateGlobalKeys(wants []*mywant.Want, renames map[string]string) int {
	if len(renames) == 0 {
		return 0
	}
	rename := func(key string) (string, bool) {
		neu, ok := renames[key]
		return neu, ok
	}

	touched := 0
	for _, w := range wants {
		if w == nil {
			continue
		}
		changed := false

		for i := range w.Spec.Exposes {
			e := &w.Spec.Exposes[i]
			for _, field := range []*string{&e.As, &e.AsGoal, &e.AsPlan, &e.AsGlobalParam} {
				if neu, ok := rename(*field); ok {
					*field = neu
					changed = true
				}
			}
		}

		if len(w.Spec.Imports) > 0 {
			imports := make(map[string]string, len(w.Spec.Imports))
			for k, v := range w.Spec.Imports {
				if neu, ok := rename(k); ok {
					imports[neu] = v
					changed = true
					continue
				}
				imports[k] = v
			}
			w.Spec.Imports = imports
		}

		for k, v := range w.Spec.Params {
			if neu, ok := renameParamRef(v, renames); ok {
				w.Spec.Params[k] = neu
				changed = true
			}
		}

		for i := range w.Spec.When {
			if neu, ok := rename(w.Spec.When[i].FromGlobalParam); ok {
				w.Spec.When[i].FromGlobalParam = neu
				changed = true
			}
		}

		// Correlation labels carry the key inside them
		// ("stateAccess/consumer:expose/<key>"). They are derived from the specs
		// and would be rebuilt eventually, but they are also what was persisted
		// and what the board reads to label a road — so a restart must not show
		// the old name while waiting for a rebuild that only a change triggers.
		for i := range w.Metadata.Correlation {
			labels := w.Metadata.Correlation[i].Labels
			for j, l := range labels {
				idx := strings.LastIndex(l, "expose/")
				if idx < 0 {
					continue
				}
				if neu, ok := rename(l[idx+len("expose/"):]); ok {
					labels[j] = l[:idx+len("expose/")] + neu
					changed = true
				}
			}
		}

		if changed {
			touched++
		}
	}

	if touched > 0 {
		pairs := make([]string, 0, len(renames))
		for old, neu := range renames {
			pairs = append(pairs, old+"→"+neu)
		}
		log.Printf("[SERVER] renamed %d global key(s) across %d want(s): %s",
			len(renames), touched, strings.Join(pairs, ", "))
	}
	return touched
}
