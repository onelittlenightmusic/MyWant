package mywant

import "testing"

// A param edited from a reference to a literal reads the literal, not what the
// reference last resolved to.
func TestGetParameterLiteralOverStaleResolved(t *testing.T) {
	w := NewWantWithLocals(Metadata{Name: "route", Type: "transit_search"},
		WantSpec{Params: map[string]any{"from": map[string]any{"fromGlobalParam": "transit_search_instance.from"}}}, nil, "base")
	w.setResolvedParam("from", "国分寺")
	if v, _ := w.GetParameter("from"); v != "国分寺" {
		t.Fatalf("while a reference: %v", v)
	}
	w.Spec.Params["from"] = "中野坂上"
	if v, _ := w.GetParameter("from"); v != "中野坂上" {
		t.Errorf("after the edit to a literal: %v", v)
	}
}
