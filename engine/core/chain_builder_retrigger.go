package mywant

import (
	"fmt"
	"log"
	"sort"
	"strings"
)

// selectorToKey converts a label selector map to a unique string key Used for label-to-users mapping in completed want detection Example: {role: "coordinator", stage: "final"} → "role:coordinator,stage:final"
func (cb *ChainBuilder) selectorToKey(selector map[string]string) string {
	if len(selector) == 0 {
		return ""
	}

	// Sort keys for consistent ordering
	keys := make([]string, 0, len(selector))
	for k := range selector {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s:%s", k, selector[k]))
	}
	return strings.Join(parts, ",")
}

func (cb *ChainBuilder) buildLabelToUsersMapping() {
	cb.labelToUsers = make(map[string][]string)

	for wantName, runtimeWant := range cb.wants {
		want := runtimeWant.want
		spec := want.GetSpec()
		if spec == nil || spec.Using == nil {
			continue
		}

		// For each "using" selector, record this want as a user
		for _, selector := range spec.Using {
			selectorKey := cb.selectorToKey(selector)
			if selectorKey != "" {
				cb.labelToUsers[selectorKey] = append(cb.labelToUsers[selectorKey], wantName)
			}
		}
	}
}

// RetriggerReceiverWant is called when a packet is provided to a receiver
// This is more reliable because it directly reflects execution state
func (cb *ChainBuilder) RetriggerReceiverWant(wantName string) {
	cb.reconcileMutex.RLock()
	runtimeWant, exists := cb.wants[wantName]
	cb.reconcileMutex.RUnlock()

	if !exists {
		InfoLog("[RETRIGGER-RECEIVER] WARNING: receiver want '%s' not found\n", wantName)
		return
	}

	want := runtimeWant.want

	// Use want's retrigger decision function to determine if retrigger is needed
	// This encapsulates the logic: check goroutine state and pending packets
	if want.ShouldRetrigger() {
		// Restart the want's execution (sets status to Idle)
		// The reconcile loop's startPhase() will detect the Idle status and restart the want
		// This avoids duplicate execution and keeps retrigger logic in one place
		want.RestartWant()

		if err := cb.TriggerReconcile(); err != nil {
			InfoLog("[RETRIGGER-RECEIVER] WARNING: failed to trigger reconcile for '%s': %v\n", wantName, err)
		}
	}
}

func (cb *ChainBuilder) checkAndRetriggerCompletedWants() {

	// Take snapshot of completed flags to avoid holding lock during notification
	cb.completedFlagsMutex.RLock()
	completedSnapshot := make(map[string]bool)
	for name, isCompleted := range cb.wantCompletedFlags {
		completedSnapshot[name] = isCompleted
	}
	cb.completedFlagsMutex.RUnlock()

	// Take snapshot of wants to avoid holding lock during SetStatus
	cb.reconcileMutex.RLock()
	wantSnapshot := make(map[string]*runtimeWant)
	for name, rw := range cb.wants {
		wantSnapshot[name] = rw
	}
	cb.reconcileMutex.RUnlock()
	anyWantRetriggered := false
	for wantID, isCompleted := range completedSnapshot {

		if isCompleted {
			InfoLog("[RETRIGGER:CHECK] Checking users for completed want ID '%s'\n", wantID)
			users := cb.findUsersOfCompletedWant(wantID)
			InfoLog("[RETRIGGER:CHECK] Found %d users for want ID '%s'\n", len(users), wantID)

			if len(users) > 0 {
				InfoLog("[RETRIGGER] Want ID '%s' completed, found %d users to retrigger\n", wantID, len(users))

				for _, userName := range users {
					// Restart dependent want so it can be re-executed This allows the want to pick up new data from the completed source
					if runtimeWant, ok := wantSnapshot[userName]; ok {
						runtimeWant.want.RestartWant()
						anyWantRetriggered = true
					}
				}
			}
		}
	}

	// If any want was retriggered, queue a reconciliation trigger (cannot call reconcileWants() directly due to mutex re-entrancy)
	if anyWantRetriggered {
		select {
		case cb.reconcileTrigger <- &TriggerCommand{Type: "reconcile"}:
			// Trigger queued successfully
		default:
			// Channel full, ignore (next reconciliation cycle will handle it)
		}
	}
}

