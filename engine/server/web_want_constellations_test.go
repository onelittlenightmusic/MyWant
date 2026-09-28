package server

import (
	"reflect"
	"testing"
)

// A chain keeps only marks the page has, each once, in the order tied; fewer
// than two left, or no name, and it is not a constellation.
func TestCleanWebWantConstellations(t *testing.T) {
	els := []WebWantElement{{Selector: "#a"}, {Selector: "#b"}, {Selector: "#c"}}
	got := cleanWebWantConstellations([]WebWantConstellation{
		{Name: " 検索の流れ ", Selectors: []string{"#c", "#gone", "#a", "#c", "#b"}},
		{Name: "ひとつだけ", Selectors: []string{"#a", "#gone"}},
		{Name: "", Selectors: []string{"#a", "#b"}},
		{Name: "検索の流れ", Selectors: []string{"#a", "#b"}},
	}, els)
	want := []WebWantConstellation{{Name: "検索の流れ", Selectors: []string{"#c", "#a", "#b"}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}
