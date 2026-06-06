package mywant

import (
	"fmt"
	"mywant/engine/core/pubsub"
	"reflect"
	"sort"
	"strings"
	"time"
)

// want_io.go — channel I/O: Use/Provide/UnusedExists and PubSub helpers

// serializeLabels converts label map to deterministic topic name for PubSub routing.
// Ensures consistent topic names across publisher and subscribers.
// Example: {role: "processor", stage: "final"} → "role=processor,stage=final"
func serializeLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}

	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = fmt.Sprintf("%s=%s", k, labels[k])
	}
	return strings.Join(parts, ",")
}

// provideRaw sends a raw data packet to PubSub topic for subscribers.
// Internal use only — callers should use Provide(*DataObject) instead.
func (n *Want) provideRaw(payload any) {
	cb := GetGlobalChainBuilder()

	n.metadataMutex.RLock()
	topic := n.Metadata.Name // Fallback to name if no labels
	hasLabels := len(n.Metadata.Labels) > 0
	wantName := n.Metadata.Name

	if hasLabels {
		topic = serializeLabels(n.Metadata.Labels)
	}
	n.metadataMutex.RUnlock()

	if cb != nil && cb.pubsub != nil && hasLabels {
		msg := &pubsub.Message{
			Payload:   payload,
			Timestamp: time.Now(),
			Done:      false,
		}
		if err := cb.pubsub.Publish(topic, msg); err != nil {
			ErrorLog("[PubSub] Failed to publish packet from '%s' to topic '%s': %v",
				wantName, topic, err)
		}

		InfoLog("[PROVIDE] Want '%s' published packet to PubSub topic '%s'",
			wantName, topic)
	}
}

// ProvideDone sends a termination signal to PubSub topic
func (n *Want) ProvideDone() {
	cb := GetGlobalChainBuilder()

	n.metadataMutex.RLock()
	topic := n.Metadata.Name
	hasLabels := len(n.Metadata.Labels) > 0
	wantName := n.Metadata.Name

	if hasLabels {
		topic = serializeLabels(n.Metadata.Labels)
	}
	n.metadataMutex.RUnlock()

	if cb != nil && cb.pubsub != nil && hasLabels {
		msg := &pubsub.Message{
			Payload:   nil,
			Timestamp: time.Now(),
			Done:      true,
		}
		if err := cb.pubsub.Publish(topic, msg); err != nil {
			ErrorLog("[PubSub] Failed to publish Done signal from '%s' to topic '%s': %v",
				wantName, topic, err)
		}
		InfoLog("[PROVIDE_DONE] Want '%s' published Done signal to PubSub topic '%s'",
			wantName, topic)
	}
}

func (n *Want) GetType() string {
	return n.WantType
}
func (n *Want) GetConnectivityMetadata() ConnectivityMetadata {
	return n.ConnectivityMetadata
}
func (n *Want) GetInCount() int {
	return n.paths.GetInCount()
}
func (n *Want) GetOutCount() int {
	return n.paths.GetOutCount()
}
func (n *Want) GetPaths() *Paths {
	return &n.paths
}

// UnusedExists checks if there are unused packets in the cache or any input channel.
// It uses a single, blocking `reflect.Select` call to wait for a packet, which is
// then cached internally with its original channel index. This avoids polling loops.
// timeoutMs: wait time in milliseconds. If 0, performs a non-blocking check.
// Returns true if a packet is in the cache or received from a channel.
func (n *Want) UnusedExists(timeoutMs int) bool {
	n.cacheMutex.Lock()
	if n.cachedPacket != nil {
		n.cacheMutex.Unlock()
		return true
	}
	n.cacheMutex.Unlock()

	paths := n.GetPaths()
	if paths == nil || len(paths.In) == 0 {
		return false
	}

	cases := make([]reflect.SelectCase, 0, len(paths.In)+1)
	channelIndexMap := make([]int, 0, len(paths.In)+1)

	for i, pathInfo := range paths.In {
		if pathInfo.Channel != nil {
			cases = append(cases, reflect.SelectCase{
				Dir:  reflect.SelectRecv,
				Chan: reflect.ValueOf(pathInfo.Channel),
			})
			channelIndexMap = append(channelIndexMap, i)
		}
	}

	if len(cases) == 0 {
		return false
	}

	if timeoutMs > 0 {
		timeoutChan := time.After(time.Duration(timeoutMs) * time.Millisecond)
		timeoutCase := reflect.SelectCase{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(timeoutChan),
		}
		cases = append(cases, timeoutCase)
	} else {
		defaultCase := reflect.SelectCase{Dir: reflect.SelectDefault}
		cases = append(cases, defaultCase)
	}

	chosen, _, ok := reflect.Select(cases)

	isTimeout := (timeoutMs > 0 && chosen == len(cases)-1)
	isDefault := (timeoutMs <= 0 && chosen == len(cases)-1)

	if isTimeout || isDefault {
		if isTimeout {
			n.StoreLog("[UnusedExists] TIMEOUT after %dms (no packets found).\n", timeoutMs)
		}
		return false
	}

	if !ok {
		n.StoreLog("[UnusedExists] A channel was closed (index: %d).\n", chosen)
		return false
	}

	return true
}

