package pubsub

import (
	"sync"
	"testing"
	"time"
)

// TestPublishAndSubscribe tests basic publish and subscribe functionality.
func TestPublishAndSubscribe(t *testing.T) {
	ps := NewInMemoryPubSub()
	defer ps.Close()

	topic := "test-topic"
	payload := "test-message"

	// Subscribe first
	sub, err := ps.Subscribe(topic, "consumer1")
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}

	// Publish message
	msg := &Message{
		Payload:   payload,
		Timestamp: time.Now(),
		Done:      false,
	}

	if err := ps.Publish(topic, msg); err != nil {
		t.Fatalf("Failed to publish: %v", err)
	}

	// Receive message
	select {
	case received := <-sub.Chan():
		if received.Payload != payload {
			t.Errorf("Expected payload %v, got %v", payload, received.Payload)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for message")
	}
}

// TestCacheReplay tests that messages are replayed to late subscribers.
func TestCacheReplay(t *testing.T) {
	ps := NewInMemoryPubSub()
	defer ps.Close()

	topic := "test-topic"

	// Publish 5 messages
	messages := make([]any, 5)
	for i := 0; i < 5; i++ {
		messages[i] = i
		msg := &Message{
			Payload:   i,
			Timestamp: time.Now(),
			Done:      false,
		}
		if err := ps.Publish(topic, msg); err != nil {
			t.Fatalf("Failed to publish: %v", err)
		}
	}

	// Subscribe after all messages
	sub, err := ps.Subscribe(topic, "consumer1")
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}

	// Should receive all 5 cached messages
	for i := 0; i < 5; i++ {
		select {
		case msg := <-sub.Chan():
			if msg.Payload != i {
				t.Errorf("Expected message %d, got %v", i, msg.Payload)
			}
		case <-time.After(1 * time.Second):
			t.Fatal("Timeout waiting for message")
		}
	}
}

// TestMultipleConsumers tests that multiple consumers receive messages independently.
func TestMultipleConsumers(t *testing.T) {
	ps := NewInMemoryPubSub()
	defer ps.Close()

	topic := "test-topic"

	// Subscribe 3 consumers
	subs := make([]Subscription, 3)
	for i := 0; i < 3; i++ {
		sub, err := ps.Subscribe(topic, "consumer"+string(rune(i)))
		if err != nil {
			t.Fatalf("Failed to subscribe: %v", err)
		}
		subs[i] = sub
	}

	// Publish 2 messages
	for i := 0; i < 2; i++ {
		msg := &Message{
			Payload:   i,
			Timestamp: time.Now(),
			Done:      false,
		}
		if err := ps.Publish(topic, msg); err != nil {
			t.Fatalf("Failed to publish: %v", err)
		}
	}

	// Each consumer should receive both messages
	for consumerIdx := 0; consumerIdx < 3; consumerIdx++ {
		for msgIdx := 0; msgIdx < 2; msgIdx++ {
			select {
			case msg := <-subs[consumerIdx].Chan():
				if msg.Payload != msgIdx {
					t.Errorf("Consumer %d: expected message %d, got %v",
						consumerIdx, msgIdx, msg.Payload)
				}
			case <-time.After(1 * time.Second):
				t.Fatal("Timeout waiting for message")
			}
		}
	}
}

