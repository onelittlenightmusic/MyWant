package main

import (
	"sync"
	"time"
)

// ReactionData represents a user reaction to a reminder
type ReactionData struct {
	ReactionID string
	Approved   bool
	Comment    string
	Timestamp  time.Time
}

// ReactionQueue is a thread-safe in-memory queue for user reactions
type ReactionQueue struct {
	queue map[string]ReactionData
	mutex sync.RWMutex
}

// NewReactionQueue creates a new reaction queue
func NewReactionQueue() *ReactionQueue {
	return &ReactionQueue{
		queue: make(map[string]ReactionData),
	}
}

// AddReaction adds a reaction to the queue
func (rq *ReactionQueue) AddReaction(id string, approved bool, comment string) {
	rq.mutex.Lock()
	defer rq.mutex.Unlock()

	rq.queue[id] = ReactionData{
		ReactionID: id,
		Approved:   approved,
		Comment:    comment,
		Timestamp:  time.Now(),
	}
}

// GetReaction retrieves a reaction from the queue (non-destructive)
func (rq *ReactionQueue) GetReaction(id string) (ReactionData, bool) {
	rq.mutex.RLock()
	defer rq.mutex.RUnlock()

	reaction, exists := rq.queue[id]
	return reaction, exists
}

// RemoveReaction removes a reaction from the queue after processing
func (rq *ReactionQueue) RemoveReaction(id string) {
	rq.mutex.Lock()
	defer rq.mutex.Unlock()

	delete(rq.queue, id)
}

// ListReactions returns all pending reactions (for debugging)
func (rq *ReactionQueue) ListReactions() []ReactionData {
	rq.mutex.RLock()
	defer rq.mutex.RUnlock()

	reactions := make([]ReactionData, 0, len(rq.queue))
	for _, reaction := range rq.queue {
		reactions = append(reactions, reaction)
	}
	return reactions
}

// Size returns the current queue size
func (rq *ReactionQueue) Size() int {
	rq.mutex.RLock()
	defer rq.mutex.RUnlock()

	return len(rq.queue)
}
