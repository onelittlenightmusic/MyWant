package mywant

import (
	"mywant/engine/src/chain"
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
