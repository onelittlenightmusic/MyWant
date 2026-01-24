package pubsub

import (
	"fmt"
	"log"
	"sync"
	"time"
)

// InMemoryPubSub provides a lightweight in-memory pub/sub implementation.
// It maintains message caches per topic and supports multiple consumers per topic.
// Features:
// - FIFO message ordering via sequence numbers
// - Automatic cache replay to late subscribers
// - Configurable cache size and buffer limits
// - Optional disk persistence
type InMemoryPubSub struct {
	topics      map[string]*Topic // Topic name -> Topic
	mu          sync.RWMutex      // Protects topics map
	persistence Persister         // Optional disk backup
	cacheSize   int               // Default cache size per topic
	consumerBuf int               // Default output channel buffer per consumer
}

// NewInMemoryPubSub creates a new in-memory PubSub with default settings.
func NewInMemoryPubSub() *InMemoryPubSub {
	return &InMemoryPubSub{
		topics:      make(map[string]*Topic),
		cacheSize:   DefaultCacheSize,
		consumerBuf: DefaultConsumerBuf,
	}
}

// NewInMemoryPubSubWithPersistence creates a new in-memory PubSub with optional persistence.
func NewInMemoryPubSubWithPersistence(persister Persister) *InMemoryPubSub {
	return &InMemoryPubSub{
		topics:      make(map[string]*Topic),
		persistence: persister,
		cacheSize:   DefaultCacheSize,
		consumerBuf: DefaultConsumerBuf,
	}
}

// Publish publishes a message to a topic.
// Non-blocking: slow consumers don't block the publisher.
// Slow consumers (channel full) will have their message dropped with a warning.
func (ps *InMemoryPubSub) Publish(topic string, message *Message) error {
	// Get or create topic
	ps.mu.Lock()
	t, exists := ps.topics[topic]
	if !exists {
		t = &Topic{
			Name:        topic,
			Cache:       make([]*Message, 0, ps.cacheSize),
			Consumers:   make(map[string]*Consumer),
			CacheSize:   ps.cacheSize,
			ConsumerBuf: ps.consumerBuf,
		}
		ps.topics[topic] = t
		log.Printf("[PubSub] Created topic: %s", topic)
	}
	ps.mu.Unlock()

	// Lock topic for modification
	t.Mutex.Lock()
	defer t.Mutex.Unlock()

	// Assign sequence number
	message.Sequence = t.Sequence
	t.Sequence++

	// Add to cache (FIFO, bounded by cacheSize)
	t.Cache = append(t.Cache, message)
	if len(t.Cache) > t.CacheSize {
		// Evict oldest message
		t.Cache = t.Cache[1:]
	}

	log.Printf("[PubSub] Published message (seq=%d, topic=%s, cache=%d/%d)",
		message.Sequence, topic, len(t.Cache), t.CacheSize)

	// Broadcast to all consumers (non-blocking)
	for consumerID, consumer := range t.Consumers {
		// Only send if this is a new message for this consumer
		if message.Sequence > consumer.LastSeq {
			select {
			case consumer.Channel <- message:
				consumer.LastSeq = message.Sequence
			default:
				// Consumer too slow, drop packet
				log.Printf("[PubSub] Consumer %s slow, dropping message (seq=%d, topic=%s)",
					consumerID, message.Sequence, topic)
			}
		}
	}

	// Optional: persist to disk
	if ps.persistence != nil {
		if err := ps.persistence.Save(topic, t.Cache); err != nil {
			log.Printf("[PubSub] Failed to persist topic %s: %v", topic, err)
		}
	}

	return nil
}

