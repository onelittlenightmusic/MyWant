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

	// Hook: when an achievement is unlocked, link it into AgentRegistry.
	// available_capabilities is now computed on-demand from the achievement store,
	// so no global state update is needed here.
	RegisterOnAchievementAdded(func(a Achievement) {
		if a.UnlocksCapability == "" {
			return
		}
		cb := GetGlobalChainBuilder()
		if cb == nil {
			return
		}
		wants := cb.GetWants()
		if len(wants) == 0 {
			return
		}
		registry := wants[0].GetAgentRegistry()
		if registry == nil {
			return
		}
		registry.LinkCapabilityToAgent(a.AgentName, a.UnlocksCapability)
		log.Printf("[CapabilityManager] Linked capability '%s' to agent '%s'",
			a.UnlocksCapability, a.AgentName)
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

	// Evaluate each active rule: unlock a matching locked achievement when conditions are met.
	// Rules do NOT create achievements — they only unlock ones that already exist (locked).
	for _, rule := range rules {
		if !rule.Active {
			continue
		}
		cond := rule.Condition

		for agentName, capCounts := range completions {
			// Check capability filter
			conditionMet := false
			if cond.AgentCapability != "" {
				count := capCounts[cond.AgentCapability]
				if count < cond.CompletedCount {
					continue
				}
				rec := lastWant[agentName][cond.AgentCapability]
				if cond.WantType != "" && rec.wantType != cond.WantType {
					continue
				}
				conditionMet = true
			} else {
				total := 0
				for _, c := range capCounts {
					total += c
				}
				if total >= cond.CompletedCount {
					conditionMet = true
				}
			}
			if !conditionMet {
				continue
			}

			// Find the existing locked achievement to unlock.
			a, ok := FindAchievementByAgentAndTitle(agentName, rule.Award.Title)
			if !ok {
				// No matching achievement exists — nothing to unlock.
				continue
			}
			if a.Unlocked {
				// Already unlocked — nothing to do.
				continue
			}
			if _, ok := UnlockAchievement(a.ID); ok {
				want.DirectLog("[CapabilityManager] Unlocked '%s' for agent '%s' (rule: %s)",
					rule.Award.Title, agentName, rule.ID)
			}
		}
	}
	return nil
}
