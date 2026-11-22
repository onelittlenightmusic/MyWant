package mywant

import (
	"encoding/json"
	"fmt"
	"sort"
)

// A want's answer told with what changed in it: at a timing that compares a
// field with its value at the last answer (onChange, onIncrease, onDecrease),
// what it was and what it is (changed), or the items a list gained or lost
// (added / removed) — so a want that hears it knows what happened without
// knowing the type that said it, and can tell one item from another (its key)
// to act on that same item later.

// Change kinds.
const (
	ChangeChanged = "changed"
	ChangeAdded   = "added"
	ChangeRemoved = "removed"
)

// Change is what one compared field did since the last answer.
type Change struct {
	Kind  string       `json:"kind"`
	Field string       `json:"field"`
	From  any          `json:"from,omitempty"`
	To    any          `json:"to,omitempty"`
	Items []ChangeItem `json:"items,omitempty"`
}

// ChangeItem is one item a list gained or lost: its key (the same for the
// same item, answer to answer) and the item.
type ChangeItem struct {
	Key   string `json:"key"`
	Value any    `json:"value"`
	// Labels: what the item is about in set words — when (MurmurLabelTime)
	// and where (MurmurLabelPlace) — read off its fields (itemLabels), so a
	// want that hears it acts on an item of any type alike.
	Labels map[string]string `json:"labels,omitempty"`
}

// fieldChange is what field did, from before to now, as the timing that
// compared it asked: changed for onChange, the items gained or lost for a
// list's onIncrease / onDecrease (a number's, changed). Nil when nothing did.
func fieldChange(field string, before, now any, list bool) *Change {
	if list {
		b, bok := before.([]any)
		n, nok := now.([]any)
		if bok || nok {
			added, removed := diffItems(b, n)
			switch {
			case len(added) > 0 && len(removed) == 0:
				return &Change{Kind: ChangeAdded, Field: field, Items: added}
			case len(removed) > 0 && len(added) == 0:
				return &Change{Kind: ChangeRemoved, Field: field, Items: removed}
			case len(added) > 0:
				// Both: told as added, the lost ones too (a reader that
				// cares looks at both).
				return &Change{Kind: ChangeAdded, Field: field, Items: append(added, removed...)}
			}
			return nil
		}
	}
	if sameAnswerResult(before, now) {
		return nil
	}
	return &Change{Kind: ChangeChanged, Field: field, From: before, To: now}
}

// diffItems are the items of now not in before (added) and of before not in
// now (removed), each by its key.
func diffItems(before, now []any) (added, removed []ChangeItem) {
	keys := func(list []any) map[string]any {
		m := map[string]any{}
		for _, v := range list {
			m[ItemKey(v)] = v
		}
		return m
	}
	b, n := keys(before), keys(now)
	for k, v := range n {
		if _, ok := b[k]; !ok {
			added = append(added, ChangeItem{Key: k, Value: v, Labels: itemLabels(v)})
		}
	}
	for k, v := range b {
		if _, ok := n[k]; !ok {
			removed = append(removed, ChangeItem{Key: k, Value: v, Labels: itemLabels(v)})
		}
	}
	sort.Slice(added, func(i, j int) bool { return added[i].Key < added[j].Key })
	sort.Slice(removed, func(i, j int) bool { return removed[i].Key < removed[j].Key })
	return added, removed
}

// The fields an item says when and where it is by, the first it has.
var (
	itemTimeFields  = []string{"datetime_rfc3339", "start", "starts_at", "event_time", "datetime", "at"}
	itemPlaceFields = []string{"place", "store", "location", "venue", "room"}
)

// itemLabels are an item's set labels: its time (RFC3339, as it is given)
// and place, when it has them.
func itemLabels(v any) map[string]string {
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	labels := map[string]string{}
	for _, f := range itemTimeFields {
		if s, ok := m[f].(string); ok && s != "" {
			labels[MurmurLabelTime] = s
			break
		}
	}
	if labels[MurmurLabelTime] == "" {
		// A day and a clock reading apart (a free slot: date, time).
		d, _ := m["date"].(string)
		t, _ := m["time"].(string)
		if d != "" && t != "" {
			labels[MurmurLabelTime] = d + "T" + t + ":00"
		}
	}
	for _, f := range itemPlaceFields {
		if s, ok := m[f].(string); ok && s != "" {
			labels[MurmurLabelPlace] = s
			break
		}
	}
	if len(labels) == 0 {
		return nil
	}
	return labels
}

// FailedTiming is the timing a type names to be told of its want failing:
// told with the status and the error (ResultHistoryEntry.Status, Error).
const FailedTiming = "failed"

// itemKeyFields are the fields an item is known by, the first it has: its
// own id, else what a reservation or a booking is (where and when), else its
// link (a link alone may be shared by several: a store's page).
var itemKeyFields = []string{"id", "key", "datetime_rfc3339", "url"}

// ItemKey is how an item is known from one answer to the next: its id (or
// key, url, time) when it has one, else the item itself written out.
func ItemKey(v any) string {
	if m, ok := v.(map[string]any); ok {
		for _, f := range itemKeyFields {
			if s, ok := m[f].(string); ok && s != "" {
				if f == "datetime_rfc3339" {
					// A time alone is not a reservation: where, too.
					for _, where := range []string{"room", "store"} {
						if p, ok := m[where].(string); ok && p != "" {
							return p + "@" + s
						}
					}
				}
				return s
			}
			if f == "datetime_rfc3339" {
				break
			}
		}
		// Where, on which day, at what time (a free slot).
		d, _ := m["date"].(string)
		t, _ := m["time"].(string)
		where, _ := m["room"].(string)
		if where == "" {
			where, _ = m["store"].(string)
		}
		if d != "" && t != "" && where != "" {
			return where + "@" + d + " " + t
		}
		if s, ok := m["url"].(string); ok && s != "" {
			return s
		}
	}
	if s, ok := v.(string); ok {
		return s
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(b)
}
