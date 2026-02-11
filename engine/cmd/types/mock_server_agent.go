package types

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"syscall"

	mywant "mywant/engine/core"
)

func init() {
	mywant.RegisterDoAgentType("mock_server_manager",
		[]mywant.Capability{mywant.Cap("mock_server_management")},
		manageMockServer)
}

// manageMockServer handles mock server start and stop based on want phase
func manageMockServer(ctx context.Context, want *mywant.Want) error {
	want.StoreLog("[AGENT] manageMockServer called for want %s", want.Metadata.Name)

	// Get current phase
	phase, ok := want.GetStateString("server_phase", "")
	if !ok || phase == "" {
		// No phase set yet, nothing to do
		want.StoreLog("[AGENT] No server_phase set, returning")
		return nil
	}

	// Check if we already have a server PID
	existingPID, _ := want.GetStateInt("server_pid", 0)

	// Get server binary path from params (default: ./bin/flight-server)
	serverBinary := "./bin/flight-server"
	if binPath, ok := want.Spec.Params["server_binary"]; ok {
		serverBinary = fmt.Sprintf("%v", binPath)
	}

	// Handle phase transitions
	switch phase {
	case "starting":
		// Start server if not already running
		if existingPID == 0 {
			pid, err := startMockServer(serverBinary, want)
			if err != nil {
				want.StoreLog("[ERROR] Failed to start mock server: %v", err)
				return err
			}

			// Store PID in want state
			want.StoreState("server_pid", pid)
			want.StoreLog("[INFO] Started mock server with PID %d", pid)
		} else {
			want.StoreLog("[INFO] Mock server already running with PID %d", existingPID)
		}

	case "stopping":
		// Stop server if running
		if existingPID != 0 {
			err := stopMockServer(existingPID, want)
			if err != nil {
				// Log error but don't fail - cleanup is not critical
				want.StoreLog("[WARN] Failed to stop mock server PID %d: %v", existingPID, err)
			} else {
				want.StoreLog("[INFO] Stopped mock server PID %d", existingPID)
				// Clear PID from state
				want.StoreState("server_pid", 0)
			}
		}
	}

	return nil
}

// startMockServer starts the flight mock server binary
func startMockServer(binaryPath string, want *mywant.Want) (int, error) {
	// Check if binary exists
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		return 0, fmt.Errorf("mock server binary not found: %s", binaryPath)
	}

	// Get log file path from params (default: logs/flight-server.log)
	logFile := "logs/flight-server.log"
	if logPath, ok := want.Spec.Params["log_file"]; ok {
		logFile = fmt.Sprintf("%v", logPath)
	}

	// Create logs directory if it doesn't exist
	if err := os.MkdirAll("logs", 0755); err != nil {
		return 0, fmt.Errorf("failed to create logs directory: %w", err)
	}

	// Open log file
	logFileHandle, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return 0, fmt.Errorf("failed to open log file: %w", err)
	}

	// Start the server process
	cmd := exec.Command(binaryPath)
	cmd.Stdout = logFileHandle
	cmd.Stderr = logFileHandle

	// Set process group so we can kill it properly
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		logFileHandle.Close()
		return 0, fmt.Errorf("failed to start mock server: %w", err)
	}

	pid := cmd.Process.Pid
	want.StoreLog("[INFO] Mock server started: %s (PID: %d, logs: %s)", binaryPath, pid, logFile)

	// Store the log file handle so it can be closed later
	// Note: In production, you might want to close this immediately and let the process handle its own logging
	go func() {
		cmd.Wait()
		logFileHandle.Close()
	}()

	return pid, nil
}

// stopMockServer stops the mock server process by PID
func stopMockServer(pid int, want *mywant.Want) error {
	// Find the process
	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("failed to find process %d: %w", pid, err)
	}

	// Send SIGTERM signal for graceful shutdown
	if err := process.Signal(syscall.SIGTERM); err != nil {
		// If SIGTERM fails, try SIGKILL
		want.StoreLog("[WARN] SIGTERM failed for PID %d, trying SIGKILL: %v", pid, err)
		if err := process.Kill(); err != nil {
			return fmt.Errorf("failed to kill process %d: %w", pid, err)
		}
	}

	want.StoreLog("[INFO] Sent termination signal to mock server PID %d", pid)
	return nil
}

