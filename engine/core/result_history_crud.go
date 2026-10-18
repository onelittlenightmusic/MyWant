package mywant

import (
	"encoding/json"
	"strings"
	"time"
)

// A want's answers, edited by hand (the API's /wants/{id}/results): read,
// added, changed, let go.
//
// The engine keeps them by itself (recordResultHistory); these are for a
// person who knows better — an answer that is not one, a reservation the page
// never showed. An answer let go is remembered as let go (its key, and its own
// time when the type reads one, history-at), so a list its run reads again
// (history-from) does not bring it back.

// forgottenAnswersKey is the state where the answers let go are remembered.
const forgottenAnswersKey = "result_history_forgotten"

// ResultHistoryKey is how an answer is named — its id, else when it came.
func ResultHistoryKey(e ResultHistoryEntry) string {
	if e.ID != "" {
		return e.ID
	}
	return e.Timestamp.Format(time.RFC3339Nano)
}

// ResultHistoryEntries are the want's answers, oldest first.
func (n *Want) ResultHistoryEntries() []ResultHistoryEntry {
	return n.getHistoryManager().ResultHistoryRing.Snapshot(0)
}

// AddResultHistory adds an answer by hand, among the others in the order of
// what they are about. Its id and times are filled in when it has none.
func (n *Want) AddResultHistory(e ResultHistoryEntry) ResultHistoryEntry {
	now := time.Now()
	if e.ID == "" {
		e.ID = "out-" + strings.TrimPrefix(GenerateUUID(), "want-")
	}
	if e.Timestamp.IsZero() {
		e.Timestamp = now
	}
	if e.LastSeen.IsZero() {
		e.LastSeen = e.Timestamp
	}
	if e.Type == "" {
		e.Type = n.outputType(n.Spec.FinalResultField, e.Result)
	}
	n.editResultHistory(func(entries []ResultHistoryEntry) []ResultHistoryEntry {
		return append(entries, e)
	})
	return e
}

// UpdateResultHistory changes one answer; false when there is none by that key.
func (n *Want) UpdateResultHistory(key string, change func(*ResultHistoryEntry)) (ResultHistoryEntry, bool) {
	var out ResultHistoryEntry
	found := false
	n.editResultHistory(func(entries []ResultHistoryEntry) []ResultHistoryEntry {
		for i := range entries {
			if ResultHistoryKey(entries[i]) == key {
				change(&entries[i])
				entries[i].ID = key // the key stays the answer's name
				out, found = entries[i], true
			}
		}
		return entries
	})
	return out, found
}

// DeleteResultHistory lets one answer go, and remembers it; false when there
// is none by that key.
func (n *Want) DeleteResultHistory(key string) bool {
	var gone *ResultHistoryEntry
	n.editResultHistory(func(entries []ResultHistoryEntry) []ResultHistoryEntry {
		kept := entries[:0:0]
		for _, e := range entries {
			if ResultHistoryKey(e) == key {
				e := e
				gone = &e
				continue
			}
			kept = append(kept, e)
		}
		return kept
	})
	if gone == nil {
		return false
	}
	forgotten := n.forgottenAnswers()
	forgotten[key] = true
	forgotten[forgottenResultKey(gone.Result)] = true
	if at := n.historyAt(); at != "" {
		if t, ok := answerAt(gone.Result, at); ok {
			forgotten[t.Format(time.RFC3339)] = true
		}
	}
	list := make([]any, 0, len(forgotten))
	for k := range forgotten {
		list = append(list, k)
	}
	n.StoreState(forgottenAnswersKey, list)
	return true
}

// forgottenResultKey names an answer by what it says, so the same answer
// found again by the want's own run — the one it gives now, let go — is not
// kept again either.
func forgottenResultKey(result any) string {
	b, err := json.Marshal(result)
	if err != nil {
		return ""
	}
	return "result:" + string(b)
}

// forgottenAnswers are the keys and times of the answers let go by hand.
func (n *Want) forgottenAnswers() map[string]bool {
	out := map[string]bool{}
	raw, _ := n.getState(forgottenAnswersKey)
	switch t := raw.(type) {
	case []any:
		for _, v := range t {
			if s, ok := v.(string); ok {
				out[s] = true
			}
		}
	case []string:
		for _, s := range t {
			out[s] = true
		}
	}
	return out
}

// historyAt is the field an answer's own time is read from, or "".
func (n *Want) historyAt() string {
	if n.WantTypeDefinition == nil {
		return ""
	}
	return strings.TrimSpace(n.WantTypeDefinition.Metadata.Labels[historyAtLabel])
}

// editResultHistory rewrites the answers under the history's lock: put in
// order, cut to the depth kept. Then says so (resultHistoryChanged).
func (n *Want) editResultHistory(edit func([]ResultHistoryEntry) []ResultHistoryEntry) {
	h := n.getHistoryManager()
	func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		entries := edit(h.ResultHistoryRing.Snapshot(0))
		entries = orderAnswers(entries, n.historyAt())
		if len(entries) > resultHistoryDepth {
			entries = entries[len(entries)-resultHistoryDepth:]
		}
		h.ResultHistoryRing.Clear()
		for _, e := range entries {
			h.ResultHistoryRing.Append(e)
		}
	}()
	n.resultHistoryChanged()
}

// OnResultHistoryChanged, when set, hears that a want's answers changed —
// one added by its run or read from its list, let go by its type's rule or by
// hand. The server says the want changed (board_changed), and every page of
// the GUI reads it again: its cards' pages and its balls are its answers.
var OnResultHistoryChanged func(w *Want)

// resultHistoryChanged is every change to the answers, said in one place.
func (n *Want) resultHistoryChanged() {
	if OnResultHistoryChanged != nil {
		go OnResultHistoryChanged(n)
	}
}
