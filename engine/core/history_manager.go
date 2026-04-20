package mywant

import (
	"sync"
	"time"
)

// HistoryManager handles all ring buffers for a want
type HistoryManager struct {
	StateHistoryRing     *ringBuffer[StateHistoryEntry]
	ParameterHistoryRing *ringBuffer[StateHistoryEntry]
	LogHistoryRing       *ringBuffer[LogHistoryEntry]
	AgentHistoryRing     *ringBuffer[AgentExecution]
	mu                   sync.Mutex

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
	}
}

// GetHistory returns a snapshot of all histories
func (h *HistoryManager) GetHistory() WantHistory {
	h.mu.Lock()
	defer h.mu.Unlock()

	return WantHistory{
		StateHistory:     h.StateHistoryRing.Snapshot(0),
		ParameterHistory: h.ParameterHistoryRing.Snapshot(0),
		LogHistory:       h.LogHistoryRing.Snapshot(0),
		AgentHistory:     h.AgentHistoryRing.Snapshot(0),
	}
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
