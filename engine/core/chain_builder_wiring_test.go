package mywant

import "testing"

func TestParseWireRef(t *testing.T) {
	cases := []struct{ in, target, state string; ok bool }{
		{"want:transit-1/departure", "transit-1", "departure", true},
		{"want:my transit/arrival", "my transit", "arrival", true},
		// A name with a slash in it still resolves: the LAST slash is the one
		// that separates the state, because the state never has one.
		{"want:a/b/departure", "a/b", "departure", true},
		{"leave_at", "", "", false},
		{"want:departure", "", "", false},
		{"want:/departure", "", "", false},
		{"want:transit-1/", "", "", false},
	}
	for _, c := range cases {
		target, state, ok := parseWireRef(c.in)
		if ok != c.ok || target != c.target || state != c.state {
			t.Errorf("parseWireRef(%q) = (%q,%q,%v), want (%q,%q,%v)", c.in, target, state, ok, c.target, c.state, c.ok)
		}
	}
}

// The reconciler's own question: is the provider already publishing this, in
// the flavour this inlet needs? A global state key and a global parameter are
// two different publications of the same field.
func TestHasExposeForTellsTheTwoFlavoursApart(t *testing.T) {
	w := &Want{}
	w.Spec.Exposes = []ExposeEntry{{CurrentState: "departure", AsGlobalParam: "k"}}

	if !hasExposeFor(w, "departure", "k", true) {
		t.Error("published as a global parameter, which is what a parameter inlet reads")
	}
	if hasExposeFor(w, "departure", "k", false) {
		t.Error("a global parameter is not a global state key; a state inlet is not fed by it")
	}
	if hasExposeFor(w, "arrival", "k", true) {
		t.Error("wrong field")
	}
}
