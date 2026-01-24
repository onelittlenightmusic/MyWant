package pubsub

import (
	"sync"
	"time"
)

// Message represents a published message with payload and metadata.
type Message struct {
	Payload   any       // Actual data being transmitted
	Sequence  int64     // Monotonically increasing sequence number
	Timestamp time.Time // When the message was published
	Done      bool      // DONE signal flag (indicates stream termination)
}

// TopicStats contains metrics about a topic.
type TopicStats struct {
	MessageCount  int64   // Total messages published to this topic
	ConsumerCount int     // Number of active consumers
	CacheSize     int     // Current size of message cache
	PublishRate   float64 // Messages per second (for future use)
}

// Subscription represents a consumer's connection to a topic.
// Messages received from the channel are guaranteed to be in order.
type Subscription interface {
	// Chan returns the read-only channel for receiving messages.
	// Messages are guaranteed to be in sequence order.
	Chan() <-chan *Message

	// Close closes the subscription and releases resources.
	Close() error
}

// PubSub defines a plugin-compatible pub/sub interface.
// Inspired by Kafka/NATS patterns, it supports:
// - Multiple consumers per topic with independent state
// - Automatic message replay to late subscribers
// - Optional disk persistence
type PubSub interface {
	// Publish publishes a message to a topic.
	// Message sequence is assigned by the PubSub system.
	// Non-blocking publish; slow consumers are handled according to implementation.
	Publish(topic string, message *Message) error

	// Subscribe subscribes a consumer to a topic.
	// Returns a Subscription for receiving messages.
	// If the consumer was previously subscribed, returns the existing subscription.
	// Automatically replays cached messages to the new subscriber.
	Subscribe(topic string, consumerID string) (Subscription, error)

	// Unsubscribe removes a consumer from a topic.
	// Closes the consumer's channel and releases resources.
	Unsubscribe(topic string, consumerID string) error

	// GetStats returns statistics about a topic.
	// Returns nil if the topic doesn't exist.
	GetStats(topic string) (*TopicStats, error)

	// Close closes the PubSub system and releases all resources.
	Close() error
}

// Persister defines optional disk backup for message caches.
type Persister interface {
	// Save persists messages for a topic to disk.
	Save(topic string, messages []*Message) error

	// Load loads persisted messages for a topic from disk.
	Load(topic string) ([]*Message, error)

	// Delete removes persisted messages for a topic.
	Delete(topic string) error

	// Close closes the persister and releases resources.
	Close() error
}

// ============================================================================
// Internal structures (used by implementations)
// ============================================================================

// Consumer represents an individual consumer in a topic.
type Consumer struct {
	ID        string
	Channel   chan *Message
	LastSeq   int64
	CreatedAt time.Time
}

// Topic represents a single topic with its cache and consumers.
type Topic struct {
	Name        string
	Cache       []*Message              // FIFO message cache (bounded)
	Consumers   map[string]*Consumer    // Consumer ID -> Consumer
	Sequence    int64                   // Next sequence number to assign
	Mutex       sync.RWMutex            // Protects Cache, Consumers, Sequence
	CacheSize   int                     // Maximum cache size (default 10)
	ConsumerBuf int                     // Output channel buffer size (default 30)
}

// SubscriptionImpl implements the Subscription interface.
type SubscriptionImpl struct {
	consumer *Consumer
	mu       sync.Mutex
	closed   bool
}

// Chan returns the message channel.
func (s *SubscriptionImpl) Chan() <-chan *Message {
	return s.consumer.Channel
}

// Close closes the subscription.
func (s *SubscriptionImpl) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.closed {
		close(s.consumer.Channel)
		s.closed = true
	}
	return nil
}

// DefaultCacheSize is the default maximum messages to cache per topic.
const DefaultCacheSize = 10

// DefaultConsumerBuf is the default output channel buffer size (3x cache).
const DefaultConsumerBuf = 30

// DefaultPersistenceDir is the default directory for file-based persistence.
const DefaultPersistenceDir = "./data/pubsub"
