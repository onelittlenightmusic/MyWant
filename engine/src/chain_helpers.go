package mywant

import (
	"mywant/engine/src/chain"
	"reflect"
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
func (n *Want) ReceiveFromAnyInputChannel() (int, interface{}, bool) {
	inCount := n.GetInCount()
	if inCount == 0 {
		return -1, nil, false
	}

	// Build select cases dynamically for all input channels
	cases := make([]reflect.SelectCase, inCount)
	for i := 0; i < inCount; i++ {
		ch, available := n.GetInputChannel(i)
		if available {
			cases[i] = reflect.SelectCase{
				Dir:  reflect.SelectRecv,
				Chan: reflect.ValueOf(ch),
			}
		} else {
			// Channel not available, use a nil case (will never match)
			cases[i] = reflect.SelectCase{
				Dir: reflect.SelectRecv,
			}
		}
	}

	// Use reflect.Select with default to make it non-blocking
	// We need to add a default case to make it non-blocking
	cases = append(cases, reflect.SelectCase{
		Dir: reflect.SelectDefault,
	})

	chosen, recv, recvOK := reflect.Select(cases)

	// If default case was chosen (last index), no data available
	if chosen == len(cases)-1 {
		return -1, nil, false
	}

	// If we got here, data was received
	if recvOK {
		return chosen, recv.Interface(), true
	}

	return chosen, nil, false
}
