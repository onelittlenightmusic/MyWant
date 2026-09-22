package server

import (
	"path/filepath"
	"testing"

	mywant "github.com/onelittlenightmusic/want-spec"
)

// A parameter that widened what it takes is usually filled from something the
// board already has a name for, and naming it again under the field's own
// heading is how 東京ドーム — a location somebody had named — became a station
// as well, standing beside itself under two kinds.
func TestThingHookLeavesAlreadyNamedValuesAlone(t *testing.T) {
	store := newThingStore()
	store.SetPath(filepath.Join(t.TempDir(), "things.yaml"))
	if err := store.Record("location", "東京ドーム"); err != nil {
		t.Fatal(err)
	}

	// A route's `from`: a station, but it takes places and locations too.
	from := mywant.ParameterDef{
		Name:    "from",
		SubType: "station",
		Accepts: []string{"place", "location_coordinate", "address", "location"},
	}
	if got := store.KnownAs(thingSubtypesFor(from), "東京ドーム"); got != "location" {
		t.Errorf("KnownAs = %q, want %q — a value already named is already named", got, "location")
	}

	// Something nobody has named is still recorded, under the field's own
	// SubType: that is what a value typed into it IS.
	if got := store.KnownAs(thingSubtypesFor(from), "新横浜"); got != "" {
		t.Errorf("KnownAs = %q for a value nothing has recorded, want \"\"", got)
	}
}

// The parameter's own SubType answers first when the value is recorded under
// both, so a caller asking "what is this already" gets the field's own word
// for it rather than a synonym.
func TestThingSubtypesForOrder(t *testing.T) {
	pd := mywant.ParameterDef{SubType: "station", Accepts: []string{"location"}}
	got := thingSubtypesFor(pd)
	if len(got) == 0 || got[0] != "station" {
		t.Fatalf("thingSubtypesFor = %v, want it to start with the parameter's own subType", got)
	}
	var sawLocation bool
	for _, s := range got {
		if s == "location" {
			sawLocation = true
		}
	}
	if !sawLocation {
		t.Errorf("thingSubtypesFor = %v, want it to include what the parameter accepts", got)
	}
}

// A parameter that takes several places names each of them. Reading only the
// string form left every entry of a list unremembered.
func TestStringValuesReadsListsAndStrings(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want []string
	}{
		{"one string", "新宿駅", []string{"新宿駅"}},
		{"a list", []any{"新宿駅", "渋谷駅"}, []string{"新宿駅", "渋谷駅"}},
		{"typed list", []string{"新宿駅"}, []string{"新宿駅"}},
		{"blanks dropped", []any{"新宿駅", "  ", ""}, []string{"新宿駅"}},
		{"not names", []any{1, true, map[string]any{}}, []string{}},
		{"neither", 42, []string{}},
	}
	for _, c := range cases {
		got := stringValues(c.in)
		if len(got) != len(c.want) {
			t.Errorf("%s: stringValues(%v) = %q, want %q", c.name, c.in, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%s: stringValues(%v) = %q, want %q", c.name, c.in, got, c.want)
				break
			}
		}
	}
}
