package types

import (
	"encoding/json"
	. "mywant/engine/src"
)

func init() {
	RegisterWantImplementation[SilencerWant, SilencerLocals]("silencer")
}

// SilencerWant represents a want that automatically processes reaction requests
type SilencerWant struct {
	Want
}

func (s *SilencerWant) GetLocals() *SilencerLocals {
	return GetLocals[SilencerLocals](&s.Want)
}

// SilencerLocals holds type-specific local state for SilencerWant
type SilencerLocals struct {
	Policy string
}

// Initialize prepares the silencer want for execution
func (s *SilencerWant) Initialize() {
	s.StoreLog("[SILENCER] Initializing silencer: %s\n", s.Metadata.Name)

	// Get or initialize locals
	locals := s.GetLocals()
	if locals == nil {
		locals = &SilencerLocals{}
		s.Locals = locals
	}
	locals.Policy = s.GetStringParam("policy", "all_true")

	// Initialize state
	s.StoreStateMulti(map[string]any{
		"processed_count":   0,
		"last_processed_id": "",
		"silencer_phase":    "active",
	})
}

// IsAchieved - Silencers are processors, they stay active to process stream
// until they receive a completion signal
func (s *SilencerWant) IsAchieved() bool {
	phase, _ := s.GetStateString("silencer_phase", "active")
	return phase == "completed"
}

// CalculateAchievingPercentage returns the progress percentage
func (s *SilencerWant) CalculateAchievingPercentage() int {
	if s.IsAchieved() || s.Status == WantStatusAchieved || s.Status == WantStatusFailed {
		return 100
	}
	return 50 // In-progress processors stay at 50%
}

// Progress implements Progressable for SilencerWant
func (s *SilencerWant) Progress() {
	// Update achieving percentage
	s.StoreState("achieving_percentage", s.CalculateAchievingPercentage())

	// Try to get a packet from input channels
	// Use blocking wait (infinite) for stream processing
	// Processor wants should continuously wait for packets from stream
	_, data, done, ok := s.UseForever()
	if !ok {
		return
	}

	if done {
		s.StoreLog("📦 Silencer received DONE signal")
		s.StoreStateMulti(map[string]any{
			"silencer_phase":       "completed",
			"achieving_percentage": 100,
		})
		return
	}

	// Process the received packet
	s.processPacket(data)
}

// processPacket handles the reaction request data
func (s *SilencerWant) processPacket(data any) {
	packet, ok := data.(map[string]any)
	if !ok {
		// Try to decode if it's a JSON string
		if str, ok := data.(string); ok {
			if err := json.Unmarshal([]byte(str), &packet); err != nil {
				s.StoreLog("[SILENCER] ERROR: Failed to parse packet string: %v", err)
				return
			}
		} else {
			s.StoreLog("[SILENCER] ERROR: Invalid packet format: %T", data)
			return
		}
	}

	reactionType := "internal"
	if rt, ok := packet["reaction_type"].(string); ok {
		reactionType = rt
	}

	reactionID, ok := packet["reaction_id"].(string)
	if !ok || reactionID == "" {
		s.StoreLog("[SILENCER] ERROR: Missing or invalid reaction_id in packet")
		return
	}

	locals := s.GetLocals()
	if reactionType == "internal" {
		// Store target reaction ID for agents - use SetStateAtomic for immediate visibility
		s.SetStateAtomic(map[string]any{"_target_reaction_id": reactionID})

		// If policy is all_true, trigger DoAgent for auto-approval
		if locals.Policy == "all_true" {
			if err := s.ExecuteAgents(); err != nil {
				s.StoreLog("[SILENCER] ERROR: Failed to execute auto-approval agent: %v", err)
			} else {
				// Update state after successful execution
				count, _ := s.GetStateInt("processed_count", 0)
				s.StoreStateMulti(map[string]any{
					"processed_count":   count + 1,
					"last_processed_id": reactionID,
				})
				s.StoreLog("📦 Silencer auto-approved reaction: %s", reactionID)
			}
		} else {
			// Otherwise maybe use monitor?
			// For now just log
			s.StoreLog("[SILENCER] Policy '%s' not implemented for internal reaction type", locals.Policy)
		}
	} else {
		s.StoreLog("[SILENCER] Reaction type '%s' not supported", reactionType)
	}
}
