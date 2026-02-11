package types

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ReactionData represents a user reaction to a reminder
type ReactionData struct {
	ReactionID string    `json:"reaction_id"`
	Approved   bool      `json:"approved"`
	Comment    string    `json:"comment,omitempty"`
	Timestamp  time.Time `json:"timestamp"`
}

// QueueMetadata contains basic information about a reaction queue
type QueueMetadata struct {
	QueueID       string    `json:"queue_id"`
	CreatedAt     time.Time `json:"created_at"`
	ReactionCount int       `json:"reaction_count"`
}

// ReactionQueue represents a single reaction queue that can hold multiple reactions
type ReactionQueue struct {
	ID        string         `json:"id"`
	reactions []ReactionData `json:"-"`
	mutex     sync.RWMutex   `json:"-"`
	CreatedAt time.Time      `json:"created_at"`
}

// NewReactionQueue creates a new reaction queue with the given ID
func NewReactionQueue(id string) *ReactionQueue {
	return &ReactionQueue{
		ID:        id,
		reactions: make([]ReactionData, 0),
		CreatedAt: time.Now(),
	}
}

// AddReaction adds a reaction to this queue (supports multiple reactions)
func (rq *ReactionQueue) AddReaction(approved bool, comment string) (string, error) {
	rq.mutex.Lock()
	defer rq.mutex.Unlock()

	reactionID := uuid.New().String()
	reaction := ReactionData{
		ReactionID: reactionID,
		Approved:   approved,
		Comment:    comment,
		Timestamp:  time.Now(),
	}

	rq.reactions = append(rq.reactions, reaction)
	return reactionID, nil
}

// GetReactions retrieves all reactions from this queue (non-destructive)
func (rq *ReactionQueue) GetReactions() []ReactionData {
	rq.mutex.RLock()
	defer rq.mutex.RUnlock()

	// Return a copy to prevent external modifications
	reactions := make([]ReactionData, len(rq.reactions))
	copy(reactions, rq.reactions)
	return reactions
}

// Size returns the number of reactions in this queue
func (rq *ReactionQueue) Size() int {
	rq.mutex.RLock()
	defer rq.mutex.RUnlock()

	return len(rq.reactions)
}

// ReactionQueueManager manages multiple reaction queues
type ReactionQueueManager struct {
	queues map[string]*ReactionQueue
	mutex  sync.RWMutex
}

// NewReactionQueueManager creates a new reaction queue manager
func NewReactionQueueManager() *ReactionQueueManager {
	return &ReactionQueueManager{
		queues: make(map[string]*ReactionQueue),
	}
}

// CreateQueue creates a new reaction queue and returns its unique ID
func (rqm *ReactionQueueManager) CreateQueue() (string, error) {
	rqm.mutex.Lock()
	defer rqm.mutex.Unlock()

	queueID := uuid.New().String()
	rqm.queues[queueID] = NewReactionQueue(queueID)

	return queueID, nil
}

// GetQueue retrieves a queue by ID
func (rqm *ReactionQueueManager) GetQueue(queueID string) (*ReactionQueue, error) {
	rqm.mutex.RLock()
	defer rqm.mutex.RUnlock()

	queue, exists := rqm.queues[queueID]
	if !exists {
		return nil, fmt.Errorf("queue not found: %s", queueID)
	}

	return queue, nil
}

// DeleteQueue deletes a queue by ID
func (rqm *ReactionQueueManager) DeleteQueue(queueID string) error {
	rqm.mutex.Lock()
	defer rqm.mutex.Unlock()

	if _, exists := rqm.queues[queueID]; !exists {
		return fmt.Errorf("queue not found: %s", queueID)
	}

	delete(rqm.queues, queueID)
	return nil
}

// ListQueues returns metadata for all queues
func (rqm *ReactionQueueManager) ListQueues() []QueueMetadata {
	rqm.mutex.RLock()
	defer rqm.mutex.RUnlock()

	metadata := make([]QueueMetadata, 0, len(rqm.queues))
	for queueID, queue := range rqm.queues {
		metadata = append(metadata, QueueMetadata{
			QueueID:       queueID,
			CreatedAt:     queue.CreatedAt,
			ReactionCount: queue.Size(),
		})
	}

	return metadata
}

// AddReactionToQueue adds a reaction to a specific queue
func (rqm *ReactionQueueManager) AddReactionToQueue(queueID string, approved bool, comment string) (string, error) {
	queue, err := rqm.GetQueue(queueID)
	if err != nil {
		return "", err
	}

	return queue.AddReaction(approved, comment)
}
