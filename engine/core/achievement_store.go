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

// Achievement represents a single earned title for an agent.
type Achievement struct {
	ID                string         `yaml:"id"                          json:"id"`
	Title             string         `yaml:"title"                       json:"title"`
	Description       string         `yaml:"description"                 json:"description"`
	AgentName         string         `yaml:"agentName"                   json:"agentName"`
	WantID            string         `yaml:"wantID"                      json:"wantID"`
	WantName          string         `yaml:"wantName"                    json:"wantName"`
	Category          string         `yaml:"category"                    json:"category"` // execution / quality / specialization
	Level             int            `yaml:"level"                       json:"level"`    // 1=bronze 2=silver 3=gold
	EarnedAt          time.Time      `yaml:"earnedAt"                    json:"earnedAt"`
	AwardedBy         string         `yaml:"awardedBy"                   json:"awardedBy"` // system / capability_manager / human
	UnlocksCapability string         `yaml:"unlocksCapability,omitempty" json:"unlocksCapability,omitempty"`
	Metadata          map[string]any `yaml:"metadata,omitempty"          json:"metadata,omitempty"`
}

// AchievementCondition defines when an achievement should be auto-awarded.
type AchievementCondition struct {
	// AgentCapability: only count completions where the agent has this capability
	AgentCapability string `yaml:"agentCapability,omitempty" json:"agentCapability,omitempty"`
	// WantType: only count completions of wants with this type
	WantType string `yaml:"wantType,omitempty" json:"wantType,omitempty"`
	// CompletedCount: award when the agent has completed >= this many qualifying wants
	CompletedCount int `yaml:"completedCount" json:"completedCount"`
}

// AchievementAward describes what to award when conditions are met.
type AchievementAward struct {
	Title             string `yaml:"title"                       json:"title"`
	Description       string `yaml:"description"                 json:"description"`
	Level             int    `yaml:"level"                       json:"level"`
	Category          string `yaml:"category"                    json:"category"`
	UnlocksCapability string `yaml:"unlocksCapability,omitempty" json:"unlocksCapability,omitempty"`
}

// AchievementRule defines the condition under which an achievement is auto-awarded.
type AchievementRule struct {
	ID        string               `yaml:"id"        json:"id"`
	Active    bool                 `yaml:"active"    json:"active"`
	Condition AchievementCondition `yaml:"condition" json:"condition"`
	Award     AchievementAward     `yaml:"award"     json:"award"`
}

// AchievementStore is the root document persisted to ~/.mywant/achievements.yaml.
type AchievementStore struct {
	Achievements []Achievement    `yaml:"achievements"`
	Rules        []AchievementRule `yaml:"rules"`
}

// OnAchievementAddedFunc is a hook called after an achievement is persisted.
// Typically used to register unlocked capabilities into the AgentRegistry.
type OnAchievementAddedFunc func(a Achievement)

var (
	globalAchievementStore     *achievementManager
	globalAchievementStoreOnce sync.Once
)

type achievementManager struct {
	mu       sync.RWMutex
	path     string
	lastHash string
	store    AchievementStore
	hooks    []OnAchievementAddedFunc
}

// GetAchievementManager returns the singleton achievement manager.
func GetAchievementManager() *achievementManager {
	globalAchievementStoreOnce.Do(func() {
		home, err := os.UserHomeDir()
		if err != nil {
			log.Printf("[WARN] achievement_store: cannot determine home dir: %v", err)
			home = "."
		}
		path := filepath.Join(home, ".mywant", "achievements.yaml")
		m := &achievementManager{path: path}
		m.load()
		globalAchievementStore = m
	})
	return globalAchievementStore
}

// RegisterOnAchievementAdded registers a hook called after every new achievement.
func RegisterOnAchievementAdded(fn OnAchievementAddedFunc) {
	m := GetAchievementManager()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.hooks = append(m.hooks, fn)
}

func (m *achievementManager) load() {
	data, err := os.ReadFile(m.path)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("[WARN] achievement_store: failed to read %s: %v", m.path, err)
		}
		return
	}
	var s AchievementStore
	if err := yaml.Unmarshal(data, &s); err != nil {
		log.Printf("[WARN] achievement_store: failed to unmarshal %s: %v", m.path, err)
		return
	}
	m.store = s
	m.lastHash = fmt.Sprintf("%x", md5.Sum(data))
	log.Printf("[AchievementStore] Loaded %d achievements, %d rules from %s",
		len(s.Achievements), len(s.Rules), m.path)
}

func (m *achievementManager) save() {
	data, err := yaml.Marshal(&m.store)
	if err != nil {
		log.Printf("[WARN] achievement_store: marshal failed: %v", err)
		return
	}
	newHash := fmt.Sprintf("%x", md5.Sum(data))
	if newHash == m.lastHash {
		return
	}
	if err := os.MkdirAll(filepath.Dir(m.path), 0755); err != nil {
		log.Printf("[WARN] achievement_store: mkdir failed: %v", err)
		return
	}
	if err := os.WriteFile(m.path, data, 0644); err != nil {
		log.Printf("[WARN] achievement_store: write failed: %v", err)
		return
	}
	m.lastHash = newHash
}

