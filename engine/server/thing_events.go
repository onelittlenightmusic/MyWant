package server

import (
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// Memo event sources — how a value came to be recorded in the memo.
const (
	MemoSourceWantParam       = "want-param"      // a want parameter with a subType (ThingHook)
	ThingSourceAuraDefinition = "aura-definition" // a catalog definition named via aura-defaults
	MemoSourceCardName        = "card-name"       // pressing X on a card to name its final result
	// A name taken back. The ledger only ever grows, so un-naming something is
	// another line in it rather than the removal of an earlier one — which is
	// what lets the whole history stay readable after the fact.
	ThingSourceAuraDefinitionRemoved = "aura-definition-removed"
)

// maxMemoEvents caps the on-disk event log. Oldest events are dropped first.
const maxMemoEvents = 2000

// ThingEvent is one provenance record: a named value entering (or re-entering)
// the memo, with when it happened and which want / character produced it.
type ThingEvent struct {
	At      string `json:"at"      yaml:"at"`      // RFC3339 timestamp
	Catalog string `json:"catalog" yaml:"catalog"` // memo.yaml section key, e.g. "places"
	Subtype string `json:"subtype" yaml:"subtype"` // data subtype/kind, e.g. "place"
	Value   string `json:"value"   yaml:"value"`   // the NAME given, e.g. "会社", "run"
	Source  string `json:"source"  yaml:"source"`  // one of MemoSource*

	// NamedValue is what the name was given TO — the image URL behind "run", the
	// coordinate behind "会社". Value above is only the name, so without this the
	// ledger cannot say what was actually remembered, and the value lives
	// nowhere but the character who signed the mark. A thing is meant to be the
	// record, not a label pointing at one.
	//
	// `any` because a named value is a coordinate object as often as it is a
	// string. Absent for sources that name a value with no separate subject
	// (a want parameter IS its own value).
	NamedValue any `json:"namedValue,omitempty" yaml:"namedValue,omitempty"`

	WantID   string `json:"wantId,omitempty"   yaml:"wantId,omitempty"`
	WantType string `json:"wantType,omitempty" yaml:"wantType,omitempty"`

	CharacterID   string `json:"characterId,omitempty"   yaml:"characterId,omitempty"`
	CharacterName string `json:"characterName,omitempty" yaml:"characterName,omitempty"`
}

// ThingEventStore appends provenance events to ~/.mywant/memo-events.yaml.
// The memo itself (memo.yaml) only keeps the deduplicated value list; this store
// keeps the timeline of when/where each value was named. Thread-safe.
type ThingEventStore struct {
	path string
	mu   sync.Mutex
}

func newThingEventStore() *ThingEventStore {
	return &ThingEventStore{path: thingPath("thing-events.yaml")}
}

func (m *ThingEventStore) load() []ThingEvent {
	bytes, err := os.ReadFile(m.path)
	if err != nil {
		return nil
	}
	var events []ThingEvent
	_ = yaml.Unmarshal(bytes, &events)
	return events
}

func (m *ThingEventStore) save(events []ThingEvent) error {
	if err := os.MkdirAll(filepath.Dir(m.path), 0o755); err != nil {
		return err
	}
	bytes, err := yaml.Marshal(events)
	if err != nil {
		return err
	}
	return os.WriteFile(m.path, bytes, 0o644)
}

// Record appends one event, stamping the time if unset. Newest events are stored
// last on disk; readers reverse for most-recent-first.
func (m *ThingEventStore) Record(ev ThingEvent) error {
	if ev.Value == "" || ev.Catalog == "" {
		return nil
	}
	if ev.At == "" {
		ev.At = time.Now().Format(time.RFC3339)
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	events := m.load()
	events = append(events, ev)
	if len(events) > maxMemoEvents {
		events = events[len(events)-maxMemoEvents:]
	}
	return m.save(events)
}

// All returns every event, most-recent first, capped at limit (0 = no cap).
func (m *ThingEventStore) All(limit int) []ThingEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	return reverseCap(m.load(), limit)
}

// ForValue returns events for one (catalog, value) pair, most-recent first,
// capped at limit (0 = no cap).
func (m *ThingEventStore) ForValue(catalog, value string, limit int) []ThingEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	all := m.load()
	filtered := make([]ThingEvent, 0)
	for _, ev := range all {
		if ev.Catalog == catalog && ev.Value == value {
			filtered = append(filtered, ev)
		}
	}
	return reverseCap(filtered, limit)
}

// WantTypeCount is a want type and how many times a value was used with it.
type WantTypeCount struct {
	Type  string `json:"type"`
	Count int    `json:"count"`
}

