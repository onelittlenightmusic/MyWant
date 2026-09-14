package mywant

import (
	"strings"
	"testing"

	want_spec "github.com/onelittlenightmusic/want-spec"
)

// A finalize condition reading a persistent field is a want that can never
// leave that state — the two shapes that actually bit, one per direction.
func TestFinalizeGateWarnings(t *testing.T) {
	def := func(persistent bool, fw *want_spec.FinalizeWhen) *WantTypeDefinition {
		return &WantTypeDefinition{
			Metadata:     WantTypeMetadata{Name: "t"},
			State:        []StateDef{{Name: "gate", Type: "string", Label: "current", Persistent: persistent}},
			FinalizeWhen: fw,
		}
	}
	cond := &want_spec.ConditionDef{Field: "gate", Operator: "!=", Value: ""}

	// transit_search: a persistent `summary` behind the achieve gate meant the
	// want was achieved on arrival and never searched again.
	got := finalizeGateWarnings(def(true, &want_spec.FinalizeWhen{Achieved: cond}))
	if len(got) != 1 || !strings.Contains(got[0], "never re-runs") {
		t.Errorf("achieved gate on a persistent field: got %v", got)
	}

	// smartgolf_check_reserved: a persistent `error` behind the fail gate meant
	// a 504 the want could never be restarted out of.
	got = finalizeGateWarnings(def(true, &want_spec.FinalizeWhen{Failed: cond}))
	if len(got) != 1 || !strings.Contains(got[0], "never be restarted out of it") {
		t.Errorf("failed gate on a persistent field: got %v", got)
	}

	// Both at once is two warnings, since they are two different frozen states.
	if got = finalizeGateWarnings(def(true, &want_spec.FinalizeWhen{Achieved: cond, Failed: cond})); len(got) != 2 {
		t.Errorf("both gates: want 2 warnings, got %d: %v", len(got), got)
	}

	// The healthy shape — the gate resets, the answer fields keep themselves —
	// says nothing at all.
	if got = finalizeGateWarnings(def(false, &want_spec.FinalizeWhen{Achieved: cond, Failed: cond})); len(got) != 0 {
		t.Errorf("non-persistent gate should be silent, got %v", got)
	}
	// And so does a type with no finalize contract.
	if got = finalizeGateWarnings(def(true, nil)); len(got) != 0 {
		t.Errorf("no finalizeWhen should be silent, got %v", got)
	}
	// A gate naming a field the type never declared is somebody else's bug, and
	// not this one's to guess at.
	missing := def(true, &want_spec.FinalizeWhen{Achieved: &want_spec.ConditionDef{Field: "nope"}})
	if got = finalizeGateWarnings(missing); len(got) != 0 {
		t.Errorf("undeclared gate field should be silent, got %v", got)
	}
}
