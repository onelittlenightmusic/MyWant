package types

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	mywant "mywant/engine/core"
)

const playwrightRecordAgentName = "playwright_record_monitor"

var (
	playwrightServerMu      sync.Mutex
	playwrightServerProcess *exec.Cmd
)

func init() {
	mywant.RegisterMonitorAgentType(
		playwrightRecordAgentName,
		[]mywant.Capability{
			mywant.Cap("playwright_recording"),
		},
		monitorPlaywrightRecording,
	)
}

// monitorPlaywrightRecording is the MonitorAgent poll function for browser recording.
// It watches for start/stop signals via webhook state and controls the Playwright MCP App Server.
func monitorPlaywrightRecording(ctx context.Context, want *mywant.Want) (bool, error) {
	// First run: register webhook IDs in state so the frontend can use them.
	// Check value emptiness (not just key existence) because YAML initializes the key to "" at creation.
	startID := mywant.GetCurrent(want, "startWebhookId", "")
	if startID == "" {
		want.SetCurrent("startWebhookId", want.Metadata.ID+"-start")
		want.SetCurrent("stopWebhookId", want.Metadata.ID+"-stop")
		want.SetCurrent("debugStartWebhookId", want.Metadata.ID+"-debug-start")
		want.SetCurrent("debugStopWebhookId", want.Metadata.ID+"-debug-stop")
		want.SetCurrent("replayWebhookId", want.Metadata.ID+"-replay")
		want.SetCurrent("action_by_agent", playwrightRecordAgentName)

		want.StoreLog("[PLAYWRIGHT-RECORD] Registered webhook IDs: %s-start / %s-stop / %s-debug-start / %s-debug-stop / %s-replay",
			want.Metadata.ID, want.Metadata.ID, want.Metadata.ID, want.Metadata.ID, want.Metadata.ID)
		return false, nil
	}

	isActive := mywant.GetCurrent(want, "recording_active", false)
	isDebugActive := mywant.GetCurrent(want, "debug_recording_active", false)
	isReplayActive := mywant.GetCurrent(want, "replay_active", false)

	if !isActive && !isDebugActive && !isReplayActive {
		// Waiting for normal start signal
		if mywant.GetPlan(want, "start_recording_requested", false) {
			want.StoreLog("[PLAYWRIGHT-RECORD] start_recording_requested=true, starting Playwright recording...")
			return false, startPlaywrightRecording(ctx, want)
		}
		// Waiting for debug start signal
		if mywant.GetPlan(want, "start_debug_recording_requested", false) {
			want.StoreLog("[PLAYWRIGHT-RECORD] start_debug_recording_requested=true, starting debug recording...")
			return false, startDebugRecording(ctx, want)
		}

		// Waiting for replay signal
		if mywant.GetPlan(want, "start_replay_requested", false) {
			want.StoreLog("[PLAYWRIGHT-RECORD] start_replay_requested=true, starting replay...")
			return false, startReplay(ctx, want)
		}
		// Idle - nothing to do
		return false, nil
	}

	if isActive {
		// Normal recording active - check for stop signal
		if mywant.GetPlan(want, "stop_recording_requested", false) {
			want.StoreLog("[PLAYWRIGHT-RECORD] stop_recording_requested=true, stopping Playwright recording...")
			return false, stopPlaywrightRecording(ctx, want)
		}
		want.StoreLog("[PLAYWRIGHT-RECORD] Recording active, waiting for stop signal...")
		return false, nil
	}

	if isDebugActive {
		// Debug recording active - check for stop signal
		if mywant.GetPlan(want, "stop_debug_recording_requested", false) {
			want.StoreLog("[PLAYWRIGHT-RECORD] stop_debug_recording_requested=true, stopping debug recording...")
			return false, stopDebugRecording(ctx, want)
		}
		want.StoreLog("[PLAYWRIGHT-RECORD] Debug recording active, waiting for finish signal...")
		return false, nil
	}

	if isReplayActive {
		// Replay in progress - poll for completion
		return pollReplay(ctx, want)
	}

	return false, nil
}