// TestDoneSignal tests that Done flag is properly transmitted.
func TestDoneSignal(t *testing.T) {
	ps := NewInMemoryPubSub()
	defer ps.Close()

	topic := "test-topic"

	sub, err := ps.Subscribe(topic, "consumer1")
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}

	// Publish normal message
	msg1 := &Message{
		Payload:   "data",
		Timestamp: time.Now(),
		Done:      false,
	}
	if err := ps.Publish(topic, msg1); err != nil {
		t.Fatalf("Failed to publish: %v", err)
	}

	// Publish Done message
	msgDone := &Message{
		Payload:   nil,
		Timestamp: time.Now(),
		Done:      true,
	}
	if err := ps.Publish(topic, msgDone); err != nil {
		t.Fatalf("Failed to publish: %v", err)
	}

	// Receive both messages
	select {
	case msg := <-sub.Chan():
		if msg.Payload != "data" || msg.Done != false {
			t.Errorf("Expected data message with Done=false")
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for message")
	}

	select {
	case msg := <-sub.Chan():
		if msg.Done != true {
			t.Errorf("Expected Done=true message")
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for Done message")
	}
}

// TestCacheOverflow tests that cache respects size limits.
func TestCacheOverflow(t *testing.T) {
	ps := NewInMemoryPubSub()
	ps.SetCacheSize(5)
	defer ps.Close()

	topic := "test-topic"

	// Publish 10 messages (cache size is 5)
	for i := 0; i < 10; i++ {
		msg := &Message{
			Payload:   i,
			Timestamp: time.Now(),
			Done:      false,
		}
		if err := ps.Publish(topic, msg); err != nil {
			t.Fatalf("Failed to publish: %v", err)
		}
	}

	// Subscribe - should only get last 5 messages (5-9)
	sub, err := ps.Subscribe(topic, "consumer1")
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}

	for i := 5; i < 10; i++ {
		select {
		case msg := <-sub.Chan():
			if msg.Payload != i {
				t.Errorf("Expected message %d, got %v", i, msg.Payload)
			}
		case <-time.After(1 * time.Second):
			t.Fatal("Timeout waiting for message")
		}
	}
}

// TestSequenceOrdering tests that sequence numbers maintain order.
func TestSequenceOrdering(t *testing.T) {
	ps := NewInMemoryPubSub()
	defer ps.Close()

	topic := "test-topic"

	sub, err := ps.Subscribe(topic, "consumer1")
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}

	// Publish 10 messages
	for i := 0; i < 10; i++ {
		msg := &Message{
			Payload:   i,
			Timestamp: time.Now(),
			Done:      false,
		}
		if err := ps.Publish(topic, msg); err != nil {
			t.Fatalf("Failed to publish: %v", err)
		}
	}

	// Verify sequence numbers are monotonic
	var lastSeq int64 = -1
	for i := 0; i < 10; i++ {
		select {
		case msg := <-sub.Chan():
			if msg.Sequence <= lastSeq {
				t.Errorf("Sequence number not monotonic: %d <= %d", msg.Sequence, lastSeq)
			}
			lastSeq = msg.Sequence
		case <-time.After(1 * time.Second):
			t.Fatal("Timeout waiting for message")
		}
	}
}

// TestUnsubscribe tests that unsubscribe properly removes a consumer.
func TestUnsubscribe(t *testing.T) {
	ps := NewInMemoryPubSub()
	defer ps.Close()

	topic := "test-topic"

	sub, err := ps.Subscribe(topic, "consumer1")
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}

	// Publish first message
	msg1 := &Message{
		Payload:   1,
		Timestamp: time.Now(),
		Done:      false,
	}
	if err := ps.Publish(topic, msg1); err != nil {
		t.Fatalf("Failed to publish: %v", err)
	}

	// Receive first message
	<-sub.Chan()

	// Unsubscribe
	if err := ps.Unsubscribe(topic, "consumer1"); err != nil {
		t.Fatalf("Failed to unsubscribe: %v", err)
	}

	// Channel should be closed
	_, ok := <-sub.Chan()
	if ok {
		t.Errorf("Channel should be closed after unsubscribe")
	}
}

// TestConcurrentPublish tests concurrent publishing with sufficient buffering.
func TestConcurrentPublish(t *testing.T) {
	ps := NewInMemoryPubSub()
	ps.SetConsumerBuf(500) // Increase buffer for concurrent test
	defer ps.Close()

	topic := "test-topic"
	numPublishers := 5
	messagesPerPublisher := 50
	totalMessages := numPublishers * messagesPerPublisher

	// Subscribe
	sub, err := ps.Subscribe(topic, "consumer1")
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}

	// Publish concurrently
	var wg sync.WaitGroup
	for p := 0; p < numPublishers; p++ {
		wg.Add(1)
		go func(publisherID int) {
			defer wg.Done()
			for i := 0; i < messagesPerPublisher; i++ {
				msg := &Message{
					Payload:   publisherID*1000 + i,
					Timestamp: time.Now(),
					Done:      false,
				}
				if err := ps.Publish(topic, msg); err != nil {
					t.Errorf("Failed to publish: %v", err)
				}
			}
		}(p)
	}

	wg.Wait()

	// Receive all messages (don't drain all in parallel, just verify count)
	receivedCount := 0
	timeout := time.After(3 * time.Second)
	for {
		select {
		case <-sub.Chan():
			receivedCount++
			if receivedCount >= totalMessages {
				goto done
			}
		case <-timeout:
			t.Fatalf("Timeout: received %d/%d messages", receivedCount, totalMessages)
		}
	}

