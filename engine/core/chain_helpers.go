package mywant

import (
	"mywant/engine/core/chain"
)

// in, connectionAvailable := w.GetInputChannel(0) if !connectionAvailable { return true }
//
//	data := (<-in).(DataType)
func (n *Want) GetInputChannel(index int) (chain.Chan, bool) {
	if index < 0 || index >= n.paths.GetInCount() {
		return nil, false
	}
	return n.paths.In[index].Channel, true
}

// out, connectionAvailable := w.GetOutputChannel(0) if !connectionAvailable { return true }
//
//	out <- Data{Value: 42}
func (n *Want) GetOutputChannel(index int) (chain.Chan, bool) {
	if index < 0 || index >= n.paths.GetOutCount() {
		return nil, false
	}
	return n.paths.Out[index].Channel, true
}

// in, connectionAvailable := w.GetFirstInputChannel() if !connectionAvailable { return true }
//
//	data := (<-in).(DataType)
func (n *Want) GetFirstInputChannel() (chain.Chan, bool) {
	return n.GetInputChannel(0)
}

// out, connectionAvailable := w.GetFirstOutputChannel() if !connectionAvailable { return true }
//
//	out <- Data{Value: 42}
func (n *Want) GetFirstOutputChannel() (chain.Chan, bool) {
	return n.GetOutputChannel(0)
}

// w.SetPaths(inPaths, outPaths)
func (n *Want) SetPaths(inPaths, outPaths []PathInfo) {
	n.paths.In = inPaths
	n.paths.Out = outPaths
}
