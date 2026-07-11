package types

import (
	"context"

	mywant "mywant/engine/core"
)

const webInspectorAgentName = "web_inspector_monitor"

func init() {
	mywant.RegisterWithInit(func() {
		mywant.RegisterMonitorAgentType(
			webInspectorAgentName,
			[]mywant.Capability{
				mywant.Cap("web_inspection"),
			},
			monitorWebInspector,
		)
	})
}

// monitorWebInspector registers the done webhook and waits — the browser
// side is always the extension's own auto-launch poll (see mywant-gui's
// webext/webext-src/background.js's pollForPendingAction/handleAutoLaunch
// and handlers_web_wants.go's claimPendingAutoLaunch) or a desktop
// bookmarklet/iOS Shortcut (see WebInspectorModal.tsx, in mywant-gui), never
// a CDP-controlled Chrome driven from here. Either way, the browser side
// finds this want itself via GET /api/v1/web-wants/active-inspection
// (handlers_web_wants.go).
func monitorWebInspector(ctx context.Context, want *mywant.Want) (bool, error) {
	doneWebhookID := mywant.GetCurrent(want, "doneWebhookId", "")
	if doneWebhookID == "" {
		webhookID := want.Metadata.ID + "-done"
		want.SetCurrent("doneWebhookId", webhookID)
		want.StoreLog("[WEB-INSPECTOR] Registered webhook ID: %s", webhookID)
		want.SetCurrent("action_by_agent", webInspectorAgentName)
		return false, nil
	}

	// Check if the done webhook was received (stored in state by webhook handler).
	if mywant.GetCurrent(want, "inspection_done_received", false) {
		want.SetCurrent("inspection_done_received", false)
		want.SetCurrent("inspection_complete", true)
		want.SetCurrent("achieved", true)
		want.SetCurrent("action_by_agent", webInspectorAgentName)
		want.StoreLog("[WEB-INSPECTOR] Inspection complete — elements stored in state")
		return true, nil
	}

	return false, nil
}
