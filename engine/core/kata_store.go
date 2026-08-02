package mywant

import (
	"crypto/md5"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// Kata (型) is the unit of achievement: a named combination of 所作 (waza) that
// becomes "極まる" when every waza is satisfied within one 稽古 (window).
//
// Two axes, deliberately orthogonal:
//   - 幅: levels (帯).極めた型が promotion.requiredKata に達すると次の帯が開く.
//   - 深さ: mastery (練度). 同じ型を繰り返すほど 初伝 → 中伝 → 皆伝 と深まる.
//
// Definitions are always read from YAML seeds (engine/bundled/kata/) so they can
// be edited freely; only the practice records — the evidence that a kata was
// 極まった — are persisted to ~/.mywant/kata-records.yaml.

// Mastery rank names. Two ranks only: holding a kata once, and holding it
// often enough that the system can take it over.
const (
	MasteryNone   = ""
	MasteryShoden = "shoden" // 初伝
	MasteryKaiden = "kaiden" // 皆伝
)

// Waza (所作) is one condition parcel inside a kata.
//
// Kind determines which fields matter:
//   - want_type: Type (+ Status, Count) — that many wants of this type reached Status
//   - memo:      Subtype (+ MinCount)   — remembered values of this subtype
//   - repeat:    Kata (+ MinCount)      — another kata was 極まった that many times
type Waza struct {
	Kind string `yaml:"kind" json:"kind"`

	// want_type
	Type   string `yaml:"type,omitempty"   json:"type,omitempty"`
	Status string `yaml:"status,omitempty" json:"status,omitempty"`
	Count  int    `yaml:"count,omitempty"  json:"count,omitempty"`

	// memo
	Subtype string `yaml:"subtype,omitempty" json:"subtype,omitempty"`

	// repeat
	Kata string `yaml:"kata,omitempty" json:"kata,omitempty"`

	// memo / repeat
	MinCount int `yaml:"minCount,omitempty" json:"minCount,omitempty"`

	// Join names the parameter whose value must be a member of the kata's
	// joined memo group. Only meaningful on want_type waza inside a kata that
	// declares a join — it is what makes two wants be about the same thing.
	Join string `yaml:"join,omitempty" json:"join,omitempty"`
}

// Need returns how many occurrences this waza requires (always >= 1).
func (w Waza) Need() int {
	switch {
	case w.MinCount > 0:
		return w.MinCount
	case w.Count > 0:
		return w.Count
	default:
		return 1
	}
}

// Henka (変化) is an alternative way into the same kata — the OR branch.
// A kata with no henka has a single implicit variation built from its Waza.
type Henka struct {
	ID    string `yaml:"id"    json:"id"`
	Label string `yaml:"label" json:"label"`
	Waza  []Waza `yaml:"waza"  json:"waza"`
}

// KataJoin ties a kata's 所作 to one shared thing.
//
// A kata is never a schedule and never an order — it is a set of wants that are
// about the SAME thing, and the memo group is what says so. "渋谷" as a station
// and "渋谷" as a city are different memo values; putting both in one group is
// the user declaring they are one place. A route searched to the station and a
// forecast fetched for the city then join, and the pair can say something
// neither said alone.
//
//	kind: memo_group — every joined 所作 must resolve inside one memo group
type KataJoin struct {
	Kind string `yaml:"kind" json:"kind"`
}

// KataUnlocks is what deepening a kata grants. Three axes, never a feature gate:
// every want type stays usable from day one — only the shorthand, the delegation
// and the vocabulary are earned.
type KataUnlocks struct {
	Shortcuts  []string `yaml:"shortcuts,omitempty"  json:"shortcuts,omitempty"`  // 手数
	Autonomy   []string `yaml:"autonomy,omitempty"   json:"autonomy,omitempty"`   // 権限
	Vocabulary []string `yaml:"vocabulary,omitempty" json:"vocabulary,omitempty"` // 語彙
}

// MasteryThresholds maps rank → number of times the kata must be 極まった.
type MasteryThresholds struct {
	Shoden int `yaml:"shoden" json:"shoden"`
	Kaiden int `yaml:"kaiden" json:"kaiden"`
}

// Kata is one named form.
type Kata struct {
	ID      string `yaml:"id"                json:"id"`
	Name    string `yaml:"name"              json:"name"`
	Reading string `yaml:"reading,omitempty" json:"reading,omitempty"`
	Level   string `yaml:"level"             json:"level"`
	Order   int    `yaml:"order,omitempty"   json:"order,omitempty"`
	Intent  string `yaml:"intent,omitempty"  json:"intent,omitempty"`

	// Yields is what the combination hands the user — the answer that none of
	// its 所作 produces alone. This is the whole reason the kata exists, and it
	// is available the moment the 所作 line up; 練度 only makes it cheaper to
	// reach, never gates it.
	Yields string `yaml:"yields,omitempty" json:"yields,omitempty"`

	// Hidden marks a 口伝 — it is not listed in advance. It only reveals itself
	// once it is one waza away, so it can never be ground for, only discovered.
	Hidden bool `yaml:"hidden,omitempty" json:"hidden,omitempty"`

	// Contains names the lower kata whose 所作 this form subsumes. Upper forms
	// inherit the basics, exactly like an advanced kata in a martial art.
	Contains []string `yaml:"contains,omitempty" json:"contains,omitempty"`

	Join    KataJoin               `yaml:"join,omitempty"    json:"join"`
	Waza    []Waza                 `yaml:"waza,omitempty"    json:"waza,omitempty"`
	Henka   []Henka                `yaml:"henka,omitempty"   json:"henka,omitempty"`
	Mastery MasteryThresholds      `yaml:"mastery,omitempty" json:"mastery"`
	Unlocks map[string]KataUnlocks `yaml:"unlocks,omitempty" json:"unlocks,omitempty"`
}

// Variations returns the kata's OR branches, wrapping a bare Waza list in a
// single implicit variation so callers never special-case it.
func (k Kata) Variations() []Henka {
	if len(k.Henka) > 0 {
		return k.Henka
	}
	return []Henka{{ID: "default", Waza: k.Waza}}
}

// RankFor returns the mastery rank earned at n completions.
func (k Kata) RankFor(n int) string {
	switch {
	case k.Mastery.Kaiden > 0 && n >= k.Mastery.Kaiden:
		return MasteryKaiden
	case k.Mastery.Shoden > 0 && n >= k.Mastery.Shoden:
		return MasteryShoden
	case n > 0:
		return MasteryShoden
	default:
		return MasteryNone
	}
}

// KataPromotion is the bar for opening the next level.
type KataPromotion struct {
	RequiredKata int `yaml:"requiredKata" json:"requiredKata"`
}

// KataLevel (帯) groups kata. Clearing RequiredKata of them opens the next belt.
type KataLevel struct {
	ID       string `yaml:"id"                 json:"id"`
	Name     string `yaml:"name"               json:"name"`
	Grade    string `yaml:"grade,omitempty"    json:"grade,omitempty"`
	Order    int    `yaml:"order"              json:"order"`
	Theme    string `yaml:"theme,omitempty"    json:"theme,omitempty"`
	Subtitle string `yaml:"subtitle,omitempty" json:"subtitle,omitempty"`
	// Color is the belt's own colour, drawn as the belt bar. Accent is the ink
	// used for dots, markers and borders — a white belt's colour is unusable
	// as ink, so the two must be separate.
	Color    string        `yaml:"color,omitempty"    json:"color,omitempty"`
	Accent   string        `yaml:"accent,omitempty"   json:"accent,omitempty"`
	Unlocked bool          `yaml:"unlocked,omitempty" json:"unlocked,omitempty"` // seed value: the first belt is open
	Kata     []string      `yaml:"kata"               json:"kata"`
	Promote  KataPromotion `yaml:"promotion"          json:"promotion"`
}

// KataRecord is the evidence that a kata was 極まった once.
// SessionKey deduplicates: the same set of wants must not count twice.
type KataRecord struct {
	KataID     string    `yaml:"kataID"            json:"kataID"`
	At         time.Time `yaml:"at"                json:"at"`
	SessionKey string    `yaml:"sessionKey"        json:"sessionKey"`
	WantIDs    []string  `yaml:"wantIDs,omitempty" json:"wantIDs,omitempty"`
	Variation  string    `yaml:"variation,omitempty" json:"variation,omitempty"`
}

// kataConfigFile is the YAML seed format (engine/bundled/kata/*.yaml).
type kataConfigFile struct {
	Levels []KataLevel `yaml:"levels"`
	Kata   []Kata      `yaml:"kata"`
}

// kataRecordFile is the on-disk shape of ~/.mywant/kata-records.yaml.
type kataRecordFile struct {
	Records []KataRecord `yaml:"records"`
}

var (
	globalKataManager     *kataManager
	globalKataManagerOnce sync.Once
)

type kataManager struct {
	mu       sync.RWMutex
	path     string
	lastHash string
	levels   []KataLevel
	kata     []Kata
	records  []KataRecord
}

// GetKataManager returns the singleton kata manager.
func GetKataManager() *kataManager {
	globalKataManagerOnce.Do(func() {
		home, err := os.UserHomeDir()
		if err != nil {
			log.Printf("[WARN] kata_store: cannot determine home dir: %v", err)
			home = "."
		}
		m := &kataManager{path: filepath.Join(home, ".mywant", "kata-records.yaml")}
		m.loadRecords()
		globalKataManager = m
	})
	return globalKataManager
}

func (m *kataManager) loadRecords() {
	data, err := os.ReadFile(m.path)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("[WARN] kata_store: failed to read %s: %v", m.path, err)
		}
		return
	}
	var f kataRecordFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		log.Printf("[WARN] kata_store: failed to unmarshal %s: %v", m.path, err)
		return
	}
	m.records = f.Records
	m.lastHash = fmt.Sprintf("%x", md5.Sum(data))
	log.Printf("[KataStore] Loaded %d practice records from %s", len(f.Records), m.path)
}

