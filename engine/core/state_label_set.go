package mywant

import (
	"encoding/json"
	"sync"
	"sync/atomic"
)

// StateLabelSet is a want's state labels — which of its state keys are goal,
// current, plan or internal — kept as its state is (Want.State): in a
// sync.Map, written and read at once from any goroutine.
//
// It was a plain map, written in place while the want ran (a target's recipe
// labels, a plan's status, a struct field first synced) and read by the
// server's handlers and every SetCurrent at the same time: a concurrent map
// read and write. Copying the map for each label written cost ~16µs for 20
// fields; a sync.Map, ~1µs, each label seen the moment it is set, with no
// moment to choose when to commit. A label is written once and read ever
// after — the use sync.Map is made for.
//
// Replaced whole (a type reloaded) through an atomic pointer, so a reader
// sees the old labels or the new, never half of them. The zero value is
// empty and ready. Written to JSON as the plain map it was; to YAML by
// Want.MarshalYAML, as State is.
type StateLabelSet struct {
	p atomic.Pointer[sync.Map]
}

func (s *StateLabelSet) load() *sync.Map { return s.p.Load() }

// writable is the map to write to, made on the first write.
func (s *StateLabelSet) writable() *sync.Map {
	if m := s.p.Load(); m != nil {
		return m
	}
	s.p.CompareAndSwap(nil, &sync.Map{})
	return s.p.Load()
}

// Get is the key's label, and whether it has one.
func (s *StateLabelSet) Get(key string) (StateLabel, bool) {
	m := s.load()
	if m == nil {
		return LabelNone, false
	}
	v, ok := m.Load(key)
	if !ok {
		return LabelNone, false
	}
	return v.(StateLabel), true
}

// Is: the key is labelled l.
func (s *StateLabelSet) Is(key string, l StateLabel) bool {
	got, ok := s.Get(key)
	return ok && got == l
}

// Has: the key has a label.
func (s *StateLabelSet) Has(key string) bool {
	_, ok := s.Get(key)
	return ok
}

// Set labels the key, seen at once.
func (s *StateLabelSet) Set(key string, l StateLabel) {
	s.writable().Store(key, l)
}

// SetIfAbsent labels the key unless it has a label; the label it has.
func (s *StateLabelSet) SetIfAbsent(key string, l StateLabel) StateLabel {
	v, _ := s.writable().LoadOrStore(key, l)
	return v.(StateLabel)
}

// Range calls fn for each key and its label until fn says false.
func (s *StateLabelSet) Range(fn func(key string, l StateLabel) bool) {
	m := s.load()
	if m == nil {
		return
	}
	m.Range(func(k, v any) bool { return fn(k.(string), v.(StateLabel)) })
}

// Replace puts these labels in place of all there were, in one step.
func (s *StateLabelSet) Replace(labels map[string]StateLabel) {
	m := &sync.Map{}
	for k, l := range labels {
		m.Store(k, l)
	}
	s.p.Store(m)
}

// Map is the labels as a plain map, a copy.
func (s *StateLabelSet) Map() map[string]StateLabel {
	out := map[string]StateLabel{}
	s.Range(func(k string, l StateLabel) bool { out[k] = l; return true })
	return out
}

// Len is how many keys have a label.
func (s *StateLabelSet) Len() int {
	n := 0
	s.Range(func(string, StateLabel) bool { n++; return true })
	return n
}

// IsZero: no labels (omitted from JSON — omitzero — as the empty map was).
func (s *StateLabelSet) IsZero() bool { return s == nil || s.Len() == 0 }

func (s *StateLabelSet) MarshalJSON() ([]byte, error) { return json.Marshal(s.Map()) }

func (s *StateLabelSet) UnmarshalJSON(data []byte) error {
	var m map[string]StateLabel
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	s.Replace(m)
	return nil
}