// Subscribe subscribes a consumer to a topic.
// Automatically replays cached messages to the new subscriber.
// If the consumer is already subscribed, returns the existing subscription.
func (ps *InMemoryPubSub) Subscribe(topic string, consumerID string) (Subscription, error) {
	// Get or create topic
	ps.mu.Lock()
	t, exists := ps.topics[topic]
	if !exists {
		// Try to load from persistence if available
		var cache []*Message
		if ps.persistence != nil {
			var err error
			cache, err = ps.persistence.Load(topic)
			if err != nil {
				log.Printf("[PubSub] Failed to load persisted topic %s: %v", topic, err)
				cache = make([]*Message, 0)
			}
		}

		t = &Topic{
			Name:        topic,
			Cache:       cache,
			Consumers:   make(map[string]*Consumer),
			CacheSize:   ps.cacheSize,
			ConsumerBuf: ps.consumerBuf,
		}

		// Calculate sequence from cache
		if len(cache) > 0 {
			t.Sequence = cache[len(cache)-1].Sequence + 1
		}

		ps.topics[topic] = t
		log.Printf("[PubSub] Created topic: %s (cache size: %d)", topic, len(cache))
	}
	ps.mu.Unlock()

	// Lock topic for subscription
	t.Mutex.Lock()
	defer t.Mutex.Unlock()

	// Check if already subscribed
	if existing, exists := t.Consumers[consumerID]; exists {
		log.Printf("[PubSub] Consumer %s already subscribed to topic %s", consumerID, topic)
		return &SubscriptionImpl{consumer: existing}, nil
	}

	// Create new consumer
	consumer := &Consumer{
		ID:        consumerID,
		Channel:   make(chan *Message, t.ConsumerBuf),
		LastSeq:   -1, // Start before first message
		CreatedAt: time.Now(),
	}

	// Replay cache to new subscriber (non-blocking)
	replayedCount := 0
	for _, msg := range t.Cache {
		select {
		case consumer.Channel <- msg:
			consumer.LastSeq = msg.Sequence
			replayedCount++
		default:
			// Channel full, stop replay (consumer will process what was sent)
			break
		}
	}

	t.Consumers[consumerID] = consumer
	log.Printf("[PubSub] Subscribed consumer '%s' to topic '%s', replayed %d cached messages",
		consumerID, topic, replayedCount)

	return &SubscriptionImpl{consumer: consumer}, nil
}

// Unsubscribe removes a consumer from a topic.
func (ps *InMemoryPubSub) Unsubscribe(topic string, consumerID string) error {
	ps.mu.RLock()
	t, exists := ps.topics[topic]
	ps.mu.RUnlock()

	if !exists {
		return fmt.Errorf("topic %s not found", topic)
	}

	t.Mutex.Lock()
	defer t.Mutex.Unlock()

	consumer, exists := t.Consumers[consumerID]
	if !exists {
		return fmt.Errorf("consumer %s not subscribed to topic %s", consumerID, topic)
	}

	// Close the consumer's channel
	close(consumer.Channel)
	delete(t.Consumers, consumerID)

	log.Printf("[PubSub] Unsubscribed consumer '%s' from topic '%s'", consumerID, topic)

	return nil
}

// GetStats returns statistics about a topic.
func (ps *InMemoryPubSub) GetStats(topic string) (*TopicStats, error) {
	ps.mu.RLock()
	t, exists := ps.topics[topic]
	ps.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("topic %s not found", topic)
	}

	t.Mutex.RLock()
	defer t.Mutex.RUnlock()

	stats := &TopicStats{
		MessageCount:  t.Sequence,
		ConsumerCount: len(t.Consumers),
		CacheSize:     len(t.Cache),
	}

	return stats, nil
}

// Close closes the PubSub system.
func (ps *InMemoryPubSub) Close() error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	// Close all consumer channels
	for _, t := range ps.topics {
		t.Mutex.Lock()
		for consumerID, consumer := range t.Consumers {
			close(consumer.Channel)
			log.Printf("[PubSub] Closed consumer '%s' on topic '%s'", consumerID, t.Name)
		}
		t.Mutex.Unlock()
	}

	ps.topics = make(map[string]*Topic)

	// Close persistence if available
	if ps.persistence != nil {
		if err := ps.persistence.Close(); err != nil {
			log.Printf("[PubSub] Failed to close persister: %v", err)
		}
	}

	log.Printf("[PubSub] PubSub system closed")
	return nil
}

// SetCacheSize sets the default cache size for new topics.
func (ps *InMemoryPubSub) SetCacheSize(size int) {
	ps.cacheSize = size
}

// SetConsumerBuf sets the default consumer buffer size for new topics.
func (ps *InMemoryPubSub) SetConsumerBuf(size int) {
	ps.consumerBuf = size
}
