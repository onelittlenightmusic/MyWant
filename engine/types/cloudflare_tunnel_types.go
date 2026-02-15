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
	RegisterWantImplementation[CloudflareTunnelWant, CloudflareTunnelLocals]("cloudflare_tunnel")
}

// CloudflareTunnelPhase constants
const (
	CloudflareTunnelPhaseRunning  = "running"
	CloudflareTunnelPhaseStopping = "stopping"
	CloudflareTunnelPhaseStopped  = "stopped"
	CloudflareTunnelPhaseFailed   = "failed"
)

// CloudflareTunnelLocals holds type-specific local state for CloudflareTunnelWant
type CloudflareTunnelLocals struct {
	Phase      string
	Port       string
	Protocol   string
	LogFile    string
	ServerPID  int
	TunnelURL  string
}

// CloudflareTunnelWant manages a Cloudflare Tunnel (cloudflared) lifecycle using the live_server_manager agent
type CloudflareTunnelWant struct {
	Want
}

func (n *CloudflareTunnelWant) GetLocals() *CloudflareTunnelLocals {
	return GetLocals[CloudflareTunnelLocals](&n.Want)
}

// regex patterns for extracting cloudflare tunnel URL
var (
	// Example: |  Your quick tunnel has been created! Visit it at https://xxx.trycloudflare.com
	cloudflareURLPattern = regexp.MustCompile(`Your quick tunnel has been created! Visit it at\s+(https?://\S+)`)
)

// Initialize starts the cloudflared process and waits for the forwarding URL
func (n *CloudflareTunnelWant) Initialize() {
	n.StoreLog("[CLOUDFLARE] Initializing: %s", n.Metadata.Name)

	locals := n.GetLocals()
	if locals == nil {
		locals = &CloudflareTunnelLocals{}
		n.Locals = locals
	}
	locals.ServerPID = 0
	locals.TunnelURL = ""

	// Read cloudflare-specific params
	locals.Port = getStringParam(&n.Want, "port", "8080")
	locals.Protocol = getStringParam(&n.Want, "protocol", "http")

	// Set up log file for capturing cloudflared stdout/stderr
	logFile := getStringParam(&n.Want, "log_file", "")
	if logFile == "" {
		logFile = fmt.Sprintf("/tmp/cloudflare-%s.log", n.Metadata.Name)
	}
	locals.LogFile = logFile

	// Target URL for the tunnel
	targetURL := fmt.Sprintf("%s://localhost:%s", locals.Protocol, locals.Port)

	// Store config in state for live_server_manager agent to read
	// cloudflared tunnel --url http://localhost:8080
	argsJSON, _ := json.Marshal([]string{"tunnel", "--url", targetURL})
	n.StoreStateMulti(map[string]any{
		"server_phase":    "starting",
		"server_pid":      0,
		"tunnel_url":      "",
		"server_command":  "cloudflared",
		"server_args":     string(argsJSON),
		"server_log_file": logFile,
	})

	// Start cloudflared process via live_server_manager agent
	if err := n.ExecuteAgents(); err != nil {
		n.failWithError(locals, fmt.Sprintf("Agent execution failed: %v", err))
		return
	}

	pid, ok := n.GetStateInt("server_pid", 0)
	if !ok || pid == 0 {
		n.failWithError(locals, "agent did not start cloudflared process")
		return
	}
	locals.ServerPID = pid

	// Wait for tunnel URL in log file
	url := n.waitForTunnelURL(logFile)
	if url == "" {
		n.failWithError(locals, "timed out waiting for cloudflare tunnel URL")
		return
	}

	locals.TunnelURL = url
	locals.Phase = CloudflareTunnelPhaseRunning
	n.StoreStateMulti(map[string]any{
		"tunnel_url":   url,
		"server_phase": CloudflareTunnelPhaseRunning,
	})
	n.StoreLog("[CLOUDFLARE] Tunnel running - PID: %d, URL: %s", pid, url)
	n.Locals = locals
}

// IsAchieved checks if the cloudflare tunnel is running with a public URL
func (n *CloudflareTunnelWant) IsAchieved() bool {
	phase, _ := n.GetStateString("server_phase", "")
	url, _ := n.GetStateString("tunnel_url", "")
	return phase == CloudflareTunnelPhaseRunning && url != ""
}

