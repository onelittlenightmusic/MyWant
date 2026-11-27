package mywant

import "testing"

func TestTimingsAtReadsAStateThatIsNotAnAnswer(t *testing.T) {
	defs := []TimingDef{
		{Name: "different", If: "errand_different"},
		{Name: "uneasy", If: "mood_uneasy", OnChange: "mood"},
		{Name: "more", OnIncrease: "count"},
	}
	seen := map[string]any{}
	state := map[string]any{"mood": "worried", "mood_uneasy": true, "count": 1}
	read := func(f string) any { return state[f] }

	// The first reading is where things stand: nothing changed yet.
	if got := TimingsAt(defs, seen, read); len(got) != 0 {
		t.Fatalf("first reading told %v", got)
	}
	// The same again: still nothing.
	if got := TimingsAt(defs, seen, read); len(got) != 0 {
		t.Fatalf("unchanged told %v", got)
	}
	// Back from an errand that found other than expected: told, each time.
	state["errand_different"] = true
	if got := TimingsAt(defs, seen, read); len(got) != 1 || got[0] != "different" {
		t.Fatalf("errand %v", got)
	}
	state["errand_different"] = false
	// Calm, then worried again: uneasy once, on the change.
	state["mood"], state["mood_uneasy"] = "calm", false
	TimingsAt(defs, seen, read)
	state["mood"], state["mood_uneasy"], state["count"] = "sad", true, 3
	got := TimingsAt(defs, seen, read)
	if len(got) != 2 || got[0] != "uneasy" || got[1] != "more" {
		t.Fatalf("change %v", got)
	}
}
