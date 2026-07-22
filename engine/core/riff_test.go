package mywant

import (
	"strings"
	"testing"
)

func sampleRiffInput() RiffInput {
	return RiffInput{
		Named: []RiffNamedThing{
			{Kind: "location_coordinate", Name: "会社"},
			{Kind: "person", Name: "上司"},
		},
		Sources: []RiffSource{
			{Type: "weather", Name: "tokyo-weather"},
		},
		Reactions: []RiffReaction{
			{Type: "fireworks_effect", Title: "Fireworks"},
			{Type: "chime_effect", Title: "Chime"},
		},
	}
}

// A riff must read like the user's own world: a place trigger names the place,
// a person trigger names the person, and the reaction is a playful world act.
func TestGenerateRiffsPhrasing(t *testing.T) {
	got := GenerateRiffs(sampleRiffInput(), 5, 1)
	if len(got) == 0 {
		t.Fatal("expected proposals, got none")
	}
	for _, p := range got {
		if !strings.HasSuffix(p.Text, "。") {
			t.Errorf("proposal not a full sentence: %q", p.Text)
		}
	}
	// The place we named must appear, phrased as an arrival.
	joined := ""
	for _, p := range got {
		joined += p.Text + "\n"
	}
	if !strings.Contains(joined, "「会社」に着いたら") {
		t.Errorf("named place not phrased as arrival in:\n%s", joined)
	}
}

// Named-thing triggers are the personal hook, so they must fill the limited
// slots ahead of bare source triggers when both exist.
func TestGenerateRiffsPrefersNamedThings(t *testing.T) {
	got := GenerateRiffs(sampleRiffInput(), 2, 1)
	if len(got) != 2 {
		t.Fatalf("expected 2 proposals, got %d", len(got))
	}
	for _, p := range got {
		if p.TriggerKind != "named" {
			t.Errorf("source trigger took a slot from a named one: %+v", p)
		}
	}
}

// Determinism: the same seed yields the same proposals (reproducible surfacing
// and testable), a different seed may differ.
func TestGenerateRiffsDeterministic(t *testing.T) {
	a := GenerateRiffs(sampleRiffInput(), 3, 42)
	b := GenerateRiffs(sampleRiffInput(), 3, 42)
	if len(a) != len(b) {
		t.Fatalf("same seed gave different counts: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i].Text != b[i].Text {
			t.Errorf("same seed diverged at %d: %q vs %q", i, a[i].Text, b[i].Text)
		}
	}
}

// With nothing named and no source, or no toys, there is nothing to riff on —
// emptiness is the signal to go name/deploy more, not an error.
func TestGenerateRiffsEmptyWhenNoMaterial(t *testing.T) {
	if r := GenerateRiffs(RiffInput{Reactions: []RiffReaction{{Type: "fireworks_effect"}}}, 3, 1); r != nil {
		t.Errorf("no triggers should yield no proposals, got %v", r)
	}
	if r := GenerateRiffs(RiffInput{Named: []RiffNamedThing{{Kind: "place", Name: "家"}}}, 3, 1); r != nil {
		t.Errorf("no reactions should yield no proposals, got %v", r)
	}
}

// Show a realistic riff so a failing eyeball is one `go test -v` away — the fun
// is meant to be read, not just asserted.
func TestGenerateRiffsSample(t *testing.T) {
	for _, p := range GenerateRiffs(sampleRiffInput(), 5, 7) {
		t.Logf("riff: %s", p.Text)
	}
}
