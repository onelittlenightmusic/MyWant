package server

import (
	"os"
	"path/filepath"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// maxNotifications caps the on-disk notice log. Oldest entries are dropped first.
const maxNotifications = 500

// NotificationEntry is one notice the GUI spoke through the robot's bubble.
//
// Every transient message the web UI shows the user goes through one client-side
// call (robotStore.notify), which posts here — so this log is the record of what
// the user was actually told, including the failures a console.error would have
// kept to itself. Distinct from RobotLogEntry, which records robot *commands*
// issued from the CLI; a notice is raised by the browser about its own state.
type NotificationEntry struct {
	ID      string `json:"id"      yaml:"id"`
	At      string `json:"at"      yaml:"at"`      // RFC3339 timestamp
	Message string `json:"message" yaml:"message"` // exactly what the bubble said

	// Where the bubble pointed, when it pointed at something (same
	// data-robot-target vocabulary the robot commands use).
	TargetType string `json:"targetType,omitempty" yaml:"targetType,omitempty"`
	TargetID   string `json:"targetId,omitempty"   yaml:"targetId,omitempty"`

	// Client context, so a notice raised on someone else's device is still
	// legible here: the SPA route it was raised on, and who was looking.
	Route       string `json:"route,omitempty"       yaml:"route,omitempty"`
	CharacterID string `json:"characterId,omitempty" yaml:"characterId,omitempty"`

	// Kind separates a deliberate per-want alert ("alert") — one that raises a
	// badge on the want's tile until the want is opened — from the transient
	// robot-bubble log ("" / legacy). Read is only meaningful for "alert".
	Kind string `json:"kind,omitempty" yaml:"kind,omitempty"`
	Read bool   `json:"read,omitempty" yaml:"read,omitempty"`
}

// NotificationStore appends notices to ~/.mywant/notifications.yaml.
// Mirrors ThingEventStore: load-modify-save under a mutex, newest last on disk,
// readers reverse for most-recent-first. Thread-safe.
type NotificationStore struct {
	path string
	mu   sync.Mutex
}

func newNotificationStore() *NotificationStore {
	home, _ := os.UserHomeDir()
	return &NotificationStore{path: filepath.Join(home, ".mywant", "notifications.yaml")}
}

func (n *NotificationStore) load() []NotificationEntry {
	bytes, err := os.ReadFile(n.path)
	if err != nil {
		return nil
	}
	var entries []NotificationEntry
	_ = yaml.Unmarshal(bytes, &entries)
	return entries
}

func (n *NotificationStore) save(entries []NotificationEntry) error {
	if err := os.MkdirAll(filepath.Dir(n.path), 0o755); err != nil {
		return err
	}
	bytes, err := yaml.Marshal(entries)
	if err != nil {
		return err
	}
	return os.WriteFile(n.path, bytes, 0o644)
}

// Record appends one notice, stamping id and time if unset.
func (n *NotificationStore) Record(entry NotificationEntry) error {
	if entry.Message == "" {
		return nil
	}
	now := time.Now()
	if entry.At == "" {
		entry.At = now.Format(time.RFC3339)
	}
	if entry.ID == "" {
		entry.ID = "notif-" + now.UTC().Format("20060102T150405.000000000")
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	entries := append(n.load(), entry)
	if len(entries) > maxNotifications {
		entries = entries[len(entries)-maxNotifications:]
	}
	return n.save(entries)
}

// All returns every notice, most-recent first, capped at limit (0 = no cap).
func (n *NotificationStore) All(limit int) []NotificationEntry {
	n.mu.Lock()
	defer n.mu.Unlock()

	entries := n.load()
	reversed := make([]NotificationEntry, 0, len(entries))
	for i := len(entries) - 1; i >= 0; i-- {
		if limit > 0 && len(reversed) >= limit {
			break
		}
		reversed = append(reversed, entries[i])
	}
	return reversed
}

// Clear drops every stored notice.
func (n *NotificationStore) Clear() error {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.save([]NotificationEntry{})
}

// ── per-want alerts ─────────────────────────────────────────────────────────

// UnreadWantCounts returns, per want id, how many unread "alert" notices target
// it — the number a tile / minimap badge shows.
func (n *NotificationStore) UnreadWantCounts() map[string]int {
	n.mu.Lock()
	defer n.mu.Unlock()
	counts := map[string]int{}
	for _, e := range n.load() {
		if e.Kind == "alert" && e.TargetType == "want" && e.TargetID != "" && !e.Read {
			counts[e.TargetID]++
		}
	}
	return counts
}

// ForWant returns the notices targeting one want, most-recent first (limit 0 =
// all), plus how many of them are unread alerts.
func (n *NotificationStore) ForWant(id string, limit int) (entries []NotificationEntry, unread int) {
	n.mu.Lock()
	defer n.mu.Unlock()
	all := n.load()
	for i := len(all) - 1; i >= 0; i-- {
		e := all[i]
		if e.TargetType != "want" || e.TargetID != id {
			continue
		}
		if e.Kind == "alert" && !e.Read {
			unread++
		}
		if limit == 0 || len(entries) < limit {
			entries = append(entries, e)
		}
	}
	return entries, unread
}

// MarkWantRead flags every unread "alert" notice for one want as read and
// returns how many changed.
func (n *NotificationStore) MarkWantRead(id string) (int, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	entries := n.load()
	changed := 0
	for i := range entries {
		e := &entries[i]
		if e.Kind == "alert" && e.TargetType == "want" && e.TargetID == id && !e.Read {
			e.Read = true
			changed++
		}
	}
	if changed == 0 {
		return 0, nil
	}
	return changed, n.save(entries)
}