// saveRecords persists records. Caller must hold the write lock.
func (m *kataManager) saveRecords() {
	data, err := yaml.Marshal(&kataRecordFile{Records: m.records})
	if err != nil {
		log.Printf("[WARN] kata_store: marshal failed: %v", err)
		return
	}
	newHash := fmt.Sprintf("%x", md5.Sum(data))
	if newHash == m.lastHash {
		return
	}
	if err := os.MkdirAll(filepath.Dir(m.path), 0755); err != nil {
		log.Printf("[WARN] kata_store: mkdir failed: %v", err)
		return
	}
	if err := os.WriteFile(m.path, data, 0644); err != nil {
		log.Printf("[WARN] kata_store: write failed: %v", err)
		return
	}
	m.lastHash = newHash
}

// ── Definitions ───────────────────────────────────────────────────────────────

// addDefinitions merges one parsed seed file, replacing same-ID entries so an
// edited YAML always wins over what a previous file declared.
func (m *kataManager) addDefinitions(cfg kataConfigFile) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, lv := range cfg.Levels {
		replaced := false
		for i, existing := range m.levels {
			if existing.ID == lv.ID {
				m.levels[i] = lv
				replaced = true
				break
			}
		}
		if !replaced {
			m.levels = append(m.levels, lv)
		}
		n++
	}
	for _, k := range cfg.Kata {
		replaced := false
		for i, existing := range m.kata {
			if existing.ID == k.ID {
				m.kata[i] = k
				replaced = true
				break
			}
		}
		if !replaced {
			m.kata = append(m.kata, k)
		}
		n++
	}
	sort.SliceStable(m.levels, func(i, j int) bool { return m.levels[i].Order < m.levels[j].Order })
	// Kata sort by belt first, then by their order within it, so a flat list
	// still reads as a curriculum.
	levelOrder := make(map[string]int, len(m.levels))
	for _, lv := range m.levels {
		levelOrder[lv.ID] = lv.Order
	}
	sort.SliceStable(m.kata, func(i, j int) bool {
		li, lj := levelOrder[m.kata[i].Level], levelOrder[m.kata[j].Level]
		if li != lj {
			return li < lj
		}
		return m.kata[i].Order < m.kata[j].Order
	})
	return n
}

