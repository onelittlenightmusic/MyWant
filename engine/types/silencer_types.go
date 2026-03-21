package types

import (
	. "mywant/engine/core"
)

func init() {
	RegisterWantImplementation[SilencerWant, SilencerLocals]("silencer")
}

// SilencerWant represents a want that automatically processes reaction requests
type SilencerWant struct {
	Want
}

func (s *SilencerWant) GetLocals() *SilencerLocals {
	return CheckLocalsInitialized[SilencerLocals](&s.Want)
}

// SilencerLocals holds type-specific local state for SilencerWant
type SilencerLocals struct {
	Policy string
}

// Initialize prepares the silencer want for execution
func (s *SilencerWant) Initialize() {
	s.StoreLog("[SILENCER] Initializing silencer: %s\n", s.Metadata.Name)

	// Get locals (guaranteed to be initialized by framework)
	locals := s.GetLocals()
	locals.Policy = s.GetStringParam("policy", "all_true")

	// Initialize state
	s.SetCurrent("processed_count", 0)
	s.SetCurrent("last_processed_id", "")
	s.SetCurrent("silencer_phase", "active")
}

// IsAchieved - Silencers are processors, they stay active to process stream
// until they receive a completion signal
func (s *SilencerWant) IsAchieved() bool {
	return GetCurrent(s, "silencer_phase", "active") == "completed"
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
	s.SetCurrent("achieving_percentage", s.CalculateAchievingPercentage())

	// Try to get a packet from input channels
	// Use blocking wait (infinite) for stream processing
	// Processor wants should continuously wait for packets from stream
	_, obj, done, ok := s.UseForeverTyped("reaction_request")
	if !ok {
		return
	}

	if done {
		s.StoreLog("📦 Silencer received DONE signal")
		s.SetCurrent("silencer_phase", "completed")
		s.SetCurrent("achieving_percentage", 100)
		return
	}

	// Process the received packet
	s.processPacket(obj)
}

// processPacket handles the reaction request data
func (s *SilencerWant) processPacket(obj *DataObject) {
	if obj == nil {
		s.StoreLog("[SILENCER] ERROR: nil packet received")
		return
	}

	reactionType := GetTyped(obj, "reaction_type", "internal")

	reactionID := GetTyped(obj, "reaction_id", "")
	if reactionID == "" {
		s.StoreLog("[SILENCER] ERROR: Missing or invalid reaction_id in packet")
		return
	}

	locals := s.GetLocals()
	if reactionType == "internal" {
		// Store target reaction ID directly to current state so DoAgent can read it synchronously
		s.SetCurrent("target_reaction_id", reactionID)

		// If policy is all_true, trigger DoAgent for auto-approval
		if locals.Policy == "all_true" {
			if err := s.ExecuteAgents(); err != nil {
				s.StoreLog("[SILENCER] ERROR: Failed to execute auto-approval agent: %v", err)
			} else {
				// Update state after successful execution
				count := GetCurrent(s, "processed_count", 0)
				s.SetCurrent("processed_count", count+1)
				s.SetCurrent("last_processed_id", reactionID)
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
