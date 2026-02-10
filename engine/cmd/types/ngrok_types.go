package types

import (
	"fmt"
	"time"

	. "mywant/engine/src"
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
	Phase         string
	Port          string
	Protocol      string
	PID           int
	PublicURL     string
	LastCheckTime time.Time
}

// NgrokWant represents a want that manages an ngrok tunnel lifecycle
type NgrokWant struct {
	Want
}

func (n *NgrokWant) GetLocals() *NgrokLocals {
	return GetLocals[NgrokLocals](&n.Want)
}

// Initialize prepares the ngrok want for execution
func (n *NgrokWant) Initialize() {
	n.StoreLog("[NGROK] Initializing ngrok tunnel: %s", n.Metadata.Name)

	locals := n.GetLocals()
	if locals == nil {
		locals = &NgrokLocals{}
		n.Locals = locals
	}
	locals.Phase = NgrokPhaseStarting
	locals.LastCheckTime = time.Now()
	locals.Port = "8080"
	locals.Protocol = "http"
	locals.PID = 0
	locals.PublicURL = ""

	// Parse port parameter
	if port, ok := n.Spec.Params["port"]; ok {
		locals.Port = fmt.Sprintf("%v", port)
	}

	// Parse protocol parameter
	if protocol, ok := n.Spec.Params["protocol"]; ok {
		locals.Protocol = fmt.Sprintf("%v", protocol)
	}

	// Store initial state
	stateMap := map[string]any{
		"ngrok_phase":      locals.Phase,
		"ngrok_port":       locals.Port,
		"ngrok_protocol":   locals.Protocol,
		"ngrok_pid":        0,
		"ngrok_public_url": "",
	}

	n.StoreStateMulti(stateMap)
	n.Locals = locals
}

// IsAchieved checks if the ngrok tunnel is running with a public URL
func (n *NgrokWant) IsAchieved() bool {
	phase, _ := n.GetStateString("ngrok_phase", "")
	url, _ := n.GetStateString("ngrok_public_url", "")
	return phase == NgrokPhaseRunning && url != ""
}

// CalculateAchievingPercentage returns the progress percentage
func (n *NgrokWant) CalculateAchievingPercentage() int {
	if n.IsAchieved() || n.Status == WantStatusAchieved {
		return 100
	}
	phase, _ := n.GetStateString("ngrok_phase", "")
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
		if n.GetAgentRegistry() != nil {
			if err := n.ExecuteAgents(); err != nil {
				n.StoreLog("[ERROR] Failed to execute agents: %v", err)
				n.StoreState("ngrok_phase", NgrokPhaseFailed)
				n.StoreState("error_message", fmt.Sprintf("Agent execution failed: %v", err))
				locals.Phase = NgrokPhaseFailed
				n.updateLocals(locals)
				n.Status = "failed"
				return
			}
		}

		// Check if we have a PID and public URL
		if pid, ok := n.GetStateInt("ngrok_pid", 0); ok && pid > 0 {
			url, _ := n.GetStateString("ngrok_public_url", "")
			if url != "" {
				n.StoreLog("[NGROK] Tunnel is running with PID %d, URL: %s", pid, url)
				n.StoreState("ngrok_phase", NgrokPhaseRunning)
				locals.Phase = NgrokPhaseRunning
				locals.PID = pid
				locals.PublicURL = url
				n.updateLocals(locals)
				n.ProvideDone()
			}
		}

	case NgrokPhaseRunning:
		// Tunnel is running, nothing to do
		break

	case NgrokPhaseStopping:
		if pid, _ := n.GetStateInt("ngrok_pid", 0); pid == 0 {
			n.StoreLog("[NGROK] Tunnel stopped successfully")
			n.StoreState("ngrok_phase", NgrokPhaseStopped)
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

// OnDelete stops the ngrok process when the want is deleted
func (n *NgrokWant) OnDelete() {
	n.StoreLog("[NGROK] Want is being deleted, stopping ngrok tunnel")

	n.StoreState("ngrok_phase", NgrokPhaseStopping)

	if err := n.ExecuteAgents(); err != nil {
		n.StoreLog("[ERROR] Failed to stop ngrok during deletion: %v", err)
	} else {
		n.StoreLog("[NGROK] Ngrok tunnel stopped successfully")
	}

	if err := n.StopAllBackgroundAgents(); err != nil {
		n.StoreLog("[ERROR] Failed to stop background agents: %v", err)
	}
}

// getOrInitializeLocals retrieves or initializes the locals
func (n *NgrokWant) getOrInitializeLocals() *NgrokLocals {
	if locals := n.GetLocals(); locals != nil {
		return locals
	}

	locals := &NgrokLocals{
		Phase:         NgrokPhaseStarting,
		Port:          "8080",
		Protocol:      "http",
		LastCheckTime: time.Now(),
	}

	n.GetStateMulti(Dict{
		"ngrok_phase":      &locals.Phase,
		"ngrok_port":       &locals.Port,
		"ngrok_protocol":   &locals.Protocol,
		"ngrok_pid":        &locals.PID,
		"ngrok_public_url": &locals.PublicURL,
	})

	return locals
}

// updateLocals updates the in-memory locals
func (n *NgrokWant) updateLocals(locals *NgrokLocals) {
	n.Locals = locals
}
