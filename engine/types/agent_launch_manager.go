package types

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	mywant "mywant/engine/core"
)

func init() {
	mywant.RegisterWithInit(func() {
		mywant.RegisterDoAgent("launch_manager", manageLaunch)
	})
}

// manageLaunch dispatches to the appropriate lifecycle implementation.
//
// It supports two plan keys:
//   - launch_plan ("start"|"stop") + launch_type ("process"|"docker_compose") — managed_launch
//   - docker_plan  ("start"|"stop") — docker_run (legacy docker_management)
func manageLaunch(ctx context.Context, want *mywant.Want) error {
	if plan := mywant.GetPlan(want, "launch_plan", ""); plan != "" {
		launchType := mywant.GetCurrent(want, "launch_type", "process")
		status := mywant.GetCurrent(want, "status", "")
		switch plan {
		case "start":
			if status == "running" {
				return launchPoll(ctx, want, launchType)
			}
			return launchStart(ctx, want, launchType)
		case "stop":
			return launchStop(ctx, want, launchType)
		}
		return nil
	}

	// Legacy: docker_run (docker_plan → docker_phase)
	if plan := mywant.GetPlan(want, "docker_plan", ""); plan != "" {
		return manageDocker(ctx, want)
	}

	// process_plan: cloudflare-plugin and similar custom-type plugins that use
	// process_* state fields and check process_status in achievedWhen.
	if plan := mywant.GetPlan(want, "process_plan", ""); plan != "" {
		processStatus := mywant.GetCurrent(want, "process_status", "")
		switch plan {
		case "start":
			if processStatus == "running" {
				return launchProcessPoll(want)
			}
			err := launchProcessStart(ctx, want)
			if err == nil {
				want.SetCurrent("process_status", "running")
			} else {
				want.SetCurrent("process_status", "failed")
			}
			return err
		case "stop":
			err := launchProcessStop(want)
			want.SetCurrent("process_status", "stopped")
			return err
		}
		return nil
	}

	return nil
}

func launchStart(ctx context.Context, want *mywant.Want, launchType string) error {
	switch launchType {
	case "docker_compose":
		return launchDockerComposeStart(ctx, want)
	default: // "process" and any other process-based type
		return launchProcessStart(ctx, want)
	}
}

func launchStop(ctx context.Context, want *mywant.Want, launchType string) error {
	switch launchType {
	case "docker_compose":
		return launchDockerComposeStop(ctx, want)
	default: // "process"
		return launchProcessStop(want)
	}
}

func launchPoll(_ context.Context, want *mywant.Want, launchType string) error {
	switch launchType {
	case "docker_compose":
		return launchDockerComposePoll(want)
	default: // "process" and any other process-based type
		return launchProcessPoll(want)
	}
}

// --- Process implementation (delegates to live_server helpers) ---

func launchProcessStart(ctx context.Context, want *mywant.Want) error {
	logFile := mywant.GetCurrent(want, "process_log_file", "")
	if logFile == "" {
		logFile = fmt.Sprintf("/tmp/managed-launch-%s.log", want.Metadata.Name)
		want.SetCurrent("process_log_file", logFile)
	}

	EnsureProcessStopped(want, "process_pid")
	EnsureLogFileTruncated(want, "process_log_file")

	pid, err := startLiveServer(want)
	if err != nil {
		want.SetCurrent("status", "failed")
		want.SetCurrent("launch_error", err.Error())
		return err
	}
	want.SetCurrent("process_pid", pid)
	want.DirectLog("[LAUNCH] Started process PID %d", pid)

	healthURL := getConfigString(want, "server_health_check_url", "health_check_url", "")
	if healthURL != "" {
		if _, err := waitForHealthCheck(ctx, want, healthURL); err != nil {
			want.SetCurrent("status", "failed")
			want.SetCurrent("launch_error", fmt.Sprintf("health check failed: %v", err))
			launchProcessStop(want) //nolint:errcheck
			return err
		}
	}

	want.SetCurrent("status", "running")
	{
		cmd := mywant.GetCurrent(want, "process_command", "")
		summaryJSON, _ := json.Marshal(map[string]any{
			"status":   "running",
			"command":  cmd,
			"pid":      pid,
			"log_file": logFile,
		})
		want.SetCurrent("launch_summary", string(summaryJSON))
	}

	// Optional: extract a URL from the log using url_regex → store in result_field
	urlRegex := mywant.GetCurrent(want, "url_regex", "")
	if urlRegex != "" {
		maxRetries := mywant.GetCurrent(want, "max_retries", 30)
		resultField := mywant.GetCurrent(want, "result_field", "server_url")
		want.DirectLog("[LAUNCH] Waiting for URL pattern in log (max %d retries)...", maxRetries)
		if url := waitForPattern(ctx, want, logFile, urlRegex, maxRetries); url != "" {
			want.DirectLog("[LAUNCH] Captured URL: %s → %s", url, resultField)
			want.SetCurrent(resultField, url)
		} else {
			want.DirectLog("[LAUNCH] Warning: url_regex did not match within %d retries", maxRetries)
		}
	}

	return nil
}

