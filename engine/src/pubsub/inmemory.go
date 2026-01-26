package pubsub

import (
	"context"
	"fmt"
	"sync"
	"time"

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
	consumerBuf int

	// Track active subscriptions
	subscriptions map[string]context.CancelFunc
	chanMap       map[string]Subscription
	subMu         sync.Mutex
}

type TopicStats struct {
	MessageCount  int
	ConsumerCount int
	CacheSize     int
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
		consumerBuf:   2000,
		subscriptions: make(map[string]context.CancelFunc),
		chanMap:       make(map[string]Subscription),
	}
}

func (ps *WatermillPubSub) SetCacheSize(size int) {
	ps.cacheMu.Lock()
	defer ps.cacheMu.Unlock()
	ps.maxCache = size
}

func (ps *WatermillPubSub) SetConsumerBuf(size int) {
	ps.cacheMu.Lock()
	defer ps.cacheMu.Unlock()
	ps.consumerBuf = size
}

func (ps *WatermillPubSub) GetStats(topic string) (TopicStats, error) {
	ps.cacheMu.RLock()
	defer ps.cacheMu.RUnlock()
	ps.subMu.Lock()
	defer ps.subMu.Unlock()

	msgCount := 0
	if ids, ok := ps.topicCache[topic]; ok {
		msgCount = len(ids)
	}

	consumerCount := 0
	for key := range ps.subscriptions {
		// key is "consumerID:topic"
		if len(key) > len(topic) && key[len(key)-len(topic)-1:] == ":"+topic {
			consumerCount++
		}
	}

	return TopicStats{
		MessageCount:  msgCount,
		ConsumerCount: consumerCount,
		CacheSize:     msgCount,
	}, nil
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
	key := consumerID + ":" + topic
	ps.subMu.Lock()
	// Idempotency: return existing subscription
	if sub, exists := ps.chanMap[key]; exists {
		ps.subMu.Unlock()
		return sub, nil
	}
	ps.subMu.Unlock()

	subCtx, subCancel := context.WithCancel(ps.ctx)
	
	ps.subMu.Lock()
	ps.subscriptions[key] = subCancel
	ps.subMu.Unlock()

	messages, err := ps.subscriber.Subscribe(subCtx, topic)
	if err != nil {
		subCancel()
		ps.subMu.Lock()
		delete(ps.subscriptions, key)
		ps.subMu.Unlock()
		return nil, fmt.Errorf("watermill subscribe failed: %w", err)
	}

	ps.cacheMu.RLock()
	bufSize := ps.consumerBuf
	ps.cacheMu.RUnlock()
	outChan := make(chan *Message, bufSize)

	// Phase 1: Replay existing cache
	ps.cacheMu.RLock()
	var replayMsgs []*Message
	if ids, ok := ps.topicCache[topic]; ok {
		for _, id := range ids {
			if m, exists := ps.pointerMap[id]; exists {
				replayMsgs = append(replayMsgs, m)
			}
		}
	}
	ps.cacheMu.RUnlock()

	for _, m := range replayMsgs {
		select {
		case outChan <- m:
		case <-subCtx.Done():
			return nil, subCtx.Err()
		case <-time.After(100 * time.Millisecond):
			// Don't block forever if consumer is not reading
		}
	}

	// Phase 2: Start real-time delivery
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
					wMsg.Ack()
				}
			}
		}
	}()

	sub := &SubscriptionImpl{
		MsgChan: outChan,
		Ctx:     subCtx,
		Cancel:  subCancel,
	}

	ps.subMu.Lock()
	ps.chanMap[key] = sub
	ps.subMu.Unlock()

	return sub, nil
}

// IsSubscribed checks if a consumer is already subscribed to a topic.
func (ps *WatermillPubSub) IsSubscribed(topic string, consumerID string) bool {
	ps.subMu.Lock()
	defer ps.subMu.Unlock()
	key := consumerID + ":" + topic
	_, exists := ps.subscriptions[key]
	return exists
}

// Unsubscribe stops a subscription.
func (ps *WatermillPubSub) Unsubscribe(topic string, consumerID string) error {
	ps.subMu.Lock()
	defer ps.subMu.Unlock()
	
	key := consumerID + ":" + topic
	if cancel, exists := ps.subscriptions[key]; exists {
		cancel()
		delete(ps.subscriptions, key)
		delete(ps.chanMap, key)
	}
	
	return nil
}

// Close closes the entire PubSub system.
func (ps *WatermillPubSub) Close() error {
	ps.cancel()
	return ps.publisher.Close()
}