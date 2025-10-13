package types

import (
	"context"
	"fmt"
	"gopkg.in/yaml.v3"
	. "mywant/engine/src"
	"os"
	"path/filepath"
	"time"
)

// MonitorFlight extends MonitorAgent to read flight state from YAML files
type MonitorFlight struct {
	MonitorAgent
	ExecutionCount int
	TestDataPath   string
}

// FlightState represents the state structure in YAML files
type FlightState struct {
	State FlightStateData `yaml:"state,omitempty"`
}

type FlightStateData struct {
	DepartureTime time.Time `yaml:"departure_time,omitempty"`
	ArrivalTime   time.Time `yaml:"arrival_time,omitempty"`
	FlightType    string    `yaml:"flight_type,omitempty"`
	FlightNumber  string    `yaml:"flight_number,omitempty"`
	Name          string    `yaml:"name,omitempty"`
}

// NewMonitorFlight creates a new flight monitor agent
func NewMonitorFlight(name string, capabilities []string, uses []string) *MonitorFlight {
	return &MonitorFlight{
		MonitorAgent: MonitorAgent{
			BaseAgent: BaseAgent{
				Name:         name,
				Capabilities: capabilities,
				Uses:         uses,
				Type:         MonitorAgentType,
			},
		},
		ExecutionCount: 0,
		TestDataPath:   "/Users/hiroyukiosaki/work/MyWant/test/initial",
	}
}

// Exec executes flight monitoring by reading state from YAML files
func (m *MonitorFlight) Exec(ctx context.Context, want *Want) error {
	fmt.Printf("[MONITOR_FLIGHT] Starting monitoring for %s (execution #%d)\n", want.Metadata.Name, m.ExecutionCount)

	// Determine which YAML file to read based on execution count
	filename := fmt.Sprintf("flight%d.yaml", m.ExecutionCount)
	filepath := filepath.Join(m.TestDataPath, filename)

	fmt.Printf("[MONITOR_FLIGHT] Reading state from %s\n", filepath)

	// Read the YAML file
	data, err := os.ReadFile(filepath)
	if err != nil {
		fmt.Printf("[MONITOR_FLIGHT] Error reading %s: %v\n", filepath, err)
		return err
	}

	// Parse the YAML content
	var flightState FlightState
	if err := yaml.Unmarshal(data, &flightState); err != nil {
		fmt.Printf("[MONITOR_FLIGHT] Error parsing YAML: %v\n", err)
		return err
	}

	// Debug: Print parsed state
	fmt.Printf("[MONITOR_FLIGHT] DEBUG: Parsed state: %+v\n", flightState)
	fmt.Printf("[MONITOR_FLIGHT] DEBUG: Departure time: %v, IsZero: %v\n",
		flightState.State.DepartureTime, flightState.State.DepartureTime.IsZero())

	// Store the state information using StoreState
	if !flightState.State.DepartureTime.IsZero() {
		// Convert to FlightSchedule format
		schedule := FlightSchedule{
			DepartureTime:    flightState.State.DepartureTime,
			ArrivalTime:      flightState.State.ArrivalTime,
			FlightType:       flightState.State.FlightType,
			FlightNumber:     flightState.State.FlightNumber,
			ReservationName:  flightState.State.Name,
			PremiumLevel:     "standard",
			ServiceTier:      "monitor",
			PremiumAmenities: []string{"monitoring_data"},
		}

		fmt.Printf("[MONITOR_FLIGHT] DEBUG: About to store agent_result: %+v\n", schedule)
		want.StoreState("agent_result", schedule)
		want.StoreState("monitor_execution_count", m.ExecutionCount)
		want.StoreState("monitor_source_file", filename)
		fmt.Printf("[MONITOR_FLIGHT] DEBUG: Stored agent_result successfully\n")

		fmt.Printf("[MONITOR_FLIGHT] Loaded existing schedule: %s from %s to %s\n",
			flightState.State.Name,
			flightState.State.DepartureTime.Format("15:04 Jan 2"),
			flightState.State.ArrivalTime.Format("15:04 Jan 2"))
	} else {
		// No existing schedule found - store explicit first record
		want.StoreState("agent_result", nil)
		want.StoreState("monitor_execution_count", m.ExecutionCount)
		want.StoreState("monitor_source_file", filename)
		fmt.Printf("[MONITOR_FLIGHT] No existing schedule found in %s - stored first record\n", filename)
	}

	// Increment execution count for next call
	m.ExecutionCount++

	return nil
}

// HasExistingSchedule checks if the monitoring found an existing schedule
func (m *MonitorFlight) HasExistingSchedule(want *Want) bool {
	result, exists := want.GetState("agent_result")
	return exists && result != nil
}
