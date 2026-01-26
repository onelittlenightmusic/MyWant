package pubsub

import (
	"context"
	"sync"
)

// SimpleInMemPubSub is a high-performance, mutex-protected in-memory PubSub
// specifically designed for high-concurrency pointer preservation without context overhead.
type SimpleInMemPubSub struct {
	mu            sync.RWMutex
	topics        map[string][]*Message
	subscribers   map[string][]chan *Message
	maxCache      int
	ctx           context.Context
	cancel        context.CancelFunc
}

// NewInMemoryPubSub creates a new lightweight PubSub system.
func NewInMemoryPubSub() *SimpleInMemPubSub {
	ctx, cancel := context.WithCancel(context.Background())
	return &SimpleInMemPubSub{
		topics:      make(map[string][]*Message),
		subscribers: make(map[string][]chan *Message),
		maxCache:    10000,
		ctx:         ctx,
		cancel:      cancel,
	}
}

// Publish publishes a message to all current subscribers and adds to cache.
func (ps *SimpleInMemPubSub) Publish(topic string, msg *Message) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	// 1. Add to cache
	cache := ps.topics[topic]
	cache = append(cache, msg)
	if len(cache) > ps.maxCache {
		cache = cache[1:]
	}
	ps.topics[topic] = cache

	// 2. Broadcast to subscribers
	if subs, ok := ps.subscribers[topic]; ok {
		for _, ch := range subs {
			select {
			case ch <- msg:
			default:
				// Buffer full, message dropped for this subscriber
			}
		}
	}

	return nil
}

// Subscribe joins a topic and replays cached messages.
func (ps *SimpleInMemPubSub) Subscribe(topic string, consumerID string) (Subscription, error) {
	subCtx, subCancel := context.WithCancel(ps.ctx)
	
	// Create buffered channel for this subscriber
	outChan := make(chan *Message, 2000)

	ps.mu.Lock()
	// Replay cache
	if cache, ok := ps.topics[topic]; ok {
		for _, msg := range cache {
			select {
			case outChan <- msg:
			default:
			}
		}
	}
	// Register subscriber
	ps.subscribers[topic] = append(ps.subscribers[topic], outChan)
	ps.mu.Unlock()

	// Cleanup on context cancellation
	go func() {
		<-subCtx.Done()
		ps.mu.Lock()
		defer ps.mu.Unlock()
		
		subs := ps.subscribers[topic]
		for i, ch := range subs {
			if ch == outChan {
				ps.subscribers[topic] = append(subs[:i], subs[i+1:]...)
				close(outChan)
				break
			}
		}
	}()

	return &SubscriptionImpl{
		msgChan: outChan,
		ctx:     subCtx,
		cancel:  subCancel,
	}, nil
}

// Unsubscribe is handled by the subscription's Close/Cancel.
func (ps *SimpleInMemPubSub) Unsubscribe(topic string, consumerID string) error {
	return nil // Handled via subscription Close
}

// Close closes the entire PubSub system.
func (ps *SimpleInMemPubSub) Close() error {
	ps.cancel()
	return nil
}