// MemoStat is the per-value usage summary derived from the event log.
type MemoStat struct {
	Count        int             `json:"count"`        // number of recorded uses
	LastUsed     string          `json:"lastUsed"`     // RFC3339 timestamp of the most recent use
	TopWantTypes []WantTypeCount `json:"topWantTypes"` // most-used want types, desc (up to 3)
}

// Stats aggregates the event log into per-(catalog,value) usage counts, last use
// time, and the top want types the value was used with. RFC3339 timestamps (same
// local zone) sort lexicographically, so a string max gives the most recent.
func (m *ThingEventStore) Stats() map[string]map[string]MemoStat {
	m.mu.Lock()
	defer m.mu.Unlock()

	type acc struct {
		count  int
		last   string
		byType map[string]int
	}
	agg := make(map[string]map[string]*acc)
	for _, ev := range m.load() {
		if ev.Catalog == "" || ev.Value == "" {
			continue
		}
		byVal := agg[ev.Catalog]
		if byVal == nil {
			byVal = make(map[string]*acc)
			agg[ev.Catalog] = byVal
		}
		a := byVal[ev.Value]
		if a == nil {
			a = &acc{byType: make(map[string]int)}
			byVal[ev.Value] = a
		}
		a.count++
		if ev.At > a.last {
			a.last = ev.At
		}
		if ev.WantType != "" {
			a.byType[ev.WantType]++
		}
	}

	out := make(map[string]map[string]MemoStat, len(agg))
	for cat, byVal := range agg {
		out[cat] = make(map[string]MemoStat, len(byVal))
		for val, a := range byVal {
			out[cat][val] = MemoStat{Count: a.count, LastUsed: a.last, TopWantTypes: topWantTypes(a.byType, 3)}
		}
	}
	return out
}

// topWantTypes returns the n most-used want types, count desc then name asc.
func topWantTypes(byType map[string]int, n int) []WantTypeCount {
	list := make([]WantTypeCount, 0, len(byType))
	for t, c := range byType {
		list = append(list, WantTypeCount{Type: t, Count: c})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Count != list[j].Count {
			return list[i].Count > list[j].Count
		}
		return list[i].Type < list[j].Type
	})
	if len(list) > n {
		list = list[:n]
	}
	return list
}

// reverseCap returns src reversed (newest first) and truncated to limit.
func reverseCap(src []ThingEvent, limit int) []ThingEvent {
	out := make([]ThingEvent, 0, len(src))
	for i := len(src) - 1; i >= 0; i-- {
		out = append(out, src[i])
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

// ── Definitions: the named values, read back out of the ledger ───────────────

// ThingDefinition is one name currently in force: what it is called, what it
// was given to, and who signed it.
//
// Derived from the ledger rather than stored: the ledger is the record, and a
// definition is just its latest word on one (character, kind, name). That also
// means the whole history behind a name stays available — the same events, read
// without the collapsing.
type ThingDefinition struct {
	Catalog string `json:"catalog"`
	Subtype string `json:"subtype"`
	Name    string `json:"name"`
	Value   any    `json:"value,omitempty"`
	At      string `json:"at"`

	CharacterID   string `json:"characterId,omitempty"`
	CharacterName string `json:"characterName,omitempty"`

	WantID   string `json:"wantId,omitempty"`
	WantType string `json:"wantType,omitempty"`
}

// Definitions collapses the ledger to the names in force right now.
//
// Last line wins, per (character, subtype, name): a rename writes a new line,
// and taking a name back writes a removal that simply leaves nothing behind.
// Events are stored oldest-first, so a forward pass gets this for free.
func (m *ThingEventStore) Definitions() []ThingDefinition {
	m.mu.Lock()
	defer m.mu.Unlock()

	type key struct{ char, subtype, name string }
	latest := map[key]ThingDefinition{}
	order := []key{}

	for _, ev := range m.load() {
		if ev.Source != ThingSourceAuraDefinition && ev.Source != ThingSourceAuraDefinitionRemoved {
			continue
		}
		k := key{ev.CharacterID, ev.Subtype, ev.Value}
		if _, seen := latest[k]; !seen {
			order = append(order, k)
		}
		if ev.Source == ThingSourceAuraDefinitionRemoved {
			delete(latest, k)
			continue
		}
		latest[k] = ThingDefinition{
			Catalog:       ev.Catalog,
			Subtype:       ev.Subtype,
			Name:          ev.Value,
			Value:         ev.NamedValue,
			At:            ev.At,
			CharacterID:   ev.CharacterID,
			CharacterName: ev.CharacterName,
			WantID:        ev.WantID,
			WantType:      ev.WantType,
		}
	}

	out := make([]ThingDefinition, 0, len(latest))
	for _, k := range order {
		if d, ok := latest[k]; ok {
			out = append(out, d)
		}
	}
	return out
}
