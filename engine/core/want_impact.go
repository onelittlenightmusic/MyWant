package mywant

import (
	"fmt"
	"sync/atomic"
)

// What a want just did to the wants it reaches, kept on the want that acted
// (predefined.yaml's `impact`), for whoever draws the board to show it
// landing on each of them: a value it handed on (feed), or any kind a
// plugin's want writes itself (the battle plugin's attack, mend, freeze).
const (
	ImpactField = "impact"
	ImpactFeed  = "feed"
)

// impactSeq numbers impacts, so the same kind on the same wants again is
// still a change of the field — another one to show.
var impactSeq atomic.Int64

// impactValueMax is how much of a value handed on is said with it: enough to
// read at a glance over a tile, not the value itself.
const impactValueMax = 24

// RecordImpact records that this want's connections just had their effect: of
// what kind, on which wants (by name), and — for a value handed on — what it
// was, shortened, when it can be said in a few words.
func (n *Want) RecordImpact(kind string, to []string, value any) {
	if len(to) == 0 {
		return
	}
	imp := map[string]any{"kind": kind, "to": to, "n": impactSeq.Add(1)}
	if s, ok := impactValueText(value); ok {
		imp["value"] = s
	}
	n.StoreState(ImpactField, imp)
}

func impactValueText(v any) (string, bool) {
	var s string
	switch x := v.(type) {
	case nil:
		return "", false
	case string:
		s = x
	case bool, int, int64, float64, float32:
		s = fmt.Sprint(x)
	default:
		return "", false // a map or a list says nothing in a few words
	}
	if s == "" {
		return "", false
	}
	if r := []rune(s); len(r) > impactValueMax {
		s = string(r[:impactValueMax-1]) + "…"
	}
	return s, true
}

// globalParamReaders is the names of the wants that read the global parameter
// `key` — the wants a value exposed as it is handed on to.
func globalParamReaders(key string) []string {
	wantRegistryMutex.RLock()
	defer wantRegistryMutex.RUnlock()
	var out []string
	for name, w := range wantRegistry {
		for _, k := range globalParamRefsOf(w) {
			if k == key {
				out = append(out, name)
				break
			}
		}
	}
	return out
}
