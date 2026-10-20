package mywant

import (
	"encoding/json"
	"sync"
	"time"
)

// HistoryManager handles all ring buffers for a want
type HistoryManager struct {
	StateHistoryRing     *ringBuffer[StateHistoryEntry]
	ParameterHistoryRing *ringBuffer[StateHistoryEntry]
	LogHistoryRing       *ringBuffer[LogHistoryEntry]
	AgentHistoryRing     *ringBuffer[AgentExecution]
	// ResultHistoryRing holds the want's answers, shaped (see result_history.go).
	ResultHistoryRing *ringBuffer[ResultHistoryEntry]
	mu                sync.Mutex

	// OnStateEntry is called after a state entry is recorded. Used by Want to emit
	// OTEL span events without coupling HistoryManager to the tracing library.
	OnStateEntry func(key string, value any)
}

// NewHistoryManager creates and initializes a new HistoryManager
func NewHistoryManager() *HistoryManager {
	return &HistoryManager{
		StateHistoryRing:     newRingBuffer[StateHistoryEntry](200),
		ParameterHistoryRing: newRingBuffer[StateHistoryEntry](50),
		LogHistoryRing:       newRingBuffer[LogHistoryEntry](100),
		AgentHistoryRing:     newRingBuffer[AgentExecution](100),
		ResultHistoryRing:    newRingBuffer[ResultHistoryEntry](resultHistoryDepth),
	}
}

// History budgets, in bytes of the entries as JSON (roughly — see approxSize).
//
// The rings cap how many entries they keep, not how big one is. A state entry
// holds whatever was written within its second, and a robot's holds every
// response it has given — 36KB a snapshot. That was written into state.yaml on
// every change and sent with the want each time it was asked for, and no one
// reads two hundred of them. So each history keeps its newest entries while
// they fit its budget, and always its newest one.
//
// The answers (resultHistory) are not cut: they are what the board pages
// through, notices name them by id, and the ring already keeps only twenty
// distinct ones.
const (
	stateHistoryBudget     = 64 << 10
	parameterHistoryBudget = 32 << 10
	logHistoryBudget       = 32 << 10
	agentHistoryBudget     = 32 << 10
)

// GetHistory returns a snapshot of all histories, each cut to its budget.
func (h *HistoryManager) GetHistory() WantHistory {
	h.mu.Lock()
	defer h.mu.Unlock()

	stateSize := func(e StateHistoryEntry) int { return approxSize(e.StateValue) + 64 }
	return WantHistory{
		StateHistory:     newestWithin(h.StateHistoryRing.Snapshot(0), stateHistoryBudget, stateSize),
		ParameterHistory: newestWithin(h.ParameterHistoryRing.Snapshot(0), parameterHistoryBudget, stateSize),
		LogHistory: newestWithin(h.LogHistoryRing.Snapshot(0), logHistoryBudget,
			func(e LogHistoryEntry) int { return len(e.Logs) + 48 }),
		AgentHistory: newestWithin(h.AgentHistoryRing.Snapshot(0), agentHistoryBudget,
			func(e AgentExecution) int { return approxSize(e) }),
		ResultHistory: h.ResultHistoryRing.Snapshot(0),
	}
}

// newestWithin is the newest entries (oldest first, as given) whose sizes add
// up to no more than budget — and the newest one even when it alone is over.
func newestWithin[T any](entries []T, budget int, size func(T) int) []T {
	total := 0
	for i := len(entries) - 1; i >= 0; i-- {
		total += size(entries[i])
		if total > budget && i < len(entries)-1 {
			return entries[i+1:]
		}
	}
	return entries
}

