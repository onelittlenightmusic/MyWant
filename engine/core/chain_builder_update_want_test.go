package mywant

import (
	"path/filepath"
	"sync"
	"testing"
)

// newUpdateWantTestBuilder wires up a ChainBuilder with one want present in
// both cb.config and cb.wants under the same ID — the shape UpdateWant
// expects to find (an existing config want being edited).
//
// The runtime want is its own object, cloned from w rather than literally w
// itself — matching what addWant actually does at deploy time (see its own
// clone of wantConfig). Sharing the bare pointer here would test a shape
// that can't occur in production and would fail on a race nothing shipped
// causes: cb.config's entry and cb.wants' runtime want are two objects from
// the moment a want is created, never one.
func newUpdateWantTestBuilder(w *Want) *ChainBuilder {
	cb := NewChainBuilder([]*Want{w})
	runtime := &Want{
		Metadata: cloneMetadata(w.Metadata),
		Spec:     cloneSpec(w.Spec),
		Status:   w.Status,
	}
	cb.wants[w.Metadata.ID] = &runtimeWant{want: runtime}
	cb.wantNameToID[w.Metadata.Name] = w.Metadata.ID
	cb.inReconciliation = true // skip the reconcile-trigger send; not what this test is about
	return cb
}

// This is the exact aliasing the crash traced back to: UpdateWant used to
// assign `rw.want.Metadata = wantConfig.Metadata` by value, which only
// copies the struct header — Labels is a map, a reference type, so
// cb.config[i] and the runtime want ended up pointing at the very same map.
// A later SetLabel on the runtime want (properly locked under its own
// metadataMutex) then mutated a map that writeStatsToMemory reads straight
// off cb.config with no lock at all, by design — a classic concurrent
// map read/write, and what actually took the server down.
func TestUpdateWantDoesNotAliasLabelsWithConfig(t *testing.T) {
	original := &Want{
		Metadata: Metadata{ID: "want-1", Name: "w1", Type: "t"},
	}
	cb := newUpdateWantTestBuilder(original)

	updated := &Want{
		Metadata: Metadata{
			ID: "want-1", Name: "w1", Type: "t",
			Labels: map[string]string{"mywant.io/canvas-x": "5"},
		},
	}
	cb.UpdateWant(updated)

	rw := cb.wants["want-1"]
	rw.want.SetLabel("mywant.io/canvas-x", "9") // simulates a later footstep/drag write

	cfgWant := cb.config[0]
	if got := cfgWant.Metadata.Labels["mywant.io/canvas-x"]; got != "5" {
		t.Fatalf("expected cb.config's copy to stay at the value UpdateWant set (5), got %q — Labels is still aliased with the runtime want", got)
	}
	if got := rw.want.GetLabel("mywant.io/canvas-x"); got != "9" {
		t.Fatalf("expected the runtime want's own SetLabel to take, got %q", got)
	}
}

// Same shape, for Spec.Params and Spec.Imports — both maps, both assigned by
// value alongside Metadata in the same UpdateWant call.
func TestUpdateWantDoesNotAliasSpecMapsWithConfig(t *testing.T) {
	original := &Want{Metadata: Metadata{ID: "want-1", Name: "w1", Type: "t"}}
	cb := newUpdateWantTestBuilder(original)

	updated := &Want{
		Metadata: Metadata{ID: "want-1", Name: "w1", Type: "t"},
		Spec: WantSpec{
			Params:  map[string]any{"speed": 1.0},
			Imports: map[string]string{"g_key": "local_key"},
		},
	}
	cb.UpdateWant(updated)

	rw := cb.wants["want-1"]
	rw.want.metadataMutex.Lock()
	rw.want.Spec.Params["speed"] = 9.0
	rw.want.Spec.Imports["g_key"] = "changed"
	rw.want.metadataMutex.Unlock()

	cfgWant := cb.config[0]
	if got := cfgWant.Spec.Params["speed"]; got != 1.0 {
		t.Fatalf("expected cb.config's Params copy to stay at 1.0, got %v — Params is still aliased", got)
	}
	if got := cfgWant.Spec.Imports["g_key"]; got != "local_key" {
		t.Fatalf("expected cb.config's Imports copy to stay at %q, got %q — Imports is still aliased", "local_key", got)
	}
}

// Reproduces the crash's actual shape under the race detector (run with
// `go test -race`): UpdateWant + SetLabel racing writeStatsToMemory's
// marshal of cb.config, on separate goroutines, the same way an HTTP handler
// and the reconcile-loop's stats ticker do in production. Without the fix in
// UpdateWant and the wantsMu snapshot in writeStatsToMemory, `go test -race`
// flags this; without -race it can still panic outright, which is the
// original crash. A plain `go test` run (no -race) checks only that nothing
// panics; the aliasing itself is covered by the two tests above.
func TestWriteStatsToMemoryConcurrentWithUpdateWantDoesNotRace(t *testing.T) {
	w := &Want{Metadata: Metadata{ID: "want-1", Name: "w1", Type: "t"}}
	cb := newUpdateWantTestBuilder(w)
	cb.memoryPath = filepath.Join(t.TempDir(), "state.yaml")

	const iterations = 200
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			update := &Want{
				Metadata: Metadata{
					ID: "want-1", Name: "w1", Type: "t",
					Labels: map[string]string{"mywant.io/canvas-x": "5"},
				},
			}
			cb.UpdateWant(update)
			cb.wantsMu.RLock()
			rw, ok := cb.wants["want-1"]
			cb.wantsMu.RUnlock()
			if ok {
				rw.want.SetLabel("mywant.io/canvas-y", "3")
			}
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			bumpStateEpoch() // otherwise the epoch guard skips every write but the first
			cb.writeStatsToMemory()
		}
	}()

	wg.Wait()
}
