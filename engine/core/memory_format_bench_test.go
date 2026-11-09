package mywant

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// What writing the memory file costs, YAML against JSON, on a real state:
// MYWANT_STATE_BENCH=~/.mywant/state.yaml go test ./core -run TestMemoryWriteCost -v
// Every want is loaded as a running want (its state, labels and answers), and
// writeStatsToMemory is run whole — the deep copies and the history included.
func TestMemoryWriteCost(t *testing.T) {
	src := os.Getenv("MYWANT_STATE_BENCH")
	if src == "" {
		t.Skip("MYWANT_STATE_BENCH not set")
	}
	loaded, err := LoadWantsFromMemoryFile(src)
	if err != nil {
		t.Fatal(err)
	}
	cb := NewChainBuilder(nil)
	cb.inReconciliation = true
	for _, w := range loaded {
		w.RestoreResultHistory(w.ResultHistory)
		cb.config = append(cb.config, &Want{Metadata: cloneMetadata(w.Metadata), Spec: cloneSpec(w.Spec)})
		cb.wants[w.Metadata.ID] = &runtimeWant{want: w}
		cb.wantNameToID[w.Metadata.Name] = w.Metadata.ID
	}
	dir := t.TempDir()
	// The smallest want: what a location webhook changes every second.
	var small *Want
	smallest := -1
	for _, w := range loaded {
		if b, err := encodeMemoryWant(w, true); err == nil && (smallest < 0 || len(b) < smallest) {
			small, smallest = w, len(b)
		}
	}
	const rounds = 10
	measure := func(name, scenario string, before func(i int)) {
		var ms0, ms1 runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&ms0)
		var spent time.Duration
		for i := 0; i < rounds; i++ {
			before(i)
			bumpStateEpoch()
			cb.lastStatsHash = "" // written every round, as when something has changed
			start := time.Now()
			cb.writeStatsToMemory()
			spent += time.Since(start)
		}
		runtime.ReadMemStats(&ms1)
		info, err := os.Stat(cb.memoryPath)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("%-10s %-26s %8d bytes  %7.2f ms/write  %6.2f MB allocated/write",
			name, scenario, info.Size(), float64(spent.Microseconds())/1000/rounds,
			float64(ms1.TotalAlloc-ms0.TotalAlloc)/rounds/1e6)
	}
	for _, name := range []string{"state.yaml", "state.json"} {
		cb.memoryPath = filepath.Join(dir, name)
		noCache := func(int) { cb.memCache.finish(nil) }
		measure(name, "whole (no cache)", noCache)
		cb.memCache.finish(nil)
		bumpStateEpoch()
		cb.lastStatsHash = ""
		cb.writeStatsToMemory() // the cache filled
		measure(name, "cached, nothing changed", func(int) {})
		measure(name, "cached, one small changed", func(i int) { small.StoreState("_bench_tick", i) })

		// And it reads back: the same wants.
		start := time.Now()
		back, err := LoadWantsFromMemoryFile(cb.memoryPath)
		if err != nil || len(back) != len(loaded) {
			t.Fatalf("%s read back: %v, %d wants", name, err, len(back))
		}
		t.Logf("%-10s read back in %.1f ms", name, float64(time.Since(start).Microseconds())/1000)
	}
	t.Logf("the small want: %s (%d bytes)", small.Metadata.Name, smallest)
}