// ── Achievements ──────────────────────────────────────────────────────────────

func (m *achievementManager) List() []Achievement {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Achievement, len(m.store.Achievements))
	copy(out, m.store.Achievements)
	return out
}

func (m *achievementManager) Get(id string) (*Achievement, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, a := range m.store.Achievements {
		if a.ID == id {
			cp := a
			return &cp, true
		}
	}
	return nil, false
}

func (m *achievementManager) Add(a Achievement) Achievement {
	m.mu.Lock()
	if a.ID == "" {
		a.ID = "ach-" + uuid.New().String()[:8]
	}
	if a.EarnedAt.IsZero() {
		a.EarnedAt = time.Now()
	}
	m.store.Achievements = append(m.store.Achievements, a)
	m.save()
	hooks := make([]OnAchievementAddedFunc, len(m.hooks))
	copy(hooks, m.hooks)
	m.mu.Unlock()

	log.Printf("[AchievementStore] Added achievement %s (%s) for agent %s", a.ID, a.Title, a.AgentName)
	for _, fn := range hooks {
		fn(a)
	}
	return a
}

func (m *achievementManager) Update(id string, updated Achievement) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, a := range m.store.Achievements {
		if a.ID == id {
			updated.ID = id
			m.store.Achievements[i] = updated
			m.save()
			return true
		}
	}
	return false
}

func (m *achievementManager) Delete(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, a := range m.store.Achievements {
		if a.ID == id {
			m.store.Achievements = append(m.store.Achievements[:i], m.store.Achievements[i+1:]...)
			m.save()
			return true
		}
	}
	return false
}

func (m *achievementManager) ListByAgent(agentName string) []Achievement {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []Achievement
	for _, a := range m.store.Achievements {
		if a.AgentName == agentName {
			out = append(out, a)
		}
	}
	return out
}

// HasAchievement returns true if the agent already has an achievement with the given title.
func (m *achievementManager) HasAchievement(agentName, title string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, a := range m.store.Achievements {
		if a.AgentName == agentName && a.Title == title {
			return true
		}
	}
	return false
}

// ── Rules ─────────────────────────────────────────────────────────────────────

func (m *achievementManager) ListRules() []AchievementRule {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]AchievementRule, len(m.store.Rules))
	copy(out, m.store.Rules)
	return out
}

func (m *achievementManager) GetRule(id string) (*AchievementRule, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, r := range m.store.Rules {
		if r.ID == id {
			cp := r
			return &cp, true
		}
	}
	return nil, false
}

func (m *achievementManager) AddRule(r AchievementRule) AchievementRule {
	m.mu.Lock()
	defer m.mu.Unlock()
	if r.ID == "" {
		r.ID = "rule-" + uuid.New().String()[:8]
	}
	m.store.Rules = append(m.store.Rules, r)
	m.save()
	log.Printf("[AchievementStore] Added rule %s (%s)", r.ID, r.Award.Title)
	return r
}

func (m *achievementManager) UpdateRule(id string, updated AchievementRule) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, r := range m.store.Rules {
		if r.ID == id {
			updated.ID = id
			m.store.Rules[i] = updated
			m.save()
			return true
		}
	}
	return false
}

func (m *achievementManager) DeleteRule(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, r := range m.store.Rules {
		if r.ID == id {
			m.store.Rules = append(m.store.Rules[:i], m.store.Rules[i+1:]...)
			m.save()
			return true
		}
	}
	return false
}

// ── Package-level functions ───────────────────────────────────────────────────

func AddAchievement(a Achievement) Achievement        { return GetAchievementManager().Add(a) }
func ListAchievements() []Achievement                 { return GetAchievementManager().List() }
func GetAchievement(id string) (*Achievement, bool)   { return GetAchievementManager().Get(id) }
func DeleteAchievement(id string) bool                { return GetAchievementManager().Delete(id) }
func UpdateAchievement(id string, u Achievement) bool { return GetAchievementManager().Update(id, u) }
func ListAchievementsByAgent(n string) []Achievement  { return GetAchievementManager().ListByAgent(n) }
func HasAchievement(agentName, title string) bool     { return GetAchievementManager().HasAchievement(agentName, title) }

func AddAchievementRule(r AchievementRule) AchievementRule    { return GetAchievementManager().AddRule(r) }
func ListAchievementRules() []AchievementRule                  { return GetAchievementManager().ListRules() }
func GetAchievementRule(id string) (*AchievementRule, bool)   { return GetAchievementManager().GetRule(id) }
func DeleteAchievementRule(id string) bool                     { return GetAchievementManager().DeleteRule(id) }
func UpdateAchievementRule(id string, r AchievementRule) bool { return GetAchievementManager().UpdateRule(id, r) }
