package server

import (
	"strings"
	"testing"
)

// The same call twice in a turn is the same call whatever the type name's case.
func TestFMSameArgs(t *testing.T) {
	if !fmSameArgs(map[string]string{"type": "transit_search"}, map[string]string{"type": "TRANSIT_SEARCH"}) {
		t.Error("a type named in another case is the same call")
	}
	if fmSameArgs(map[string]string{"params": "from=大森, to=国分寺"}, map[string]string{"params": "from=新宿, to=国分寺"}) {
		t.Error("other parameters are another call")
	}
	if fmSameArgs(map[string]string{"a": "1"}, map[string]string{"a": "1", "b": "2"}) {
		t.Error("an argument more is another call")
	}
}

// What the model hears the second time: what it got, and to answer now.
func TestFMRepeatOutput(t *testing.T) {
	out := fmRepeatOutput(fmStep{Output: "Deployed \"新宿→国分寺\" (transit_search), next to the person.\nthe rest"})
	if !strings.Contains(out, "新宿→国分寺") || strings.Contains(out, "the rest") || !strings.Contains(out, "Answer the person now") {
		t.Errorf("got %q", out)
	}
}
