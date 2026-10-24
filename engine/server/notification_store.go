package server

import (
	"os"
	"path/filepath"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	mywant "mywant/engine/core"
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

	// Mood is how a want's alert feels, when its type's voice said it
	// (mywant.WantNewsVoice): the same moods the tile's own line has.
	Mood string `json:"mood,omitempty" yaml:"mood,omitempty"`

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

	// Output is the answer this alert announces, when it announces one — the
	// very entry the want's history keeps (see mywant.OnWantOutput), not a
	// description of it. Message is only that answer said in a sentence, for a
	// push notification that cannot draw one.
	Output *mywant.ResultHistoryEntry `json:"output,omitempty" yaml:"output,omitempty"`
}

// NotificationStore appends notices to ~/.mywant/notifications.yaml.
// Mirrors ThingEventStore: load-modify-save under a mutex, newest last on disk,
// readers reverse for most-recent-first. Thread-safe.
//
// The log runs to maxNotifications entries — hundreds of KB of YAML — and the
// board asks for its unread counts on every poll, so what was last read or
// written is kept beside the file's stamp (fileStamp) and parsed again only
// when the file changes.
type NotificationStore struct {
	path string
	mu   sync.Mutex

	cached []NotificationEntry
	stamp  fileStamp
}

func newNotificationStore() *NotificationStore {
	home, _ := os.UserHomeDir()
	return &NotificationStore{path: filepath.Join(home, ".mywant", "notifications.yaml")}
}

// load is the log, oldest first — a copy, since callers edit it and save it back.
func (n *NotificationStore) load() []NotificationEntry {
	if !n.stamp.matches(n.path) {
		stamp := stampOf(n.path) // before reading: a write in between reads again next time
		bytes, err := os.ReadFile(n.path)
		if err != nil {
			n.cached, n.stamp = nil, fileStamp{}
			return nil
		}
		var entries []NotificationEntry
		_ = yaml.Unmarshal(bytes, &entries)
		n.cached, n.stamp = entries, stamp
	}
	return append([]NotificationEntry(nil), n.cached...)
}

func (n *NotificationStore) save(entries []NotificationEntry) error {
	if err := os.MkdirAll(filepath.Dir(n.path), 0o755); err != nil {
		return err
	}
	bytes, err := yaml.Marshal(entries)
	if err != nil {
		return err
	}
	n.stamp = fileStamp{}
	if err := os.WriteFile(n.path, bytes, 0o644); err != nil {
		return err
	}
	n.cached, n.stamp = append([]NotificationEntry(nil), entries...), stampOf(n.path)
	return nil
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

// UnreadWantOutputs returns, per want id, the ids of the outputs its unread
// alerts announce — newest last, the order they were raised in. The canvas
// draws a want's recent outputs and lights up the ones in this list.
func (n *NotificationStore) UnreadWantOutputs() map[string][]string {
	n.mu.Lock()
	defer n.mu.Unlock()
	out := map[string][]string{}
	for _, e := range n.load() {
		if e.Kind == "alert" && e.TargetType == "want" && e.TargetID != "" && !e.Read && e.Output != nil && e.Output.ID != "" {
			out[e.TargetID] = append(out[e.TargetID], e.Output.ID)
		}
	}
	return out
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

// LatestUnreadAlert is the newest alert on one want not yet read, or nil.
func (n *NotificationStore) LatestUnreadAlert(id string) *NotificationEntry {
	n.mu.Lock()
	defer n.mu.Unlock()
	all := n.load()
	for i := len(all) - 1; i >= 0; i-- {
		e := all[i]
		if e.Kind == "alert" && e.TargetType == "want" && e.TargetID == id && !e.Read {
			return &e
		}
	}
	return nil
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
