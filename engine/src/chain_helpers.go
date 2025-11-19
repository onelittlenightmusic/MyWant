package mywant

import (
	"mywant/engine/src/chain"
)

// GetInputChannel returns input channel by index
// Returns (nil, true) if not available
//
// Usage:
//   in, skipExec := w.GetInputChannel(0)
//   if skipExec {
//       return true
//   }
//   data := (<-in).(DataType)
func (n *Want) GetInputChannel(index int) (chain.Chan, bool) {
	if index < 0 || index >= n.paths.GetInCount() {
		return nil, true
	}
	return n.paths.In[index].Channel, false
}

// GetOutputChannel returns output channel by index
// Returns (nil, true) if not available
//
// Usage:
//   out, skipExec := w.GetOutputChannel(0)
//   if skipExec {
//       return true
//   }
//   out <- Data{Value: 42}
func (n *Want) GetOutputChannel(index int) (chain.Chan, bool) {
	if index < 0 || index >= n.paths.GetOutCount() {
		return nil, true
	}
	return n.paths.Out[index].Channel, false
}

// GetFirstInputChannel returns the first input channel from paths
// Returns (nil, true) if no input channels available (skip execution)
//
// Usage:
//   in, skipExec := w.GetFirstInputChannel()
//   if skipExec {
//       return true
//   }
//   data := (<-in).(DataType)
func (n *Want) GetFirstInputChannel() (chain.Chan, bool) {
	return n.GetInputChannel(0)
}

// GetFirstOutputChannel returns the first output channel from paths
// Returns (nil, true) if no output channels available (skip execution)
//
// Usage:
//   out, skipExec := w.GetFirstOutputChannel()
//   if skipExec {
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
