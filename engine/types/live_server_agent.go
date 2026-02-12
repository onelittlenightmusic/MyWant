package types

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	mywant "mywant/engine/core"
)

func init() {
	mywant.RegisterDoAgentType("live_server_manager",
		[]mywant.Capability{mywant.Cap("live_server_management")},
		manageLiveServer)
}

// manageLiveServer handles server start and stop based on want phase
func manageLiveServer(ctx context.Context, want *mywant.Want) error {
	want.StoreLog("[AGENT] manageLiveServer called for want %s", want.Metadata.Name)

	phase, ok := want.GetStateString("server_phase", "")
	if !ok || phase == "" {
		want.StoreLog("[AGENT] No server_phase set, returning")
		return nil
	}

	existingPID, _ := want.GetStateInt("server_pid", 0)

	switch phase {
	case "starting":
		if existingPID == 0 {
			pid, err := startLiveServer(want)
			if err != nil {
				want.StoreLog("[ERROR] Failed to start server: %v", err)
				return err
			}
			want.StoreState("server_pid", pid)
			want.StoreLog("[INFO] Started server with PID %d", pid)

			// If health_check_url is configured, poll for readiness
			healthCheckURL := getConfigString(want, "server_health_check_url", "health_check_url", "")
			if healthCheckURL != "" {
				body, err := waitForHealthCheck(ctx, want, healthCheckURL)
				if err != nil {
					want.StoreLog("[ERROR] Health check failed: %v", err)
					if proc, findErr := os.FindProcess(pid); findErr == nil {
						proc.Signal(syscall.SIGTERM)
					}
					want.StoreState("server_pid", 0)
					return err
				}
				want.StoreState("health_check_response", body)
				want.StoreLog("[INFO] Health check passed")
			}
		} else {
			want.StoreLog("[INFO] Server already running with PID %d", existingPID)
		}

	case "stopping":
		if existingPID != 0 {
			err := stopLiveServer(existingPID, want)
			if err != nil {
				want.StoreLog("[WARN] Failed to stop server PID %d: %v", existingPID, err)
			} else {
				want.StoreLog("[INFO] Stopped server PID %d", existingPID)
				want.StoreState("server_pid", 0)
				want.StoreState("health_check_response", "")
			}
		}
	}

	return nil
}

// startLiveServer starts a server process using command and args from state (fallback: params)
func startLiveServer(want *mywant.Want) (int, error) {
	command := getConfigString(want, "server_command", "command", "")
	if command == "" {
		return 0, fmt.Errorf("command parameter is required")
	}
	args := getConfigArgs(want)
	logFile := getConfigString(want, "server_log_file", "log_file", "")

	// Binary lookup: LookPath first, then os.Stat fallback
	binPath, err := exec.LookPath(command)
	if err != nil {
		if _, statErr := os.Stat(command); statErr != nil {
			return 0, fmt.Errorf("command not found: %s", command)
		}
		binPath = command
	}

	cmd := exec.Command(binPath, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	var logFileHandle *os.File
	if logFile != "" {
		if dir := filepath.Dir(logFile); dir != "." {
			if mkdirErr := os.MkdirAll(dir, 0755); mkdirErr != nil {
				return 0, fmt.Errorf("failed to create log directory: %w", mkdirErr)
			}
		}

		logFileHandle, err = os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return 0, fmt.Errorf("failed to open log file: %w", err)
		}
		cmd.Stdout = logFileHandle
		cmd.Stderr = logFileHandle
	}

	if err := cmd.Start(); err != nil {
		if logFileHandle != nil {
			logFileHandle.Close()
		}
		return 0, fmt.Errorf("failed to start server: %w", err)
	}

	pid := cmd.Process.Pid
	want.StoreLog("[INFO] Server started: %s %v (PID: %d)", binPath, args, pid)

	go func() {
		cmd.Wait()
		if logFileHandle != nil {
			logFileHandle.Close()
		}
	}()

	return pid, nil
}

