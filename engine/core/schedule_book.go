package mywant

import (
	"container/heap"
	"reflect"
	"time"
)

// scheduleBook is every want's schedule, kept by the scheduler (SchedulerWant):
// each want's entries (spec.when, its type's default-every, its own wake), and
// a heap holding each want's earliest — so the scheduler waits for the one
// time that comes first, and looks at nothing else meanwhile.
//
// A want's entries are worked out again only when it is told something about
// it changed (scheduleChanged): it was added or deleted, its spec changed, or
// it wrote its wake_at.
type scheduleBook struct {
	wants map[string]*bookedWant
	h     bookHeap
}

// bookedWant is one want's schedule.
type bookedWant struct {
	name    string
	when    []WhenSpec // as last read, to tell a change from none
	wake    time.Time  // its own wake as last read; zero for none
	entries []ParsedSchedule
	seq     uint64 // bumped on each change: older heap items for it are passed over
}

// bookItem is a want's earliest entry in the heap, as of its seq.
type bookItem struct {
	at  time.Time
	id  string
	seq uint64
}

type bookHeap []bookItem

func (h bookHeap) Len() int           { return len(h) }
func (h bookHeap) Less(i, j int) bool { return h[i].at.Before(h[j].at) }
func (h bookHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *bookHeap) Push(x any)        { *h = append(*h, x.(bookItem)) }
func (h *bookHeap) Pop() any {
	old := *h
	it := old[len(old)-1]
	*h = old[:len(old)-1]
	return it
}

func newScheduleBook() *scheduleBook {
	return &scheduleBook{wants: map[string]*bookedWant{}}
}

// put sets a want's schedule from its when and its wake. Nothing changed,
// nothing done: the times already worked out stand.
func (b *scheduleBook) put(id, name string, when []WhenSpec, wake time.Time, now time.Time) {
	bw := b.wants[id]
	if bw != nil && bw.name == name && reflect.DeepEqual(bw.when, when) && bw.wake.Equal(wake) {
		return
	}
	if bw == nil {
		bw = &bookedWant{}
		b.wants[id] = bw
	}
	bw.name = name
	bw.when = append([]WhenSpec(nil), when...)
	bw.wake = wake
	bw.entries = bw.entries[:0]
	for _, spec := range when {
		if err := ValidateWhenSpec(spec); err != nil {
			InfoLog("[SCHEDULER] '%s': %v\n", name, err)
			continue
		}
		next, err := CalculateNextExecution(spec, now)
		if err != nil {
			InfoLog("[SCHEDULER] '%s': %v\n", name, err)
			continue
		}
		interval, _ := ParseFrequencyExpression(spec.Every)
		bw.entries = append(bw.entries, ParsedSchedule{
			Time: next, Interval: interval, IsAbsolute: spec.At != "",
			OriginalAt: spec.At, OriginalEvery: spec.Every,
		})
	}
	// Its own wake: once (Interval 0), unless long gone.
	if !wake.IsZero() && now.Sub(wake) <= wakeGrace {
		bw.entries = append(bw.entries, ParsedSchedule{Time: wake, IsAbsolute: true, OriginalAt: wake.Format(time.RFC3339)})
		InfoLog("[SCHEDULER] Wake for Want '%s' at %s\n", name, wake.Format(time.RFC3339))
	}
	if len(bw.entries) == 0 {
		delete(b.wants, id)
		return
	}
	b.push(id, bw)
}

// remove forgets a want's schedule.
func (b *scheduleBook) remove(id string) { delete(b.wants, id) }

// push puts the want's earliest entry in the heap, as its newest.
func (b *scheduleBook) push(id string, bw *bookedWant) {
	bw.seq++
	heap.Push(&b.h, bookItem{at: earliestOf(bw.entries), id: id, seq: bw.seq})
}

// valid is whether a heap item is still its want's earliest.
func (b *scheduleBook) valid(it bookItem) bool {
	bw := b.wants[it.id]
	return bw != nil && bw.seq == it.seq
}

// next is the first time any want's schedule comes; ok is false for none.
func (b *scheduleBook) next() (time.Time, bool) {
	for b.h.Len() > 0 {
		if b.valid(b.h[0]) {
			return b.h[0].at, true
		}
		heap.Pop(&b.h)
	}
	return time.Time{}, false
}

// due takes the wants whose schedule has come by now, and moves each on: an
// entry that came goes on by its interval, a wake (done once) is dropped.
func (b *scheduleBook) due(now time.Time) []string {
	var out []string
	for b.h.Len() > 0 && !b.h[0].at.After(now) {
		it := heap.Pop(&b.h).(bookItem)
		if !b.valid(it) {
			continue
		}
		bw := b.wants[it.id]
		out = append(out, it.id)
		kept := bw.entries[:0]
		for _, e := range bw.entries {
			if e.Time.After(now) {
				kept = append(kept, e)
			} else if e.Interval > 0 {
				e.Time = e.Time.Add(e.Interval)
				for !e.Time.After(now) { // asleep past several: once, then on
					e.Time = e.Time.Add(e.Interval)
				}
				kept = append(kept, e)
			}
		}
		bw.entries = kept
		if len(kept) == 0 {
			// Its wake done and nothing else: its wake is still as read, so
			// the want's next change (the restart taking it back) finds it.
			bw.entries = nil
			continue
		}
		b.push(it.id, bw)
	}
	return out
}

// count is how many wants have something to come.
func (b *scheduleBook) count() int {
	n := 0
	for _, bw := range b.wants {
		if len(bw.entries) > 0 {
			n++
		}
	}
	return n
}

// name is a want's name as booked.
func (b *scheduleBook) name(id string) string {
	if bw := b.wants[id]; bw != nil {
		return bw.name
	}
	return id
}

func earliestOf(entries []ParsedSchedule) time.Time {
	earliest := entries[0].Time
	for _, e := range entries[1:] {
		if e.Time.Before(earliest) {
			earliest = e.Time
		}
	}
	return earliest
}
