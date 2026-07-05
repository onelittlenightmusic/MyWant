package types

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
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
	// First run: register webhook ID and open browser with inspector overlay —
	// unless launch_mode=manual, meaning the browser side is a desktop
	// bookmarklet or iOS Shortcut instead of a CDP-controlled Chrome tab (see
	// WebInspectorModal.tsx). In that case there's nothing to drive here: just
	// register the webhook and wait — the standalone overlay finds this want
	// itself via GET /api/v1/web-wants/active-inspection (handlers_web_wants.go).
	doneWebhookID := mywant.GetCurrent(want, "doneWebhookId", "")
	if doneWebhookID == "" {
		webhookID := want.Metadata.ID + "-done"
		want.SetCurrent("doneWebhookId", webhookID)
		want.StoreLog("[WEB-INSPECTOR] Registered webhook ID: %s", webhookID)
		want.SetCurrent("action_by_agent", webInspectorAgentName)
		if paramOrCurrent(want, "launch_mode") == "manual" {
			want.StoreLog("[WEB-INSPECTOR] launch_mode=manual — skipping CDP auto-launch")
			return false, nil
		}
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
	suggestNameURL := fmt.Sprintf("http://localhost:%s/api/v1/web-wants/suggest-name", mywantPort)

	// Resolve the acting character's color server-side (never trust a
	// client-supplied color) so marks left in the inspector overlay are
	// attributed and colored consistently with aura marks elsewhere.
	// paramOrCurrent falls back to the original request param when the
	// current-labeled mirror is still empty (e.g. want was created before
	// ScriptableWant's Initialize() had a chance to copy it, or any other
	// reason the mirror didn't take) — Spec.Params always reflects exactly
	// what the frontend sent, so this guarantees we never silently drop the
	// acting character back to the cyan default.
	characterID := paramOrCurrent(want, "characterId")
	color := ""
	avatar := ""
	if character, ok := mywant.GetCharacter(characterID); ok {
		color = character.Color
		avatar = character.Avatar
	}

	var existingMarksJSON string
	if hostname := hostnameOf(targetURL); hostname != "" {
		if marks := mywant.GetWebMarks(hostname); len(marks) > 0 {
			if b, err := json.Marshal(marks); err == nil {
				existingMarksJSON = string(b)
			}
		}
	}

	// When reopening the inspector for an existing web want type (review mode),
	// also load the elements that type's Launch action uses — the frontend passes
	// which type this is via wantTypeName so the overlay can show the same
	// CursorMan navigation over them that Launch itself displays.
	var navElementsJSON string
	if wantTypeName := paramOrCurrent(want, "wantTypeName"); wantTypeName != "" {
		if navElements := loadWantTypeNavElements(wantTypeName); len(navElements) > 0 {
			if b, err := json.Marshal(navElements); err == nil {
				navElementsJSON = string(b)
			}
		}
	}

	toolArgs := map[string]any{
		"cdp_url":          cdpURL,
		"target_url":       targetURL,
		"done_webhook_url": doneWebhookURL,
		"suggest_name_url": suggestNameURL,
		"character_id":     characterID,
		"color":            color,
		"avatar":           avatar,
		"existing_marks":   existingMarksJSON,
		"nav_elements":     navElementsJSON,
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

// paramOrCurrent reads a string field via its current-labeled mirror first,
// falling back to the raw request param (Spec.Params) if the mirror is
// empty. Spec.Params always holds exactly what the frontend sent at want
// creation, regardless of whether/when the current-labeled copy was made.
func paramOrCurrent(want *mywant.Want, key string) string {
	if v := mywant.GetCurrent(want, key, ""); v != "" {
		return v
	}
	if v, ok := want.Spec.Params[key].(string); ok {
		return v
	}
	return ""
}

// hostnameOf returns targetURL's hostname, matching what the injected
// overlay computes client-side via window.location.hostname at Done time —
// this is the key web marks are stored/looked-up under.
func hostnameOf(targetURL string) string {
	u, err := url.Parse(targetURL)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

// loadWantTypeNavElements reads the same elements.json a web want type's
// Launch action reads (see handlers_web_wants.go's launchWebWant), flattened
// across hostnames, for display in the inspector overlay.
func loadWantTypeNavElements(wantTypeName string) []WebNavElement {
	elemFile := filepath.Join(mywant.UserCustomTypesDir(), wantTypeName, "elements.json")
	data, err := os.ReadFile(elemFile)
	if err != nil {
		return nil
	}
	var allElems map[string][]WebNavElement
	if err := json.Unmarshal(data, &allElems); err != nil {
		return nil
	}
	var elements []WebNavElement
	for _, elems := range allElems {
		elements = append(elements, elems...)
	}
	return elements
}
