package mywant

import (
	"crypto/md5"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
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
	// AuraDefaults maps an AuraTarget key (see AuraTarget.Key) to the aura-default
	// mark this character has set for it — shown as an aura-colored dog-ear
	// flag/star on the marked option/state in that want's card UI, toggled via
	// the X key/button. Keyed by target rather than by want instance so a mark is
	// a shareable asset: it survives redeploys and means the same thing in
	// another install, where the instance UUIDs it used to name do not exist.
	AuraDefaults map[string]AuraMark `yaml:"auraDefaults,omitempty" json:"auraDefaults,omitempty"`
	// AuraCardWantID is the want this character has bookmarked as their "aura
	// card" — an ordinary want whose tile/card visually represents this
	// character wherever it appears (dashboard grid or canvas), toggled via
	// the ★ button on any want card. One want per character; distinct from
	// AuraDefaults, which marks a single value within a specific want's
	// controls rather than the whole want. Empty = no aura card set.
	AuraCardWantID string `yaml:"auraCardWantId,omitempty" json:"auraCardWantId,omitempty"`
	// TileDesign / AuraDesign are the design-plugin ids this character picks for
	// the want tiles and aura they own on the canvas (e.g. "cubic", "forest").
	// Empty = inherit the canvas-level design (config.canvas_design).
	TileDesign string `yaml:"tileDesign,omitempty" json:"tile_design,omitempty"`
	AuraDesign string `yaml:"auraDesign,omitempty" json:"aura_design,omitempty"`
}

// AuraTarget addresses what an aura mark is about. Scope (Kind+Name) names
// something that exists identically in any install — never an instance UUID,
// which is what makes a mark portable. Path locates a point within that scope,
// and its grammar belongs to the Kind: only the resolver for a Kind parses it,
// so a new Kind adds a resolver rather than changing this struct.
//
// A mark's Kind also says what the mark *does* (see AuraMark.Role):
//   - "wantType" is a BINDING target. Name is the want type name, Path is
//     "<section>/<key>" (section: current/goal/plan/internal, or "parameter"
//     for a spec param). The mark's value is applied to that field.
//   - a CATALOG kind (e.g. "place", "site") is a DEFINITION target. Name is the
//     name being defined, Path is empty (the whole object) or a sub-field. The
//     mark's value *is* the definition that name resolves to — nothing is
//     "applied"; other things reference it by name.
type AuraTarget struct {
	Kind string `yaml:"kind" json:"kind"`
	Name string `yaml:"name" json:"name"`
	Path string `yaml:"path" json:"path"`
}

// AuraTargetKindWantType is the binding kind — marks on want-type fields.
const AuraTargetKindWantType = "wantType"

// Key is the AuraTarget's canonical string form, used as the AuraDefaults map
// key. "|" separates scope from path because a Path may itself contain "/".
func (t AuraTarget) Key() string { return t.Kind + ":" + t.Name + "|" + t.Path }

// Valid reports whether the target names a scope. Path is required for binding
// targets (which field) but optional for definition targets (empty Path defines
// the whole named object), so it is not checked here.
func (t AuraTarget) Valid() bool { return t.Kind != "" && t.Name != "" }

// IsBinding reports whether this target addresses a want field (so a mark on it
// applies its value), as opposed to defining a catalog entry.
func (t AuraTarget) IsBinding() bool { return t.Kind == AuraTargetKindWantType }

// SectionKey splits a binding Path into its section and key halves.
func (t AuraTarget) SectionKey() (string, string) {
	if i := strings.Index(t.Path, "/"); i >= 0 {
		return t.Path[:i], t.Path[i+1:]
	}
	return t.Path, ""
}

// AuraMark is one character's mark on a target. It does one of two things,
// decided by whether its target is a binding or a definition:
//
//   - BINDING (Target.IsBinding): Value is a scalar applied to a want field
//     when an aura covering that want activates. Mode set/endorse governs
//     whether it is written back at all.
//   - DEFINITION (catalog target): Value is the object the target's name
//     resolves to — e.g. place "会社" → {lat, lng, radius}. It is stored and
//     read by name; nothing applies it to a want.
//
// Value is `any` precisely so it can hold both a scalar and a structured
// definition. A pre-existing mark whose value was a plain string simply
// unmarshals as a string here — no migration needed for the widening.
type AuraMark struct {
	Target AuraTarget `yaml:"target" json:"target"`
	Value  any        `yaml:"value"  json:"value"`
	// Mode distinguishes a binding that should be applied to its field
	// (AuraModeSet, the default) from one that only records "this observed
	// value is the good one" (AuraModeEndorse) and is never written back.
	// Ignored for definition marks.
	Mode string `yaml:"mode,omitempty" json:"mode,omitempty"`
	// By is the ID of the character who signed this mark — an aura is a shared
	// asset, so it carries its author rather than being owned by the record it
	// currently sits in.
	By string `yaml:"by,omitempty" json:"by,omitempty"`

	// legacySection/legacyKey are only ever populated by unmarshalling a
	// pre-target characters.yaml, where a mark was keyed by want instance ID and
	// carried a flat section/key pair. MigrateAuraDefaults consumes and clears
	// them; they are never written back (omitempty + always empty after
	// migration) and are invisible to the API.
	LegacySection string `yaml:"section,omitempty" json:"-"`
	LegacyKey     string `yaml:"key,omitempty"     json:"-"`
}

