package types

import (
	"fmt"
	"log"
	"time"

	. "mywant/engine/src"
)

// MockServerPhase constants
const (
	MockServerPhaseStarting = "starting"
	MockServerPhaseRunning  = "running"
	MockServerPhaseStopping = "stopping"
	MockServerPhaseStopped  = "stopped"
	MockServerPhaseFailed   = "failed"
)

// MockServerLocals holds type-specific local state for FlightMockServerWant
type MockServerLocals struct {
	Phase         string
	ServerBinary  string
	LogFile       string
	ServerPID     int
	LastCheckTime time.Time
}

// FlightMockServerWant represents a want that manages the flight mock server lifecycle
type FlightMockServerWant struct {
	Want
}

func init() {
	RegisterWantImplementation[FlightMockServerWant, MockServerLocals]("flight_mock_server")
}

// Initialize prepares the mock server want for execution
func (m *FlightMockServerWant) Initialize() {
	log.Printf("[MOCK_SERVER] Initialize() called for %s\n", m.Metadata.Name)
	m.StoreLog("[MOCK_SERVER] Initializing flight mock server: %s", m.Metadata.Name)

	// Get or initialize locals
	locals, ok := m.Locals.(*MockServerLocals)
	if !ok {
		locals = &MockServerLocals{}
		m.Locals = locals
	}
	locals.Phase = MockServerPhaseStarting
	locals.LastCheckTime = time.Now()
	locals.ServerBinary = "./bin/flight-server"
	locals.LogFile = "logs/flight-server.log"
	locals.ServerPID = 0

	// Parse server_binary parameter (optional, default: ./bin/flight-server)
	if binPath, ok := m.Spec.Params["server_binary"]; ok {
		locals.ServerBinary = fmt.Sprintf("%v", binPath)
	}

	// Parse log_file parameter (optional, default: logs/flight-server.log)
	if logPath, ok := m.Spec.Params["log_file"]; ok {
		locals.LogFile = fmt.Sprintf("%v", logPath)
	}

	// Store initial state
	stateMap := map[string]any{
		"server_phase":  locals.Phase,
		"server_binary": locals.ServerBinary,
		"log_file":      locals.LogFile,
		"server_pid":    0,
	}

	m.StoreStateMulti(stateMap)
	m.Locals = locals

	// Set up agent requirements for mock server management
	m.Spec.Requires = []string{
		"mock_server_management", // DoAgent manages server lifecycle
	}

	m.StoreLog("[MOCK_SERVER] Initialized mock server '%s' with binary=%s, log=%s",
		m.Metadata.Name, locals.ServerBinary, locals.LogFile)

	// Note: Don't execute agents here - agentRegistry may not be set yet
	// Agent execution will happen in first Progress() call
}

// IsAchieved checks if the mock server is running
func (m *FlightMockServerWant) IsAchieved() bool {
	phase, _ := m.GetState("server_phase")
	return phase == MockServerPhaseRunning
}

// CalculateAchievingPercentage returns the progress percentage
func (m *FlightMockServerWant) CalculateAchievingPercentage() int {
	if m.IsAchieved() || m.Status == WantStatusAchieved {
		return 100
	}
	phase, _ := m.GetState("server_phase")
	switch phase {
	case MockServerPhaseStarting:
		return 50
	case MockServerPhaseRunning:
		return 100
	case MockServerPhaseStopping:
		return 75
	case MockServerPhaseStopped, MockServerPhaseFailed:
		return 0
	default:
		return 0
	}
}