// ensurePlaywrightServer ensures the playwright-app MCP server is running with live pipes.
// If the process has exited or its pipes are stale (detected by forceRestart=true), it is restarted.
func ensurePlaywrightServer(ctx context.Context, serverPath string, forceRestart bool) error {
	playwrightServerMu.Lock()
	defer playwrightServerMu.Unlock()

	needStart := forceRestart || playwrightServerProcess == nil || playwrightServerProcess.ProcessState != nil
	if !needStart {
		return nil
	}

	// Kill old process if still alive
	if playwrightServerProcess != nil && playwrightServerProcess.ProcessState == nil {
		_ = playwrightServerProcess.Process.Kill()
		_ = playwrightServerProcess.Wait()
	}
	GetMCPServerRegistry().Unregister("playwright-mcp-app")
	playwrightServerProcess = nil

	// Kill any stale process still holding the WS port (e.g. from a previous server run)
	_ = exec.Command("sh", "-c", "lsof -ti:9321 | xargs kill -9 2>/dev/null || true").Run()
	time.Sleep(500 * time.Millisecond)

	cmd := exec.CommandContext(ctx, "node", serverPath)
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe for playwright-app: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe for playwright-app: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start playwright-app server: %w", err)
	}

	playwrightServerProcess = cmd
	GetMCPServerRegistry().Register("playwright-mcp-app", &MCPServerProcess{
		Cmd:    cmd,
		Stdin:  stdin,
		Stdout: stdout,
		Name:   "playwright-mcp-app",
	})
	log.Printf("[PLAYWRIGHT-RECORD] Started playwright-app server (PID: %d)\n", cmd.Process.Pid)
	time.Sleep(2 * time.Second)
	return nil
}

// isPipeClosed returns true if the error indicates a closed stdio pipe.
func isPipeClosed(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "file already closed") || strings.Contains(msg, "broken pipe")
}

// startPlaywrightRecording launches the Playwright MCP App Server and begins recording.
func startPlaywrightRecording(ctx context.Context, want *mywant.Want) error {
	targetURL := want.GetStringParam("target_url", "https://example.com")

	serverPath := resolvePlaywrightServerPath()
	if serverPath == "" {
		return fmt.Errorf("[PLAYWRIGHT-RECORD] playwright-app server not found; build mcp/playwright-app first")
	}

	mgr := GetNativeMCPManager(ctx)

	if err := ensurePlaywrightServer(ctx, serverPath, false); err != nil {
		return err
	}

	// Call start_recording MCP tool
	result, err := mgr.ExecuteTool(ctx,
		"playwright-mcp-app",
		"node", []string{serverPath},
		"start_recording",
		map[string]any{
			"target_url": targetURL,
		})
	if err != nil {
		return fmt.Errorf("start_recording MCP tool failed: %w", err)
	}

	sessionID, uiURL := parseStartRecordingResult(result)
	if sessionID == "" {
		return fmt.Errorf("start_recording returned no session_id")
	}

	want.StoreLog("[PLAYWRIGHT-RECORD] Recording started: session=%s ui=%s", sessionID, uiURL)
	want.SetCurrent("recording_session_id", sessionID)
	want.SetCurrent("recording_iframe_url", uiURL)
	want.SetCurrent("recording_active", true)
	want.SetPlan("start_recording_requested", false)
	want.SetCurrent("action_by_agent", playwrightRecordAgentName)
	return nil
}

