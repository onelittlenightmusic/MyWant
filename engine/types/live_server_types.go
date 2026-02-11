package types

import (
	"fmt"
	"time"

	. "mywant/engine/core"
)

func init() {
	RegisterWantImplementation[LiveServerWant, LiveServerLocals]("live_server")
}

// LiveServerPhase constants
const (
	LiveServerPhaseStarting = "starting"
	LiveServerPhaseRunning  = "running"
	LiveServerPhaseStopping = "stopping"
	LiveServerPhaseStopped  = "stopped"
	LiveServerPhaseFailed   = "failed"
)

// LiveServerLocals holds type-specific local state for LiveServerWant
type LiveServerLocals struct {
	Phase                 string
	Command               string
	Args                  []string
	LogFile               string
	HealthCheckURL        string
	HealthCheckInterval   string
	HealthCheckMaxRetries int
	ServerPID             int
	LastCheckTime         time.Time
}

// LiveServerWant represents a want that manages a generic server/process lifecycle
type LiveServerWant struct {
	Want
}

func (l *LiveServerWant) GetLocals() *LiveServerLocals {
	return GetLocals[LiveServerLocals](&l.Want)
}

// Initialize prepares the live server want for execution
func (l *LiveServerWant) Initialize() {
	l.StoreLog("[LIVE_SERVER] Initializing: %s", l.Metadata.Name)

	locals := l.GetLocals()
	if locals == nil {
		locals = &LiveServerLocals{}
		l.Locals = locals
	}
	locals.Phase = LiveServerPhaseStarting
	locals.LastCheckTime = time.Now()
	locals.ServerPID = 0

	// Parse params
	locals.Command = getStringParam(&l.Want, "command", "")
	locals.Args = getArgsParam(&l.Want)
	locals.LogFile = getStringParam(&l.Want, "log_file", "")
	locals.HealthCheckURL = getStringParam(&l.Want, "health_check_url", "")
	locals.HealthCheckInterval = getStringParam(&l.Want, "health_check_interval", "500ms")
	locals.HealthCheckMaxRetries = getIntParam(&l.Want, "health_check_max_retries", 15)

	// Store initial state
	stateMap := map[string]any{
		"server_phase":          locals.Phase,
		"server_pid":            0,
		"health_check_response": "",
	}

	l.StoreStateMulti(stateMap)
	l.Locals = locals
}

// IsAchieved checks if the server is running (and health check passed if configured)
func (l *LiveServerWant) IsAchieved() bool {
	phase, _ := l.GetStateString("server_phase", "")
	if phase != LiveServerPhaseRunning {
		return false
	}
	// If health_check_url is configured, also require a response
	if url := getStringParam(&l.Want, "health_check_url", ""); url != "" {
		resp, _ := l.GetStateString("health_check_response", "")
		return resp != ""
	}
	return true
}

// CalculateAchievingPercentage returns the progress percentage
func (l *LiveServerWant) CalculateAchievingPercentage() int {
	if l.IsAchieved() || l.Status == WantStatusAchieved {
		return 100
	}
	phase, _ := l.GetStateString("server_phase", "")
	switch phase {
	case LiveServerPhaseStarting:
		return 50
	case LiveServerPhaseRunning:
		return 100
	case LiveServerPhaseStopping:
		return 75
	case LiveServerPhaseStopped, LiveServerPhaseFailed:
		return 0
	default:
		return 0
	}
}

// Progress implements Progressable for LiveServerWant
func (l *LiveServerWant) Progress() {
	locals := l.getOrInitializeLocals()

	l.StoreState("achieving_percentage", l.CalculateAchievingPercentage())

	switch locals.Phase {
	case LiveServerPhaseStarting:
		if l.GetAgentRegistry() != nil {
			if err := l.ExecuteAgents(); err != nil {
				l.StoreLog("[ERROR] Failed to execute agents: %v", err)
				l.StoreState("server_phase", LiveServerPhaseFailed)
				l.StoreState("error_message", fmt.Sprintf("Agent execution failed: %v", err))
				locals.Phase = LiveServerPhaseFailed
				l.updateLocals(locals)
				l.Status = "failed"
				return
			}
		}

		// Check if we have a PID (server started)
		if pid, ok := l.GetStateInt("server_pid", 0); ok && pid > 0 {
			locals.ServerPID = pid

			// If health check is configured, check for response
			if url := getStringParam(&l.Want, "health_check_url", ""); url != "" {
				resp, _ := l.GetStateString("health_check_response", "")
				if resp == "" {
					l.updateLocals(locals)
					return
				}
			}

			l.StoreLog("[LIVE_SERVER] Server is running with PID %d", pid)
			l.StoreState("server_phase", LiveServerPhaseRunning)
			locals.Phase = LiveServerPhaseRunning
			l.updateLocals(locals)
			l.ProvideDone()
		}

	case LiveServerPhaseRunning:
		break

	case LiveServerPhaseStopping:
		if pid, _ := l.GetStateInt("server_pid", 0); pid == 0 {
			l.StoreLog("[LIVE_SERVER] Server stopped successfully")
			l.StoreState("server_phase", LiveServerPhaseStopped)
			locals.Phase = LiveServerPhaseStopped
			l.updateLocals(locals)
		}

	case LiveServerPhaseStopped, LiveServerPhaseFailed:
		break

	default:
		l.SetModuleError("Phase", fmt.Sprintf("Unknown phase: %s", locals.Phase))
		l.updateLocals(locals)
	}
}

// OnDelete stops the server process when the want is deleted
func (l *LiveServerWant) OnDelete() {
	l.StoreLog("[LIVE_SERVER] Want is being deleted, stopping server")

	l.StoreState("server_phase", LiveServerPhaseStopping)

	if err := l.ExecuteAgents(); err != nil {
		l.StoreLog("[ERROR] Failed to stop server during deletion: %v", err)
	} else {
		l.StoreLog("[LIVE_SERVER] Server stopped successfully")
	}

	if err := l.StopAllBackgroundAgents(); err != nil {
		l.StoreLog("[ERROR] Failed to stop background agents: %v", err)
	}
}

// getOrInitializeLocals retrieves or initializes the locals
func (l *LiveServerWant) getOrInitializeLocals() *LiveServerLocals {
	if locals := l.GetLocals(); locals != nil {
		return locals
	}

	locals := &LiveServerLocals{
		Phase:         LiveServerPhaseStarting,
		LastCheckTime: time.Now(),
	}

	l.GetStateMulti(Dict{
		"server_phase": &locals.Phase,
		"server_pid":   &locals.ServerPID,
	})

	// Restore from params
	locals.Command = getStringParam(&l.Want, "command", "")
	locals.Args = getArgsParam(&l.Want)
	locals.LogFile = getStringParam(&l.Want, "log_file", "")
	locals.HealthCheckURL = getStringParam(&l.Want, "health_check_url", "")
	locals.HealthCheckInterval = getStringParam(&l.Want, "health_check_interval", "500ms")
	locals.HealthCheckMaxRetries = getIntParam(&l.Want, "health_check_max_retries", 15)

	return locals
}

// updateLocals updates the in-memory locals
func (l *LiveServerWant) updateLocals(locals *LiveServerLocals) {
	l.Locals = locals
}

// getStringParam gets a string parameter from want params with a default value
func getStringParam(want *Want, key, defaultVal string) string {
	if v, ok := want.Spec.Params[key]; ok {
		return fmt.Sprintf("%v", v)
	}
	return defaultVal
}

// getIntParam gets an int parameter from want params with a default value
func getIntParam(want *Want, key string, defaultVal int) int {
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
func getArgsParam(want *Want) []string {
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
