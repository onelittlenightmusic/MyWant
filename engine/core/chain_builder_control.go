package mywant

import (
	"fmt"
	"time"
)

// ============================== Suspend/Resume Control Methods ==============================

// SuspendWant suspends execution of a specific want and propagates to children
func (cb *ChainBuilder) SuspendWant(wantID string) error {
	cmd := &ControlCommand{
		Trigger:   ControlTriggerSuspend,
		WantID:    wantID,
		Timestamp: time.Now(),
		Reason:    "Suspended via API",
	}
	return cb.SendControlCommand(cmd)
}

// ResumeWant resumes execution of a specific want and propagates to children
func (cb *ChainBuilder) ResumeWant(wantID string) error {
	cmd := &ControlCommand{
		Trigger:   ControlTriggerResume,
		WantID:    wantID,
		Timestamp: time.Now(),
		Reason:    "Resumed via API",
	}
	return cb.SendControlCommand(cmd)
}

// StopWant stops execution of a specific want
func (cb *ChainBuilder) StopWant(wantID string) error {
	cmd := &ControlCommand{
		Trigger:   ControlTriggerStop,
		WantID:    wantID,
		Timestamp: time.Now(),
		Reason:    "Stopped via API",
	}
	return cb.SendControlCommand(cmd)
}

// RestartWant restarts execution of a specific want by setting its status to Idle
// This triggers the reconcile loop to re-run the want
func (cb *ChainBuilder) RestartWant(wantID string) error {
	// Find and restart the want by calling its RestartWant() method
	cb.reconcileMutex.RLock()
	var targetWant *Want
	for _, runtime := range cb.wants {
		if runtime.want.Metadata.ID == wantID {
			targetWant = runtime.want
			break
		}
	}
	cb.reconcileMutex.RUnlock()

	if targetWant == nil {
		return fmt.Errorf("want with ID %s not found", wantID)
	}

	// Call Want's RestartWant method which sets status to Idle
	targetWant.RestartWant()
	InfoLog("[RESTART:DEBUG] Want '%s' status now: %s\n", targetWant.Metadata.Name, targetWant.GetStatus())

	// Trigger reconciliation immediately to detect the Idle status and restart the goroutine
	select {
	case cb.reconcileTrigger <- &TriggerCommand{Type: "reconcile"}:
		InfoLog("[RESTART:DEBUG] Reconciliation trigger sent for '%s'\n", targetWant.Metadata.Name)
	default:
		InfoLog("[RESTART:DEBUG] Reconciliation trigger channel full for '%s'\n", targetWant.Metadata.Name)
	}

	return nil
}

func (cb *ChainBuilder) SendControlCommand(cmd *ControlCommand) error {
	select {
	case cb.reconcileTrigger <- &TriggerCommand{
		Type:           "control",
		ControlCommand: cmd,
	}:
		return nil
	default:
		return fmt.Errorf("failed to send control command - trigger channel full")
	}
}

// initializeSystemScheduler creates and starts the system scheduler Want
// This is called during startPhase to ensure the scheduler is always running
func (cb *ChainBuilder) initializeSystemScheduler() {
	// Check if scheduler already exists
	for _, want := range cb.wants {
		if want.want.Metadata.Type == "scheduler" {
			return // Scheduler already exists, nothing to do
		}
	}

	// Create a new Scheduler Want
	schedulerWant := &Want{
		Metadata: Metadata{
			ID:           generateUUID(),
			Name:         "system-scheduler",
			Type:         "scheduler",
			IsSystemWant: true, // Mark as system-managed want
			Labels: map[string]string{
				"system": "true",
				"role":   "scheduler",
			},
		},
		Spec: WantSpec{
			Params: Dict{
				"scan_interval": 60,
			},
		},
	}

	// Add the scheduler want asynchronously
	if err := cb.AddWantsAsync([]*Want{schedulerWant}); err != nil {
		InfoLog("[SYSTEM] Failed to initialize Scheduler Want: %v\n", err)
		return
	}

	InfoLog("[SYSTEM] System Scheduler Want initialized\n")
}

// Suspend pauses the execution of all wants (deprecated - use SuspendWant instead)
func (cb *ChainBuilder) Suspend() error {
	cb.suspended.Store(true)
	return nil
}

// Resume resumes the execution of all wants (deprecated - use ResumeWant instead)
func (cb *ChainBuilder) Resume() error {
	cb.suspended.Store(false)
	return nil
}

// IsSuspended returns the current suspension state
func (cb *ChainBuilder) IsSuspended() bool {
	return cb.suspended.Load()
}

