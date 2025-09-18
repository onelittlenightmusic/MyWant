package mywant

import (
	"fmt"
	"mywant/src/chain"
)

type HotelWant struct {
	Want *Want
}

func (hw *HotelWant) Exec(using []chain.Chan, outputs []chain.Chan) bool {
	fmt.Printf("🏨 Hotel %s starting reservation process\n", hw.Want.Metadata.Name)
	hw.Want.SetStatus(WantStatusRunning)

	// Begin exec cycle for state management
	hw.Want.BeginExecCycle()
	defer hw.Want.EndExecCycle()

	// Execute agents based on requirements
	if err := hw.Want.ExecuteAgents(); err != nil {
		fmt.Printf("❌ Hotel %s failed to execute agents: %v\n", hw.Want.Metadata.Name, err)
		hw.Want.SetStatus(WantStatusFailed)
		return false
	}

	// Store initial hotel state
	hotelType := "standard"
	if ht, exists := hw.Want.Spec.Params["hotel_type"]; exists {
		if htStr, ok := ht.(string); ok {
			hotelType = htStr
		}
	}

	checkIn := "2025-09-20"
	if ci, exists := hw.Want.Spec.Params["check_in"]; exists {
		if ciStr, ok := ci.(string); ok {
			checkIn = ciStr
		}
	}

	checkOut := "2025-09-22"
	if co, exists := hw.Want.Spec.Params["check_out"]; exists {
		if coStr, ok := co.(string); ok {
			checkOut = coStr
		}
	}

	hw.Want.StoreState("hotel_type", hotelType)
	hw.Want.StoreState("check_in", checkIn)
	hw.Want.StoreState("check_out", checkOut)
	hw.Want.StoreState("status", "processing")

	fmt.Printf("🏨 Hotel %s configured: type=%s, check_in=%s, check_out=%s\n",
		hw.Want.Metadata.Name, hotelType, checkIn, checkOut)

	// Simulate some processing time for the hotel want
	// In a real implementation, this might wait for agents to complete
	fmt.Printf("🏨 Hotel %s waiting for agents to process reservation\n", hw.Want.Metadata.Name)

	hw.Want.SetStatus(WantStatusCompleted)
	return true
}

func (hw *HotelWant) GetWant() *Want {
	return hw.Want
}

func RegisterHotelWantTypes(builder *ChainBuilder, agentRegistry *AgentRegistry) {
	builder.RegisterWantType("hotel", func(metadata Metadata, spec WantSpec) interface{} {
		want := &Want{
			Metadata: metadata,
			Spec:     spec,
			State:    make(map[string]interface{}),
		}
		if agentRegistry != nil {
			want.SetAgentRegistry(agentRegistry)
		}
		return &HotelWant{Want: want}
	})
}
