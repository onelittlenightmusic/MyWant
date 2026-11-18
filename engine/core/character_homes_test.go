package mywant

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The ledger sits beside the characters: one entry per want, written to the
// file atomically, never in what the character API returns, never undone by a
// settings save, and gone with its character.
func TestCharacterAskLedger(t *testing.T) {
	path := filepath.Join(t.TempDir(), "characters.yaml")
	m := &characterManager{path: path, store: characterStoreFile{Characters: []Character{{ID: "chr-me", Name: "Me"}}}}
	ask := CharacterAsk{Want: "w1", Type: "transit_search", At: time.Date(2026, 11, 17, 21, 0, 0, 0, time.UTC),
		Places: []AskPlace{{Param: "to", Value: "中野坂上", Kind: "station"}}}

	if !m.RecordAsk("chr-me", ask) {
		t.Fatal("first ask not recorded")
	}
	if m.RecordAsk("chr-me", ask) {
		t.Error("the same want recorded twice")
	}
	if m.RecordAsk("chr-nobody", CharacterAsk{Want: "w2", Places: ask.Places}) {
		t.Error("an unknown character got a ledger")
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Error("temporary file left behind")
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "homes:") || !strings.Contains(string(data), "中野坂上") {
		t.Errorf("ledger not in the file:\n%s", data)
	}

	// Not in what the character API returns.
	js, _ := json.Marshal(m.List())
	if strings.Contains(string(js), "中野坂上") {
		t.Errorf("ledger leaked into the characters: %s", js)
	}
	// A settings save does not undo it.
	_, v0 := m.Asks("chr-me")
	m.Update("chr-me", Character{ID: "chr-me", Name: "Me again"})
	if asks, _ := m.Asks("chr-me"); len(asks) != 1 {
		t.Errorf("asks after a settings save = %d", len(asks))
	}
	// Gone with the character, and the version moves.
	m.Delete("chr-me")
	if asks, v := m.Asks("chr-me"); len(asks) != 0 || v == v0 {
		t.Errorf("after delete: %d asks, version %d → %d", len(asks), v0, v)
	}
}
