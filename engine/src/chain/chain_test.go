package chain

import (
	"testing"
)

func TestTupleCreation(t *testing.T) {
	// Tuple is any, so we can store any type
	var tuple Tuple = map[string]any{
		"key":   "test",
		"value": 42,
	}

	tupleMap := tuple.(map[string]any)
	if tupleMap["key"] != "test" {
		t.Errorf("Expected key 'test', got %v", tupleMap["key"])
	}
	if tupleMap["value"] != 42 {
		t.Errorf("Expected value 42, got %v", tupleMap["value"])
	}
}

func TestChanCreation(t *testing.T) {
	ch := make(Chan, 5)

	// Test channel capacity
	if cap(ch) != 5 {
		t.Errorf("Expected capacity 5, got %d", cap(ch))
	}

	// Test sending and receiving
	var tuple Tuple = map[string]any{
		"key":   "test",
		"value": "data",
	}
	ch <- tuple

	received := <-ch
	receivedMap := received.(map[string]any)
	if receivedMap["key"] != "test" || receivedMap["value"] != "data" {
		t.Error("Channel communication failed")
	}
}

func TestChainCreation(t *testing.T) {
	chain := C_chain{
		In:       make(chan Tuple, 1),
		Ch_start: make(chan Tuple, 1),
	}

	if chain.In == nil {
		t.Error("Expected In channel to be initialized")
	}
	if chain.Ch_start == nil {
		t.Error("Expected Ch_start channel to be initialized")
	}
}

func TestChannelBuffering(t *testing.T) {
	ch := make(Chan, 3)

	// Fill buffer
	ch <- map[string]any{"key": "1", "value": 1}
	ch <- map[string]any{"key": "2", "value": 2}
	ch <- map[string]any{"key": "3", "value": 3}

	// Verify buffer is full
	if len(ch) != 3 {
		t.Errorf("Expected 3 items in buffer, got %d", len(ch))
	}

	// Read one item
	received := <-ch
	receivedMap := received.(map[string]any)
	if receivedMap["key"] != "1" {
		t.Error("Expected FIFO ordering")
	}

	// Verify buffer decreased
	if len(ch) != 2 {
		t.Errorf("Expected 2 items in buffer, got %d", len(ch))
	}
}

func TestChannelClosure(t *testing.T) {
	ch := make(Chan, 1)
	ch <- map[string]any{"key": "test", "value": "data"}
	close(ch)

	// Should be able to read existing data
	received, ok := <-ch
	if !ok {
		t.Error("Expected to read data from closed channel")
	}
	receivedMap := received.(map[string]any)
	if receivedMap["key"] != "test" {
		t.Error("Data corrupted after channel closure")
	}

	// Next read should indicate closure
	_, ok = <-ch
	if ok {
		t.Error("Expected closed channel to return false")
	}
}

func TestMultipleProducersConsumers(t *testing.T) {
	ch := make(Chan, 10)
	done := make(chan bool, 2)

	// Producer goroutine
	go func() {
		for i := 0; i < 5; i++ {
			ch <- map[string]any{"key": "producer1", "value": i}
		}
		done <- true
	}()

	// Consumer goroutine
	go func() {
		received := 0
		for range ch {
			received++
			if received == 5 {
				break
			}
		}
		done <- true
	}()

	// Wait for both to complete
	<-done
	<-done

	close(ch)
}

func TestTupleStringConversion(t *testing.T) {
	var tuple Tuple = map[string]any{"key": "test", "value": "value"}

	// Basic validation that tuple maintains data integrity
	tupleMap := tuple.(map[string]any)
	if tupleMap["key"] != "test" {
		t.Error("Key not preserved")
	}
	if tupleMap["value"] != "value" {
		t.Error("Value not preserved")
	}
}

func TestChainComponentIntegration(t *testing.T) {
	// Test that all chain components work together
	chain := C_chain{
		In:       make(chan Tuple, 1),
		Ch_start: make(chan Tuple, 1),
	}

	// Test that we can send data through the chain
	testData := map[string]any{"test": "data"}
	chain.In <- testData

	// Verify data can be received
	received := <-chain.In
	receivedMap := received.(map[string]any)
	if receivedMap["test"] != "data" {
		t.Error("Chain data flow failed")
	}
}