// stopPlaywrightRecording sends stop_recording to the MCP App Server and saves the script.
func stopPlaywrightRecording(ctx context.Context, want *mywant.Want) error {
	sessionID := mywant.GetCurrent(want, "recording_session_id", "")
	if sessionID == "" {
		return fmt.Errorf("no recording_session_id found in state")
	}

	serverPath := resolvePlaywrightServerPath()
	mgr := GetNativeMCPManager(ctx)

	result, err := mgr.ExecuteTool(ctx,
		"playwright-mcp-app",
		"node", []string{serverPath},
		"stop_recording",
		map[string]any{
			"session_id": sessionID,
		})
	if err != nil {
		return fmt.Errorf("stop_recording MCP tool failed: %w", err)
	}

	script, actions, startURL := parseStopRecordingResult(result)
	want.StoreLog("[PLAYWRIGHT-RECORD] Recording stopped, script length=%d bytes", len(script))

	actionsJSON, _ := json.Marshal(actions)
	want.SetCurrent("replay_script", script)
	want.SetCurrent("final_result", script)
	want.SetCurrent("replay_actions", string(actionsJSON))
	want.SetCurrent("replay_start_url", startURL)
	want.SetCurrent("recording_active", false)
	want.SetPlan("stop_recording_requested", false)
	want.SetCurrent("action_by_agent", playwrightRecordAgentName)
	
	want.SetStatus(mywant.WantStatusAchieved)
	return nil
}

// startDebugRecording attaches to an existing Chrome via CDP and begins recording.
func startDebugRecording(ctx context.Context, want *mywant.Want) error {
	host := want.GetStringParam("debug_chrome_host", "localhost")
	port := want.GetStringParam("debug_chrome_port", "9222")
	cdpURL := fmt.Sprintf("http://%s:%s", host, port)

	serverPath := resolvePlaywrightServerPath()
	if serverPath == "" {
		return fmt.Errorf("[PLAYWRIGHT-RECORD] playwright-app server not found; build mcp/playwright-app first")
	}

	mgr := GetNativeMCPManager(ctx)

	if err := ensurePlaywrightServer(ctx, serverPath, false); err != nil {
		return err
	}

	targetURL := want.GetStringParam("target_url", "")
	debugToolArgs := map[string]any{"cdp_url": cdpURL}
	if targetURL != "" {
		debugToolArgs["target_url"] = targetURL
	}

	want.StoreLog("[PLAYWRIGHT-RECORD] Calling start_recording_debug: cdp=%s target=%s", cdpURL, targetURL)
	toolCtx, toolCancel := context.WithTimeout(ctx, 30*time.Second)
	defer toolCancel()
	result, err := mgr.ExecuteTool(toolCtx,
		"playwright-mcp-app",
		"node", []string{serverPath},
		"start_recording_debug",
		debugToolArgs)
	if isPipeClosed(err) {
		// Stale pipes detected — restart the server and retry once
		want.StoreLog("[PLAYWRIGHT-RECORD] Pipe closed, restarting playwright-app server and retrying...")
		if restartErr := ensurePlaywrightServer(ctx, serverPath, true); restartErr != nil {
			return fmt.Errorf("failed to restart playwright-app server: %w", restartErr)
		}
		toolCtx2, toolCancel2 := context.WithTimeout(ctx, 30*time.Second)
		defer toolCancel2()
		result, err = mgr.ExecuteTool(toolCtx2,
			"playwright-mcp-app",
			"node", []string{serverPath},
			"start_recording_debug",
			debugToolArgs)
	}
	if err != nil {
		want.StoreLog("[PLAYWRIGHT-RECORD] ERROR start_recording_debug failed: %v", err)
		return fmt.Errorf("start_recording_debug MCP tool failed: %w", err)
	}

	// If the MCP tool itself returned an error (e.g. CDP connection refused), surface it and stop retrying
	if result != nil && result.IsError {
		errMsg := extractMCPErrorText(result)
		want.StoreLog("[PLAYWRIGHT-RECORD] ERROR from start_recording_debug tool: %s", errMsg)
		// Clear the request flag so we don't keep retrying on a permanent error
		want.SetPlan("start_debug_recording_requested", false)
		want.SetCurrent("debug_recording_error", errMsg)
		want.SetCurrent("action_by_agent", playwrightRecordAgentName)
		return nil
	}

	// parseStartRecordingResult also extracts session_id; ui_url will be empty for debug mode
	sessionID, _ := parseStartRecordingResult(result)
	if sessionID == "" {
		want.StoreLog("[PLAYWRIGHT-RECORD] ERROR start_recording_debug returned no session_id")
		return fmt.Errorf("start_recording_debug returned no session_id")
	}

	want.StoreLog("[PLAYWRIGHT-RECORD] Debug recording started: session=%s cdp=%s", sessionID, cdpURL)
	want.SetCurrent("debug_recording_session_id", sessionID)
	want.SetCurrent("debug_recording_active", true)
	want.SetPlan("start_debug_recording_requested", false)
	want.SetCurrent("action_by_agent", playwrightRecordAgentName)
	return nil
}

