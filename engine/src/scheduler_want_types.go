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

	// Create a snapshot of wants while holding the lock to avoid concurrent map access
	s.builder.reconcileMutex.RLock()
	allWantsSnapshot := make(map[string]*runtimeWant)
	for name, rw := range s.builder.wants {
		allWantsSnapshot[name] = rw
	}
	s.builder.reconcileMutex.RUnlock()

	// Scan each Want for scheduling requirements
	for name, rw := range allWantsSnapshot {
		want := rw.want
		// TARGET check: Target wants always need a progression loop even without a schedule
		isTarget := want.Metadata.Type == "target" || want.Metadata.Type == "custom_target"
		
		if isTarget {
			// For targets, we just need to ensure they are started
			// The builder will handle starting the progression loop if not already active
			s.builder.startWant(name, rw)
			continue // No need to create a scheduler agent for targets
		}

		if len(want.Spec.When) == 0 {
			continue // No schedule specified for this Want
		}

		// Skip the scheduler itself to avoid recursion
		if want.Metadata.Type == "scheduler" {
			continue
		}

		agentID := fmt.Sprintf("scheduler-%s", want.Metadata.Name)

		// Check if agent already exists for this Want
		if existingAgent, exists := s.GetBackgroundAgent(agentID); exists {
			// Verify if the want ID has changed
			if schedAgent, ok := existingAgent.(*SchedulerAgent); ok {
				if schedAgent.wantID == want.Metadata.ID {
					continue // ID matches, no update needed
				}
				// ID mismatch! This happens when a Want is redeployed.
				// Stop and remove the old agent before creating a new one.
				InfoLog("[SCHEDULER_WANT] Want ID changed for '%s' (%s -> %s), updating agent\n",
					want.Metadata.Name, schedAgent.wantID, want.Metadata.ID)
				s.DeleteBackgroundAgent(agentID)
			} else {
				continue // Should not happen, but safe fallback
			}
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

		InfoLog("[SCHEDULER_WANT] Added SchedulerAgent for Want '%s'\n",
			want.Metadata.Name)
	}

	// Update scheduler statistics in state
	s.StoreState("total_scheduled_wants", s.GetBackgroundAgentCount())
	s.StoreState("last_scan_time", time.Now().Unix())
}

// IsAchieved always returns false since the scheduler runs continuously
// Initialize resets state before execution begins
func (s *SchedulerWant) Initialize() {
	// No state reset needed for scheduler wants
}

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
