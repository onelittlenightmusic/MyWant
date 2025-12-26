package mywant

import (
	"fmt"
	"time"
)

// SchedulerWant is a System Want that manages all scheduled Wants in the system
// It continuously monitors all Wants and manages their SchedulerAgents
type SchedulerWant struct {
	Want
	builder *ChainBuilder // Reference to ChainBuilder for coordinating restarts
}

// NewSchedulerWant creates a new Scheduler Want
func NewSchedulerWant() *SchedulerWant {
	return &SchedulerWant{}
}

// SetChainBuilder sets the ChainBuilder reference for this scheduler
func (s *SchedulerWant) SetChainBuilder(builder *ChainBuilder) {
	s.builder = builder
}

// Progress is called during the want execution cycle
// It scans all Wants and manages their SchedulerAgents
func (s *SchedulerWant) Progress() {
	if s.builder == nil {
		return // ChainBuilder not yet set
	}

	// Get all current Want states
	allWants := s.builder.GetAllWantStates()

	// Get the list of registered agent IDs from state
	registeredAgentsAny, exists := s.GetState("registered_agents")
	registeredAgents := make(map[string]bool)
	if exists {
		if agentList, ok := registeredAgentsAny.([]interface{}); ok {
			for _, agent := range agentList {
				if agentID, ok := agent.(string); ok {
					registeredAgents[agentID] = true
				}
			}
		}
	}

	// Track which agents are currently registered
	currentAgents := make([]string, 0)

	// Scan each Want for scheduling requirements
	for _, want := range allWants {
		if len(want.Spec.When) == 0 {
			continue // No schedule specified for this Want
		}

		// Skip the scheduler itself to avoid recursion
		if want.Metadata.Type == "scheduler" {
			continue
		}

		agentID := fmt.Sprintf("scheduler-%s", want.Metadata.Name)
		currentAgents = append(currentAgents, agentID)

		// Skip if agent already exists for this Want
		if registeredAgents[agentID] {
			continue
		}

		// Create a new SchedulerAgent for this Want
		agent, err := NewSchedulerAgent(
			want.Spec.When,
			want.Metadata.ID,
			want.Metadata.Name,
			s.builder,
		)
		if err != nil {
			InfoLog("[SCHEDULER_WANT] Failed to create agent for '%s': %v\n",
				want.Metadata.Name, err)
			continue
		}

		// Add the agent as a BackgroundAgent to this Scheduler Want
		if err := s.AddBackgroundAgent(agent); err != nil {
			InfoLog("[SCHEDULER_WANT] Failed to add agent for '%s': %v\n",
				want.Metadata.Name, err)
			continue
		}

		registeredAgents[agentID] = true
		InfoLog("[SCHEDULER_WANT] Added SchedulerAgent for Want '%s'\n",
			want.Metadata.Name)
	}

	// Update the list of registered agents
	s.StoreState("registered_agents", currentAgents)

	// Update scheduler statistics in state
	s.StoreState("total_scheduled_wants", s.GetBackgroundAgentCount())
	s.StoreState("last_scan_time", time.Now().Unix())
}

// IsAchieved always returns false since the scheduler runs continuously
func (s *SchedulerWant) IsAchieved() bool {
	return false
}

// RegisterSchedulerWantTypes registers the scheduler want type with the ChainBuilder
func RegisterSchedulerWantTypes(builder *ChainBuilder) {
	builder.RegisterWantType("scheduler", func(m Metadata, spec WantSpec) Progressable {
		schedulerWant := NewSchedulerWant()
		schedulerWant.Want = Want{Metadata: m, Spec: spec}
		schedulerWant.SetChainBuilder(builder)
		return schedulerWant
	})
}
