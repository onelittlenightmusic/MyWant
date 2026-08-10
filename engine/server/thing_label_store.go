package server

import (
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"
)

// ThingLabelStore gives memo values the metadata (labels) that memo.yaml itself
// can't hold — memo.yaml is just catalogKey → []value, with no per-value slot.
// Labels are keyed by a memo value id ("catalogKey::value") and are a plain
// key→value string map, mirroring a want's metadata.labels. Groups ride on top
// of this via the reserved "group/<name>"="true" convention (see the /groups
// facade in handlers_groups.go). Thread-safe; persisted to ~/.mywant/memo-labels.yaml.
type ThingLabelStore struct {
	path string
	mu   sync.Mutex
}

// thingLabelData is the on-disk YAML schema: value id → labels.
type thingLabelData map[string]map[string]string

func newThingLabelStore() *ThingLabelStore {
	return &ThingLabelStore{path: thingPath("thing-labels.yaml")}
}

func (m *ThingLabelStore) load() (thingLabelData, error) {
	data := make(thingLabelData)
	bytes, err := os.ReadFile(m.path)
	if os.IsNotExist(err) {
		return data, nil
	}
	if err != nil {
		return data, err
	}
	_ = yaml.Unmarshal(bytes, &data)
	if data == nil {
		data = make(thingLabelData)
	}
	return data, nil
}

func (m *ThingLabelStore) save(data thingLabelData) error {
	if err := os.MkdirAll(filepath.Dir(m.path), 0o755); err != nil {
		return err
	}
	bytes, err := yaml.Marshal(data)
	if err != nil {
		return err
	}
	return os.WriteFile(m.path, bytes, 0o644)
}

// All returns a deep copy of every value's labels.
// Rekey moves every label from one id to another, in one write.
//
// Used by the identity migration: labels used to hang off "catalog::value" and
// now hang off the thing's own id, and doing that a label at a time through
// Set/Remove would leave the file half-moved if anything failed in between.
func (m *ThingLabelStore) Rekey(mapping map[string]string) error {
	if len(mapping) == 0 {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	data, err := m.load()
	if err != nil {
		return err
	}
	changed := false
	for from, to := range mapping {
		labels, ok := data[from]
		if !ok || from == to {
			continue
		}
		// Merge rather than overwrite: whatever is already under the new id was
		// put there deliberately and outranks a carried-over copy.
		if existing, ok := data[to]; ok {
			for k, v := range labels {
				if _, taken := existing[k]; !taken {
					existing[k] = v
				}
			}
		} else {
			data[to] = labels
		}
		delete(data, from)
		changed = true
	}
	if !changed {
		return nil
	}
	return m.save(data)
}

func (m *ThingLabelStore) All() map[string]map[string]string {
	m.mu.Lock()
	defer m.mu.Unlock()
	data, _ := m.load()
	out := make(map[string]map[string]string, len(data))
	for id, labels := range data {
		cp := make(map[string]string, len(labels))
		for k, v := range labels {
			cp[k] = v
		}
		out[id] = cp
	}
	return out
}

// Get returns a value's labels (copy), or an empty map.
func (m *ThingLabelStore) Get(valueID string) map[string]string {
	m.mu.Lock()
	defer m.mu.Unlock()
	data, _ := m.load()
	out := make(map[string]string)
	for k, v := range data[valueID] {
		out[k] = v
	}
	return out
}

// Set adds/updates one label on a value.
func (m *ThingLabelStore) Set(valueID, key, value string) error {
	if valueID == "" || key == "" {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	data, err := m.load()
	if err != nil {
		return err
	}
	if data[valueID] == nil {
		data[valueID] = make(map[string]string)
	}
	data[valueID][key] = value
	return m.save(data)
}

// Remove deletes one label from a value, pruning the value entry when empty.
func (m *ThingLabelStore) Remove(valueID, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	data, err := m.load()
	if err != nil {
		return err
	}
	if labels, ok := data[valueID]; ok {
		delete(labels, key)
		if len(labels) == 0 {
			delete(data, valueID)
		}
	}
	return m.save(data)
}

// ValuesWithLabel returns the value ids carrying the given label key.
func (m *ThingLabelStore) ValuesWithLabel(key string) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	data, _ := m.load()
	var out []string
	for id, labels := range data {
		if _, ok := labels[key]; ok {
			out = append(out, id)
		}
	}
	return out
}