// stopDebugRecording stops the debug recording, saves the Playwright script, and captures target_object.
func stopDebugRecording(ctx context.Context, want *mywant.Want) error {
	sessionID := mywant.GetCurrent(want, "debug_recording_session_id", "")
	if sessionID == "" {
		return fmt.Errorf("no debug_recording_session_id found in state")
	}

	serverPath := resolvePlaywrightServerPath()
	mgr := GetNativeMCPManager(ctx)

	result, err := mgr.ExecuteTool(ctx,
		"playwright-mcp-app",
		"node", []string{serverPath},
		"stop_recording_debug",
		map[string]any{
			"session_id": sessionID,
		})
	if err != nil {
		return fmt.Errorf("stop_recording_debug MCP tool failed: %w", err)
	}

	script, actions, startURL, targetObject := parseDebugStopResult(result)
	want.StoreLog("[PLAYWRIGHT-RECORD] Debug recording stopped, script=%d bytes target_object=%v", len(script), targetObject != nil)

	actionsJSON, _ := json.Marshal(actions)
	want.SetCurrent("replay_script", script)
	want.SetCurrent("final_result", script)
	want.SetCurrent("replay_actions", string(actionsJSON))
	want.SetCurrent("replay_start_url", startURL)
	want.SetCurrent("debug_recording_active", false)
	want.SetPlan("stop_debug_recording_requested", false)
	want.SetCurrent("action_by_agent", playwrightRecordAgentName)
	
	if targetObject != nil {
		want.SetCurrent("target_object", targetObject)
	}
	want.SetStatus(mywant.WantStatusAchieved)
	return nil
}

// startReplay launches a replay session via the run_replay MCP tool.
func startReplay(ctx context.Context, want *mywant.Want) error {
	actionsJSON := mywant.GetCurrent(want, "replay_actions", "[]")
	startURL := mywant.GetCurrent(want, "replay_start_url", "")
	if startURL == "" {
		startURL = want.GetStringParam("target_url", "https://example.com")
	}

	var actions []string
	if err := json.Unmarshal([]byte(actionsJSON), &actions); err != nil || len(actions) == 0 {
		want.StoreLog("[PLAYWRIGHT-RECORD] No replay_actions available for replay")
		want.SetPlan("start_replay_requested", false)
		want.SetCurrent("action_by_agent", playwrightRecordAgentName)
		return nil
	}

	serverPath := resolvePlaywrightServerPath()
	if serverPath == "" {
		return fmt.Errorf("[PLAYWRIGHT-RECORD] playwright-app server not found")
	}
	mgr := GetNativeMCPManager(ctx)
	if err := ensurePlaywrightServer(ctx, serverPath, false); err != nil {
		return err
	}

	want.StoreLog("[PLAYWRIGHT-RECORD] Starting replay: start_url=%s actions=%d", startURL, len(actions))
	toolCtx, toolCancel := context.WithTimeout(ctx, 30*time.Second)
	defer toolCancel()
	result, err := mgr.ExecuteTool(toolCtx, "playwright-mcp-app", "node", []string{serverPath},
		"run_replay", map[string]any{"start_url": startURL, "actions": actions})
	if err != nil {
		return fmt.Errorf("run_replay MCP tool failed: %w", err)
	}

	sessionID, uiURL := parseStartRecordingResult(result)
	if sessionID == "" {
		return fmt.Errorf("run_replay returned no session_id")
	}

	want.StoreLog("[PLAYWRIGHT-RECORD] Replay started: session=%s ui=%s", sessionID, uiURL)
	want.SetCurrent("replay_session_id", sessionID)
	want.SetCurrent("replay_iframe_url", uiURL)
	want.SetCurrent("replay_active", true)
	want.SetPlan("start_replay_requested", false)
	want.SetCurrent("action_by_agent", playwrightRecordAgentName)
	return nil
}