// Progress implements Progressable for FlightMockServerWant
func (m *FlightMockServerWant) Progress() {
	locals := m.getOrInitializeLocals()

	// Update achieving percentage based on current phase
	m.StoreState("achieving_percentage", m.CalculateAchievingPercentage())

	switch locals.Phase {
	case MockServerPhaseStarting:
		// First time in starting phase - execute agents to start server
		if m.GetAgentRegistry() != nil {
			m.StoreLog("[MOCK_SERVER] Executing agents to start server")
			if err := m.ExecuteAgents(); err != nil {
				m.StoreLog("[ERROR] Failed to execute agents: %v", err)
				m.StoreState("server_phase", MockServerPhaseFailed)
				m.StoreState("error_message", fmt.Sprintf("Agent execution failed: %v", err))
				locals.Phase = MockServerPhaseFailed
				m.updateLocals(locals)
				m.Status = "failed"
				return
			}
		}

		// Check if we have a PID now (server started)
		// Server should transition to running after DoAgent execution
		// Check if we have a PID
		if pidValue, exists := m.GetState("server_pid"); exists && pidValue != nil {
			var pid int
			switch v := pidValue.(type) {
			case int:
				pid = v
			case float64:
				pid = int(v)
			}

			if pid > 0 {
				m.StoreLog("[MOCK_SERVER] Server is running with PID %d", pid)
				m.StoreState("server_phase", MockServerPhaseRunning)
				locals.Phase = MockServerPhaseRunning
				locals.ServerPID = pid
				m.updateLocals(locals)
				m.ProvideDone()
			}
		}

	case MockServerPhaseRunning:
		// Server is running, nothing to do
		// The want stays achieved until deleted
		break

	case MockServerPhaseStopping:
		// Server is stopping, check if PID is cleared
		if pidValue, exists := m.GetState("server_pid"); !exists || pidValue == nil || pidValue == 0 {
			m.StoreLog("[MOCK_SERVER] Server stopped successfully")
			m.StoreState("server_phase", MockServerPhaseStopped)
			locals.Phase = MockServerPhaseStopped
			m.updateLocals(locals)
		}

	case MockServerPhaseStopped, MockServerPhaseFailed:
		// Already stopped or failed, nothing to do
		break

	default:
		m.StoreLog("[ERROR] Unknown phase: %s", locals.Phase)
		m.StoreState("server_phase", MockServerPhaseFailed)
		locals.Phase = MockServerPhaseFailed
		m.Status = "failed"
		m.updateLocals(locals)
	}
}

// OnDelete is called when the want is being deleted
// This is where we stop the mock server process
func (m *FlightMockServerWant) OnDelete() {
	m.StoreLog("[MOCK_SERVER] Want is being deleted, stopping mock server")

	// Update phase to stopping
	m.StoreState("server_phase", MockServerPhaseStopping)

	// Execute DoAgent to stop the server
	if err := m.ExecuteAgents(); err != nil {
		m.StoreLog("[ERROR] Failed to stop mock server during deletion: %v", err)
	} else {
		m.StoreLog("[MOCK_SERVER] Mock server stopped successfully")
	}

	// Stop any background agents (if any)
	if err := m.StopAllBackgroundAgents(); err != nil {
		m.StoreLog("[ERROR] Failed to stop background agents: %v", err)
	}
}

// getOrInitializeLocals retrieves or initializes the locals
func (m *FlightMockServerWant) getOrInitializeLocals() *MockServerLocals {
	if m.Locals != nil {
		if locals, ok := m.Locals.(*MockServerLocals); ok {
			return locals
		}
	}

	// Initialize from state
	locals := &MockServerLocals{
		Phase:         MockServerPhaseStarting,
		LastCheckTime: time.Now(),
		ServerBinary:  "./bin/flight-server",
		LogFile:       "logs/flight-server.log",
	}

	if phase, exists := m.GetState("server_phase"); exists {
		if phaseStr, ok := phase.(string); ok {
			locals.Phase = phaseStr
		}
	}

	if binPath, exists := m.GetState("server_binary"); exists {
		locals.ServerBinary = fmt.Sprintf("%v", binPath)
	}

	if logPath, exists := m.GetState("log_file"); exists {
		locals.LogFile = fmt.Sprintf("%v", logPath)
	}

	if pidValue, exists := m.GetState("server_pid"); exists && pidValue != nil {
		switch v := pidValue.(type) {
		case int:
			locals.ServerPID = v
		case float64:
			locals.ServerPID = int(v)
		}
	}

	return locals
}

// updateLocals updates the in-memory locals
func (m *FlightMockServerWant) updateLocals(locals *MockServerLocals) {
	m.Locals = locals
}
