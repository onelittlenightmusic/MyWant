package mywant

import "time"

// effect_bridge.go — triggering a want, and the effect a trigger plays.
//
// A want that acts on an input (a button, a fireworks, a chime) is triggered
// one way, whoever triggers it: TriggerWant drops the payload into its
// webhook_payload, for its Progress to consume (ConsumeWebhookAction). A
// press in a browser (POST /webhooks/{id}), a place arrived at
// (place_arrival), a want that heard another (spec.hear) — all come here.
//
// A want of an effect type (category effect: fireworks, chime, heart, …) is
// also seen going off, on every board: TriggerWant plays its effect through
// OnBoardEffect, which the server registers as a moment of kind effect (the
// board's one event for what just happened). Over the character who set it
// off when the payload names one (characterId: the one who pressed, the one
// who arrived), else over the want itself (a want that heard another).

// OnBoardEffect, when set by the server, plays effectType over a want or a
// character (`on`: "want" | "character") on every board.
var OnBoardEffect func(on, id, effectType string)

// EffectCategory is the category of the want types that are effects.
const EffectCategory = "effect"

// TriggerWant hands payload to w as its webhook would, and plays its effect
// when it is one.
func TriggerWant(w *Want, payload map[string]any) {
	if w == nil {
		return
	}
	StoreStateMulti(w, map[string]any{
		"webhook_payload": payload,
		// Nanoseconds: every trigger differs from the last, so none dedupes.
		"webhook_received_at": time.Now().Format(time.RFC3339Nano),
	})
	if !IsEffectWant(w) || OnBoardEffect == nil {
		return
	}
	if by, _ := payload["characterId"].(string); by != "" {
		OnBoardEffect("character", by, w.Metadata.Type)
		return
	}
	OnBoardEffect("want", w.Metadata.ID, w.Metadata.Type)
}

// TriggerWantByID is TriggerWant for a want known by its id.
func TriggerWantByID(id string, payload map[string]any) {
	cb := GetGlobalChainBuilder()
	if cb == nil {
		return
	}
	if w, _, ok := cb.FindWantByID(id); ok {
		TriggerWant(w, payload)
	}
}

// IsEffectWant: w's type is an effect (category effect).
func IsEffectWant(w *Want) bool {
	return w != nil && w.WantTypeDefinition != nil && w.WantTypeDefinition.Metadata.Category == EffectCategory
}