// Use attempts to receive data from any available input channel.
// It first checks an internal cache (filled by UnusedExists) before attempting
// to receive from the channels directly.
//
// Timeout behavior:
//   - timeoutMilliseconds < 0: infinite wait (blocks until data arrives or channels close)
//   - timeoutMilliseconds == 0: non-blocking (returns immediately if no data available)
//   - timeoutMilliseconds > 0: wait up to specified milliseconds
//
// Returns: (channelIndex, data, done, ok)
//   - channelIndex: Index of the channel that provided data (-1 if no data available)
//   - data: The data received (nil if ok is false)
//   - done: True if the sender signalled completion
//   - ok: True if data was successfully received, false if timeout or no channels
//
// Usage:
//
//	index, data, done, ok := w.Use(1000)
//	if ok {
//	    if done { ... }
//	    fmt.Printf("Received data: %v\n", data)
//	}
func (n *Want) Use(timeoutMilliseconds int) (int, any, bool, bool) {
	var rawPacket any
	var originalIndex int
	var received bool

	// 1. Check internal cache first (filled by UnusedExists)
	n.cacheMutex.Lock()
	if n.cachedPacket != nil {
		cached := n.cachedPacket
		n.cachedPacket = nil // Consume from cache
		rawPacket = cached.Packet
		originalIndex = cached.OriginalIndex
		received = true
	}
	n.cacheMutex.Unlock()

	if !received {
		if len(n.paths.In) == 0 {
			return -1, nil, false, false
		}

		inCount := len(n.paths.In)
		cases := make([]reflect.SelectCase, 0, inCount+1)
		channelIndexMap := make([]int, 0, inCount+1) // Maps case index -> original channel index

		for i := 0; i < inCount; i++ {
			pathInfo := n.paths.In[i]
			if pathInfo.Channel != nil {
				cases = append(cases, reflect.SelectCase{
					Dir:  reflect.SelectRecv,
					Chan: reflect.ValueOf(pathInfo.Channel),
				})
				channelIndexMap = append(channelIndexMap, i)
			}
		}

		if len(cases) == 0 {
			return -1, nil, false, false
		}

		if timeoutMilliseconds >= 0 {
			timeoutChan := time.After(time.Duration(timeoutMilliseconds) * time.Millisecond)
			cases = append(cases, reflect.SelectCase{
				Dir: reflect.SelectRecv, Chan: reflect.ValueOf(timeoutChan),
			})
			channelIndexMap = append(channelIndexMap, -1) // -1 for timeout case
		}

		chosen, recv, recvOK := reflect.Select(cases)

		if chosen == len(cases)-1 && timeoutMilliseconds >= 0 {
			return -1, nil, false, false
		}

		if recvOK {
			originalIndex = channelIndexMap[chosen]
			rawPacket = recv.Interface()
			received = true

			n.storeState(fmt.Sprintf("packet_received_from_channel_%d", originalIndex), time.Now().Unix())
			n.storeState("last_packet_received_timestamp", getCurrentTimestamp())
		} else {
			return -1, nil, false, false
		}
	}

	if tp, ok := rawPacket.(TransportPacket); ok {
		return originalIndex, tp.Payload, tp.Done, true
	}

	return originalIndex, rawPacket, false, true
}

// UseForever attempts to receive data from any available input channel,
// blocking indefinitely until data arrives or all channels are closed.
// This is a convenience wrapper around Use(-1) for infinite wait.
func (n *Want) UseForever() (int, any, bool, bool) {
	return n.Use(-1)
}

// UseTyped calls Use() and auto-parses the result into a *DataObject.
// typeName specifies the expected data type (must be loaded in DataTypeLoader).
// If typeName is empty or DataTypeLoader is unavailable, wraps raw data in a generic DataObject.
func (n *Want) UseTyped(typeName string, timeoutMilliseconds int) (int, *DataObject, bool, bool) {
	idx, raw, done, ok := n.Use(timeoutMilliseconds)
	if !ok || done {
		return idx, nil, done, ok
	}
	loader := GetGlobalDataTypeLoader()
	if loader == nil || typeName == "" {
		return idx, wrapRaw(raw), done, ok
	}
	obj, err := loader.Parse(typeName, raw)
	if err != nil {
		WarnLog("[UseTyped] Failed to parse data as type %q: %v", typeName, err)
		return idx, wrapRaw(raw), done, ok
	}
	return idx, obj, done, ok
}

// UseForeverTyped calls UseForever() and auto-parses the result into a *DataObject.
func (n *Want) UseForeverTyped(typeName string) (int, *DataObject, bool, bool) {
	idx, raw, done, ok := n.UseForever()
	if !ok || done {
		return idx, nil, done, ok
	}
	loader := GetGlobalDataTypeLoader()
	if loader == nil || typeName == "" {
		return idx, wrapRaw(raw), done, ok
	}
	obj, err := loader.Parse(typeName, raw)
	if err != nil {
		WarnLog("[UseForeverTyped] Failed to parse data as type %q: %v", typeName, err)
		return idx, wrapRaw(raw), done, ok
	}
	return idx, obj, done, ok
}

// Provide validates the DataObject against its schema and sends it downstream.
// Validation errors are logged as warnings but do not block sending.
func (n *Want) Provide(obj *DataObject) {
	n.providedThisCycle = true
	if obj == nil {
		n.provideRaw(nil)
		return
	}
	loader := GetGlobalDataTypeLoader()
	if loader != nil && obj.TypeName() != "" {
		if err := loader.Validate(obj.TypeName(), obj.ToMap()); err != nil {
			WarnLog("[Provide] Validation warning for type %q: %v", obj.TypeName(), err)
		}
	}
	n.provideRaw(obj.ToMap())
}
