package types

import (
	"os"
	"time"

	mywant "mywant/engine/core"
)

// isProcessAlive returns true if a process with the given PID is still running.
func isProcessAlive(pid int) bool {
	if pid == 0 {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Unix, Signal(0) checks if the process exists without sending a signal
	err = process.Signal(os.Signal(nil))
	return err == nil
}

// IdempotentStart ensures an agent can be safely re-invoked by cleaning up any
// resources left over from a previous (possibly crashed) execution before
// proceeding with a fresh start.
//
// Pattern for process-managing agents:
//
//	func myAgent(ctx context.Context, want *mywant.Want) error {
//	    EnsureProcessStopped(want, "server_pid")
//	    EnsureLogFileTruncated(want, "server_log_file")
//	    // ... start fresh
//	}

// EnsureProcessStopped kills the process whose PID is stored in pidStateKey
// and clears the state entry. Safe to call when no process is running (PID == 0).
func EnsureProcessStopped(want *mywant.Want, pidStateKey string) {
	pid := mywant.GetCurrent(want, pidStateKey, 0)
	if pid == 0 {
		return
	}
	want.DirectLog("[IDEMPOTENT] Killing stale process PID %d (key: %s)", pid, pidStateKey)
	stopLiveServer(pid, want)
	want.SetCurrent(pidStateKey, 0)
	time.Sleep(300 * time.Millisecond) // wait for process group to exit
}

// EnsureLogFileTruncated truncates the log file whose path is stored in
// logFileStateKey so that pattern searches start from fresh output.
// Safe to call when the key is empty or the file does not exist.
func EnsureLogFileTruncated(want *mywant.Want, logFileStateKey string) {
	path := mywant.GetCurrent(want, logFileStateKey, "")
	if path == "" {
		return
	}
	if err := os.Truncate(path, 0); err != nil && !os.IsNotExist(err) {
		want.DirectLog("[IDEMPOTENT] Warning: could not truncate log file %s: %v", path, err)
	}
}
