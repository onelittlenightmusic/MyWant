package labels

import (
	"path/filepath"
	"sync"
	"testing"
)

func TestMatches(t *testing.T) {
	l := map[string]string{"role": "a", "tier": "x"}
	cases := []struct {
		sel  map[string]string
		want bool
	}{
		{nil, true},
		{map[string]string{"role": "a"}, true},
		{map[string]string{"role": "b"}, false},
		{map[string]string{"missing": "v"}, false},
		// An empty value matches an absent key — the chain builder's rule.
		{map[string]string{"missing": ""}, true},
	}
	for _, c := range cases {
		if got := Matches(l, c.sel); got != c.want {
			t.Errorf("Matches(%v) = %v, want %v", c.sel, got, c.want)
		}
	}
}

func TestKeyNameOf(t *testing.T) {
	k := Key("constellation", "中央線")
	if k != "constellation/中央線" || NameOf("constellation", k) != "中央線" || NameOf("other", k) != "" {
		t.Fatalf("Key/NameOf round trip failed: %q", k)
	}
}

func TestWithWithoutCopy(t *testing.T) {
	orig := map[string]string{"a": "1"}
	w := With(orig, "b", "2")
	if len(orig) != 1 || w["b"] != "2" || w["a"] != "1" {
		t.Fatalf("With wrote through or lost keys: orig=%v w=%v", orig, w)
	}
	wo := Without(w, "a")
	if _, ok := wo["a"]; ok || w["a"] != "1" {
		t.Fatalf("Without wrote through: w=%v wo=%v", w, wo)
	}
}

func TestGuardedCopiesAndOnChange(t *testing.T) {
	var mu sync.RWMutex
	var m map[string]string // nil until the first write
	changes := 0
	g := Guard(&mu, &m, func() { changes++ })
	g.Set("a", "1")
	g.SetMany(map[string]string{"b": "2"})
	all := g.All()
	all["a"] = "changed"
	if v, _ := g.Get("a"); v != "1" {
		t.Fatalf("All handed out the live map")
	}
	g.Delete("a")
	g.Replace(map[string]string{"z": "9"})
	if _, ok := g.Get("b"); ok || !g.Matches(map[string]string{"z": "9"}) {
		t.Fatalf("Replace did not swap the map: %v", g.All())
	}
	if changes != 4 {
		t.Fatalf("onChange ran %d times, want 4", changes)
	}
}

// Run under -race: readers ranging over copies while writers change the map
// is exactly what used to abort the server.
func TestGuardedConcurrent(t *testing.T) {
	var mu sync.RWMutex
	m := map[string]string{}
	g := Guard(&mu, &m, nil)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				g.Set(string(rune('a'+i)), "v")
				g.Delete(string(rune('a' + i)))
			}
		}(i)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				for range g.All() {
				}
				g.Matches(map[string]string{"a": "v"})
			}
		}()
	}
	wg.Wait()
}

func TestFileStore(t *testing.T) {
	s := NewFileStore(filepath.Join(t.TempDir(), "labels.yaml"))
	if len(s.Get("x")) != 0 {
		t.Fatal("empty store returned labels")
	}
	_ = s.Set("x", "k", "v")
	_ = s.Set("x", "k2", "v2")
	_ = s.Set("y", "k", "w")
	got := s.Get("x")
	got["k"] = "changed"
	if s.Get("x")["k"] != "v" {
		t.Fatal("Get handed out stored map")
	}
	if ids := s.IDsWithLabel("k"); len(ids) != 2 {
		t.Fatalf("IDsWithLabel = %v", ids)
	}
	_ = s.Remove("y", "k")
	if _, ok := s.All()["y"]; ok {
		t.Fatal("an id with no labels left was kept")
	}
	_ = s.Set("z", "k", "old")
	_ = s.Set("z", "only-old", "1")
	_ = s.Rekey(map[string]string{"x": "z"})
	z := s.Get("z")
	if z["k"] != "old" || z["k2"] != "v2" || z["only-old"] != "1" {
		t.Fatalf("Rekey should keep the target's own keys and fill the rest: %v", z)
	}
	if len(s.Get("x")) != 0 {
		t.Fatal("Rekey left the old id behind")
	}
}