func (cb *ChainBuilder) findUsersOfCompletedWant(completedWantID string) []string {
	var runtimeWant *runtimeWant
	for _, rw := range cb.wants {
		if rw.want.Metadata.ID == completedWantID {
			runtimeWant = rw
			break
		}
	}

	if runtimeWant == nil {
		return []string{}
	}

	completedWant := runtimeWant.want
	labels := completedWant.GetLabels()
	if len(labels) == 0 {
		return []string{}
	}

	// For each label in the completed want, find users
	users := make(map[string]bool) // De-duplicate users

	// Generate selector keys from completed want's labels and look up users in the pre-computed mapping
	for labelKey, labelValue := range labels {
		selector := map[string]string{labelKey: labelValue}
		selectorKey := cb.selectorToKey(selector)
		if selectorKey != "" {
			if usersForSelector, exists := cb.labelToUsers[selectorKey]; exists {
				for _, userName := range usersForSelector {
					users[userName] = true
				}
			}
		}
	}
	userList := make([]string, 0, len(users))
	for userName := range users {
		userList = append(userList, userName)
	}
	return userList
}

// UpdateCompletedFlag updates the completed flag for a want based on its status Called from Want.SetStatus() to track which wants are completed Uses mutex to protect concurrent access MarkWantCompleted is the new preferred method for wants to notify the ChainBuilder of completion
// MarkWantCompleted marks a want as completed using want ID Called by receiver wants (e.g., Coordinators) when they reach completion state Replaces the previous pattern where senders would call UpdateCompletedFlag
func (cb *ChainBuilder) MarkWantCompleted(wantID string, status WantStatus) {
	cb.completedFlagsMutex.Lock()
	defer cb.completedFlagsMutex.Unlock()

	isCompleted := (status == WantStatusAchieved)
	cb.wantCompletedFlags[wantID] = isCompleted
	log.Printf("[WANT-COMPLETED] Want ID '%s' notified completion with status=%s\n", wantID, status)
}

// UpdateCompletedFlag updates completion flag using want ID Deprecated: Use MarkWantCompleted instead
func (cb *ChainBuilder) UpdateCompletedFlag(wantID string, status WantStatus) {
	cb.completedFlagsMutex.Lock()
	defer cb.completedFlagsMutex.Unlock()

	isCompleted := (status == WantStatusAchieved)
	cb.wantCompletedFlags[wantID] = isCompleted
	log.Printf("[UPDATE-COMPLETED-FLAG] Want ID '%s' status=%s, isCompleted=%v\n", wantID, status, isCompleted)
}

// IsCompleted returns whether a want is currently in completed state Safe to call from any goroutine with RLock protection
func (cb *ChainBuilder) IsCompleted(wantID string) bool {
	cb.completedFlagsMutex.RLock()
	defer cb.completedFlagsMutex.RUnlock()
	return cb.wantCompletedFlags[wantID]
}

// TriggerCompletedWantRetriggerCheck sends a non-blocking trigger to the reconcile loop to check for completed wants and notify their dependents Uses the unified reconcileTrigger channel with Type="check_completed_retrigger"
func (cb *ChainBuilder) TriggerCompletedWantRetriggerCheck() {
	select {
	case cb.reconcileTrigger <- &TriggerCommand{
		Type: "check_completed_retrigger",
	}:
		// Trigger sent successfully InfoLog("[RETRIGGER:SEND] Non-blocking retrigger check trigger sent to reconcile loop\n")
	default:
		// Channel is full (rare), trigger is already pending
		InfoLog("[RETRIGGER:SEND] Warning: reconcileTrigger channel full, skipping trigger\n")
	}
}
