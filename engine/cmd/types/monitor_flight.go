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
	want.StoreLog(fmt.Sprintf("Starting monitoring for %s (execution #%d)", want.Metadata.Name, m.ExecutionCount))

	// Determine which YAML file to read based on execution count
	filename := fmt.Sprintf("flight%d.yaml", m.ExecutionCount)
	filepath := filepath.Join(m.TestDataPath, filename)

	want.StoreLog(fmt.Sprintf("Reading state from %s", filepath))

	// Read the YAML file
	data, err := os.ReadFile(filepath)
	if err != nil {
		want.StoreLog(fmt.Sprintf("Error reading %s: %v", filepath, err))
		return err
	}
	var flightState FlightState
	if err := yaml.Unmarshal(data, &flightState); err != nil {
		want.StoreLog(fmt.Sprintf("Error parsing YAML: %v", err))
		return err
	}

	// Debug: Print parsed state
	want.StoreLog(fmt.Sprintf("DEBUG: Parsed state: %+v", flightState))
	want.StoreLog(fmt.Sprintf("DEBUG: Departure time: %v, IsZero: %v",
		flightState.State.DepartureTime, flightState.State.DepartureTime.IsZero()))
	if !flightState.State.DepartureTime.IsZero() {
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

		want.StoreLog(fmt.Sprintf("DEBUG: About to store agent_result: %+v", schedule))
		want.StoreStateMulti(map[string]interface{}{
			"agent_result":            schedule,
			"monitor_execution_count": m.ExecutionCount,
			"monitor_source_file":     filename,
		})
		want.StoreLog("DEBUG: Stored agent_result successfully")

		want.StoreLog(fmt.Sprintf("Loaded existing schedule: %s from %s to %s",
			flightState.State.Name,
			flightState.State.DepartureTime.Format("15:04 Jan 2"),
			flightState.State.ArrivalTime.Format("15:04 Jan 2")))
	} else {
		// No existing schedule found - store explicit first record
		want.StoreStateMulti(map[string]interface{}{
			"agent_result":            nil,
			"monitor_execution_count": m.ExecutionCount,
			"monitor_source_file":     filename,
		})
		want.StoreLog(fmt.Sprintf("No existing schedule found in %s - stored first record", filename))
	}
	m.ExecutionCount++

	return nil
}

// HasExistingSchedule checks if the monitoring found an existing schedule
func (m *MonitorFlight) HasExistingSchedule(want *Want) bool {
	result, exists := want.GetState("agent_result")
	return exists && result != nil
}
