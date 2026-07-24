package mywant

// OnCharacterEffectFire, when set by the server, plays an effect on a
// character's cursor so every client animates it. It is the seam server-side
// triggers use to reach the cursor stream without the types package depending
// on the server package: the server registers it (see RegisterEffectBridge),
// and wants such as place_arrival call FireCharacterEffect through it.
var OnCharacterEffectFire func(characterID, effectType string)

// FireCharacterEffect plays effectType on characterID's cursor if a handler is
// registered; a no-op otherwise (e.g. in tests, or before server startup).
func FireCharacterEffect(characterID, effectType string) {
	if OnCharacterEffectFire != nil {
		OnCharacterEffectFire(characterID, effectType)
	}
}
