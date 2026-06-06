package mywant

import (
	"context"
	"reflect"
)

// want_loop.go — StartProgressionLoop helpers: control signal handling,
// progress execution, and post-execution status checks.

// stopAgents stops all background agents and logs any error with the given label.
// Used throughout the progression loop to DRY up the repeated stop+log pattern.
func (n *Want) stopAgents(label string) {
	if err := n.StopAllBackgroundAgents(); err != nil {
		n.StoreLog("ERROR: Failed to stop background agents %s: %v", label, err)
	}
}

// initializeForRun resets per-run state, calls Initialize() on the progressable,
// syncs locals, and starts background agents. Used at loop startup and on ControlTriggerRestart.
func (n *Want) initializeForRun() {
	n.prepareForRestart()
	if n.progressable != nil {
		n.progressable.Initialize()
		n.syncLocalsAfterInitialize()
	}
	if err := n.StartBackgroundAgents(); err != nil {
		n.StoreLog("ERROR: Failed to start background agents: %v", err)
	}
}

// loopSignal is a sentinel returned by loop-phase helpers to direct the main goroutine.
type loopSignal int

const (
	loopSignalNone     loopSignal = iota // continue to next step
	loopSignalReturn                     // exit the goroutine
	loopSignalContinue                   // jump to next loop iteration
)

// handleControlSignal processes a single ControlCommand and returns the action the
// goroutine should take. The caller must check the returned signal:
//
//	loopSignalReturn   → return from goroutine
//	loopSignalContinue → continue to next iteration
//	loopSignalNone     → proceed normally
func (n *Want) handleControlSignal(cmd *ControlCommand) loopSignal {
	switch cmd.Trigger {
	case ControlTriggerSuspend:
		n.SetSuspended(true)
		n.SetStatus(WantStatusSuspended)
		return loopSignalNone

	case ControlTriggerResume:
		n.SetSuspended(false)
		n.SetStatus(WantStatusReaching)
		return loopSignalNone

	case ControlTriggerStop:
		n.SetStatus(WantStatusTerminated)
		return loopSignalReturn

	case ControlTriggerRestart:
		n.SetSuspended(false)
		n.stopAgents("on restart")
		n.initializeForRun()
		return loopSignalContinue
	}
	return loopSignalNone
}

// runProgressWithRecovery syncs locals, calls Progress(), and recovers from
// ModuleErrorPanic / ConfigErrorPanic.
// Returns true if the goroutine should exit (ModuleErrorPanic).
func (n *Want) runProgressWithRecovery() (exitLoop bool) {
	defer func() {
		if r := recover(); r != nil {
			switch r.(type) {
			case ModuleErrorPanic:
				// Status already set by SetModuleErrorAndExit()
				n.stopAgents("")
				exitLoop = true
			case ConfigErrorPanic:
				// Status already set by SetConfigErrorAndExit()
				// Handled in next iteration (wait for config update)
			default:
				panic(r) // unexpected panic — bubble up
			}
		}
	}()

	// Declarative mapping: State → Locals
	var locals any
	if progressableVal := reflect.ValueOf(n.progressable); progressableVal.Kind() == reflect.Pointer {
		method := progressableVal.MethodByName("GetLocals")
		if method.IsValid() && method.Type().NumIn() == 0 && method.Type().NumOut() == 1 {
			results := method.Call(nil)
			locals = results[0].Interface()
			SyncLocalsState(n, locals, true)
		}
	}

	n.progressable.Progress()

	// Declarative mapping: Locals → State
	if locals != nil {
		SyncLocalsState(n, locals, false)
	}
	return false
}

// checkPostProgressStatus inspects want status and failable/achieved state after
// Progress() completes. Returns:
//
//	loopSignalReturn   → goroutine should exit
//	loopSignalContinue → jump to next iteration
//	loopSignalNone     → continue normally
func (n *Want) checkPostProgressStatus() loopSignal {
	currentStatus := n.GetStatus()

	if currentStatus == WantStatusModuleError {
		n.stopAgents("on module error")
		return loopSignalReturn
	}

	if currentStatus == WantStatusCancelled {
		n.stopAgents("on cancel")
		return loopSignalReturn
	}

	if currentStatus == WantStatusConfigError {
		return loopSignalContinue
	}

	// Check failable state
	if failable, ok := n.progressable.(Failable); ok && failable.IsFailed() {
		n.SetStatus(WantStatusFailed)
		n.FlushThinkingAgents(context.Background())
		n.stopAgents("on failed (post-progress)")
		return loopSignalReturn
	}

	// Check achieved state
	if n.progressable != nil && n.progressable.IsAchieved() {
		n.SetStatus(WantStatusAchieved)
		n.FlushThinkingAgents(context.Background())
		n.stopAgents("on achieved (post-progress)")
		return loopSignalReturn
	}

	return loopSignalNone
}