func launchProcessStop(want *mywant.Want) error {
	pid := mywant.GetCurrent(want, "process_pid", 0)
	if pid == 0 {
		want.SetCurrent("status", "stopped")
		return nil
	}
	if err := stopLiveServer(pid, want); err != nil {
		want.DirectLog("[LAUNCH] Failed to stop process PID %d: %v", pid, err)
	}
	want.SetCurrent("process_pid", 0)
	want.SetCurrent("status", "stopped")
	return nil
}

func launchProcessPoll(want *mywant.Want) error {
	pid := mywant.GetCurrent(want, "process_pid", 0)
	if pid == 0 {
		return nil
	}
	if !isProcessAlive(pid) {
		want.SetCurrent("status", "exited")
		want.SetCurrent("launch_error", fmt.Sprintf("process PID %d exited unexpectedly", pid))
		want.DirectLog("[LAUNCH] Process PID %d exited unexpectedly", pid)
	}
	return nil
}

// --- Docker Compose implementation ---

func launchDockerComposeStart(ctx context.Context, want *mywant.Want) error {
	composeFile := mywant.GetCurrent(want, "launch_compose_file", "")
	if composeFile == "" {
		err := fmt.Errorf("launch_compose_file not set")
		want.SetCurrent("status", "failed")
		want.SetCurrent("launch_error", err.Error())
		return err
	}

	want.DirectLog("[LAUNCH] Running: docker compose -f %s up -d", composeFile)
	cmd := exec.CommandContext(ctx, "docker", "compose", "-f", composeFile, "up", "-d", "--remove-orphans")
	cmd.Env = composeEnv(want)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := fmt.Sprintf("docker compose up failed: %v\n%s", err, string(out))
		want.SetCurrent("status", "failed")
		want.SetCurrent("launch_error", msg)
		return fmt.Errorf("%s", msg)
	}
	want.DirectLog("[LAUNCH] docker compose up: %s", strings.TrimSpace(string(out)))

	// Optional health check
	healthURL := getConfigString(want, "server_health_check_url", "health_check_url", "")
	if healthURL != "" {
		if _, err := waitForHealthCheck(ctx, want, healthURL); err != nil {
			want.SetCurrent("status", "failed")
			want.SetCurrent("launch_error", fmt.Sprintf("health check failed: %v", err))
			launchDockerComposeStop(ctx, want) //nolint:errcheck
			return err
		}
	}

	want.SetCurrent("status", "running")
	want.SetCurrent("launch_summary", composeRunningSummary(composeFile))
	return nil
}

func launchDockerComposeStop(ctx context.Context, want *mywant.Want) error {
	composeFile := mywant.GetCurrent(want, "launch_compose_file", "")
	if composeFile == "" {
		want.SetCurrent("status", "stopped")
		return nil
	}

	want.DirectLog("[LAUNCH] Running: docker compose -f %s down", composeFile)
	cmd := exec.CommandContext(ctx, "docker", "compose", "-f", composeFile, "down")
	cmd.Env = composeEnv(want)
	out, err := cmd.CombinedOutput()
	if err != nil {
		want.DirectLog("[LAUNCH] docker compose down warning: %v\n%s", err, string(out))
	} else {
		want.DirectLog("[LAUNCH] docker compose down: %s", strings.TrimSpace(string(out)))
	}
	want.SetCurrent("status", "stopped")
	return nil
}

