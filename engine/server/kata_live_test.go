package server

import (
	"strings"
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

// A constellation hangs off its memo values: they chain to each other, and each
// want spokes off the value it names. Two wants are never joined — they share a
// place, not a thread.
func TestKataEdgesSpokesFromMemo(t *testing.T) {
	s := &Server{}
	edges := s.kataEdges([]string{"cities::Kokubunji", "stations::国分寺"}, []string{"want-a", "want-b"})

	// No want is named by a param here (there is no builder), so both fall back
	// to the first memo value: one memo↔memo link plus one spoke per want.
	if len(edges) != 3 {
		t.Fatalf("edges = %d, want 3", len(edges))
	}
	if edges[0].From != "cities::Kokubunji" || edges[0].To != "stations::国分寺" {
		t.Errorf("first edge = %s→%s, want the memo chain", edges[0].From, edges[0].To)
	}
	for _, e := range edges[1:] {
		if e.From != "cities::Kokubunji" {
			t.Errorf("spoke starts at %s, want the memo hub", e.From)
		}
	}
	for _, e := range edges {
		if strings.HasPrefix(e.From, "want-") && strings.HasPrefix(e.To, "want-") {
			t.Errorf("want↔want edge %s→%s: wants share a place, not a thread", e.From, e.To)
		}
	}
}

// 糧 (a restaurant and a budget) declares no memo 所作, so there is no hub to
// hang from — without a fallback its wants would light separately and read as
// unrelated.
func TestKataEdgesChainsWantsWithoutMemo(t *testing.T) {
	s := &Server{}
	edges := s.kataEdges(nil, []string{"want-a", "want-b"})

	if len(edges) != 1 || edges[0].From != "want-a" || edges[0].To != "want-b" {
		t.Errorf("edges = %+v, want the two wants chained", edges)
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