// CalculateAchievingPercentage returns the progress percentage
func (n *CloudflareTunnelWant) CalculateAchievingPercentage() int {
	if n.IsAchieved() || n.Status == WantStatusAchieved {
		return 100
	}
	phase, _ := n.GetStateString("server_phase", "")
	switch phase {
	case CloudflareTunnelPhaseRunning:
		return 100
	case CloudflareTunnelPhaseStopping:
		return 75
	case CloudflareTunnelPhaseStopped, CloudflareTunnelPhaseFailed:
		return 0
	default:
		return 0
	}
}

// failWithError transitions to failed phase with an error message
func (n *CloudflareTunnelWant) failWithError(locals *CloudflareTunnelLocals, msg string) {
	n.StoreLog("[ERROR] %s", msg)
	locals.Phase = CloudflareTunnelPhaseFailed
	n.StoreState("server_phase", CloudflareTunnelPhaseFailed)
	n.StoreState("error_message", msg)
	n.Locals = locals
	n.Status = "failed"
}

// Progress implements Progressable for CloudflareTunnelWant
func (n *CloudflareTunnelWant) Progress() {
	locals := n.getOrInitializeLocals()
	n.StoreState("achieving_percentage", n.CalculateAchievingPercentage())

	switch locals.Phase {
	case CloudflareTunnelPhaseRunning:
		n.ProvideDone()

	case CloudflareTunnelPhaseStopping:
		if pid, _ := n.GetStateInt("server_pid", 0); pid == 0 {
			n.StoreState("tunnel_url", "")
			locals.Phase = CloudflareTunnelPhaseStopped
			n.StoreState("server_phase", CloudflareTunnelPhaseStopped)
			n.Locals = locals
			n.StoreLog("[CLOUDFLARE] Tunnel stopped successfully")
		}

	case CloudflareTunnelPhaseStopped, CloudflareTunnelPhaseFailed:
		// Nothing to do
	}
}

// OnDelete stops the cloudflare tunnel when the want is deleted
func (n *CloudflareTunnelWant) OnDelete() {
	n.StoreLog("[CLOUDFLARE] Want is being deleted, stopping tunnel")

	// Kill process directly (and its process group)
	if pid, ok := n.GetStateInt("server_pid", 0); ok && pid > 0 {
		n.StoreLog("[CLOUDFLARE] Killing cloudflared process group PID %d", pid)
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
	locals := n.getOrInitializeLocals()
	if locals.LogFile != "" {
		os.Remove(locals.LogFile)
	}
}

// waitForTunnelURL polls the log file until the forwarding URL appears
func (n *CloudflareTunnelWant) waitForTunnelURL(logFile string) string {
	const maxRetries = 40 // Cloudflare might take a bit longer
	const interval = 500 * time.Millisecond

	for i := 0; i < maxRetries; i++ {
		if url := parseCloudflareURL(logFile); url != "" {
			return url
		}
		n.StoreLog("[DEBUG] Waiting for cloudflare URL (attempt %d/%d)...", i+1, maxRetries)
		time.Sleep(interval)
	}
	return ""
}

// parseCloudflareURL reads the log file and extracts the cloudflare tunnel URL
func parseCloudflareURL(logFile string) string {
	f, err := os.Open(logFile)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if m := cloudflareURLPattern.FindStringSubmatch(line); len(m) > 1 {
			return m[1]
		}
	}
	return ""
}

// getOrInitializeLocals retrieves or initializes the locals
func (n *CloudflareTunnelWant) getOrInitializeLocals() *CloudflareTunnelLocals {
	if locals := n.GetLocals(); locals != nil {
		return locals
	}

	locals := &CloudflareTunnelLocals{}

	n.GetStateMulti(Dict{
		"server_phase": &locals.Phase,
		"server_pid":   &locals.ServerPID,
		"tunnel_url":   &locals.TunnelURL,
	})

	locals.Port = getStringParam(&n.Want, "port", "8080")
	locals.Protocol = getStringParam(&n.Want, "protocol", "http")
	logFile := getStringParam(&n.Want, "log_file", "")
	if logFile == "" {
		logFile = fmt.Sprintf("/tmp/cloudflare-%s.log", n.Metadata.Name)
	}
	locals.LogFile = logFile

	return locals
}
