package types

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"syscall"
	"time"

	. "mywant/engine/core"
)

func init() {
	RegisterWantImplementation[NgrokWant, NgrokLocals]("ngrok")
}

// NgrokPhase constants
const (
	NgrokPhaseRunning  = "running"
	NgrokPhaseStopping = "stopping"
	NgrokPhaseStopped  = "stopped"
	NgrokPhaseFailed   = "failed"
)

// NgrokLocals holds type-specific local state for NgrokWant
type NgrokLocals struct {
	Phase     string
	Port      string
	Protocol  string
	LogFile   string
	ServerPID int
	NgrokURL  string
}

// NgrokWant manages an ngrok tunnel lifecycle using the live_server_manager agent
type NgrokWant struct {
	Want
}

func (n *NgrokWant) GetLocals() *NgrokLocals {
	return CheckLocalsInitialized[NgrokLocals](&n.Want)
}

// regex patterns for extracting ngrok forwarding URL
var (
	// TUI format: Forwarding                    https://xxx.ngrok-free.dev -> http://localhost:8080
	forwardingURLPattern = regexp.MustCompile(`Forwarding\s+(https?://\S+)`)
	// logfmt format: url=https://xxx.ngrok-free.dev
	logURLPattern = regexp.MustCompile(`url=(https?://\S+)`)
)

// Initialize starts the ngrok process and waits for the forwarding URL
func (n *NgrokWant) Initialize() {
	n.StoreLog("[NGROK] Initializing: %s", n.Metadata.Name)

	// Get locals (guaranteed to be initialized by framework)
	locals := n.GetLocals()
	locals.ServerPID = 0
	locals.NgrokURL = ""

	// Read ngrok-specific params
	locals.Port = n.GetStringParam("port", "8080")
	locals.Protocol = n.GetStringParam("protocol", "http")

	// Set up log file for capturing ngrok stdout
	logFile := n.GetStringParam("log_file", "")
	if logFile == "" {
		logFile = fmt.Sprintf("/tmp/ngrok-%s.log", n.Metadata.Name)
	}
	locals.LogFile = logFile

	// Store config in state for live_server_manager agent to read
	argsJSON, _ := json.Marshal([]string{locals.Protocol, locals.Port, "--log=stdout"})
	n.StoreStateMulti(map[string]any{
		"server_phase":    "starting",
		"server_pid":      0,
		"ngrok_url":       "",
		"server_command":  "ngrok",
		"server_args":     string(argsJSON),
		"server_log_file": logFile,
	})

	// Start ngrok process via live_server_manager agent
	if err := n.ExecuteAgents(); err != nil {
		n.failWithError(locals, fmt.Sprintf("Agent execution failed: %v", err))
		return
	}

	pid, ok := n.GetStateInt("server_pid", 0)
	if !ok || pid == 0 {
		n.failWithError(locals, "agent did not start ngrok process")
		return
	}
	locals.ServerPID = pid

	// Wait for forwarding URL in log file
	url := n.waitForNgrokURL(logFile)
	if url == "" {
		n.failWithError(locals, "timed out waiting for ngrok forwarding URL")
		return
	}

	locals.NgrokURL = url
	locals.Phase = NgrokPhaseRunning
	n.StoreStateMulti(map[string]any{
		"ngrok_url":    url,
		"server_phase": NgrokPhaseRunning,
	})
	n.StoreLog("[NGROK] Tunnel running - PID: %d, URL: %s", pid, url)
}

// IsAchieved checks if the ngrok tunnel is running with a public URL
func (n *NgrokWant) IsAchieved() bool {
	phase, _ := n.GetStateString("server_phase", "")
	url, _ := n.GetStateString("ngrok_url", "")
	return phase == NgrokPhaseRunning && url != ""
}

// CalculateAchievingPercentage returns the progress percentage
func (n *NgrokWant) CalculateAchievingPercentage() int {
	if n.IsAchieved() || n.Status == WantStatusAchieved {
		return 100
	}
	phase, _ := n.GetStateString("server_phase", "")
	switch phase {
	case NgrokPhaseRunning:
		return 100
	case NgrokPhaseStopping:
		return 75
	case NgrokPhaseStopped, NgrokPhaseFailed:
		return 0
	default:
		return 0
	}
}

// failWithError transitions to failed phase with an error message
func (n *NgrokWant) failWithError(locals *NgrokLocals, msg string) {
	n.StoreLog("[ERROR] %s", msg)
	locals.Phase = NgrokPhaseFailed
	n.StoreState("server_phase", NgrokPhaseFailed)
	n.StoreState("error_message", msg)
	n.Status = "failed"
}

// Progress implements Progressable for NgrokWant
func (n *NgrokWant) Progress() {
	locals := n.GetLocals()
	n.StoreState("achieving_percentage", n.CalculateAchievingPercentage())

	switch locals.Phase {
	case NgrokPhaseRunning:
		n.ProvideDone()

	case NgrokPhaseStopping:
		if pid, _ := n.GetStateInt("server_pid", 0); pid == 0 {
			n.StoreState("ngrok_url", "")
			locals.Phase = NgrokPhaseStopped
			n.StoreState("server_phase", NgrokPhaseStopped)
			n.StoreLog("[NGROK] Tunnel stopped successfully")
		}

	case NgrokPhaseStopped, NgrokPhaseFailed:
		// Nothing to do
	}
}

// OnDelete stops the ngrok tunnel when the want is deleted
func (n *NgrokWant) OnDelete() {
	n.StoreLog("[NGROK] Want is being deleted, stopping tunnel")

	// Kill process directly (and its process group)
	if pid, ok := n.GetStateInt("server_pid", 0); ok && pid > 0 {
		n.StoreLog("[NGROK] Killing ngrok process group PID %d", pid)
		// Try to kill process group first (since live_server_manager starts with Setpgid: true)
		if err := syscall.Kill(-pid, syscall.SIGTERM); err != nil {
			n.StoreLog("[DEBUG] SIGTERM to process group %d failed: %v, trying individual process", pid, err)
			if proc, err := os.FindProcess(pid); err == nil {
				if err := proc.Signal(syscall.SIGTERM); err != nil {
					n.StoreLog("[WARN] SIGTERM failed for PID %d, trying SIGKILL: %v", pid, err)
					proc.Kill()
				}
			}
		}
		n.StoreState("server_pid", 0)
	}

	if err := n.StopAllBackgroundAgents(); err != nil {
		n.StoreLog("[ERROR] Failed to stop background agents: %v", err)
	}

	// Clean up log file
	locals := n.GetLocals()
	if locals.LogFile != "" {
		os.Remove(locals.LogFile)
	}
}

// waitForNgrokURL polls the log file until the forwarding URL appears
func (n *NgrokWant) waitForNgrokURL(logFile string) string {
	const maxRetries = 30
	const interval = 500 * time.Millisecond

	for i := 0; i < maxRetries; i++ {
		if url := parseNgrokURL(logFile); url != "" {
			return url
		}
		n.StoreLog("[DEBUG] Waiting for ngrok URL (attempt %d/%d)...", i+1, maxRetries)
		time.Sleep(interval)
	}
	return ""
}

// parseNgrokURL reads the log file and extracts the ngrok forwarding URL
func parseNgrokURL(logFile string) string {
	f, err := os.Open(logFile)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if m := forwardingURLPattern.FindStringSubmatch(line); len(m) > 1 {
			return m[1]
		}
		if m := logURLPattern.FindStringSubmatch(line); len(m) > 1 {
			return m[1]
		}
	}
	return ""
}
