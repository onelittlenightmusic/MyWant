package types

import (
	"context"
	"log"

	. "mywant/engine/core"
)

const capabilityManagerAgentName = "capability_manager_agent"

func init() {
	RegisterWantImplementation[CapabilityManagerWant, CapabilityManagerLocals]("capability_manager")
	RegisterThinkAgentType(capabilityManagerAgentName, []Capability{
		{Name: "capability_management", Gives: []string{"capability_management"},
			Description: "Evaluates achievement rules and dynamically awards achievements and capabilities"},
	}, capabilityManagerThink)

	// Hook: when an achievement with UnlocksCapability is added, register it in AgentRegistry.
	RegisterOnAchievementAdded(func(a Achievement) {
		if a.UnlocksCapability == "" {
			return
		}
		cb := GetGlobalChainBuilder()
		if cb == nil {
			return
		}
		// Get registry from any want (they all share the same registry instance)
		wants := cb.GetWants()
		if len(wants) == 0 {
			return
		}
		registry := wants[0].GetAgentRegistry()
		if registry == nil {
			return
		}
		// Register the new capability and link it to the agent that earned it.
		registry.LinkCapabilityToAgent(a.AgentName, a.UnlocksCapability)
		log.Printf("[CapabilityManager] Linked capability '%s' to agent '%s'",
			a.UnlocksCapability, a.AgentName)

		// Store in global state so OPA / itinerary can read it
		existing, _ := GetGlobalState("available_capabilities")
		var list []string
		if sl, ok := existing.([]any); ok {
			for _, v := range sl {
				if s, ok := v.(string); ok {
					list = append(list, s)
				}
			}
		} else if sl, ok := existing.([]string); ok {
			list = sl
		}
		// Deduplicate
		found := false
		for _, c := range list {
			if c == a.UnlocksCapability {
				found = true
				break
			}
		}
		if !found {
			list = append(list, a.UnlocksCapability)
			StoreGlobalState("available_capabilities", list)
			log.Printf("[CapabilityManager] Updated available_capabilities: %v", list)
		}
	})
}

// ── Want type ─────────────────────────────────────────────────────────────────

type CapabilityManagerLocals struct{}

type CapabilityManagerWant struct {
	Want
}

func (w *CapabilityManagerWant) GetLocals() *CapabilityManagerLocals {
	return CheckLocalsInitialized[CapabilityManagerLocals](&w.Want)
}

func (w *CapabilityManagerWant) Initialize() {}

// IsAchieved never returns true — this want runs indefinitely.
func (w *CapabilityManagerWant) IsAchieved() bool { return false }

// Progress is a no-op; all logic is in the ThinkAgent.
func (w *CapabilityManagerWant) Progress() {}

// ── ThinkAgent logic ──────────────────────────────────────────────────────────

// capabilityManagerThink scans all achieved wants, evaluates achievement rules,
// and awards achievements + registers unlocked capabilities as needed.
func capabilityManagerThink(ctx context.Context, want *Want) error {
	cb := GetGlobalChainBuilder()
	if cb == nil {
		return nil
	}
	registry := want.GetAgentRegistry()
	if registry == nil {
		return nil
	}

	rules := ListAchievementRules()
	if len(rules) == 0 {
		return nil
	}

	// Tally completed wants per agent per capability.
	// completions[agentName][capabilityName] = count of achieved wants
	completions := make(map[string]map[string]int)
	// Also track want details for the achievement record
	type wantRecord struct {
		id       string
		name     string
		wantType string
	}
	lastWant := make(map[string]map[string]wantRecord)

	for _, w := range cb.GetWants() {
		if w.Status != WantStatusAchieved {
			continue
		}
		hist := w.BuildHistory()
		for _, exec := range hist.AgentHistory {
			if exec.Status != "achieved" {
				continue
			}
			agentName := exec.AgentName
			agent, ok := registry.GetAgent(agentName)
			if !ok {
				continue
			}
			for _, capName := range agent.GetCapabilities() {
				if completions[agentName] == nil {
					completions[agentName] = make(map[string]int)
					lastWant[agentName] = make(map[string]wantRecord)
				}
				completions[agentName][capName]++
				lastWant[agentName][capName] = wantRecord{
					id:       w.Metadata.ID,
					name:     w.Metadata.Name,
					wantType: w.Metadata.Type,
				}
			}
		}
	}

	// Evaluate each active rule
	for _, rule := range rules {
		if !rule.Active {
			continue
		}
		cond := rule.Condition

		for agentName, capCounts := range completions {
			// Check capability filter
			if cond.AgentCapability != "" {
				count := capCounts[cond.AgentCapability]
				if count < cond.CompletedCount {
					continue
				}
				rec := lastWant[agentName][cond.AgentCapability]
				// Check WantType filter
				if cond.WantType != "" && rec.wantType != cond.WantType {
					continue
				}
				// Already awarded?
				if HasAchievement(agentName, rule.Award.Title) {
					continue
				}
				// Award it
				a := Achievement{
					Title:             rule.Award.Title,
					Description:       rule.Award.Description,
					AgentName:         agentName,
					WantID:            rec.id,
					WantName:          rec.name,
					Category:          rule.Award.Category,
					Level:             rule.Award.Level,
					AwardedBy:         "capability_manager",
					UnlocksCapability: rule.Award.UnlocksCapability,
				}
				awarded := AddAchievement(a)
				want.DirectLog("[CapabilityManager] Awarded '%s' to agent '%s' (rule: %s)",
					rule.Award.Title, agentName, rule.ID)
				_ = awarded
			} else {
				// No capability filter — count total completions across all capabilities
				total := 0
				for _, c := range capCounts {
					total += c
				}
				if total < cond.CompletedCount {
					continue
				}
				if HasAchievement(agentName, rule.Award.Title) {
					continue
				}
				// Pick any want record for the reference
				var rec wantRecord
				for _, r := range lastWant[agentName] {
					rec = r
					break
				}
				a := Achievement{
					Title:             rule.Award.Title,
					Description:       rule.Award.Description,
					AgentName:         agentName,
					WantID:            rec.id,
					WantName:          rec.name,
					Category:          rule.Award.Category,
					Level:             rule.Award.Level,
					AwardedBy:         "capability_manager",
					UnlocksCapability: rule.Award.UnlocksCapability,
				}
				awarded := AddAchievement(a)
				want.DirectLog("[CapabilityManager] Awarded '%s' to agent '%s' (rule: %s)",
					rule.Award.Title, agentName, rule.ID)
				_ = awarded
			}
		}
	}
	return nil
}
