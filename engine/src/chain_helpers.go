package mywant

import (
	"mywant/engine/src/chain"
	"reflect"
	"time"
)

// GetInputChannel returns input channel by index
// Returns (nil, false) if not available, (channel, true) if available
//
// Usage:
//   in, connectionAvailable := w.GetInputChannel(0)
//   if !connectionAvailable {
//       return true
//   }
//   data := (<-in).(DataType)
func (n *Want) GetInputChannel(index int) (chain.Chan, bool) {
	if index < 0 || index >= n.paths.GetInCount() {
		return nil, false
	}
	return n.paths.In[index].Channel, true
}

// GetOutputChannel returns output channel by index
// Returns (nil, false) if not available, (channel, true) if available
//
// Usage:
//   out, connectionAvailable := w.GetOutputChannel(0)
//   if !connectionAvailable {
//       return true
//   }
//   out <- Data{Value: 42}
func (n *Want) GetOutputChannel(index int) (chain.Chan, bool) {
	if index < 0 || index >= n.paths.GetOutCount() {
		return nil, false
	}
	return n.paths.Out[index].Channel, true
}

// GetFirstInputChannel returns the first input channel from paths
// Returns (nil, false) if no input channels available, (channel, true) if available
//
// Usage:
//   in, connectionAvailable := w.GetFirstInputChannel()
//   if !connectionAvailable {
//       return true
//   }
//   data := (<-in).(DataType)
func (n *Want) GetFirstInputChannel() (chain.Chan, bool) {
	return n.GetInputChannel(0)
}

// GetFirstOutputChannel returns the first output channel from paths
// Returns (nil, false) if no output channels available, (channel, true) if available
//
// Usage:
//   out, connectionAvailable := w.GetFirstOutputChannel()
//   if !connectionAvailable {
//       return true
//   }
//   out <- Data{Value: 42}
func (n *Want) GetFirstOutputChannel() (chain.Chan, bool) {
	return n.GetOutputChannel(0)
}

// SetPaths sets both input and output paths for this want
// This is the proper way to synchronize paths instead of direct field access
//
// Usage:
//   w.SetPaths(inPaths, outPaths)
func (n *Want) SetPaths(inPaths, outPaths []PathInfo) {
	n.paths.In = inPaths
	n.paths.Out = outPaths
}

// ReceiveFromAnyInputChannel attempts to receive data from any available input channel
// using non-blocking select. Returns the channel index that had data, the data itself,
// and whether a successful read occurred.
//
// This function directly accesses all input channels from paths and constructs a
// dynamic select statement to watch all channels asynchronously without iterating
// through GetInputChannel(i).
//
// Returns: (channelIndex, data, ok)
//   - channelIndex: Index of the channel that provided data (-1 if no data available)
//   - data: The data received (nil if ok is false)
//   - ok: True if data was successfully received, false otherwise
//
// Usage:
//   index, data, ok := w.ReceiveFromAnyInputChannel()
//   if ok {
//       fmt.Printf("Received data from channel %d: %v\n", index, data)
//   }
func (n *Want) ReceiveFromAnyInputChannel(timeoutMilliseconds int) (int, interface{}, bool) {
	// Access input channels directly from paths structure
	if len(n.paths.In) == 0 {
		return -1, nil, false
	}

	inCount := len(n.paths.In)

	// Build select cases dynamically for all input channels from paths
	// IMPORTANT: Must track the original channel index mapping!
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

	timeoutChan := time.After(time.Duration(timeoutMilliseconds) * time.Millisecond)
	// Add default case for non-blocking behavior
	cases = append(cases, reflect.SelectCase{
		Dir: reflect.SelectRecv, Chan: reflect.ValueOf(timeoutChan),
	})
	// Default case doesn't map to a channel index, but we track it anyway
	channelIndexMap = append(channelIndexMap, -1)

	chosen, recv, recvOK := reflect.Select(cases)

	// If default case was chosen (last index), no data available
	if chosen == len(cases)-1 {
		return -1, nil, false
	}

	// If we got here, data was received
	// IMPORTANT: Return the ORIGINAL channel index, not the case index!
	if recvOK {
		originalIndex := channelIndexMap[chosen]
		return originalIndex, recv.Interface(), true
	}

	return channelIndexMap[chosen], nil, false
}
