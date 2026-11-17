package labels

import (
	"os"
	"path/filepath"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// FileStore keeps labels by object id in one YAML file — for objects whose
// labels are not part of the object itself: a thing (a value in a catalog), a
// kata (a bundled definition the user does not own).
//
// What is on disk is always the truth: a hand edit is picked up on the next
// call. The file is parsed again only when it has changed (its size or
// modification time), not on every call — the board's thing motion asks for
// every label four times a second, and parsing the world's label file each
// time was the most CPU the idle server spent (2026-11-17: 37% of it, the file
// 56KB). Written on every change. Every read hands back copies.
type FileStore struct {
	mu   sync.Mutex
	path string

	cached fileData // as last read or written; never handed out
	stamp  fileStamp
}

// fileStamp is what the file looked like when cached was read or written.
type fileStamp struct {
	size int64
	mod  time.Time
	ok   bool
}

func stampOf(path string) fileStamp {
	fi, err := os.Stat(path)
	if err != nil {
		return fileStamp{}
	}
	return fileStamp{size: fi.Size(), mod: fi.ModTime(), ok: true}
}

type fileData map[string]map[string]string

// NewFileStore returns a store backed by path. The file need not exist yet.
func NewFileStore(path string) *FileStore { return &FileStore{path: path} }

// SetPath moves the store to another file (tests point it at a temp dir).
func (s *FileStore) SetPath(p string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.path = p
	s.cached, s.stamp = nil, fileStamp{}
}

// load is the file's labels — the cached ones, read again only when the file
// changed. Callers must not change what it returns (update copies it first).
func (s *FileStore) load() (fileData, error) {
	now := stampOf(s.path)
	if s.cached != nil && now.ok && s.stamp.ok && now.size == s.stamp.size && now.mod.Equal(s.stamp.mod) {
		return s.cached, nil
	}
	data := make(fileData)
	b, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		s.cached, s.stamp = nil, fileStamp{}
		return data, nil
	}
	if err != nil {
		s.cached, s.stamp = nil, fileStamp{}
		return data, err
	}
	_ = yaml.Unmarshal(b, &data)
	if data == nil {
		data = make(fileData)
	}
	// The stamp taken before the read: a write in between is read again next time.
	s.cached, s.stamp = data, now
	return data, nil
}

func (s *FileStore) save(data fileData) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	b, err := yaml.Marshal(data)
	if err != nil {
		return err
	}
	s.cached, s.stamp = nil, fileStamp{}
	if err := os.WriteFile(s.path, b, 0o644); err != nil {
		return err
	}
	s.cached, s.stamp = data, stampOf(s.path)
	return nil
}

// All returns every object's labels, copied.
func (s *FileStore) All() map[string]map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, _ := s.load()
	out := make(map[string]map[string]string, len(data))
	for id, l := range data {
		out[id] = Clone(l)
	}
	return out
}

// Get returns one object's labels, copied (empty when it has none).
func (s *FileStore) Get(id string) map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, _ := s.load()
	return Clone(data[id])
}

// Set writes one label on one object.
func (s *FileStore) Set(id, key, value string) error {
	if id == "" || key == "" {
		return nil
	}
	return s.update(func(data fileData) bool {
		if data[id] == nil {
			data[id] = make(map[string]string)
		}
		data[id][key] = value
		return true
	})
}

// Remove deletes one label from one object, dropping the object when it has
// none left.
func (s *FileStore) Remove(id, key string) error {
	return s.update(func(data fileData) bool {
		l, ok := data[id]
		if !ok {
			return false
		}
		delete(l, key)
		if len(l) == 0 {
			delete(data, id)
		}
		return true
	})
}

// IDsWithLabel returns the ids of objects carrying the key.
func (s *FileStore) IDsWithLabel(key string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, _ := s.load()
	var out []string
	for id, l := range data {
		if _, ok := l[key]; ok {
			out = append(out, id)
		}
	}
	return out
}

// Rekey moves labels from old ids to new ones. When the new id already has
// labels, the old ones fill in only the keys it lacks.
func (s *FileStore) Rekey(mapping map[string]string) error {
	if len(mapping) == 0 {
		return nil
	}
	return s.update(func(data fileData) bool {
		changed := false
		for from, to := range mapping {
			l, ok := data[from]
			if !ok || from == to {
				continue
			}
			if existing, ok := data[to]; ok {
				for k, v := range l {
					if _, taken := existing[k]; !taken {
						existing[k] = v
					}
				}
			} else {
				data[to] = l
			}
			delete(data, from)
			changed = true
		}
		return changed
	})
}

// update loads, applies fn and saves when fn reports a change — the one
// read-modify-write every writer goes through, under the one lock.
func (s *FileStore) update(fn func(data fileData) bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	loaded, err := s.load()
	if err != nil {
		return err
	}
	// A copy changed: the cache stays as the file is if fn or the write fails.
	data := make(fileData, len(loaded))
	for id, l := range loaded {
		data[id] = Clone(l)
	}
	if !fn(data) {
		return nil
	}
	return s.save(data)
}
