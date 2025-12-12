package mywant

import (
	"mywant/engine/src/chain"
	"reflect"
	"time"
)
// in, connectionAvailable := w.GetInputChannel(0) if !connectionAvailable { return true }
//   data := (<-in).(DataType)
func (n *Want) GetInputChannel(index int) (chain.Chan, bool) {
	if index < 0 || index >= n.paths.GetInCount() {
		return nil, false
	}
	return n.paths.In[index].Channel, true
}
// out, connectionAvailable := w.GetOutputChannel(0) if !connectionAvailable { return true }
//   out <- Data{Value: 42}
func (n *Want) GetOutputChannel(index int) (chain.Chan, bool) {
	if index < 0 || index >= n.paths.GetOutCount() {
		return nil, false
	}
	return n.paths.Out[index].Channel, true
}
// in, connectionAvailable := w.GetFirstInputChannel() if !connectionAvailable { return true }
//   data := (<-in).(DataType)
func (n *Want) GetFirstInputChannel() (chain.Chan, bool) {
	return n.GetInputChannel(0)
}
// out, connectionAvailable := w.GetFirstOutputChannel() if !connectionAvailable { return true }
//   out <- Data{Value: 42}
func (n *Want) GetFirstOutputChannel() (chain.Chan, bool) {
	return n.GetOutputChannel(0)
}
//   w.SetPaths(inPaths, outPaths)
func (n *Want) SetPaths(inPaths, outPaths []PathInfo) {
	n.paths.In = inPaths
	n.paths.Out = outPaths
}

// ReceiveFromAnyInputChannel attempts to receive data from any available input channel using non-blocking select. Returns the channel index that had data, the data itself, and whether a successful read occurred.
// This function directly accesses all input channels from paths and constructs a dynamic select statement to watch all channels asynchronously without iterating through GetInputChannel(i).
//
// Timeout behavior:
//   - timeoutMilliseconds < 0: infinite wait (blocks until data arrives or channels close)
//   - timeoutMilliseconds == 0: non-blocking (returns immediately if no data available)
//   - timeoutMilliseconds > 0: wait up to specified milliseconds
//
// fmt.Printf("Received data from channel %d: %v\n", index, data) }
func (n *Want) ReceiveFromAnyInputChannel(timeoutMilliseconds int) (int, interface{}, bool) {
	// Access input channels directly from paths structure
	if len(n.paths.In) == 0 {
		return -1, nil, false
	}

	inCount := len(n.paths.In)
	cases := make([]reflect.SelectCase, 0, inCount+1)
	channelIndexMap := make([]int, 0, inCount+1)  // Maps case index -> original channel index

	for i := 0; i < inCount; i++ {
		pathInfo := n.paths.In[i]
		if pathInfo.Channel != nil {
			cases = append(cases, reflect.SelectCase{
				Dir:  reflect.SelectRecv,
				Chan: reflect.ValueOf(pathInfo.Channel),
			})
			channelIndexMap = append(channelIndexMap, i)  // Track which channel index this case corresponds to
		}
	}

	// If no valid channels found, return immediately
	if len(cases) == 0 {
		return -1, nil, false
	}

	// Handle timeout:
	// - Negative timeout: infinite wait (no timeout case added)
	// - Zero timeout: non-blocking (add immediate timeout)
	// - Positive timeout: wait up to specified milliseconds
	if timeoutMilliseconds >= 0 {
		timeoutChan := time.After(time.Duration(timeoutMilliseconds) * time.Millisecond)
		cases = append(cases, reflect.SelectCase{
			Dir: reflect.SelectRecv, Chan: reflect.ValueOf(timeoutChan),
		})
		channelIndexMap = append(channelIndexMap, -1)
	}

	chosen, recv, recvOK := reflect.Select(cases)

	// If timeout case was chosen (last index), no data available
	if chosen == len(cases)-1 {
		return -1, nil, false
	}

	// If we got here, data was received - Return the ORIGINAL channel index, not the case index!
	if recvOK {
		originalIndex := channelIndexMap[chosen]
		return originalIndex, recv.Interface(), true
	}

	return channelIndexMap[chosen], nil, false
}

// ReceiveFromAnyInputChannelForever attempts to receive data from any available input channel,
// blocking indefinitely until data arrives or all channels are closed.
// This is a convenience wrapper around ReceiveFromAnyInputChannel(-1) for infinite wait.
//
// Returns: (channelIndex, data, ok)
//   - channelIndex: Index of the channel that provided data (-1 if channels closed)
//   - data: The data received (nil if ok is false)
//   - ok: True if data was successfully received, false if all channels are closed
//
// Usage:
//   index, data, ok := w.ReceiveFromAnyInputChannelForever()
//   if ok {
//       fmt.Printf("Received data from channel %d: %v\n", index, data)
//   } else {
//       // All input channels are closed
//   }
func (n *Want) ReceiveFromAnyInputChannelForever() (int, interface{}, bool) {
	return n.ReceiveFromAnyInputChannel(-1)
}
