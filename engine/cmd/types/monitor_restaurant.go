package types

import (
	"context"
	"fmt"
	"gopkg.in/yaml.v3"
	"log"
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
				Uses:         uses,
				Type:         MonitorAgentType,
			},
		},
		ExecutionCount: 0,
		TestDataPath:   "/Users/hiroyukiosaki/work/MyWant/test/initial",
	}
}

// Exec executes restaurant monitoring by reading state from YAML files
func (m *MonitorRestaurant) Exec(ctx context.Context, want *Want) error {
	log.Printf("[MONITOR_RESTAURANT] Starting monitoring for %s (execution #%d)\n", want.Metadata.Name, m.ExecutionCount)

	// Determine which YAML file to read based on execution count
	filename := fmt.Sprintf("restaurant%d.yaml", m.ExecutionCount)
	filepath := filepath.Join(m.TestDataPath, filename)

	log.Printf("[MONITOR_RESTAURANT] Reading state from %s\n", filepath)

	// Read the YAML file
	data, err := os.ReadFile(filepath)
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("[MONITOR_RESTAURANT] No initial state file found at %s, proceeding without existing schedule.\n", filepath)
		} else {
			log.Printf("[MONITOR_RESTAURANT] Error reading %s: %v\n", filepath, err)
			return err
		}
	}

	// Parse the YAML content
	var restaurantState RestaurantState
	if err := yaml.Unmarshal(data, &restaurantState); err != nil {
		log.Printf("[MONITOR_RESTAURANT] Error parsing YAML: %v\n", err)
		return err
	}

	// Debug: Print parsed state
	log.Printf("[MONITOR_RESTAURANT] DEBUG: Parsed state: %+v\n", restaurantState)
	log.Printf("[MONITOR_RESTAURANT] DEBUG: Start time: %v, IsZero: %v\n", restaurantState.State.Start, restaurantState.State.Start.IsZero())

	// Store the state information using StoreState
	if !restaurantState.State.Start.IsZero() {
		// Convert to RestaurantSchedule format
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

		log.Printf("[MONITOR_RESTAURANT] DEBUG: About to store agent_result: %+v\n", schedule)
		want.StoreStateMulti(map[string]interface{}{
			"agent_result":            schedule,
			"monitor_execution_count": m.ExecutionCount,
			"monitor_source_file":     filename,
		})
		log.Printf("[MONITOR_RESTAURANT] DEBUG: Stored agent_result successfully\n")

		log.Printf("[MONITOR_RESTAURANT] Loaded existing schedule: %s from %s to %s\n",
			restaurantState.State.Name,
			restaurantState.State.Start.Format("15:04 Jan 2"),
			restaurantState.State.End.Format("15:04 Jan 2"))
	} else {
		// No existing schedule found - store explicit first record
		want.StoreStateMulti(map[string]interface{}{
			"agent_result":            nil,
			"monitor_execution_count": m.ExecutionCount,
			"monitor_source_file":     filename,
		})
		log.Printf("[MONITOR_RESTAURANT] No existing schedule found in %s - stored first record\n", filename)
	}

	// Increment execution count for next call
	m.ExecutionCount++

	return nil
}

// HasExistingSchedule checks if the monitoring found an existing schedule
func (m *MonitorRestaurant) HasExistingSchedule(want *Want) bool {
	result, exists := want.GetState("agent_result")
	return exists && result != nil
}