// pollReplay checks the replay status via check_replay MCP tool.
func pollReplay(ctx context.Context, want *mywant.Want) (bool, error) {
	sessionID := mywant.GetCurrent(want, "replay_session_id", "")
	if sessionID == "" {
		want.SetCurrent("replay_active", false)
		want.SetCurrent("action_by_agent", playwrightRecordAgentName)
		return true, nil
	}

	serverPath := resolvePlaywrightServerPath()
	mgr := GetNativeMCPManager(ctx)

	toolCtx, toolCancel := context.WithTimeout(ctx, 10*time.Second)
	defer toolCancel()
	result, err := mgr.ExecuteTool(toolCtx, "playwright-mcp-app", "node", []string{serverPath},
		"check_replay", map[string]any{"session_id": sessionID})
	if err != nil {
		want.StoreLog("[PLAYWRIGHT-RECORD] check_replay error: %v", err)
		return false, nil
	}

	texts := flattenMCPContent(result.Content)
	for _, text := range texts {
		var inner struct {
			Done   bool           `json:"done"`
			Result map[string]any `json:"result"`
			Error  string         `json:"error"`
		}
		if err := json.Unmarshal([]byte(text), &inner); err == nil {
			if !inner.Done {
				want.StoreLog("[PLAYWRIGHT-RECORD] Replay in progress...")
				return false, nil
			}
			// Replay complete
			replayResultJSON, _ := json.Marshal(inner.Result)
			
			want.SetCurrent("replay_active", false)
			want.SetCurrent("replay_session_id", "")
			want.SetCurrent("replay_iframe_url", "")
			want.SetCurrent("action_by_agent", playwrightRecordAgentName)

			if inner.Error != "" {
				want.SetCurrent("replay_error", inner.Error)
				want.StoreLog("[PLAYWRIGHT-RECORD] Replay failed: %s", inner.Error)
			} else {
				want.SetCurrent("replay_result", string(replayResultJSON))
				// Set final_result to the selected_text from the replay result
				if selectedText, ok := inner.Result["selected_text"].(string); ok && selectedText != "" {
					want.SetCurrent("final_result", selectedText)
				}
				// Move screenshot from MCP-written path to want-specific path
				if screenshotPath, ok := inner.Result["screenshot_path"].(string); ok && screenshotPath != "" {
					if screenshotURL, err := moveReplayScreenshot(screenshotPath, want.Metadata.ID); err == nil {
						want.SetCurrent("replay_screenshot_url", screenshotURL)
					} else {
						want.StoreLog("[PLAYWRIGHT-RECORD] Failed to save screenshot: %s", err)
					}
				}
				want.StoreLog("[PLAYWRIGHT-RECORD] Replay complete: result=%s", string(replayResultJSON))
			}
			return true, nil
		}
	}

	return false, nil
}


