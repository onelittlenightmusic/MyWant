package server

import "testing"

// A want the robot makes is named by its type's rule when it has one, and
// only when every parameter the rule names was given.
func TestFmNameFrom(t *testing.T) {
	route := "{from}→{to}"
	if got := robotNameFrom(route, map[string]any{"from": "中野坂上", "to": "戸塚"}); got != "中野坂上→戸塚" {
		t.Fatalf("both given: %q", got)
	}
	if got := robotNameFrom(route, map[string]any{"to": "戸塚"}); got != "" {
		t.Fatalf("from missing should leave the model's name: %q", got)
	}
	if got := robotNameFrom("", map[string]any{"to": "戸塚"}); got != "" {
		t.Fatalf("no rule: %q", got)
	}
}

// The parameters as the model is told them, in a steady order.
func TestFmParamsText(t *testing.T) {
	if got := robotParamsText(map[string]any{"to": "戸塚", "from": "中野坂上", "time": ""}); got != "from=中野坂上, to=戸塚" {
		t.Fatalf("got %q", got)
	}
	if got := robotParamsText(nil); got != "no parameters" {
		t.Fatalf("got %q", got)
	}
}
