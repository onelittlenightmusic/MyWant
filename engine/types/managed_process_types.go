package types

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"syscall"
	"time"

	. "mywant/engine/core"
)

func init() {
	RegisterWantImplementation[ManagedProcessWant, ManagedProcessLocals]("managed_process")
	RegisterWantImplementation[ManagedProcessWant, ManagedProcessLocals]("ngrok")
	RegisterWantImplementation[ManagedProcessWant, ManagedProcessLocals]("cloudflare_tunnel")
}

// ManagedProcessPhase constants
const (
	ProcessPhaseRunning  = "running"
	ProcessPhaseStopping = "stopping"
	ProcessPhaseStopped  = "stopped"
	ProcessPhaseFailed   = "failed"
)

// ManagedProcessLocals holds type-specific local state
type ManagedProcessLocals struct {
	ServerPhase   string
	ServerLogFile string
	ServerPid     int
	ResultUrl     string
}

// ManagedProcessWant is a generic want type for managing external processes
type ManagedProcessWant struct {
	Want
}

func (n *ManagedProcessWant) GetLocals() *ManagedProcessLocals {
	return CheckLocalsInitialized[ManagedProcessLocals](&n.Want)
}

// Initialize starts the process and waits for a result URL in the logs
func (n *ManagedProcessWant) Initialize() {
	locals := n.GetLocals()
	locals.ServerPid = 0
	locals.ResultUrl = ""

	// 1. Read process configuration from params
	command := n.GetStringParam("command", "")
	if command == "" {
		n.failWithError(locals, "command parameter is required")
		return
	}

	// Arguments can be a JSON array string or a simple string (space-separated)
	var args []string
	argsParam := n.GetStringParam("args", "[]")
	if err := json.Unmarshal([]byte(argsParam), &args); err != nil {
		// Fallback to space-separated string if not JSON
		args = strings.Split(argsParam, " ")
	}

	// Log file configuration
	logFile := n.GetStringParam("log_file", "")
	if logFile == "" {
		logFile = fmt.Sprintf("/tmp/managed-process-%s.log", n.Metadata.Name)
	}
	locals.ServerLogFile = logFile

	n.DirectLog("[%s] Initializing process: %s %v", n.Metadata.Name, command, args)

	// 2. Configure state for live_server_manager agent
	argsJSON, _ := json.Marshal(args)
	n.SetCurrent("server_phase", "starting")
	n.SetCurrent("server_pid", 0)
	n.SetCurrent("server_command", command)
	n.SetCurrent("server_args", string(argsJSON))
	n.SetCurrent("server_log_file", logFile)

	// 3. Start process via agent
	if err := n.ExecuteAgents(); err != nil {
		n.failWithError(locals, fmt.Sprintf("Agent execution failed: %v", err))
		return
	}

	pid := GetCurrent(n, "server_pid", 0)
	if pid == 0 {
		n.failWithError(locals, "agent did not start process")
		return
	}
	locals.ServerPid = pid

	// 4. Optionally wait for a pattern in logs
	urlRegex := n.GetStringParam("url_regex", "")
	resultField := n.GetStringParam("result_field", "result_url")
	// Promote url_regex, result_field and max_retries params → state so IsAchieved/waitForPattern read from GetCurrent
	n.SetCurrent("url_regex", urlRegex)
	n.SetCurrent("result_field", resultField)
	n.SetCurrent("max_retries", n.GetIntParam("max_retries", 40))
	if urlRegex != "" {
		n.DirectLog("[%s] Waiting for result URL in %s", n.Metadata.Name, logFile)
		url := n.waitForPattern(logFile, urlRegex)
		if url == "" {
			n.failWithError(locals, fmt.Sprintf("timed out waiting for result URL in %s", logFile))
			return
		}
		locals.ResultUrl = url
		n.SetCurrent("result_url", url)
		// Specific field for backward compatibility or UI (can be parameterized)
		n.SetCurrent(resultField, url)
	}

	locals.ServerPhase = ProcessPhaseRunning
	n.SetCurrent("server_phase", ProcessPhaseRunning)
	n.DirectLog("[%s] Process running - PID: %d, URL: %s", n.Metadata.Name, pid, locals.ResultUrl)
}

func (n *ManagedProcessWant) IsAchieved() bool {
	phase := GetCurrent(n, "server_phase", "")
	urlRegex := GetCurrent(n, "url_regex", "")
	if urlRegex != "" {
		url := GetCurrent(n, "result_url", "")
		return phase == ProcessPhaseRunning && url != ""
	}
	return phase == ProcessPhaseRunning
}

func (n *ManagedProcessWant) CalculateAchievingPercentage() int {
	if n.IsAchieved() || n.Status == WantStatusAchieved {
		return 100
	}
	phase := GetCurrent(n, "server_phase", "")
	switch phase {
	case ProcessPhaseRunning: return 100
	case ProcessPhaseStopping: return 75
	default: return 0
	}
}

func (n *ManagedProcessWant) failWithError(locals *ManagedProcessLocals, msg string) {
	n.DirectLog("[ERROR] %s", msg)
	locals.ServerPhase = ProcessPhaseFailed
	n.SetCurrent("server_phase", ProcessPhaseFailed)
	n.SetCurrent("error_message", msg)
	n.Status = "failed"
}

func (n *ManagedProcessWant) Progress() {
	locals := n.GetLocals()
	n.SetCurrent("achieving_percentage", n.CalculateAchievingPercentage())

	switch locals.ServerPhase {
	case ProcessPhaseRunning:
		n.ProvideDone()
	case ProcessPhaseStopping:
		if locals.ServerPid == 0 {
			locals.ServerPhase = ProcessPhaseStopped
			n.StoreLog("[%s] Process stopped successfully", n.Metadata.Name)
		}
	}
}

func (n *ManagedProcessWant) OnDelete() {
	n.StoreLog("[%s] Want is being deleted, stopping process", n.Metadata.Name)

	if pid := GetCurrent(n, "server_pid", 0); pid > 0 {
		syscall.Kill(-pid, syscall.SIGTERM)
		n.SetCurrent("server_pid", 0)
	}

	n.StopAllBackgroundAgents()

	locals := n.GetLocals()
	if locals.ServerLogFile != "" {
		os.Remove(locals.ServerLogFile)
	}
}

func (n *ManagedProcessWant) waitForPattern(logFile, pattern string) string {
	re, err := regexp.Compile(pattern)
	if err != nil {
		n.DirectLog("[ERROR] Invalid regex: %v", err)
		return ""
	}

	maxRetries := GetCurrent(n, "max_retries", 40)
	interval := 500 * time.Millisecond

	for i := range maxRetries {
		if url := parsePattern(logFile, re); url != "" {
			return url
		}
		n.DirectLog("[DEBUG] Waiting for pattern (attempt %d/%d)...", i+1, maxRetries)
		time.Sleep(interval)
	}
	return ""
}

func parsePattern(logFile string, re *regexp.Regexp) string {
	f, err := os.Open(logFile)
	if err != nil { return "" }
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if m := re.FindStringSubmatch(scanner.Text()); len(m) > 1 {
			return m[1]
		}
	}
	return ""
}
