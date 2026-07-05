package mywant

import (
	"crypto/md5"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"
)

// WebElementMark records one character's mark on a DOM element found by the
// web inspector overlay, attributed by CharacterID/Color (resolved server-side
// from the character record, never trusted from the client) so the same
// hostname can accumulate marks from multiple characters across sessions.
type WebElementMark struct {
	Role        string `yaml:"role"                json:"role"`
	Name        string `yaml:"name"                json:"name"`
	Selector    string `yaml:"selector"             json:"selector"`
	FieldKey    string `yaml:"fieldKey,omitempty"   json:"fieldKey,omitempty"`
	HtmlName    string `yaml:"htmlName,omitempty"   json:"htmlName,omitempty"`
	CharacterID string `yaml:"characterId"          json:"characterId"`
	Color       string `yaml:"color"                json:"color"`
}

// webMarkStoreFile is the root document persisted to ~/.mywant/web_marks.yaml.
type webMarkStoreFile struct {
	Marks map[string][]WebElementMark `yaml:"marks"` // hostname -> marks, all characters merged
}

var (
	globalWebMarkStore     *webMarkManager
	globalWebMarkStoreOnce sync.Once
)

type webMarkManager struct {
	mu       sync.RWMutex
	path     string
	lastHash string
	store    webMarkStoreFile
}

// GetWebMarkManager returns the singleton web-mark manager.
func GetWebMarkManager() *webMarkManager {
	globalWebMarkStoreOnce.Do(func() {
		home, err := os.UserHomeDir()
		if err != nil {
			log.Printf("[WARN] web_mark_store: cannot determine home dir: %v", err)
			home = "."
		}
		path := filepath.Join(home, ".mywant", "web_marks.yaml")
		m := &webMarkManager{path: path}
		m.load()
		globalWebMarkStore = m
	})
	return globalWebMarkStore
}

func (m *webMarkManager) load() {
	data, err := os.ReadFile(m.path)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("[WARN] web_mark_store: failed to read %s: %v", m.path, err)
		}
		return
	}
	var s webMarkStoreFile
	if err := yaml.Unmarshal(data, &s); err != nil {
		log.Printf("[WARN] web_mark_store: failed to unmarshal %s: %v", m.path, err)
		return
	}
	m.store = s
	m.lastHash = fmt.Sprintf("%x", md5.Sum(data))
	log.Printf("[WebMarkStore] Loaded marks for %d hostname(s) from %s", len(s.Marks), m.path)
}

func (m *webMarkManager) save() {
	data, err := yaml.Marshal(&m.store)
	if err != nil {
		log.Printf("[WARN] web_mark_store: marshal failed: %v", err)
		return
	}
	newHash := fmt.Sprintf("%x", md5.Sum(data))
	if newHash == m.lastHash {
		return
	}
	if err := os.MkdirAll(filepath.Dir(m.path), 0755); err != nil {
		log.Printf("[WARN] web_mark_store: mkdir failed: %v", err)
		return
	}
	if err := os.WriteFile(m.path, data, 0644); err != nil {
		log.Printf("[WARN] web_mark_store: write failed: %v", err)
		return
	}
	m.lastHash = newHash
}

// Get returns every mark (from every character) recorded for the given hostname.
func (m *webMarkManager) Get(hostname string) []WebElementMark {
	m.mu.RLock()
	defer m.mu.RUnlock()
	src := m.store.Marks[hostname]
	out := make([]WebElementMark, len(src))
	copy(out, src)
	return out
}

// Add upserts marks for a hostname, keyed by (Selector, CharacterID) — a
// character re-marking the same element on a later visit replaces its prior
// mark there instead of accumulating duplicates.
func (m *webMarkManager) Add(hostname, characterID, color string, marks []WebElementMark) {
	if hostname == "" || characterID == "" || len(marks) == 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.store.Marks == nil {
		m.store.Marks = map[string][]WebElementMark{}
	}
	existing := m.store.Marks[hostname]
	for _, mark := range marks {
		mark.CharacterID = characterID
		mark.Color = color
		replaced := false
		for i, e := range existing {
			if e.Selector == mark.Selector && e.CharacterID == characterID {
				existing[i] = mark
				replaced = true
				break
			}
		}
		if !replaced {
			existing = append(existing, mark)
		}
	}
	m.store.Marks[hostname] = existing
	m.save()
	log.Printf("[WebMarkStore] Recorded %d mark(s) for %s from character %s", len(marks), hostname, characterID)
}

// Package-level functions
func GetWebMarks(hostname string) []WebElementMark { return GetWebMarkManager().Get(hostname) }
func AddWebMarks(hostname, characterID, color string, marks []WebElementMark) {
	GetWebMarkManager().Add(hostname, characterID, color, marks)
}
