package mywant

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A write that takes each want's bytes from the cache writes what a write
// made whole would — whatever path changed a want: each of them moves what the
// cache is checked against.
func TestMemoryCacheWritesWhatAWholeWriteWould(t *testing.T) {
	for _, name := range []string{"state.yaml", "state.json"} {
		t.Run(name, func(t *testing.T) {
			a := &Want{Metadata: Metadata{ID: "want-a", Name: "a", Type: "t"}}
			b := &Want{Metadata: Metadata{ID: "want-b", Name: "b", Type: "t"}}
			cb := newUpdateWantTestBuilder(a)
			cb.config = append(cb.config, b)
			rb := &Want{Metadata: cloneMetadata(b.Metadata), Spec: cloneSpec(b.Spec)}
			cb.wants[b.Metadata.ID] = &runtimeWant{want: rb}
			cb.wantNameToID[b.Metadata.Name] = b.Metadata.ID
			ra := cb.wants[a.Metadata.ID].want
			cb.memoryPath = filepath.Join(t.TempDir(), name)

			write := func() string {
				bumpStateEpoch()
				cb.lastStatsHash = ""
				cb.writeStatsToMemory()
				data, err := os.ReadFile(cb.memoryPath)
				if err != nil {
					t.Fatal(err)
				}
				return string(data)
			}
			whole := func() string {
				cb.memCache.mu.Lock()
				cb.memCache.rebuiltAt = time.Time{} // the cache not trusted this once
				cb.memCache.mu.Unlock()
				return write()
			}

			ra.StoreState("answers", []any{"x"})
			rb.StoreState("note", "first")
			write() // the cache filled

			for _, change := range []struct {
				name string
				do   func()
			}{
				{"StoreState", func() { ra.StoreState("answers", []any{"x", "y"}) }},
				{"StoreStateMulti (a webhook)", func() { StoreStateMulti(rb, map[string]any{"webhook_payload": map[string]any{"lat": 35.7}}) }},
				{"MergeState", func() { ra.MergeState(map[string]any{"routes": map[string]any{"fast": 30}}) }},
				{"AppendState", func() { rb.AppendState("events", "step") }},
				{"DrainState", func() { rb.DrainState("events") }},
				{"DeleteState", func() { rb.DeleteState("note") }},
				{"SetStatus", func() { ra.SetStatus("achieved") }},
				{"SetLabel", func() { rb.SetLabel("mywant.io/canvas-x", "4") }},
				{"UpdateParameter", func() { ra.UpdateParameter("to", "銀座") }},
				{"an answer recorded", func() {
					ra.getHistoryManager().AddResultEntry(ResultHistoryEntry{ID: "r1", Timestamp: time.Now(), Result: "1時間"})
				}},
			} {
				change.do()
				got := write()
				if want := whole(); got != want {
					t.Errorf("after %s the cached write differs from a whole one:\n%s\n---\n%s", change.name, got, want)
				}
			}
		})
	}
}
