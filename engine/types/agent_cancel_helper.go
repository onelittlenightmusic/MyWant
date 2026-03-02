package types

import . "mywant/engine/core"

// cancelPreviousWant marks the want identified by prevWantID as cancelled.
// This is called by DoAgents when executing a cost-reduction (rebook) action:
// the agent first cancels the existing booking, then creates a new cheaper one.
//
// For mock agents (hotel, restaurant, buffet) there is no external API to call —
// cancellation is a state-only operation. For agents backed by real APIs (e.g.
// flight), the agent should call the cancel endpoint before invoking this helper.
//
// The cancelled want keeps completed=true (terminal state for the framework) but
// gains _cancelled=true and cancelled=true so the UI can display it accordingly.
func cancelPreviousWant(want *Want, prevWantID string, wantTypeName string) {
	cb := GetGlobalChainBuilder()
	if cb == nil {
		want.StoreLog("[CANCEL] ChainBuilder unavailable — cannot cancel previous %s want", wantTypeName)
		return
	}
	for _, w := range cb.GetWants() {
		if w.Metadata.ID != prevWantID {
			continue
		}
		w.StoreState("_cancelled", true)
		w.StoreState("cancelled", true)
		// Zero out this want's cost contribution in the parent's "costs" map so
		// BudgetThinker does not count the cancelled booking alongside the rebook.
		w.MergeParentState(map[string]any{
			"costs": map[string]any{w.Metadata.Name: 0.0},
		})
		w.AggregateChanges()
		want.StoreLog("[CANCEL] Cancelled previous %s reservation (want: %s)", wantTypeName, prevWantID)
		return
	}
	want.StoreLog("[CANCEL] Previous %s want %s not found (may have been cleaned up)", wantTypeName, prevWantID)
}
