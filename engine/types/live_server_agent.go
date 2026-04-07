package types

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	mywant "mywant/engine/core"
)

// startLiveServer starts a server process using command and args from state (fallback: params)
func startLiveServer(want *mywant.Want) (int, error) {
	command := getConfigString(want, "process_command", "command", "")
	if command == "" {
		return 0, fmt.Errorf("command parameter is required")
	}
	args := getConfigArgs(want)
	logFile := getConfigString(want, "process_log_file", "log_file", "")

	// Binary lookup: LookPath first, then os.Stat fallback
	binPath, err := exec.LookPath(command)
	if err != nil {
		if _, statErr := os.Stat(command); statErr != nil {
			return 0, fmt.Errorf("command not found: %s", command)
		}
		binPath = command
	}

	want.DirectLog("[INFO] Executing: %s %v", binPath, args)
	cmd := exec.Command(binPath, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	var logFileHandle *os.File
	if logFile != "" {
		if dir := filepath.Dir(logFile); dir != "." {
			if mkdirErr := os.MkdirAll(dir, 0755); mkdirErr != nil {
				return 0, fmt.Errorf("failed to create log directory: %w", mkdirErr)
			}
		}

		logFileHandle, err = os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
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
	want.DirectLog("[INFO] Server started: %s %v (PID: %d)", binPath, args, pid)

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
			want.DirectLog("[DEBUG] Waiting for health check (attempt %d/%d)...", i+1, maxRetries)
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
	if v := mywant.GetCurrent(want, stateKey, ""); v != "" {
		return v
	}
	return getStringParam(want, paramKey, defaultVal)
}

// getConfigInt reads an int from state first, then falls back to params.
func getConfigInt(want *mywant.Want, stateKey, paramKey string, defaultVal int) int {
	if v := mywant.GetCurrent(want, stateKey, 0); v != 0 {
		return v
	}
	return getIntParam(want, paramKey, defaultVal)
}

// getConfigArgs reads args from state (JSON string) first, then falls back to params.
func getConfigArgs(want *mywant.Want) []string {
	if v := mywant.GetCurrent(want, "process_args", ""); v != "" {
		want.DirectLog("[DEBUG] Unmarshalling process_args: %q", v)
		// Stored as JSON array string
		var args []string
		if err := json.Unmarshal([]byte(v), &args); err == nil {
			return args
		} else {
			want.DirectLog("[DEBUG] Unmarshal failed: %v", err)
		}
	}
	return getArgsParam(want)
}

// getStringParam gets a string parameter from want params with a default value
func getStringParam(want *mywant.Want, key, defaultVal string) string {
	if v, ok := want.Spec.GetParam(key); ok {
		return fmt.Sprintf("%v", v)
	}
	return defaultVal
}

// getIntParam gets an int parameter from want params with a default value
func getIntParam(want *mywant.Want, key string, defaultVal int) int {
	v, ok := want.Spec.GetParam(key)
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
	raw, ok := want.Spec.GetParam("args")
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
	case string:
		// JSON array string e.g. `'["http","8080"]'`
		var parsed []string
		if err := json.Unmarshal([]byte(v), &parsed); err == nil {
			return parsed
		}
		// fallback: space-separated
		return strings.Fields(v)
	}
	return nil
}

// stopLiveServer stops the server process by PID (and its process group)
func stopLiveServer(pid int, want *mywant.Want) error {
	// Try to kill process group first (since we start with Setpgid: true)
	// On Unix, sending signal to -pid sends it to the entire process group
	err := syscall.Kill(-pid, syscall.SIGTERM)
	if err == nil {
		want.DirectLog("[INFO] Sent SIGTERM to process group %d", pid)
		return nil
	}

	// Fallback to single process if group killing failed
	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("failed to find process %d: %w", pid, err)
	}

	if err := process.Signal(syscall.SIGTERM); err != nil {
		want.DirectLog("[WARN] SIGTERM failed for PID %d, trying SIGKILL: %v", pid, err)
		if err := process.Kill(); err != nil {
			return fmt.Errorf("failed to kill process %d: %w", pid, err)
		}
	}

	want.DirectLog("[INFO] Sent termination signal to server PID %d", pid)
	return nil
}

// waitForPattern polls logFile until the regex matches, returning the first capture group.
func waitForPattern(ctx context.Context, want *mywant.Want, logFile, pattern string, maxRetries int) string {
	re, err := regexp.Compile(pattern)
	if err != nil {
		want.DirectLog("[ERROR] Invalid url_regex: %v", err)
		return ""
	}
	interval := 500 * time.Millisecond
	for i := range maxRetries {
		select {
		case <-ctx.Done():
			return ""
		default:
		}
		if url := scanPattern(logFile, re); url != "" {
			return url
		}
		want.DirectLog("[DEBUG] Waiting for URL pattern (attempt %d/%d)...", i+1, maxRetries)
		time.Sleep(interval)
	}
	return ""
}

// scanPattern scans logFile line by line and returns the first regex capture group match.
func scanPattern(logFile string, re *regexp.Regexp) string {
	f, err := os.Open(logFile)
	if err != nil {
		return ""
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if m := re.FindStringSubmatch(scanner.Text()); len(m) > 1 {
			return m[1]
		}
	}
	return ""
}