func (m *kataManager) ListLevels() []KataLevel {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]KataLevel, len(m.levels))
	copy(out, m.levels)
	return out
}

func (m *kataManager) ListKata() []Kata {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Kata, len(m.kata))
	copy(out, m.kata)
	return out
}

func (m *kataManager) GetKata(id string) (*Kata, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, k := range m.kata {
		if k.ID == id {
			cp := k
			return &cp, true
		}
	}
	return nil, false
}

// ── Records ───────────────────────────────────────────────────────────────────

func (m *kataManager) ListRecords() []KataRecord {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]KataRecord, len(m.records))
	copy(out, m.records)
	return out
}

// MasteryCount returns how many times the kata has been 極まった.
func (m *kataManager) MasteryCount(kataID string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	n := 0
	for _, r := range m.records {
		if r.KataID == kataID {
			n++
		}
	}
	return n
}

// RecordPractice stores one completion, ignoring a repeat of the same session.
// Returns true when this is a newly counted practice.
func (m *kataManager) RecordPractice(rec KataRecord) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.records {
		if r.KataID == rec.KataID && r.SessionKey == rec.SessionKey {
			return false
		}
	}
	if rec.At.IsZero() {
		rec.At = time.Now()
	}
	m.records = append(m.records, rec)
	m.saveRecords()
	log.Printf("[KataStore] 極まった: %s (session %s)", rec.KataID, rec.SessionKey)
	return true
}

