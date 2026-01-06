package types

import (
	"encoding/json"
	. "mywant/engine/src"
)

// SilencerWant represents a want that automatically processes reaction requests
type SilencerWant struct {
	Want
}

// SilencerLocals holds type-specific local state for SilencerWant
type SilencerLocals struct {
	Policy string
}

// NewSilencerWant creates a new SilencerWant
func NewSilencerWant(want *Want) *SilencerWant {
	return &SilencerWant{Want: *want}
}

// Initialize prepares the silencer want for execution
func (s *SilencerWant) Initialize() {
	s.StoreLog("[SILENCER] Initializing silencer: %s\n", s.Metadata.Name)

	// Initialize locals
	locals := &SilencerLocals{
		Policy: s.GetStringParam("policy", "all_true"),
	}
	s.Locals = locals

	// Initialize state
	s.StoreStateMulti(map[string]any{
		"processed_count":   0,
		"last_processed_id": "",
		"silencer_phase":    "active",
	})

	// Register required capability for processing reactions
	s.Spec.Requires = []string{"reaction_auto_approval"}
}

// IsAchieved - Silencers are processors, they stay active to process stream
// until they receive a completion signal
func (s *SilencerWant) IsAchieved() bool {
	phase, _ := s.GetState("silencer_phase")
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
	// Non-blocking check for packets
	index, data, done, ok := s.Use(0)
	if !ok {
		return
	}

	if done {
		s.StoreLog("[SILENCER] Received DONE signal. Finalizing...")
		InfoLog("[SILENCER:%s] Received DONE signal. Finalizing...\n", s.Metadata.Name)
		s.StoreStateMulti(map[string]any{
			"silencer_phase":       "completed",
			"achieving_percentage": 100,
		})
		return
	}

	s.StoreLog("[SILENCER] Received packet from channel %d: %v", index, data)
	InfoLog("[SILENCER:%s] Received packet from channel %d: %v", s.Metadata.Name, index, data)

	// Process the received packet
	s.processPacket(data)
}

// processPacket handles the reaction request data
func (s *SilencerWant) processPacket(data any) {
	s.StoreLog("[SILENCER] Received packet data type: %T", data)
	packet, ok := data.(map[string]any)
	if !ok {
		// Try to decode if it's a JSON string
		if str, ok := data.(string); ok {
			s.StoreLog("[SILENCER] Decoding JSON string packet")
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

	s.StoreLog("[SILENCER] Processing reaction %s (type: %s)", reactionID, reactionType)

	locals := s.Locals.(*SilencerLocals)
	if reactionType == "internal" {
		// Store target reaction ID for agents - use SetStateAtomic for immediate visibility
		s.SetStateAtomic(map[string]any{"_target_reaction_id": reactionID})
		
		// If policy is all_true, trigger DoAgent for auto-approval
		if locals.Policy == "all_true" {
			s.Spec.Requires = []string{"reaction_auto_approval"}
			if err := s.ExecuteAgents(); err != nil {
				s.StoreLog("[SILENCER] ERROR: Failed to execute auto-approval agent: %v", err)
			} else {
				// Update state after successful execution
				count, _ := s.GetStateInt("processed_count", 0)
				s.StoreStateMulti(map[string]any{
					"processed_count":   count + 1,
					"last_processed_id": reactionID,
				})
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

// RegisterSilencerWantType registers the SilencerWant type with the ChainBuilder
func RegisterSilencerWantType(builder *ChainBuilder) {
	builder.RegisterWantType("silencer", func(metadata Metadata, spec WantSpec) Progressable {
		want := &Want{
			Metadata: metadata,
			Spec:     spec,
		}
		want.Init() // Critical: Register for events
		return NewSilencerWant(want)
	})
}
