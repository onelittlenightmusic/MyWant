package mywant

// GetFirstInputChannel returns the first input channel from paths
// Returns (nil, true) if no input channels available (skip execution)
//
// Usage:
//   in, skipExec := w.GetFirstInputChannel()
//   if skipExec {
//       return true
//   }
//   // Use in directly without local variable assignment
//   data := (<-in).(DataType)
func (n *Want) GetFirstInputChannel() (chan interface{}, bool) {
	if n.paths.GetInCount() == 0 {
		return nil, true
	}
	return n.paths.In[0].Channel, false
}

// GetFirstOutputChannel returns the first output channel from paths
// Returns (nil, true) if no output channels available (skip execution)
//
// Usage:
//   out, skipExec := w.GetFirstOutputChannel()
//   if skipExec {
//       return true
//   }
//   // Use out directly without local variable assignment
//   out <- Data{Value: 42}
func (n *Want) GetFirstOutputChannel() (chan interface{}, bool) {
	if n.paths.GetOutCount() == 0 {
		return nil, true
	}
	return n.paths.Out[0].Channel, false
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
//   // Use in and out directly without local variable assignment
//   data := (<-in).(DataType)
//   out <- ProcessedData{Value: process(data)}
func (n *Want) GetInputAndOutputChannels() (chan interface{}, chan interface{}, bool) {
	if n.paths.GetInCount() == 0 {
		return nil, nil, true
	}

	in := n.paths.In[0].Channel
	var out chan interface{}
	if n.paths.GetOutCount() > 0 {
		out = n.paths.Out[0].Channel
	}

	return in, out, false
}