// SessionKeyFor builds a stable dedup key from the want IDs that satisfied a kata.
// Same wants → same key → counted once, however often evaluation runs.
func SessionKeyFor(wantIDs []string) string {
	ids := make([]string, len(wantIDs))
	copy(ids, wantIDs)
	sort.Strings(ids)
	return fmt.Sprintf("%x", md5.Sum([]byte(strings.Join(ids, "|"))))[:12]
}

// ── Package-level helpers ─────────────────────────────────────────────────────

func ListKataLevels() []KataLevel          { return GetKataManager().ListLevels() }
func ListKata() []Kata                     { return GetKataManager().ListKata() }
func GetKata(id string) (*Kata, bool)      { return GetKataManager().GetKata(id) }
func ListKataRecords() []KataRecord        { return GetKataManager().ListRecords() }
func KataMasteryCount(id string) int       { return GetKataManager().MasteryCount(id) }
func RecordKataPractice(r KataRecord) bool { return GetKataManager().RecordPractice(r) }

// ── Seed loading ──────────────────────────────────────────────────────────────

// LoadKataConfigs reads all *.yaml files from dir as kata definitions.
func LoadKataConfigs(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	m := GetKataManager()
	loaded := 0
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			log.Printf("[KataStore] failed to read %s: %v", path, err)
			continue
		}
		var cfg kataConfigFile
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			log.Printf("[KataStore] failed to parse %s: %v", path, err)
			continue
		}
		loaded += m.addDefinitions(cfg)
	}
	if loaded > 0 {
		log.Printf("[KataStore] Loaded %d definitions from %s", loaded, dir)
	}
	return nil
}

// LoadKataConfigsFromFS loads kata definitions from an embedded fs.FS.
func LoadKataConfigsFromFS(fsys fs.FS, fsRoot string) error {
	m := GetKataManager()
	loaded := 0
	err := fs.WalkDir(fsys, fsRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if filepath.Ext(d.Name()) != ".yaml" {
			return nil
		}
		data, readErr := fs.ReadFile(fsys, path)
		if readErr != nil {
			log.Printf("[KataStore] failed to read embedded %s: %v", path, readErr)
			return nil
		}
		var cfg kataConfigFile
		if yamlErr := yaml.Unmarshal(data, &cfg); yamlErr != nil {
			log.Printf("[KataStore] failed to parse embedded %s: %v", path, yamlErr)
			return nil
		}
		loaded += m.addDefinitions(cfg)
		return nil
	})
	if loaded > 0 {
		log.Printf("[KataStore] Loaded %d definitions from embedded FS", loaded)
	}
	return err
}
