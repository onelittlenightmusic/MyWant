package mywant

import "fmt"

// AddWantsAsync adds wants to the execution queue asynchronously
func (cb *ChainBuilder) AddWantsAsync(wants []*Want) error {
	if len(wants) == 0 {
		return nil
	}
	select {
	case cb.addWantsChan <- wants:
		return nil
	default:
		return fmt.Errorf("failed to send wants to reconcile loop (channel full)")
	}
}

func (cb *ChainBuilder) AddWantsAsyncWithTracking(wants []*Want) ([]string, error) {
	// Extract IDs from wants
	ids := make([]string, len(wants))
	for i, want := range wants {
		if want.Metadata.ID == "" {
			return nil, fmt.Errorf("want %s has no ID for tracking", want.Metadata.Name)
		}
		ids[i] = want.Metadata.ID
	}

	// Pre-check: Verify no duplicate names in existing wants
	cb.reconcileMutex.RLock()
	for _, newWant := range wants {
		for _, rw := range cb.wants {
			if rw.want.Metadata.Name == newWant.Metadata.Name {
				cb.reconcileMutex.RUnlock()
				return nil, fmt.Errorf("want with name '%s' already exists", newWant.Metadata.Name)
			}
		}
	}
	cb.reconcileMutex.RUnlock()

	if err := cb.AddWantsAsync(wants); err != nil {
		return nil, err
	}

	return ids, nil
}

// AreWantsAdded checks if all wants with the given IDs have been added to the runtime Returns true only if ALL wants are present in the runtime
func (cb *ChainBuilder) AreWantsAdded(wantIDs []string) bool {
	if len(wantIDs) == 0 {
		return true
	}

	cb.reconcileMutex.RLock()
	defer cb.reconcileMutex.RUnlock()
	for _, id := range wantIDs {
		found := false
		for _, rw := range cb.wants {
			if rw.want.Metadata.ID == id {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}

// DeleteWantsAsync sends want IDs to be deleted asynchronously through the reconcile loop This is the preferred method for deleting wants to avoid race conditions
func (cb *ChainBuilder) DeleteWantsAsync(wantIDs []string) error {
	select {
	case cb.deleteWantsChan <- wantIDs:
		return nil
	default:
		return fmt.Errorf("failed to send wants to delete through reconcile loop (channel full)")
	}
}

// DeleteWantsAsyncWithTracking sends want IDs to be deleted asynchronously and returns them for tracking The caller can use the returned IDs to poll with AreWantsDeleted() to confirm deletion
func (cb *ChainBuilder) DeleteWantsAsyncWithTracking(wantIDs []string) ([]string, error) {
	if err := cb.DeleteWantsAsync(wantIDs); err != nil {
		return nil, err
	}
	return wantIDs, nil
}

// AreWantsDeleted checks if all wants with the given IDs have been removed from the runtime Returns true only if ALL wants are no longer present in the runtime
func (cb *ChainBuilder) AreWantsDeleted(wantIDs []string) bool {
	if len(wantIDs) == 0 {
		return true
	}

	cb.reconcileMutex.RLock()
	defer cb.reconcileMutex.RUnlock()
	for _, id := range wantIDs {
		for _, rw := range cb.wants {
			if rw.want.Metadata.ID == id {
				// Found the want, so it's not deleted yet
				return false
			}
		}
	}

	// All wants are gone
	return true
}
