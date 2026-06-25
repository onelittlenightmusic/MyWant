package types

import (
	"context"
	"fmt"
	"time"

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

func monitorWebInspector(ctx context.Context, want *mywant.Want) (bool, error) {
	// First run: register webhook ID and open browser with inspector overlay.
	doneWebhookID := mywant.GetCurrent(want, "doneWebhookId", "")
	if doneWebhookID == "" {
		webhookID := want.Metadata.ID + "-done"
		want.SetCurrent("doneWebhookId", webhookID)
		want.StoreLog("[WEB-INSPECTOR] Registered webhook ID: %s", webhookID)
		want.SetCurrent("action_by_agent", webInspectorAgentName)
		return false, openInspectorTab(ctx, want, webhookID)
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

func openInspectorTab(ctx context.Context, want *mywant.Want, webhookID string) error {
	serverPath := resolvePlaywrightServerPath()
	if serverPath == "" {
		return fmt.Errorf("[WEB-INSPECTOR] playwright-app server not found; build mcp/playwright-app first")
	}

	mgr := GetNativeMCPManager(ctx)

	if err := ensurePlaywrightServer(ctx, serverPath, false); err != nil {
		return err
	}

	targetURL := mywant.GetCurrent(want, "target_url", "https://example.com")
	host := mywant.GetCurrent(want, "debug_chrome_host", "localhost")
	port := mywant.GetCurrent(want, "debug_chrome_port", "9222")
	cdpURL := fmt.Sprintf("http://%s:%s", host, port)

	mywantPort := mywant.GetCurrent(want, "mywant_api_port", "8080")
	doneWebhookURL := fmt.Sprintf("http://localhost:%s/api/v1/webhooks/%s", mywantPort, webhookID)

	toolArgs := map[string]any{
		"cdp_url":          cdpURL,
		"target_url":       targetURL,
		"done_webhook_url": doneWebhookURL,
	}

	want.StoreLog("[WEB-INSPECTOR] Opening inspector tab: cdp=%s url=%s webhook=%s", cdpURL, targetURL, doneWebhookURL)

	toolCtx, toolCancel := context.WithTimeout(ctx, 30*time.Second)
	defer toolCancel()

	result, err := mgr.ExecuteTool(toolCtx,
		"playwright-mcp-app",
		"node", []string{serverPath},
		"open_inspector_tab",
		toolArgs)
	if isPipeClosed(err) {
		if restartErr := ensurePlaywrightServer(ctx, serverPath, true); restartErr != nil {
			return fmt.Errorf("failed to restart playwright-app server: %w", restartErr)
		}
		toolCtx2, toolCancel2 := context.WithTimeout(ctx, 30*time.Second)
		defer toolCancel2()
		result, err = mgr.ExecuteTool(toolCtx2,
			"playwright-mcp-app",
			"node", []string{serverPath},
			"open_inspector_tab",
			toolArgs)
	}
	if err != nil {
		return fmt.Errorf("open_inspector_tab MCP tool failed: %w", err)
	}
	if result != nil && result.IsError {
		errMsg := extractMCPErrorText(result)
		want.SetCurrent("inspector_error", errMsg)
		want.StoreLog("[WEB-INSPECTOR] ERROR: %s", errMsg)
		return nil
	}

	want.SetCurrent("inspector_open", true)
	want.StoreLog("[WEB-INSPECTOR] Inspector tab opened successfully")
	return nil
}
