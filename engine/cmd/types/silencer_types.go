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
	})

	// Register required capability for processing reactions
	s.Spec.Requires = []string{"reaction_auto_approval"}
}

// IsAchieved - Silencers are processors, they stay active to process stream
func (s *SilencerWant) IsAchieved() bool {
	return false
}

// Progress implements Progressable for SilencerWant
func (s *SilencerWant) Progress() {
	// Try to get a packet from input channels
	// Non-blocking check for packets
	index, data, ok := s.Use(0)
	if !ok {
		// Log occasionally or based on some condition if needed
		return
	}

	s.StoreLog("[SILENCER] Received packet from channel %d: %v", index, data)
	s.StoreLog("[SILENCER] Received packet for want %s from channel %d", s.Metadata.Name, index)

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

	reactionID, ok := packet["reaction_id"].(string)
	if !ok || reactionID == "" {
		s.StoreLog("[SILENCER] ERROR: Missing or invalid reaction_id in packet")
		return
	}

	reactionType := "internal"
	if rt, ok := packet["reaction_type"].(string); ok {
		reactionType = rt
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
		return NewSilencerWant(want)
	})
}
