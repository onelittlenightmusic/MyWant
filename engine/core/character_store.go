package mywant

import (
	"crypto/md5"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

// Character represents a named persona that can be assigned to one or more browser devices.
type Character struct {
	ID                string   `yaml:"id"                json:"id"`
	Name              string   `yaml:"name"              json:"name"`
	Avatar            string   `yaml:"avatar"            json:"avatar"`    // emoji e.g. "🧙"
	Color             string   `yaml:"color"             json:"color"`     // hex e.g. "#6366f1"
	CreatedAt         int64    `yaml:"createdAt"         json:"createdAt"` // Unix ms
	AssignedDeviceIDs []string `yaml:"assignedDeviceIds" json:"assignedDeviceIds"`
	// AuraDefaults maps a target want's ID to the aura-default mark this
	// character has set for it — shown as an aura-colored dog-ear flag/star on
	// the marked option/state in that want's card UI, toggled via the X key/button.
	AuraDefaults map[string]AuraMark `yaml:"auraDefaults,omitempty" json:"auraDefaults,omitempty"`
	// TileDesign / AuraDesign are the design-plugin ids this character picks for
	// the want tiles and aura they own on the canvas (e.g. "cubic", "forest").
	// Empty = inherit the canvas-level design (config.canvas_design).
	TileDesign string `yaml:"tileDesign,omitempty" json:"tile_design,omitempty"`
	AuraDesign string `yaml:"auraDesign,omitempty" json:"aura_design,omitempty"`
}

// AuraMark records one character's aura-default pick for a target want: which
// state section (current/goal/plan/internal) and key within it — or
// "parameter" for a spec parameter — the value applies to, plus the
// stringified value itself (JSON-stringified for object values, plain string
// otherwise — mirrors the frontend's getChoiceValue encoding). Section and key
// are captured at mark time by the card that owns the field, so applying a
// mark (see engine/types/aura_types.go) doesn't need to know about specific
// want types.
type AuraMark struct {
	Section string `yaml:"section" json:"section"`
	Key     string `yaml:"key"     json:"key"`
	Value   string `yaml:"value"   json:"value"`
}

// characterStoreFile is the root document persisted to ~/.mywant/characters.yaml.
type characterStoreFile struct {
	Characters []Character `yaml:"characters"`
}

var (
	globalCharacterStore     *characterManager
	globalCharacterStoreOnce sync.Once
)

type characterManager struct {
	mu       sync.RWMutex
	path     string
	lastHash string
	store    characterStoreFile
}

// GetCharacterManager returns the singleton character manager.
func GetCharacterManager() *characterManager {
	globalCharacterStoreOnce.Do(func() {
		home, err := os.UserHomeDir()
		if err != nil {
			log.Printf("[WARN] character_store: cannot determine home dir: %v", err)
			home = "."
		}
		path := filepath.Join(home, ".mywant", "characters.yaml")
		m := &characterManager{path: path}
		m.load()
		globalCharacterStore = m
	})
	return globalCharacterStore
}

func (m *characterManager) load() {
	data, err := os.ReadFile(m.path)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("[WARN] character_store: failed to read %s: %v", m.path, err)
		}
		return
	}
	var s characterStoreFile
	if err := yaml.Unmarshal(data, &s); err != nil {
		log.Printf("[WARN] character_store: failed to unmarshal %s: %v", m.path, err)
		return
	}
	for i := range s.Characters {
		if s.Characters[i].AssignedDeviceIDs == nil {
			s.Characters[i].AssignedDeviceIDs = []string{}
		}
	}
	m.store = s
	m.lastHash = fmt.Sprintf("%x", md5.Sum(data))
	log.Printf("[CharacterStore] Loaded %d characters from %s", len(s.Characters), m.path)
}

func (m *characterManager) save() {
	data, err := yaml.Marshal(&m.store)
	if err != nil {
		log.Printf("[WARN] character_store: marshal failed: %v", err)
		return
	}
	newHash := fmt.Sprintf("%x", md5.Sum(data))
	if newHash == m.lastHash {
		return
	}
	if err := os.MkdirAll(filepath.Dir(m.path), 0755); err != nil {
		log.Printf("[WARN] character_store: mkdir failed: %v", err)
		return
	}
	if err := os.WriteFile(m.path, data, 0644); err != nil {
		log.Printf("[WARN] character_store: write failed: %v", err)
		return
	}
	m.lastHash = newHash
}

func (m *characterManager) List() []Character {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Character, len(m.store.Characters))
	copy(out, m.store.Characters)
	return out
}

func (m *characterManager) Get(id string) (*Character, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, c := range m.store.Characters {
		if c.ID == id {
			cp := c
			return &cp, true
		}
	}
	return nil, false
}

func (m *characterManager) Add(c Character) Character {
	m.mu.Lock()
	defer m.mu.Unlock()
	if c.ID == "" {
		c.ID = "chr-" + uuid.New().String()[:8]
	}
	if c.CreatedAt == 0 {
		c.CreatedAt = time.Now().UnixMilli()
	}
	if c.AssignedDeviceIDs == nil {
		c.AssignedDeviceIDs = []string{}
	}
	m.store.Characters = append(m.store.Characters, c)
	m.save()
	log.Printf("[CharacterStore] Added character %s (%s)", c.ID, c.Name)
	return c
}

