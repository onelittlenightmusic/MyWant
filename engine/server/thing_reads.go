package server

import (
	"os"
	"path/filepath"
	"sync"
	"time"

	mywant "mywant/engine/core"

	"gopkg.in/yaml.v3"
)

// ThingRead is when a thing was last read by a want — its name in a parameter
// of a want whose Progress ran — and as what: "transit_search.from". Not when
// it was named (the event log has that), but when it was in use; a keyboard
// puts what was read lately first, and what was read as `from` first in a
// `from`.
type ThingRead struct {
	LastRead string `yaml:"lastRead" json:"lastRead"`
	// The last time it was read as each "<want type>.<parameter>".
	ReadAs map[string]string `yaml:"readAs,omitempty" json:"readAs,omitempty"`
}

// ThingReadStore keeps the reads, by the thing's name, in memory and in
// <world>-reads.yaml. Marked at each reconcile (its READ phase, OnProgressed),
// written on a goroutine of its own, never in the reconcile.
type ThingReadStore struct {
	mu    sync.Mutex
	path  string
	reads map[string]*ThingRead
	dirty bool
	// One write at a time, outside mu.
	writing sync.Mutex
}

func newThingReadStore() *ThingReadStore {
	s := &ThingReadStore{path: thingPath("thing-reads.yaml")}
	s.reads = s.read(s.path)
	return s
}

func (s *ThingReadStore) read(path string) map[string]*ThingRead {
	out := map[string]*ThingRead{}
	if data, err := os.ReadFile(path); err == nil {
		_ = yaml.Unmarshal(data, &out)
	}
	return out
}

// SetPath moves to another world's file: what is unwritten goes to the old
// one first.
func (s *ThingReadStore) SetPath(path string) {
	s.Flush()
	reads := s.read(path)
	s.mu.Lock()
	s.path, s.reads, s.dirty = path, reads, false
	s.mu.Unlock()
}

// Mark records `name` as read as `as` at `at` (to the minute; never earlier
// than what is there).
func (s *ThingReadStore) Mark(name, as string, at time.Time) {
	if name == "" {
		return
	}
	stamp := at.Truncate(time.Minute).Format(time.RFC3339)
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.reads[name]
	if r == nil {
		r = &ThingRead{}
		s.reads[name] = r
	}
	if stamp > r.LastRead {
		r.LastRead = stamp
		s.dirty = true
	}
	if as != "" {
		if r.ReadAs == nil {
			r.ReadAs = map[string]string{}
		}
		if stamp > r.ReadAs[as] {
			r.ReadAs[as] = stamp
			s.dirty = true
		}
	}
}

// Get is a copy of what is known of `name`.
func (s *ThingReadStore) Get(name string) (ThingRead, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.reads[name]
	if !ok {
		return ThingRead{}, false
	}
	c := ThingRead{LastRead: r.LastRead}
	if len(r.ReadAs) > 0 {
		c.ReadAs = make(map[string]string, len(r.ReadAs))
		for k, v := range r.ReadAs {
			c.ReadAs[k] = v
		}
	}
	return c, true
}

// Flush writes what changed since the last write.
func (s *ThingReadStore) Flush() {
	s.writing.Lock()
	defer s.writing.Unlock()
	s.mu.Lock()
	if !s.dirty {
		s.mu.Unlock()
		return
	}
	data, err := yaml.Marshal(s.reads)
	path := s.path
	s.dirty = false
	s.mu.Unlock()
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	tmp := path + ".tmp"
	if os.WriteFile(tmp, data, 0o644) == nil {
		_ = os.Rename(tmp, path)
	}
}

// markThingReads is the reconcile's READ phase for things: each want that
// progressed has read the things its parameters name — those whose type
// gives them a subtype, as ThingHook records them — as "<type>.<param>", at
// the time its Progress ran. Quick, in memory; written on its own goroutine.
func (s *Server) markThingReads(progressed []*mywant.Want) {
	if s.thingReads == nil || s.globalBuilder == nil {
		return
	}
	for _, w := range progressed {
		typ := w.Metadata.Type
		def := s.globalBuilder.GetWantTypeDefinition(typ)
		if def == nil {
			continue
		}
		at := w.ProgressedAt()
		for _, pd := range def.Parameters {
			if pd.SubType == "" {
				continue
			}
			val, ok := w.GetParameter(pd.Name)
			if !ok {
				continue
			}
			for _, name := range stringValues(val) {
				s.thingReads.Mark(name, typ+"."+pd.Name, at)
			}
		}
	}
	go s.thingReads.Flush()
}