// waitForHealthCheck polls the health check URL until it responds successfully
func waitForHealthCheck(ctx context.Context, want *mywant.Want, url string) (string, error) {
	intervalStr := getConfigString(want, "server_health_check_interval", "health_check_interval", "500ms")
	interval, err := time.ParseDuration(intervalStr)
	if err != nil {
		interval = 500 * time.Millisecond
	}

	maxRetries := getConfigInt(want, "server_health_check_max_retries", "health_check_max_retries", 15)

	client := &http.Client{Timeout: 2 * time.Second}

	for i := 0; i < maxRetries; i++ {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		resp, err := client.Get(url)
		if err != nil {
			want.StoreLog("[DEBUG] Waiting for health check (attempt %d/%d)...", i+1, maxRetries)
			time.Sleep(interval)
			continue
		}

		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			time.Sleep(interval)
			continue
		}

		if resp.StatusCode >= 200 && resp.StatusCode < 400 {
			return string(body), nil
		}

		time.Sleep(interval)
	}

	return "", fmt.Errorf("timed out waiting for health check at %s", url)
}

// getConfigString reads a value from state (stateKey) first, then falls back to params (paramKey).
// Want types store derived config in state; direct YAML usage stores in params.
func getConfigString(want *mywant.Want, stateKey, paramKey, defaultVal string) string {
	if v, ok := want.GetStateString(stateKey, ""); ok && v != "" {
		return v
	}
	return getStringParam(want, paramKey, defaultVal)
}

// getConfigInt reads an int from state first, then falls back to params.
func getConfigInt(want *mywant.Want, stateKey, paramKey string, defaultVal int) int {
	if v, ok := want.GetStateInt(stateKey, 0); ok && v != 0 {
		return v
	}
	return getIntParam(want, paramKey, defaultVal)
}

// getConfigArgs reads args from state (JSON string) first, then falls back to params.
func getConfigArgs(want *mywant.Want) []string {
	if v, ok := want.GetStateString("server_args", ""); ok && v != "" {
		// Stored as JSON array string
		var args []string
		if json.Unmarshal([]byte(v), &args) == nil {
			return args
		}
	}
	return getArgsParam(want)
}

// getStringParam gets a string parameter from want params with a default value
func getStringParam(want *mywant.Want, key, defaultVal string) string {
	if v, ok := want.Spec.Params[key]; ok {
		return fmt.Sprintf("%v", v)
	}
	return defaultVal
}

// getIntParam gets an int parameter from want params with a default value
func getIntParam(want *mywant.Want, key string, defaultVal int) int {
	v, ok := want.Spec.Params[key]
	if !ok {
		return defaultVal
	}
	switch n := v.(type) {
	case int:
		return n
	case float64:
		return int(n)
	case string:
		var result int
		if _, err := fmt.Sscanf(n, "%d", &result); err == nil {
			return result
		}
	}
	return defaultVal
}

// getArgsParam extracts the args parameter as []string from a Want
func getArgsParam(want *mywant.Want) []string {
	raw, ok := want.Spec.Params["args"]
	if !ok {
		return nil
	}
	switch v := raw.(type) {
	case []interface{}:
		args := make([]string, len(v))
		for i, a := range v {
			args[i] = fmt.Sprintf("%v", a)
		}
		return args
	case []string:
		return v
	}
	return nil
}

// stopLiveServer stops the server process by PID
func stopLiveServer(pid int, want *mywant.Want) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("failed to find process %d: %w", pid, err)
	}

	if err := process.Signal(syscall.SIGTERM); err != nil {
		want.StoreLog("[WARN] SIGTERM failed for PID %d, trying SIGKILL: %v", pid, err)
		if err := process.Kill(); err != nil {
			return fmt.Errorf("failed to kill process %d: %w", pid, err)
		}
	}

	want.StoreLog("[INFO] Sent termination signal to server PID %d", pid)
	return nil
}
