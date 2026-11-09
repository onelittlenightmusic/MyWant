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
	const rounds = 10
	for _, name := range []string{"state.yaml", "state.json"} {
		cb.memoryPath = filepath.Join(dir, name)
		cb.lastStatsHash = ""
		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)
		start := time.Now()
		for i := 0; i < rounds; i++ {
			bumpStateEpoch()
			cb.lastStatsHash = "" // written every round, as when something has changed
			cb.writeStatsToMemory()
		}
		elapsed := time.Since(start) / rounds
		runtime.ReadMemStats(&after)
		info, err := os.Stat(cb.memoryPath)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("%-10s %d wants  %8d bytes  %8.1f ms/write  %6.1f MB allocated/write",
			name, len(loaded), info.Size(), float64(elapsed.Microseconds())/1000,
			float64(after.TotalAlloc-before.TotalAlloc)/rounds/1e6)

		// And it reads back: the same wants.
		back, err := LoadWantsFromMemoryFile(cb.memoryPath)
		if err != nil || len(back) != len(loaded) {
			t.Fatalf("%s read back: %v, %d wants", name, err, len(back))
		}
		start = time.Now()
		_, _ = LoadWantsFromMemoryFile(cb.memoryPath)
		t.Logf("%-10s read back in %.1f ms", name, float64(time.Since(start).Microseconds())/1000)
	}
}
