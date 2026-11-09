package mywant

import (
	"sync"
	"time"
)

// The memory file's copy of each want, kept between writes.
//
// writeStatsToMemory made every want again for every write — its state deep
// copied, its history built, all of it marshalled — though on a board of fifty
// wants one had usually changed: on fly a location webhook every second, and
// the 3.4MB file rebuilt each time (59ms and 37MB of allocations a write, on a
// laptop). Each want's bytes are now kept with what they were made from, and
// made again only when that has moved:
//   - the configured want (UpdateWant replaces it, a reconcile too),
//   - the running want's persistRev (any write to what the file keeps of it),
//   - its answers' ring (ringBuffer.Version).
// Whatever changes a want and is missed by those is caught by a write made
// whole now and then (memoryRebuildEvery).

// memoryRebuildEvery: a write made whole, the cache not trusted, at least this
// often — what keeps a change no counter saw from staying unwritten for long.
const memoryRebuildEvery = 60 * time.Second

type memoryCacheKey struct {
	config  *Want // the configured want it was made from (nil: running only)
	runtime *Want
	rev     uint64 // runtime.persistRev
	ring    uint64 // runtime's answers' ring version
	asJSON  bool
}

type memoryCacheEntry struct {
	key  memoryCacheKey
	data []byte
}

type memoryCache struct {
	mu      sync.Mutex
	entries map[string]memoryCacheEntry // want id → its bytes in the file
	// When the file was last written whole.
	rebuiltAt time.Time
}

// persistTouched: something the memory file keeps of this want has changed.
func (n *Want) persistTouched() { n.persistRev.Add(1) }

// memoryKeyOf is what a want's bytes in the memory file are made from, read
// before they are made — a change made while they are being made moves it
// again, so the next write makes them again.
func memoryKeyOf(config, runtime *Want, asJSON bool) memoryCacheKey {
	k := memoryCacheKey{config: config, runtime: runtime, asJSON: asJSON}
	if runtime != nil {
		k.rev = runtime.persistRev.Load()
		if h := runtime.history; h != nil && h.ResultHistoryRing != nil {
			k.ring = h.ResultHistoryRing.Version()
		}
	}
	return k
}

// begin starts a write: whether the cache may be used this time (not when a
// whole write is due), and a map to fill with this write's entries — the
// wants gone from the board fall out of the cache with it.
func (c *memoryCache) begin(now time.Time) (useCache bool, next map[string]memoryCacheEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	useCache = c.entries != nil && now.Sub(c.rebuiltAt) < memoryRebuildEvery
	if !useCache {
		c.rebuiltAt = now
	}
	return useCache, make(map[string]memoryCacheEntry, len(c.entries))
}

// get is the want's bytes, if they were made from what it is now.
func (c *memoryCache) get(id string, key memoryCacheKey) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[id]
	if !ok || e.key != key {
		return nil, false
	}
	return e.data, true
}

// finish keeps this write's entries for the next.
func (c *memoryCache) finish(next map[string]memoryCacheEntry) {
	c.mu.Lock()
	c.entries = next
	c.mu.Unlock()
}
