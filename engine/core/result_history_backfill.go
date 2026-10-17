package mywant

import (
	"sort"
	"strings"
	"time"
)

// Answers a want did not arrive at itself, but read: a list its run found of
// the answers that came before.
//
// smartgolf_check_reserved answers with the next reservation, so its history
// holds only the reservations that were next when it happened to look. The
// reservations page lists the past ones too — the rounds played between two
// checks, a booking made and played the same day — and each of those is an
// answer a score card belongs with. A type names that list (label
// history-from) and the field of each item that says when it is (history-at);
// every item the history does not have yet goes in, at its own time, among the
// answers already there. None is announced: they are the past, found late.

const (
	historyFromLabel = "history-from"
	historyAtLabel   = "history-at"
)

// backfillResultHistory merges the type's history-from list into the answers.
func (n *Want) backfillResultHistory() {
	def := n.WantTypeDefinition
	if def == nil {
		return
	}
	from := strings.TrimSpace(def.Metadata.Labels[historyFromLabel])
	at := strings.TrimSpace(def.Metadata.Labels[historyAtLabel])
	if from == "" || at == "" {
		return
	}
	raw, ok := resolveNestedStateField(n, from)
	if !ok {
		return
	}
	items, _ := raw.([]any)
	if len(items) == 0 {
		return
	}
	h := n.getHistoryManager()
	h.mu.Lock()
	defer h.mu.Unlock()
	entries := h.ResultHistoryRing.Snapshot(0)
	have := map[string]bool{}
	for _, e := range entries {
		if t, ok := answerAt(e.Result, at); ok {
			have[t.Format(time.RFC3339)] = true
		}
	}
	added := false
	for _, it := range items {
		item, ok := it.(map[string]any)
		if !ok {
			continue
		}
		t, ok := answerAt(item, at)
		if !ok || have[t.Format(time.RFC3339)] {
			continue
		}
		have[t.Format(time.RFC3339)] = true
		entries = append(entries, ResultHistoryEntry{
			ID:        "out-" + strings.TrimPrefix(GenerateUUID(), "want-"),
			Type:      n.outputType(n.Spec.FinalResultField, item),
			Timestamp: t,
			LastSeen:  t,
			About:     t.In(time.Local).Format("2006-01-02 15:04"),
			Result:    item,
		})
		added = true
	}
	if !added {
		return
	}
	entries = orderAnswers(entries, at)
	if len(entries) > resultHistoryDepth {
		entries = entries[len(entries)-resultHistoryDepth:]
	}
	h.ResultHistoryRing.Clear()
	for _, e := range entries {
		h.ResultHistoryRing.Append(e)
	}
}

// orderAnswers puts answers in the order of what they are about — an item's
// own time where it has one, else when it was found — oldest first. Stable, so
// answers about the same moment keep the order they came in.
func orderAnswers(entries []ResultHistoryEntry, at string) []ResultHistoryEntry {
	when := func(e ResultHistoryEntry) time.Time {
		if t, ok := answerAt(e.Result, at); ok {
			return t
		}
		return e.Timestamp
	}
	sort.SliceStable(entries, func(i, j int) bool { return when(entries[i]).Before(when(entries[j])) })
	return entries
}

// answerAt is the time an answer's field says it is about.
func answerAt(result any, field string) (time.Time, bool) {
	m, ok := result.(map[string]any)
	if !ok {
		return time.Time{}, false
	}
	s, ok := m[field].(string)
	if !ok {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(s))
	return t, err == nil
}