// distributeControlCommand distributes a control command to target want(s) and propagates to child wants if the target is a parent want
func (cb *ChainBuilder) distributeControlCommand(cmd *ControlCommand) {
	cb.reconcileMutex.RLock()
	defer cb.reconcileMutex.RUnlock()
	var targetRuntime *runtimeWant
	for _, runtime := range cb.wants {
		if runtime.want.Metadata.ID == cmd.WantID {
			targetRuntime = runtime
			break
		}
	}

	if targetRuntime == nil {
		return
	}
	if err := targetRuntime.want.SendControlCommand(cmd); err != nil {
	} else {
	}

	// TODO: Propagate control to child wants if this is a parent want This will require finding parent-child relationships in the Target want implementation
}

// Stop stops execution by clearing all wants from the configuration
func (cb *ChainBuilder) Stop() error {
	// Clear the config wants which will trigger reconciliation to clean up
	cb.reconcileMutex.Lock()
	cb.config.Wants = []*Want{}
	cb.reconcileMutex.Unlock()

	// Trigger reconciliation to process the empty config
	select {
	case cb.reconcileTrigger <- &TriggerCommand{Type: "reconcile"}:
	default:
	}

	return nil
}

// Start restarts execution by triggering reconciliation of existing configuration
func (cb *ChainBuilder) Start() error {
	// Trigger reconciliation - this will reload from memory and restart wants
	select {
	case cb.reconcileTrigger <- &TriggerCommand{Type: "reconcile"}:
		return nil
	default:
		return fmt.Errorf("failed to trigger reconciliation - channel full")
	}
}

// IsRunning returns whether the chain has any active wants
func (cb *ChainBuilder) IsRunning() bool {
	cb.reconcileMutex.RLock()
	defer cb.reconcileMutex.RUnlock()
	return len(cb.wants) > 0
}

// TriggerReconcile triggers the reconciliation loop to process current config
func (cb *ChainBuilder) TriggerReconcile() error {
	select {
	case cb.reconcileTrigger <- &TriggerCommand{Type: "reconcile"}:
		return nil
	default:
		return fmt.Errorf("failed to trigger reconciliation - channel full")
	}
}

// DeleteWantByID removes a want from runtime by its ID If the want has children (based on ownerReferences), they will be deleted first (cascade deletion)
func (cb *ChainBuilder) DeleteWantByID(wantID string) error {
	// Phase 1: Identify if parent want exists
	cb.reconcileMutex.RLock()
	var found bool
	for _, rw := range cb.wants {
		if rw.want.Metadata.ID == wantID {
			found = true
			break
		}
	}
	cb.reconcileMutex.RUnlock()

	if !found {
		// Also check config if not in runtime
		cb.reconcileMutex.RLock()
		for _, cfgWant := range cb.config.Wants {
			if cfgWant.Metadata.ID == wantID {
				found = true
				break
			}
		}
		cb.reconcileMutex.RUnlock()
	}

	if !found {
		return fmt.Errorf("want with ID %s not found", wantID)
	}

	// Phase 2: Find all children first (cascade deletion) with read lock
	var childrenIDsToDelete []string

	cb.reconcileMutex.RLock()
	for _, runtimeWant := range cb.wants {
		if runtimeWant.want.Metadata.OwnerReferences != nil {
			for _, ownerRef := range runtimeWant.want.Metadata.OwnerReferences {
				if ownerRef.ID == wantID {
					childrenIDsToDelete = append(childrenIDsToDelete, runtimeWant.want.Metadata.ID)
					break
				}
			}
		}
	}
	cb.reconcileMutex.RUnlock()

	// Phase 3: Delete children first (with write lock for each deletion)
	for _, childID := range childrenIDsToDelete {
		cb.reconcileMutex.Lock()
		cb.deleteWantByID(childID)

		// Also remove child from config to keep it in sync with cb.wants
		for i, cfgWant := range cb.config.Wants {
			if cfgWant.Metadata.ID == childID {
				cb.config.Wants = append(cb.config.Wants[:i], cb.config.Wants[i+1:]...)
				break
			}
		}
		cb.reconcileMutex.Unlock()
	}

	// Phase 4: Delete the parent want (with write lock)
	cb.reconcileMutex.Lock()
	cb.deleteWantByID(wantID)

	// Also remove from config so detectConfigChanges sees the deletion
	// Remove ALL occurrences of the want ID (handles duplicates in config)
	newWants := make([]*Want, 0, len(cb.config.Wants))
	for _, cfgWant := range cb.config.Wants {
		if cfgWant.Metadata.ID != wantID {
			newWants = append(newWants, cfgWant)
		}
	}
	cb.config.Wants = newWants

	// Persist configuration change to memory file
	if err := cb.copyConfigToMemory(); err != nil {
		// Log error but don't fail the deletion operation itself as in-memory state is correct
		fmt.Printf("[ERROR] Failed to persist config after deleting want %s: %v\n", wantID, err)
	}

	cb.reconcileMutex.Unlock()

	return nil
}