// Aura mark modes (binding marks only).
const (
	AuraModeSet     = "set"
	AuraModeEndorse = "endorse"
)

// IsEmpty reports whether the mark carries no value — the signal to clear it.
// Covers both a nil/absent value and the empty string a binding clear sends.
func (m AuraMark) IsEmpty() bool {
	if m.Value == nil {
		return true
	}
	s, ok := m.Value.(string)
	return ok && s == ""
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
		m.ensureDefaultCharacters()
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
			updated.AuraDefaults = c.AuraDefaults     // preserve aura-default marks
			updated.AuraCardWantID = c.AuraCardWantID // preserve aura-card pick
			updated.TileDesign = c.TileDesign         // preserve design picks (set via /design)
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

// SetAuraDefault pins mark.Value at mark.Target for a character, or clears the
// mark on that target when the value is empty (see AuraMark.IsEmpty).
func (m *characterManager) SetAuraDefault(characterID string, mark AuraMark) (*Character, bool) {
	if !mark.Target.Valid() {
		return nil, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, c := range m.store.Characters {
		if c.ID != characterID {
			continue
		}
		key := mark.Target.Key()
		if mark.IsEmpty() {
			delete(m.store.Characters[i].AuraDefaults, key)
		} else {
			// Mode is meaningful only for bindings; definition marks leave it
			// empty so nothing tries to "apply" them.
			if mark.Mode == "" && mark.Target.IsBinding() {
				mark.Mode = AuraModeSet
			}
			mark.By = characterID
			if m.store.Characters[i].AuraDefaults == nil {
				m.store.Characters[i].AuraDefaults = map[string]AuraMark{}
			}
			m.store.Characters[i].AuraDefaults[key] = mark
		}
		m.save()
		cp := m.store.Characters[i]
		return &cp, true
	}
	return nil, false
}

// MigrateAuraDefaults rewrites pre-target aura marks — keyed by want instance
// UUID, carrying a flat section/key pair — into target-keyed form, using
// resolveWantType to turn each stored want ID into the want type the mark
// should from now on belong to. Marks whose want no longer exists cannot be
// re-addressed and are dropped. Idempotent: already-migrated marks are left
// alone, so it is safe to call on every startup.
func (m *characterManager) MigrateAuraDefaults(resolveWantType func(wantID string) (string, bool)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	migrated, dropped := 0, 0
	for i := range m.store.Characters {
		old := m.store.Characters[i].AuraDefaults
		if len(old) == 0 {
			continue
		}
		next := make(map[string]AuraMark, len(old))
		for key, mark := range old {
			if mark.Target.Valid() {
				next[key] = mark // already target-keyed
				continue
			}
			wantType, ok := resolveWantType(key)
			if !ok {
				log.Printf("[CharacterStore] Dropping aura mark for unknown want %s (character %s)",
					key, m.store.Characters[i].Name)
				dropped++
				continue
			}
			mark.Target = AuraTarget{
				Kind: AuraTargetKindWantType,
				Name: wantType,
				Path: mark.LegacySection + "/" + mark.LegacyKey,
			}
			mark.LegacySection, mark.LegacyKey = "", ""
			if mark.Mode == "" {
				mark.Mode = AuraModeSet
			}
			if mark.By == "" {
				mark.By = m.store.Characters[i].ID
			}
			next[mark.Target.Key()] = mark
			migrated++
		}
		m.store.Characters[i].AuraDefaults = next
	}
	if migrated > 0 || dropped > 0 {
		m.save()
		log.Printf("[CharacterStore] Migrated %d aura mark(s) to target keys, dropped %d", migrated, dropped)
	}
}

// SetAuraCardWant sets (or clears, with wantID == "") the want this character
// has bookmarked as their aura card. One want per character — setting a new
// one replaces any previous pick.
func (m *characterManager) SetAuraCardWant(characterID, wantID string) (*Character, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, c := range m.store.Characters {
		if c.ID != characterID {
			continue
		}
		m.store.Characters[i].AuraCardWantID = wantID
		m.save()
		cp := m.store.Characters[i]
		return &cp, true
	}
	return nil, false
}

// ensureDefaultCharacters seeds characters that must always exist, run once
// after loading from disk. Currently just "robot" — the always-on chat
// companion — pre-bound to the "robot" system want (same fixed ID) as its
// aura card, so it shows the right avatar/color without a manual ★ press.
// Guarded on ID so a user's later changes (rename, re-picked aura card, ...)
// are never overwritten on subsequent restarts.
func (m *characterManager) ensureDefaultCharacters() {
	m.mu.Lock()
	for _, c := range m.store.Characters {
		if c.ID == "robot" {
			m.mu.Unlock()
			return
		}
	}
	m.store.Characters = append(m.store.Characters, Character{
		ID:                "robot",
		Name:              "robot",
		Avatar:            "🤖",
		Color:             "#8b5cf6",
		CreatedAt:         time.Now().UnixMilli(),
		AssignedDeviceIDs: []string{},
		AuraCardWantID:    "robot",
	})
	m.save()
	m.mu.Unlock()
	log.Printf("[CharacterStore] Seeded default 'robot' character")
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

// ResolveAuraDefinition returns the definition value any character has set for
// the catalog entry (kind, name) — the payload a definition mark carries, e.g.
// place "会社" → {lat, lng, radius}. Consumers resolve a catalog name to its
// object through this without knowing which character authored it. Binding
// marks (want-type targets) are never returned. Whole-object definitions use an
// empty path; ok is false when no such definition exists.
func (m *characterManager) ResolveAuraDefinition(kind, name, path string) (any, bool) {
	key := AuraTarget{Kind: kind, Name: name, Path: path}.Key()
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, c := range m.store.Characters {
		if mark, ok := c.AuraDefaults[key]; ok && !mark.Target.IsBinding() {
			return mark.Value, true
		}
	}
	return nil, false
}

// AuraDefinitions returns every definition mark of the given catalog kind
// across all characters, keyed by the entry name — the readable catalog for
// that kind (e.g. all known places). Binding marks are excluded.
func (m *characterManager) AuraDefinitions(kind string) map[string]AuraMark {
	out := map[string]AuraMark{}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, c := range m.store.Characters {
		for _, mark := range c.AuraDefaults {
			if mark.Target.Kind == kind && !mark.Target.IsBinding() {
				out[mark.Target.Name] = mark
			}
		}
	}
	return out
}

// AllAuraDefinitions returns every definition mark across all characters and
// kinds — the whole named vocabulary someone has built. Binding marks are
// excluded. Used as raw material for generating riffs.
func (m *characterManager) AllAuraDefinitions() []AuraMark {
	out := []AuraMark{}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, c := range m.store.Characters {
		for _, mark := range c.AuraDefaults {
			if !mark.Target.IsBinding() {
				out = append(out, mark)
			}
		}
	}
	return out
}

// Package-level functions
func ListCharacters() []Character                 { return GetCharacterManager().List() }
func ResolveAuraDefinition(kind, name, path string) (any, bool) {
	return GetCharacterManager().ResolveAuraDefinition(kind, name, path)
}
func AuraDefinitions(kind string) map[string]AuraMark {
	return GetCharacterManager().AuraDefinitions(kind)
}
func AllAuraDefinitions() []AuraMark { return GetCharacterManager().AllAuraDefinitions() }
func GetCharacter(id string) (*Character, bool)   { return GetCharacterManager().Get(id) }
func AddCharacter(c Character) Character          { return GetCharacterManager().Add(c) }
func UpdateCharacter(id string, c Character) bool { return GetCharacterManager().Update(id, c) }
func DeleteCharacter(id string) bool              { return GetCharacterManager().Delete(id) }
func AssignDevicesToCharacter(charID string, deviceIDs []string) (*Character, bool) {
	return GetCharacterManager().AssignDevices(charID, deviceIDs)
}
func PruneCharacterDevices(deviceIDs []string) { GetCharacterManager().PruneDevices(deviceIDs) }
func SetCharacterAuraDefault(characterID string, mark AuraMark) (*Character, bool) {
	return GetCharacterManager().SetAuraDefault(characterID, mark)
}
func MigrateCharacterAuraDefaults(resolveWantType func(wantID string) (string, bool)) {
	GetCharacterManager().MigrateAuraDefaults(resolveWantType)
}
func SetCharacterAuraCardWant(characterID, wantID string) (*Character, bool) {
	return GetCharacterManager().SetAuraCardWant(characterID, wantID)
}
func SetCharacterDesign(characterID, tileDesign, auraDesign string) (*Character, bool) {
	return GetCharacterManager().SetDesign(characterID, tileDesign, auraDesign)
}
