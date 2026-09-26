// Package labels is how anything in mywant carries labels: wants, things,
// kata, characters, want types and recipes alike.
//
// A label is a key and a string value on an object, and what differs between
// objects is only where the map lives and who else touches it at the same
// time. So the handling is here once, in three pieces:
//
//   - plain functions on a map (Clone, Matches, Key, NameOf) for labels nobody
//     writes concurrently — a want type's, a recipe's, a selector;
//   - Guarded, for a map that lives inside another struct and is protected by
//     that struct's own lock — a want's Metadata.Labels under its
//     metadataMutex;
//   - FileStore, for labels kept apart from the objects they describe, by id,
//     in a YAML file — things, kata.
//
// The rule every piece keeps: a live map never leaves its lock. Reads hand
// back copies, and writes go through a method that takes the lock. Ranging
// over a map that another goroutine writes is not a data race Go tolerates —
// the runtime aborts the whole process ("concurrent map read and map
// write"), and recover() cannot catch it.
package labels

import (
	"maps"
	"strings"
	"sync"
)

// Clone copies a label map. A nil map clones to an empty one, so callers can
// write to the result without checking.
func Clone(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	maps.Copy(out, m)
	return out
}

// With returns a copy of m with key set to value — copy-on-write, for labels
// kept in a struct that is handed out by (shallow) copy, where writing the map
// in place would write every copy's map too.
func With(m map[string]string, key, value string) map[string]string {
	out := Clone(m)
	out[key] = value
	return out
}

// Without returns a copy of m without the keys — With's counterpart.
func Without(m map[string]string, keys ...string) map[string]string {
	out := Clone(m)
	for _, k := range keys {
		delete(out, k)
	}
	return out
}

// Matches reports whether labels carry every key=value in selector. An empty
// selector matches everything, and a selector value of "" also matches a key
// that is absent — the rule the chain builder's `using` has always applied.
func Matches(labels, selector map[string]string) bool {
	for k, v := range selector {
		if labels[k] != v {
			return false
		}
	}
	return true
}

// Key builds a namespaced label key, "prefix/name" — constellation/中央線.
func Key(prefix, name string) string { return prefix + "/" + name }

// NameOf is Key's inverse: the name in a "prefix/name" key, or "" when the
// key is not under that prefix.
func NameOf(prefix, key string) string {
	if name, ok := strings.CutPrefix(key, prefix+"/"); ok {
		return name
	}
	return ""
}

// Guarded is a label map protected by a lock that belongs to something else —
// the want whose Metadata.Labels it is. It holds the map by pointer, so
// Replace can swap it and a nil map is created on first write.
//
// onChange, when set, runs after every write while the lock is still held —
// for bookkeeping that has to move with the labels (a want's persistence
// epoch). It must not take the same lock.
type Guarded struct {
	mu       *sync.RWMutex
	m        *map[string]string
	onChange func()
}

// Guard wraps a map and the lock that protects it.
func Guard(mu *sync.RWMutex, m *map[string]string, onChange func()) Guarded {
	return Guarded{mu: mu, m: m, onChange: onChange}
}

// All returns a copy of the labels.
func (g Guarded) All() map[string]string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return Clone(*g.m)
}

// Get returns one label.
func (g Guarded) Get(key string) (string, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	v, ok := (*g.m)[key]
	return v, ok
}

// Matches reports whether the labels carry every key=value in selector.
func (g Guarded) Matches(selector map[string]string) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return Matches(*g.m, selector)
}

// Set writes one label.
func (g Guarded) Set(key, value string) {
	g.Update(func(m map[string]string) { m[key] = value })
}

// SetMany merges labels in.
func (g Guarded) SetMany(labels map[string]string) {
	if len(labels) == 0 {
		return
	}
	g.Update(func(m map[string]string) { maps.Copy(m, labels) })
}

// Delete removes labels.
func (g Guarded) Delete(keys ...string) {
	g.Update(func(m map[string]string) {
		for _, k := range keys {
			delete(m, k)
		}
	})
}

// Replace swaps the whole map for a copy of labels.
func (g Guarded) Replace(labels map[string]string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	*g.m = Clone(labels)
	if g.onChange != nil {
		g.onChange()
	}
}

// Update runs fn on the live map under the write lock — for a change made of
// several steps that must be seen together. fn must not keep the map.
func (g Guarded) Update(fn func(m map[string]string)) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if *g.m == nil {
		*g.m = make(map[string]string)
	}
	fn(*g.m)
	if g.onChange != nil {
		g.onChange()
	}
}
