package mywant

import (
	"encoding/json"
	"sync/atomic"
)

// ringBuffer is a fixed-capacity FIFO ring buffer for append-mostly concurrent data
// (e.g., notification history, API logs).
//
// Appends are lock-free: each writer atomically claims a unique slot index via
// atomic.Add, then stores the value using atomic.Value, so multiple concurrent
// writers never touch the same slot simultaneously.
//
// Snapshots read each slot via atomic.Value.Load, which is safe to call
// concurrently with Store on the same slot.
type ringBuffer[T any] struct {
	buf  []atomic.Value // pre-allocated fixed-size slots
	cap_ int64
	head atomic.Int64 // monotonically increasing write count
}

func newRingBuffer[T any](capacity int) *ringBuffer[T] {
	return &ringBuffer[T]{
		buf:  make([]atomic.Value, capacity),
		cap_: int64(capacity),
	}
}

// Append adds v to the ring buffer, overwriting the oldest entry when full.
// Lock-free and safe for concurrent use.
func (r *ringBuffer[T]) Append(v T) {
	idx := r.head.Add(1) - 1
	r.buf[idx%r.cap_].Store(v)
}

// Snapshot returns the most recent entries in FIFO order.
// Pass limit <= 0 to get all available entries (up to capacity).
func (r *ringBuffer[T]) Snapshot(limit int) []T {
	total := r.head.Load()
	if total == 0 {
		return nil
	}
	size := total
	if size > r.cap_ {
		size = r.cap_
	}
	if limit > 0 && int64(limit) < size {
		size = int64(limit)
	}
	start := total - size
	result := make([]T, 0, size)
	for i := int64(0); i < size; i++ {
		v := r.buf[(start+i)%r.cap_].Load()
		if v != nil {
			result = append(result, v.(T))
		}
	}
	return result
}

// PeekLast returns the most recently appended entry without consuming it.
// Returns (zero, false) if the buffer is empty.
func (r *ringBuffer[T]) PeekLast() (T, bool) {
	total := r.head.Load()
	if total == 0 {
		var zero T
		return zero, false
	}
	v := r.buf[(total-1)%r.cap_].Load()
	if v == nil {
		var zero T
		return zero, false
	}
	return v.(T), true
}

// UpdateLast atomically replaces the most recently appended entry.
// fn receives a pointer to a copy of the entry; modifications are stored back.
// Returns false if the buffer is empty.
// Note: not safe against concurrent Append on the same slot (suitable for
// single-writer or best-effort history merging scenarios).
func (r *ringBuffer[T]) UpdateLast(fn func(*T)) bool {
	total := r.head.Load()
	if total == 0 {
		return false
	}
	idx := (total - 1) % r.cap_
	v := r.buf[idx].Load()
	if v == nil {
		return false
	}
	entry := v.(T)
	fn(&entry)
	r.buf[idx].Store(entry)
	return true
}


// Clear resets the ring buffer. Not safe to call concurrently with Append.
func (r *ringBuffer[T]) Clear() {
	r.head.Store(0)
	for i := range r.buf {
		r.buf[i] = atomic.Value{}
	}
}

// Len returns the number of entries currently stored (capped at capacity).
func (r *ringBuffer[T]) Len() int {
	total := r.head.Load()
	if total > r.cap_ {
		return int(r.cap_)
	}
	return int(total)
}

// MarshalJSON implements json.Marshaler, serializing as a JSON array.
// A nil ring buffer marshals as an empty array.
func (r *ringBuffer[T]) MarshalJSON() ([]byte, error) {
	if r == nil {
		return []byte("[]"), nil
	}
	return json.Marshal(r.Snapshot(0))
}

// UnmarshalJSON implements json.Unmarshaler, populating from a JSON array.
func (r *ringBuffer[T]) UnmarshalJSON(data []byte) error {
	var items []T
	if err := json.Unmarshal(data, &items); err != nil {
		return err
	}
	for _, item := range items {
		r.Append(item)
	}
	return nil
}