// moveReplayScreenshot moves the MCP-written screenshot to ~/.mywant/screenshots/<wantID>.png.
// Returns the API URL path for serving the image.
func moveReplayScreenshot(srcPath, wantID string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not determine home dir: %w", err)
	}
	dir := filepath.Join(home, ".mywant", "screenshots")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("could not create screenshots dir: %w", err)
	}
	filename := wantID + ".png"
	dstPath := filepath.Join(dir, filename)
	// Try rename first (same filesystem), fall back to copy
	if err := os.Rename(srcPath, dstPath); err != nil {
		data, readErr := os.ReadFile(srcPath)
		if readErr != nil {
			return "", fmt.Errorf("could not read screenshot: %w", readErr)
		}
		if writeErr := os.WriteFile(dstPath, data, 0644); writeErr != nil {
			return "", fmt.Errorf("could not write screenshot: %w", writeErr)
		}
		os.Remove(srcPath)
	}
	return "/api/v1/screenshots/" + filename, nil
}

// resolvePlaywrightServerPath returns the absolute path to the playwright-app server.js.
func resolvePlaywrightServerPath() string {
	candidates := []string{
		"mcp/playwright-app/dist/server.js",
		"mcp/playwright-app/server.js",
	}

	_, filename, _, ok := runtime.Caller(0)
	var sourceRoot string
	if ok {
		// engine/types/agent_playwright_record.go → go up 2 levels to project root
		sourceRoot = filepath.Join(filepath.Dir(filename), "..", "..")
	}

	for _, rel := range candidates {
		if _, err := os.Stat(rel); err == nil {
			abs, _ := filepath.Abs(rel)
			return abs
		}
		if sourceRoot != "" {
			p := filepath.Join(sourceRoot, rel)
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}
	return ""
}

// parseStartRecordingResult extracts session_id and ui_url from MCP CallToolResult content.
func parseStartRecordingResult(result *mcp.CallToolResult) (sessionID, uiURL string) {
	if result == nil {
		return
	}
	texts := flattenMCPContent(result.Content)
	for _, text := range texts {
		var inner struct {
			SessionID string `json:"session_id"`
			UIURL     string `json:"ui_url"`
		}
		if err := json.Unmarshal([]byte(text), &inner); err == nil {
			if inner.SessionID != "" {
				return inner.SessionID, inner.UIURL
			}
		}
	}
	return
}

// extractMCPErrorText returns the error message from an isError MCP tool result.
func extractMCPErrorText(result *mcp.CallToolResult) string {
	if result == nil {
		return "unknown error"
	}
	texts := flattenMCPContent(result.Content)
	for _, t := range texts {
		if t != "" {
			return t
		}
	}
	return "unknown error"
}

// parseDebugStopResult extracts the Playwright script, raw actions, startURL, and target_object from stop_recording_debug result.
func parseDebugStopResult(result *mcp.CallToolResult) (script string, actions []string, startURL string, targetObject map[string]any) {
	if result == nil {
		return "", nil, "", nil
	}
	texts := flattenMCPContent(result.Content)
	for _, text := range texts {
		var inner struct {
			Script       string         `json:"script"`
			Actions      []string       `json:"actions"`
			StartURL     string         `json:"start_url"`
			TargetObject map[string]any `json:"target_object"`
		}
		if err := json.Unmarshal([]byte(text), &inner); err == nil && inner.Script != "" {
			return inner.Script, inner.Actions, inner.StartURL, inner.TargetObject
		}
	}
	return "", nil, "", nil
}

// parseStopRecordingResult extracts the Playwright script, raw actions, and startURL from stop_recording result.
func parseStopRecordingResult(result *mcp.CallToolResult) (script string, actions []string, startURL string) {
	if result == nil {
		return "", nil, ""
	}
	texts := flattenMCPContent(result.Content)
	for _, text := range texts {
		var inner struct {
			Script   string   `json:"script"`
			Actions  []string `json:"actions"`
			StartURL string   `json:"start_url"`
		}
		if err := json.Unmarshal([]byte(text), &inner); err == nil && inner.Script != "" {
			return inner.Script, inner.Actions, inner.StartURL
		}
		if text != "" {
			return text, nil, ""
		}
	}
	return "", nil, ""
}
