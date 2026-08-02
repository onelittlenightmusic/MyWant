package server

import (
	"testing"

	"mywant/engine/bundled"
	mywant "mywant/engine/core"
)

// TestKataSeedsLoad guards the bundled kata YAML: every kata must belong to a
// declared belt, carry at least one 所作, and only reference kata that exist.
func TestKataSeedsLoad(t *testing.T) {
	if err := mywant.LoadKataConfigsFromFS(bundled.BuiltinFS, "kata"); err != nil {
		t.Fatalf("load kata seeds: %v", err)
	}

	levels := mywant.ListKataLevels()
	kata := mywant.ListKata()
	if len(levels) == 0 || len(kata) == 0 {
		t.Fatalf("no kata definitions loaded (levels=%d kata=%d)", len(levels), len(kata))
	}

	known := make(map[string]bool, len(kata))
	for _, k := range kata {
		known[k.ID] = true
	}
	levelIDs := make(map[string]bool, len(levels))
	for _, lv := range levels {
		levelIDs[lv.ID] = true
		if lv.Promote.RequiredKata > len(lv.Kata) {
			t.Errorf("level %s requires %d kata but only lists %d",
				lv.ID, lv.Promote.RequiredKata, len(lv.Kata))
		}
		for _, id := range lv.Kata {
			if !known[id] {
				t.Errorf("level %s lists unknown kata %q", lv.ID, id)
			}
		}
	}

	for _, k := range kata {
		if !levelIDs[k.Level] {
			t.Errorf("kata %s belongs to unknown level %q", k.ID, k.Level)
		}
		for _, v := range k.Variations() {
			if len(v.Waza) == 0 {
				t.Errorf("kata %s variation %q has no waza", k.ID, v.ID)
			}
			for _, wz := range v.Waza {
				// A join only means something inside a kata that declares one.
				if wz.Join != "" && k.Join.Kind == "" {
					t.Errorf("kata %s joins on %q but declares no join", k.ID, wz.Join)
				}
				switch wz.Kind {
				case "want_type", "memo":
				case "repeat":
					if !known[wz.Kata] {
						t.Errorf("kata %s repeats unknown kata %q", k.ID, wz.Kata)
					}
				default:
					t.Errorf("kata %s has unknown waza kind %q", k.ID, wz.Kind)
				}
			}
		}
		for _, id := range k.Contains {
			if !known[id] {
				t.Errorf("kata %s contains unknown kata %q", k.ID, id)
			}
		}
	}
}

// TestKataRankFor pins the mastery ladder.
func TestKataRankFor(t *testing.T) {
	k := mywant.Kata{Mastery: mywant.MasteryThresholds{Shoden: 1, Kaiden: 3}}
	cases := []struct {
		n    int
		want string
	}{
		{0, mywant.MasteryNone},
		{1, mywant.MasteryShoden},
		{2, mywant.MasteryShoden},
		{3, mywant.MasteryKaiden},
		{9, mywant.MasteryKaiden},
	}
	for _, c := range cases {
		if got := k.RankFor(c.n); got != c.want {
			t.Errorf("RankFor(%d) = %q, want %q", c.n, got, c.want)
		}
	}
}

// TestSessionKeyStable ensures dedup keys ignore witness ordering, so the same
// practice is never credited twice just because evaluation saw wants in a
// different order.
func TestSessionKeyStable(t *testing.T) {
	a := mywant.SessionKeyFor([]string{"w1", "w2", "w3"})
	b := mywant.SessionKeyFor([]string{"w3", "w1", "w2"})
	if a != b {
		t.Errorf("session key not order-independent: %q vs %q", a, b)
	}
	if c := mywant.SessionKeyFor([]string{"w1", "w2"}); c == a {
		t.Errorf("different witness sets produced the same key %q", c)
	}
}