func (m *characterManager) Update(id string, updated Character) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, c := range m.store.Characters {
		if c.ID == id {
			updated.ID = id
			updated.CreatedAt = c.CreatedAt
			updated.AssignedDeviceIDs = c.AssignedDeviceIDs // preserve assignments
			if updated.AssignedDeviceIDs == nil {
				updated.AssignedDeviceIDs = []string{}
			}
			updated.AuraDefaults = c.AuraDefaults // preserve aura-default marks
			updated.TileDesign = c.TileDesign     // preserve design picks (set via /design)
			updated.AuraDesign = c.AuraDesign
			m.store.Characters[i] = updated
			m.save()
			return true
		}
	}
	return false
}

func (m *characterManager) Delete(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, c := range m.store.Characters {
		if c.ID == id {
			m.store.Characters = append(m.store.Characters[:i], m.store.Characters[i+1:]...)
			m.save()
			log.Printf("[CharacterStore] Deleted character %s", c.Name)
			return true
		}
	}
	return false
}

// AssignDevices sets the assignedDeviceIds for the given character, atomically removing
// each device ID from any other character it was previously assigned to.
func (m *characterManager) AssignDevices(characterID string, deviceIDs []string) (*Character, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Ensure target exists.
	targetIdx := -1
	for i, c := range m.store.Characters {
		if c.ID == characterID {
			targetIdx = i
			break
		}
	}
	if targetIdx < 0 {
		return nil, false
	}

	incoming := make(map[string]bool, len(deviceIDs))
	for _, d := range deviceIDs {
		incoming[d] = true
	}

	// Remove these device IDs from every other character.
	for i := range m.store.Characters {
		if m.store.Characters[i].ID == characterID {
			continue
		}
		filtered := m.store.Characters[i].AssignedDeviceIDs[:0]
		for _, d := range m.store.Characters[i].AssignedDeviceIDs {
			if !incoming[d] {
				filtered = append(filtered, d)
			}
		}
		m.store.Characters[i].AssignedDeviceIDs = filtered
		if m.store.Characters[i].AssignedDeviceIDs == nil {
			m.store.Characters[i].AssignedDeviceIDs = []string{}
		}
	}

	// Set the new assignment.
	safe := make([]string, len(deviceIDs))
	copy(safe, deviceIDs)
	m.store.Characters[targetIdx].AssignedDeviceIDs = safe
	m.save()

	cp := m.store.Characters[targetIdx]
	log.Printf("[CharacterStore] Assigned %d device(s) to character %s", len(deviceIDs), cp.Name)
	return &cp, true
}

// SetAuraDefault marks the given want's aura-default selection for a
// character, or clears it when mark.Value is "".
func (m *characterManager) SetAuraDefault(characterID, wantID string, mark AuraMark) (*Character, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, c := range m.store.Characters {
		if c.ID != characterID {
			continue
		}
		if mark.Value == "" {
			delete(m.store.Characters[i].AuraDefaults, wantID)
		} else {
			if m.store.Characters[i].AuraDefaults == nil {
				m.store.Characters[i].AuraDefaults = map[string]AuraMark{}
			}
			m.store.Characters[i].AuraDefaults[wantID] = mark
		}
		m.save()
		cp := m.store.Characters[i]
		return &cp, true
	}
	return nil, false
}

// SetDesign sets the tile/aura design-plugin ids for a character (empty string
// = inherit the canvas-level design).
func (m *characterManager) SetDesign(characterID, tileDesign, auraDesign string) (*Character, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, c := range m.store.Characters {
		if c.ID != characterID {
			continue
		}
		m.store.Characters[i].TileDesign = tileDesign
		m.store.Characters[i].AuraDesign = auraDesign
		m.save()
		cp := m.store.Characters[i]
		return &cp, true
	}
	return nil, false
}

// PruneDevices removes the given device IDs from all characters (called when devices go offline).
func (m *characterManager) PruneDevices(deviceIDs []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	stale := make(map[string]bool, len(deviceIDs))
	for _, d := range deviceIDs {
		stale[d] = true
	}
	changed := false
	for i := range m.store.Characters {
		filtered := m.store.Characters[i].AssignedDeviceIDs[:0]
		for _, d := range m.store.Characters[i].AssignedDeviceIDs {
			if !stale[d] {
				filtered = append(filtered, d)
			}
		}
		if len(filtered) != len(m.store.Characters[i].AssignedDeviceIDs) {
			m.store.Characters[i].AssignedDeviceIDs = filtered
			if m.store.Characters[i].AssignedDeviceIDs == nil {
				m.store.Characters[i].AssignedDeviceIDs = []string{}
			}
			changed = true
		}
	}
	if changed {
		m.save()
	}
}

// Package-level functions
func ListCharacters() []Character                     { return GetCharacterManager().List() }
func GetCharacter(id string) (*Character, bool)       { return GetCharacterManager().Get(id) }
func AddCharacter(c Character) Character              { return GetCharacterManager().Add(c) }
func UpdateCharacter(id string, c Character) bool     { return GetCharacterManager().Update(id, c) }
func DeleteCharacter(id string) bool                  { return GetCharacterManager().Delete(id) }
func AssignDevicesToCharacter(charID string, deviceIDs []string) (*Character, bool) {
	return GetCharacterManager().AssignDevices(charID, deviceIDs)
}
func PruneCharacterDevices(deviceIDs []string) { GetCharacterManager().PruneDevices(deviceIDs) }
func SetCharacterAuraDefault(characterID, wantID string, mark AuraMark) (*Character, bool) {
	return GetCharacterManager().SetAuraDefault(characterID, wantID, mark)
}
func SetCharacterDesign(characterID, tileDesign, auraDesign string) (*Character, bool) {
	return GetCharacterManager().SetDesign(characterID, tileDesign, auraDesign)
}
