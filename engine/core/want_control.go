package mywant

import "fmt"

// want_control.go — control channel (suspend/resume/restart signals)

func (n *Want) InitializeControlChannel() {
	// Always create a fresh channel so that stale commands (e.g. ControlTriggerStop from a
	// previous stop→start sequence) are not inherited by the new goroutine.
	// Called only from startWant(), which guards against calling this while the goroutine is active.
	n.controlChannel = make(chan *ControlCommand, 10)
}

func (n *Want) SendControlCommand(cmd *ControlCommand) error {
	if n.controlChannel == nil {
		return fmt.Errorf("control channel not initialized for want %s", n.Metadata.Name)
	}
	select {
	case n.controlChannel <- cmd:
		return nil
	default:
		return fmt.Errorf("control channel full for want %s", n.Metadata.Name)
	}
}

func (n *Want) CheckControlSignal() (*ControlCommand, bool) {
	if n.controlChannel == nil {
		return nil, false
	}
	select {
	case cmd := <-n.controlChannel:
		return cmd, true
	default:
		return nil, false
	}
}

// IsSuspended returns whether the want is currently suspended.
func (n *Want) IsSuspended() bool {
	return n.suspended.Load()
}

func (n *Want) SetSuspended(suspended bool) {
	n.suspended.Store(suspended)
}
