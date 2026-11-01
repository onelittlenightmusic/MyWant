package mywant

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// state.yaml is written one want at a time (writeStatsToMemory), each as a
// one-item sequence; end to end they must be the list marshalled whole, so
// the file reads back as it always has.
func TestWriteStatsToMemoryStreamsTheSameList(t *testing.T) {
	a := &Want{Metadata: Metadata{ID: "want-a", Name: "a", Type: "t"}}
	b := &Want{Metadata: Metadata{ID: "want-b", Name: "b", Type: "t"}}
	cb := newUpdateWantTestBuilder(a)
	cb.config = append(cb.config, b)
	a.StoreState("answers", []any{map[string]any{"room": "r1", "goto_values": map[string]any{"v": "x"}}})
	b.StoreState("note", "line one\nline two")
	cb.memoryPath = filepath.Join(t.TempDir(), "state.yaml")

	bumpStateEpoch()
	cb.writeStatsToMemory()

	got, err := os.ReadFile(cb.memoryPath)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := yaml.Marshal([]*Want{a, b})
	if string(got) != string(want) {
		t.Fatalf("streamed state.yaml differs from the whole list marshalled:\n%s\n---\n%s", got, want)
	}
	var back []*Want
	if err := yaml.Unmarshal(got, &back); err != nil || len(back) != 2 {
		t.Fatalf("read back: %v, %d wants", err, len(back))
	}
	if _, err := os.Stat(cb.memoryPath + ".tmp"); !os.IsNotExist(err) {
		t.Fatal("the file written beside it was left behind")
	}
}
