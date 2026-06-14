package server

import (
	_ "embed"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"gopkg.in/yaml.v3"
)

//go:embed subtypes.yaml
var subtypesYAML []byte

// subtypeDefs is loaded once at init from the embedded subtypes.yaml.
var subtypeDefs map[string]SubtypeInfo

func init() {
	subtypeDefs = make(map[string]SubtypeInfo)
	_ = yaml.Unmarshal(subtypesYAML, &subtypeDefs)
}

// MemoStore persists user-entered values to ~/.mywant/memo.yaml, grouped by subtype.
// Thread-safe; reads and writes are serialised via a mutex.
type MemoStore struct {
	path string
	mu   sync.Mutex
}

// memoData is the on-disk YAML schema.
// Each subtype key maps to a deduplicated list of recorded values.
type memoData map[string][]string

func newMemoStore() *MemoStore {
	home, _ := os.UserHomeDir()
	return &MemoStore{path: filepath.Join(home, ".mywant", "memo.yaml")}
}

func (m *MemoStore) load() (memoData, error) {
	data := make(memoData)
	bytes, err := os.ReadFile(m.path)
	if os.IsNotExist(err) {
		return data, nil
	}
	if err != nil {
		return data, err
	}
	_ = yaml.Unmarshal(bytes, &data)
	return data, nil
}

func (m *MemoStore) save(data memoData) error {
	if err := os.MkdirAll(filepath.Dir(m.path), 0o755); err != nil {
		return err
	}
	bytes, err := yaml.Marshal(data)
	if err != nil {
		return err
	}
	return os.WriteFile(m.path, bytes, 0o644)
}

// Record adds value to the list for subtype, deduplicating and capping at 100 entries.
func (m *MemoStore) Record(subtype, value string) error {
	if subtype == "" || value == "" {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := m.load()
	if err != nil {
		return err
	}

	key := subtypeToKey(subtype)
	existing := data[key]
	// Deduplicate: remove old occurrence, prepend new value.
	filtered := make([]string, 0, len(existing))
	for _, v := range existing {
		if v != value {
			filtered = append(filtered, v)
		}
	}
	updated := append([]string{value}, filtered...)
	if len(updated) > 100 {
		updated = updated[:100]
	}
	data[key] = updated
	return m.save(data)
}

// Suggestions returns up to limit recorded values for subtype, most-recent first.
func (m *MemoStore) Suggestions(subtype string, limit int) []string {
	if subtype == "" {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := m.load()
	if err != nil {
		return nil
	}
	vals := data[subtypeToKey(subtype)]
	if limit > 0 && len(vals) > limit {
		vals = vals[:limit]
	}
	return vals
}

// Replace overwrites the entire memo with the provided data.
func (m *MemoStore) Replace(data memoData) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.save(data)
}

// All returns the full memo data as-is from disk.
func (m *MemoStore) All() memoData {
	m.mu.Lock()
	defer m.mu.Unlock()
	data, _ := m.load()
	return data
}

// AllSubtypes returns all known subtype keys sorted alphabetically.
func (m *MemoStore) AllSubtypes() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	data, _ := m.load()
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// SubtypeInfo holds display metadata for a parameter subtype.
type SubtypeInfo struct {
	Key  string `json:"key"`  // memo.yaml section key (e.g. "cities")
	Icon string `json:"icon"` // Lucide icon component name (e.g. "Building2")
}

// SubtypeDefinitions returns a copy of all known subtype definitions.
func SubtypeDefinitions() map[string]SubtypeInfo {
	out := make(map[string]SubtypeInfo, len(subtypeDefs))
	for k, v := range subtypeDefs {
		out[k] = v
	}
	return out
}

func subtypeToKey(subtype string) string {
	if info, ok := subtypeDefs[subtype]; ok {
		return info.Key
	}
	// Default: append "s"
	return subtype + "s"
}
