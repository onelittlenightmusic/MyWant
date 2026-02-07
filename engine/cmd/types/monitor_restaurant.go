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

// MonitorRestaurant extends MonitorAgent to read restaurant state from YAML files
type MonitorRestaurant struct {
	MonitorAgent
	ExecutionCount int
	TestDataPath   string
}

// RestaurantState represents the state structure in YAML files
type RestaurantState struct {
	State RestaurantStateData `yaml:"state,omitempty"`
}

type RestaurantStateData struct {
	Start time.Time `yaml:"start,omitempty"`
	End   time.Time `yaml:"end,omitempty"`
	Type  string    `yaml:"type,omitempty"`
	Name  string    `yaml:"name,omitempty"`
}

// NewMonitorRestaurant creates a new restaurant monitor agent
func NewMonitorRestaurant(name string, capabilities []string, uses []string) *MonitorRestaurant {
	return &MonitorRestaurant{
		MonitorAgent: MonitorAgent{
			BaseAgent: BaseAgent{
				Name:         name,
				Capabilities: capabilities,
				Type:         MonitorAgentType,
			},
		},
		ExecutionCount: 0,
		TestDataPath:   "/Users/hiroyukiosaki/work/MyWant/test/initial",
	}
}

// Exec executes restaurant monitoring by reading state from YAML files
func (m *MonitorRestaurant) Exec(ctx context.Context, want *Want) (bool, error) {
	want.StoreLog("Starting monitoring for %s (execution #%d)", want.Metadata.Name, m.ExecutionCount)

	// Determine which YAML file to read based on execution count
	filename := fmt.Sprintf("restaurant%d.yaml", m.ExecutionCount)
	filepath := filepath.Join(m.TestDataPath, filename)

	want.StoreLog("Reading state from %s", filepath)

	// Read the YAML file
	data, err := os.ReadFile(filepath)
	if err != nil {
		if os.IsNotExist(err) {
			want.StoreLog("No initial state file found at %s, proceeding without existing schedule", filepath)
		} else {
			want.StoreLog("Error reading %s: %v", filepath, err)
			return false, err
		}
	}
	var restaurantState RestaurantState
	if err := yaml.Unmarshal(data, &restaurantState); err != nil {
		want.StoreLog("Error parsing YAML: %v", err)
		return false, err
	}

	// Debug: Print parsed state
	want.StoreLog("DEBUG: Parsed state: %+v", restaurantState)
	want.StoreLog("DEBUG: Start time: %v, IsZero: %v", restaurantState.State.Start, restaurantState.State.Start.IsZero())
	if !restaurantState.State.Start.IsZero() {
		duration := restaurantState.State.End.Sub(restaurantState.State.Start)

		schedule := RestaurantSchedule{
			ReservationTime:  restaurantState.State.Start,
			DurationHours:    duration.Hours(),
			RestaurantType:   "fine dining", // default from YAML data
			ReservationName:  restaurantState.State.Name,
			PremiumLevel:     "standard",
			ServiceTier:      "monitor",
			PremiumAmenities: []string{"monitoring_data"},
		}

		want.StoreLog("DEBUG: About to store agent_result: %+v", schedule)
		want.StoreStateMulti(map[string]any{
			"agent_result":            schedule,
			"monitor_execution_count": m.ExecutionCount,
			"monitor_source_file":     filename,
		})
		want.StoreLog("DEBUG: Stored agent_result successfully")

		want.StoreLog("Loaded existing schedule: %s from %s to %s",
			restaurantState.State.Name,
			restaurantState.State.Start.Format("15:04 Jan 2"),
			restaurantState.State.End.Format("15:04 Jan 2"))
	} else {
		// No existing schedule found - store explicit first record
		want.StoreStateMulti(map[string]any{
			"agent_result":            nil,
			"monitor_execution_count": m.ExecutionCount,
			"monitor_source_file":     filename,
		})
		want.StoreLog("No existing schedule found in %s - stored first record", filename)
	}
	m.ExecutionCount++

	return false, nil // Continue monitoring
}

// HasExistingSchedule checks if the monitoring found an existing schedule
func (m *MonitorRestaurant) HasExistingSchedule(want *Want) bool {
	result, exists := want.GetState("agent_result")
	return exists && result != nil
}