// approxSize is about how many bytes v takes as JSON. The shapes state is
// made of are walked without encoding them, since this runs each time a
// history is read; anything else is encoded.
func approxSize(v any) int {
	switch x := v.(type) {
	case nil:
		return 4
	case string:
		return len(x) + 2
	case bool:
		return 5
	case int, int64, float64, float32, int32, uint, uint64:
		return 8
	case map[string]any:
		n := 2
		for k, e := range x {
			n += len(k) + 4 + approxSize(e)
		}
		return n
	case []any:
		n := 2
		for _, e := range x {
			n += approxSize(e) + 1
		}
		return n
	case []map[string]any:
		n := 2
		for _, e := range x {
			n += approxSize(e) + 1
		}
		return n
	case []string:
		n := 2
		for _, e := range x {
			n += len(e) + 3
		}
		return n
	}
	b, err := json.Marshal(v)
	if err != nil {
		return 0
	}
	return len(b)
}

// AddStateEntry adds an entry to state history, merging with the last entry if it's very recent
func (h *HistoryManager) AddStateEntry(key string, value any) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if last, ok := h.StateHistoryRing.PeekLast(); ok {
		if time.Since(last.Timestamp) < 1*time.Second {
			h.StateHistoryRing.UpdateLast(func(e *StateHistoryEntry) {
				if lastMap, ok := e.StateValue.(map[string]any); ok {
					lastMap[key] = value
				} else {
					e.StateValue = map[string]any{key: value}
				}
			})
			return
		}
	}

	h.StateHistoryRing.Append(StateHistoryEntry{
		Timestamp:  time.Now(),
		StateValue: map[string]any{key: value},
	})

	if h.OnStateEntry != nil {
		h.OnStateEntry(key, value)
	}
}

// AddResultEntry records an answer, or notes that the last one is still current.
//
// The merge here is by VALUE, not by time the way AddStateEntry's is: a monitor
// want re-checks on its own schedule and usually finds what it found last time,
// and twenty copies of one reservation is not a history of anything. Two
// answers that fingerprint the same are one answer seen twice, and the second
// sighting only moves LastSeen.
//
// It reports whether the entry was appended as a new answer (false when it
// only extended the last one's LastSeen), and the answer it followed, if any.
func (h *HistoryManager) AddResultEntry(entry ResultHistoryEntry) (appended bool, prev *ResultHistoryEntry) {
	h.mu.Lock()
	defer h.mu.Unlock()

	fp := resultFingerprint(entry)
	last, ok := h.ResultHistoryRing.PeekLast()
	if ok && resultFingerprint(last) == fp {
		h.ResultHistoryRing.UpdateLast(func(e *ResultHistoryEntry) {
			e.LastSeen = entry.Timestamp
		})
		return false, nil
	}

	h.ResultHistoryRing.Append(entry)
	if ok {
		return true, &last
	}
	return true, nil
}

// AddParameterEntry adds an entry to parameter history, with similar merging logic
func (h *HistoryManager) AddParameterEntry(key string, value any) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if last, ok := h.ParameterHistoryRing.PeekLast(); ok {
		if time.Since(last.Timestamp) < 1*time.Second {
			h.ParameterHistoryRing.UpdateLast(func(e *StateHistoryEntry) {
				if lastMap, ok := e.StateValue.(map[string]any); ok {
					lastMap[key] = value
				} else {
					e.StateValue = map[string]any{key: value}
				}
			})
			return
		}
	}

	h.ParameterHistoryRing.Append(StateHistoryEntry{
		Timestamp:  time.Now(),
		StateValue: map[string]any{key: value},
	})
}

// AddLogEntry adds a log message, appending to the last entry if it's very recent
func (h *HistoryManager) AddLogEntry(message string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if last, ok := h.LogHistoryRing.PeekLast(); ok {
		if time.Since(last.Timestamp) < 500*time.Millisecond {
			h.LogHistoryRing.UpdateLast(func(e *LogHistoryEntry) {
				e.Logs += "\n" + message
			})
			return
		}
	}

	h.LogHistoryRing.Append(LogHistoryEntry{
		Timestamp: time.Now(),
		Logs:      message,
	})
}

// AddAgentEntry adds an entry to agent execution history
func (h *HistoryManager) AddAgentEntry(entry AgentExecution) {
	h.AgentHistoryRing.Append(entry)
}
