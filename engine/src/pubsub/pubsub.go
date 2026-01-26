package pubsub

import (
	"context"
	"time"
)

// Message represents a published message with payload and metadata.
type Message struct {
	Payload   any       // Actual data being transmitted
	Sequence  int64     // Monotonically increasing sequence number
	Timestamp time.Time // When the message was published
	Done      bool      // DONE signal flag (indicates stream termination)
}

// Subscription represents a consumer's connection to a topic.
type Subscription interface {
	Chan() <-chan *Message
	Close() error
}

// PubSub defines a flexible pub/sub interface compatible with Watermill.
type PubSub interface {
	Publish(topic string, message *Message) error
	Subscribe(topic string, consumerID string) (Subscription, error)
	Unsubscribe(topic string, consumerID string) error
	Close() error
}

// SubscriptionImpl implements the Subscription interface.
type SubscriptionImpl struct {
	msgChan chan *Message
	ctx     context.Context
	cancel  context.CancelFunc
}

func (s *SubscriptionImpl) Chan() <-chan *Message {
	return s.msgChan
}

func (s *SubscriptionImpl) Close() error {
	s.cancel()
	return nil
}
