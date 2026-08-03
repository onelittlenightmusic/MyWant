package server

import (
	"testing"

	mywant "mywant/engine/core"
)

func wazaWant(satisfied bool, ids ...string) WazaProgress {
	return WazaProgress{
		Waza:       mywant.Waza{Kind: "want_type", Type: "transit_search"},
		Satisfied:  satisfied,
		MatchedIDs: ids,
	}
}

func wazaMemo(subtype string, values ...string) WazaProgress {
	return WazaProgress{
		Waza:       mywant.Waza{Kind: "memo", Subtype: subtype},
		Satisfied:  true,
		MatchedIDs: values,
	}
}

func TestLiveEvidenceSplitsWantsFromMemo(t *testing.T) {
	wants, memo := liveEvidence([]WazaProgress{
		wazaMemo("station", "国分寺"),
		wazaWant(true, "want-b", "want-a"),
	})

	if len(wants) != 2 || wants[0] != "want-a" || wants[1] != "want-b" {
		t.Errorf("wants = %v, want them sorted", wants)
	}
	// Memo values are prefixed with the catalog key so they match the member ids
	// constellations and canvas tiles are made of.
	if len(memo) != 1 || memo[0] != "stations::国分寺" {
		t.Errorf("memo = %v, want [stations::国分寺]", memo)
	}
}

// An unsatisfied 所作 witnesses nothing, and a `repeat` 所作 is satisfied out of
// the record book — neither may end up on the canvas.
func TestLiveEvidenceIgnoresUnsatisfiedAndRepeat(t *testing.T) {
	wants, memo := liveEvidence([]WazaProgress{
		wazaWant(false, "want-unsatisfied"),
		{
			Waza:       mywant.Waza{Kind: "repeat", Kata: "kata-kasa", MinCount: 3},
			Satisfied:  true,
			MatchedIDs: []string{"kata-kasa:3"},
		},
	})

	if len(wants) != 0 || len(memo) != 0 {
		t.Errorf("evidence = %v / %v, want nothing drawable", wants, memo)
	}
}

func TestMemoCatalogKey(t *testing.T) {
	if got := memoCatalogKey("station"); got != "stations" {
		t.Errorf("station → %q, want stations", got)
	}
	if got := memoCatalogKey("city"); got != "cities" {
		t.Errorf("city → %q, want cities (the catalog key, not a naive plural)", got)
	}
	if got := memoCatalogKey("madeup"); got != "madeups" {
		t.Errorf("unknown type → %q, want the naive plural", got)
	}
}

// The chain is what keeps four stars from turning into a blot: one link per
// neighbour, memo values first, never a complete graph.
func TestKataEdgesChainsMembers(t *testing.T) {
	s := &Server{}
	edges := s.kataEdges([]string{"cities::Kokubunji", "stations::国分寺"}, []string{"want-a", "want-b"})

	if len(edges) != 3 {
		t.Fatalf("edges = %d, want 3 for four members", len(edges))
	}
	expect := [][2]string{
		{"cities::Kokubunji", "stations::国分寺"},
		{"stations::国分寺", "want-a"},
		{"want-a", "want-b"},
	}
	for i, e := range edges {
		if e.From != expect[i][0] || e.To != expect[i][1] {
			t.Errorf("edge %d = %s→%s, want %s→%s", i, e.From, e.To, expect[i][0], expect[i][1])
		}
		if e.Kind != "waza" {
			t.Errorf("edge %d kind = %q, want waza (no relation exists between these)", i, e.Kind)
		}
	}
}

func TestKataEdgesNeedsTwoMembers(t *testing.T) {
	s := &Server{}
	if edges := s.kataEdges(nil, []string{"want-a"}); edges != nil {
		t.Errorf("edges = %v, want none for a single member", edges)
	}
}

// The fingerprint is what puts a kata catching fire into the want collection's
// ETag; it must not depend on map iteration order.
func TestKataLabelFingerprintStable(t *testing.T) {
	a := map[string]map[string]string{
		"want-a": {"kata/kata-kasa": "国分寺", "kata/kata-ate": "国分寺"},
		"want-b": {"kata/kata-sora": "国分寺"},
	}
	b := map[string]map[string]string{
		"want-b": {"kata/kata-sora": "国分寺"},
		"want-a": {"kata/kata-ate": "国分寺", "kata/kata-kasa": "国分寺"},
	}
	if kataLabelFingerprint(a) != kataLabelFingerprint(b) {
		t.Error("fingerprint depends on iteration order")
	}
	if kataLabelFingerprint(nil) != "" {
		t.Error("no labels should fingerprint to the empty string, leaving the ETag untouched")
	}

	c := map[string]map[string]string{"want-a": {"kata/kata-kasa": "中野"}}
	if kataLabelFingerprint(a) == kataLabelFingerprint(c) {
		t.Error("a different constellation must change the fingerprint")
	}
}
