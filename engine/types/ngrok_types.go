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
	NgrokPhaseStarting = "starting"
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
	return GetLocals[NgrokLocals](&n.Want)
}

// regex patterns for extracting ngrok forwarding URL
var (
	// TUI format: Forwarding                    https://xxx.ngrok-free.dev -> http://localhost:8080
	forwardingURLPattern = regexp.MustCompile(`Forwarding\s+(https?://\S+)`)
	// logfmt format: url=https://xxx.ngrok-free.dev
	logURLPattern = regexp.MustCompile(`url=(https?://\S+)`)
)

// Initialize prepares the ngrok want for execution
func (n *NgrokWant) Initialize() {
	n.StoreLog("[NGROK] Initializing: %s", n.Metadata.Name)

	locals := n.GetLocals()
	if locals == nil {
		locals = &NgrokLocals{}
		n.Locals = locals
	}
	locals.Phase = NgrokPhaseStarting
	locals.ServerPID = 0
	locals.NgrokURL = ""

	// Read ngrok-specific params
	locals.Port = getStringParam(&n.Want, "port", "8080")
	locals.Protocol = getStringParam(&n.Want, "protocol", "http")

	// Set up log file for capturing ngrok stdout
	logFile := getStringParam(&n.Want, "log_file", "")
	if logFile == "" {
		logFile = fmt.Sprintf("/tmp/ngrok-%s.log", n.Metadata.Name)
	}
	locals.LogFile = logFile

	// Store config in state for live_server_manager agent to read
	argsJSON, _ := json.Marshal([]string{locals.Protocol, locals.Port, "--log=stdout"})
	n.StoreStateMulti(map[string]any{
		"server_phase":    locals.Phase,
		"server_pid":      0,
		"ngrok_url":       "",
		"server_command":  "ngrok",
		"server_args":     string(argsJSON),
		"server_log_file": logFile,
	})

	n.Locals = locals
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
	case NgrokPhaseStarting:
		return 50
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

// Progress implements Progressable for NgrokWant
func (n *NgrokWant) Progress() {
	locals := n.getOrInitializeLocals()

	n.StoreState("achieving_percentage", n.CalculateAchievingPercentage())

	switch locals.Phase {
	case NgrokPhaseStarting:
		// Execute live_server_manager agent to start ngrok process
		if n.GetAgentRegistry() != nil {
			if err := n.ExecuteAgents(); err != nil {
				n.StoreLog("[ERROR] Failed to execute agents: %v", err)
				n.StoreState("server_phase", NgrokPhaseFailed)
				n.StoreState("error_message", fmt.Sprintf("Agent execution failed: %v", err))
				locals.Phase = NgrokPhaseFailed
				n.updateLocals(locals)
				n.Status = "failed"
				return
			}
		}

		// Check if process started, then wait for forwarding URL
		if pid, ok := n.GetStateInt("server_pid", 0); ok && pid > 0 {
			locals.ServerPID = pid

			url := n.waitForNgrokURL(locals.LogFile)
			if url == "" {
				n.StoreLog("[ERROR] Timed out waiting for ngrok forwarding URL")
				n.StoreState("server_phase", NgrokPhaseFailed)
				n.StoreState("error_message", "timed out waiting for ngrok forwarding URL")
				locals.Phase = NgrokPhaseFailed
				n.updateLocals(locals)
				n.Status = "failed"
				return
			}

			locals.NgrokURL = url
			n.StoreState("ngrok_url", url)
			n.StoreState("server_phase", NgrokPhaseRunning)
			n.StoreLog("[NGROK] Tunnel running - PID: %d, URL: %s", pid, url)
			locals.Phase = NgrokPhaseRunning
			n.updateLocals(locals)
			n.ProvideDone()
		}

	case NgrokPhaseRunning:
		break

	case NgrokPhaseStopping:
		if pid, _ := n.GetStateInt("server_pid", 0); pid == 0 {
			n.StoreLog("[NGROK] Tunnel stopped successfully")
			n.StoreState("server_phase", NgrokPhaseStopped)
			n.StoreState("ngrok_url", "")
			locals.Phase = NgrokPhaseStopped
			n.updateLocals(locals)
		}

	case NgrokPhaseStopped, NgrokPhaseFailed:
		break

	default:
		n.SetModuleError("Phase", fmt.Sprintf("Unknown phase: %s", locals.Phase))
		n.updateLocals(locals)
	}
}

// OnDelete stops the ngrok tunnel when the want is deleted
func (n *NgrokWant) OnDelete() {
	n.StoreLog("[NGROK] Want is being deleted, stopping tunnel")

	// Kill process directly — don't rely on agent round-trip through state
	if pid, ok := n.GetStateInt("server_pid", 0); ok && pid > 0 {
		n.StoreLog("[NGROK] Killing ngrok process PID %d", pid)
		if proc, err := os.FindProcess(pid); err == nil {
			if err := proc.Signal(syscall.SIGTERM); err != nil {
				n.StoreLog("[WARN] SIGTERM failed for PID %d, trying SIGKILL: %v", pid, err)
				proc.Kill()
			}
		}
		n.StoreState("server_pid", 0)
	}

	if err := n.StopAllBackgroundAgents(); err != nil {
		n.StoreLog("[ERROR] Failed to stop background agents: %v", err)
	}

	// Clean up log file
	locals := n.getOrInitializeLocals()
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
		// Try TUI format: Forwarding  https://xxx.ngrok-free.dev -> ...
		if m := forwardingURLPattern.FindStringSubmatch(line); len(m) > 1 {
			return m[1]
		}
		// Try logfmt format: url=https://xxx.ngrok-free.dev
		if m := logURLPattern.FindStringSubmatch(line); len(m) > 1 {
			return m[1]
		}
	}
	return ""
}

// getOrInitializeLocals retrieves or initializes the locals
func (n *NgrokWant) getOrInitializeLocals() *NgrokLocals {
	if locals := n.GetLocals(); locals != nil {
		return locals
	}

	locals := &NgrokLocals{
		Phase: NgrokPhaseStarting,
	}

	n.GetStateMulti(Dict{
		"server_phase": &locals.Phase,
		"server_pid":   &locals.ServerPID,
		"ngrok_url":    &locals.NgrokURL,
	})

	locals.Port = getStringParam(&n.Want, "port", "8080")
	locals.Protocol = getStringParam(&n.Want, "protocol", "http")
	logFile := getStringParam(&n.Want, "log_file", "")
	if logFile == "" {
		logFile = fmt.Sprintf("/tmp/ngrok-%s.log", n.Metadata.Name)
	}
	locals.LogFile = logFile

	return locals
}

// updateLocals updates the in-memory locals
func (n *NgrokWant) updateLocals(locals *NgrokLocals) {
	n.Locals = locals
}
