package mywant

import (
	"fmt"
	"time"
)

// QueueOperation queues an operation for processing by reconcile loop (non-blocking with default case)
// Returns error if channel is full
func (cb *ChainBuilder) QueueOperation(op *WantOperation) error {
	if op == nil {
		return fmt.Errorf("operation cannot be nil")
	}
	select {
	case cb.operationChan <- op:
		return nil
	default:
		return fmt.Errorf("operation queue full (buffer size: 20)")
	}
}

// QueueWantAdd queues a want addition operation
func (cb *ChainBuilder) QueueWantAdd(wants []*Want) error {
	if len(wants) == 0 {
		return fmt.Errorf("wants list cannot be empty")
	}
	return cb.QueueOperation(&WantOperation{
		Type:       "add",
		EntityType: "want",
		Wants:      wants,
	})
}

// QueueWantDelete queues a want deletion operation
func (cb *ChainBuilder) QueueWantDelete(ids []string) error {
	if len(ids) == 0 {
		return fmt.Errorf("want IDs cannot be empty")
	}
	return cb.QueueOperation(&WantOperation{
		Type:       "delete",
		EntityType: "want",
		IDs:        ids,
	})
}

// QueueWantSuspend queues a want suspension operation
func (cb *ChainBuilder) QueueWantSuspend(ids []string) error {
	if len(ids) == 0 {
		return fmt.Errorf("want IDs cannot be empty")
	}
	return cb.QueueOperation(&WantOperation{
		Type:       "suspend",
		EntityType: "want",
		IDs:        ids,
	})
}

// QueueWantResume queues a want resume operation
func (cb *ChainBuilder) QueueWantResume(ids []string) error {
	if len(ids) == 0 {
		return fmt.Errorf("want IDs cannot be empty")
	}
	return cb.QueueOperation(&WantOperation{
		Type:       "resume",
		EntityType: "want",
		IDs:        ids,
	})
}

// QueueWantStop queues a want stop operation
func (cb *ChainBuilder) QueueWantStop(ids []string) error {
	if len(ids) == 0 {
		return fmt.Errorf("want IDs cannot be empty")
	}
	return cb.QueueOperation(&WantOperation{
		Type:       "stop",
		EntityType: "want",
		IDs:        ids,
	})
}

// QueueWantStart queues a want start operation
func (cb *ChainBuilder) QueueWantStart(ids []string) error {
	if len(ids) == 0 {
		return fmt.Errorf("want IDs cannot be empty")
	}
	return cb.QueueOperation(&WantOperation{
		Type:       "start",
		EntityType: "want",
		IDs:        ids,
	})
}

// QueueWantAddLabel queues a label addition operation
func (cb *ChainBuilder) QueueWantAddLabel(wantID, key, value string) error {
	if wantID == "" || key == "" {
		return fmt.Errorf("want ID and label key cannot be empty")
	}
	cb.AddLabelToRegistry(key, value)
	return cb.QueueOperation(&WantOperation{
		Type:       "addLabel",
		EntityType: "want",
		IDs:        []string{wantID},
		Data: map[string]any{
			"key":   key,
			"value": value,
		},
	})
}

// QueueWantRemoveLabel queues a label removal operation
func (cb *ChainBuilder) QueueWantRemoveLabel(wantID, key string) error {
	if wantID == "" || key == "" {
		return fmt.Errorf("want ID and label key cannot be empty")
	}
	return cb.QueueOperation(&WantOperation{
		Type:       "removeLabel",
		EntityType: "want",
		IDs:        []string{wantID},
		Data: map[string]any{
			"key": key,
		},
	})
}

// processWantOperation processes a queued want operation (suspend, resume, stop, start, labels, etc.)
func (cb *ChainBuilder) processWantOperation(op *WantOperation) {
	if op == nil {
		return
	}

	// Helper function to send error back to callback channel non-blocking
	sendError := func(err error) {
		if op.Callback != nil {
			select {
			case op.Callback <- err:
			default:
				// Channel full or closed, silently drop (non-blocking)
			}
		}
	}

	switch op.Type {
	case "add":
		// Add new wants
		if len(op.Wants) > 0 {
			cb.reconcileMutex.Lock()
			for _, want := range op.Wants {
				cb.config.Wants = append(cb.config.Wants, want)
			}
			cb.reconcileMutex.Unlock()
			// Trigger reconciliation to connect and start new wants
			cb.reconcileWants()
		}

	case "delete":
		// Delete wants
		if len(op.IDs) > 0 {
			deletedCount := 0
			for _, wantID := range op.IDs {
				if err := cb.DeleteWantByID(wantID); err != nil {
					// Continue deleting others even if one fails
				} else {
					deletedCount++
				}
			}
			if deletedCount > 0 {
				// Trigger reconciliation after deletion
				cb.reconcileWants()
			}
		}

	case "suspend":
		// Suspend wants
		for _, wantID := range op.IDs {
			if err := cb.SuspendWant(wantID); err != nil {
				sendError(err)
				return
			}
		}

	case "resume":
		// Resume wants
		for _, wantID := range op.IDs {
			if err := cb.ResumeWant(wantID); err != nil {
				sendError(err)
				return
			}
		}

	case "stop":
		// Stop wants
		for _, wantID := range op.IDs {
			if err := cb.StopWant(wantID); err != nil {
				sendError(err)
				return
			}
		}

	case "start":
		// Start/restart wants
		for _, wantID := range op.IDs {
			if err := cb.RestartWant(wantID); err != nil {
				sendError(err)
				return
			}
		}

	case "addLabel":
		// Add label to want
		if len(op.IDs) > 0 && op.Data != nil {
			wantID := op.IDs[0]
			key, keyOk := op.Data["key"].(string)
			value, valueOk := op.Data["value"].(string)

			if !keyOk || !valueOk {
				sendError(fmt.Errorf("label key and value must be strings"))
				return
			}

			if want, _, found := cb.FindWantByID(wantID); found && want != nil {
				want.metadataMutex.Lock()
				if want.Metadata.Labels == nil {
					want.Metadata.Labels = make(map[string]string)
				}
				want.Metadata.Labels[key] = value
				want.metadataMutex.Unlock()
				want.Metadata.UpdatedAt = time.Now().Unix()
			} else {
				sendError(fmt.Errorf("want with ID %s not found", wantID))
				return
			}
		}

	case "removeLabel":
		// Remove label from want
		if len(op.IDs) > 0 && op.Data != nil {
			wantID := op.IDs[0]
			key, keyOk := op.Data["key"].(string)

			if !keyOk {
				sendError(fmt.Errorf("label key must be a string"))
				return
			}

			if want, _, found := cb.FindWantByID(wantID); found && want != nil {
				want.metadataMutex.Lock()
				if want.Metadata.Labels != nil {
					delete(want.Metadata.Labels, key)
				}
				want.metadataMutex.Unlock()
				want.Metadata.UpdatedAt = time.Now().Unix()
			} else {
				sendError(fmt.Errorf("want with ID %s not found", wantID))
				return
			}
		}

	default:
		sendError(fmt.Errorf("unknown operation type: %s", op.Type))
	}

	// Send success (nil error) to callback
	sendError(nil)
}