func launchDockerComposePoll(want *mywant.Want) error {
	composeFile := mywant.GetCurrent(want, "launch_compose_file", "")
	if composeFile == "" {
		return nil
	}

	// Check if at least one service is still running
	out, err := exec.Command("docker", "compose", "-f", composeFile, "ps", "--services", "--filter", "status=running").Output()
	if err != nil {
		return nil // docker not available or compose file gone
	}

	running := strings.TrimSpace(string(out))
	if running == "" {
		// All services stopped
		logs := composeRecentLogs(composeFile, 30)
		want.SetCurrent("status", "exited")
		want.SetCurrent("launch_error", "all compose services stopped unexpectedly")
		want.DirectLog("[LAUNCH] All compose services stopped unexpectedly. Last logs:\n%s", logs)
	}
	return nil
}

// composeEnv returns the environment for docker compose commands.
// It starts from the current process environment and appends variables from two sources:
//  1. State keys with the "launch_env_" prefix — e.g. launch_env_OTP_DATA_DIR → OTP_DATA_DIR=value
//  2. The launch_env state field as a JSON object — e.g. {"OTP_DATA_DIR": "/opt/otp"}
func composeEnv(want *mywant.Want) []string {
	env := os.Environ()

	// On macOS, Docker Desktop installs credential helpers under its app bundle.
	// When the server runs as a daemon it may not have this directory in PATH,
	// causing "docker-credential-desktop: executable file not found in $PATH".
	// Append the well-known location if it exists and is not already in PATH.
	extraPaths := []string{
		"/Applications/Docker.app/Contents/Resources/bin",
		"/usr/local/bin",
	}
	currentPath := os.Getenv("PATH")
	var additions []string
	for _, p := range extraPaths {
		if !strings.Contains(currentPath, p) {
			if _, err := os.Stat(p); err == nil {
				additions = append(additions, p)
			}
		}
	}
	if len(additions) > 0 {
		newPath := strings.Join(additions, ":") + ":" + currentPath
		for i, e := range env {
			if strings.HasPrefix(e, "PATH=") {
				env[i] = "PATH=" + newPath
				break
			}
		}
	}

	// Source 1: launch_env_* prefix keys (YAML-friendly, set via onInitialize.current)
	all := want.GetAllCurrent()
	for k, v := range all {
		if strings.HasPrefix(k, "launch_env_") {
			varName := strings.TrimPrefix(k, "launch_env_")
			env = append(env, fmt.Sprintf("%s=%s", varName, fmt.Sprintf("%v", v)))
		}
	}

	// Source 2: launch_env JSON object (Go-code-friendly)
	if raw := mywant.GetCurrent(want, "launch_env", ""); raw != "" {
		var vars map[string]string
		if err := json.Unmarshal([]byte(raw), &vars); err != nil {
			want.DirectLog("[LAUNCH] Warning: could not parse launch_env JSON: %v", err)
		} else {
			for k, v := range vars {
				env = append(env, fmt.Sprintf("%s=%s", k, v))
			}
		}
	}

	return env
}

// composeRunningSummary returns a JSON summary of running compose services.
func composeRunningSummary(composeFile string) string {
	out, err := exec.Command("docker", "compose", "-f", composeFile, "ps", "--format", "{{.Service}}:{{.State}}").Output()
	var services []string
	if err == nil {
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				services = append(services, line)
			}
		}
	}
	summaryJSON, _ := json.Marshal(map[string]any{
		"status":       "running",
		"compose_file": composeFile,
		"services":     services,
	})
	return string(summaryJSON)
}

// composeRecentLogs returns the last n lines from all compose services.
func composeRecentLogs(composeFile string, lines int) string {
	out, err := exec.Command("docker", "compose", "-f", composeFile, "logs", "--tail", fmt.Sprintf("%d", lines)).CombinedOutput()
	if err != nil && len(out) == 0 {
		return fmt.Sprintf("(failed to get compose logs: %v)", err)
	}
	return strings.TrimSpace(string(out))
}
