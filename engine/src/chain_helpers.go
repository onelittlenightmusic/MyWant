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
		return nil, false // Channel not available (out of bounds)
	}
	return n.paths.In[index].Channel, n.paths.In[index].Active
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

// GetInputAndOutputChannels returns both input and output channels from paths
// Returns (nil, nil, true) if input is not available (skip execution)
// Returns (in, nil, false) if output is not available (check if needed)
//
// Usage:
//   in, out, skipExec := w.GetInputAndOutputChannels()
//   if skipExec {
//       return true
//   }
//   if out == nil {
//       return true  // if output is required
//   }
//   data := (<-in).(DataType)
//   out <- ProcessedData{Value: process(data)}
func (n *Want) GetInputAndOutputChannels() (chain.Chan, chain.Chan, bool) {
	return n.GetInputAndOutputChannelsAt(0, 0)
}

// GetInputAndOutputChannelsAt returns both input and output channels at specified indices
// Returns (nil, nil, true) if input is not available (skip execution)
// Returns (in, nil, false) if output is not available (check if needed)
//
// Usage:
//   in, out, skipExec := w.GetInputAndOutputChannelsAt(0, 1)
//   if skipExec {
//       return true
//   }
func (n *Want) GetInputAndOutputChannelsAt(inIndex, outIndex int) (chain.Chan, chain.Chan, bool) {
	in, skipExec := n.GetInputChannel(inIndex)
	if skipExec {
		return nil, nil, true
	}

	out, _ := n.GetOutputChannel(outIndex)
	return in, out, false
}

// SetPaths sets both input and output paths for this want
// This is the proper way to synchronize paths instead of direct field access
//
// Usage:
//   w.SetPaths(inPaths, outPaths)
func (n *Want) SetPaths(inPaths, outPaths []PathInfo) {
	n.paths.In = inPaths
	n.paths.Out = outPaths
	// Add debug logging to confirm the active status of the channels being set
	firstInActive := false
	if len(n.paths.In) > 0 {
		firstInActive = n.paths.In[0].Active
	}
	firstOutActive := false
	if len(n.paths.Out) > 0 {
		firstOutActive = n.paths.Out[0].Active
	}
	DebugLog("[WANT:SET_PATHS] Want %s: Set In=%d, Out=%d. First In channel active: %v, First Out channel active: %v\n",
		n.Metadata.Name, len(n.paths.In), len(n.paths.Out), firstInActive, firstOutActive)
}
