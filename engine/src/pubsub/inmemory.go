package pubsub

import (
	"context"
	"fmt"
	"sync"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/ThreeDotsLabs/watermill/pubsub/gochannel"
)

// WatermillPubSub wraps Watermill's pub/sub functionality with an in-memory pointer cache
// to preserve type information and support late-subscriber replay.
type WatermillPubSub struct {
	publisher  message.Publisher
	subscriber message.Subscriber
	logger     watermill.LoggerAdapter
	ctx        context.Context
	cancel     context.CancelFunc
	
	// pointerMap stores the original Message objects indexed by Watermill UUID
	// to preserve type information across the bridge.
	pointerMap map[string]*Message
	// topicCache stores sequence of IDs per topic for replay support
	topicCache map[string][]string 
	cacheMu    sync.RWMutex
	maxCache   int

	// Track active subscriptions
	subscriptions map[string]context.CancelFunc
	subMu         sync.Mutex
}

// NewInMemoryPubSub creates a new Watermill-backed PubSub with pointer preservation.
func NewInMemoryPubSub() *WatermillPubSub {
	logger := watermill.NewStdLogger(false, false)
	
	pubSub := gochannel.NewGoChannel(
		gochannel.Config{
			BlockPublishUntilSubscriberAck: false,
		},
		logger,
	)

	ctx, cancel := context.WithCancel(context.Background())

	return &WatermillPubSub{
		publisher:     pubSub,
		subscriber:    pubSub,
		logger:        logger,
		ctx:           ctx,
		cancel:        cancel,
		pointerMap:    make(map[string]*Message),
		topicCache:    make(map[string][]string),
		maxCache:      10000,
		subscriptions: make(map[string]context.CancelFunc),
	}
}

// Publish publishes a message.
func (ps *WatermillPubSub) Publish(topic string, msg *Message) error {
	msgID := watermill.NewUUID()
	
	ps.cacheMu.Lock()
	// Add to correlation map
	ps.pointerMap[msgID] = msg
	// Add to topic replay cache
	ps.topicCache[topic] = append(ps.topicCache[topic], msgID)
	
	// Cleanup old entries
	if len(ps.topicCache[topic]) > ps.maxCache {
		oldID := ps.topicCache[topic][0]
		ps.topicCache[topic] = ps.topicCache[topic][1:]
		delete(ps.pointerMap, oldID)
	}
	ps.cacheMu.Unlock()

	wMsg := message.NewMessage(msgID, []byte(msgID))
	if err := ps.publisher.Publish(topic, wMsg); err != nil {
		return fmt.Errorf("watermill publish failed: %w", err)
	}

	return nil
}

// Subscribe subscribes to a topic and replays cached messages.
func (ps *WatermillPubSub) Subscribe(topic string, consumerID string) (Subscription, error) {
	subCtx, subCancel := context.WithCancel(ps.ctx)
	
	key := consumerID + ":" + topic
	ps.subMu.Lock()
	if oldCancel, exists := ps.subscriptions[key]; exists {
		oldCancel()
	}
	ps.subscriptions[key] = subCancel
	ps.subMu.Unlock()

	messages, err := ps.subscriber.Subscribe(subCtx, topic)
	if err != nil {
		subCancel()
		return nil, fmt.Errorf("watermill subscribe failed: %w", err)
	}

	outChan := make(chan *Message, 2000)

	// Replay existing cache
	ps.cacheMu.RLock()
	if ids, ok := ps.topicCache[topic]; ok {
		for _, id := range ids {
			if m, exists := ps.pointerMap[id]; exists {
				select {
				case outChan <- m:
				default:
				}
			}
		}
	}
	ps.cacheMu.RUnlock()

	go func() {
		defer close(outChan)
		for {
			select {
			case <-subCtx.Done():
				return
			case wMsg, ok := <-messages:
				if !ok {
					return
				}
				
				ps.cacheMu.RLock()
				// Use the actual Watermill Message ID to find the correct pointer
				foundMsg, exists := ps.pointerMap[wMsg.UUID]
				ps.cacheMu.RUnlock()

				if exists {
					select {
					case outChan <- foundMsg:
						wMsg.Ack()
					case <-subCtx.Done():
						return
					}
				} else {
					// Fallback for expired cache entries
					wMsg.Ack()
				}
			}
		}
	}()

	return &SubscriptionImpl{
		msgChan: outChan,
		ctx:     subCtx,
		cancel:  subCancel,
	}, nil
}

// Unsubscribe stops a subscription.
func (ps *WatermillPubSub) Unsubscribe(topic string, consumerID string) error {
	ps.subMu.Lock()
	defer ps.subMu.Unlock()
	
	key := consumerID + ":" + topic
	if cancel, exists := ps.subscriptions[key]; exists {
		cancel()
		delete(ps.subscriptions, key)
	}
	
	return nil
}

// Close closes the entire PubSub system.
func (ps *WatermillPubSub) Close() error {
	ps.cancel()
	return ps.publisher.Close()
}