done:
	if err := ps.Unsubscribe(topic, "consumer1"); err != nil {
		t.Fatalf("Failed to unsubscribe: %v", err)
	}
}

// TestGetStats tests statistics retrieval.
func TestGetStats(t *testing.T) {
	ps := NewInMemoryPubSub()
	defer ps.Close()

	topic := "test-topic"

	// Subscribe 3 consumers
	for i := 0; i < 3; i++ {
		if _, err := ps.Subscribe(topic, "consumer"+string(rune(i))); err != nil {
			t.Fatalf("Failed to subscribe: %v", err)
		}
	}

	// Publish 5 messages
	for i := 0; i < 5; i++ {
		msg := &Message{
			Payload:   i,
			Timestamp: time.Now(),
			Done:      false,
		}
		if err := ps.Publish(topic, msg); err != nil {
			t.Fatalf("Failed to publish: %v", err)
		}
	}

	// Get stats
	stats, err := ps.GetStats(topic)
	if err != nil {
		t.Fatalf("Failed to get stats: %v", err)
	}

	if stats.MessageCount != 5 {
		t.Errorf("Expected 5 messages, got %d", stats.MessageCount)
	}

	if stats.ConsumerCount != 3 {
		t.Errorf("Expected 3 consumers, got %d", stats.ConsumerCount)
	}

	if stats.CacheSize != 5 {
		t.Errorf("Expected cache size 5, got %d", stats.CacheSize)
	}
}

// TestNonBlockingPublish tests that slow consumers don't block publisher.
func TestNonBlockingPublish(t *testing.T) {
	ps := NewInMemoryPubSub()
	ps.SetConsumerBuf(2) // Small buffer
	defer ps.Close()

	topic := "test-topic"

	sub, err := ps.Subscribe(topic, "consumer1")
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}

	// Publish 10 messages quickly (consumer buffer is 2)
	start := time.Now()
	for i := 0; i < 10; i++ {
		msg := &Message{
			Payload:   i,
			Timestamp: time.Now(),
			Done:      false,
		}
		if err := ps.Publish(topic, msg); err != nil {
			t.Fatalf("Failed to publish: %v", err)
		}
	}
	elapsed := time.Since(start)

	// Should complete quickly (non-blocking)
	// Even with small consumer buffer, publisher should not block
	if elapsed > 100*time.Millisecond {
		t.Errorf("Publishing took too long: %v (publisher may have been blocked)", elapsed)
	}

	// Just verify we can still receive some messages from the buffer
	timeout := time.After(1 * time.Second)
	receivedCount := 0
	for receivedCount < 2 {
		select {
		case <-sub.Chan():
			receivedCount++
		case <-timeout:
			if receivedCount < 2 {
				t.Errorf("Could not receive 2 messages, got %d", receivedCount)
			}
			return
		}
	}
}

// TestAlreadySubscribed tests that re-subscribing returns existing subscription.
func TestAlreadySubscribed(t *testing.T) {
	ps := NewInMemoryPubSub()
	defer ps.Close()

	topic := "test-topic"
	consumerID := "consumer1"

	// Subscribe first time
	sub1, err := ps.Subscribe(topic, consumerID)
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}

	// Subscribe again with same ID
	sub2, err := ps.Subscribe(topic, consumerID)
	if err != nil {
		t.Fatalf("Failed to subscribe again: %v", err)
	}

	// Channels should be the same
	if sub1.Chan() != sub2.Chan() {
		t.Errorf("Re-subscribing should return the same subscription")
	}
}
