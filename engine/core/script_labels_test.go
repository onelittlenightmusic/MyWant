package mywant

import "testing"

func TestApplyLabelUpdates(t *testing.T) {
	want := &Want{Metadata: Metadata{Name: "transit", ID: "t1", Type: "transit_search",
		Labels: map[string]string{"mywant.io/canvas-x": "4", "tile-old": "gone"}}}

	applyLabelUpdates(want, map[string]any{"label_updates": map[string]any{
		"tile-effect-colors": []any{"#9ACD32", "#F15A22"},
		"tile-readout":       " fare ",
		"tile-old":           nil,
		"mywant.io/canvas-x": "99",
		"tile-bad":           42,
	}}, "test")

	got := want.GetLabels()
	if got["tile-effect-colors"] != "#9ACD32,#F15A22" {
		t.Errorf("a list is joined with commas, got %q", got["tile-effect-colors"])
	}
	if got["tile-readout"] != "fare" {
		t.Errorf("a string is written trimmed, got %q", got["tile-readout"])
	}
	if _, ok := got["tile-old"]; ok {
		t.Errorf("null takes the label away")
	}
	if got["mywant.io/canvas-x"] != "4" {
		t.Errorf("a label outside tile- is not a script's to change, got %q", got["mywant.io/canvas-x"])
	}
	if _, ok := got["tile-bad"]; ok {
		t.Errorf("a value that is neither string nor list is skipped")
	}
}

func TestApplyLabelUpdatesWithoutKey(t *testing.T) {
	want := &Want{Metadata: Metadata{Name: "w", ID: "w1", Labels: map[string]string{"tile-x": "1"}}}
	applyLabelUpdates(want, map[string]any{"routes": []any{}}, "test")
	if want.GetLabels()["tile-x"] != "1" {
		t.Errorf("output without label_updates leaves the labels as they are")
	}
}
