package server

import (
	_ "embed"

	"github.com/google/uuid"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

//go:embed datatypes.yaml
var datatypesYAML []byte

// dataTypeDefs is loaded once at init from the embedded datatypes.yaml.
var dataTypeDefs map[string]DataTypeInfo

func init() {
	dataTypeDefs = make(map[string]DataTypeInfo)
	_ = yaml.Unmarshal(datatypesYAML, &dataTypeDefs)
}

// ThingStore persists user-entered values to ~/.mywant/thing.yaml.
// Thread-safe; reads and writes are serialised via a mutex.
type ThingStore struct {
	path string
	mu   sync.Mutex
}

// thingData is the catalog VIEW of the store: catalog key → values, most
// recent first. It is what most callers want — suggestions for a subtype, the
// values a kata evaluates, the catalogs usage is derived against — and none of
// those questions are about identity, so they are still answered this way.
type thingData map[string][]string

// ThingEntry is a thing as stored: an identity, and what is currently true
// about it.
//
// The id used to be the catalog and the value joined, which made identity a
// consequence of two things that can both change. Everything that referred to
// a thing — where it sits on the board, which group it is in — hung off that
// id, so changing a thing's category silently destroyed one thing and created
// another, and every reference was left pointing at something that no longer
// existed. A tile simply vanished from the canvas and nothing said why.
//
// The id is now a UUID and means nothing at all, which is the point: nothing
// about a thing can change in a way that changes what it IS.
type ThingEntry struct {
	ID      string `yaml:"id"      json:"id"`
	Catalog string `yaml:"catalog" json:"catalog"`
	Value   string `yaml:"value"   json:"value"`
}

// thingFile is the on-disk YAML schema. Version 1 was the bare catalog map and
// is migrated on first read — see migrateThingFile.
type thingFile struct {
	Version int          `yaml:"version"`
	Things  []ThingEntry `yaml:"things"`
}

const thingSchemaVersion = 2

func newThingStore() *ThingStore {
	return &ThingStore{path: thingPath("thing.yaml")}
}

// SetPath moves the store to another file. Every read and write goes through
// `path` at the moment it happens, so this is the whole of "things belong to a
// world": point it at the open world's file and the saves the store already
// performs are saves into that world. See world_things.go.
func (m *ThingStore) SetPath(p string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.path = p
}

// loadEntries reads the store as it is stored: a list of identified things.
func (m *ThingStore) loadEntries() ([]ThingEntry, error) {
	bytes, err := os.ReadFile(m.path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var file thingFile
	if err := yaml.Unmarshal(bytes, &file); err == nil && file.Version >= thingSchemaVersion {
		return file.Things, nil
	}
	// Version 1: the bare catalog map. Read it as such and mint identities.
	// Not written back here — migrateThingStore does that once, at startup,
	// together with the label remap that has to happen in the same breath.
	var legacy thingData
	if err := yaml.Unmarshal(bytes, &legacy); err != nil {
		return nil, err
	}
	return entriesFromCatalogs(legacy), nil
}

// load returns the catalog view, most-recent-first within each catalog, which
// is the order the list form preserves.
func (m *ThingStore) load() (thingData, error) {
	entries, err := m.loadEntries()
	if err != nil {
		return make(thingData), err
	}
	return catalogsFromEntries(entries), nil
}

func (m *ThingStore) saveEntries(entries []ThingEntry) error {
	if err := os.MkdirAll(filepath.Dir(m.path), 0o755); err != nil {
		return err
	}
	bytes, err := yaml.Marshal(thingFile{Version: thingSchemaVersion, Things: entries})
	if err != nil {
		return err
	}
	return os.WriteFile(m.path, bytes, 0o644)
}

// save takes the catalog view and writes it back, keeping the id of anything
// already stored under the same catalog and value. Callers that work in the
// catalog view are saying what exists, not who it is, so identity is preserved
// here rather than being quietly reissued.
func (m *ThingStore) save(data thingData) error {
	existing, _ := m.loadEntries()
	byPair := make(map[string]string, len(existing))
	for _, e := range existing {
		byPair[e.Catalog+"\x00"+e.Value] = e.ID
	}
	var out []ThingEntry
	for _, catalog := range sortedKeys(data) {
		for _, v := range data[catalog] {
			if v == "" {
				continue
			}
			id := byPair[catalog+"\x00"+v]
			if id == "" {
				id = newThingID()
			}
			out = append(out, ThingEntry{ID: id, Catalog: catalog, Value: v})
		}
	}
	return m.saveEntries(out)
}

func (m *ThingStore) saveLegacy(data thingData) error {
	if err := os.MkdirAll(filepath.Dir(m.path), 0o755); err != nil {
		return err
	}
	bytes, err := yaml.Marshal(data)
	if err != nil {
		return err
	}
	return os.WriteFile(m.path, bytes, 0o644)
}

// ── entries ↔ catalogs ───────────────────────────────────────────────────────

func newThingID() string {
	return "thg-" + uuid.NewString()
}

func sortedKeys(data thingData) []string {
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// entriesFromCatalogs mints an identity for every value in the catalog map,
// preserving the order within each catalog (which is most-recent-first).
func entriesFromCatalogs(data thingData) []ThingEntry {
	var out []ThingEntry
	for _, catalog := range sortedKeys(data) {
		for _, v := range data[catalog] {
			if v == "" {
				continue
			}
			out = append(out, ThingEntry{ID: newThingID(), Catalog: catalog, Value: v})
		}
	}
	return out
}

func catalogsFromEntries(entries []ThingEntry) thingData {
	data := make(thingData)
	for _, e := range entries {
		if e.Value == "" {
			continue
		}
		data[e.Catalog] = append(data[e.Catalog], e.Value)
	}
	return data
}

// Entries returns every thing as stored, identity included.
func (m *ThingStore) Entries() []ThingEntry {
	m.mu.Lock()
	defer m.mu.Unlock()
	entries, _ := m.loadEntries()
	out := make([]ThingEntry, len(entries))
	copy(out, entries)
	return out
}

// Update changes a thing's catalog and/or value, keeping its identity — the
// whole point of the identity being a UUID. Empty fields mean "leave alone".
// Returns the entry as it now stands.
func (m *ThingStore) Update(id, catalog, value string) (ThingEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	entries, err := m.loadEntries()
	if err != nil {
		return ThingEntry{}, err
	}
	var updated ThingEntry
	found := false
	for i, e := range entries {
		if e.ID != id {
			continue
		}
		if catalog != "" {
			entries[i].Catalog = catalog
		}
		if value != "" {
			entries[i].Value = value
		}
		updated, found = entries[i], true
		break
	}
	if !found {
		return ThingEntry{}, os.ErrNotExist
	}
	// A move onto a pair that already exists would leave two things claiming to
	// be the same value in the same catalog, which every catalog-view caller
	// would then see twice.
	for _, e := range entries {
		if e.ID != updated.ID && e.Catalog == updated.Catalog && e.Value == updated.Value {
			return ThingEntry{}, os.ErrExist
		}
	}
	return updated, m.saveEntries(entries)
}

// Delete removes one thing by identity.
func (m *ThingStore) Delete(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	entries, err := m.loadEntries()
	if err != nil {
		return err
	}
	out := entries[:0]
	for _, e := range entries {
		if e.ID != id {
			out = append(out, e)
		}
	}
	return m.saveEntries(out)
}

// Add remembers a value under a catalog and returns its identity, reusing the
// existing one when the pair is already stored.
func (m *ThingStore) Add(catalog, value string) (ThingEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entries, err := m.loadEntries()
	if err != nil {
		return ThingEntry{}, err
	}
	for _, e := range entries {
		if e.Catalog == catalog && e.Value == value {
			return e, nil
		}
	}
	entry := ThingEntry{ID: newThingID(), Catalog: catalog, Value: value}
	return entry, m.saveEntries(append(entries, entry))
}

// Record adds value to the list for subtype, deduplicating and capping at 100 entries.
func (m *ThingStore) Record(subtype, value string) error {
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
func (m *ThingStore) Suggestions(subtype string, limit int) []string {
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
func (m *ThingStore) Replace(data thingData) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.save(data)
}

// GetCategory returns all values stored under the given category key directly
// (no subtype mapping applied). Implements core.MemoReader.
func (m *ThingStore) GetCategory(key string) []string {
	if key == "" {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	data, err := m.load()
	if err != nil {
		return nil
	}
	vals := data[key]
	out := make([]string, len(vals))
	copy(out, vals)
	return out
}

// All returns the full memo data as-is from disk.
func (m *ThingStore) All() thingData {
	m.mu.Lock()
	defer m.mu.Unlock()
	data, _ := m.load()
	return data
}

// AllSubtypes returns all known subtype keys sorted alphabetically.
func (m *ThingStore) AllSubtypes() []string {
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

// DataTypeInfo holds display metadata for a data type (primitive or subtype).
type DataTypeInfo struct {
	Key      string `yaml:"key"       json:"memoKey"`            // memo.yaml section key (e.g. "cities")
	Icon     string `yaml:"icon"      json:"icon"`               // Lucide icon component name
	Color    string `yaml:"color"     json:"color"`              // hex color for UI
	BaseType string `yaml:"base_type" json:"baseType,omitempty"` // parent primitive type; empty = primitive
	// Catalog is the catalog kind a value of this subtype is NAMED into. Usually
	// the subtype itself; set it when the data subtype and the catalog it names
	// into differ — e.g. a location_coordinate value is named as a "place".
	Catalog string `yaml:"catalog" json:"catalog,omitempty"`
}

// DataTypeDefinitions returns a copy of all known data type definitions.
func DataTypeDefinitions() map[string]DataTypeInfo {
	out := make(map[string]DataTypeInfo, len(dataTypeDefs))
	for k, v := range dataTypeDefs {
		out[k] = v
	}
	return out
}

// baseTypeOf is what a subtype ultimately IS — "station" and "location" are
// both strings, "percentage" is a number. Primitives are their own base.
func baseTypeOf(subtype string) string {
	info, ok := dataTypeDefs[subtype]
	if !ok {
		return ""
	}
	if info.BaseType == "" {
		return subtype // a primitive
	}
	return info.BaseType
}

// subtypesInterchangeable reports whether a value of one subtype may be offered
// for a field declaring the other.
//
// A route's `from` is declared as a station, but a place, a landmark or a city
// is just as good a thing to leave from — they are all strings, and the field
// only ever wanted a string with a meaning attached. Requiring the exact
// subtype meant a thing you had already named was invisible to a field that
// could plainly use it.
//
// Deliberately permissive: an exact match still ranks above this wherever the
// two are ordered, so widening what is ACCEPTED does not change what is
// SUGGESTED FIRST.
func subtypesInterchangeable(a, b string) bool {
	if a == b {
		return true
	}
	if a == "" || b == "" {
		return false
	}
	base := baseTypeOf(a)
	return base != "" && base == baseTypeOf(b)
}

// orderedInterchangeable lists the subtypes a field declaring `subtype` will
// take: itself first, then the others sharing its base type, alphabetically so
// the order is the same from one call to the next.
func orderedInterchangeable(subtype string) []string {
	if subtype == "" {
		return nil
	}
	base := baseTypeOf(subtype)
	if base == "" {
		return []string{subtype}
	}
	out := []string{subtype}
	var rest []string
	for name, info := range dataTypeDefs {
		if name == subtype || info.BaseType == "" {
			continue // itself, or a primitive: a bare "string" names nothing
		}
		if info.BaseType == base {
			rest = append(rest, name)
		}
	}
	sort.Strings(rest)
	return append(out, rest...)
}

func subtypeToKey(subtype string) string {
	if info, ok := dataTypeDefs[subtype]; ok {
		return info.Key
	}
	// Default: append "s"
	return subtype + "s"
}

// keyToSubtype is subtypeToKey read backwards: thing.yaml is keyed by catalog
// ("artists") while the data type catalog is keyed by type name ("artist"), so
// anything holding a catalog needs this to name the type again. A real subtype
// wins over a primitive sharing the same key, matching the frontend's
// buildKeyToType.
func keyToSubtype(key string) string {
	best := ""
	for name, info := range dataTypeDefs {
		if info.Key != key {
			continue
		}
		if best == "" || (dataTypeDefs[best].BaseType == "" && info.BaseType != "") {
			best = name
		}
	}
	if best != "" {
		return best
	}
	// Default: undo the "s" subtypeToKey appends.
	return strings.TrimSuffix(key, "s")
}
